import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { listPairs, listSyncRecords } from '../api/client';
import type { SyncPair, SyncRecord } from '../api/client';
import { CheckIcon, DownloadIcon, SyncIcon, UploadIcon, WarningIcon } from '../components/Icons';
import { useI18n } from '../i18n';

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

function formatDate(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

export const RecentRecords: React.FC = () => {
  const { t } = useI18n();
  const [records, setRecords] = useState<SyncRecord[]>([]);
  const [pairs, setPairs] = useState<SyncPair[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [nextRecords, nextPairs] = await Promise.all([listSyncRecords(200), listPairs()]);
      setRecords(nextRecords);
      setPairs(nextPairs);
    } catch (e) {
      setError(e instanceof Error ? e.message : t('recent.loadFailed'));
      setRecords([]);
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => { load(); }, [load]);

  const pairNames = useMemo(() => new Map(pairs.map((pair) => [pair.id, pair.name])), [pairs]);

  return (
    <div style={{ padding: 'var(--space-6)', maxWidth: '1100px', margin: '0 auto' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-3)', marginBottom: 'var(--space-6)' }}>
        <div>
          <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>{t('recent.title')}</h1>
          <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>{t('recent.subtitle')}</p>
        </div>
        <button className="btn" onClick={load}>{t('dashboard.refresh')}</button>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 'var(--space-10)', color: 'var(--text-secondary)' }}>{t('common.loading')}</div>
      ) : error ? (
        <div className="card" style={{ padding: 'var(--space-10)', color: 'var(--accent-red)', textAlign: 'center' }}>
          <div style={{ marginBottom: 'var(--space-3)' }}>{t('recent.loadFailed')}: {error}</div>
          <button className="btn" onClick={load}>{t('common.retry')}</button>
        </div>
      ) : records.length === 0 ? (
        <div className="card" style={{ padding: 'var(--space-10)', color: 'var(--text-tertiary)', textAlign: 'center' }}>{t('recent.empty')}</div>
      ) : (
        <div className="card" style={{ padding: 0, overflow: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 'var(--text-sm)' }}>
            <thead>
              <tr>
                <th style={thStyle}>{t('recent.file')}</th>
                <th style={thStyle}>{t('recent.pair')}</th>
                <th style={thStyle}>{t('recent.time')}</th>
                <th style={thStyle}>{t('recent.status')}</th>
                <th style={thStyle}>{t('recent.direction')}</th>
                <th style={thStyle}>{t('recent.size')}</th>
              </tr>
            </thead>
            <tbody>
              {records.map((record) => (
                <tr key={record.id} style={{ borderBottom: '1px solid var(--border-muted)' }}>
                  <td style={{ ...tdStyle, fontFamily: 'var(--font-mono)', fontSize: 'var(--text-xs)' }}>{record.path}</td>
                  <td style={tdStyle}>{record.pair_name || pairNames.get(record.pair_id) || record.pair_id}</td>
                  <td style={tdStyle}>{formatDate(record.finished_at)}</td>
                  <td style={tdStyle}>
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 'var(--space-1)', color: record.status === 'failed' ? 'var(--accent-red)' : 'var(--accent-green)' }}>
                      {record.status === 'failed' ? <WarningIcon size={14} color="var(--accent-red)" /> : <CheckIcon size={14} color="var(--accent-green)" />}
                      {record.status === 'failed' ? t('status.error') : t('status.synced')}
                    </span>
                    {record.error && <div style={{ color: 'var(--accent-red)', fontSize: 'var(--text-xs)', marginTop: 3 }}>{record.error}</div>}
                  </td>
                  <td style={tdStyle}>
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 'var(--space-1)' }}>
                      {record.direction === 'up' ? <UploadIcon size={14} color="var(--accent-blue)" /> : record.direction === 'down' ? <DownloadIcon size={14} color="var(--accent-green)" /> : <SyncIcon size={14} color="var(--text-secondary)" />}
                      {record.direction || '-'}
                    </span>
                  </td>
                  <td style={tdStyle}>{formatBytes(record.bytes_total)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
};

const thStyle: React.CSSProperties = {
  textAlign: 'left',
  padding: 'var(--space-3) var(--space-4)',
  fontSize: 'var(--text-xs)',
  fontWeight: 500,
  color: 'var(--text-secondary)',
  textTransform: 'uppercase',
  letterSpacing: '0.05em',
  borderBottom: '1px solid var(--border-default)',
  background: 'var(--bg-surface)',
};

const tdStyle: React.CSSProperties = {
  padding: 'var(--space-3) var(--space-4)',
  verticalAlign: 'top',
};
