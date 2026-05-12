import React, { useCallback, useEffect, useState } from 'react';
import {
  listPairs, createPair, updatePair, deletePair, triggerSync, listProviders,
} from '../api/client';
import type { SyncPair } from '../api/client';
import { StatusIcon } from '../components/StatusIcon';
import { SyncIcon, PlayIcon } from '../components/Icons';
import { Modal } from '../components/Modal';
import { showToast } from '../components/Toast';
import { getPairDirectionLabelKey, getPairModeLabelKey, useI18n } from '../i18n';
import { useSyncProgress } from '../hooks/useSyncProgress';

const emptyForm = {
  name: '',
  local_path: '',
  remote_path: '',
  provider: '',
  mode: 'normal',
  direction: 'both',
  conflict_strategy: 'latest_wins',
  include_patterns: '',
  exclude_patterns: '',
};

type PairForm = typeof emptyForm;

function truncate(str: string, maxLen: number): string {
  if (str.length <= maxLen) return str;
  return '...' + str.slice(str.length - maxLen + 3);
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return (bytes / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
}

export const SyncPairs: React.FC = () => {
  const { t } = useI18n();
  const { getProgress } = useSyncProgress();
  const [pairs, setPairs] = useState<SyncPair[]>([]);
  const [providers, setProviders] = useState<{ id: string; name: string }[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [syncingId, setSyncingId] = useState<string | null>(null);

  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<PairForm>({ ...emptyForm });
  const [submitting, setSubmitting] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const [p, prov] = await Promise.all([listPairs(), listProviders()]);
      setPairs(p);
      setProviders(prov.map((x) => ({ id: x.id, name: x.name })));
    } catch (e) {
      const message = e instanceof Error ? e.message : t('pairs.loadFailed');
      setLoadError(message);
      setPairs([]);
      showToast(message, 'error');
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => { load(); }, [load]);

  const handleSync = async (id: string) => {
    setSyncingId(id);
    try {
      await triggerSync(id);
      showToast(t('pairs.syncTriggered'), 'success');
    } catch (e) {
      showToast(e instanceof Error ? e.message : t('pairs.operationFailed'), 'error');
    } finally {
      setSyncingId(null);
    }
  };

  const openCreate = () => {
    setEditingId(null);
    setForm({ ...emptyForm });
    setModalOpen(true);
  };

  const openEdit = (pair: SyncPair) => {
    setEditingId(pair.id);
    setForm({
      name: pair.name || '',
      local_path: pair.local_path || '',
      remote_path: pair.remote_path || '',
      provider: pair.provider || '',
      mode: pair.mode || 'normal',
      direction: pair.direction || 'both',
      conflict_strategy: pair.conflict_strategy || 'latest_wins',
      include_patterns: pair.include_patterns || '',
      exclude_patterns: pair.exclude_patterns || '',
    });
    setModalOpen(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    try {
      if (editingId) {
        await updatePair(editingId, form);
        showToast(t('pairs.pairUpdated'), 'success');
      } else {
        await createPair(form);
        showToast(t('pairs.pairCreated'), 'success');
      }
      setModalOpen(false);
      await load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : t('pairs.operationFailed'), 'error');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm(t('pairs.confirmDelete'))) return;
    try {
      await deletePair(id);
      showToast(t('pairs.pairDeleted'), 'success');
      await load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : t('pairs.operationFailed'), 'error');
    }
  };

  const handleToggle = async (pair: SyncPair) => {
    try {
      await updatePair(pair.id, { enabled: !pair.enabled });
      showToast(pair.enabled ? t('pairs.pairDisabled') : t('pairs.pairEnabled'), 'success');
      await load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : t('pairs.operationFailed'), 'error');
    }
  };

  const field = (key: keyof PairForm, label: string, opts?: { type?: string; placeholder?: string }) => (
    <div style={{ display: 'grid', gap: 'var(--space-1)' }}>
      <label style={{ fontSize: 'var(--text-sm)', fontWeight: 500, color: 'var(--text-secondary)' }}>{label}</label>
      {key === 'provider' ? (
        <select
          value={form[key]}
          onChange={(e) => setForm((f) => ({ ...f, [key]: e.target.value }))}
          style={inputStyle}
        >
          <option value="">{t('pairs.selectProvider')}</option>
          {providers.map((p) => <option key={p.id} value={p.name}>{p.name}</option>)}
        </select>
      ) : key === 'mode' ? (
        <select value={form[key]} onChange={(e) => setForm((f) => ({ ...f, [key]: e.target.value }))} style={inputStyle}>
          <option value="normal">{t('pairs.normal')}</option>
          <option value="virtual">{t('pairs.virtual')}</option>
        </select>
      ) : key === 'direction' ? (
        <select value={form[key]} onChange={(e) => setForm((f) => ({ ...f, [key]: e.target.value }))} style={inputStyle}>
          <option value="both">{t('pairs.bidirectional')}</option>
          <option value="up">{t('pairs.uploadOnly')}</option>
          <option value="down">{t('pairs.downloadOnly')}</option>
        </select>
      ) : key === 'conflict_strategy' ? (
        <select value={form[key]} onChange={(e) => setForm((f) => ({ ...f, [key]: e.target.value }))} style={inputStyle}>
          <option value="latest_wins">{t('pairs.latestWins')}</option>
          <option value="local_wins">{t('pairs.localWins')}</option>
          <option value="remote_wins">{t('pairs.remoteWins')}</option>
          <option value="manual">{t('pairs.manual')}</option>
          <option value="skip">{t('pairs.skip')}</option>
        </select>
      ) : (
        <input
          type={opts?.type || 'text'}
          value={form[key]}
          onChange={(e) => setForm((f) => ({ ...f, [key]: e.target.value }))}
          placeholder={opts?.placeholder}
          style={inputStyle}
        />
      )}
    </div>
  );

  return (
    <div style={{ padding: 'var(--space-6)', maxWidth: '1000px', margin: '0 auto' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 'var(--space-6)' }}>
        <div>
          <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>{t('pairs.title')}</h1>
          <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
            {t('pairs.subtitle')}
          </p>
        </div>
        <button className="btn btn-primary" onClick={openCreate}>{t('pairs.newPair')}</button>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 'var(--space-12)', color: 'var(--text-secondary)' }}>{t('common.loading')}</div>
      ) : loadError ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--accent-red)' }}>
          <div style={{ marginBottom: 'var(--space-3)' }}>{t('pairs.loadFailed')}: {loadError}</div>
          <button className="btn" onClick={load}>{t('common.retry')}</button>
        </div>
      ) : pairs.length === 0 ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--text-tertiary)' }}>
          {t('pairs.noPairs')}
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          {pairs.map((pair) => {
            const progress = getProgress(pair.id);
            const isActivelySyncing = progress?.status === 'syncing';
            const progressPercent = progress && progress.filesTotal > 0
              ? Math.round((progress.filesSynced / progress.filesTotal) * 100)
              : 0;

            return (
              <div key={pair.id} className="card" style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', padding: 'var(--space-4) var(--space-5)', flexWrap: 'wrap' }}>
                {isActivelySyncing ? (
                  <SyncIcon size={20} color="var(--accent-green)" spinning />
                ) : (
                  <StatusIcon status={pair.status} size={20} />
                )}
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontWeight: 600, marginBottom: '2px' }}>{pair.name}</div>
                  <div style={{ fontSize: 'var(--text-xs)', color: 'var(--text-secondary)' }}>
                    {pair.local_path} <SyncIcon size={12} color="var(--accent-blue)" /> {pair.remote_path}
                  </div>
                  {isActivelySyncing && (
                    <div style={{ marginTop: '4px' }}>
                      <div style={{
                        width: '100%', height: '3px', background: 'var(--border-default)',
                        borderRadius: '2px', overflow: 'hidden',
                      }}>
                        <div style={{
                          width: `${progressPercent}%`, height: '100%', background: 'var(--accent-green)',
                          borderRadius: '2px', transition: 'width 0.3s ease',
                        }} />
                      </div>
                      <div style={{ fontSize: 'var(--text-xs)', color: 'var(--text-tertiary)', marginTop: '2px', display: 'flex', gap: 'var(--space-3)' }}>
                        <span>{progress.currentFile ? `📁 ${truncate(progress.currentFile, 50)}` : 'Processing...'}</span>
                        {progress.filesTotal > 0 && <span>{progress.filesSynced}/{progress.filesTotal} files</span>}
                        {progress.bytesTransferred > 0 && <span>{formatBytes(progress.bytesTransferred)}{progress.bytesTotal > 0 ? `/${formatBytes(progress.bytesTotal)}` : ''}</span>}
                      </div>
                    </div>
                  )}
                </div>
                <span className={`badge ${pair.enabled ? 'badge-green' : 'badge-blue'}`}>{pair.enabled ? t('common.enabled') : t('common.disabled')}</span>
                <span className="badge badge-blue">{t(getPairModeLabelKey(pair.mode))}</span>
                <span className="badge badge-blue">{t(getPairDirectionLabelKey(pair.direction))}</span>
                <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                  <button className="btn btn-sm btn-primary" onClick={() => handleSync(pair.id)} disabled={syncingId === pair.id || isActivelySyncing} style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
                    {syncingId === pair.id || isActivelySyncing ? <SyncIcon size={14} color="#fff" spinning /> : <PlayIcon size={14} color="#fff" />}
                    {t('dashboard.sync')}
                  </button>
                  <button className="btn btn-sm" onClick={() => openEdit(pair)} disabled={isActivelySyncing}>{t('common.edit')}</button>
                  <button className="btn btn-sm" onClick={() => handleToggle(pair)}>{pair.enabled ? t('pairs.disable') : t('pairs.enable')}</button>
                  <button className="btn btn-sm" style={{ color: 'var(--accent-red)' }} onClick={() => handleDelete(pair.id)} disabled={isActivelySyncing}>{t('common.delete')}</button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      <Modal open={modalOpen} onClose={() => setModalOpen(false)} title={editingId ? t('pairs.editPair') : t('pairs.newPairTitle')}>
        <form onSubmit={handleSubmit} style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--space-4)' }}>
          {field('name', t('common.name'), { placeholder: 'my-photos' })}
          {field('provider', t('pairs.provider'))}
          {field('local_path', t('pairs.localPath'), { placeholder: '/home/user/photos' })}
          {field('remote_path', t('pairs.remotePath'), { placeholder: '/photos' })}
          {field('direction', t('pairs.direction'))}
          {field('mode', t('pairs.mode'))}
          {field('conflict_strategy', t('pairs.conflictStrategy'))}
          <div />
          {field('include_patterns', t('pairs.includePatterns'), { placeholder: '*.md, docs/**' })}
          {field('exclude_patterns', t('pairs.excludePatterns'), { placeholder: '*.tmp, cache/**' })}
          <div style={{ gridColumn: '1 / -1', display: 'flex', gap: 'var(--space-2)', marginTop: 'var(--space-2)' }}>
            <button className="btn btn-primary" type="submit" disabled={submitting}>
              {submitting ? t('common.saving') : editingId ? t('pairs.saveChanges') : t('pairs.createPair')}
            </button>
            <button className="btn" type="button" onClick={() => setModalOpen(false)}>{t('common.cancel')}</button>
          </div>
        </form>
      </Modal>
    </div>
  );
};

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
