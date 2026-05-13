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

type FileProgressItem struct {
	Path             string    `json:"path"`
	TaskType         string    `json:"task_type"`
	Status           string    `json:"status"`
	Direction        string    `json:"direction"`
	BytesTransferred int64     `json:"bytes_transferred"`
	BytesTotal       int64     `json:"bytes_total"`
	Percent          float64   `json:"percent"`
	QueuedAt         time.Time `json:"queued_at"`
	StartedAt        time.Time `json:"started_at,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
	FinishedAt       time.Time `json:"finished_at,omitempty"`
	Error            string    `json:"error,omitempty"`
}

type SyncRecord struct {
	ID         int64     `json:"id"`
	PairID     int64     `json:"pair_id"`
	PairName   string    `json:"pair_name"`
	Path       string    `json:"path"`
	TaskType   string    `json:"task_type"`
	Status     string    `json:"status"`
	Direction  string    `json:"direction"`
	BytesTotal int64     `json:"bytes_total"`
	FinishedAt time.Time `json:"finished_at"`
	Error      string    `json:"error,omitempty"`
}

type PairProgressSnapshot struct {
	PairID       int64               `json:"pair_id"`
	PairName     string              `json:"pair_name"`
	Status       string              `json:"status"`
	Direction    string              `json:"direction"`
	ActiveFile   *ActiveFileProgress `json:"active_file,omitempty"`
	Queue        []FileProgressItem  `json:"queue,omitempty"`
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
	records   []SyncRecord
	nextID    int64
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

func (t *ProgressTracker) QueueTask(pairID int64, taskType, filePath, direction string) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	snap := t.snapshots[pairID]
	if snap == nil {
		snap = &PairProgressSnapshot{PairID: pairID, Status: "syncing", StartedAt: now}
		t.snapshots[pairID] = snap
	}
	if direction == "" {
		direction = directionForTask(taskType)
	}
	item := t.queueItemLocked(snap, taskType, filePath, true)
	item.Status = "pending"
	item.Direction = direction
	item.QueuedAt = now
	item.UpdatedAt = now
	snap.UpdatedAt = now
}

func (t *ProgressTracker) StartTask(pairID int64, taskType, filePath string, bytesTotal int64) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if snap := t.snapshots[pairID]; snap != nil {
		item := t.queueItemLocked(snap, taskType, filePath, true)
		item.Status = "syncing"
		if item.Direction == "" {
			item.Direction = directionForTask(taskType)
		}
		item.BytesTotal = bytesTotal
		item.StartedAt = now
		item.UpdatedAt = now
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
	item := t.queueItemLocked(snap, snap.ActiveFile.TaskType, filePath, true)
	item.Status = "syncing"
	item.BytesTransferred = transferred
	item.BytesTotal = total
	if total > 0 {
		item.Percent = float64(transferred) / float64(total) * 100
	}
	item.UpdatedAt = now
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
		now := time.Now()
		item := t.queueItemLocked(snap, taskType, filePath, true)
		item.Status = "completed"
		if item.Direction == "" {
			item.Direction = directionForTask(taskType)
		}
		if item.BytesTotal > 0 {
			item.BytesTransferred = item.BytesTotal
		}
		item.Percent = 100
		item.FinishedAt = now
		item.UpdatedAt = now
		if taskType == string(TaskUpload) || taskType == string(TaskDownload) {
			snap.FilesSynced++
		}
		if snap.PendingTasks > 0 {
			snap.PendingTasks--
		}
		if snap.ActiveFile != nil && snap.ActiveFile.Path == filePath {
			snap.ActiveFile = nil
		}
		snap.UpdatedAt = now
		t.pushRecordLocked(snap, *item, "completed", "")
	}
}

func (t *ProgressTracker) FailTask(pairID int64, taskType, filePath, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if snap := t.snapshots[pairID]; snap != nil {
		now := time.Now()
		item := t.queueItemLocked(snap, taskType, filePath, true)
		item.Status = "failed"
		if item.Direction == "" {
			item.Direction = directionForTask(taskType)
		}
		item.Error = message
		item.FinishedAt = now
		item.UpdatedAt = now
		snap.Status = "failed"
		snap.Error = message
		if snap.PendingTasks > 0 {
			snap.PendingTasks--
		}
		if snap.ActiveFile != nil && snap.ActiveFile.Path == filePath {
			snap.ActiveFile = nil
		}
		snap.UpdatedAt = now
		t.pushRecordLocked(snap, *item, "failed", message)
	}
}

func (t *ProgressTracker) FailSync(pairID int64, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if snap := t.snapshots[pairID]; snap != nil {
		snap.Status = "failed"
		snap.Error = message
		snap.ActiveFile = nil
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

func (t *ProgressTracker) Records(limit int) []SyncRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if limit <= 0 || limit > len(t.records) {
		limit = len(t.records)
	}
	out := make([]SyncRecord, limit)
	copy(out, t.records[:limit])
	return out
}

func (t *ProgressTracker) queueItemLocked(snap *PairProgressSnapshot, taskType, filePath string, create bool) *FileProgressItem {
	for i := range snap.Queue {
		if snap.Queue[i].Path == filePath && (taskType == "" || snap.Queue[i].TaskType == taskType || snap.Queue[i].TaskType == "") {
			if snap.Queue[i].TaskType == "" {
				snap.Queue[i].TaskType = taskType
			}
			return &snap.Queue[i]
		}
	}
	if !create {
		return nil
	}
	now := time.Now()
	snap.Queue = append(snap.Queue, FileProgressItem{
		Path:      filePath,
		TaskType:  taskType,
		Status:    "pending",
		Direction: directionForTask(taskType),
		QueuedAt:  now,
		UpdatedAt: now,
	})
	return &snap.Queue[len(snap.Queue)-1]
}

func (t *ProgressTracker) pushRecordLocked(snap *PairProgressSnapshot, item FileProgressItem, status, message string) {
	if item.Path == "" {
		return
	}
	t.nextID++
	record := SyncRecord{
		ID:         t.nextID,
		PairID:     snap.PairID,
		PairName:   snap.PairName,
		Path:       item.Path,
		TaskType:   item.TaskType,
		Status:     status,
		Direction:  item.Direction,
		BytesTotal: item.BytesTotal,
		FinishedAt: item.FinishedAt,
		Error:      message,
	}
	t.records = append([]SyncRecord{record}, t.records...)
	if len(t.records) > 200 {
		t.records = t.records[:200]
	}
}

func directionForTask(taskType string) string {
	switch taskType {
	case string(TaskUpload):
		return "up"
	case string(TaskDownload):
		return "down"
	default:
		return ""
	}
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
	if in.Queue != nil {
		out.Queue = append([]FileProgressItem(nil), in.Queue...)
	}
	return &out
}
