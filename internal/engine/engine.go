package engine

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/rain/every-sync/internal/logger"
	"github.com/rain/every-sync/internal/provider"
	"github.com/rain/every-sync/internal/store"
)

type Direction string

const (
	DirectionUp   Direction = "up"
	DirectionDown Direction = "down"
	DirectionBoth Direction = "both"
)

type TaskType string

const (
	TaskUpload    TaskType = "upload"
	TaskDownload  TaskType = "download"
	TaskDelete    TaskType = "delete"
	TaskMove      TaskType = "move"
	TaskHash      TaskType = "hash"
	TaskVirtual   TaskType = "virtual"
	TaskConflict  TaskType = "conflict"
	TaskCreateDir TaskType = "create_dir"
	TaskDeleteDir TaskType = "delete_dir"
)

const partialSuffix = ".every-sync.part"

// FileListEntry represents a single file or directory entry in a file listing response.
type FileListEntry struct {
	Name       string     `json:"name"`
	Path       string     `json:"path"`
	Size       int64      `json:"size"`
	ModTime    *time.Time `json:"mod_time,omitempty"`
	IsDir      bool       `json:"is_dir"`
	SyncState  string     `json:"sync_state"`
	LocalHash  string     `json:"local_hash,omitempty"`
	RemoteHash string     `json:"remote_hash,omitempty"`
	Selected   bool       `json:"selected,omitempty"` // only meaningful for directories
}

// FileListResponse is the JSON response for the file listing API.
type FileListResponse struct {
	Path    string           `json:"path"`
	Entries []*FileListEntry `json:"entries"`
}

type SyncTask struct {
	ID           string
	Type         TaskType
	PairID       int64
	Path         string
	Priority     int
	Retries      int
	Target       string // "local", "remote", or "db_cleanup"
	ConflictType string // "modify_delete", "delete_modify", "modify_modify", etc.
	CycleID      uint64 // identifies which sync cycle this task belongs to
}

type TaskResult struct {
	Task  SyncTask
	Error error
}

type Config struct {
	MaxWorkers      int
	UploadWorkers   int
	DownloadWorkers int
	QueueSize       int
	RetryMax        int
	RetryDelay      time.Duration
	ScanInterval    time.Duration
	UploadLimit     int64
	DownloadLimit   int64
	ChunkSize       int64
	ChunkThreshold  int64
	WebhookURL      string
	EmailSMTPAddr   string
	EmailUsername   string
	EmailPassword   string
	EmailFrom       string
	EmailTo         []string
	DryRun          bool
}

func DefaultConfig() Config {
	return Config{
		MaxWorkers:      runtime.NumCPU() * 2,
		UploadWorkers:   4,
		DownloadWorkers: 8,
		QueueSize:       10000,
		RetryMax:        3,
		RetryDelay:      5 * time.Second,
		ScanInterval:    5 * time.Minute,
		ChunkSize:       8 * 1024 * 1024,
		ChunkThreshold:  16 * 1024 * 1024,
	}
}

type Event struct {
	Type             string    `json:"type"`
	Time             time.Time `json:"time"`
	PairID           int64     `json:"pair_id,omitempty"`
	PairName         string    `json:"pair_name,omitempty"`
	TaskType         string    `json:"task_type,omitempty"`
	Path             string    `json:"path,omitempty"`
	Pending          int64     `json:"pending"`
	Error            string    `json:"error,omitempty"`
	Message          string    `json:"message,omitempty"`
	Direction        string    `json:"direction,omitempty"`
	BytesTransferred int64     `json:"bytes_transferred,omitempty"`
	BytesTotal       int64     `json:"bytes_total,omitempty"`
	FilesSynced      int       `json:"files_synced,omitempty"`
	FilesTotal       int       `json:"files_total,omitempty"`
}

type Status struct {
	Running         bool             `json:"running"`
	StartedAt       *time.Time       `json:"started_at,omitempty"`
	RegisteredPairs int              `json:"registered_pairs"`
	Pending         int64            `json:"pending"`
	MaxWorkers      int              `json:"max_workers"`
	ScanInterval    string           `json:"scan_interval"`
	UploadLimit     int64            `json:"upload_limit"`
	DownloadLimit   int64            `json:"download_limit"`
	ChunkSize       int64            `json:"chunk_size"`
	ChunkThreshold  int64            `json:"chunk_threshold"`
	Stats           *store.SyncStats `json:"stats,omitempty"`
	Pairs           []PairStatus     `json:"pairs"`
}

type PairStatus struct {
	ID                 int64                 `json:"id"`
	Name               string                `json:"name"`
	Direction          string                `json:"direction"`
	Mode               string                `json:"mode"`
	Enabled            bool                  `json:"enabled"`
	Provider           string                `json:"provider"`
	LocalPath          string                `json:"local_path"`
	RemotePath         string                `json:"remote_path"`
	IncludePatterns    string                `json:"include_patterns"`
	ExcludePatterns    string                `json:"exclude_patterns"`
	ConflictStrategy   string                `json:"conflict_strategy"`
	ResumableUpload    bool                  `json:"resumable_upload"`
	ResumableDownload  bool                  `json:"resumable_download"`
	LocalCapabilities  provider.Capabilities `json:"local_capabilities"`
	RemoteCapabilities provider.Capabilities `json:"remote_capabilities"`
}

// PairRegistrar creates providers for a sync pair.
type PairRegistrar func(pair *store.SyncPair) (provider.Provider, provider.Provider, error)

type Engine struct {
	store     *store.Store
	config    Config
	locals    map[int64]provider.Provider // pairID -> local provider
	remotes   map[int64]provider.Provider // pairID -> remote provider
	pairs     map[int64]*store.SyncPair
	registrar PairRegistrar

	taskQueue chan SyncTask
	results   chan TaskResult

	pending   int64 // atomic counter for pending tasks
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startedAt *time.Time
	running   bool

	subsMu sync.Mutex
	subs   map[chan Event]struct{}

	// Per-pair file sync progress tracking for current sync cycle.
	pairFilesSynced map[int64]int    // pairID -> count of synced files in current sync cycle
	pairFilesTotal  map[int64]int    // pairID -> total files to sync
	pairPending     map[int64]int64  // pairID -> number of pending tasks for this pair's current sync
	pairDirection   map[int64]string // pairID -> direction of current sync
	pairCycle       map[int64]uint64 // pairID -> current sync cycle ID
	pairFailed      map[int64]bool   // pairID -> whether current sync cycle had a permanent failure
	pairSyncing     map[int64]bool   // pairID -> whether a sync is in progress for this pair
	syncCycle       uint64           // monotonically increasing sync cycle counter

	progress *ProgressTracker
}

func New(s *store.Store, cfg Config) *Engine {
	if cfg.MaxWorkers == 0 {
		cfg.MaxWorkers = runtime.NumCPU() * 2
	}
	return &Engine{
		store:           s,
		config:          cfg,
		locals:          make(map[int64]provider.Provider),
		remotes:         make(map[int64]provider.Provider),
		pairs:           make(map[int64]*store.SyncPair),
		taskQueue:       make(chan SyncTask, cfg.QueueSize),
		results:         make(chan TaskResult, cfg.QueueSize),
		subs:            make(map[chan Event]struct{}),
		pairFilesSynced: make(map[int64]int),
		pairFilesTotal:  make(map[int64]int),
		pairPending:     make(map[int64]int64),
		pairDirection:   make(map[int64]string),
		pairCycle:       make(map[int64]uint64),
		pairFailed:      make(map[int64]bool),
		pairSyncing:     make(map[int64]bool),
		progress:        NewProgressTracker(),
	}
}

// WithRegistrar sets the callback used to create providers for dynamic pair registration.
func (e *Engine) WithRegistrar(r PairRegistrar) *Engine {
	e.registrar = r
	return e
}

func (e *Engine) Progress() []PairProgressSnapshot {
	return e.progress.Snapshots()
}

func (e *Engine) SyncRecords(limit int) []SyncRecord {
	return e.progress.Records(limit)
}

// RegisterPair binds providers to a sync pair.
func (e *Engine) RegisterPair(pair *store.SyncPair, local, remote provider.Provider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pairs[pair.ID] = pair
	e.locals[pair.ID] = local
	e.remotes[pair.ID] = remote
	logger.L.Info().Int64("pair_id", pair.ID).Str("name", pair.Name).Msg("pair registered")
	e.broadcast(Event{Type: "pair_registered", PairID: pair.ID, PairName: pair.Name})
	if e.ctx != nil {
		e.startPairWatcherLocked(pair.ID, pair, local)
	}
}

// UnregisterPair removes a sync pair from the engine.
func (e *Engine) UnregisterPair(pairID int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if p, ok := e.pairs[pairID]; ok {
		logger.L.Info().Int64("pair_id", pairID).Str("name", p.Name).Msg("pair unregistered")
	}
	local := e.locals[pairID]
	remote := e.remotes[pairID]
	delete(e.pairs, pairID)
	delete(e.locals, pairID)
	delete(e.remotes, pairID)
	closeProvider(local)
	closeProvider(remote)
	e.broadcast(Event{Type: "pair_unregistered", PairID: pairID})
}

// RefreshPairs reloads pairs from DB and updates changed pair registrations.
func (e *Engine) RefreshPairs() error {
	return e.refreshPairs(false)
}

// RefreshAllPairs reloads pairs from DB and recreates providers for enabled pairs.
func (e *Engine) RefreshAllPairs() error {
	return e.refreshPairs(true)
}

// refreshPairs reloads pairs from DB, registers enabled pairs, and unregisters disabled ones.
func (e *Engine) refreshPairs(force bool) error {
	if e.registrar == nil {
		return nil
	}

	pairs, err := e.store.ListSyncPairs()
	if err != nil {
		return fmt.Errorf("list pairs: %w", err)
	}

	e.mu.Lock()
	registered := make(map[int64]bool, len(e.pairs))
	for id := range e.pairs {
		registered[id] = true
	}
	e.mu.Unlock()

	for _, pair := range pairs {
		if pair.Enabled {
			e.mu.RLock()
			current, exists := e.pairs[pair.ID]
			e.mu.RUnlock()

			if exists && !force && syncPairRuntimeEqual(current, pair) {
				continue
			}

			if e.registrar != nil {
				local, remote, err := e.registrar(pair)
				if err != nil {
					logger.L.Error().Err(err).Int64("pair_id", pair.ID).Str("name", pair.Name).Msg("failed to create providers for pair")
					continue
				}
				if exists {
					e.replacePair(pair, local, remote)
				} else {
					e.RegisterPair(pair, local, remote)
				}
			}
		} else {
			e.mu.RLock()
			_, exists := e.pairs[pair.ID]
			e.mu.RUnlock()

			if exists {
				e.UnregisterPair(pair.ID)
			}
		}
	}

	return nil
}

func (e *Engine) replacePair(pair *store.SyncPair, local, remote provider.Provider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	oldLocal := e.locals[pair.ID]
	oldRemote := e.remotes[pair.ID]
	closeProvider(oldLocal)
	closeProvider(oldRemote)
	e.pairs[pair.ID] = pair
	e.locals[pair.ID] = local
	e.remotes[pair.ID] = remote
	logger.L.Info().Int64("pair_id", pair.ID).Str("name", pair.Name).Msg("pair refreshed")
	e.broadcast(Event{Type: "pair_refreshed", PairID: pair.ID, PairName: pair.Name})
	if e.ctx != nil {
		e.startPairWatcherLocked(pair.ID, pair, local)
	}
}

func closeProvider(p provider.Provider) {
	if p != nil {
		_ = p.Close()
	}
}

func syncPairRuntimeEqual(a, b *store.SyncPair) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name &&
		a.LocalPath == b.LocalPath &&
		a.RemotePath == b.RemotePath &&
		a.Provider == b.Provider &&
		a.Mode == b.Mode &&
		a.Direction == b.Direction &&
		a.Enabled == b.Enabled &&
		a.Schedule == b.Schedule &&
		a.IncludePatterns == b.IncludePatterns &&
		a.ExcludePatterns == b.ExcludePatterns &&
		a.ConflictStrategy == b.ConflictStrategy &&
		a.SelectedFolders == b.SelectedFolders
}

// ListPairFiles returns a one-level-deep file listing for the given pair and directory path.
// The side parameter determines which provider to query ("local" or "remote").
// Each entry includes sync state from the database and selected status for directories.
func (e *Engine) ListPairFiles(ctx context.Context, pairID int64, dirPath, side string) ([]*FileListEntry, error) {
	e.mu.RLock()
	pair := e.pairs[pairID]
	var p provider.Provider
	if side == "remote" {
		p = e.remotes[pairID]
	} else {
		p = e.locals[pairID]
	}
	e.mu.RUnlock()

	if pair == nil || p == nil {
		return nil, fmt.Errorf("pair %d not found or not enabled", pairID)
	}

	entries, err := p.ListDir(ctx, dirPath)
	if err != nil {
		return nil, fmt.Errorf("list directory %s: %w", dirPath, err)
	}

	// Load DB entries to get sync_state for each file.
	dbEntries, _ := e.store.ListFileEntriesByPair(pairID)
	entryMap := make(map[string]*store.FileEntry, len(dbEntries))
	for _, de := range dbEntries {
		entryMap[path.Clean(de.Path)] = de
	}

	// Parse selected folders for directory selection markers.
	var selectedFolders []string
	if pair.SelectedFolders != "" && pair.SelectedFolders != "[]" {
		_ = json.Unmarshal([]byte(pair.SelectedFolders), &selectedFolders)
	}

	result := make([]*FileListEntry, 0, len(entries))
	for _, meta := range entries {
		key := path.Clean(meta.Path)
		dbEntry := entryMap[key]

		fle := &FileListEntry{
			Name:    path.Base(key),
			Path:    key,
			Size:    meta.Size,
			ModTime: &meta.ModTime,
			IsDir:   meta.IsDir,
		}

		if dbEntry != nil {
			fle.SyncState = dbEntry.SyncState
			fle.LocalHash = dbEntry.LocalHash
			fle.RemoteHash = dbEntry.RemoteHash
		}

		if meta.IsDir {
			for _, f := range selectedFolders {
				if f == "" {
					continue
				}
				// Mark selected if the folder matches a selected folder exactly,
				// is a parent of a selected folder, or is itself a selected folder.
				cleanKey := strings.TrimPrefix(key, "/")
				if cleanKey == f || strings.HasPrefix(f, cleanKey+"/") || strings.HasPrefix(cleanKey, f+"/") {
					fle.Selected = true
					break
				}
			}
		}

		result = append(result, fle)
	}

	return result, nil
}

func (e *Engine) Start(parent context.Context) error {
	e.mu.Lock()
	e.ctx, e.cancel = context.WithCancel(parent)
	now := time.Now()
	e.startedAt = &now
	e.running = true
	e.mu.Unlock()

	for i := 0; i < e.config.MaxWorkers; i++ {
		e.wg.Add(1)
		go e.worker(i)
	}

	e.wg.Add(1)
	go e.processResults()

	e.wg.Add(1)
	go e.periodicScan()

	e.mu.RLock()
	for id, pair := range e.pairs {
		e.startPairWatcherLocked(id, pair, e.locals[id])
	}
	e.mu.RUnlock()

	logger.L.Info().Int("workers", e.config.MaxWorkers).Dur("scan_interval", e.config.ScanInterval).Msg("engine started")
	e.broadcast(Event{Type: "engine_started", Message: "engine started"})
	return nil
}

// Stop gracefully shuts down the engine.
func (e *Engine) Stop() {
	e.mu.RLock()
	cancel := e.cancel
	e.mu.RUnlock()

	if cancel != nil {
		cancel()
	}
	e.wg.Wait()
	e.mu.Lock()
	for _, p := range e.locals {
		_ = p.Close()
	}
	for _, p := range e.remotes {
		_ = p.Close()
	}
	e.running = false
	e.mu.Unlock()
	logger.L.Info().Msg("engine stopped")
	e.broadcast(Event{Type: "engine_stopped", Message: "engine stopped"})
}

func (e *Engine) Status() Status {
	e.mu.RLock()
	defer e.mu.RUnlock()

	pairs := make([]PairStatus, 0, len(e.pairs))
	for id, pair := range e.pairs {
		localCaps := provider.DetectCapabilities(e.locals[id])
		remoteCaps := provider.DetectCapabilities(e.remotes[id])
		pairs = append(pairs, PairStatus{
			ID:                 pair.ID,
			Name:               pair.Name,
			Direction:          pair.Direction,
			Mode:               pair.Mode,
			Enabled:            pair.Enabled,
			Provider:           pair.Provider,
			LocalPath:          pair.LocalPath,
			RemotePath:         pair.RemotePath,
			IncludePatterns:    pair.IncludePatterns,
			ExcludePatterns:    pair.ExcludePatterns,
			ConflictStrategy:   pair.ConflictStrategy,
			ResumableUpload:    localCaps.RangeRead && remoteCaps.ResumeWrite,
			ResumableDownload:  remoteCaps.RangeRead && localCaps.ResumeWrite,
			LocalCapabilities:  localCaps,
			RemoteCapabilities: remoteCaps,
		})
	}

	stats, _ := e.store.GetSyncStats()
	return Status{
		Running:         e.running,
		StartedAt:       e.startedAt,
		RegisteredPairs: len(e.pairs),
		Pending:         atomic.LoadInt64(&e.pending),
		MaxWorkers:      e.config.MaxWorkers,
		ScanInterval:    e.config.ScanInterval.String(),
		UploadLimit:     e.config.UploadLimit,
		DownloadLimit:   e.config.DownloadLimit,
		ChunkSize:       e.config.ChunkSize,
		ChunkThreshold:  e.config.ChunkThreshold,
		Stats:           stats,
		Pairs:           pairs,
	}
}

func (e *Engine) Subscribe(ctx context.Context) <-chan Event {
	ch := make(chan Event, 32)
	e.subsMu.Lock()
	e.subs[ch] = struct{}{}
	e.subsMu.Unlock()

	ch <- Event{Type: "snapshot", Time: time.Now(), Pending: atomic.LoadInt64(&e.pending)}

	go func() {
		<-ctx.Done()
		e.subsMu.Lock()
		delete(e.subs, ch)
		e.subsMu.Unlock()
		close(ch)
	}()

	return ch
}

// Drain waits for all pending tasks to complete or until timeout.
func (e *Engine) Drain(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for atomic.LoadInt64(&e.pending) > 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
}

// SyncPair triggers an immediate sync for a specific pair.
func (e *Engine) SyncPair(ctx context.Context, pairID int64, direction string) error {
	e.mu.RLock()
	pair, ok := e.pairs[pairID]
	if !ok {
		e.mu.RUnlock()
		return fmt.Errorf("sync pair %d not found", pairID)
	}
	local := e.locals[pairID]
	remote := e.remotes[pairID]
	e.mu.RUnlock()

	dir, err := ResolveDirection(direction, pair.Direction)
	if err != nil {
		return err
	}

	// Virtual mode always forces bidirectional sync so local uploads work
	// alongside remote virtualization.
	if isVirtualMode(pair) {
		dir = DirectionBoth
	}

	e.mu.Lock()
	if e.pairSyncing[pair.ID] {
		e.mu.Unlock()
		return fmt.Errorf("sync pair %d is already syncing", pair.ID)
	}
	e.pairFilesSynced[pair.ID] = 0
	e.pairFilesTotal[pair.ID] = 0
	e.pairDirection[pair.ID] = string(dir)
	e.syncCycle++
	cycleID := e.syncCycle
	e.pairCycle[pair.ID] = cycleID
	e.pairFailed[pair.ID] = false
	e.pairSyncing[pair.ID] = true
	e.mu.Unlock()

	e.progress.StartSync(pair.ID, pair.Name, string(dir), 0)

	e.broadcast(Event{Type: "sync_started", PairID: pair.ID, PairName: pair.Name, Direction: string(dir)})

	// Wrap syncOnePair to inject cycleID into tasks
	if err := e.syncOnePairWithCycle(ctx, pair, local, remote, dir, cycleID); err != nil {
		e.mu.Lock()
		e.pairSyncing[pair.ID] = false
		e.mu.Unlock()
		e.progress.FailSync(pair.ID, err.Error())
		e.broadcast(Event{Type: "sync_failed", PairID: pair.ID, PairName: pair.Name, Direction: string(dir), Error: err.Error()})
		return err
	}

	return nil
}

// SyncAll triggers an immediate sync for all registered pairs.
func (e *Engine) SyncAll(ctx context.Context) error {
	e.mu.RLock()
	pairIDs := make([]int64, 0, len(e.pairs))
	for id := range e.pairs {
		pairIDs = append(pairIDs, id)
	}
	e.mu.RUnlock()

	for _, id := range pairIDs {
		if err := e.SyncPair(ctx, id, ""); err != nil {
			logger.L.Error().Err(err).Int64("pair_id", id).Msg("sync pair failed")
		}
	}
	return nil
}

// MaterializeVirtual downloads one virtual file to the local side on demand.
func (e *Engine) MaterializeVirtual(ctx context.Context, pairID int64, filePath string) error {
	e.mu.RLock()
	pair, ok := e.pairs[pairID]
	local := e.locals[pairID]
	remote := e.remotes[pairID]
	e.mu.RUnlock()
	if !ok || local == nil || remote == nil {
		return fmt.Errorf("sync pair %d not found", pairID)
	}
	cleaned := path.Clean("/" + filePath)
	if !pathAllowed(cleaned, splitPatterns(pair.IncludePatterns), splitPatterns(pair.ExcludePatterns)) {
		return fmt.Errorf("path %s is filtered out by selective rules", cleaned)
	}
	if err := e.doDownload(ctx, pair, local, remote, cleaned); err != nil {
		return err
	}
	_ = e.store.AddSyncStats(0, 0, 0, 0, 1, 0)
	e.broadcast(Event{Type: "file_materialized", PairID: pair.ID, PairName: pair.Name, Path: cleaned})
	return nil
}

// ResolveConflict applies a conflict strategy to a recorded conflict.
func (e *Engine) ResolveConflict(ctx context.Context, conflictID int64, strategy string) error {
	conflict, err := e.store.GetConflict(conflictID)
	if err != nil {
		return err
	}
	if conflict == nil {
		return fmt.Errorf("conflict %d not found", conflictID)
	}
	if conflict.Status != "open" {
		return nil
	}

	e.mu.RLock()
	pair := e.pairs[conflict.SyncPairID]
	local := e.locals[conflict.SyncPairID]
	remote := e.remotes[conflict.SyncPairID]
	e.mu.RUnlock()
	if pair == nil || local == nil || remote == nil {
		return fmt.Errorf("providers not found for pair %d", conflict.SyncPairID)
	}

	resolution := normalizeConflictStrategy(strategy)
	if strings.EqualFold(strings.TrimSpace(strategy), "skip") {
		resolution = "skip"
	}
	switch resolution {
	case "local_wins":
		if err := e.doUpload(ctx, pair, local, remote, conflict.Path); err != nil {
			return err
		}
	case "remote_wins":
		if err := e.doDownload(ctx, pair, local, remote, conflict.Path); err != nil {
			return err
		}
	case "latest_wins":
		localMeta, localErr := local.Stat(ctx, conflict.Path)
		remoteMeta, remoteErr := remote.Stat(ctx, conflict.Path)
		if localErr != nil || remoteErr != nil {
			return fmt.Errorf("stat conflict sides: local=%v remote=%v", localErr, remoteErr)
		}
		tasks := latestWinsTask(pair.ID, conflict.Path, localMeta, remoteMeta)
		if len(tasks) == 0 {
			return e.store.ResolveConflict(conflictID, resolution)
		}
		if err := e.executeTask(ctx, tasks[0]); err != nil {
			return err
		}
	case "keep_both":
		// Record both versions in file_versions for history.
		e.recordProviderVersion(ctx, pair.ID, conflict.Path, "local", local)
		e.recordProviderVersion(ctx, pair.ID, conflict.Path, "remote", remote)
		// Apply the newer version to both sides.
		localMeta, localErr := local.Stat(ctx, conflict.Path)
		remoteMeta, remoteErr := remote.Stat(ctx, conflict.Path)
		if localErr != nil || remoteErr != nil {
			return fmt.Errorf("stat conflict sides: local=%v remote=%v", localErr, remoteErr)
		}
		if localMeta.ModTime.After(remoteMeta.ModTime) {
			if err := e.doUpload(ctx, pair, local, remote, conflict.Path); err != nil {
				return err
			}
		} else {
			if err := e.doDownload(ctx, pair, local, remote, conflict.Path); err != nil {
				return err
			}
		}
	case "rename":
		// Determine loser and winner by ModTime, rename the loser with a
		// conflict suffix, and apply the winner to both sides.
		localMeta, localErr := local.Stat(ctx, conflict.Path)
		remoteMeta, remoteErr := remote.Stat(ctx, conflict.Path)
		if localErr != nil || remoteErr != nil {
			return fmt.Errorf("stat conflict sides: local=%v remote=%v", localErr, remoteErr)
		}
		ext := path.Ext(conflict.Path)
		base := strings.TrimSuffix(conflict.Path, ext)
		dateStr := time.Now().Format("2006-01-02")
		renamedPath := fmt.Sprintf("%s (冲突副本 %s)%s", base, dateStr, ext)

		if localMeta.ModTime.After(remoteMeta.ModTime) {
			// Remote is loser: rename remote file, upload local version.
			if err := remote.MoveFile(ctx, conflict.Path, renamedPath); err != nil {
				return fmt.Errorf("rename remote loser: %w", err)
			}
			if err := e.doUpload(ctx, pair, local, remote, conflict.Path); err != nil {
				return err
			}
		} else {
			// Local is loser: rename local file, download remote version.
			if err := local.MoveFile(ctx, conflict.Path, renamedPath); err != nil {
				return fmt.Errorf("rename local loser: %w", err)
			}
			if err := e.doDownload(ctx, pair, local, remote, conflict.Path); err != nil {
				return err
			}
		}
	case "skip", "manual":
		resolution = "skip"
	}

	if err := e.store.ResolveConflict(conflictID, resolution); err != nil {
		return err
	}
	e.broadcast(Event{Type: "conflict_resolved", PairID: pair.ID, PairName: pair.Name, Path: conflict.Path, Message: resolution})
	return nil
}

func (e *Engine) syncOnePairWithCycle(ctx context.Context, pair *store.SyncPair, local, remote provider.Provider, dir Direction, cycleID uint64) error {
	return e.syncOnePair(ctx, pair, local, remote, dir, cycleID)
}

func (e *Engine) syncOnePair(ctx context.Context, pair *store.SyncPair, local, remote provider.Provider, dir Direction, cycleID ...uint64) error {
	logger.L.Info().Str("pair", pair.Name).Str("direction", string(dir)).Msg("syncing pair")

	var localFiles, remoteFiles []*provider.FileMeta
	var err error

	localFiles, err = e.scanRecursive(ctx, local, "/")
	if err != nil {
		return fmt.Errorf("scan local: %w", err)
	}

	// Attempt incremental scan for remote side.
	// If the remote root's tag matches the cached value, skip the full remote scan
	// and reconstruct file list from DB entries.
	remoteRootMissing := false
	remoteFiles, err = e.scanRemote(ctx, remote, pair)
	if err != nil {
		if !isMissingRootScan(err) || dir == DirectionDown {
			return fmt.Errorf("scan remote: %w", err)
		}
		if err := remote.CreateDir(ctx, "/"); err != nil {
			return fmt.Errorf("create remote root: %w", err)
		}
		remoteRootMissing = true
		remoteFiles = nil
	}
	localFiles = filterPairFiles(pair, localFiles)
	remoteFiles = filterPairFiles(pair, remoteFiles)

	logger.L.Info().
		Str("pair", pair.Name).
		Int("local_files", len(localFiles)).
		Int("remote_files", len(remoteFiles)).
		Msg("scan complete")

	// Load DB entries for deletion detection
	dbEntries, _ := e.store.ListFileEntriesByPair(pair.ID)

	taskDirection := dir
	if remoteRootMissing && dir == DirectionBoth {
		taskDirection = DirectionUp
	}
	tasks := e.generateTasks(ctx, pair, localFiles, remoteFiles, dbEntries, taskDirection)
	taskPaths := make(map[string]bool, len(tasks))

	uploadCount, downloadCount, deleteCount := 0, 0, 0
	for _, t := range tasks {
		taskPaths[t.Path] = true
		switch t.Type {
		case TaskUpload:
			uploadCount++
		case TaskDownload:
			downloadCount++
		case TaskDelete:
			deleteCount++
		}
	}
	logger.L.Info().
		Str("pair", pair.Name).
		Int("uploads", uploadCount).
		Int("downloads", downloadCount).
		Int("deletes", deleteCount).
		Msg("tasks generated")

	// Track total files to sync for progress events.
	e.mu.Lock()
	e.pairFilesTotal[pair.ID] = uploadCount + downloadCount
	taskCount := int64(len(tasks))
	if taskCount > 0 {
		e.pairPending[pair.ID] = taskCount
	}
	e.mu.Unlock()

	e.progress.SetTotals(pair.ID, uploadCount+downloadCount, taskCount)

	if e.config.DryRun {
		for _, task := range tasks {
			logger.L.Info().
				Str("pair", pair.Name).
				Str("task", string(task.Type)).
				Str("path", task.Path).
				Str("delete_target", task.Target).
				Msg("dry run task")
		}
		return nil
	}

	if err := e.indexCleanFiles(ctx, pair, localFiles, remoteFiles, dbEntries, taskPaths, dir); err != nil {
		return err
	}

	// Broadcast sync_completed immediately if no tasks were generated.
	if len(tasks) == 0 {
		e.mu.Lock()
		e.pairSyncing[pair.ID] = false
		e.mu.Unlock()
		e.progress.FinishSync(pair.ID)
		e.broadcast(Event{Type: "sync_completed", PairID: pair.ID, PairName: pair.Name, Direction: string(dir), FilesSynced: 0, FilesTotal: 0})
	}

	for i := range tasks {
		if len(cycleID) > 0 {
			tasks[i].CycleID = cycleID[0]
		}
		atomic.AddInt64(&e.pending, 1)
		e.progress.QueueTask(tasks[i].PairID, string(tasks[i].Type), tasks[i].Path, taskProgressDirection(tasks[i]))
		select {
		case e.taskQueue <- tasks[i]:
			e.broadcast(Event{Type: "task_queued", PairID: tasks[i].PairID, PairName: pair.Name, TaskType: string(tasks[i].Type), Path: tasks[i].Path, Direction: taskProgressDirection(tasks[i]), Pending: atomic.LoadInt64(&e.pending), FilesTotal: uploadCount + downloadCount})
		case <-ctx.Done():
			atomic.AddInt64(&e.pending, -1)
			return ctx.Err()
		}
	}

	return nil
}

func isMissingRootScan(err error) bool {
	return errors.Is(err, provider.ErrNotFound) && strings.Contains(err.Error(), "list directory /:")
}

func taskProgressDirection(task SyncTask) string {
	switch task.Type {
	case TaskUpload:
		return "up"
	case TaskDownload, TaskVirtual:
		return "down"
	case TaskDelete, TaskDeleteDir:
		if task.Target == "local" {
			return "down"
		}
		if task.Target == "remote" {
			return "up"
		}
	case TaskCreateDir:
		if task.Target == "local" {
			return "down"
		}
		if task.Target == "remote" {
			return "up"
		}
	}
	return ""
}

func (e *Engine) scanRecursive(ctx context.Context, p provider.Provider, rootPath string) ([]*provider.FileMeta, error) {
	var result []*provider.FileMeta

	queue := []string{rootPath}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		entries, err := p.ListDir(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("list directory %s: %w", current, err)
		}

		for _, entry := range entries {
			if shouldSkipPath(entry.Path) {
				continue
			}
			if entry.IsDir {
				queue = append(queue, entry.Path)
			}
			result = append(result, entry)
		}
	}

	return result, nil
}

// scanRemote attempts an incremental scan for the remote provider.
// If the remote implements IncrementalLister and the root directory's tag matches
// the cached value, the full remote scan is skipped and the file list is
// reconstructed from DB entries. Otherwise a full recursive scan is performed.
func (e *Engine) scanRemote(ctx context.Context, remote provider.Provider, pair *store.SyncPair) ([]*provider.FileMeta, error) {
	lister, ok := remote.(provider.IncrementalLister)
	if !ok {
		return e.scanRecursive(ctx, remote, "/")
	}

	// Look up the cached root ETag from DB entries.
	// We store the root directory's tag as the RemoteEtag of the "/" entry.
	cachedTag := ""
	rootEntry, err := e.store.GetFileEntry(pair.ID, "/")
	if err == nil && rootEntry != nil && rootEntry.RemoteEtag != "" {
		cachedTag = rootEntry.RemoteEtag
	}

	_, unchanged, err := lister.IncrementalList(ctx, "/", cachedTag)
	if err != nil {
		return e.scanRecursive(ctx, remote, "/")
	}

	if !unchanged {
		// Root changed (or first scan) — update the cached tag and do a full
		// recursive scan so we also traverse subdirectories.
		tag, tagErr := remote.GetChangeToken(ctx, "/")
		if tagErr == nil {
			e.store.UpsertFileEntry(&store.FileEntry{
				Path:       "/",
				SyncPairID: pair.ID,
				RemoteEtag: tag,
				IsDir:      true,
				SyncState:  "synced",
			})
		}
		return e.scanRecursive(ctx, remote, "/")
	}

	// Root unchanged — reconstruct file list from DB.
	logger.L.Info().Str("pair", pair.Name).Msg("remote root unchanged, using cached entries")
	dbEntries, err := e.store.ListFileEntriesByPair(pair.ID)
	if err != nil {
		return e.scanRecursive(ctx, remote, "/")
	}

	result := make([]*provider.FileMeta, 0, len(dbEntries))
	for _, entry := range dbEntries {
		meta := &provider.FileMeta{
			Path:  entry.Path,
			IsDir: entry.IsDir,
			Size:  entry.RemoteSize,
		}
		if entry.RemoteMTime != nil {
			meta.ModTime = *entry.RemoteMTime
		}
		result = append(result, meta)
	}

	return result, nil
}

func shouldSkipPath(filePath string) bool {
	base := path.Base(path.Clean(filePath))
	return base == "Identifier" ||
		strings.Contains(base, ":Zone.Identifier") ||
		strings.HasPrefix(base, "Zone.Identifier") ||
		strings.HasSuffix(base, partialSuffix)
}

func (e *Engine) generateTasks(ctx context.Context, pair *store.SyncPair, localFiles, remoteFiles []*provider.FileMeta, dbEntries []*store.FileEntry, dir Direction) []SyncTask {
	var tasks []SyncTask

	localMap := make(map[string]*provider.FileMeta, len(localFiles))
	for _, f := range localFiles {
		key := path.Clean(f.Path)
		localMap[key] = f
	}

	remoteMap := make(map[string]*provider.FileMeta, len(remoteFiles))
	for _, f := range remoteFiles {
		key := path.Clean(f.Path)
		remoteMap[key] = f
	}

	entryMap := make(map[string]*store.FileEntry, len(dbEntries))
	for _, entry := range dbEntries {
		entryMap[path.Clean(entry.Path)] = entry
	}

	keys := make(map[string]struct{}, len(localMap)+len(remoteMap)+len(entryMap))
	for key := range localMap {
		keys[key] = struct{}{}
	}
	for key := range remoteMap {
		keys[key] = struct{}{}
	}
	for key := range entryMap {
		keys[key] = struct{}{}
	}

	for key := range keys {
		localMeta, hasLocal := localMap[key]
		remoteMeta, hasRemote := remoteMap[key]
		entry := entryMap[key]

		// Determine if this is a directory entry
		isDir := (hasLocal && localMeta.IsDir) || (hasRemote && remoteMeta.IsDir) || (entry != nil && entry.IsDir)

		switch dir {
		case DirectionUp:
			tasks = append(tasks, generateUpTasks(pair, key, localMeta, remoteMeta, entry, hasLocal, hasRemote, isDir)...)
		case DirectionDown:
			tasks = append(tasks, generateDownTasks(pair, key, localMeta, remoteMeta, entry, hasLocal, hasRemote, isDir)...)
		case DirectionBoth:
			tasks = append(tasks, e.generateBothTasks(ctx, pair, key, localMeta, remoteMeta, entry, hasLocal, hasRemote, isDir)...)
		}
	}

	// Sort tasks: TaskCreateDir first, then file tasks, then TaskDeleteDir last.
	sort.SliceStable(tasks, func(i, j int) bool {
		ti, tj := tasks[i], tasks[j]
		tiTier := taskTier(ti.Type)
		tjTier := taskTier(tj.Type)
		if tiTier != tjTier {
			return tiTier < tjTier
		}
		return ti.Priority < tj.Priority
	})

	return tasks
}

// taskTier returns the execution tier for a task type:
// 0 = create directories first, 1 = file operations, 2 = delete directories last.
func taskTier(t TaskType) int {
	switch t {
	case TaskCreateDir:
		return 0
	case TaskDeleteDir:
		return 2
	default:
		return 1
	}
}

func generateUpTasks(pair *store.SyncPair, key string, localMeta, remoteMeta *provider.FileMeta, entry *store.FileEntry, hasLocal, hasRemote bool, isDir bool) []SyncTask {
	if isDir {
		return generateDirUpTasks(pair, key, localMeta, remoteMeta, entry, hasLocal, hasRemote)
	}
	if hasLocal {
		if !hasRemote || !sameSnapshot(localMeta, remoteMeta) {
			if entry == nil || !metaMatchesEntry(localMeta, entry.LocalMTime, entry.LocalSize) || !metaMatchesEntry(remoteMeta, entry.RemoteMTime, entry.RemoteSize) {
				return []SyncTask{newTask(TaskUpload, pair.ID, key)}
			}
		}
		return nil
	}

	if hasRemote && isSynced(entry) && metaMatchesEntry(remoteMeta, entry.RemoteMTime, entry.RemoteSize) {
		return []SyncTask{newDeleteTask(pair.ID, key, "remote")}
	}
	return nil
}

func generateDownTasks(pair *store.SyncPair, key string, localMeta, remoteMeta *provider.FileMeta, entry *store.FileEntry, hasLocal, hasRemote bool, isDir bool) []SyncTask {
	if isDir {
		return generateDirDownTasks(pair, key, localMeta, remoteMeta, entry, hasLocal, hasRemote)
	}
	if hasRemote {
		if isVirtualMode(pair) && (!hasLocal || isVirtual(entry)) {
			if entry == nil || !metaMatchesEntry(remoteMeta, entry.RemoteMTime, entry.RemoteSize) || !isVirtual(entry) {
				return []SyncTask{newTask(TaskVirtual, pair.ID, key)}
			}
			return nil
		}
		if !hasLocal || !sameSnapshot(localMeta, remoteMeta) {
			if entry == nil || !metaMatchesEntry(localMeta, entry.LocalMTime, entry.LocalSize) || !metaMatchesEntry(remoteMeta, entry.RemoteMTime, entry.RemoteSize) {
				return []SyncTask{newTask(TaskDownload, pair.ID, key)}
			}
		}
		return nil
	}

	if hasLocal && isSynced(entry) && metaMatchesEntry(localMeta, entry.LocalMTime, entry.LocalSize) {
		return []SyncTask{newDeleteTask(pair.ID, key, "local")}
	}
	return nil
}

func (e *Engine) generateBothTasks(ctx context.Context, pair *store.SyncPair, key string, localMeta, remoteMeta *provider.FileMeta, entry *store.FileEntry, hasLocal, hasRemote bool, isDir bool) []SyncTask {
	if isDir {
		return generateDirBothTasks(pair, key, localMeta, remoteMeta, entry, hasLocal, hasRemote)
	}
	if hasLocal && hasRemote {
		if entry == nil || !isSynced(entry) {
			// First sync: both sides have the file, no synced DB record.
			// Compare content to determine if they are identical.
			equal, err := e.compareContent(ctx, e.locals[pair.ID], e.remotes[pair.ID], key)
			if err == nil && equal {
				// Content is identical — indexCleanFiles will mark as synced.
				return nil
			}
			// Content differs — apply conflict strategy.
			return conflictStrategyTasks(pair, key, localMeta, remoteMeta)
		}

		// Two-stage change detection.
		localResult, _ := e.detectChange(ctx, localMeta, entry, "local", e.locals[pair.ID], key)
		remoteResult, _ := e.detectChange(ctx, remoteMeta, entry, "remote", e.remotes[pair.ID], key)

		localChanged := localResult.changed
		remoteChanged := remoteResult.changed

		// When metadata changed but hash matched (false alarm), update the DB
		// entry's ModTime so we don't re-check on the next scan.
		if localResult.hash != "" && !localChanged {
			e.updateEntryHash(pair.ID, key, "local", localResult.hash, localMeta)
		}
		if remoteResult.hash != "" && !remoteChanged {
			e.updateEntryHash(pair.ID, key, "remote", remoteResult.hash, remoteMeta)
		}

		switch {
		case !localChanged && !remoteChanged:
			return nil
		case localChanged && !remoteChanged:
			return []SyncTask{newTask(TaskUpload, pair.ID, key)}
		case !localChanged && remoteChanged:
			// In virtual mode, re-virtualize instead of downloading when remote changes.
			if isVirtualMode(pair) {
				return []SyncTask{newTask(TaskVirtual, pair.ID, key)}
			}
			return []SyncTask{newTask(TaskDownload, pair.ID, key)}
		default:
			// Both changed: in virtual mode prefer uploading local change and re-virtualizing remote metadata.
			if isVirtualMode(pair) {
				return []SyncTask{newTask(TaskUpload, pair.ID, key)}
			}
			return conflictStrategyTasks(pair, key, localMeta, remoteMeta)
		}
	}

	// hasLocal && !hasRemote: local file exists, remote is gone.
	if hasLocal {
		if isSynced(entry) {
			// Check if local actually changed compared to the DB record.
			// If unchanged, remote initiated the delete — delete local copy.
			// If changed, local was modified while remote was deleted — conflict.
			localChanged := !metaMatchesEntry(localMeta, entry.LocalMTime, entry.LocalSize)
			if localChanged {
				return conflictWithTasks(pair, key, "modify_delete")
			}
			return []SyncTask{newDeleteTask(pair.ID, key, "local")}
		}
		return []SyncTask{newTask(TaskUpload, pair.ID, key)}
	}

	// !hasLocal && hasRemote: remote file exists, local is gone.
	if hasRemote {
		if isVirtualMode(pair) {
			if entry == nil || !metaMatchesEntry(remoteMeta, entry.RemoteMTime, entry.RemoteSize) || !isVirtual(entry) {
				return []SyncTask{newTask(TaskVirtual, pair.ID, key)}
			}
			return nil
		}
		if isSynced(entry) {
			// Check if remote actually changed compared to the DB record.
			// If unchanged, local initiated the delete — delete remote copy.
			// If changed, remote was modified while local was deleted — conflict.
			remoteChanged := !metaMatchesEntry(remoteMeta, entry.RemoteMTime, entry.RemoteSize)
			if remoteChanged {
				return conflictWithTasks(pair, key, "delete_modify")
			}
			return []SyncTask{newDeleteTask(pair.ID, key, "remote")}
		}
		return []SyncTask{newTask(TaskDownload, pair.ID, key)}
	}

	// !hasLocal && !hasRemote: both sides deleted the file.
	if entry != nil {
		// Both deleted — clean up the stale DB entry.
		return []SyncTask{newDeleteTask(pair.ID, key, "db_cleanup")}
	}
	return nil
}

// updateEntryHash updates the stored mtime and hash for a side when metadata
// changed but content did not (false alarm), preventing redundant hash
// computation on the next scan.
func (e *Engine) updateEntryHash(pairID int64, filePath, side, hash string, meta *provider.FileMeta) {
	if meta == nil {
		return
	}
	entry, err := e.store.GetFileEntry(pairID, filePath)
	if err != nil || entry == nil {
		return
	}
	switch side {
	case "local":
		entry.LocalMTime = &meta.ModTime
		entry.LocalHash = hash
	case "remote":
		entry.RemoteMTime = &meta.ModTime
		entry.RemoteHash = hash
	}
	_ = e.store.UpsertFileEntry(entry)
}

func conflictStrategyTasks(pair *store.SyncPair, key string, localMeta, remoteMeta *provider.FileMeta) []SyncTask {
	switch normalizeConflictStrategy(pair.ConflictStrategy) {
	case "manual", "keep_both", "rename":
		return []SyncTask{newTask(TaskConflict, pair.ID, key)}
	case "local_wins":
		return []SyncTask{newTask(TaskUpload, pair.ID, key)}
	case "remote_wins":
		return []SyncTask{newTask(TaskDownload, pair.ID, key)}
	default:
		return latestWinsTask(pair.ID, key, localMeta, remoteMeta)
	}
}

// compareContent reads file content from both providers, hashes them, and
// returns whether the content is identical.
func (e *Engine) compareContent(ctx context.Context, local, remote provider.Provider, filePath string) (bool, error) {
	localHash, err := e.computeHashViaProvider(ctx, local, filePath)
	if err != nil {
		return false, fmt.Errorf("compare local hash: %w", err)
	}
	remoteHash, err := e.computeHashViaProvider(ctx, remote, filePath)
	if err != nil {
		return false, fmt.Errorf("compare remote hash: %w", err)
	}
	return localHash == remoteHash, nil
}

// conflictWithTasks generates a conflict task with the specified conflict type.
func conflictWithTasks(pair *store.SyncPair, key, conflictType string) []SyncTask {
	task := newTask(TaskConflict, pair.ID, key)
	task.ConflictType = conflictType
	return []SyncTask{task}
}

func latestWinsTask(pairID int64, key string, localMeta, remoteMeta *provider.FileMeta) []SyncTask {
	if localMeta == nil || remoteMeta == nil {
		return nil
	}
	if localMeta.ModTime.After(remoteMeta.ModTime) {
		return []SyncTask{newTask(TaskUpload, pairID, key)}
	}
	if remoteMeta.ModTime.After(localMeta.ModTime) {
		return []SyncTask{newTask(TaskDownload, pairID, key)}
	}
	if localMeta.Size != remoteMeta.Size {
		return []SyncTask{newTask(TaskUpload, pairID, key)}
	}
	return nil
}

// --- Directory task generation ---

// generateDirUpTasks handles directory entries in "up" direction.
// Directories are synced via create_dir; deletion is handled by generateDirBothTasks
// in bidirectional mode. In unidirectional "up", a missing remote dir needs creating.
func generateDirUpTasks(pair *store.SyncPair, key string, localMeta, remoteMeta *provider.FileMeta, entry *store.FileEntry, hasLocal, hasRemote bool) []SyncTask {
	if hasLocal && !hasRemote {
		// Local has directory, remote doesn't -> create on remote
		return []SyncTask{newDirTask(TaskCreateDir, pair.ID, key, "remote")}
	}
	return nil
}

// generateDirDownTasks handles directory entries in "down" direction.
func generateDirDownTasks(pair *store.SyncPair, key string, localMeta, remoteMeta *provider.FileMeta, entry *store.FileEntry, hasLocal, hasRemote bool) []SyncTask {
	if hasRemote && !hasLocal {
		// Remote has directory, local doesn't -> create on local
		return []SyncTask{newDirTask(TaskCreateDir, pair.ID, key, "local")}
	}
	// Directory deleted on remote side: local still has it, was previously synced
	if !hasRemote && hasLocal && isSynced(entry) && !isRootPath(key) {
		return []SyncTask{newDirTask(TaskDeleteDir, pair.ID, key, "local")}
	}
	return nil
}

// generateDirBothTasks handles directory entries in "both" direction.
func generateDirBothTasks(pair *store.SyncPair, key string, localMeta, remoteMeta *provider.FileMeta, entry *store.FileEntry, hasLocal, hasRemote bool) []SyncTask {
	if hasLocal && !hasRemote {
		if isSynced(entry) && !isRootPath(key) {
			// Directory was synced but now deleted on remote -> delete on local
			return []SyncTask{newDirTask(TaskDeleteDir, pair.ID, key, "local")}
		}
		// New local directory -> create on remote
		return []SyncTask{newDirTask(TaskCreateDir, pair.ID, key, "remote")}
	}
	if hasRemote && !hasLocal {
		if isSynced(entry) && !isRootPath(key) {
			// Directory was synced but now deleted on local -> delete on remote
			return []SyncTask{newDirTask(TaskDeleteDir, pair.ID, key, "remote")}
		}
		// New remote directory -> create on local
		return []SyncTask{newDirTask(TaskCreateDir, pair.ID, key, "local")}
	}
	return nil
}

func newDirTask(taskType TaskType, pairID int64, key, target string) SyncTask {
	return SyncTask{
		Type:     taskType,
		PairID:   pairID,
		Path:     key,
		Priority: 0, // Lower priority so file tasks run first
		Target:   target,
	}
}

func isRootPath(p string) bool {
	return path.Clean(p) == "/" || path.Clean(p) == "."
}

func newTask(taskType TaskType, pairID int64, key string) SyncTask {
	return SyncTask{
		Type:     taskType,
		PairID:   pairID,
		Path:     key,
		Priority: 2,
	}
}

func newDeleteTask(pairID int64, key, target string) SyncTask {
	return SyncTask{
		Type:     TaskDelete,
		PairID:   pairID,
		Path:     key,
		Priority: 1,
		Target:   target,
	}
}

func ResolveDirection(requested, fallback string) (Direction, error) {
	dir := Direction(requested)
	if dir == "" {
		dir = Direction(fallback)
	}
	if dir == "" {
		dir = DirectionBoth
	}

	switch dir {
	case DirectionUp, DirectionDown, DirectionBoth:
		return dir, nil
	default:
		return "", fmt.Errorf("invalid sync direction %q: expected up, down, or both", dir)
	}
}

func isSynced(entry *store.FileEntry) bool {
	return entry != nil && entry.SyncState == "synced"
}

func isVirtual(entry *store.FileEntry) bool {
	return entry != nil && entry.SyncState == "virtual"
}

func isVirtualMode(pair *store.SyncPair) bool {
	return pair != nil && strings.EqualFold(pair.Mode, "virtual")
}

// isNormalMode returns true for "normal" mode (the unified mirror+selective mode).
// "mirror" is accepted as an alias for backward compatibility during transition.
func isNormalMode(pair *store.SyncPair) bool {
	if pair == nil {
		return false
	}
	m := strings.ToLower(pair.Mode)
	return m == "normal" || m == "mirror" || m == "selective"
}

// filterBySelectedFolders returns true if path should be synced based on
// the SelectedFolders configuration. Empty SelectedFolders means all files
// pass through (equivalent to the old mirror behavior).
func filterBySelectedFolders(pair *store.SyncPair, relativePath string) bool {
	if pair.SelectedFolders == "" || pair.SelectedFolders == "[]" {
		return true
	}
	var folders []string
	if err := json.Unmarshal([]byte(pair.SelectedFolders), &folders); err != nil {
		return true // fallback to all on parse error
	}
	if len(folders) == 0 {
		return true
	}
	cleaned := strings.TrimPrefix(path.Clean(relativePath), "/")
	for _, f := range folders {
		if f == "" {
			continue
		}
		if cleaned == f || strings.HasPrefix(cleaned, f+"/") {
			return true
		}
	}
	return false
}

// NormalizeSelectedFolders merges child paths into their parents.
// For example: ["docs/work/2024", "docs/work"] becomes ["docs/work"].
func NormalizeSelectedFolders(folders []string) []string {
	sort.Strings(folders)
	var result []string
	for _, f := range folders {
		if f == "" {
			continue
		}
		contained := false
		for _, existing := range result {
			if f == existing || strings.HasPrefix(f, existing+"/") {
				contained = true
				break
			}
		}
		if !contained {
			result = append(result, f)
		}
	}
	return result
}

func normalizeConflictStrategy(strategy string) string {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "manual", "local_wins", "remote_wins", "latest_wins", "keep_both", "rename":
		return strings.ToLower(strings.TrimSpace(strategy))
	default:
		return "latest_wins"
	}
}

func sameSnapshot(a, b *provider.FileMeta) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Size == b.Size && timesClose(a.ModTime, b.ModTime)
}

func metaMatchesEntry(meta *provider.FileMeta, recorded *time.Time, size int64) bool {
	if meta == nil || recorded == nil {
		return false
	}
	return meta.Size == size && timesClose(meta.ModTime, *recorded)
}

// computeHashViaProvider reads file content through a provider and computes an
// xxhash of the content. This works for both local and remote providers without
// needing direct filesystem access.
func (e *Engine) computeHashViaProvider(ctx context.Context, p provider.Provider, filePath string) (string, error) {
	reader, _, err := p.GetFile(ctx, filePath)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	h := xxhash.New()
	if _, err := io.Copy(h, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// changeDetectionResult holds the outcome of two-stage change detection.
type changeDetectionResult struct {
	// changed is true when the file has genuinely changed and a sync task should
	// be generated.
	changed bool
	// hash is the newly computed content hash (empty when metadata matched and
	// hash computation was skipped).
	hash string
}

// detectChange performs two-stage change detection:
//
// Stage 1 (metadata): compare Size and ModTime with the database record.
// If both match, the file has not changed — skip.
//
// Stage 2 (content hash): metadata differs, so compute the file's content hash
// and compare with the cached hash in the database. If hashes match it is a
// false alarm (e.g. file was touched). If hashes differ it is a real change.
//
// For the "remote" side where we cannot cheaply re-hash, the function falls
// back to pure metadata comparison when no cached hash is available.
func (e *Engine) detectChange(
	ctx context.Context,
	meta *provider.FileMeta,
	entry *store.FileEntry,
	side string,
	p provider.Provider,
	filePath string,
) (changeDetectionResult, error) {
	// Determine which recorded mtime/size/hash to compare against.
	var recorded *time.Time
	var size int64
	var cachedHash string
	switch side {
	case "local":
		recorded = entry.LocalMTime
		size = entry.LocalSize
		cachedHash = entry.LocalHash
	case "remote":
		recorded = entry.RemoteMTime
		size = entry.RemoteSize
		cachedHash = entry.RemoteHash
	}

	// Stage 1: metadata comparison — if both size and modtime match, no change.
	if metaMatchesEntry(meta, recorded, size) {
		return changeDetectionResult{changed: false}, nil
	}

	// Stage 2: content hash verification.
	// For local side always compute hash. For remote side, only compute hash if
	// a cached hash exists (first time we see the file, fall back to metadata).
	if side == "local" || (side == "remote" && cachedHash != "") {
		newHash, err := e.computeHashViaProvider(ctx, p, filePath)
		if err != nil {
			// Hash computation failed (e.g. file disappeared) — treat as changed
			// so the sync engine handles it.
			logger.L.Debug().Err(err).Str("side", side).Str("path", filePath).Msg("hash computation failed, treating as changed")
			return changeDetectionResult{changed: true}, nil
		}
		if newHash == cachedHash {
			// Metadata changed but content is the same — false alarm.
			return changeDetectionResult{changed: false, hash: newHash}, nil
		}
		// Content truly changed.
		return changeDetectionResult{changed: true, hash: newHash}, nil
	}

	// Remote side with no cached hash — fall back to metadata-only comparison.
	// Metadata already differed (we passed Stage 1), so it is a change.
	return changeDetectionResult{changed: true}, nil
}

func timesClose(a, b time.Time) bool {
	if a.Equal(b) {
		return true
	}
	diff := a.Sub(b)
	if diff < 0 {
		diff = -diff
	}
	return diff <= time.Second
}

func filterPairFiles(pair *store.SyncPair, files []*provider.FileMeta) []*provider.FileMeta {
	if pair == nil {
		return files
	}

	hasFolderFilter := pair.SelectedFolders != "" && pair.SelectedFolders != "[]"
	hasPatternFilter := strings.TrimSpace(pair.IncludePatterns) != "" || strings.TrimSpace(pair.ExcludePatterns) != ""

	if !hasFolderFilter && !hasPatternFilter {
		return files
	}

	include := splitPatterns(pair.IncludePatterns)
	exclude := splitPatterns(pair.ExcludePatterns)
	filtered := make([]*provider.FileMeta, 0, len(files))
	for _, file := range files {
		if file == nil {
			continue
		}
		// Step 1: directory-level filter via SelectedFolders
		if hasFolderFilter && !filterBySelectedFolders(pair, file.Path) {
			continue
		}
		// Step 2: file-level filter via include/exclude patterns (directories pass through)
		if hasPatternFilter && !file.IsDir && !pathAllowed(file.Path, include, exclude) {
			continue
		}
		filtered = append(filtered, file)
	}
	return filtered
}

func splitPatterns(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})
	patterns := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			patterns = append(patterns, field)
		}
	}
	return patterns
}

func pathAllowed(filePath string, include, exclude []string) bool {
	cleaned := strings.TrimPrefix(path.Clean(filePath), "/")
	if len(include) > 0 && !matchesAnyPattern(cleaned, include) {
		return false
	}
	return !matchesAnyPattern(cleaned, exclude)
}

func matchesAnyPattern(filePath string, patterns []string) bool {
	for _, pattern := range patterns {
		normalized := strings.TrimPrefix(path.Clean(pattern), "/")
		if strings.HasPrefix(pattern, "**/") {
			normalized = strings.TrimPrefix(pattern, "**/")
		}
		if ok, _ := path.Match(normalized, filePath); ok {
			return true
		}
		if strings.HasSuffix(normalized, "/**") && strings.HasPrefix(filePath, strings.TrimSuffix(normalized, "/**")+"/") {
			return true
		}
		if strings.HasPrefix(pattern, "**/") {
			if ok, _ := path.Match(normalized, path.Base(filePath)); ok {
				return true
			}
		}
	}
	return false
}

func (e *Engine) indexCleanFiles(ctx context.Context, pair *store.SyncPair, localFiles, remoteFiles []*provider.FileMeta, dbEntries []*store.FileEntry, taskPaths map[string]bool, dir Direction) error {
	if dir != DirectionUp && dir != DirectionDown && dir != DirectionBoth {
		return nil
	}

	localMap := make(map[string]*provider.FileMeta, len(localFiles))
	for _, f := range localFiles {
		localMap[path.Clean(f.Path)] = f
	}
	remoteMap := make(map[string]*provider.FileMeta, len(remoteFiles))
	for _, f := range remoteFiles {
		remoteMap[path.Clean(f.Path)] = f
	}
	entryMap := make(map[string]*store.FileEntry, len(dbEntries))
	for _, entry := range dbEntries {
		entryMap[path.Clean(entry.Path)] = entry
	}

	for key, localMeta := range localMap {
		if taskPaths[key] {
			continue
		}
		remoteMeta := remoteMap[key]
		if remoteMeta == nil {
			continue
		}
		if isSynced(entryMap[key]) {
			continue
		}

		localHash := ""
		remoteHash := ""
		if !sameSnapshot(localMeta, remoteMeta) {
			if localMeta.IsDir || remoteMeta.IsDir || localMeta.Size != remoteMeta.Size {
				continue
			}
			var err error
			localHash, err = e.computeHashViaProvider(ctx, e.locals[pair.ID], key)
			if err != nil {
				continue
			}
			remoteHash, err = e.computeHashViaProvider(ctx, e.remotes[pair.ID], key)
			if err != nil || localHash != remoteHash {
				continue
			}
		}

		// Compute hashes for the file so subsequent scans can use them for
		// two-stage detection. If size and mtime already matched, the task
		// generator has already accepted this as a clean snapshot.
		if !localMeta.IsDir && localHash == "" {
			localHash, _ = e.computeHashViaProvider(ctx, e.locals[pair.ID], key)
			remoteHash = localHash
		}

		if err := e.store.UpsertFileEntry(&store.FileEntry{
			Path:        key,
			SyncPairID:  pair.ID,
			LocalMTime:  &localMeta.ModTime,
			RemoteMTime: &remoteMeta.ModTime,
			LocalSize:   localMeta.Size,
			RemoteSize:  remoteMeta.Size,
			LocalHash:   localHash,
			RemoteHash:  remoteHash,
			SyncState:   "synced",
			IsDir:       localMeta.IsDir,
		}); err != nil {
			return fmt.Errorf("index clean file %s: %w", key, err)
		}
	}

	return nil
}

func (e *Engine) worker(id int) {
	defer e.wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return
		case task, ok := <-e.taskQueue:
			if !ok {
				return
			}
			err := e.executeTask(e.ctx, task)
			e.results <- TaskResult{Task: task, Error: err}
		}
	}
}

func (e *Engine) executeTask(ctx context.Context, task SyncTask) error {
	e.mu.RLock()
	local := e.locals[task.PairID]
	remote := e.remotes[task.PairID]
	pair := e.pairs[task.PairID]
	e.mu.RUnlock()

	if local == nil || remote == nil || pair == nil {
		return fmt.Errorf("providers not found for pair %d", task.PairID)
	}

	switch task.Type {
	case TaskUpload:
		logger.L.Debug().Str("path", task.Path).Str("pair", pair.Name).Msg("uploading")
		return e.doUpload(ctx, pair, local, remote, task.Path)
	case TaskDownload:
		logger.L.Debug().Str("path", task.Path).Str("pair", pair.Name).Msg("downloading")
		return e.doDownload(ctx, pair, local, remote, task.Path)
	case TaskDelete:
		if task.Target == "db_cleanup" {
			// Both sides deleted — just clean up the stale DB entry.
			entry, _ := e.store.GetFileEntry(pair.ID, task.Path)
			if entry != nil {
				e.store.DeleteFileEntry(entry.ID)
			}
			logger.L.Debug().Str("path", task.Path).Str("pair", pair.Name).Msg("cleaned up db entry")
			return nil
		}
		var target provider.Provider
		if task.Target == "local" {
			target = local
		} else {
			target = remote
		}
		logger.L.Debug().Str("path", task.Path).Str("target", task.Target).Str("pair", pair.Name).Msg("deleting")
		return e.doDelete(ctx, pair, target, task.Target, task.Path)
	case TaskVirtual:
		logger.L.Debug().Str("path", task.Path).Str("pair", pair.Name).Msg("virtualizing")
		return e.doVirtualize(ctx, pair, remote, task.Path)
	case TaskConflict:
		logger.L.Debug().Str("path", task.Path).Str("pair", pair.Name).Msg("recording conflict")
		return e.doRecordConflict(ctx, pair, local, remote, task.Path, task.ConflictType)
	case TaskCreateDir:
		var target provider.Provider
		if task.Target == "local" {
			target = local
		} else {
			target = remote
		}
		logger.L.Debug().Str("path", task.Path).Str("target", task.Target).Str("pair", pair.Name).Msg("creating directory")
		return e.doCreateDir(ctx, pair, target, task.Target, task.Path)
	case TaskDeleteDir:
		var target provider.Provider
		if task.Target == "local" {
			target = local
		} else {
			target = remote
		}
		logger.L.Debug().Str("path", task.Path).Str("target", task.Target).Str("pair", pair.Name).Msg("deleting directory")
		return e.doDeleteDir(ctx, pair, target, task.Target, task.Path)
	default:
		return fmt.Errorf("unknown task type: %s", task.Type)
	}
}

func (e *Engine) doUpload(ctx context.Context, pair *store.SyncPair, local, remote provider.Provider, filePath string) error {
	if meta, err := local.Stat(ctx, filePath); err == nil && e.canResumeTransfer(local, remote, meta.Size) {
		e.recordProviderVersion(ctx, pair.ID, filePath, "remote", remote)
		remoteMeta, err := e.doResumableTransfer(ctx, pair, local, remote, filePath, meta, e.config.UploadLimit, TaskUpload)
		if err != nil {
			return fmt.Errorf("resumable upload to remote: %w", err)
		}
		localHash, _ := e.computeHashViaProvider(ctx, local, filePath)
		remoteHash, _ := e.computeHashViaProvider(ctx, remote, filePath)
		if err := e.store.UpsertFileEntry(&store.FileEntry{
			Path:        filePath,
			SyncPairID:  pair.ID,
			LocalMTime:  &meta.ModTime,
			RemoteMTime: &remoteMeta.ModTime,
			LocalSize:   meta.Size,
			RemoteSize:  remoteMeta.Size,
			LocalHash:   localHash,
			RemoteHash:  remoteHash,
			SyncState:   "synced",
		}); err != nil {
			return fmt.Errorf("record upload: %w", err)
		}
		_ = e.store.AddSyncStats(meta.Size, 0, 0, 0, 0, 0)
		logger.L.Info().Str("path", filePath).Str("pair", pair.Name).Bool("resumed", true).Msg("uploaded")
		return nil
	}

	reader, meta, err := local.GetFile(ctx, filePath)
	if err != nil {
		return fmt.Errorf("read local file: %w", err)
	}
	defer reader.Close()
	e.startTaskProgress(pair, filePath, TaskUpload, meta.Size)
	transferReader := e.transferReader(reader, meta.Size, e.config.UploadLimit, pair, filePath, TaskUpload)

	dir := path.Dir(filePath)
	if dir != "/" && dir != "." {
		remote.CreateDir(ctx, dir)
	}

	e.recordProviderVersion(ctx, pair.ID, filePath, "remote", remote)
	if err := remote.PutFile(ctx, filePath, transferReader, meta); err != nil {
		return fmt.Errorf("upload to remote: %w", err)
	}

	remoteMeta, err := remote.Stat(ctx, filePath)
	if err != nil {
		remoteMeta = meta
	}

	localHash, _ := e.computeHashViaProvider(ctx, local, filePath)
	remoteHash, _ := e.computeHashViaProvider(ctx, remote, filePath)
	if err := e.store.UpsertFileEntry(&store.FileEntry{
		Path:        filePath,
		SyncPairID:  pair.ID,
		LocalMTime:  &meta.ModTime,
		RemoteMTime: &remoteMeta.ModTime,
		LocalSize:   meta.Size,
		RemoteSize:  remoteMeta.Size,
		LocalHash:   localHash,
		RemoteHash:  remoteHash,
		SyncState:   "synced",
	}); err != nil {
		return fmt.Errorf("record upload: %w", err)
	}

	_ = e.store.AddSyncStats(meta.Size, 0, 0, 0, 0, 0)
	logger.L.Info().Str("path", filePath).Str("pair", pair.Name).Msg("uploaded")
	return nil
}

func (e *Engine) doDownload(ctx context.Context, pair *store.SyncPair, local, remote provider.Provider, filePath string) error {
	if meta, err := remote.Stat(ctx, filePath); err == nil && e.canResumeTransfer(remote, local, meta.Size) {
		e.recordProviderVersion(ctx, pair.ID, filePath, "local", local)
		localMeta, err := e.doResumableTransfer(ctx, pair, remote, local, filePath, meta, e.config.DownloadLimit, TaskDownload)
		if err != nil {
			return fmt.Errorf("resumable download to local: %w", err)
		}
		localHash, _ := e.computeHashViaProvider(ctx, local, filePath)
		remoteHash, _ := e.computeHashViaProvider(ctx, remote, filePath)
		if err := e.store.UpsertFileEntry(&store.FileEntry{
			Path:        filePath,
			SyncPairID:  pair.ID,
			LocalMTime:  &localMeta.ModTime,
			RemoteMTime: &meta.ModTime,
			LocalSize:   localMeta.Size,
			RemoteSize:  meta.Size,
			LocalHash:   localHash,
			RemoteHash:  remoteHash,
			SyncState:   "synced",
		}); err != nil {
			return fmt.Errorf("record download: %w", err)
		}
		_ = e.store.AddSyncStats(0, meta.Size, 0, 0, 0, 0)
		logger.L.Info().Str("path", filePath).Str("pair", pair.Name).Bool("resumed", true).Msg("downloaded")
		return nil
	}

	reader, meta, err := remote.GetFile(ctx, filePath)
	if err != nil {
		return fmt.Errorf("read remote file: %w", err)
	}
	defer reader.Close()
	e.startTaskProgress(pair, filePath, TaskDownload, meta.Size)
	transferReader := e.transferReader(reader, meta.Size, e.config.DownloadLimit, pair, filePath, TaskDownload)

	dir := path.Dir(filePath)
	if dir != "/" && dir != "." {
		local.CreateDir(ctx, dir)
	}

	e.recordProviderVersion(ctx, pair.ID, filePath, "local", local)
	if err := local.PutFile(ctx, filePath, transferReader, meta); err != nil {
		return fmt.Errorf("download to local: %w", err)
	}

	localMeta, err := local.Stat(ctx, filePath)
	if err != nil {
		localMeta = meta
	}

	localHash, _ := e.computeHashViaProvider(ctx, local, filePath)
	remoteHash, _ := e.computeHashViaProvider(ctx, remote, filePath)
	if err := e.store.UpsertFileEntry(&store.FileEntry{
		Path:        filePath,
		SyncPairID:  pair.ID,
		LocalMTime:  &localMeta.ModTime,
		RemoteMTime: &meta.ModTime,
		LocalSize:   localMeta.Size,
		RemoteSize:  meta.Size,
		LocalHash:   localHash,
		RemoteHash:  remoteHash,
		SyncState:   "synced",
	}); err != nil {
		return fmt.Errorf("record download: %w", err)
	}

	_ = e.store.AddSyncStats(0, meta.Size, 0, 0, 0, 0)
	logger.L.Info().Str("path", filePath).Str("pair", pair.Name).Msg("downloaded")
	return nil
}

func (e *Engine) canResumeTransfer(source, target provider.Provider, size int64) bool {
	if e.config.ChunkThreshold <= 0 || size < e.config.ChunkThreshold {
		return false
	}
	_, canRangeRead := source.(provider.RangeReader)
	_, canResumeWrite := target.(provider.ResumeWriter)
	return canRangeRead && canResumeWrite
}

func (e *Engine) doResumableTransfer(ctx context.Context, pair *store.SyncPair, source, target provider.Provider, filePath string, sourceMeta *provider.FileMeta, bytesPerSecond int64, taskType TaskType) (*provider.FileMeta, error) {
	rangeReader := source.(provider.RangeReader)
	resumeWriter := target.(provider.ResumeWriter)
	partialPath := filePath + partialSuffix

	dir := path.Dir(filePath)
	if dir != "/" && dir != "." {
		_ = target.CreateDir(ctx, dir)
	}

	offset := int64(0)
	if partialMeta, err := target.Stat(ctx, partialPath); err == nil && partialMeta != nil && !partialMeta.IsDir {
		switch {
		case partialMeta.Size < sourceMeta.Size:
			offset = partialMeta.Size
			e.broadcast(Event{
				Type:     "transfer_resumed",
				PairID:   pair.ID,
				PairName: pair.Name,
				TaskType: string(taskType),
				Path:     filePath,
				Message:  fmt.Sprintf("%d/%d", offset, sourceMeta.Size),
			})
		case partialMeta.Size == sourceMeta.Size:
			if err := target.MoveFile(ctx, partialPath, filePath); err != nil {
				return nil, fmt.Errorf("finalize complete partial file: %w", err)
			}
			targetMeta, err := target.Stat(ctx, filePath)
			if err != nil {
				targetMeta = sourceMeta
			}
			return targetMeta, nil
		default:
			_ = target.DeleteFile(ctx, partialPath)
		}
	}

	reader, _, err := rangeReader.GetFileRange(ctx, filePath, offset, sourceMeta.Size-offset)
	if err != nil {
		return nil, fmt.Errorf("read source range: %w", err)
	}
	defer reader.Close()

	e.startTaskProgress(pair, filePath, taskType, sourceMeta.Size)
	transferReader := e.transferReader(reader, sourceMeta.Size-offset, bytesPerSecond, pair, filePath, taskType)
	if err := resumeWriter.PutFileResume(ctx, partialPath, transferReader, sourceMeta, offset); err != nil {
		return nil, fmt.Errorf("write partial file: %w", err)
	}
	if err := target.MoveFile(ctx, partialPath, filePath); err != nil {
		return nil, fmt.Errorf("finalize partial file: %w", err)
	}

	targetMeta, err := target.Stat(ctx, filePath)
	if err != nil {
		targetMeta = sourceMeta
	}
	return targetMeta, nil
}

func (e *Engine) doDelete(ctx context.Context, pair *store.SyncPair, target provider.Provider, source, filePath string) error {
	e.recordProviderVersion(ctx, pair.ID, filePath, source, target)
	if err := target.DeleteFile(ctx, filePath); err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			logger.L.Debug().Str("path", filePath).Msg("file already deleted")
		} else {
			return fmt.Errorf("delete file %s: %w", filePath, err)
		}
	}

	// Remove file entry from DB
	entry, _ := e.store.GetFileEntry(pair.ID, filePath)
	if entry != nil {
		e.store.DeleteFileEntry(entry.ID)
	}

	_ = e.store.AddSyncStats(0, 0, 1, 0, 0, 0)
	logger.L.Info().Str("path", filePath).Str("pair", pair.Name).Msg("deleted")
	return nil
}

func (e *Engine) doCreateDir(ctx context.Context, pair *store.SyncPair, target provider.Provider, targetSide, dirPath string) error {
	if err := target.CreateDir(ctx, dirPath); err != nil {
		return fmt.Errorf("create directory %s on %s: %w", dirPath, targetSide, err)
	}

	// Record the directory entry in the DB
	meta, err := target.Stat(ctx, dirPath)
	if err != nil {
		meta = &provider.FileMeta{Path: dirPath, IsDir: true, ModTime: time.Now()}
	}
	entry := &store.FileEntry{
		Path:       dirPath,
		SyncPairID: pair.ID,
		SyncState:  "synced",
		IsDir:      true,
	}
	if meta != nil {
		if targetSide == "local" {
			entry.LocalMTime = &meta.ModTime
			entry.LocalSize = meta.Size
		} else {
			entry.RemoteMTime = &meta.ModTime
			entry.RemoteSize = meta.Size
		}
	}
	if err := e.store.UpsertFileEntry(entry); err != nil {
		return fmt.Errorf("record directory entry %s: %w", dirPath, err)
	}

	logger.L.Info().Str("path", dirPath).Str("pair", pair.Name).Str("side", targetSide).Msg("directory created")
	return nil
}

func (e *Engine) doDeleteDir(ctx context.Context, pair *store.SyncPair, target provider.Provider, targetSide, dirPath string) error {
	// Delete all synced child files under this directory from the target provider
	dbEntries, err := e.store.ListFileEntriesByPair(pair.ID)
	if err != nil {
		return fmt.Errorf("list file entries for directory deletion: %w", err)
	}
	prefix := path.Clean(dirPath) + "/"
	for _, entry := range dbEntries {
		if entry.IsDir {
			continue // skip sub-directory entries; they get their own tasks
		}
		if strings.HasPrefix(entry.Path, prefix) {
			// Delete the file from the target provider
			if err := target.DeleteFile(ctx, entry.Path); err != nil {
				if errors.Is(err, provider.ErrNotFound) {
					logger.L.Debug().Str("path", entry.Path).Msg("child file already deleted")
				} else {
					return fmt.Errorf("delete child file %s: %w", entry.Path, err)
				}
			}
			// Remove the file entry from DB only if provider deletion succeeded
			e.store.DeleteFileEntry(entry.ID)
		}
	}

	// Delete the directory itself
	if err := target.DeleteFile(ctx, dirPath); err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			logger.L.Debug().Str("path", dirPath).Msg("directory already deleted")
		} else {
			return fmt.Errorf("delete directory %s: %w", dirPath, err)
		}
	}

	// Remove the directory entry from DB
	dirEntry, _ := e.store.GetFileEntry(pair.ID, dirPath)
	if dirEntry != nil {
		e.store.DeleteFileEntry(dirEntry.ID)
	}

	logger.L.Info().Str("path", dirPath).Str("pair", pair.Name).Str("side", targetSide).Msg("directory deleted")
	return nil
}

func (e *Engine) doVirtualize(ctx context.Context, pair *store.SyncPair, remote provider.Provider, filePath string) error {
	meta, err := remote.Stat(ctx, filePath)
	if err != nil {
		return fmt.Errorf("stat remote virtual file: %w", err)
	}
	if err := e.store.UpsertFileEntry(&store.FileEntry{
		Path:        filePath,
		SyncPairID:  pair.ID,
		RemoteMTime: &meta.ModTime,
		RemoteSize:  meta.Size,
		SyncState:   "virtual",
	}); err != nil {
		return fmt.Errorf("record virtual file: %w", err)
	}
	_ = e.store.AddSyncStats(0, 0, 0, 1, 0, 0)
	e.broadcast(Event{Type: "file_virtualized", PairID: pair.ID, PairName: pair.Name, Path: filePath, Message: fmt.Sprintf("%d bytes", meta.Size)})
	return nil
}

func (e *Engine) doRecordConflict(ctx context.Context, pair *store.SyncPair, local, remote provider.Provider, filePath, conflictType string) error {
	localMeta, _ := local.Stat(ctx, filePath)
	remoteMeta, _ := remote.Stat(ctx, filePath)

	var localMTime *time.Time
	var remoteMTime *time.Time
	var localSize, remoteSize int64

	if localMeta != nil {
		localMTime = &localMeta.ModTime
		localSize = localMeta.Size
	}
	if remoteMeta != nil {
		remoteMTime = &remoteMeta.ModTime
		remoteSize = remoteMeta.Size
	}

	conflict := &store.ConflictRecord{
		SyncPairID:   pair.ID,
		Path:         filePath,
		LocalMTime:   localMTime,
		RemoteMTime:  remoteMTime,
		LocalSize:    localSize,
		RemoteSize:   remoteSize,
		Status:       "open",
		Strategy:     "manual",
		ConflictType: conflictType,
	}
	if err := e.store.UpsertOpenConflict(conflict); err != nil {
		return err
	}
	_ = e.store.UpsertFileEntry(&store.FileEntry{
		Path:        filePath,
		SyncPairID:  pair.ID,
		LocalMTime:  localMTime,
		RemoteMTime: remoteMTime,
		LocalSize:   localSize,
		RemoteSize:  remoteSize,
		SyncState:   "conflict",
	})
	_ = e.store.AddSyncStats(0, 0, 0, 0, 0, 1)
	e.broadcast(Event{Type: "conflict_detected", PairID: pair.ID, PairName: pair.Name, Path: filePath})
	return nil
}

func (e *Engine) recordProviderVersion(ctx context.Context, pairID int64, filePath, source string, p provider.Provider) {
	if p == nil {
		return
	}
	meta, err := p.Stat(ctx, filePath)
	if err != nil || meta == nil || meta.IsDir {
		return
	}
	_ = e.store.CreateFileVersion(&store.FileVersion{
		SyncPairID: pairID,
		Path:       filePath,
		Source:     source,
		Size:       meta.Size,
		ModTime:    &meta.ModTime,
		Hash:       meta.Hash,
	})
}

func (e *Engine) processResults() {
	defer e.wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return
		case result, ok := <-e.results:
			if !ok {
				return
			}
			pairID := result.Task.PairID
			terminal := false
			permanentFailure := false

			if result.Error != nil {
				if result.Task.Retries < e.config.RetryMax {
					result.Task.Retries++
					logger.L.Warn().Err(result.Error).Str("path", result.Task.Path).Int("retry", result.Task.Retries).Msg("task failed, retrying")
					time.AfterFunc(e.config.RetryDelay, func() {
						select {
						case e.taskQueue <- result.Task:
						case <-e.ctx.Done():
							atomic.AddInt64(&e.pending, -1)
						}
					})
				} else {
					terminal = true
					permanentFailure = true
					atomic.AddInt64(&e.pending, -1)
					logger.L.Error().Err(result.Error).Str("path", result.Task.Path).Str("type", string(result.Task.Type)).Msg("task permanently failed")
					e.progress.FailTask(pairID, string(result.Task.Type), result.Task.Path, result.Error.Error())
					e.broadcast(Event{Type: "task_failed", PairID: pairID, TaskType: string(result.Task.Type), Path: result.Task.Path, Error: result.Error.Error(), Pending: atomic.LoadInt64(&e.pending)})
				}
			} else {
				terminal = true
				atomic.AddInt64(&e.pending, -1)
				e.progress.CompleteTask(pairID, string(result.Task.Type), result.Task.Path)
				// Track synced file count for upload/download tasks.
				if result.Task.Type == TaskUpload || result.Task.Type == TaskDownload {
					e.mu.Lock()
					e.pairFilesSynced[pairID]++
					synced := e.pairFilesSynced[pairID]
					total := e.pairFilesTotal[pairID]
					e.mu.Unlock()
					e.broadcast(Event{Type: "task_completed", PairID: pairID, TaskType: string(result.Task.Type), Path: result.Task.Path, Pending: atomic.LoadInt64(&e.pending), FilesSynced: synced, FilesTotal: total})
				} else {
					e.broadcast(Event{Type: "task_completed", PairID: pairID, TaskType: string(result.Task.Type), Path: result.Task.Path, Pending: atomic.LoadInt64(&e.pending)})
				}
			}

			// Decrement per-pair pending count and broadcast sync_completed when all tasks done.
			// Only count tasks from the current sync cycle.
			if terminal {
				e.mu.Lock()
				currentCycle := e.pairCycle[pairID]
				if result.Task.CycleID == currentCycle && e.pairPending[pairID] > 0 {
					if permanentFailure {
						e.pairFailed[pairID] = true
					}
					e.pairPending[pairID]--
					if e.pairPending[pairID] == 0 {
						pairName := ""
						direction := e.pairDirection[pairID]
						if pair, ok := e.pairs[pairID]; ok {
							pairName = pair.Name
						}
						synced := e.pairFilesSynced[pairID]
						total := e.pairFilesTotal[pairID]
						failed := e.pairFailed[pairID]
						e.pairSyncing[pairID] = false
						e.mu.Unlock()
						if failed {
							e.broadcast(Event{Type: "sync_failed", PairID: pairID, PairName: pairName, Direction: direction, FilesSynced: synced, FilesTotal: total, Error: "one or more tasks failed"})
						} else {
							e.progress.FinishSync(pairID)
							e.broadcast(Event{Type: "sync_completed", PairID: pairID, PairName: pairName, Direction: direction, FilesSynced: synced, FilesTotal: total})
						}
					} else {
						e.mu.Unlock()
					}
				} else {
					e.mu.Unlock()
				}
			}
		}
	}
}

func (e *Engine) periodicScan() {
	defer e.wg.Done()

	// Use a 30-second base tick so we can check each pair's individual interval.
	// Pairs with shorter intervals are still respected; pairs with longer intervals
	// simply skip ticks until their interval elapses.
	baseInterval := e.config.ScanInterval
	if baseInterval > 30*time.Second {
		baseInterval = 30 * time.Second
	}
	ticker := time.NewTicker(baseInterval)
	defer ticker.Stop()

	// Track last scan time per pair.
	lastScan := make(map[int64]time.Time)

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			logger.L.Debug().Msg("periodic scan tick")
			if err := e.RefreshPairs(); err != nil {
				logger.L.Error().Err(err).Msg("refresh pairs failed")
			}

			e.mu.RLock()
			pairIDs := make([]int64, 0, len(e.pairs))
			for id := range e.pairs {
				pairIDs = append(pairIDs, id)
			}
			e.mu.RUnlock()

			now := time.Now()
			for _, id := range pairIDs {
				e.mu.RLock()
				pair := e.pairs[id]
				e.mu.RUnlock()
				if pair == nil {
					continue
				}

				// Determine effective scan interval: use per-pair value if set,
				// otherwise fall back to the global config.
				interval := e.config.ScanInterval
				if pair.ScanInterval > 0 {
					interval = time.Duration(pair.ScanInterval) * time.Second
				}

				last := lastScan[id]
				if now.Sub(last) >= interval {
					lastScan[id] = now
					if err := e.SyncPair(e.ctx, id, ""); err != nil {
						logger.L.Error().Err(err).Int64("pair_id", id).Msg("periodic sync pair failed")
					}
				}
			}
		}
	}
}

func (e *Engine) startPairWatcherLocked(pairID int64, pair *store.SyncPair, local provider.Provider) {
	ctx := e.ctx
	if ctx == nil || local == nil {
		return
	}

	changes, err := local.WatchChanges(ctx, "/")
	if err != nil {
		if !errors.Is(err, provider.ErrNotSupported) {
			logger.L.Warn().Err(err).Int64("pair_id", pairID).Str("name", pair.Name).Msg("local watcher unavailable")
		}
		return
	}

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		var timer *time.Timer
		for {
			var timerC <-chan time.Time
			if timer != nil {
				timerC = timer.C
			}
			select {
			case <-ctx.Done():
				if timer != nil {
					timer.Stop()
				}
				return
			case event, ok := <-changes:
				if !ok {
					return
				}
				e.broadcast(Event{Type: "file_changed", PairID: pairID, PairName: pair.Name, Path: event.Path, Message: string(event.Type)})
				if timer == nil {
					timer = time.NewTimer(500 * time.Millisecond)
				} else {
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(500 * time.Millisecond)
				}
			case <-timerC:
				timer = nil
				logger.L.Debug().Int64("pair_id", pairID).Str("name", pair.Name).Msg("watch-triggered sync")
				if err := e.SyncPair(ctx, pairID, ""); err != nil {
					logger.L.Error().Err(err).Int64("pair_id", pairID).Msg("watch-triggered sync failed")
				}
			}
		}
	}()
}

func (e *Engine) broadcast(event Event) {
	event.Time = time.Now()
	if event.Pending == 0 {
		event.Pending = atomic.LoadInt64(&e.pending)
	}

	e.subsMu.Lock()
	for ch := range e.subs {
		select {
		case ch <- event:
		default:
		}
	}
	e.subsMu.Unlock()

	if shouldNotify(event.Type) {
		go e.notify(event)
	}
}

func shouldNotify(eventType string) bool {
	switch eventType {
	case "sync_completed", "sync_failed", "task_failed", "conflict_detected", "conflict_resolved", "file_materialized":
		return true
	default:
		return false
	}
}

func (e *Engine) notify(event Event) {
	if e.config.WebhookURL != "" {
		e.sendWebhook(event)
	}
	if e.config.EmailSMTPAddr != "" && e.config.EmailFrom != "" && len(e.config.EmailTo) > 0 {
		e.sendEmail(event)
	}
}

func (e *Engine) sendWebhook(event Event) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, e.config.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		logger.L.Warn().Err(err).Msg("create webhook notification")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.L.Warn().Err(err).Msg("send webhook notification")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		logger.L.Warn().Int("status", resp.StatusCode).Msg("webhook notification returned non-success status")
	}
}

func (e *Engine) sendEmail(event Event) {
	host := e.config.EmailSMTPAddr
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}

	var auth smtp.Auth
	if e.config.EmailUsername != "" || e.config.EmailPassword != "" {
		auth = smtp.PlainAuth("", e.config.EmailUsername, e.config.EmailPassword, host)
	}

	subject := fmt.Sprintf("EverySync %s", event.Type)
	body := fmt.Sprintf("type=%s\npair=%s\npath=%s\nmessage=%s\nerror=%s\ntime=%s\n",
		event.Type, event.PairName, event.Path, event.Message, event.Error, event.Time.Format(time.RFC3339))
	msg := []byte("To: " + strings.Join(e.config.EmailTo, ", ") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" + body)

	if err := smtp.SendMail(e.config.EmailSMTPAddr, auth, e.config.EmailFrom, e.config.EmailTo, msg); err != nil {
		logger.L.Warn().Err(err).Msg("send email notification")
	}
}

func (e *Engine) startTaskProgress(pair *store.SyncPair, filePath string, taskType TaskType, bytesTotal int64) {
	e.progress.StartTask(pair.ID, string(taskType), filePath, bytesTotal)
	e.broadcast(Event{
		Type:       "task_started",
		PairID:     pair.ID,
		PairName:   pair.Name,
		TaskType:   string(taskType),
		Path:       filePath,
		Direction:  directionForTask(string(taskType)),
		BytesTotal: bytesTotal,
	})
}

func (e *Engine) transferReader(reader io.Reader, size, bytesPerSecond int64, pair *store.SyncPair, filePath string, taskType TaskType) io.Reader {
	if e.config.ChunkSize > 0 && e.config.ChunkThreshold > 0 && size >= e.config.ChunkThreshold {
		reader = &chunkProgressReader{
			reader:    reader,
			chunkSize: e.config.ChunkSize,
			size:      size,
			pair:      pair,
			filePath:  filePath,
			taskType:  taskType,
			emit:      e.broadcast,
			track:     e.progress.ChunkTransferred,
		}
	}
	if bytesPerSecond <= 0 {
		return reader
	}
	return &rateLimitedReader{
		reader:         reader,
		bytesPerSecond: bytesPerSecond,
		started:        time.Now(),
	}
}

type chunkProgressReader struct {
	reader    io.Reader
	chunkSize int64
	size      int64
	read      int64
	next      int64
	pair      *store.SyncPair
	filePath  string
	taskType  TaskType
	emit      func(Event)
	track     func(pairID int64, filePath string, transferred, total int64)
}

func (r *chunkProgressReader) Read(p []byte) (int, error) {
	if r.next == 0 {
		r.next = r.chunkSize
	}
	n, err := r.reader.Read(p)
	if n > 0 {
		r.read += int64(n)
		for r.read >= r.next || (err == io.EOF && r.read == r.size) {
			r.track(r.pair.ID, r.filePath, r.read, r.size)
			r.emit(Event{
				Type:             "chunk_transferred",
				PairID:           r.pair.ID,
				PairName:         r.pair.Name,
				TaskType:         string(r.taskType),
				Path:             r.filePath,
				Direction:        directionForTask(string(r.taskType)),
				Message:          fmt.Sprintf("%d/%d", r.read, r.size),
				BytesTransferred: r.read,
				BytesTotal:       r.size,
			})
			r.next += r.chunkSize
			if r.read < r.next {
				break
			}
		}
	}
	return n, err
}

type rateLimitedReader struct {
	reader         io.Reader
	bytesPerSecond int64
	started        time.Time
	read           int64
}

func (r *rateLimitedReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.read += int64(n)
		wantElapsed := time.Duration(r.read*int64(time.Second)) / time.Duration(r.bytesPerSecond)
		if sleep := r.started.Add(wantElapsed).Sub(time.Now()); sleep > 0 {
			time.Sleep(sleep)
		}
	}
	return n, err
}
