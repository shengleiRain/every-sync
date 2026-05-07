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
	TaskUpload  TaskType = "upload"
	TaskDownload TaskType = "download"
	TaskDelete  TaskType = "delete"
	TaskMove    TaskType = "move"
	TaskHash    TaskType = "hash"
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

	dir := Direction(direction)
	if dir == "" {
		dir = Direction(pair.Direction)
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

	if dir == DirectionUp || dir == DirectionBoth {
		localFiles, err = e.scanRecursive(ctx, local, "/")
		if err != nil {
			return fmt.Errorf("scan local: %w", err)
		}
	}

	if dir == DirectionDown || dir == DirectionBoth {
		remoteFiles, err = e.scanRecursive(ctx, remote, "/")
		if err != nil {
			return fmt.Errorf("scan remote: %w", err)
		}
	}

	logger.L.Info().
		Str("pair", pair.Name).
		Int("local_files", len(localFiles)).
		Int("remote_files", len(remoteFiles)).
		Msg("scan complete")

	// Load DB entries for deletion detection
	dbEntries, _ := e.store.ListFileEntriesByPair(pair.ID)

	tasks := e.generateTasks(pair, localFiles, remoteFiles, dbEntries, dir)

	uploadCount, downloadCount, deleteCount := 0, 0, 0
	for _, t := range tasks {
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

	if dir == DirectionUp || dir == DirectionBoth {
		for key, localMeta := range localMap {
			remoteMeta, exists := remoteMap[key]
			if !exists {
				tasks = append(tasks, SyncTask{
					Type:     TaskUpload,
					PairID:   pair.ID,
					Path:     key,
					Priority: 2,
				})
			} else if localMeta.ModTime.After(remoteMeta.ModTime) {
				tasks = append(tasks, SyncTask{
					Type:     TaskUpload,
					PairID:   pair.ID,
					Path:     key,
					Priority: 2,
				})
			}
			delete(remoteMap, key)
		}
	}

	if dir == DirectionDown || dir == DirectionBoth {
		for key, remoteMeta := range remoteMap {
			localMeta, exists := localMap[key]
			if !exists {
				tasks = append(tasks, SyncTask{
					Type:     TaskDownload,
					PairID:   pair.ID,
					Path:     key,
					Priority: 2,
				})
			} else if remoteMeta.ModTime.After(localMeta.ModTime) {
				tasks = append(tasks, SyncTask{
					Type:     TaskDownload,
					PairID:   pair.ID,
					Path:     key,
					Priority: 2,
				})
			}
		}
	}

	// Deletion detection: check DB entries against current scan results
	for _, entry := range dbEntries {
		if entry.SyncState != "synced" {
			continue
		}

		key := path.Clean(entry.Path)
		_, inLocal := localMap[key]
		_, inRemote := remoteMap[key]

		// File deleted from local (only detect if local was scanned)
		if (dir == DirectionUp || dir == DirectionBoth) && !inLocal {
			tasks = append(tasks, SyncTask{
				Type:         TaskDelete,
				PairID:       pair.ID,
				Path:         key,
				Priority:     1,
				DeleteTarget: "remote",
			})
			continue
		}

		// File deleted from remote (only detect if remote was scanned)
		if (dir == DirectionDown || dir == DirectionBoth) && !inRemote {
			tasks = append(tasks, SyncTask{
				Type:         TaskDelete,
				PairID:       pair.ID,
				Path:         key,
				Priority:     1,
				DeleteTarget: "local",
			})
		}
	}

	return tasks
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

	// Upsert file entry as synced
	e.store.UpsertFileEntry(&store.FileEntry{
		Path:       filePath,
		SyncPairID: pair.ID,
		LocalMTime: &meta.ModTime,
		LocalSize:  meta.Size,
		SyncState:  "synced",
	})

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

	// Upsert file entry as synced
	e.store.UpsertFileEntry(&store.FileEntry{
		Path:       filePath,
		SyncPairID: pair.ID,
		RemoteMTime: &meta.ModTime,
		RemoteSize: meta.Size,
		SyncState:  "synced",
	})

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
			atomic.AddInt64(&e.pending, -1)
			if result.Error != nil {
				if result.Task.Retries < e.config.RetryMax {
					result.Task.Retries++
					logger.L.Warn().Err(result.Error).Str("path", result.Task.Path).Int("retry", result.Task.Retries).Msg("task failed, retrying")
					time.AfterFunc(e.config.RetryDelay, func() {
						select {
						case e.taskQueue <- result.Task:
						case <-e.ctx.Done():
						}
					})
				} else {
					logger.L.Error().Err(result.Error).Str("path", result.Task.Path).Str("type", string(result.Task.Type)).Msg("task permanently failed")
				}
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
