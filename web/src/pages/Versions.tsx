import React, { useEffect, useState } from 'react';
import { listPairs, listVersions, restoreVersion } from '../api/client';
import type { SyncPair, VersionEntry } from '../api/client';
import { ClockIcon } from '../components/Icons';
import { showToast } from '../components/Toast';
import { useI18n } from '../i18n';

export const Versions: React.FC = () => {
  const { t } = useI18n();
  const [pairs, setPairs] = useState<SyncPair[]>([]);
  const [selectedPair, setSelectedPair] = useState('');
  const [searchPath, setSearchPath] = useState('');
  const [versions, setVersions] = useState<VersionEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);

  useEffect(() => {
    listPairs().then(setPairs).catch(() => setPairs([]));
  }, []);

  const handleSearch = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedPair) return;
    setLoading(true);
    setSearched(true);
    try {
      const data = await listVersions(selectedPair, searchPath);
      setVersions(data);
    } catch {
      showToast(t('versions.loadFailed'), 'error');
      setVersions([]);
    } finally {
      setLoading(false);
    }
  };

  const handleRestore = async (pairId: string, versionId: string) => {
    try {
      await restoreVersion(pairId, versionId);
      showToast(t('versions.versionRestored'), 'success');
    } catch (e) {
      showToast(e instanceof Error ? e.message : t('versions.restoreFailed'), 'error');
    }
  };

  function formatBytes(b: number): string {
    if (b === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(b) / Math.log(1024));
    return (b / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
  }

  const inputStyle: React.CSSProperties = {
    width: '100%',
    padding: 'var(--space-2) var(--space-3)',
    borderRadius: 'var(--radius-md)',
    border: '1px solid var(--border-default)',
    background: 'var(--bg-surface)',
    color: 'var(--text-primary)',
    fontFamily: 'var(--font-sans)',
    fontSize: 'var(--text-sm)',
    minHeight: '36px',
  };

  return (
    <div style={{ padding: 'var(--space-6)', maxWidth: '1000px', margin: '0 auto' }}>
      <div style={{ marginBottom: 'var(--space-6)' }}>
        <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>{t('versions.title')}</h1>
        <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
          {t('versions.subtitle')}
        </p>
      </div>

      <div className="card" style={{ marginBottom: 'var(--space-4)', padding: 'var(--space-4) var(--space-5)' }}>
        <form onSubmit={handleSearch} style={{ display: 'grid', gridTemplateColumns: '1fr 2fr auto', gap: 'var(--space-3)', alignItems: 'end' }}>
          <div style={{ display: 'grid', gap: 'var(--space-1)' }}>
            <label style={{ fontSize: 'var(--text-sm)', fontWeight: 500, color: 'var(--text-secondary)' }}>{t('versions.syncPair')}</label>
            <select value={selectedPair} onChange={(e) => setSelectedPair(e.target.value)} style={inputStyle}>
              <option value="">{t('versions.selectPair')}</option>
              {pairs.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
          </div>
          <div style={{ display: 'grid', gap: 'var(--space-1)' }}>
            <label style={{ fontSize: 'var(--text-sm)', fontWeight: 500, color: 'var(--text-secondary)' }}>{t('versions.path')}</label>
            <input value={searchPath} onChange={(e) => setSearchPath(e.target.value)} placeholder="/path/to/file" style={inputStyle} />
          </div>
          <button className="btn btn-primary" type="submit">{t('versions.search')}</button>
        </form>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 'var(--space-8)', color: 'var(--text-secondary)' }}>{t('common.loading')}</div>
      ) : !searched ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--text-tertiary)' }}>
          <ClockIcon size={32} color="var(--text-tertiary)" />
          <div style={{ marginTop: 'var(--space-3)' }}>{t('versions.selectHint')}</div>
        </div>
      ) : versions.length === 0 ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--text-tertiary)' }}>{t('versions.noRecords')}</div>
      ) : (
        <div className="card" style={{ padding: 0, overflow: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 'var(--text-sm)' }}>
            <thead>
              <tr>
                <th style={thStyle}>{t('versions.syncPair')}</th>
                <th style={thStyle}>{t('versions.path')}</th>
                <th style={thStyle}>{t('versions.source')}</th>
                <th style={thStyle}>{t('versions.size')}</th>
                <th style={thStyle}>{t('versions.fileTime')}</th>
                <th style={thStyle}>{t('versions.recorded')}</th>
                <th style={{ ...thStyle, textAlign: 'right' }}>{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {versions.map((v) => {
                const pairName = pairs.find((p) => p.id === v.pair_id)?.name ?? v.pair_id;
                return (
                  <tr key={v.id} style={{ borderBottom: '1px solid var(--border-muted)' }}>
                    <td style={tdStyle}>{pairName}</td>
                    <td style={{ ...tdStyle, fontFamily: 'var(--font-mono)', fontSize: 'var(--text-xs)' }}>{v.path}</td>
                    <td style={tdStyle}><span className="badge badge-blue">{v.version}</span></td>
                    <td style={tdStyle}>{formatBytes(v.size)}</td>
                    <td style={tdStyle}>{new Date(v.modified).toLocaleString()}</td>
                    <td style={tdStyle}>{new Date(v.modified).toLocaleString()}</td>
                    <td style={{ ...tdStyle, textAlign: 'right' }}>
                      <button className="btn btn-sm" onClick={() => handleRestore(v.pair_id, v.id)}>{t('versions.restore')}</button>
                    </td>
                  </tr>
                );
              })}
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
  verticalAlign: 'middle',
};
