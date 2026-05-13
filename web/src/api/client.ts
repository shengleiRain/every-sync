// Every-Sync API Client
// ---------------------

const BASE = '/api/v1';

export async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });
  const body = await res.text();
  if (!res.ok) {
    throw new Error(`API ${res.status}: ${body || res.statusText}`);
  }
  return (body ? JSON.parse(body) : undefined) as T;
}

// ---- Types ----

export type SyncMode = 'normal' | 'virtual';
export type SyncDirection = 'both' | 'up' | 'down';
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
  provider: string;
  remote_path: string;
  status: SyncStatus;
  last_sync?: string;
  enabled: boolean;
  include_patterns?: string;
  exclude_patterns?: string;
  conflict_strategy?: string;
  selected_folders?: string;
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
  local_modified?: string;
  remote_modified?: string;
  local_size: number;
  remote_size: number;
  status: string;
  resolved: boolean;
}

export interface VersionEntry {
  id: string;
  path: string;
  source: string;
  modified?: string;
  recorded: string;
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
  params?: Record<string, string>;
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

export interface WSEngineEvent {
  type: string;
  time?: string;
  pair_id?: number | string;
  pair_name?: string;
  task_type?: string;
  path?: string;
  pending?: number;
  error?: string;
  message?: string;
  direction?: string;
}

export type WSEvent = WSProgressEvent | WSStatusEvent | WSConflictEvent | WSLogEvent | WSEngineEvent;

interface APIFileListResponse {
  entries?: APIFileEntry[];
}

interface APIFileEntry {
  name: string;
  path: string;
  size?: number;
  mod_time?: string;
  modified?: string;
  is_dir?: boolean;
  type?: ResourceType;
  sync_state?: string;
  status?: string;
  selected?: boolean;
  children_count?: number;
}

interface APIEngineStatus {
  running: boolean;
  registered_pairs?: number;
  pending?: number;
  max_workers?: number;
  stats?: {
    uploaded_bytes?: number;
    downloaded_bytes?: number;
    virtual_files?: number;
    conflicts?: number;
  };
  pairs?: Array<{ enabled?: boolean }>;
}

function normalizeId(value: unknown): string {
  return String(value ?? '');
}

function numericId(value: string): number {
  const n = Number(value);
  if (!Number.isFinite(n)) {
    throw new Error(`Invalid numeric id: ${value}`);
  }
  return n;
}

function normalizeStatus(value: unknown, enabled?: boolean): SyncStatus {
  const status = String(value ?? '');
  if (['synced', 'syncing', 'virtual', 'conflict', 'excluded', 'pending', 'error'].includes(status)) {
    return status as SyncStatus;
  }
  return enabled === false ? 'excluded' : 'pending';
}

function normalizeMode(value: unknown): SyncMode {
  const mode = String(value ?? '').toLowerCase();
  return mode === 'virtual' ? 'virtual' : 'normal';
}

function normalizePair(raw: Record<string, unknown>): SyncPair {
  const enabled = Boolean(raw.enabled);
  return {
    id: normalizeId(raw.id),
    name: String(raw.name ?? ''),
    mode: normalizeMode(raw.mode),
    direction: String(raw.direction ?? 'both') as SyncDirection,
    local_path: String(raw.local_path ?? ''),
    provider: String(raw.provider ?? raw.remote_provider ?? ''),
    remote_path: String(raw.remote_path ?? ''),
    status: normalizeStatus(raw.status, enabled),
    last_sync: raw.last_sync ? String(raw.last_sync) : undefined,
    enabled,
    include_patterns: raw.include_patterns ? String(raw.include_patterns) : '',
    exclude_patterns: raw.exclude_patterns ? String(raw.exclude_patterns) : '',
    conflict_strategy: raw.conflict_strategy ? String(raw.conflict_strategy) : '',
    selected_folders: raw.selected_folders ? String(raw.selected_folders) : '[]',
    stats: raw.stats as PairStats | undefined,
  };
}

function normalizeFileEntry(raw: APIFileEntry): FileEntry {
  const isDir = raw.is_dir ?? raw.type === 'folder';
  return {
    name: raw.name,
    path: raw.path,
    type: isDir ? 'folder' : 'file',
    size: raw.size ?? 0,
    modified: raw.mod_time ?? raw.modified ?? '',
    status: normalizeStatus(raw.sync_state ?? raw.status),
    selected: raw.selected,
    children_count: raw.children_count,
  };
}

function normalizeProvider(raw: Record<string, unknown>): Provider {
  const params = raw.params && typeof raw.params === 'object' ? raw.params as Record<string, string> : undefined;
  return {
    id: normalizeId(raw.id),
    name: String(raw.name ?? ''),
    type: String(raw.type ?? ''),
    configured: raw.configured === undefined ? Boolean(raw.name && raw.type) : Boolean(raw.configured),
    params,
    auth_url: raw.auth_url ? String(raw.auth_url) : undefined,
  };
}

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

// ---- API Functions ----

export async function getDashboardStats(): Promise<DashboardStats> {
  const status = await fetchJSON<APIEngineStatus>('/status');
  const pairs = status.pairs ?? [];
  return {
    engine_status: status.running ? 'running' : 'stopped',
    active_pairs: pairs.filter((p) => p.enabled).length,
    total_pairs: status.registered_pairs ?? pairs.length,
    pending_tasks: status.pending ?? 0,
    active_workers: status.max_workers ?? 0,
    upload_bytes: status.stats?.uploaded_bytes ?? 0,
    download_bytes: status.stats?.downloaded_bytes ?? 0,
    conflicts: status.stats?.conflicts ?? 0,
    virtual_files: status.stats?.virtual_files ?? 0,
  };
}

export async function listPairs(): Promise<SyncPair[]> {
  const pairs = await fetchJSON<Array<Record<string, unknown>>>('/pairs');
  return pairs.map(normalizePair);
}

export async function getPair(id: string): Promise<SyncPair> {
  return normalizePair(await fetchJSON<Record<string, unknown>>(`/pairs/${id}`));
}

export async function listFiles(pairId: string, path: string = '/', side: FileSide = 'local'): Promise<FileEntry[]> {
  const params = new URLSearchParams({ path, side });
  const response = await fetchJSON<APIFileListResponse | APIFileEntry[]>(`/pairs/${pairId}/files?${params}`);
  const entries = Array.isArray(response) ? response : response.entries ?? [];
  return entries.map(normalizeFileEntry);
}

export async function triggerSync(pairId: string): Promise<void> {
  await fetchJSON('/sync', { method: 'POST', body: JSON.stringify({ pair_id: numericId(pairId) }) });
}

export async function syncAll(): Promise<void> {
  await fetchJSON('/sync', { method: 'POST', body: JSON.stringify({}) });
}

export async function createPair(data: {
  name: string;
  local_path: string;
  remote_path: string;
  provider?: string;
  mode?: string;
  direction?: string;
  conflict_strategy?: string;
  include_patterns?: string;
  exclude_patterns?: string;
}): Promise<SyncPair> {
  return normalizePair(await fetchJSON<Record<string, unknown>>('/pairs', { method: 'POST', body: JSON.stringify(data) }));
}

export async function updatePair(id: string, data: Record<string, unknown>): Promise<SyncPair> {
  return normalizePair(await fetchJSON<Record<string, unknown>>(`/pairs/${id}`, { method: 'PUT', body: JSON.stringify(data) }));
}

export async function deletePair(id: string): Promise<void> {
  await fetchJSON(`/pairs/${id}`, { method: 'DELETE' });
}

export async function listConflicts(pairId?: string): Promise<ConflictEntry[]> {
  const params = new URLSearchParams({ status: 'open' });
  if (pairId) params.set('pair_id', pairId);
  const conflicts = await fetchJSON<Array<Record<string, unknown>>>(`/conflicts?${params}`);
  return conflicts.map((raw) => ({
    id: normalizeId(raw.id),
    pair_id: normalizeId(raw.sync_pair_id ?? raw.pair_id),
    path: String(raw.path ?? ''),
    local_modified: raw.local_mtime ? String(raw.local_mtime) : raw.local_modified ? String(raw.local_modified) : undefined,
    remote_modified: raw.remote_mtime ? String(raw.remote_mtime) : raw.remote_modified ? String(raw.remote_modified) : undefined,
    local_size: Number(raw.local_size ?? 0),
    remote_size: Number(raw.remote_size ?? 0),
    status: String(raw.status ?? ''),
    resolved: String(raw.status ?? '') === 'resolved' || Boolean(raw.resolved),
  }));
}

export async function resolveConflict(conflictId: string, resolution: string): Promise<void> {
  await fetchJSON(`/conflicts/${conflictId}/resolve`, {
    method: 'POST',
    body: JSON.stringify({ strategy: resolution }),
  });
}

export async function listVersions(pairId: string, path: string): Promise<VersionEntry[]> {
  const params = new URLSearchParams({ pair_id: pairId });
  if (path) params.set('path', path);
  const versions = await fetchJSON<Array<Record<string, unknown>>>(`/versions?${params}`);
  return versions.map((raw) => ({
    id: normalizeId(raw.id),
    pair_id: normalizeId(raw.sync_pair_id ?? raw.pair_id),
    path: String(raw.path ?? ''),
    source: String(raw.source ?? ''),
    size: Number(raw.size ?? 0),
    modified: raw.mod_time ? String(raw.mod_time) : raw.modified ? String(raw.modified) : undefined,
    recorded: String(raw.created_at ?? raw.recorded ?? ''),
  }));
}

export async function listLogs(pairId?: string, level?: string, limit: number = 100): Promise<LogEntry[]> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (pairId) params.set('pair_id', pairId);
  if (level) params.set('level', level);
  try {
    return await fetchJSON<LogEntry[]>(`/logs?${params}`);
  } catch (e) {
    if (e instanceof Error && e.message.startsWith('API 404:')) {
      return [];
    }
    throw e;
  }
}

export async function listProviders(): Promise<Provider[]> {
  const providers = await fetchJSON<Array<Record<string, unknown>>>('/providers');
  return providers.map(normalizeProvider);
}

export async function createProvider(data: {
  name: string;
  type: string;
  params?: Record<string, string>;
}): Promise<Provider> {
  return normalizeProvider(await fetchJSON<Record<string, unknown>>('/providers', { method: 'POST', body: JSON.stringify(data) }));
}

export async function updateProvider(id: string, data: Record<string, unknown>): Promise<Provider> {
  return normalizeProvider(await fetchJSON<Record<string, unknown>>(`/providers/${id}`, { method: 'PUT', body: JSON.stringify(data) }));
}

export async function deleteProvider(id: string): Promise<void> {
  await fetchJSON(`/providers/${id}`, { method: 'DELETE' });
}

export async function deleteProviderForce(id: string): Promise<void> {
  await fetchJSON(`/providers/${id}?force=true`, { method: 'DELETE' });
}

export async function testProvider(data: {
  id?: number;
  type?: string;
  params?: Record<string, string>;
}): Promise<{ status: string; error?: string }> {
  return fetchJSON('/providers/test', { method: 'POST', body: JSON.stringify(data) });
}

export async function materializeFile(pairId: string, path: string): Promise<void> {
  await fetchJSON(`/pairs/${pairId}/materialize`, {
    method: 'POST',
    body: JSON.stringify({ path }),
  });
}

export async function selectFolders(pairId: string, selectedFolders: string[]): Promise<SyncPair> {
  return normalizePair(await fetchJSON<Record<string, unknown>>(`/pairs/${pairId}/folders/select`, {
    method: 'POST',
    body: JSON.stringify({ selected_folders: selectedFolders }),
  }));
}
