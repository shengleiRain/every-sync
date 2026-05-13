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
