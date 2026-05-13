# File Sync Progress Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add realtime single-file sync progress to the dashboard, sync-pairs page, and file browser.

**Architecture:** Add a backend in-memory progress tracker exposed through `/api/v1/progress`, update it from sync engine lifecycle events, and hydrate the existing React `SyncProgressProvider` from that snapshot before applying WebSocket deltas. The frontend keeps short recent completed/failed history in memory only and shares reusable progress UI across the three pages.

**Tech Stack:** Go 1.22, net/http, existing Every-Sync engine events, React 19, TypeScript, Vite.

---

## Pre-Flight Notes

The current workspace already has uncommitted changes in `internal/engine/engine.go`. Before implementing, inspect them with:

```bash
git diff -- internal/engine/engine.go
```

Build on those changes when they are relevant. Do not revert them unless the user explicitly asks.

## File Structure

- Create `internal/engine/progress.go`: owns progress snapshot types and `ProgressTracker`.
- Create `internal/engine/progress_test.go`: unit tests for tracker state transitions.
- Modify `internal/engine/engine.go`: add tracker field, expose `Progress()`, emit `task_started`, update tracker on sync/task events.
- Modify `internal/server/handler/ws.go`: extend engine interface and add `Progress` handler.
- Modify `internal/server/handler/ws_test.go`: fake engines implement `Progress()`, add handler test.
- Modify `internal/server/server.go`: register `GET /api/v1/progress`.
- Modify `web/src/api/client.ts`: add progress API types and `getProgressSnapshots()`.
- Modify `web/package.json` and `web/package-lock.json`: add a lightweight test script for progress state tests.
- Create `web/src/hooks/syncProgressState.ts`: pure progress state helpers used by the React provider.
- Create `web/src/hooks/syncProgressState.test.ts`: reducer-style tests for snapshot hydration, recent item limit, and event updates.
- Modify `web/src/hooks/useSyncProgress.tsx`: hydrate from snapshot and track recent completed/failed items.
- Create `web/src/components/ProgressBar.tsx`: shared progress bar.
- Create `web/src/components/PairProgress.tsx`: `PairProgressInline` and `PairProgressDetail`.
- Modify `web/src/pages/Dashboard.tsx`: replace inline progress UI with `PairProgressInline`.
- Modify `web/src/pages/SyncPairs.tsx`: add B-layout list + right detail panel.
- Modify `web/src/pages/FileBrowser.tsx`: add current-file row progress.
- Modify `web/src/i18n/index.tsx`: add English and Chinese labels.

## Task 1: Backend Progress Tracker

**Files:**
- Create: `internal/engine/progress.go`
- Create: `internal/engine/progress_test.go`

- [ ] **Step 1: Write failing tracker tests**

Create `internal/engine/progress_test.go`:

```go
package engine

import "testing"

func TestProgressTrackerLifecycle(t *testing.T) {
	tracker := NewProgressTracker()

	tracker.StartSync(7, "photos", "up", 3)
	tracker.StartTask(7, "upload", "/camera/IMG_1042.CR3", 1024)
	tracker.ChunkTransferred(7, "/camera/IMG_1042.CR3", 256, 1024)

	snapshots := tracker.Snapshots()
	if len(snapshots) != 1 {
		t.Fatalf("snapshots len = %d, want 1", len(snapshots))
	}
	got := snapshots[0]
	if got.PairID != 7 || got.Status != "syncing" || got.Direction != "up" {
		t.Fatalf("snapshot header = %+v", got)
	}
	if got.ActiveFile == nil {
		t.Fatalf("ActiveFile is nil")
	}
	if got.ActiveFile.Path != "/camera/IMG_1042.CR3" {
		t.Fatalf("path = %q", got.ActiveFile.Path)
	}
	if got.ActiveFile.BytesTransferred != 256 || got.ActiveFile.BytesTotal != 1024 {
		t.Fatalf("bytes = %d/%d", got.ActiveFile.BytesTransferred, got.ActiveFile.BytesTotal)
	}
	if got.ActiveFile.Percent != 25 {
		t.Fatalf("percent = %.1f, want 25", got.ActiveFile.Percent)
	}

	tracker.CompleteTask(7, "upload", "/camera/IMG_1042.CR3")
	got = tracker.Snapshot(7)
	if got == nil {
		t.Fatalf("snapshot missing while sync is active")
	}
	if got.ActiveFile != nil {
		t.Fatalf("ActiveFile after completion = %+v, want nil", got.ActiveFile)
	}
	if got.FilesSynced != 1 {
		t.Fatalf("FilesSynced = %d, want 1", got.FilesSynced)
	}

	tracker.FinishSync(7)
	if got := tracker.Snapshot(7); got != nil {
		t.Fatalf("snapshot after FinishSync = %+v, want nil", got)
	}
}

func TestProgressTrackerFailedTask(t *testing.T) {
	tracker := NewProgressTracker()
	tracker.StartSync(8, "docs", "down", 1)
	tracker.StartTask(8, "download", "/docs/report.pdf", 2048)
	tracker.FailTask(8, "download", "/docs/report.pdf", "network timeout")

	got := tracker.Snapshot(8)
	if got == nil {
		t.Fatalf("snapshot missing after failed task")
	}
	if got.Status != "failed" {
		t.Fatalf("Status = %q, want failed", got.Status)
	}
	if got.Error != "network timeout" {
		t.Fatalf("Error = %q", got.Error)
	}
	if got.ActiveFile != nil {
		t.Fatalf("ActiveFile = %+v, want nil", got.ActiveFile)
	}
}
```

- [ ] **Step 2: Run tests and confirm failure**

Run:

```bash
go test ./internal/engine -run 'TestProgressTracker' -count=1
```

Expected: FAIL because `NewProgressTracker` and related types are undefined.

- [ ] **Step 3: Implement tracker**

Create `internal/engine/progress.go`:

```go
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
		PairID:      pairID,
		PairName:    pairName,
		Status:      "syncing",
		Direction:   direction,
		FilesTotal:  filesTotal,
		StartedAt:   now,
		UpdatedAt:   now,
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
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/engine -run 'TestProgressTracker' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/engine/progress.go internal/engine/progress_test.go
git commit -m "feat: add sync progress tracker"
```

## Task 2: Wire Tracker Into Engine and API

**Files:**
- Modify: `internal/engine/engine.go`
- Modify: `internal/server/handler/ws.go`
- Modify: `internal/server/handler/ws_test.go`
- Modify: `internal/server/server.go`

- [ ] **Step 1: Write failing API handler test**

Add to `internal/server/handler/ws_test.go`:

```go
func TestProgressReturnsEngineSnapshots(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	engine := &fakeEngine{
		progress: []syncengine.PairProgressSnapshot{{
			PairID:       7,
			PairName:     "photos",
			Status:       "syncing",
			Direction:    "up",
			FilesSynced:  2,
			FilesTotal:   5,
			PendingTasks: 3,
		}},
	}
	h := New(s, engine, "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/progress", nil)
	rec := httptest.NewRecorder()

	h.Progress(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []syncengine.PairProgressSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].PairID != 7 || got[0].Status != "syncing" {
		t.Fatalf("progress = %+v", got)
	}
}
```

Update both fake engine structs in the same file:

```go
progress []syncengine.PairProgressSnapshot
```

and add methods:

```go
func (f *fakeEngine) Progress() []syncengine.PairProgressSnapshot { return f.progress }
func (f *trackingFakeEngine) Progress() []syncengine.PairProgressSnapshot { return f.progress }
```

- [ ] **Step 2: Run handler test and confirm failure**

Run:

```bash
go test ./internal/server/handler -run TestProgressReturnsEngineSnapshots -count=1
```

Expected: FAIL because `Handler.Progress` and `engine.Progress()` are not wired.

- [ ] **Step 3: Add API handler and route**

In `internal/server/handler/ws.go`, add `Progress()` to both engine interfaces and add:

```go
func (h *Handler) Progress(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.engine.Progress())
}
```

In `internal/server/server.go`, register the route near status:

```go
api.HandleFunc("GET /api/v1/progress", h.Progress)
```

- [ ] **Step 4: Wire tracker into engine**

In `internal/engine/engine.go`, add to `Engine`:

```go
progress *ProgressTracker
```

In `New`, initialize:

```go
progress: NewProgressTracker(),
```

Add method:

```go
func (e *Engine) Progress() []PairProgressSnapshot {
	return e.progress.Snapshots()
}
```

At sync start, call:

```go
e.progress.StartSync(pair.ID, pair.Name, string(dir), 0)
```

After task generation, call:

```go
e.progress.SetTotals(pair.ID, uploadCount+downloadCount, int64(len(tasks)))
```

Before each task executes in `executeTask`, emit and track file tasks:

```go
if task.Type == TaskUpload || task.Type == TaskDownload {
	e.progress.StartTask(task.PairID, string(task.Type), task.Path, 0)
	e.broadcast(Event{Type: "task_started", PairID: task.PairID, TaskType: string(task.Type), Path: task.Path})
}
```

When transfer code knows the file size, call:

```go
e.progress.StartTask(pair.ID, string(taskType), filePath, sourceMeta.Size)
```

In `chunkProgressReader.Read`, before emitting the event or next to the emit call, update through the engine path by adding a tracker callback to `chunkProgressReader`:

```go
track func(pairID int64, filePath string, transferred, total int64)
```

and call:

```go
r.track(r.pair.ID, r.filePath, r.read, r.size)
```

In `transferReader`, set:

```go
track: e.progress.ChunkTransferred,
```

In result handling:

```go
e.progress.CompleteTask(pairID, string(result.Task.Type), result.Task.Path)
```

for permanent failure:

```go
e.progress.FailTask(pairID, string(result.Task.Type), result.Task.Path, result.Error.Error())
```

when the sync cycle completes or sync fails:

```go
e.progress.FinishSync(pairID)
```

- [ ] **Step 5: Run backend tests**

Run:

```bash
go test ./internal/engine ./internal/server/handler -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/engine/engine.go internal/server/handler/ws.go internal/server/handler/ws_test.go internal/server/server.go
git commit -m "feat: expose live sync progress snapshots"
```

## Task 3: Frontend API and Progress Store

**Files:**
- Modify: `web/package.json`
- Modify: `web/package-lock.json`
- Modify: `web/src/api/client.ts`
- Create: `web/src/hooks/syncProgressState.ts`
- Create: `web/src/hooks/syncProgressState.test.ts`
- Modify: `web/src/hooks/useSyncProgress.tsx`

- [ ] **Step 1: Add frontend test runner**

Run:

```bash
cd web && npm install -D vitest
```

Update `web/package.json` scripts:

```json
{
  "scripts": {
    "dev": "vite",
    "build": "tsc --noEmit && vite build",
    "preview": "vite preview",
    "test": "vitest run"
  }
}
```

- [ ] **Step 2: Add failing progress state tests**

Create `web/src/hooks/syncProgressState.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import { applyProgressEvent, createEmptyProgress, mergeProgressSnapshot } from './syncProgressState';

describe('sync progress state', () => {
  it('hydrates an active file from a backend snapshot', () => {
    const entry = mergeProgressSnapshot(new Map(), {
      pair_id: 7,
      pair_name: 'photos',
      status: 'syncing',
      direction: 'up',
      active_file: {
        path: '/camera/IMG_1042.CR3',
        task_type: 'upload',
        bytes_transferred: 256,
        bytes_total: 1024,
        percent: 25,
        started_at: '2026-05-13T00:00:00Z',
        updated_at: '2026-05-13T00:00:01Z',
      },
      files_synced: 2,
      files_total: 5,
      pending_tasks: 3,
      started_at: '2026-05-13T00:00:00Z',
      updated_at: '2026-05-13T00:00:01Z',
    });

    expect(entry.pairId).toBe('7');
    expect(entry.currentFile).toBe('/camera/IMG_1042.CR3');
    expect(entry.activeFile?.percent).toBe(25);
    expect(entry.filesSynced).toBe(2);
    expect(entry.filesTotal).toBe(5);
  });

  it('keeps only five recent completed or failed items', () => {
    const entry = createEmptyProgress('7');
    const map = new Map([['7', entry]]);

    for (let i = 0; i < 6; i += 1) {
      applyProgressEvent(map, {
        type: 'task_completed',
        pair_id: '7',
        task_type: 'upload',
        path: `/file-${i}.txt`,
      });
    }

    expect(entry.recentItems).toHaveLength(5);
    expect(entry.recentItems[0].path).toBe('/file-5.txt');
    expect(entry.recentItems[4].path).toBe('/file-1.txt');
  });

  it('updates active file bytes from chunk events', () => {
    const entry = createEmptyProgress('7');
    const map = new Map([['7', entry]]);

    applyProgressEvent(map, {
      type: 'task_started',
      pair_id: '7',
      task_type: 'download',
      path: '/docs/report.pdf',
    });
    applyProgressEvent(map, {
      type: 'chunk_transferred',
      pair_id: '7',
      task_type: 'download',
      path: '/docs/report.pdf',
      bytes_transferred: 512,
      bytes_total: 1024,
    });

    expect(entry.activeFile?.path).toBe('/docs/report.pdf');
    expect(entry.activeFile?.bytesTransferred).toBe(512);
    expect(entry.activeFile?.percent).toBe(50);
  });
});
```

- [ ] **Step 3: Run tests and confirm failure**

Run:

```bash
cd web && npm test -- --run src/hooks/syncProgressState.test.ts
```

Expected: FAIL because `syncProgressState.ts` does not exist.

- [ ] **Step 4: Add API types and fetcher**

In `web/src/api/client.ts`, add:

```ts
export interface ActiveFileProgress {
  path: string;
  task_type: 'upload' | 'download';
  bytes_transferred: number;
  bytes_total: number;
  percent: number;
  started_at: string;
  updated_at: string;
}

export interface PairProgressSnapshot {
  pair_id: string | number;
  pair_name?: string;
  status: 'idle' | 'scanning' | 'syncing' | 'completed' | 'failed';
  direction: SyncDirection | '';
  active_file?: ActiveFileProgress;
  files_synced: number;
  files_total: number;
  pending_tasks: number;
  started_at?: string;
  updated_at: string;
  error?: string;
}

export async function getProgressSnapshots(): Promise<PairProgressSnapshot[]> {
  return fetchJSON<PairProgressSnapshot[]>('/progress');
}
```

- [ ] **Step 5: Create pure progress state helpers**

Create `web/src/hooks/syncProgressState.ts`:

```ts
import type { PairProgressSnapshot, WSEvent } from '../api/client';

export interface RecentProgressItem {
  path: string;
  taskType: string;
  status: 'completed' | 'failed';
  bytesTotal?: number;
  error?: string;
  finishedAt: string;
}

export interface ActiveFileProgressView {
  path: string;
  taskType: string;
  bytesTransferred: number;
  bytesTotal: number;
  percent: number;
  startedAt: string;
  updatedAt: string;
}

export interface SyncProgress {
  pairId: string;
  status: 'idle' | 'scanning' | 'syncing' | 'completed' | 'failed';
  currentFile: string;
  activeFile?: ActiveFileProgressView;
  recentItems: RecentProgressItem[];
  filesSynced: number;
  filesTotal: number;
  pendingTasks: number;
  bytesTransferred: number;
  bytesTotal: number;
  direction: string;
  startedAt: string;
  lastUpdated: string;
  error?: string;
}

export type ProgressMap = Map<string, SyncProgress>;

export function createEmptyProgress(pairId: string): SyncProgress {
  return {
    pairId,
    status: 'idle',
    currentFile: '',
    recentItems: [],
    filesSynced: 0,
    filesTotal: 0,
    pendingTasks: 0,
    bytesTransferred: 0,
    bytesTotal: 0,
    direction: '',
    startedAt: '',
    lastUpdated: '',
  };
}

export function ensureProgress(map: ProgressMap, pairId: string): SyncProgress {
  let entry = map.get(pairId);
  if (!entry) {
    entry = createEmptyProgress(pairId);
    map.set(pairId, entry);
  }
  return entry;
}

export function mergeProgressSnapshot(map: ProgressMap, snapshot: PairProgressSnapshot): SyncProgress {
  const pairId = String(snapshot.pair_id);
  const entry = ensureProgress(map, pairId);
  entry.status = snapshot.status;
  entry.direction = snapshot.direction;
  entry.filesSynced = snapshot.files_synced ?? 0;
  entry.filesTotal = snapshot.files_total ?? 0;
  entry.pendingTasks = snapshot.pending_tasks ?? 0;
  entry.startedAt = snapshot.started_at ?? entry.startedAt;
  entry.lastUpdated = snapshot.updated_at ?? new Date().toISOString();
  entry.error = snapshot.error;
  if (snapshot.active_file) {
    entry.currentFile = snapshot.active_file.path;
    entry.activeFile = {
      path: snapshot.active_file.path,
      taskType: snapshot.active_file.task_type,
      bytesTransferred: snapshot.active_file.bytes_transferred ?? 0,
      bytesTotal: snapshot.active_file.bytes_total ?? 0,
      percent: snapshot.active_file.percent ?? 0,
      startedAt: snapshot.active_file.started_at,
      updatedAt: snapshot.active_file.updated_at,
    };
    entry.bytesTransferred = entry.activeFile.bytesTransferred;
    entry.bytesTotal = entry.activeFile.bytesTotal;
  }
  return entry;
}

export function applyProgressEvent(map: ProgressMap, event: WSEvent): boolean {
  const pairID = 'pair_id' in event && event.pair_id != null ? String(event.pair_id) : '';
  if (!pairID) return false;
  const entry = ensureProgress(map, pairID);
  const anyEvent = event as Record<string, unknown>;
  const now = new Date().toISOString();
  entry.lastUpdated = now;

  if (event.type === 'task_started') {
    const path = String(anyEvent.path ?? '');
    entry.status = 'syncing';
    entry.currentFile = path;
    entry.activeFile = {
      path,
      taskType: String(anyEvent.task_type ?? ''),
      bytesTransferred: 0,
      bytesTotal: 0,
      percent: 0,
      startedAt: now,
      updatedAt: now,
    };
    return true;
  }

  if (event.type === 'chunk_transferred') {
    const path = String(anyEvent.path ?? entry.currentFile);
    const bytesTransferred = Number(anyEvent.bytes_transferred ?? 0);
    const bytesTotal = Number(anyEvent.bytes_total ?? 0);
    entry.currentFile = path;
    entry.activeFile = {
      path,
      taskType: String(anyEvent.task_type ?? entry.activeFile?.taskType ?? ''),
      bytesTransferred,
      bytesTotal,
      percent: bytesTotal > 0 ? (bytesTransferred / bytesTotal) * 100 : 0,
      startedAt: entry.activeFile?.startedAt ?? now,
      updatedAt: now,
    };
    entry.bytesTransferred = bytesTransferred;
    entry.bytesTotal = bytesTotal;
    return true;
  }

  if (event.type === 'task_completed' || event.type === 'task_failed') {
    pushRecent(entry, {
      path: String(anyEvent.path ?? entry.currentFile),
      taskType: String(anyEvent.task_type ?? ''),
      status: event.type === 'task_failed' ? 'failed' : 'completed',
      error: anyEvent.error ? String(anyEvent.error) : undefined,
      finishedAt: now,
    });
    if (entry.activeFile?.path === anyEvent.path) {
      entry.activeFile = undefined;
    }
    return true;
  }

  return false;
}

export function pushRecent(entry: SyncProgress, item: RecentProgressItem): void {
  entry.recentItems = [item, ...entry.recentItems].slice(0, 5);
}
```

- [ ] **Step 6: Update progress hook model**

In `web/src/hooks/useSyncProgress.tsx`, import shared model helpers:

```ts
import { applyProgressEvent, createEmptyProgress, mergeProgressSnapshot } from './syncProgressState';
import type { ProgressMap, SyncProgress } from './syncProgressState';
import { getProgressSnapshots } from '../api/client';

export type { RecentProgressItem, SyncProgress } from './syncProgressState';
```

Add snapshot hydration in provider mount:

```ts
useEffect(() => {
  let cancelled = false;
  getProgressSnapshots()
    .then((snapshots) => {
      if (cancelled) return;
      const map = progressRef.current;
      snapshots.forEach((snapshot) => mergeSnapshot(map, snapshot));
      dirtyRef.current = true;
      setSnapshot(new Map(map));
    })
    .catch(() => {
      // Progress is best-effort; WebSocket events can still populate state.
    });
  return () => { cancelled = true; };
}, []);
```

Replace the existing event-specific mutation body with `applyProgressEvent(map, event)` for `task_started`, `chunk_transferred`, `task_completed`, and `task_failed`, while preserving existing handling for `sync_started`, `sync_completed`, `status_change`, and `pair_unregistered`.

- [ ] **Step 7: Run frontend tests and build**

Run:

```bash
cd web && npm test -- --run src/hooks/syncProgressState.test.ts
cd web && npm run build
```

Expected: both commands PASS.

- [ ] **Step 8: Commit**

```bash
git add web/package.json web/package-lock.json web/src/api/client.ts web/src/hooks/syncProgressState.ts web/src/hooks/syncProgressState.test.ts web/src/hooks/useSyncProgress.tsx
git commit -m "feat(web): hydrate sync progress state"
```

## Task 4: Shared Progress Components

**Files:**
- Create: `web/src/components/ProgressBar.tsx`
- Create: `web/src/components/PairProgress.tsx`
- Modify: `web/src/i18n/index.tsx`

- [ ] **Step 1: Create shared progress bar**

Create `web/src/components/ProgressBar.tsx`:

```tsx
import React from 'react';

export const ProgressBar: React.FC<{ percent: number; tone?: 'blue' | 'green'; height?: number }> = ({
  percent,
  tone = 'blue',
  height = 4,
}) => {
  const clamped = Math.min(100, Math.max(0, Number.isFinite(percent) ? percent : 0));
  const color = tone === 'green' ? 'var(--accent-green)' : 'var(--accent-blue)';
  return (
    <div style={{ width: '100%', height, background: 'var(--border-default)', borderRadius: height, overflow: 'hidden' }}>
      <div style={{ width: `${clamped}%`, height: '100%', background: color, transition: 'width 0.2s ease' }} />
    </div>
  );
};
```

- [ ] **Step 2: Create pair progress components**

Create `web/src/components/PairProgress.tsx` with:

```tsx
import React from 'react';
import type { SyncProgress } from '../hooks/useSyncProgress';
import { SyncIcon, WarningIcon } from './Icons';
import { ProgressBar } from './ProgressBar';

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

function truncatePath(path: string, max = 48): string {
  return path.length <= max ? path : `...${path.slice(-(max - 3))}`;
}

export const PairProgressInline: React.FC<{ progress?: SyncProgress; t: (key: string) => string }> = ({ progress, t }) => {
  if (!progress || (progress.status !== 'syncing' && progress.status !== 'scanning')) return null;
  const percent = progress.activeFile?.percent ?? (progress.filesTotal > 0 ? (progress.filesSynced / progress.filesTotal) * 100 : 0);
  return (
    <div style={{ marginTop: 'var(--space-2)', display: 'grid', gap: 'var(--space-1)' }}>
      <ProgressBar percent={percent} />
      <div style={{ display: 'flex', gap: 'var(--space-3)', flexWrap: 'wrap', fontSize: 'var(--text-xs)', color: 'var(--text-tertiary)' }}>
        <span>{progress.currentFile ? truncatePath(progress.currentFile) : t('progress.processing')}</span>
        {progress.activeFile && <span>{formatBytes(progress.activeFile.bytesTransferred)} / {formatBytes(progress.activeFile.bytesTotal)}</span>}
        {progress.filesTotal > 0 && <span>{progress.filesSynced}/{progress.filesTotal}</span>}
      </div>
    </div>
  );
};

export const PairProgressDetail: React.FC<{ progress?: SyncProgress; pairName?: string; t: (key: string) => string }> = ({ progress, pairName, t }) => {
  if (!progress) {
    return <div className="card" style={{ padding: 'var(--space-5)', color: 'var(--text-tertiary)' }}>{t('progress.noActive')}</div>;
  }
  const percent = progress.activeFile?.percent ?? 0;
  return (
    <div className="card" style={{ padding: 'var(--space-5)', display: 'grid', gap: 'var(--space-3)' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-3)' }}>
        <strong>{pairName ?? progress.pairId}</strong>
        <span style={{ color: 'var(--accent-blue)', fontFamily: 'var(--font-mono)' }}>{Math.round(percent)}%</span>
      </div>
      <ProgressBar percent={percent} />
      <div style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)' }}>
        <div>{t('progress.current')}: {progress.currentFile ? truncatePath(progress.currentFile, 64) : t('progress.processing')}</div>
        <div>{t('progress.queue')}: {progress.filesSynced}/{progress.filesTotal}</div>
        {progress.activeFile && <div>{t('progress.transfer')}: {formatBytes(progress.activeFile.bytesTransferred)} / {formatBytes(progress.activeFile.bytesTotal)}</div>}
      </div>
      <div style={{ borderTop: '1px solid var(--border-muted)', paddingTop: 'var(--space-3)' }}>
        <div style={{ fontSize: 'var(--text-xs)', color: 'var(--text-tertiary)', marginBottom: 'var(--space-2)' }}>{t('progress.recent')}</div>
        {progress.recentItems.length === 0 ? (
          <div style={{ fontSize: 'var(--text-xs)', color: 'var(--text-tertiary)' }}>{t('progress.noRecent')}</div>
        ) : progress.recentItems.map((item) => (
          <div key={`${item.finishedAt}-${item.path}`} style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', fontSize: 'var(--text-xs)', color: item.status === 'failed' ? 'var(--accent-red)' : 'var(--text-secondary)' }}>
            {item.status === 'failed' ? <WarningIcon size={13} color="var(--accent-red)" /> : <SyncIcon size={13} color="var(--accent-green)" />}
            <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{truncatePath(item.path, 42)}</span>
          </div>
        ))}
      </div>
    </div>
  );
};
```

- [ ] **Step 3: Add i18n labels**

In `web/src/i18n/index.tsx`, add English and Chinese keys:

```ts
'progress.processing': 'Processing...',
'progress.noActive': 'No active transfer',
'progress.current': 'Current',
'progress.queue': 'Queue',
'progress.transfer': 'Transfer',
'progress.recent': 'Recent',
'progress.noRecent': 'No recent files',
```

```ts
'progress.processing': '处理中...',
'progress.noActive': '没有正在传输的文件',
'progress.current': '当前文件',
'progress.queue': '队列',
'progress.transfer': '传输',
'progress.recent': '最近记录',
'progress.noRecent': '暂无最近文件',
```

- [ ] **Step 4: Build frontend**

Run:

```bash
cd web && npm run build
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/ProgressBar.tsx web/src/components/PairProgress.tsx web/src/i18n/index.tsx
git commit -m "feat(web): add sync progress components"
```

## Task 5: Integrate Progress Into Pages

**Files:**
- Modify: `web/src/pages/Dashboard.tsx`
- Modify: `web/src/pages/SyncPairs.tsx`
- Modify: `web/src/pages/FileBrowser.tsx`

- [ ] **Step 1: Update dashboard**

In `web/src/pages/Dashboard.tsx`, remove the local `ProgressBar` component and import:

```ts
import { PairProgressInline } from '../components/PairProgress';
```

Replace row-level active progress markup under pair name with:

```tsx
<PairProgressInline progress={progress} t={t} />
```

- [ ] **Step 2: Update sync pairs page layout**

In `web/src/pages/SyncPairs.tsx`, import:

```ts
import { PairProgressDetail, PairProgressInline } from '../components/PairProgress';
```

Add selected pair state:

```ts
const [selectedPairId, setSelectedPairId] = useState<string>('');
```

After loading pairs, default to the first syncing pair or first pair:

```ts
setSelectedPairId((current) => current || p.find((pair) => getProgress(pair.id)?.status === 'syncing')?.id || p[0]?.id || '');
```

Wrap the list and detail panel:

```tsx
<div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) 300px', gap: 'var(--space-4)', alignItems: 'start' }}>
  <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
    {/* existing pair cards */}
  </div>
  <PairProgressDetail
    progress={selectedPairId ? getProgress(selectedPairId) : undefined}
    pairName={pairs.find((pair) => pair.id === selectedPairId)?.name}
    t={t}
  />
</div>
```

On each pair card, set selection:

```tsx
onClick={() => setSelectedPairId(pair.id)}
```

and render:

```tsx
<PairProgressInline progress={progress} t={t} />
```

- [ ] **Step 3: Update file browser row progress**

In `web/src/pages/FileBrowser.tsx`, pass active progress into `FileRow`:

```tsx
activeFile={syncProgress?.activeFile}
```

Extend props:

```ts
activeFile?: { path: string; bytesTransferred: number; bytesTotal: number; percent: number; taskType: string };
```

Inside `FileRow`, compute:

```ts
const fileProgressActive = Boolean(activeFile && activeFile.path === entry.path);
```

Under the filename, render when active:

```tsx
{fileProgressActive && activeFile && (
  <div style={{ marginTop: '4px', display: 'grid', gap: '3px' }}>
    <ProgressBar percent={activeFile.percent} />
    <span style={{ fontSize: 'var(--text-xs)', color: 'var(--accent-blue)' }}>
      {activeFile.taskType} · {formatSize(activeFile.bytesTransferred)} / {formatSize(activeFile.bytesTotal)} · {Math.round(activeFile.percent)}%
    </span>
  </div>
)}
```

- [ ] **Step 4: Build frontend**

Run:

```bash
cd web && npm run build
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/Dashboard.tsx web/src/pages/SyncPairs.tsx web/src/pages/FileBrowser.tsx
git commit -m "feat(web): show file progress in sync views"
```

## Task 6: Final Verification

**Files:**
- Verify only.

- [ ] **Step 1: Run Go tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 2: Run frontend build**

Run:

```bash
cd web && npm run build
```

Expected: PASS.

- [ ] **Step 3: Manual realtime verification**

Start the server:

```bash
make build
./bin/every-sync serve
```

Open `http://localhost:10086`, trigger a sync for a pair with a large file, and verify:

- Dashboard active pair row shows current file and progress.
- Sync pairs page shows row summary and right-side detail.
- File browser highlights only the matching current file path.
- Refreshing the page restores active progress only if the backend sync is still running.
- Recent completed/failed records disappear after browser refresh.
