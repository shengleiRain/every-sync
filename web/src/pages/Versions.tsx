import React, { useCallback, useEffect, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { listPairs, listVersions } from '../api/client';
import type { SyncPair, VersionEntry } from '../api/client';
import { ClockIcon } from '../components/Icons';
import { showToast } from '../components/Toast';
import { useI18n } from '../i18n';

export const Versions: React.FC = () => {
  const { t } = useI18n();
  const location = useLocation();
  const navigate = useNavigate();
  const initialParams = new URLSearchParams(location.search);
  const [pairs, setPairs] = useState<SyncPair[]>([]);
  const [selectedPair, setSelectedPair] = useState(initialParams.get('pair_id') ?? '');
  const [searchPath, setSearchPath] = useState(initialParams.get('path') ?? '');
  const [versions, setVersions] = useState<VersionEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [pairsLoading, setPairsLoading] = useState(true);
  const [pairsError, setPairsError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);

  useEffect(() => {
    setPairsLoading(true);
    setPairsError(null);
    listPairs()
      .then(setPairs)
      .catch((e) => {
        setPairs([]);
        setPairsError(e instanceof Error ? e.message : t('versions.pairsLoadFailed'));
      })
      .finally(() => setPairsLoading(false));
  }, [t]);

  const loadVersions = useCallback(async (pairId: string, path: string) => {
    if (!pairId) return;
    setLoading(true);
    setSearched(true);
    setLoadError(null);
    try {
      const data = await listVersions(pairId, path);
      setVersions(data);
    } catch (e) {
      const message = e instanceof Error ? e.message : t('versions.loadFailed');
      setLoadError(message);
      showToast(message, 'error');
      setVersions([]);
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const pairId = params.get('pair_id') ?? '';
    const path = params.get('path') ?? '';
    setSelectedPair(pairId);
    setSearchPath(path);
    if (pairId) {
      loadVersions(pairId, path);
    }
  }, [loadVersions, location.search]);

  const handleSearch = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedPair) return;
    const params = new URLSearchParams({ pair_id: selectedPair });
    if (searchPath) params.set('path', searchPath);
    const nextSearch = `?${params}`;
    if (location.search !== nextSearch) {
      navigate(`/versions${nextSearch}`);
      return;
    }
    await loadVersions(selectedPair, searchPath);
  };

  function formatBytes(b: number): string {
    if (b === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(Math.floor(Math.log(b) / Math.log(1024)), units.length - 1);
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
            <select value={selectedPair} onChange={(e) => setSelectedPair(e.target.value)} style={inputStyle} disabled={pairsLoading}>
              <option value="">{t('versions.selectPair')}</option>
              {pairs.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
            {pairsError && <span style={{ color: 'var(--accent-red)', fontSize: 'var(--text-xs)' }}>{pairsError}</span>}
          </div>
          <div style={{ display: 'grid', gap: 'var(--space-1)' }}>
            <label style={{ fontSize: 'var(--text-sm)', fontWeight: 500, color: 'var(--text-secondary)' }}>{t('versions.path')}</label>
            <input value={searchPath} onChange={(e) => setSearchPath(e.target.value)} placeholder="/path/to/file" style={inputStyle} />
          </div>
          <button className="btn btn-primary" type="submit" disabled={!selectedPair || loading}>{t('versions.search')}</button>
        </form>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 'var(--space-8)', color: 'var(--text-secondary)' }}>{t('common.loading')}</div>
      ) : loadError ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--accent-red)' }}>
          <div style={{ marginBottom: 'var(--space-3)' }}>{t('versions.loadFailed')}: {loadError}</div>
          <button className="btn" onClick={() => loadVersions(selectedPair, searchPath)}>{t('common.retry')}</button>
        </div>
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
              </tr>
            </thead>
            <tbody>
              {versions.map((v) => {
                const pairName = pairs.find((p) => p.id === v.pair_id)?.name ?? v.pair_id;
                return (
                  <tr key={v.id} style={{ borderBottom: '1px solid var(--border-muted)' }}>
                    <td style={tdStyle}>{pairName}</td>
                    <td style={{ ...tdStyle, fontFamily: 'var(--font-mono)', fontSize: 'var(--text-xs)' }}>{v.path}</td>
                    <td style={tdStyle}><span className="badge badge-blue">{v.source || t('common.notAvailable')}</span></td>
                    <td style={tdStyle}>{formatBytes(v.size)}</td>
                    <td style={tdStyle}>{formatDate(v.modified, t)}</td>
                    <td style={tdStyle}>{formatDate(v.recorded, t)}</td>
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

function formatDate(value: string | undefined, t: (key: string) => string): string {
  if (!value) return t('common.notAvailable');
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? t('common.notAvailable') : date.toLocaleString();
}

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
