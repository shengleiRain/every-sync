import React, { createContext, useContext, useRef, useState, useEffect, useCallback } from 'react';
import { useWebSocket } from './useWebSocket';
import type { WSEvent, WSEngineEvent } from '../api/client';

// ---- Types ----

export interface SyncProgress {
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

type ProgressMap = Map<string, SyncProgress>;

interface SyncProgressContextValue {
  getProgress: (pairId: string) => SyncProgress | undefined;
  allProgress: ProgressMap;
  isSyncing: (pairId: string) => boolean;
  activeSyncCount: number;
}

const SyncProgressContext = createContext<SyncProgressContextValue | null>(null);

// ---- Helpers ----

function ensureEntry(map: ProgressMap, pairId: string): SyncProgress {
  let entry = map.get(pairId);
  if (!entry) {
    entry = {
      pairId,
      status: 'idle',
      currentFile: '',
      filesSynced: 0,
      filesTotal: 0,
      bytesTransferred: 0,
      bytesTotal: 0,
      direction: '',
      startedAt: '',
      lastUpdated: '',
    };
    map.set(pairId, entry);
  }
  return entry;
}

function isEngineEvent(event: WSEvent): event is WSEngineEvent & Record<string, unknown> {
  return !('level' in event && event.type === 'log');
}

function getPairId(event: WSEvent): string | undefined {
  if ('pair_id' in event && event.pair_id != null) {
    return String(event.pair_id);
  }
  return undefined;
}

// ---- Provider ----

export function SyncProgressProvider({ children }: { children: React.ReactNode }): React.ReactElement {
  const progressRef = useRef<ProgressMap>(new Map());
  const [snapshot, setSnapshot] = useState<ProgressMap>(new Map());
  const dirtyRef = useRef(false);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Flush the ref-based progress map into state at most every 500ms
  useEffect(() => {
    timerRef.current = setInterval(() => {
      if (dirtyRef.current) {
        dirtyRef.current = false;
        setSnapshot(new Map(progressRef.current));
      }
    }, 500);

    return () => {
      if (timerRef.current) {
        clearInterval(timerRef.current);
      }
    };
  }, []);

  const handleEvent = useCallback((event: WSEvent) => {
    const pairId = getPairId(event);
    if (!pairId) return;

    // Handle progress events separately (they have a distinct shape)
    if (event.type === 'progress') {
      const prog = event as { pair_id: string; current: number; total: number; file_path: string; bytes_transferred: number };
      const map = progressRef.current;
      const entry = ensureEntry(map, pairId);
      entry.currentFile = prog.file_path ?? '';
      entry.filesSynced = prog.current ?? entry.filesSynced;
      entry.filesTotal = prog.total ?? entry.filesTotal;
      entry.bytesTransferred = prog.bytes_transferred ?? entry.bytesTransferred;
      entry.lastUpdated = new Date().toISOString();
      dirtyRef.current = true;
      return;
    }

    // Handle status_change events
    if (event.type === 'status_change') {
      const statusEvent = event as { pair_id: string; new_status: string };
      const map = progressRef.current;
      const entry = ensureEntry(map, pairId);
      if (statusEvent.new_status === 'syncing') {
        entry.status = 'syncing';
      } else if (statusEvent.new_status === 'synced') {
        entry.status = 'completed';
      }
      entry.lastUpdated = new Date().toISOString();
      dirtyRef.current = true;
      return;
    }

    // Handle engine events by type
    if (!isEngineEvent(event)) return;

    const map = progressRef.current;
    const engine = event as WSEngineEvent & Record<string, unknown>;
    const eventType = engine.type;

    switch (eventType) {
      case 'sync_started': {
        const entry = ensureEntry(map, pairId);
        entry.status = 'syncing';
        entry.direction = engine.direction ?? '';
        entry.startedAt = engine.time ?? new Date().toISOString();
        entry.filesSynced = 0;
        entry.filesTotal = 0;
        entry.bytesTransferred = 0;
        entry.bytesTotal = 0;
        entry.currentFile = '';
        entry.lastUpdated = new Date().toISOString();
        dirtyRef.current = true;
        break;
      }

      case 'sync_completed': {
        const entry = ensureEntry(map, pairId);
        entry.status = 'completed';
        if (typeof engine.files_synced === 'number') entry.filesSynced = engine.files_synced;
        if (typeof engine.files_total === 'number') entry.filesTotal = engine.files_total;
        entry.lastUpdated = new Date().toISOString();
        dirtyRef.current = true;
        break;
      }

      case 'sync_failed': {
        const entry = ensureEntry(map, pairId);
        entry.status = 'failed';
        entry.lastUpdated = new Date().toISOString();
        dirtyRef.current = true;
        break;
      }

      case 'task_completed': {
        const entry = ensureEntry(map, pairId);
        entry.filesSynced = (entry.filesSynced ?? 0) + 1;
        if (engine.path) entry.currentFile = engine.path;
        if (typeof engine.files_total === 'number') entry.filesTotal = engine.files_total;
        if (typeof engine.pending === 'number') {
          // pending is remaining tasks; total = synced + pending
          entry.filesTotal = Math.max(entry.filesTotal, entry.filesSynced + engine.pending);
        }
        entry.lastUpdated = new Date().toISOString();
        dirtyRef.current = true;
        break;
      }

      case 'task_queued': {
        const entry = ensureEntry(map, pairId);
        if (typeof engine.files_total === 'number') entry.filesTotal = engine.files_total;
        if (typeof engine.pending === 'number' && engine.pending > entry.filesTotal) {
          entry.filesTotal = engine.pending;
        }
        entry.lastUpdated = new Date().toISOString();
        dirtyRef.current = true;
        break;
      }

      case 'task_failed': {
        const entry = ensureEntry(map, pairId);
        if (engine.path) entry.currentFile = engine.path;
        entry.lastUpdated = new Date().toISOString();
        dirtyRef.current = true;
        break;
      }

      case 'pair_unregistered': {
        map.delete(pairId);
        dirtyRef.current = true;
        break;
      }

      case 'chunk_transferred': {
        const entry = ensureEntry(map, pairId);
        if (typeof engine.bytes_transferred === 'number') entry.bytesTransferred = engine.bytes_transferred;
        if (typeof engine.bytes_total === 'number') entry.bytesTotal = engine.bytes_total;
        entry.lastUpdated = new Date().toISOString();
        dirtyRef.current = true;
        break;
      }

      default: {
        // Catch-all for events that carry bytes_transferred / bytes_total
        const anyEvt = engine as Record<string, unknown>;
        if (typeof anyEvt.bytes_transferred === 'number' || typeof anyEvt.bytes_total === 'number') {
          const entry = ensureEntry(map, pairId);
          if (typeof anyEvt.bytes_transferred === 'number') entry.bytesTransferred = anyEvt.bytes_transferred as number;
          if (typeof anyEvt.bytes_total === 'number') entry.bytesTotal = anyEvt.bytes_total as number;
          entry.lastUpdated = new Date().toISOString();
          dirtyRef.current = true;
        }
        break;
      }
    }
  }, []);

  // Connect to WebSocket
  useWebSocket({ onEvent: handleEvent });

  // Build context value from the latest snapshot
  const getProgress = useCallback(
    (pairId: string): SyncProgress | undefined => snapshot.get(pairId),
    [snapshot],
  );

  const isSyncing = useCallback(
    (pairId: string): boolean => {
      const p = snapshot.get(pairId);
      return p?.status === 'syncing' || p?.status === 'scanning';
    },
    [snapshot],
  );

  const activeSyncCount = React.useMemo(() => {
    let count = 0;
    for (const p of snapshot.values()) {
      if (p.status === 'syncing' || p.status === 'scanning') count++;
    }
    return count;
  }, [snapshot]);

  const contextValue = React.useMemo<SyncProgressContextValue>(
    () => ({
      getProgress,
      allProgress: snapshot,
      isSyncing,
      activeSyncCount,
    }),
    [getProgress, snapshot, isSyncing, activeSyncCount],
  );

  return (
    <SyncProgressContext.Provider value={contextValue}>
      {children}
    </SyncProgressContext.Provider>
  );
}

// ---- Hook ----

export function useSyncProgress(): {
  getProgress: (pairId: string) => SyncProgress | undefined;
  allProgress: ProgressMap;
  isSyncing: (pairId: string) => boolean;
  activeSyncCount: number;
} {
  const ctx = useContext(SyncProgressContext);
  if (!ctx) {
    throw new Error('useSyncProgress must be used within a SyncProgressProvider');
  }
  return ctx;
}
