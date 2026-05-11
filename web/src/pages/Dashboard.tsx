import React, { useEffect, useState } from 'react';
import { getDashboardStats, listPairs, triggerSync } from '../api/client';
import type { DashboardStats, SyncPair } from '../api/client';
import { StatusIcon } from '../components/StatusIcon';
import { SyncIcon, UploadIcon, DownloadIcon, WarningIcon, PlayIcon } from '../components/Icons';

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return (bytes / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
}

function formatRelative(dateStr?: string): string {
  if (!dateStr) return 'Never';
  const diff = Date.now() - new Date(dateStr).getTime();
  if (diff < 60000) return 'Just now';
  if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
  if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
  return Math.floor(diff / 86400000) + 'd ago';
}

export const Dashboard: React.FC = () => {
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [pairs, setPairs] = useState<SyncPair[]>([]);
  const [loading, setLoading] = useState(true);
  const [syncingPairId, setSyncingPairId] = useState<string | null>(null);

  useEffect(() => {
    const load = async () => {
      try {
        const [s, p] = await Promise.all([getDashboardStats(), listPairs()]);
        setStats(s);
        setPairs(p);
      } catch {
        // API might not be available yet — show placeholder
        setStats({
          engine_status: 'stopped',
          active_pairs: 0,
          total_pairs: 0,
          pending_tasks: 0,
          active_workers: 0,
          upload_bytes: 0,
          download_bytes: 0,
          conflicts: 0,
          virtual_files: 0,
        });
        setPairs([]);
      } finally {
        setLoading(false);
      }
    };
    load();
  }, []);

  const handleSync = async (pairId: string) => {
    setSyncingPairId(pairId);
    try {
      await triggerSync(pairId);
    } catch {
      // ignore errors
    } finally {
      setSyncingPairId(null);
    }
  };

  if (loading) {
    return (
      <PageWrapper>
        <div style={{ textAlign: 'center', padding: 'var(--space-12)', color: 'var(--text-secondary)' }}>
          Loading...
        </div>
      </PageWrapper>
    );
  }

  const engineLabel = stats?.engine_status === 'running' ? 'Running' : stats?.engine_status === 'paused' ? 'Paused' : 'Stopped';
  const engineColor = stats?.engine_status === 'running' ? 'var(--accent-green)' : stats?.engine_status === 'paused' ? 'var(--accent-amber)' : 'var(--text-tertiary)';

  return (
    <PageWrapper>
      <PageHeader title="Dashboard" subtitle="Overview of your sync engine" />

      {/* Metric cards row */}
      <div style={styles.cardRow}>
        <MetricCard
          label="Engine Status"
          value={engineLabel}
          icon={<div style={{ width: 10, height: 10, borderRadius: '50%', background: engineColor }} />}
          badge={engineLabel === 'Running' ? 'green' : engineLabel === 'Paused' ? 'amber' : undefined}
        />
        <MetricCard
          label="Sync Pairs"
          value={`${stats?.active_pairs ?? 0} / ${stats?.total_pairs ?? 0}`}
          icon={<SyncIcon size={18} color="var(--accent-blue)" />}
        />
        <MetricCard
          label="Pending Tasks"
          value={String(stats?.pending_tasks ?? 0)}
          icon={<WarningIcon size={18} color="var(--accent-amber)" />}
          badge={(stats?.pending_tasks ?? 0) > 0 ? 'amber' : undefined}
        />
        <MetricCard
          label="Workers"
          value={String(stats?.active_workers ?? 0)}
          icon={<SyncIcon size={18} color="var(--accent-violet)" />}
        />
      </div>

      {/* Traffic cards row */}
      <div style={styles.cardRow}>
        <TrafficCard
          label="Upload"
          value={formatBytes(stats?.upload_bytes ?? 0)}
          icon={<UploadIcon size={18} color="var(--accent-blue)" />}
        />
        <TrafficCard
          label="Download"
          value={formatBytes(stats?.download_bytes ?? 0)}
          icon={<DownloadIcon size={18} color="var(--accent-green)" />}
        />
        <TrafficCard
          label="Conflicts"
          value={String(stats?.conflicts ?? 0)}
          icon={<WarningIcon size={18} color="var(--accent-red)" />}
          badge={(stats?.conflicts ?? 0) > 0 ? 'red' : undefined}
        />
        <TrafficCard
          label="Virtual Files"
          value={String(stats?.virtual_files ?? 0)}
          icon={<WarningIcon size={18} color="var(--accent-violet)" />}
        />
      </div>

      {/* Active pairs */}
      <div className="card" style={{ marginTop: 'var(--space-5)' }}>
        <h3 style={styles.sectionTitle}>Active Sync Pairs</h3>
        {pairs.length === 0 ? (
          <div style={styles.emptyState}>
            No sync pairs configured. Create one to get started.
          </div>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table style={styles.table}>
              <thead>
                <tr>
                  <th style={styles.th}>Name</th>
                  <th style={styles.th}>Mode</th>
                  <th style={styles.th}>Status</th>
                  <th style={styles.th}>Last Sync</th>
                  <th style={styles.th}>Files</th>
                  <th style={{ ...styles.th, textAlign: 'right' }}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {pairs.map((pair) => (
                  <tr key={pair.id} style={styles.tr}>
                    <td style={styles.td}>
                      <span style={{ fontWeight: 500 }}>{pair.name}</span>
                      <span style={{ display: 'block', fontSize: 'var(--text-xs)', color: 'var(--text-tertiary)' }}>
                        {pair.local_path} &rarr; {pair.remote_path}
                      </span>
                    </td>
                    <td style={styles.td}>
                      <span className="badge badge-blue">{pair.mode}</span>
                    </td>
                    <td style={styles.td}>
                      <span style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
                        <StatusIcon status={pair.status} />
                        <span style={{ textTransform: 'capitalize' }}>{pair.status}</span>
                      </span>
                    </td>
                    <td style={{ ...styles.td, color: 'var(--text-secondary)' }}>
                      {formatRelative(pair.last_sync)}
                    </td>
                    <td style={styles.td}>
                      {pair.stats ? (
                        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--text-xs)' }}>
                          {pair.stats.synced_files}/{pair.stats.total_files}
                        </span>
                      ) : '—'}
                    </td>
                    <td style={{ ...styles.td, textAlign: 'right' }}>
                      <button
                        className="btn btn-sm btn-primary"
                        onClick={() => handleSync(pair.id)}
                        disabled={syncingPairId === pair.id}
                        style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}
                      >
                        {syncingPairId === pair.id ? (
                          <SyncIcon size={14} color="#fff" spinning />
                        ) : (
                          <PlayIcon size={14} color="#fff" />
                        )}
                        Sync
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </PageWrapper>
  );
};

// ---- Layout helpers ----

const PageWrapper: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <div style={{ padding: 'var(--space-6)', maxWidth: '1200px', margin: '0 auto' }}>
    {children}
  </div>
);

const PageHeader: React.FC<{ title: string; subtitle?: string }> = ({ title, subtitle }) => (
  <div style={{ marginBottom: 'var(--space-6)' }}>
    <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, color: 'var(--text-primary)', margin: 0 }}>
      {title}
    </h1>
    {subtitle && (
      <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
        {subtitle}
      </p>
    )}
  </div>
);

interface MetricCardProps {
  label: string;
  value: string;
  icon: React.ReactNode;
  badge?: 'green' | 'amber' | 'red';
}

const MetricCard: React.FC<MetricCardProps> = ({ label, value, icon, badge }) => (
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
          {badge === 'green' ? 'Active' : badge === 'amber' ? 'Pending' : 'Alert'}
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
}

const TrafficCard: React.FC<TrafficCardProps> = ({ label, value, icon, badge }) => (
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
      {badge && <span className={`badge badge-${badge}`}>{badge === 'red' ? 'Needs attention' : ''}</span>}
    </div>
  </div>
);

// ---- Styles ----

const styles: Record<string, React.CSSProperties> = {
  cardRow: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
    gap: 'var(--space-4)',
  },
  metricCard: {
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
