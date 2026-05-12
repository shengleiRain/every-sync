import React, { useEffect, useState } from 'react';
import { listProviders, createProvider, updateProvider, deleteProvider } from '../api/client';
import type { Provider } from '../api/client';
import { GearIcon, CheckIcon, CloseIcon } from '../components/Icons';
import { Modal } from '../components/Modal';
import { showToast } from '../components/Toast';

interface ProviderForm {
  name: string;
  type: string;
  params: string;
}

const emptyForm: ProviderForm = {
  name: '',
  type: 'webdav',
  params: '{\n  "endpoint": "https://webdav.example.com/dav",\n  "username": "",\n  "password": ""\n}',
};

interface ProviderWithParams extends Provider {
  params?: Record<string, string>;
}

export const Providers: React.FC = () => {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [loading, setLoading] = useState(true);

  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<ProviderForm>({ ...emptyForm });
  const [submitting, setSubmitting] = useState(false);

  const load = async () => {
    try {
      setProviders(await listProviders());
    } catch {
      showToast('Failed to load providers', 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const openCreate = () => {
    setEditingId(null);
    setForm({ ...emptyForm });
    setModalOpen(true);
  };

  const openEdit = (p: ProviderWithParams) => {
    setEditingId(p.id);
    setForm({
      name: p.name || '',
      type: p.type || 'webdav',
      params: JSON.stringify(p.params ?? {}, null, 2),
    });
    setModalOpen(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    try {
      const params = JSON.parse(form.params || '{}');
      if (editingId) {
        await updateProvider(editingId, { name: form.name, type: form.type, params });
        showToast('Provider updated', 'success');
      } else {
        await createProvider({ name: form.name, type: form.type, params });
        showToast('Provider created', 'success');
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
    if (!confirm('Delete this provider?')) return;
    try {
      await deleteProvider(id);
      showToast('Provider deleted', 'success');
      await load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : 'Delete failed', 'error');
    }
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

  return (
    <div style={{ padding: 'var(--space-6)', maxWidth: '800px', margin: '0 auto' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 'var(--space-6)' }}>
        <div>
          <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>Providers</h1>
          <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
            Configure cloud storage providers
          </p>
        </div>
        <button className="btn btn-primary" onClick={openCreate}>+ New Provider</button>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 'var(--space-12)', color: 'var(--text-secondary)' }}>Loading...</div>
      ) : providers.length === 0 ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--text-tertiary)' }}>
          No providers configured. Click "+ New Provider" to add one.
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          {providers.map((p) => (
            <div key={p.id} className="card" style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', padding: 'var(--space-4) var(--space-5)' }}>
              <GearIcon size={20} color={p.configured ? 'var(--accent-green)' : 'var(--text-tertiary)'} />
              <div style={{ flex: 1 }}>
                <div style={{ fontWeight: 600 }}>{p.name}</div>
                <div style={{ fontSize: 'var(--text-xs)', color: 'var(--text-secondary)', textTransform: 'capitalize' }}>{p.type}</div>
              </div>
              {p.configured ? (
                <span className="badge badge-green"><CheckIcon size={12} color="var(--accent-green)" /> Configured</span>
              ) : (
                <span className="badge badge-amber"><CloseIcon size={12} color="var(--accent-amber)" /> Not configured</span>
              )}
              <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                <button className="btn btn-sm" onClick={() => openEdit(p as ProviderWithParams)}>Edit</button>
                <button className="btn btn-sm" style={{ color: 'var(--accent-red)' }} onClick={() => handleDelete(p.id)}>Delete</button>
              </div>
            </div>
          ))}
        </div>
      )}

      <Modal open={modalOpen} onClose={() => setModalOpen(false)} title={editingId ? 'Edit Provider' : 'New Provider'}>
        <form onSubmit={handleSubmit} style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--space-4)' }}>
          <div style={{ display: 'grid', gap: 'var(--space-1)' }}>
            <label style={{ fontSize: 'var(--text-sm)', fontWeight: 500, color: 'var(--text-secondary)' }}>Name</label>
            <input value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} placeholder="my-webdav" required style={inputStyle} />
          </div>
          <div style={{ display: 'grid', gap: 'var(--space-1)' }}>
            <label style={{ fontSize: 'var(--text-sm)', fontWeight: 500, color: 'var(--text-secondary)' }}>Type</label>
            <select value={form.type} onChange={(e) => setForm((f) => ({ ...f, type: e.target.value }))} style={inputStyle}>
              <option value="webdav">WebDAV</option>
              <option value="local">Local</option>
            </select>
          </div>
          <div style={{ gridColumn: '1 / -1', display: 'grid', gap: 'var(--space-1)' }}>
            <label style={{ fontSize: 'var(--text-sm)', fontWeight: 500, color: 'var(--text-secondary)' }}>Parameters (JSON)</label>
            <textarea
              value={form.params}
              onChange={(e) => setForm((f) => ({ ...f, params: e.target.value }))}
              spellCheck={false}
              style={{ ...inputStyle, minHeight: '120px', fontFamily: 'var(--font-mono)', fontSize: 'var(--text-xs)' }}
            />
          </div>
          <div style={{ gridColumn: '1 / -1', display: 'flex', gap: 'var(--space-2)', marginTop: 'var(--space-2)' }}>
            <button className="btn btn-primary" type="submit" disabled={submitting}>
              {submitting ? 'Saving...' : editingId ? 'Save Changes' : 'Create Provider'}
            </button>
            <button className="btn" type="button" onClick={() => setModalOpen(false)}>Cancel</button>
          </div>
        </form>
      </Modal>
    </div>
  );
};
