import React, { createContext, useContext, useRef, useState, useEffect, useCallback } from 'react';
import { useWebSocket } from './useWebSocket';
import type { WSEvent, WSEngineEvent } from '../api/client';
import { applyProgressEvent, createEmptyProgress, mergeProgressSnapshot } from './syncProgressState';
import type { ProgressMap, SyncProgress } from './syncProgressState';
import { getProgressSnapshots } from '../api/client';

export type { RecentProgressItem, SyncProgress } from './syncProgressState';

interface SyncProgressContextValue {
  getProgress: (pairId: string) => SyncProgress | undefined;
  allProgress: ProgressMap;
  isSyncing: (pairId: string) => boolean;
  activeSyncCount: number;
}

const SyncProgressContext = createContext<SyncProgressContextValue | null>(null);

function isEngineEvent(event: WSEvent): event is WSEngineEvent & Record<string, unknown> {
  return !('level' in event && event.type === 'log');
}

function getPairId(event: WSEvent): string | undefined {
  if ('pair_id' in event && event.pair_id != null) {
    return String(event.pair_id);
  }
  return undefined;
}

export function SyncProgressProvider({ children }: { children: React.ReactNode }): React.ReactElement {
  const progressRef = useRef<ProgressMap>(new Map());
  const [snapshot, setSnapshot] = useState<ProgressMap>(new Map());
  const dirtyRef = useRef(false);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Hydrate from backend progress snapshot on mount
  useEffect(() => {
    let cancelled = false;
    getProgressSnapshots()
      .then((snapshots) => {
        if (cancelled) return;
        const map = progressRef.current;
        snapshots.forEach((s) => mergeProgressSnapshot(map, s));
        dirtyRef.current = true;
        setSnapshot(new Map(map));
      })
      .catch(() => {
        // Progress is best-effort; WebSocket events can still populate state.
      });
    return () => { cancelled = true; };
  }, []);

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

    const map = progressRef.current;

    // Handle status_change events
    if (event.type === 'status_change') {
      const statusEvent = event as { pair_id: string; new_status: string };
      const entry = mergeProgressSnapshot(map, {
        pair_id: pairId,
        status: statusEvent.new_status === 'syncing' ? 'syncing' : statusEvent.new_status === 'synced' ? 'completed' : 'idle',
        direction: '',
        files_synced: 0,
        files_total: 0,
        pending_tasks: 0,
        updated_at: new Date().toISOString(),
      });
      entry.lastUpdated = new Date().toISOString();
      dirtyRef.current = true;
      return;
    }

    // Handle progress events (legacy format)
    if (event.type === 'progress') {
      const prog = event as { pair_id: string; current: number; total: number; file_path: string; bytes_transferred: number };
      const entry = mergeProgressSnapshot(map, {
        pair_id: pairId,
        status: 'syncing',
        direction: '',
        files_synced: prog.current ?? 0,
        files_total: prog.total ?? 0,
        pending_tasks: 0,
        updated_at: new Date().toISOString(),
      });
      entry.bytesTransferred = prog.bytes_transferred ?? entry.bytesTransferred;
      entry.lastUpdated = new Date().toISOString();
      dirtyRef.current = true;
      return;
    }

    // Handle engine events by type
    if (!isEngineEvent(event)) return;

    const engine = event as WSEngineEvent & Record<string, unknown>;
    const eventType = engine.type;

    switch (eventType) {
      case 'sync_started': {
        const entry = mergeProgressSnapshot(map, {
          pair_id: pairId,
          status: 'syncing',
          direction: (engine.direction ?? '') as 'up' | 'down' | 'both' | '',
          files_synced: 0,
          files_total: 0,
          pending_tasks: 0,
          started_at: engine.time ?? new Date().toISOString(),
          updated_at: new Date().toISOString(),
        });
        entry.currentFile = '';
        entry.bytesTransferred = 0;
        entry.bytesTotal = 0;
        dirtyRef.current = true;
        break;
      }

      case 'sync_completed': {
        const entry = mergeProgressSnapshot(map, {
          pair_id: pairId,
          status: 'completed',
          direction: '',
          files_synced: typeof engine.files_synced === 'number' ? engine.files_synced : 0,
          files_total: typeof engine.files_total === 'number' ? engine.files_total : 0,
          pending_tasks: 0,
          updated_at: new Date().toISOString(),
        });
        entry.activeFile = undefined;
        dirtyRef.current = true;
        break;
      }

      case 'sync_failed': {
        const entry = mergeProgressSnapshot(map, {
          pair_id: pairId,
          status: 'failed',
          direction: '',
          files_synced: 0,
          files_total: 0,
          pending_tasks: 0,
          updated_at: new Date().toISOString(),
          error: engine.error ?? '',
        });
        entry.activeFile = undefined;
        dirtyRef.current = true;
        break;
      }

      case 'task_completed': {
        // Use applyProgressEvent for recent items tracking
        applyProgressEvent(map, event);
        // Also update filesSynced/filesTotal from engine event fields
        const entry = map.get(pairId);
        if (entry) {
          if (typeof engine.files_synced === 'number') entry.filesSynced = engine.files_synced;
          if (typeof engine.files_total === 'number') entry.filesTotal = engine.files_total;
        }
        dirtyRef.current = true;
        break;
      }

      case 'task_failed': {
        applyProgressEvent(map, event);
        dirtyRef.current = true;
        break;
      }

      case 'task_queued': {
        const entry = mergeProgressSnapshot(map, {
          pair_id: pairId,
          status: 'syncing',
          direction: '',
          files_synced: 0,
          files_total: typeof engine.files_total === 'number' ? engine.files_total : 0,
          pending_tasks: typeof engine.pending === 'number' ? engine.pending : 0,
          updated_at: new Date().toISOString(),
        });
        dirtyRef.current = true;
        break;
      }

      case 'pair_unregistered': {
        map.delete(pairId);
        dirtyRef.current = true;
        break;
      }

      case 'task_started':
      case 'chunk_transferred': {
        applyProgressEvent(map, event);
        dirtyRef.current = true;
        break;
      }

      default: {
        const anyEvt = engine as Record<string, unknown>;
        if (typeof anyEvt.bytes_transferred === 'number' || typeof anyEvt.bytes_total === 'number') {
          applyProgressEvent(map, event);
          dirtyRef.current = true;
        }
        break;
      }
    }
  }, []);

  // Connect to WebSocket
  useWebSocket({ onEvent: handleEvent });

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
