package engine

import (
	"context"
	"errors"
	"fmt"
	"path"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

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
	TaskUpload   TaskType = "upload"
	TaskDownload TaskType = "download"
	TaskDelete   TaskType = "delete"
	TaskMove     TaskType = "move"
	TaskHash     TaskType = "hash"
)

type SyncTask struct {
	ID           string
	Type         TaskType
	PairID       int64
	Path         string
	Priority     int
	Retries      int
	DeleteTarget string // "local" or "remote"
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
	}
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

	pending int64 // atomic counter for pending tasks
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func New(s *store.Store, cfg Config) *Engine {
	if cfg.MaxWorkers == 0 {
		cfg.MaxWorkers = runtime.NumCPU() * 2
	}
	return &Engine{
		store:     s,
		config:    cfg,
		locals:    make(map[int64]provider.Provider),
		remotes:   make(map[int64]provider.Provider),
		pairs:     make(map[int64]*store.SyncPair),
		taskQueue: make(chan SyncTask, cfg.QueueSize),
		results:   make(chan TaskResult, cfg.QueueSize),
	}
}

// WithRegistrar sets the callback used to create providers for dynamic pair registration.
func (e *Engine) WithRegistrar(r PairRegistrar) *Engine {
	e.registrar = r
	return e
}

// RegisterPair binds providers to a sync pair.
func (e *Engine) RegisterPair(pair *store.SyncPair, local, remote provider.Provider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pairs[pair.ID] = pair
	e.locals[pair.ID] = local
	e.remotes[pair.ID] = remote
	logger.L.Info().Int64("pair_id", pair.ID).Str("name", pair.Name).Msg("pair registered")
}

// UnregisterPair removes a sync pair from the engine.
func (e *Engine) UnregisterPair(pairID int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if p, ok := e.pairs[pairID]; ok {
		logger.L.Info().Int64("pair_id", pairID).Str("name", p.Name).Msg("pair unregistered")
	}
	delete(e.pairs, pairID)
	delete(e.locals, pairID)
	delete(e.remotes, pairID)
}

// RefreshPairs reloads pairs from DB, registers new enabled ones, unregisters disabled ones.
func (e *Engine) RefreshPairs() error {
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
			_, exists := e.pairs[pair.ID]
			e.mu.RUnlock()

			if !exists && e.registrar != nil {
				local, remote, err := e.registrar(pair)
				if err != nil {
					logger.L.Error().Err(err).Int64("pair_id", pair.ID).Str("name", pair.Name).Msg("failed to create providers for pair")
					continue
				}
				e.RegisterPair(pair, local, remote)
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

// Start launches worker goroutines and the result processor.
func (e *Engine) Start(parent context.Context) error {
	e.mu.Lock()
	e.ctx, e.cancel = context.WithCancel(parent)
	e.mu.Unlock()

	for i := 0; i < e.config.MaxWorkers; i++ {
		e.wg.Add(1)
		go e.worker(i)
	}

	e.wg.Add(1)
	go e.processResults()

	e.wg.Add(1)
	go e.periodicScan()

	logger.L.Info().Int("workers", e.config.MaxWorkers).Dur("scan_interval", e.config.ScanInterval).Msg("engine started")
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
	logger.L.Info().Msg("engine stopped")
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

	return e.syncOnePair(ctx, pair, local, remote, dir)
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

func (e *Engine) syncOnePair(ctx context.Context, pair *store.SyncPair, local, remote provider.Provider, dir Direction) error {
	logger.L.Info().Str("pair", pair.Name).Str("direction", string(dir)).Msg("syncing pair")

	var localFiles, remoteFiles []*provider.FileMeta
	var err error

	localFiles, err = e.scanRecursive(ctx, local, "/")
	if err != nil {
		return fmt.Errorf("scan local: %w", err)
	}

	remoteFiles, err = e.scanRecursive(ctx, remote, "/")
	if err != nil {
		return fmt.Errorf("scan remote: %w", err)
	}

	logger.L.Info().
		Str("pair", pair.Name).
		Int("local_files", len(localFiles)).
		Int("remote_files", len(remoteFiles)).
		Msg("scan complete")

	// Load DB entries for deletion detection
	dbEntries, _ := e.store.ListFileEntriesByPair(pair.ID)

	tasks := e.generateTasks(pair, localFiles, remoteFiles, dbEntries, dir)
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

	if e.config.DryRun {
		for _, task := range tasks {
			logger.L.Info().
				Str("pair", pair.Name).
				Str("task", string(task.Type)).
				Str("path", task.Path).
				Str("delete_target", task.DeleteTarget).
				Msg("dry run task")
		}
		return nil
	}

	if err := e.indexCleanFiles(pair, localFiles, remoteFiles, dbEntries, taskPaths, dir); err != nil {
		return err
	}

	for _, task := range tasks {
		atomic.AddInt64(&e.pending, 1)
		select {
		case e.taskQueue <- task:
		case <-ctx.Done():
			atomic.AddInt64(&e.pending, -1)
			return ctx.Err()
		}
	}

	return nil
}

func (e *Engine) scanRecursive(ctx context.Context, p provider.Provider, rootPath string) ([]*provider.FileMeta, error) {
	var result []*provider.FileMeta

	queue := []string{rootPath}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		entries, err := p.ListDir(ctx, current)
		if err != nil {
			logger.L.Debug().Err(err).Str("path", current).Msg("skip directory")
			continue
		}

		for _, entry := range entries {
			if entry.IsDir {
				queue = append(queue, entry.Path)
			} else {
				result = append(result, entry)
			}
		}
	}

	return result, nil
}

func (e *Engine) generateTasks(pair *store.SyncPair, localFiles, remoteFiles []*provider.FileMeta, dbEntries []*store.FileEntry, dir Direction) []SyncTask {
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

		switch dir {
		case DirectionUp:
			tasks = append(tasks, generateUpTasks(pair.ID, key, localMeta, remoteMeta, entry, hasLocal, hasRemote)...)
		case DirectionDown:
			tasks = append(tasks, generateDownTasks(pair.ID, key, localMeta, remoteMeta, entry, hasLocal, hasRemote)...)
		case DirectionBoth:
			tasks = append(tasks, generateBothTasks(pair.ID, key, localMeta, remoteMeta, entry, hasLocal, hasRemote)...)
		}
	}

	return tasks
}

func generateUpTasks(pairID int64, key string, localMeta, remoteMeta *provider.FileMeta, entry *store.FileEntry, hasLocal, hasRemote bool) []SyncTask {
	if hasLocal {
		if !hasRemote || !sameSnapshot(localMeta, remoteMeta) {
			if entry == nil || !metaMatchesEntry(localMeta, entry.LocalMTime, entry.LocalSize) || !metaMatchesEntry(remoteMeta, entry.RemoteMTime, entry.RemoteSize) {
				return []SyncTask{newTask(TaskUpload, pairID, key)}
			}
		}
		return nil
	}

	if hasRemote && isSynced(entry) && metaMatchesEntry(remoteMeta, entry.RemoteMTime, entry.RemoteSize) {
		return []SyncTask{newDeleteTask(pairID, key, "remote")}
	}
	return nil
}

func generateDownTasks(pairID int64, key string, localMeta, remoteMeta *provider.FileMeta, entry *store.FileEntry, hasLocal, hasRemote bool) []SyncTask {
	if hasRemote {
		if !hasLocal || !sameSnapshot(localMeta, remoteMeta) {
			if entry == nil || !metaMatchesEntry(localMeta, entry.LocalMTime, entry.LocalSize) || !metaMatchesEntry(remoteMeta, entry.RemoteMTime, entry.RemoteSize) {
				return []SyncTask{newTask(TaskDownload, pairID, key)}
			}
		}
		return nil
	}

	if hasLocal && isSynced(entry) && metaMatchesEntry(localMeta, entry.LocalMTime, entry.LocalSize) {
		return []SyncTask{newDeleteTask(pairID, key, "local")}
	}
	return nil
}

func generateBothTasks(pairID int64, key string, localMeta, remoteMeta *provider.FileMeta, entry *store.FileEntry, hasLocal, hasRemote bool) []SyncTask {
	if hasLocal && hasRemote {
		if entry == nil || !isSynced(entry) {
			return latestWinsTask(pairID, key, localMeta, remoteMeta)
		}

		localChanged := !metaMatchesEntry(localMeta, entry.LocalMTime, entry.LocalSize)
		remoteChanged := !metaMatchesEntry(remoteMeta, entry.RemoteMTime, entry.RemoteSize)

		switch {
		case !localChanged && !remoteChanged:
			return nil
		case localChanged && !remoteChanged:
			return []SyncTask{newTask(TaskUpload, pairID, key)}
		case !localChanged && remoteChanged:
			return []SyncTask{newTask(TaskDownload, pairID, key)}
		default:
			return latestWinsTask(pairID, key, localMeta, remoteMeta)
		}
	}

	if hasLocal {
		if isSynced(entry) && metaMatchesEntry(localMeta, entry.LocalMTime, entry.LocalSize) {
			return []SyncTask{newDeleteTask(pairID, key, "local")}
		}
		return []SyncTask{newTask(TaskUpload, pairID, key)}
	}

	if hasRemote {
		if isSynced(entry) && metaMatchesEntry(remoteMeta, entry.RemoteMTime, entry.RemoteSize) {
			return []SyncTask{newDeleteTask(pairID, key, "remote")}
		}
		return []SyncTask{newTask(TaskDownload, pairID, key)}
	}

	return nil
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
		Type:         TaskDelete,
		PairID:       pairID,
		Path:         key,
		Priority:     1,
		DeleteTarget: target,
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

func (e *Engine) indexCleanFiles(pair *store.SyncPair, localFiles, remoteFiles []*provider.FileMeta, dbEntries []*store.FileEntry, taskPaths map[string]bool, dir Direction) error {
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
		if !sameSnapshot(localMeta, remoteMeta) {
			continue
		}
		if isSynced(entryMap[key]) {
			continue
		}
		if err := e.store.UpsertFileEntry(&store.FileEntry{
			Path:        key,
			SyncPairID:  pair.ID,
			LocalMTime:  &localMeta.ModTime,
			RemoteMTime: &remoteMeta.ModTime,
			LocalSize:   localMeta.Size,
			RemoteSize:  remoteMeta.Size,
			SyncState:   "synced",
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
		var target provider.Provider
		if task.DeleteTarget == "local" {
			target = local
		} else {
			target = remote
		}
		logger.L.Debug().Str("path", task.Path).Str("target", task.DeleteTarget).Str("pair", pair.Name).Msg("deleting")
		return e.doDelete(ctx, pair, target, task.Path)
	default:
		return fmt.Errorf("unknown task type: %s", task.Type)
	}
}

func (e *Engine) doUpload(ctx context.Context, pair *store.SyncPair, local, remote provider.Provider, filePath string) error {
	reader, meta, err := local.GetFile(ctx, filePath)
	if err != nil {
		return fmt.Errorf("read local file: %w", err)
	}
	defer reader.Close()

	dir := path.Dir(filePath)
	if dir != "/" && dir != "." {
		remote.CreateDir(ctx, dir)
	}

	if err := remote.PutFile(ctx, filePath, reader, meta); err != nil {
		return fmt.Errorf("upload to remote: %w", err)
	}

	remoteMeta, err := remote.Stat(ctx, filePath)
	if err != nil {
		remoteMeta = meta
	}

	if err := e.store.UpsertFileEntry(&store.FileEntry{
		Path:        filePath,
		SyncPairID:  pair.ID,
		LocalMTime:  &meta.ModTime,
		RemoteMTime: &remoteMeta.ModTime,
		LocalSize:   meta.Size,
		RemoteSize:  remoteMeta.Size,
		SyncState:   "synced",
	}); err != nil {
		return fmt.Errorf("record upload: %w", err)
	}

	logger.L.Info().Str("path", filePath).Str("pair", pair.Name).Msg("uploaded")
	return nil
}

func (e *Engine) doDownload(ctx context.Context, pair *store.SyncPair, local, remote provider.Provider, filePath string) error {
	reader, meta, err := remote.GetFile(ctx, filePath)
	if err != nil {
		return fmt.Errorf("read remote file: %w", err)
	}
	defer reader.Close()

	dir := path.Dir(filePath)
	if dir != "/" && dir != "." {
		local.CreateDir(ctx, dir)
	}

	if err := local.PutFile(ctx, filePath, reader, meta); err != nil {
		return fmt.Errorf("download to local: %w", err)
	}

	localMeta, err := local.Stat(ctx, filePath)
	if err != nil {
		localMeta = meta
	}

	if err := e.store.UpsertFileEntry(&store.FileEntry{
		Path:        filePath,
		SyncPairID:  pair.ID,
		LocalMTime:  &localMeta.ModTime,
		RemoteMTime: &meta.ModTime,
		LocalSize:   localMeta.Size,
		RemoteSize:  meta.Size,
		SyncState:   "synced",
	}); err != nil {
		return fmt.Errorf("record download: %w", err)
	}

	logger.L.Info().Str("path", filePath).Str("pair", pair.Name).Msg("downloaded")
	return nil
}

func (e *Engine) doDelete(ctx context.Context, pair *store.SyncPair, target provider.Provider, filePath string) error {
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

	logger.L.Info().Str("path", filePath).Str("pair", pair.Name).Msg("deleted")
	return nil
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
					atomic.AddInt64(&e.pending, -1)
					logger.L.Error().Err(result.Error).Str("path", result.Task.Path).Str("type", string(result.Task.Type)).Msg("task permanently failed")
				}
			} else {
				atomic.AddInt64(&e.pending, -1)
			}
		}
	}
}

func (e *Engine) periodicScan() {
	defer e.wg.Done()

	ticker := time.NewTicker(e.config.ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			logger.L.Debug().Msg("periodic scan triggered")
			if err := e.RefreshPairs(); err != nil {
				logger.L.Error().Err(err).Msg("refresh pairs failed")
			}
			_ = e.SyncAll(e.ctx)
		}
	}
}
