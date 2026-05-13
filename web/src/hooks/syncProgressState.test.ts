import { describe, expect, it } from 'vitest';
import type { WSEvent } from '../api/client';
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
    } as WSEvent);

    expect(entry.activeFile?.path).toBe('/docs/report.pdf');
    expect(entry.activeFile?.bytesTransferred).toBe(512);
    expect(entry.activeFile?.percent).toBe(50);
  });
});
