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
  const anyEvent = event as unknown as Record<string, unknown>;
  const now = new Date().toISOString();
  entry.lastUpdated = now;

  if (event.type === 'task_started') {
    const path = String(anyEvent.path ?? '');
    const bytesTransferred = Number(anyEvent.bytes_transferred ?? 0);
    const bytesTotal = Number(anyEvent.bytes_total ?? 0);
    entry.status = 'syncing';
    entry.currentFile = path;
    entry.activeFile = {
      path,
      taskType: String(anyEvent.task_type ?? ''),
      bytesTransferred,
      bytesTotal,
      percent: bytesTotal > 0 ? (bytesTransferred / bytesTotal) * 100 : 0,
      startedAt: now,
      updatedAt: now,
    };
    entry.bytesTransferred = bytesTransferred;
    entry.bytesTotal = bytesTotal;
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
