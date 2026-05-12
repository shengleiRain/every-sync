# EverySync UI/UX Fixes & Improvements Implementation Plan

## Overview

Fix 6 reported issues in the every-sync project, covering backend API gaps and frontend UI/UX improvements. The project is a Go backend + React (TypeScript) frontend file synchronization tool.

**Branch**: `feat/sync-roadmap-and-conflict`

---

## Task 1: Backend API Fixes

### 1a. Delete Sync Pair — Clean Up Active Sync

**Problem**: `DeletePair` handler only deletes from DB. Engine's `pairs` map, providers, and pending tasks remain.

**Fix in** `internal/server/handler/ws.go` → `DeletePair()`:
- Before `h.store.DeleteSyncPair(id)`, call `h.engine.UnregisterPair(id)` to stop watchers, remove providers, and broadcast `pair_unregistered` event.
- This ensures the engine cleans up properly before the DB record is removed.

**Fix in** `internal/engine/engine.go` → `UnregisterPair()`:
- Already handles removing from `pairs`, `locals`, `remotes` maps and closing providers.
- No additional changes needed — just wire it up from the handler.

### 1b. Delete Provider — Cascade Check

**Problem**: Deleting a provider doesn't check if sync pairs reference it.

**Fix in** `internal/server/handler/ws.go` → `DeleteProvider()`:
- Before deleting, query `h.store.ListSyncPairs()` to find pairs with `provider` matching the provider config's `name`.
- If pairs exist, return HTTP 409 Conflict with JSON body: `{"error": "provider has dependent sync pairs", "pairs": [{"id":..., "name":...}]}`
- The frontend will use this to show the dependent pairs and ask for confirmation.
- If `force=true` query param is present, also unregister and delete all dependent pairs first, then delete the provider.

**Fix in** `internal/store/store.go`:
- Add `ListSyncPairsByProvider(providerName string) ([]*SyncPair, error)` for efficient querying.

### 1c. Provider Connection Test Endpoint

**Problem**: No REST API for testing provider connectivity (only CLI exists).

**Fix in** `internal/server/handler/ws.go`:
- Add `TestProvider(w, r)` handler at `POST /api/v1/providers/test`.
- Accept body: `{"type": "webdav", "params": {"endpoint": "...", ...}}` (for unsaved providers) OR `{"id": 123}` (for saved providers).
- Use the provider's `Init()` method with a 10-second timeout context.
- Return `{"status": "ok"}` or `{"status": "failed", "error": "..."}`.

**Fix in** `internal/server/handler/routes.go` (or wherever routes are registered):
- Register `POST /api/v1/providers/test` → `TestProvider`.

### 1d. Implement Log Reading API

**Problem**: `ListLogs` handler returns empty array. Logs are written to file but never served.

**Fix in** `internal/server/handler/ws.go` → `ListLogs()`:
- Read the last N lines from the log file at `{data_dir}/logs/every-sync.log`.
- Accept query params: `limit` (default 200), `level` filter, `pair_id` filter.
- Parse each JSON log line into a structured response: `{id, timestamp, level, message, pair_id}`.
- For console-format logs, parse the console output format.
- Return as JSON array.

**Implementation approach**:
- Add a `LogReader` that reads from the rotating log file.
- Use `os.Seek` to read from the end of file, or read the last N lines.
- Parse each line based on the log format (JSON or console).
- The handler gets the log file path from the config or store.

### 1e. Backend: Event Enhancement for Sync Progress

**Problem**: The Engine already broadcasts `chunk_transferred` events but the Event struct lacks file progress details.

**Fix in** `internal/engine/engine.go`:
- Extend the `Event` struct to include progress fields: `BytesTransferred`, `BytesTotal`, `CurrentFile`, `FilesSynced`, `FilesTotal`.
- In the chunk transfer code (around line 1879), populate these fields in the broadcast event.
- In `sync_started` event, include `FilesTotal` from scan results.
- In `sync_completed` event, include `FilesSynced`, `BytesTransferred`.

**Event struct changes**:
```go
type Event struct {
    // ... existing fields ...
    BytesTransferred int64   `json:"bytes_transferred,omitempty"`
    BytesTotal       int64   `json:"bytes_total,omitempty"`
    CurrentFile      string  `json:"current_file,omitempty"`
    FilesSynced      int     `json:"files_synced,omitempty"`
    FilesTotal       int     `json:"files_total,omitempty"`
}
```

---

## Task 2: Frontend Shared Sync Progress State

### Create `useSyncProgress` Hook

**New file**: `web/src/hooks/useSyncProgress.ts`

**Purpose**: Centralized state management for real-time sync progress, consumed by Dashboard, SyncPairs, and FileBrowser.

**Data structure**:
```typescript
interface SyncProgress {
  pairId: string;
  status: 'idle' | 'scanning' | 'syncing' | 'completed' | 'failed';
  currentFile: string;
  filesSynced: number;
  filesTotal: number;
  bytesTransferred: number;
  bytesTotal: number;
  direction: string;
  startedAt: string;
  lastUpdated: string;
}
```

**Implementation**:
- Uses `useWebSocket` internally to listen for events.
- Maintains a `Map<string, SyncProgress>` (pairId → progress).
- Handles events: `sync_started`, `sync_completed`, `sync_failed`, `task_queued`, `task_completed`, `task_failed`, `chunk_transferred`, `pair_unregistered`.
- Provides: `getProgress(pairId)`, `allProgress`, `isSyncing(pairId)`.
- Exported as both a hook and a React Context so multiple pages share the same state.

**Context Provider**: `SyncProgressProvider` wraps the app in `App.tsx`.

---

## Task 3: Frontend Dashboard Real-Time Progress Display

### Fix Dashboard to use WebSocket for live updates

**File**: `web/src/pages/Dashboard.tsx`

**Changes**:
1. **Use `useSyncProgress` hook** instead of one-time load for pair status.
2. **Update pair status in real-time**: When `sync_started` → show "syncing" state. When `sync_completed` → refresh pair stats. When `chunk_transferred` → update progress.
3. **Active pair progress display**: In the active pairs table, replace the static status column with:
   - Progress bar (percentage based on filesSynced/filesTotal or bytesTransferred/bytesTotal)
   - Current file name (truncated with ellipsis)
   - Transfer speed indicator
4. **Fix sync pair count**: After delete, the count updates because it's derived from engine status + pair list, both refreshed via WebSocket.
5. **Add WebSocket-driven refresh**: Listen for `pair_unregistered`, `pair_registered` events to trigger `load()`.

**UI Design** (following existing dark theme):
- Progress bar: Thin bar under the pair name, green for active, blue for complete.
- Current file: Small text below progress bar, `var(--text-tertiary)` color, truncated.
- Status: Animated sync icon (spinning) when syncing.

---

## Task 4: Frontend SyncPairs Page Progress Indicators

**File**: `web/src/pages/SyncPairs.tsx`

**Changes**:
1. **Use `useSyncProgress` hook** to get real-time progress per pair.
2. **Active sync overlay**: When a pair is syncing, show:
   - Animated spinning sync icon
   - Progress bar (files synced / total)
   - Current file name
   - Disable edit/delete buttons during sync
3. **Delete confirmation enhancement**: Show "This pair is currently syncing. Deleting will stop the sync." warning.
4. **Post-delete refresh**: Already calls `load()` after delete; ensure it picks up the pair_unregistered WebSocket event.

---

## Task 5: Frontend FileBrowser Sync Status Indicators

**File**: `web/src/pages/FileBrowser.tsx`

**Changes**:
1. **Use `useSyncProgress` hook** to check if the selected pair is actively syncing.
2. **Syncing banner**: When the selected pair is syncing, show a banner at the top:
   - "Syncing in progress: [current file] ([progress]%)"
   - Thin progress bar across the width
3. **File status live update**: When `task_completed` or `task_failed` events arrive for files in the current directory, refresh the file list.
4. **Syncing file highlight**: Files currently being synced get a subtle animated border/glow effect.

---

## Task 6: Frontend Log Viewer Improvements

**File**: `web/src/pages/Logs.tsx`

**Changes**:
1. **Load history on mount**: Call `listLogs()` with increased limit (200). Now that backend returns real data, this will show recent history.
2. **Append WebSocket logs without clearing**: Keep existing `appendLogEvent` logic but DON'T clear logs when component remounts. Use `setLogs(prev => [...prev, ...newLogs])` for initial load.
3. **Auto-scroll to bottom**: Already implemented via `useEffect` on `filteredLogs.length`. Ensure it scrolls to bottom after initial history load.
4. **Scroll-to-bottom FAB**: When user scrolls up (not at bottom), show a floating "scroll to bottom" button. Hide when auto-scrolled.
5. **Connection status indicator**: Show WebSocket connection status (green dot = connected, red = disconnected).
6. **Log entry count improvements**: Show "X new" badge when paused and new logs arrive.

---

## Task 7: Frontend Provider Config UI Improvements

**File**: `web/src/pages/Providers.tsx`

**Changes**:
1. **Type-aware JSON template**: When user selects provider type, update the JSON params template:
   - `webdav` → `{"endpoint": "", "username": "", "password": "", "prefix": "", "timeout": "", "auth_mode": "basic"}`
   - `local` → `{"root_path": ""}`
   - When editing, preserve existing values.
2. **Connection test button**: Add "Test Connection" button in the provider form:
   - Calls `POST /api/v1/providers/test` with current form values.
   - Shows green checkmark + "Connection successful" or red X + error message.
   - Disable button while testing (show spinner).
3. **Enhanced delete confirmation**: When deleting a provider:
   - First call `DELETE /api/v1/providers/{id}` normally.
   - If response is 409 (has dependent pairs), show a modal listing the dependent pairs.
   - Add "Delete all (including pairs)" and "Cancel" buttons.
   - If user confirms, re-call `DELETE /api/v1/providers/{id}?force=true`.
4. **Form validation**: Validate required params per provider type before submit.

**New API client function**:
```typescript
// In client.ts
export async function testProvider(data: {
  type: string;
  params: Record<string, string>;
}): Promise<{ status: string; error?: string }> {
  return fetchJSON('/providers/test', { method: 'POST', body: JSON.stringify(data) });
}
```

---

## Task 8: Documentation Updates

### Update `config.example.yaml`:
- Add `local` provider example with `root_path` param.
- Update `webdav` params to include `prefix`, `timeout`, `auth_mode`.
- Add comments explaining each param.

### Update `README.md`:
- Update provider configuration section with both `local` and `webdav` types.
- Document all available params for each provider type.
- Add `auth_mode` explanation (auto vs basic).

---

## Task Dependencies

```
Task 1 (Backend) ─┬─→ Task 2 (Frontend shared state)
                   │        ├─→ Task 3 (Dashboard)
                   │        ├─→ Task 4 (SyncPairs)
                   │        └─→ Task 5 (FileBrowser)
                   ├─→ Task 6 (Log viewer)
                   ├─→ Task 7 (Provider UI)
                   └─→ Task 8 (Docs)
```

Tasks 3-8 can execute after their respective dependencies are met. Task 2 must come before 3-5. Task 1 must come first.

---

## Files Changed (Summary)

### Backend (Go)
- `internal/server/handler/ws.go` — DeletePair, DeleteProvider, ListLogs, TestProvider handlers
- `internal/engine/engine.go` — Event struct extension, progress data in broadcasts
- `internal/store/store.go` — ListSyncPairsByProvider query
- `internal/server/handler/routes.go` — New test provider route

### Frontend (React/TypeScript)
- `web/src/hooks/useSyncProgress.ts` — NEW: shared sync progress hook + context
- `web/src/App.tsx` — Wrap with SyncProgressProvider
- `web/src/pages/Dashboard.tsx` — Real-time progress display
- `web/src/pages/SyncPairs.tsx` — Progress indicators, enhanced delete
- `web/src/pages/FileBrowser.tsx` — Sync status indicators
- `web/src/pages/Logs.tsx` — History loading, auto-scroll improvements
- `web/src/pages/Providers.tsx` — Type-aware forms, connection test, cascade delete
- `web/src/api/client.ts` — New testProvider function, updated types
- `web/src/i18n/index.tsx` — New translation keys for progress, test, cascade

### Docs
- `config.example.yaml` — Updated provider examples
- `README.md` — Updated provider docs
