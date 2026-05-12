import React, { useEffect, useState } from 'react';
import {
  listPairs, createPair, updatePair, deletePair, triggerSync, listProviders,
} from '../api/client';
import type { SyncPair } from '../api/client';
import { StatusIcon } from '../components/StatusIcon';
import { SyncIcon, PlayIcon } from '../components/Icons';
import { Modal } from '../components/Modal';
import { showToast } from '../components/Toast';

const emptyForm = {
  name: '',
  local_path: '',
  remote_path: '',
  provider: '',
  mode: 'mirror',
  direction: 'both',
  conflict_strategy: 'latest_wins',
  include_patterns: '',
  exclude_patterns: '',
};

type PairForm = typeof emptyForm;

export const SyncPairs: React.FC = () => {
  const [pairs, setPairs] = useState<SyncPair[]>([]);
  const [providers, setProviders] = useState<{ id: string; name: string }[]>([]);
  const [loading, setLoading] = useState(true);
  const [syncingId, setSyncingId] = useState<string | null>(null);

  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<PairForm>({ ...emptyForm });
  const [submitting, setSubmitting] = useState(false);

  const load = async () => {
    try {
      const [p, prov] = await Promise.all([listPairs(), listProviders()]);
      setPairs(p);
      setProviders(prov.map((x) => ({ id: x.id, name: x.name })));
    } catch {
      showToast('Failed to load pairs', 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const handleSync = async (id: string) => {
    setSyncingId(id);
    try {
      await triggerSync(id);
      showToast('Sync triggered', 'success');
    } catch (e) {
      showToast(e instanceof Error ? e.message : 'Sync failed', 'error');
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
      provider: pair.remote_provider || '',
      mode: pair.mode || 'mirror',
      direction: pair.direction || 'both',
      conflict_strategy: 'latest_wins',
      include_patterns: '',
      exclude_patterns: '',
    });
    setModalOpen(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    try {
      if (editingId) {
        await updatePair(editingId, form);
        showToast('Pair updated', 'success');
      } else {
        await createPair(form);
        showToast('Pair created', 'success');
      }
      setModalOpen(false);
      await load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : 'Operation failed', 'error');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this sync pair?')) return;
    try {
      await deletePair(id);
      showToast('Pair deleted', 'success');
      await load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : 'Delete failed', 'error');
    }
  };

  const handleToggle = async (pair: SyncPair) => {
    try {
      await updatePair(pair.id, { enabled: !pair.enabled });
      showToast(pair.enabled ? 'Pair disabled' : 'Pair enabled', 'success');
      await load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : 'Toggle failed', 'error');
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
          <option value="">Select provider</option>
          {providers.map((p) => <option key={p.id} value={p.name}>{p.name}</option>)}
        </select>
      ) : key === 'mode' ? (
        <select value={form[key]} onChange={(e) => setForm((f) => ({ ...f, [key]: e.target.value }))} style={inputStyle}>
          <option value="mirror">Mirror</option>
          <option value="selective">Selective</option>
          <option value="virtual">Virtual</option>
        </select>
      ) : key === 'direction' ? (
        <select value={form[key]} onChange={(e) => setForm((f) => ({ ...f, [key]: e.target.value }))} style={inputStyle}>
          <option value="both">Bidirectional</option>
          <option value="up">Upload Only</option>
          <option value="down">Download Only</option>
        </select>
      ) : key === 'conflict_strategy' ? (
        <select value={form[key]} onChange={(e) => setForm((f) => ({ ...f, [key]: e.target.value }))} style={inputStyle}>
          <option value="latest_wins">Latest Wins</option>
          <option value="local_wins">Local Wins</option>
          <option value="remote_wins">Remote Wins</option>
          <option value="manual">Manual</option>
          <option value="skip">Skip</option>
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
          <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>Sync Pairs</h1>
          <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
            Configure and manage your sync pairs
          </p>
        </div>
        <button className="btn btn-primary" onClick={openCreate}>+ New Pair</button>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 'var(--space-12)', color: 'var(--text-secondary)' }}>Loading...</div>
      ) : pairs.length === 0 ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--text-tertiary)' }}>
          No sync pairs configured. Click "+ New Pair" to create one.
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          {pairs.map((pair) => (
            <div key={pair.id} className="card" style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', padding: 'var(--space-4) var(--space-5)' }}>
              <StatusIcon status={pair.status} size={20} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontWeight: 600, marginBottom: '2px' }}>{pair.name}</div>
                <div style={{ fontSize: 'var(--text-xs)', color: 'var(--text-secondary)' }}>
                  {pair.local_path} <SyncIcon size={12} color="var(--accent-blue)" /> {pair.remote_path}
                </div>
              </div>
              <span className={`badge ${pair.enabled ? 'badge-green' : 'badge-blue'}`}>{pair.enabled ? 'Enabled' : 'Disabled'}</span>
              <span className="badge badge-blue">{pair.mode}</span>
              <span className="badge badge-blue">{pair.direction}</span>
              <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                <button className="btn btn-sm btn-primary" onClick={() => handleSync(pair.id)} disabled={syncingId === pair.id} style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
                  {syncingId === pair.id ? <SyncIcon size={14} color="#fff" spinning /> : <PlayIcon size={14} color="#fff" />}
                  Sync
                </button>
                <button className="btn btn-sm" onClick={() => openEdit(pair)}>Edit</button>
                <button className="btn btn-sm" onClick={() => handleToggle(pair)}>{pair.enabled ? 'Disable' : 'Enable'}</button>
                <button className="btn btn-sm" style={{ color: 'var(--accent-red)' }} onClick={() => handleDelete(pair.id)}>Delete</button>
              </div>
            </div>
          ))}
        </div>
      )}

      <Modal open={modalOpen} onClose={() => setModalOpen(false)} title={editingId ? 'Edit Sync Pair' : 'New Sync Pair'}>
        <form onSubmit={handleSubmit} style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--space-4)' }}>
          {field('name', 'Name', { placeholder: 'my-photos' })}
          {field('provider', 'Provider')}
          {field('local_path', 'Local Path', { placeholder: '/home/user/photos' })}
          {field('remote_path', 'Remote Path', { placeholder: '/photos' })}
          {field('direction', 'Direction')}
          {field('mode', 'Mode')}
          {field('conflict_strategy', 'Conflict Strategy')}
          <div />
          {field('include_patterns', 'Include Patterns', { placeholder: '*.md, docs/**' })}
          {field('exclude_patterns', 'Exclude Patterns', { placeholder: '*.tmp, cache/**' })}
          <div style={{ gridColumn: '1 / -1', display: 'flex', gap: 'var(--space-2)', marginTop: 'var(--space-2)' }}>
            <button className="btn btn-primary" type="submit" disabled={submitting}>
              {submitting ? 'Saving...' : editingId ? 'Save Changes' : 'Create Pair'}
            </button>
            <button className="btn" type="button" onClick={() => setModalOpen(false)}>Cancel</button>
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
