import React from 'react';
import type { SyncProgress } from '../hooks/useSyncProgress';
import type { FileQueueItem } from '../hooks/syncProgressState';
import { Link } from 'react-router-dom';
import { CheckIcon, ClockIcon, DownloadIcon, SyncIcon, UploadIcon, WarningIcon } from './Icons';
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

export const CircularFileProgress: React.FC<{ item?: FileQueueItem; fallbackStatus?: string; size?: number }> = ({ item, fallbackStatus, size = 28 }) => {
  if (!item) {
    const complete = fallbackStatus === 'synced';
    const error = fallbackStatus === 'error' || fallbackStatus === 'conflict';
    const pending = fallbackStatus === 'pending';
    const color = error ? 'var(--accent-red)' : pending ? 'var(--accent-amber)' : complete ? 'var(--accent-green)' : 'var(--text-tertiary)';
    return (
      <span style={{ width: size, height: size, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', borderRadius: '50%', background: 'var(--bg-surface-hover)' }}>
        {complete ? <CheckIcon size={size - 12} color={color} /> : error ? <WarningIcon size={size - 12} color={color} /> : <ClockIcon size={size - 12} color={color} />}
      </span>
    );
  }

  if (item.status === 'syncing') {
    const radius = (size - 4) / 2;
    const circumference = 2 * Math.PI * radius;
    const offset = circumference * (1 - Math.min(100, Math.max(0, item.percent)) / 100);
    return (
      <span style={{ position: 'relative', width: size, height: size, display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }} title={`${Math.round(item.percent)}%`}>
        <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
          <circle cx={size / 2} cy={size / 2} r={radius} stroke="var(--border-default)" strokeWidth="3" fill="none" />
          <circle
            cx={size / 2}
            cy={size / 2}
            r={radius}
            stroke="var(--accent-blue)"
            strokeWidth="3"
            fill="none"
            strokeLinecap="round"
            strokeDasharray={circumference}
            strokeDashoffset={offset}
            transform={`rotate(-90 ${size / 2} ${size / 2})`}
          />
        </svg>
        <span style={{ position: 'absolute', fontSize: '9px', fontWeight: 700, color: 'var(--accent-blue)' }}>{Math.round(item.percent)}</span>
      </span>
    );
  }

  const color = item.status === 'failed' ? 'var(--accent-red)' : item.status === 'completed' ? 'var(--accent-green)' : 'var(--accent-amber)';
  return (
    <span style={{ width: size, height: size, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', borderRadius: '50%', background: item.status === 'failed' ? 'var(--accent-red-bg)' : item.status === 'completed' ? 'var(--accent-green-bg)' : 'var(--accent-amber-bg)' }}>
      {item.status === 'failed' ? <WarningIcon size={size - 12} color={color} /> : item.status === 'completed' ? <CheckIcon size={size - 12} color={color} /> : <ClockIcon size={size - 12} color={color} />}
    </span>
  );
};

export const PairSyncQueuePanel: React.FC<{ progress?: SyncProgress; t: (key: string) => string; defaultOpen?: boolean }> = ({ progress, t, defaultOpen = false }) => {
  const [open, setOpen] = React.useState(defaultOpen);
  const items = progress?.queueItems ?? [];
  const visibleItems = items.slice(0, 8);
  return (
    <div style={{ borderTop: '1px solid var(--border-muted)', paddingTop: 'var(--space-3)', marginTop: 'var(--space-3)' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-3)' }}>
        <button className="btn btn-sm" type="button" onClick={(e) => { e.stopPropagation(); setOpen((value) => !value); }}>
          {open ? t('progress.collapseQueue') : t('progress.expandQueue')} ({visibleItems.length})
        </button>
        <Link to="/recent" onClick={(e) => e.stopPropagation()} style={{ fontSize: 'var(--text-xs)', color: 'var(--accent-blue)', textDecoration: 'none' }}>
          {t('progress.viewRecent')}
        </Link>
      </div>
      {open && (
        <div style={{ display: 'grid', gap: 'var(--space-2)', marginTop: 'var(--space-3)' }}>
          {visibleItems.length === 0 ? (
            <div style={{ color: 'var(--text-tertiary)', fontSize: 'var(--text-xs)' }}>{t('progress.noQueuedFiles')}</div>
          ) : visibleItems.map((item) => (
            <div key={`${item.taskType}-${item.path}`} style={{ display: 'grid', gridTemplateColumns: '32px minmax(0, 1fr) 70px 48px', alignItems: 'center', gap: 'var(--space-2)', fontSize: 'var(--text-xs)' }}>
              <CircularFileProgress item={item} />
              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', color: item.status === 'failed' ? 'var(--accent-red)' : 'var(--text-secondary)' }}>{truncatePath(item.path, 64)}</span>
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, color: 'var(--text-tertiary)' }}>
                {item.direction === 'up' ? <UploadIcon size={13} color="var(--accent-blue)" /> : item.direction === 'down' ? <DownloadIcon size={13} color="var(--accent-green)" /> : <SyncIcon size={13} color="var(--text-tertiary)" />}
                {item.direction || '-'}
              </span>
              <span style={{ textAlign: 'right', fontFamily: 'var(--font-mono)', color: item.status === 'syncing' ? 'var(--accent-blue)' : 'var(--text-tertiary)' }}>{Math.round(item.percent)}%</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
