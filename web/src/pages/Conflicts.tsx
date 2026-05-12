import React, { useCallback, useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { listConflicts, resolveConflict } from '../api/client';
import type { ConflictEntry } from '../api/client';
import { WarningIcon } from '../components/Icons';
import { showToast } from '../components/Toast';
import { useI18n } from '../i18n';

export const Conflicts: React.FC = () => {
  const { t } = useI18n();
  const location = useLocation();
  const [conflicts, setConflicts] = useState<ConflictEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [resolving, setResolving] = useState<string | null>(null);

  const load = useCallback(() => {
    const params = new URLSearchParams(location.search);
    setLoading(true);
    setLoadError(null);
    listConflicts(params.get('pair_id') ?? undefined)
      .then(setConflicts)
      .catch((e) => {
        setConflicts([]);
        setLoadError(e instanceof Error ? e.message : t('conflicts.loadFailed'));
      })
      .finally(() => setLoading(false));
  }, [location.search, t]);

  useEffect(() => { load(); }, [load]);

  const handleResolve = async (id: string, strategy: string) => {
    setResolving(id);
    try {
      await resolveConflict(id, strategy);
      setConflicts((prev) => prev.filter((c) => c.id !== id));
      showToast(t('conflicts.resolved'), 'success');
    } catch (e) {
      showToast(e instanceof Error ? e.message : t('conflicts.resolutionFailed'), 'error');
    } finally {
      setResolving(null);
    }
  };

  return (
    <div style={{ padding: 'var(--space-6)', maxWidth: '1000px', margin: '0 auto' }}>
      <div style={{ marginBottom: 'var(--space-6)' }}>
        <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>{t('conflicts.title')}</h1>
        <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
          {t('conflicts.subtitle')}
        </p>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 'var(--space-12)', color: 'var(--text-secondary)' }}>{t('common.loading')}</div>
      ) : loadError ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--accent-red)' }}>
          <div style={{ marginBottom: 'var(--space-3)' }}>{t('conflicts.loadFailed')}: {loadError}</div>
          <button className="btn" onClick={load}>{t('common.retry')}</button>
        </div>
      ) : conflicts.length === 0 ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--text-tertiary)' }}>
          <WarningIcon size={32} color="var(--accent-green)" />
          <div style={{ marginTop: 'var(--space-3)' }}>{t('conflicts.noConflicts')}</div>
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          {conflicts.map((c) => (
            <div
              key={c.id}
              className="card"
              style={{ padding: 'var(--space-4) var(--space-5)' }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', marginBottom: 'var(--space-3)' }}>
                <WarningIcon size={20} color="var(--accent-red)" />
                <span style={{ fontWeight: 600, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {c.path}
                </span>
              </div>
              <div style={{ display: 'flex', gap: 'var(--space-6)', fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginBottom: 'var(--space-4)' }}>
                <div>
                  <div style={{ fontWeight: 500, color: 'var(--text-primary)' }}>{t('conflicts.local')}</div>
                  <div>{t('conflicts.modified')}: {formatDate(c.local_modified, t)}</div>
                  <div>{t('conflicts.size')}: {c.local_size} {t('conflicts.bytes')}</div>
                </div>
                <div>
                  <div style={{ fontWeight: 500, color: 'var(--text-primary)' }}>{t('conflicts.remote')}</div>
                  <div>{t('conflicts.modified')}: {formatDate(c.remote_modified, t)}</div>
                  <div>{t('conflicts.size')}: {c.remote_size} {t('conflicts.bytes')}</div>
                </div>
              </div>
              <div style={{ display: 'flex', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
                <button className="btn btn-sm btn-primary" disabled={resolving === c.id} onClick={() => handleResolve(c.id, 'local_wins')}>{t('conflicts.keepLocal')}</button>
                <button className="btn btn-sm" disabled={resolving === c.id} onClick={() => handleResolve(c.id, 'remote_wins')}>{t('conflicts.keepRemote')}</button>
                <button className="btn btn-sm" disabled={resolving === c.id} onClick={() => handleResolve(c.id, 'latest_wins')}>{t('conflicts.latestWins')}</button>
                <button className="btn btn-sm" style={{ color: 'var(--accent-amber)' }} disabled={resolving === c.id} onClick={() => handleResolve(c.id, 'skip')}>{t('conflicts.skip')}</button>
              </div>
            </div>
          ))}
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
