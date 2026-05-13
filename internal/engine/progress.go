package engine

import (
	"sort"
	"sync"
	"time"
)

type ActiveFileProgress struct {
	Path             string    `json:"path"`
	TaskType         string    `json:"task_type"`
	BytesTransferred int64     `json:"bytes_transferred"`
	BytesTotal       int64     `json:"bytes_total"`
	Percent          float64   `json:"percent"`
	StartedAt        time.Time `json:"started_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type PairProgressSnapshot struct {
	PairID       int64               `json:"pair_id"`
	PairName     string              `json:"pair_name"`
	Status       string              `json:"status"`
	Direction    string              `json:"direction"`
	ActiveFile   *ActiveFileProgress `json:"active_file,omitempty"`
	FilesSynced  int                 `json:"files_synced"`
	FilesTotal   int                 `json:"files_total"`
	PendingTasks int64               `json:"pending_tasks"`
	StartedAt    time.Time           `json:"started_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	Error        string              `json:"error,omitempty"`
}

type ProgressTracker struct {
	mu        sync.RWMutex
	snapshots map[int64]*PairProgressSnapshot
}

func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{snapshots: make(map[int64]*PairProgressSnapshot)}
}

func (t *ProgressTracker) StartSync(pairID int64, pairName, direction string, filesTotal int) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snapshots[pairID] = &PairProgressSnapshot{
		PairID:     pairID,
		PairName:   pairName,
		Status:     "syncing",
		Direction:  direction,
		FilesTotal: filesTotal,
		StartedAt:  now,
		UpdatedAt:  now,
	}
}

func (t *ProgressTracker) SetTotals(pairID int64, filesTotal int, pendingTasks int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if snap := t.snapshots[pairID]; snap != nil {
		snap.FilesTotal = filesTotal
		snap.PendingTasks = pendingTasks
		snap.UpdatedAt = time.Now()
	}
}

func (t *ProgressTracker) StartTask(pairID int64, taskType, filePath string, bytesTotal int64) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if snap := t.snapshots[pairID]; snap != nil {
		snap.Status = "syncing"
		snap.ActiveFile = &ActiveFileProgress{
			Path:       filePath,
			TaskType:   taskType,
			BytesTotal: bytesTotal,
			StartedAt:  now,
			UpdatedAt:  now,
		}
		snap.UpdatedAt = now
	}
}

func (t *ProgressTracker) ChunkTransferred(pairID int64, filePath string, transferred, total int64) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	snap := t.snapshots[pairID]
	if snap == nil {
		return
	}
	if snap.ActiveFile == nil || snap.ActiveFile.Path != filePath {
		snap.ActiveFile = &ActiveFileProgress{Path: filePath, StartedAt: now}
	}
	snap.ActiveFile.BytesTransferred = transferred
	snap.ActiveFile.BytesTotal = total
	if total > 0 {
		snap.ActiveFile.Percent = float64(transferred) / float64(total) * 100
	}
	snap.ActiveFile.UpdatedAt = now
	snap.UpdatedAt = now
}

func (t *ProgressTracker) CompleteTask(pairID int64, taskType, filePath string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if snap := t.snapshots[pairID]; snap != nil {
		if taskType == string(TaskUpload) || taskType == string(TaskDownload) {
			snap.FilesSynced++
		}
		if snap.PendingTasks > 0 {
			snap.PendingTasks--
		}
		if snap.ActiveFile != nil && snap.ActiveFile.Path == filePath {
			snap.ActiveFile = nil
		}
		snap.UpdatedAt = time.Now()
	}
}

func (t *ProgressTracker) FailTask(pairID int64, taskType, filePath, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if snap := t.snapshots[pairID]; snap != nil {
		snap.Status = "failed"
		snap.Error = message
		if snap.PendingTasks > 0 {
			snap.PendingTasks--
		}
		if snap.ActiveFile != nil && snap.ActiveFile.Path == filePath {
			snap.ActiveFile = nil
		}
		snap.UpdatedAt = time.Now()
	}
}

func (t *ProgressTracker) FinishSync(pairID int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.snapshots, pairID)
}

func (t *ProgressTracker) Snapshot(pairID int64) *PairProgressSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return clonePairProgress(t.snapshots[pairID])
}

func (t *ProgressTracker) Snapshots() []PairProgressSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ids := make([]int64, 0, len(t.snapshots))
	for id := range t.snapshots {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]PairProgressSnapshot, 0, len(ids))
	for _, id := range ids {
		if snap := clonePairProgress(t.snapshots[id]); snap != nil {
			out = append(out, *snap)
		}
	}
	return out
}

func clonePairProgress(in *PairProgressSnapshot) *PairProgressSnapshot {
	if in == nil {
		return nil
	}
	out := *in
	if in.ActiveFile != nil {
		active := *in.ActiveFile
		out.ActiveFile = &active
	}
	return &out
}
