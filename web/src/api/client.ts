// Every-Sync API Client
// ---------------------

const BASE = '/api/v1';

export async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
  if (!res.ok) {
    const body = await res.text().catch(() => res.statusText);
    throw new Error(`API ${res.status}: ${body}`);
  }
  return res.json();
}

// ---- Types ----

export type SyncMode = 'mirror' | 'backup' | 'selective';
export type SyncDirection = 'bidirectional' | 'upload' | 'download';
export type SyncStatus = 'synced' | 'syncing' | 'virtual' | 'conflict' | 'excluded' | 'pending' | 'error';
export type FileSide = 'local' | 'remote';
export type ResourceType = 'file' | 'folder';
export type CheckState = 'checked' | 'unchecked' | 'indeterminate';

export interface SyncPair {
  id: string;
  name: string;
  mode: SyncMode;
  direction: SyncDirection;
  local_path: string;
  remote_provider: string;
  remote_path: string;
  status: SyncStatus;
  last_sync?: string;
  enabled: boolean;
  stats?: PairStats;
}

export interface PairStats {
  total_files: number;
  synced_files: number;
  pending_files: number;
  conflict_files: number;
  virtual_files: number;
  upload_bytes: number;
  download_bytes: number;
}

export interface FileEntry {
  name: string;
  path: string;
  type: ResourceType;
  size: number;
  modified: string;
  status: SyncStatus;
  selected?: boolean;
  children_count?: number;
}

export interface ConflictEntry {
  id: string;
  pair_id: string;
  path: string;
  local_modified: string;
  remote_modified: string;
  local_size: number;
  remote_size: number;
  resolved: boolean;
}

export interface VersionEntry {
  id: string;
  path: string;
  version: number;
  modified: string;
  size: number;
  pair_id: string;
}

export interface LogEntry {
  id: string;
  timestamp: string;
  level: 'debug' | 'info' | 'warn' | 'error';
  message: string;
  pair_id?: string;
  fields?: Record<string, string>;
}

export interface Provider {
  id: string;
  name: string;
  type: string;
  configured: boolean;
  auth_url?: string;
}

export interface DashboardStats {
  engine_status: 'running' | 'paused' | 'stopped';
  active_pairs: number;
  total_pairs: number;
  pending_tasks: number;
  active_workers: number;
  upload_bytes: number;
  download_bytes: number;
  conflicts: number;
  virtual_files: number;
}

export interface WSProgressEvent {
  type: 'progress';
  pair_id: string;
  current: number;
  total: number;
  file_path: string;
  bytes_transferred: number;
}

export interface WSStatusEvent {
  type: 'status_change';
  pair_id: string;
  old_status: SyncStatus;
  new_status: SyncStatus;
}

export interface WSConflictEvent {
  type: 'conflict';
  pair_id: string;
  path: string;
  conflict_id: string;
}

export interface WSLogEvent {
  type: 'log';
  level: LogEntry['level'];
  message: string;
  pair_id?: string;
  timestamp: string;
}

export type WSEvent = WSProgressEvent | WSStatusEvent | WSConflictEvent | WSLogEvent;

// ---- API Functions ----

export async function getDashboardStats(): Promise<DashboardStats> {
  return fetchJSON<DashboardStats>('/dashboard');
}

export async function listPairs(): Promise<SyncPair[]> {
  return fetchJSON<SyncPair[]>('/pairs');
}

export async function getPair(id: string): Promise<SyncPair> {
  return fetchJSON<SyncPair>(`/pairs/${id}`);
}

export async function listFiles(pairId: string, path: string = '/', side: FileSide = 'local'): Promise<FileEntry[]> {
  const params = new URLSearchParams({ path, side });
  return fetchJSON<FileEntry[]>(`/pairs/${pairId}/files?${params}`);
}

export async function triggerSync(pairId: string): Promise<void> {
  await fetchJSON(`/pairs/${pairId}/sync`, { method: 'POST' });
}

export async function listConflicts(pairId?: string): Promise<ConflictEntry[]> {
  const query = pairId ? `?pair_id=${pairId}` : '';
  return fetchJSON<ConflictEntry[]>(`/conflicts${query}`);
}

export async function resolveConflict(conflictId: string, resolution: 'local' | 'remote'): Promise<void> {
  await fetchJSON(`/conflicts/${conflictId}/resolve`, {
    method: 'POST',
    body: JSON.stringify({ resolution }),
  });
}

export async function listVersions(pairId: string, path: string): Promise<VersionEntry[]> {
  const params = new URLSearchParams({ path });
  return fetchJSON<VersionEntry[]>(`/pairs/${pairId}/versions?${params}`);
}

export async function restoreVersion(pairId: string, versionId: string): Promise<void> {
  await fetchJSON(`/pairs/${pairId}/versions/${versionId}/restore`, { method: 'POST' });
}

export async function listLogs(pairId?: string, level?: string, limit: number = 100): Promise<LogEntry[]> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (pairId) params.set('pair_id', pairId);
  if (level) params.set('level', level);
  return fetchJSON<LogEntry[]>(`/logs?${params}`);
}

export async function listProviders(): Promise<Provider[]> {
  return fetchJSON<Provider[]>('/providers');
}

export async function materializeFile(pairId: string, path: string): Promise<void> {
  await fetchJSON(`/pairs/${pairId}/materialize`, {
    method: 'POST',
    body: JSON.stringify({ path }),
  });
}

export async function excludePath(pairId: string, path: string): Promise<void> {
  await fetchJSON(`/pairs/${pairId}/exclude`, {
    method: 'POST',
    body: JSON.stringify({ path }),
  });
}
