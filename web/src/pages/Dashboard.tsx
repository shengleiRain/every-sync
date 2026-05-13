import React, { useCallback, useEffect, useState } from 'react';
import { getDashboardStats, listPairs, triggerSync, syncAll } from '../api/client';
import { showToast } from '../components/Toast';
import type { DashboardStats, SyncPair } from '../api/client';
import { StatusIcon } from '../components/StatusIcon';
import { SyncIcon, UploadIcon, DownloadIcon, WarningIcon, PlayIcon, CloudIcon } from '../components/Icons';
import { getPairModeLabelKey, getSyncStatusLabelKey, useI18n } from '../i18n';
import { useSyncProgress } from '../hooks/useSyncProgress';
import { useIsNarrow } from '../hooks/useViewport';
import { PairProgressInline, PairSyncQueuePanel } from '../components/PairProgress';
import { useWebSocket } from '../hooks/useWebSocket';
import type { WSEvent } from '../api/client';

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return (bytes / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
}

function formatRelative(dateStr: string | undefined, t: (key: string, params?: Record<string, string | number>) => string): string {
  if (!dateStr) return t('time.never');
  const diff = Date.now() - new Date(dateStr).getTime();
  if (!Number.isFinite(diff)) return t('time.never');
  if (diff < 60000) return t('time.justNow');
  if (diff < 3600000) return t('time.minutesAgo', { n: Math.floor(diff / 60000) });
  if (diff < 86400000) return t('time.hoursAgo', { n: Math.floor(diff / 3600000) });
  return t('time.daysAgo', { n: Math.floor(diff / 86400000) });
}

export const Dashboard: React.FC = () => {
  const { t } = useI18n();
  const { getProgress, activeSyncCount } = useSyncProgress();
  const isNarrow = useIsNarrow();
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [pairs, setPairs] = useState<SyncPair[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [syncingPairId, setSyncingPairId] = useState<string | null>(null);
  const refreshTimerRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [s, p] = await Promise.all([getDashboardStats(), listPairs()]);
      setStats(s);
      setPairs(p);
    } catch (e) {
      setError(e instanceof Error ? e.message : t('dashboard.loadFailed'));
      setStats(null);
      setPairs([]);
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => () => {
    if (refreshTimerRef.current) {
      clearTimeout(refreshTimerRef.current);
    }
  }, []);

  const scheduleRefresh = useCallback(() => {
    if (refreshTimerRef.current) return;
    refreshTimerRef.current = setTimeout(() => {
      refreshTimerRef.current = null;
      load();
    }, 500);
  }, [load]);

  useWebSocket({
    onEvent: useCallback((event: WSEvent) => {
      if ([
        'engine_started',
        'engine_stopped',
        'task_queued',
        'task_completed',
        'task_failed',
        'sync_completed',
        'sync_failed',
        'conflict_detected',
        'conflict_resolved',
        'pair_registered',
        'pair_unregistered',
        'pair_refreshed',
      ].includes(event.type)) {
        scheduleRefresh();
      }
    }, [scheduleRefresh]),
  });

  // Refresh dashboard data when sync activity completes (activeSyncCount drops to 0)
  const prevSyncCountRef = React.useRef(activeSyncCount);
  useEffect(() => {
    if (prevSyncCountRef.current > 0 && activeSyncCount === 0) {
      load();
    }
    prevSyncCountRef.current = activeSyncCount;
  }, [activeSyncCount, load]);

  const handleSync = async (pairId: string) => {
    setSyncingPairId(pairId);
    try {
      await triggerSync(pairId);
      showToast(t('pairs.syncTriggered'), 'success');
    } catch (e) {
      showToast(e instanceof Error ? e.message : t('dashboard.syncFailed'), 'error');
    } finally {
      setSyncingPairId(null);
    }
  };

  const handleSyncAll = async () => {
    try {
      await syncAll();
      showToast(t('dashboard.syncTriggered'), 'success');
    } catch (e) {
      showToast(e instanceof Error ? e.message : t('dashboard.syncFailed'), 'error');
    }
  };

  if (loading) {
    return (
      <PageWrapper>
        <div style={{ textAlign: 'center', padding: 'var(--space-12)', color: 'var(--text-secondary)' }}>
          {t('common.loading')}
        </div>
      </PageWrapper>
    );
  }

  if (error) {
    return (
      <PageWrapper>
        <div className="card" style={styles.emptyState}>
          <div style={{ color: 'var(--accent-red)', marginBottom: 'var(--space-3)' }}>
            {t('dashboard.loadFailed')}: {error}
          </div>
          <button className="btn" onClick={load}>{t('common.retry')}</button>
        </div>
      </PageWrapper>
    );
  }

  const statusKey = stats?.engine_status === 'running' ? 'status.running' : stats?.engine_status === 'paused' ? 'status.paused' : 'status.stopped';
  const engineLabel = t(statusKey);
  const engineColor = stats?.engine_status === 'running' ? 'var(--accent-green)' : stats?.engine_status === 'paused' ? 'var(--accent-amber)' : 'var(--text-tertiary)';

  return (
    <PageWrapper>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-3)', flexWrap: 'wrap', marginBottom: 'var(--space-6)' }}>
        <div style={{ minWidth: '220px', flex: '1 1 auto' }}>
          <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, color: 'var(--text-primary)', margin: 0 }}>{t('dashboard.title')}</h1>
          <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>{t('dashboard.subtitle')}</p>
        </div>
        <div style={{ display: 'flex', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
          <button className="btn btn-primary" onClick={handleSyncAll} style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
            <SyncIcon size={15} color="#fff" /> {t('dashboard.syncAll')}
          </button>
          <button className="btn" onClick={load}>
            {t('dashboard.refresh')}
          </button>
        </div>
      </div>

      <div style={styles.metricsFlow}>
        <MetricCard
          label={t('dashboard.engineStatus')}
          value={engineLabel}
          icon={<div style={{ width: 10, height: 10, borderRadius: '50%', background: engineColor }} />}
          badge={stats?.engine_status === 'running' ? 'green' : stats?.engine_status === 'paused' ? 'amber' : undefined}
          t={t}
        />
        <MetricCard
          label={t('dashboard.syncPairs')}
          value={`${stats?.active_pairs ?? 0} / ${stats?.total_pairs ?? 0}`}
          icon={<SyncIcon size={18} color="var(--accent-blue)" />}
          t={t}
        />
        <MetricCard
          label={t('dashboard.pendingTasks')}
          value={String(stats?.pending_tasks ?? 0)}
          icon={<WarningIcon size={18} color="var(--accent-amber)" />}
          badge={(stats?.pending_tasks ?? 0) > 0 ? 'amber' : undefined}
          t={t}
        />
        <MetricCard
          label={t('dashboard.workers')}
          value={String(stats?.active_workers ?? 0)}
          icon={<SyncIcon size={18} color="var(--accent-violet)" />}
          t={t}
        />
        <TrafficCard label={t('dashboard.upload')} value={formatBytes(stats?.upload_bytes ?? 0)} icon={<UploadIcon size={18} color="var(--accent-blue)" />} />
        <TrafficCard label={t('dashboard.download')} value={formatBytes(stats?.download_bytes ?? 0)} icon={<DownloadIcon size={18} color="var(--accent-green)" />} />
        <TrafficCard label={t('dashboard.conflicts')} value={String(stats?.conflicts ?? 0)} icon={<WarningIcon size={18} color="var(--accent-red)" />} badge={(stats?.conflicts ?? 0) > 0 ? 'red' : undefined} t={t} />
        <TrafficCard label={t('dashboard.virtualFiles')} value={String(stats?.virtual_files ?? 0)} icon={<CloudIcon size={18} color="var(--accent-violet)" />} />
      </div>

      <div className="card" style={{ marginTop: 'var(--space-5)' }}>
        <h3 style={styles.sectionTitle}>{t('dashboard.activePairs')}</h3>
        {pairs.length === 0 ? (
          <div style={styles.emptyState}>
            {t('dashboard.noPairs')}
          </div>
        ) : (
          <div style={{ display: 'grid', gap: 'var(--space-3)' }}>
            {pairs.map((pair) => {
              const progress = getProgress(pair.id);
              const isActivelySyncing = progress?.status === 'syncing' || progress?.status === 'scanning';
              return (
                <div key={pair.id} className="card" style={{ padding: 'var(--space-4)', border: '1px solid var(--border-muted)' }}>
                  <div style={{ display: 'grid', gridTemplateColumns: isNarrow ? '1fr' : 'minmax(0, 1fr) auto', gap: 'var(--space-3)', alignItems: 'start' }}>
                    <div style={{ minWidth: 0 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
                        <span style={{ fontWeight: 600 }}>{pair.name}</span>
                        <span className="badge badge-blue">{t(getPairModeLabelKey(pair.mode))}</span>
                        {isActivelySyncing ? (
                          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, color: 'var(--accent-green)', fontSize: 'var(--text-xs)' }}>
                            <SyncIcon size={14} color="var(--accent-green)" spinning /> {t('status.syncing')}
                          </span>
                        ) : (
                          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 'var(--text-xs)' }}>
                            <StatusIcon status={pair.status} size={14} /> {t(getSyncStatusLabelKey(pair.status))}
                          </span>
                        )}
                      </div>
                      <div style={{ marginTop: 4, fontSize: 'var(--text-xs)', color: 'var(--text-tertiary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {pair.local_path} &rarr; {pair.remote_path}
                      </div>
                      <div style={{ marginTop: 'var(--space-2)', display: 'flex', gap: 'var(--space-4)', flexWrap: 'wrap', fontSize: 'var(--text-xs)', color: 'var(--text-secondary)' }}>
                        <span>{t('dashboard.lastSync')}: {formatRelative(pair.last_sync, t)}</span>
                        <span>{t('dashboard.files')}: {pair.stats ? `${pair.stats.synced_files}/${pair.stats.total_files}` : '-'}</span>
                        {isActivelySyncing && progress.bytesTransferred > 0 && <span>{formatBytes(progress.bytesTransferred)}{progress.bytesTotal > 0 && ` / ${formatBytes(progress.bytesTotal)}`}</span>}
                      </div>
                      <PairProgressInline progress={progress} t={t} />
                    </div>
                    <button
                      className="btn btn-sm btn-primary"
                      onClick={() => handleSync(pair.id)}
                      disabled={syncingPairId === pair.id || isActivelySyncing}
                      style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}
                    >
                      {syncingPairId === pair.id || isActivelySyncing ? <SyncIcon size={14} color="#fff" spinning /> : <PlayIcon size={14} color="#fff" />}
                      {t('dashboard.sync')}
                    </button>
                  </div>
                  <PairSyncQueuePanel progress={progress} t={t} />
                </div>
              );
            })}
          </div>
        )}
      </div>
    </PageWrapper>
  );
};

const PageWrapper: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <PageWrapperInner>{children}</PageWrapperInner>
);

const PageWrapperInner: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const isNarrow = useIsNarrow();
  return (
    <div style={{ padding: isNarrow ? 'var(--space-4)' : 'var(--space-6)', maxWidth: '1200px', margin: '0 auto' }}>
      {children}
    </div>
  );
};

interface MetricCardProps {
  label: string;
  value: string;
  icon: React.ReactNode;
  badge?: 'green' | 'amber' | 'red';
  t: (key: string) => string;
}

const MetricCard: React.FC<MetricCardProps> = ({ label, value, icon, badge, t }) => (
  <div className="card" style={styles.metricCard}>
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 'var(--space-2)' }}>
      <span style={{ fontSize: 'var(--text-xs)', fontWeight: 500, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
        {label}
      </span>
      {icon}
    </div>
    <div style={{ display: 'flex', alignItems: 'baseline', gap: 'var(--space-2)' }}>
      <span style={{ fontSize: 'var(--text-2xl)', fontWeight: 700, color: 'var(--text-primary)' }}>
        {value}
      </span>
      {badge && (
        <span className={`badge badge-${badge}`}>
          {badge === 'green' ? t('status.active') : badge === 'amber' ? t('status.pending') : t('status.alert')}
        </span>
      )}
    </div>
  </div>
);

interface TrafficCardProps {
  label: string;
  value: string;
  icon: React.ReactNode;
  badge?: 'green' | 'amber' | 'red';
  t?: (key: string) => string;
}

const TrafficCard: React.FC<TrafficCardProps> = ({ label, value, icon, badge, t }) => (
  <div className="card" style={styles.metricCard}>
    <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', marginBottom: 'var(--space-1)' }}>
      {icon}
      <span style={{ fontSize: 'var(--text-xs)', fontWeight: 500, color: 'var(--text-secondary)' }}>
        {label}
      </span>
    </div>
    <div style={{ display: 'flex', alignItems: 'baseline', gap: 'var(--space-2)' }}>
      <span style={{ fontSize: 'var(--text-xl)', fontWeight: 600, fontFamily: 'var(--font-mono)', color: 'var(--text-primary)' }}>
        {value}
      </span>
      {badge && t && <span className={`badge badge-${badge}`}>{badge === 'red' ? t('status.needsAttention') : ''}</span>}
    </div>
  </div>
);

const styles: Record<string, React.CSSProperties> = {
  metricsFlow: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: 'var(--space-4)',
    alignItems: 'stretch',
  },
  metricCard: {
    flex: '1 1 178px',
    minWidth: 'min(100%, 178px)',
    padding: 'var(--space-4) var(--space-5)',
  },
  sectionTitle: {
    fontSize: 'var(--text-lg)',
    fontWeight: 600,
    color: 'var(--text-primary)',
    marginBottom: 'var(--space-4)',
  },
  emptyState: {
    padding: 'var(--space-8)',
    textAlign: 'center' as const,
    color: 'var(--text-tertiary)',
    fontSize: 'var(--text-sm)',
  },
  table: {
    width: '100%',
    borderCollapse: 'collapse' as const,
    fontSize: 'var(--text-sm)',
  },
  th: {
    textAlign: 'left' as const,
    padding: 'var(--space-2) var(--space-3)',
    fontSize: 'var(--text-xs)',
    fontWeight: 500,
    color: 'var(--text-secondary)',
    textTransform: 'uppercase' as const,
    letterSpacing: '0.05em',
    borderBottom: '1px solid var(--border-default)',
  },
  tr: {
    borderBottom: '1px solid var(--border-muted)',
  },
  td: {
    padding: 'var(--space-3)',
    verticalAlign: 'middle' as const,
  },
};
