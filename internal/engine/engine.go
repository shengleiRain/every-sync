package engine

import (
	"context"
	"fmt"
	"path"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

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
	ID       string
	Type     TaskType
	PairID   int64
	Path     string
	Priority int
	Retries  int
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

type Engine struct {
	store    *store.Store
	config   Config
	locals   map[int64]provider.Provider // pairID -> local provider
	remotes  map[int64]provider.Provider // pairID -> remote provider
	pairs    map[int64]*store.SyncPair

	taskQueue chan SyncTask
	results   chan TaskResult

	pending  int64 // atomic counter for pending tasks
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
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

// RegisterPair binds providers to a sync pair.
func (e *Engine) RegisterPair(pair *store.SyncPair, local, remote provider.Provider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pairs[pair.ID] = pair
	e.locals[pair.ID] = local
	e.remotes[pair.ID] = remote
}

// UnregisterPair removes a sync pair from the engine.
func (e *Engine) UnregisterPair(pairID int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.pairs, pairID)
	delete(e.locals, pairID)
	delete(e.remotes, pairID)
}

// Start launches worker goroutines and the result processor.
func (e *Engine) Start(parent context.Context) error {
	e.mu.Lock()
	e.ctx, e.cancel = context.WithCancel(parent)
	e.mu.Unlock()

	// Start workers
	for i := 0; i < e.config.MaxWorkers; i++ {
		e.wg.Add(1)
		go e.worker(i)
	}

	// Start result processor
	e.wg.Add(1)
	go e.processResults()

	// Start periodic scanner
	e.wg.Add(1)
	go e.periodicScan()

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
			return fmt.Errorf("sync pair %d: %w", id, err)
		}
	}
	return nil
}

func (e *Engine) syncOnePair(ctx context.Context, pair *store.SyncPair, local, remote provider.Provider, dir Direction) error {
	// Phase 1: Scan
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

	// Phase 2: Compare and generate tasks
	tasks := e.generateTasks(pair, localFiles, remoteFiles, dir)

	// Phase 3: Enqueue tasks
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

func (e *Engine) generateTasks(pair *store.SyncPair, localFiles, remoteFiles []*provider.FileMeta, dir Direction) []SyncTask {
	var tasks []SyncTask

	// Build lookup maps keyed by relative path (normalized)
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
		// Upload: local files not in remote or modified
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
		// Download: remote files not in local or modified
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
		return e.doUpload(ctx, pair, local, remote, task.Path)
	case TaskDownload:
		return e.doDownload(ctx, pair, local, remote, task.Path)
	case TaskDelete:
		return e.doDelete(ctx, pair, remote, task.Path)
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

	// Ensure parent directory exists on remote
	dir := path.Dir(filePath)
	if dir != "/" && dir != "." {
		remote.CreateDir(ctx, dir)
	}

	if err := remote.PutFile(ctx, filePath, reader, meta); err != nil {
		return fmt.Errorf("upload to remote: %w", err)
	}

	return nil
}

func (e *Engine) doDownload(ctx context.Context, pair *store.SyncPair, local, remote provider.Provider, filePath string) error {
	reader, meta, err := remote.GetFile(ctx, filePath)
	if err != nil {
		return fmt.Errorf("read remote file: %w", err)
	}
	defer reader.Close()

	// Ensure parent directory exists on local
	dir := path.Dir(filePath)
	if dir != "/" && dir != "." {
		local.CreateDir(ctx, dir)
	}

	if err := local.PutFile(ctx, filePath, reader, meta); err != nil {
		return fmt.Errorf("download to local: %w", err)
	}

	return nil
}

func (e *Engine) doDelete(_ context.Context, _ *store.SyncPair, _ provider.Provider, _ string) error {
	// Will be expanded in conflict resolution phase
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
				// Retry logic
				if result.Task.Retries < e.config.RetryMax {
					result.Task.Retries++
					time.AfterFunc(e.config.RetryDelay, func() {
						select {
						case e.taskQueue <- result.Task:
						case <-e.ctx.Done():
						}
					})
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
			_ = e.SyncAll(e.ctx)
		}
	}
}
