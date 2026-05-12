import React, { useCallback, useEffect, useState } from 'react';
import { listProviders, createProvider, updateProvider, deleteProvider, deleteProviderForce, testProvider } from '../api/client';
import type { Provider } from '../api/client';
import { GearIcon, CheckIcon, CloseIcon } from '../components/Icons';
import { Modal } from '../components/Modal';
import { showToast } from '../components/Toast';
import { useI18n } from '../i18n';

interface ProviderForm {
  name: string;
  type: string;
  params: string;
}

const providerTemplates: Record<string, string> = {
  webdav: JSON.stringify({
    endpoint: "https://webdav.example.com/dav",
    username: "",
    password: "",
    prefix: "",
    timeout: "",
    auth_mode: "basic",
  }, null, 2),
  local: JSON.stringify({
    root_path: "/path/to/directory",
  }, null, 2),
};

const emptyForm: ProviderForm = {
  name: '',
  type: 'webdav',
  params: providerTemplates.webdav,
};

interface ProviderWithParams extends Provider {
  params?: Record<string, string>;
}

export const Providers: React.FC = () => {
  const { t } = useI18n();
  const [providers, setProviders] = useState<Provider[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<ProviderForm>({ ...emptyForm });
  const [submitting, setSubmitting] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; message: string } | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      setProviders(await listProviders());
    } catch (e) {
      const message = e instanceof Error ? e.message : t('providers.loadFailed');
      setLoadError(message);
      setProviders([]);
      showToast(message, 'error');
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => { load(); }, [load]);

  const openCreate = () => {
    setEditingId(null);
    setForm({ ...emptyForm });
    setTestResult(null);
    setModalOpen(true);
  };

  const openEdit = (p: ProviderWithParams) => {
    setEditingId(p.id);
    setForm({
      name: p.name || '',
      type: p.type || 'webdav',
      params: JSON.stringify(p.params ?? {}, null, 2),
    });
    setTestResult(null);
    setModalOpen(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    try {
      const params = JSON.parse(form.params || '{}');
      if (!params || Array.isArray(params) || typeof params !== 'object') {
        throw new Error(t('providers.invalidParams'));
      }
      if (editingId) {
        await updateProvider(editingId, { name: form.name, type: form.type, params });
        showToast(t('providers.providerUpdated'), 'success');
      } else {
        await createProvider({ name: form.name, type: form.type, params });
        showToast(t('providers.providerCreated'), 'success');
      }
      setModalOpen(false);
      await load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : t('pairs.operationFailed'), 'error');
    } finally {
      setSubmitting(false);
    }
  };

  const handleTestConnection = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const params = JSON.parse(form.params || '{}');
      const result = await testProvider({ type: form.type, params });
      setTestResult({
        ok: result.status === 'ok',
        message: result.status === 'ok' ? t('providers.testSuccess') : (result.error || t('providers.testFailed')),
      });
    } catch (e) {
      setTestResult({
        ok: false,
        message: e instanceof Error ? e.message : t('providers.testFailed'),
      });
    } finally {
      setTesting(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm(t('providers.confirmDelete'))) return;
    try {
      await deleteProvider(id);
      showToast(t('providers.providerDeleted'), 'success');
      await load();
    } catch (e) {
      const errorMsg = e instanceof Error ? e.message : '';

      // Check if it's a 409 conflict (has dependent pairs)
      if (errorMsg.includes('409')) {
        let dependentPairs: Array<{ id: number; name: string }> = [];
        try {
          const jsonStr = errorMsg.replace(/^API\s*409:\s*/i, '');
          const parsed = JSON.parse(jsonStr);
          dependentPairs = parsed.pairs || [];
        } catch { /* ignore parse error */ }

        const pairList = dependentPairs.length > 0
          ? dependentPairs.map(p => `"${p.name}"`).join(', ')
          : 'sync pairs';

        if (!confirm(
          t('providers.confirmCascadeDelete', { pairs: pairList })
        )) return;

        try {
          await deleteProviderForce(id);
          showToast(t('providers.providerAndPairsDeleted'), 'success');
          await load();
        } catch (e2) {
          showToast(e2 instanceof Error ? e2.message : t('providers.deleteFailed'), 'error');
        }
      } else {
        showToast(errorMsg || t('providers.deleteFailed'), 'error');
      }
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
          <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>{t('providers.title')}</h1>
          <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
            {t('providers.subtitle')}
          </p>
        </div>
        <button className="btn btn-primary" onClick={openCreate}>{t('providers.newProvider')}</button>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 'var(--space-12)', color: 'var(--text-secondary)' }}>{t('common.loading')}</div>
      ) : loadError ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--accent-red)' }}>
          <div style={{ marginBottom: 'var(--space-3)' }}>{t('providers.loadFailed')}: {loadError}</div>
          <button className="btn" onClick={load}>{t('common.retry')}</button>
        </div>
      ) : providers.length === 0 ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--text-tertiary)' }}>
          {t('providers.noProviders')}
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          {providers.map((p) => (
            <div key={p.id} className="card" style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', padding: 'var(--space-4) var(--space-5)', flexWrap: 'wrap' }}>
              <GearIcon size={20} color={p.configured ? 'var(--accent-green)' : 'var(--text-tertiary)'} />
              <div style={{ flex: 1 }}>
                <div style={{ fontWeight: 600 }}>{p.name}</div>
                <div style={{ fontSize: 'var(--text-xs)', color: 'var(--text-secondary)', textTransform: 'capitalize' }}>{p.type}</div>
              </div>
              {p.configured ? (
                <span className="badge badge-green"><CheckIcon size={12} color="var(--accent-green)" /> {t('providers.configured')}</span>
              ) : (
                <span className="badge badge-amber"><CloseIcon size={12} color="var(--accent-amber)" /> {t('providers.notConfigured')}</span>
              )}
              <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                <button className="btn btn-sm" onClick={() => openEdit(p as ProviderWithParams)}>{t('common.edit')}</button>
                <button className="btn btn-sm" style={{ color: 'var(--accent-red)' }} onClick={() => handleDelete(p.id)}>{t('common.delete')}</button>
              </div>
            </div>
          ))}
        </div>
      )}

      <Modal open={modalOpen} onClose={() => setModalOpen(false)} title={editingId ? t('providers.editProvider') : t('providers.newProviderTitle')}>
        <form onSubmit={handleSubmit} style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--space-4)' }}>
          <div style={{ display: 'grid', gap: 'var(--space-1)' }}>
            <label style={{ fontSize: 'var(--text-sm)', fontWeight: 500, color: 'var(--text-secondary)' }}>{t('common.name')}</label>
            <input value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} placeholder="my-webdav" required style={inputStyle} />
          </div>
          <div style={{ display: 'grid', gap: 'var(--space-1)' }}>
            <label style={{ fontSize: 'var(--text-sm)', fontWeight: 500, color: 'var(--text-secondary)' }}>{t('providers.type')}</label>
            <select
              value={form.type}
              onChange={(e) => {
                const newType = e.target.value;
                if (!editingId && providerTemplates[newType]) {
                  setForm(f => ({ ...f, type: newType, params: providerTemplates[newType] }));
                } else {
                  setForm(f => ({ ...f, type: newType }));
                }
              }}
              style={inputStyle}
            >
              <option value="webdav">WebDAV</option>
              <option value="local">Local</option>
            </select>
          </div>
          <div style={{ gridColumn: '1 / -1', display: 'grid', gap: 'var(--space-1)' }}>
            <label style={{ fontSize: 'var(--text-sm)', fontWeight: 500, color: 'var(--text-secondary)' }}>
              {t('providers.params')}
              {form.type && t(`providers.paramsHint.${form.type}`) && (
                <span style={{ fontWeight: 400, color: 'var(--text-tertiary)', marginLeft: 'var(--space-2)' }}>
                  ({t(`providers.paramsHint.${form.type}`)})
                </span>
              )}
            </label>
            <textarea
              value={form.params}
              onChange={(e) => setForm((f) => ({ ...f, params: e.target.value }))}
              spellCheck={false}
              style={{ ...inputStyle, minHeight: '120px', fontFamily: 'var(--font-mono)', fontSize: 'var(--text-xs)' }}
            />
          </div>
          <div style={{ gridColumn: '1 / -1', display: 'flex', gap: 'var(--space-2)', marginTop: 'var(--space-2)', alignItems: 'center', flexWrap: 'wrap' }}>
            <button
              className="btn btn-sm"
              type="button"
              disabled={testing || !form.params}
              onClick={handleTestConnection}
            >
              {testing ? t('providers.testing') : t('providers.testConnection')}
            </button>
            {testResult && (
              <span style={{
                fontSize: 'var(--text-sm)',
                color: testResult.ok ? 'var(--accent-green)' : 'var(--accent-red)',
              }}>
                {testResult.ok ? '✓' : '✗'} {testResult.message}
              </span>
            )}
            <div style={{ flex: 1 }} />
            <button className="btn btn-primary" type="submit" disabled={submitting}>
              {submitting ? t('common.saving') : editingId ? t('providers.saveChanges') : t('providers.createProvider')}
            </button>
            <button className="btn" type="button" onClick={() => setModalOpen(false)}>{t('common.cancel')}</button>
          </div>
        </form>
      </Modal>
    </div>
  );
};
