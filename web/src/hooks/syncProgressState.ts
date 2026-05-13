import type { PairProgressSnapshot, WSEvent } from '../api/client';

export interface RecentProgressItem {
  path: string;
  taskType: string;
  status: 'completed' | 'failed';
  direction: string;
  bytesTotal?: number;
  error?: string;
  finishedAt: string;
}

export interface FileQueueItem {
  path: string;
  taskType: string;
  status: 'pending' | 'syncing' | 'completed' | 'failed';
  direction: string;
  bytesTransferred: number;
  bytesTotal: number;
  percent: number;
  queuedAt?: string;
  startedAt?: string;
  updatedAt?: string;
  finishedAt?: string;
  error?: string;
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
  queueItems: FileQueueItem[];
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
    queueItems: [],
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
  if (snapshot.queue) {
    entry.queueItems = snapshot.queue.map((item) => ({
      path: item.path,
      taskType: item.task_type,
      status: item.status,
      direction: item.direction ?? directionForTask(item.task_type),
      bytesTransferred: item.bytes_transferred ?? 0,
      bytesTotal: item.bytes_total ?? 0,
      percent: item.percent ?? 0,
      queuedAt: item.queued_at,
      startedAt: item.started_at,
      updatedAt: item.updated_at,
      finishedAt: item.finished_at,
      error: item.error,
    }));
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

  if (event.type === 'task_queued') {
    const path = String(anyEvent.path ?? '');
    const taskType = String(anyEvent.task_type ?? '');
    const item = upsertQueueItem(entry, path, taskType);
    item.status = 'pending';
    item.direction = String(anyEvent.direction ?? directionForTask(taskType));
    item.updatedAt = now;
    item.queuedAt = item.queuedAt ?? now;
    entry.status = 'syncing';
    return true;
  }

  if (event.type === 'task_started') {
    const path = String(anyEvent.path ?? '');
    const taskType = String(anyEvent.task_type ?? '');
    const bytesTransferred = Number(anyEvent.bytes_transferred ?? 0);
    const bytesTotal = Number(anyEvent.bytes_total ?? 0);
    const item = upsertQueueItem(entry, path, taskType);
    item.status = 'syncing';
    item.direction = String(anyEvent.direction ?? directionForTask(taskType));
    item.bytesTransferred = bytesTransferred;
    item.bytesTotal = bytesTotal;
    item.percent = bytesTotal > 0 ? (bytesTransferred / bytesTotal) * 100 : 0;
    item.startedAt = item.startedAt ?? now;
    item.updatedAt = now;
    entry.status = 'syncing';
    entry.currentFile = path;
    entry.activeFile = {
      path,
      taskType,
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
    const taskType = String(anyEvent.task_type ?? entry.activeFile?.taskType ?? '');
    const bytesTransferred = Number(anyEvent.bytes_transferred ?? 0);
    const bytesTotal = Number(anyEvent.bytes_total ?? 0);
    const item = upsertQueueItem(entry, path, taskType);
    item.status = 'syncing';
    item.direction = String(anyEvent.direction ?? directionForTask(taskType));
    item.bytesTransferred = bytesTransferred;
    item.bytesTotal = bytesTotal;
    item.percent = bytesTotal > 0 ? (bytesTransferred / bytesTotal) * 100 : 0;
    item.updatedAt = now;
    entry.currentFile = path;
    entry.activeFile = {
      path,
      taskType,
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
    const path = String(anyEvent.path ?? entry.currentFile);
    const taskType = String(anyEvent.task_type ?? '');
    const item = upsertQueueItem(entry, path, taskType);
    item.status = event.type === 'task_failed' ? 'failed' : 'completed';
    item.direction = String(anyEvent.direction ?? item.direction ?? directionForTask(taskType));
    item.error = anyEvent.error ? String(anyEvent.error) : undefined;
    item.finishedAt = now;
    item.updatedAt = now;
    if (item.status === 'completed') {
      item.percent = 100;
      if (item.bytesTotal > 0) item.bytesTransferred = item.bytesTotal;
    }
    pushRecent(entry, {
      path,
      taskType,
      status: event.type === 'task_failed' ? 'failed' : 'completed',
      direction: item.direction,
      bytesTotal: item.bytesTotal,
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

export function findQueueItem(entry: SyncProgress | undefined, path: string): FileQueueItem | undefined {
  return entry?.queueItems.find((item) => item.path === path);
}

function upsertQueueItem(entry: SyncProgress, path: string, taskType: string): FileQueueItem {
  let item = entry.queueItems.find((candidate) => candidate.path === path && (!taskType || candidate.taskType === taskType || !candidate.taskType));
  if (!item) {
    item = {
      path,
      taskType,
      status: 'pending',
      direction: directionForTask(taskType),
      bytesTransferred: 0,
      bytesTotal: 0,
      percent: 0,
      queuedAt: new Date().toISOString(),
    };
    entry.queueItems = [...entry.queueItems, item];
  } else if (!item.taskType) {
    item.taskType = taskType;
  }
  return item;
}

function directionForTask(taskType: string): string {
  switch (taskType) {
    case 'upload':
      return 'up';
    case 'download':
    case 'virtual':
      return 'down';
    default:
      return '';
  }
}
