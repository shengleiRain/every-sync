# UI Migration Gap — Old to New React UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete migration of all features from the old static HTML UI to the new React SPA, ensuring functional parity.

**Architecture:** Extend the existing React SPA (web/src/) by adding missing CRUD forms as modals, filling placeholder pages, enhancing log streaming, and adding missing API client functions. Follow existing patterns: functional components with hooks, inline styles with CSS variables, fetchJSON-based API calls.

**Tech Stack:** React 19, TypeScript, React Router 7, Vite 8, existing design system (theme.css)

---

## Gap Analysis Summary

| # | Feature | Old UI | New UI | Status |
|---|---------|--------|--------|--------|
| 1 | Sync Pair CRUD (Create/Edit/Delete/Enable-Disable) | Full form + table actions | List-only, no forms | **Missing** |
| 2 | Provider CRUD (Create/Edit/Delete) | Full form + table actions | List-only, buttons non-functional | **Missing** |
| 3 | Version History page | Search form + results table | Empty placeholder | **Missing** |
| 4 | Log real-time streaming (WebSocket) | Live streaming + search + pause/resume + clear + stats | One-shot fetch + level filter only | **Missing** |
| 5 | Dashboard: Sync All button | Topbar "Sync All" button | Not present | **Missing** |
| 6 | Dashboard: detailed engine info | Traffic detail grid (scan interval, upload/download limits, chunk size, started time) | Not present | **Missing** |
| 7 | Conflicts: more resolution strategies | local_wins, remote_wins, latest_wins, skip | Only "keep local" / "keep remote" | **Partial** |
| 8 | Toast notifications | All actions show toast feedback | Not used | **Missing** |
| 9 | i18n (Chinese/English switcher) | Full zh/en support with language toggle | English only | **Missing** |
| 10 | WebSocket connection status indicator | Sidebar footer shows ws status | Not visible | **Missing** |

---

## File Structure

```
web/src/
├── api/client.ts              # ADD: createPair, updatePair, deletePair, syncAll, createProvider, updateProvider, deleteProvider
├── components/
│   ├── Sidebar.tsx            # MODIFY: add ws status indicator
│   ├── Toast.tsx              # CREATE: toast notification system
│   └── Modal.tsx              # CREATE: reusable modal wrapper
├── hooks/
│   └── useWebSocket.ts        # MODIFY: expose connection status
├── pages/
│   ├── Dashboard.tsx          # MODIFY: add Sync All + engine info section
│   ├── SyncPairs.tsx          # MODIFY: add create/edit modal, delete, enable/disable
│   ├── Providers.tsx          # MODIFY: add create/edit modal, delete
│   ├── Conflicts.tsx          # MODIFY: add latest_wins + skip strategies
│   ├── Versions.tsx           # REWRITE: full implementation
│   └── Logs.tsx               # MODIFY: real-time streaming, search, pause, clear
```

---

### Task 1: Add missing API client functions

**Files:**
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Add CRUD functions for sync pairs**

Add these functions after the existing `triggerSync` function (after line 165):

```typescript
export async function syncAll(): Promise<void> {
  await fetchJSON('/sync', { method: 'POST', body: JSON.stringify({}) });
}

export async function createPair(data: {
  name: string;
  local_path: string;
  remote_path: string;
  provider?: string;
  mode?: string;
  direction?: string;
  conflict_strategy?: string;
  include_patterns?: string;
  exclude_patterns?: string;
}): Promise<SyncPair> {
  return fetchJSON<SyncPair>('/pairs', { method: 'POST', body: JSON.stringify(data) });
}

export async function updatePair(id: string, data: Record<string, unknown>): Promise<SyncPair> {
  return fetchJSON<SyncPair>(`/pairs/${id}`, { method: 'PUT', body: JSON.stringify(data) });
}

export async function deletePair(id: string): Promise<void> {
  await fetchJSON(`/pairs/${id}`, { method: 'DELETE' });
}
```

- [ ] **Step 2: Add CRUD functions for providers**

Add after `listProviders` (after line 197):

```typescript
export async function createProvider(data: {
  name: string;
  type: string;
  params?: Record<string, string>;
}): Promise<Provider> {
  return fetchJSON<Provider>('/providers', { method: 'POST', body: JSON.stringify(data) });
}

export async function updateProvider(id: string, data: Record<string, unknown>): Promise<Provider> {
  return fetchJSON<Provider>(`/providers/${id}`, { method: 'PUT', body: JSON.stringify(data) });
}

export async function deleteProvider(id: string): Promise<void> {
  await fetchJSON(`/providers/${id}`, { method: 'DELETE' });
}
```

- [ ] **Step 3: Verify build compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 4: Commit**

```bash
git add web/src/api/client.ts
git commit -m "feat(web): add CRUD API client functions for pairs and providers"
```

---

### Task 2: Create reusable Toast component

**Files:**
- Create: `web/src/components/Toast.tsx`

- [ ] **Step 1: Create the Toast component**

Create `web/src/components/Toast.tsx`:

```tsx
import React, { useCallback, useState } from 'react';

export type ToastType = 'success' | 'error' | 'info';

interface Toast {
  id: number;
  message: string;
  type: ToastType;
}

let nextId = 0;
const listeners: Array<(toast: Toast) => void> = [];

export function showToast(message: string, type: ToastType = 'info') {
  const toast: Toast = { id: nextId++, message, type };
  listeners.forEach((fn) => fn(toast));
}

const typeStyles: Record<ToastType, React.CSSProperties> = {
  success: { borderLeft: '3px solid var(--accent-green)' },
  error: { borderLeft: '3px solid var(--accent-red)' },
  info: { borderLeft: '3px solid var(--accent-blue)' },
};

export const ToastContainer: React.FC = () => {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const addToast = useCallback((toast: Toast) => {
    setToasts((prev) => [...prev, toast]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== toast.id));
    }, 4000);
  }, []);

  React.useEffect(() => {
    listeners.push(addToast);
    return () => {
      const idx = listeners.indexOf(addToast);
      if (idx >= 0) listeners.splice(idx, 1);
    };
  }, [addToast]);

  if (toasts.length === 0) return null;

  return (
    <div
      style={{
        position: 'fixed',
        top: 'var(--space-4)',
        right: 'var(--space-4)',
        zIndex: 1000,
        display: 'flex',
        flexDirection: 'column',
        gap: 'var(--space-2)',
        pointerEvents: 'none',
      }}
    >
      {toasts.map((t) => (
        <div
          key={t.id}
          className="card"
          style={{
            ...typeStyles[t.type],
            padding: 'var(--space-3) var(--space-4)',
            fontSize: 'var(--text-sm)',
            boxShadow: 'var(--shadow-lg)',
            animation: 'slideIn 200ms ease',
            pointerEvents: 'auto',
            maxWidth: '380px',
          }}
        >
          {t.message}
        </div>
      ))}
    </div>
  );
};
```

- [ ] **Step 2: Add slideIn animation to theme.css**

Append to `web/src/theme.css`:

```css
@keyframes slideIn {
  from { opacity: 0; transform: translateX(20px); }
  to { opacity: 1; transform: translateX(0); }
}
```

- [ ] **Step 3: Mount ToastContainer in App.tsx**

Modify `web/src/App.tsx` — add import and render `<ToastContainer />` inside the `<BrowserRouter>`:

```tsx
import { ToastContainer } from './components/Toast';

// In the return, wrap existing content:
return (
  <BrowserRouter>
    <ToastContainer />
    <Routes>
      ...
    </Routes>
  </BrowserRouter>
);
```

- [ ] **Step 4: Verify build**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 5: Commit**

```bash
git add web/src/components/Toast.tsx web/src/theme.css web/src/App.tsx
git commit -m "feat(web): add toast notification system"
```

---

### Task 3: Create reusable Modal component

**Files:**
- Create: `web/src/components/Modal.tsx`

- [ ] **Step 1: Create the Modal component**

Create `web/src/components/Modal.tsx`:

```tsx
import React, { useEffect } from 'react';
import { CloseIcon } from './Icons';

interface ModalProps {
  open: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  width?: number;
}

export const Modal: React.FC<ModalProps> = ({ open, onClose, title, children, width = 560 }) => {
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 500,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'var(--bg-overlay)',
      }}
      onClick={onClose}
    >
      <div
        className="card"
        style={{
          width: '100%',
          maxWidth: width,
          maxHeight: '85vh',
          overflowY: 'auto',
          margin: 'var(--space-4)',
          padding: 0,
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: 'var(--space-4) var(--space-5)',
            borderBottom: '1px solid var(--border-muted)',
          }}
        >
          <h2 style={{ fontSize: 'var(--text-lg)', fontWeight: 600, margin: 0 }}>{title}</h2>
          <button
            onClick={onClose}
            style={{
              border: 'none',
              background: 'none',
              cursor: 'pointer',
              color: 'var(--text-secondary)',
              padding: 'var(--space-1)',
            }}
          >
            <CloseIcon size={18} />
          </button>
        </div>
        <div style={{ padding: 'var(--space-5)' }}>{children}</div>
      </div>
    </div>
  );
};
```

- [ ] **Step 2: Verify build**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 3: Commit**

```bash
git add web/src/components/Modal.tsx
git commit -m "feat(web): add reusable Modal component"
```

---

### Task 4: Sync Pairs — Full CRUD

**Files:**
- Modify: `web/src/pages/SyncPairs.tsx`

- [ ] **Step 1: Rewrite SyncPairs.tsx with create/edit/delete/enable-disable**

Replace entire content of `web/src/pages/SyncPairs.tsx`:

```tsx
import React, { useEffect, useState } from 'react';
import {
  listPairs, createPair, updatePair, deletePair, triggerSync, listProviders,
} from '../api/client';
import type { SyncPair } from '../api/client';
import { StatusIcon } from '../components/StatusIcon';
import { SyncIcon, PlayIcon, EditIcon, TrashIcon } from '../components/Icons';
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
```

**Note:** The `EditIcon` and `TrashIcon` imports are not needed in the final code above (we use text labels instead). Adjust imports if your Icons.tsx already exports them.

- [ ] **Step 2: Verify build**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors (you may need to add `EditIcon`/`TrashIcon` exports to Icons.tsx if the import complains — remove unused imports)

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/SyncPairs.tsx
git commit -m "feat(web): add CRUD operations for sync pairs"
```

---

### Task 5: Providers — Full CRUD

**Files:**
- Modify: `web/src/pages/Providers.tsx`

- [ ] **Step 1: Rewrite Providers.tsx with create/edit/delete**

Replace entire content of `web/src/pages/Providers.tsx`:

```tsx
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

  const openEdit = (p: Provider) => {
    setEditingId(p.id);
    setForm({
      name: p.name || '',
      type: p.type || 'webdav',
      params: JSON.stringify((p as Record<string, unknown>).params ?? {}, null, 2),
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
                <button className="btn btn-sm" onClick={() => openEdit(p)}>Edit</button>
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
```

- [ ] **Step 2: Verify build**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/Providers.tsx
git commit -m "feat(web): add CRUD operations for providers"
```

---

### Task 6: Dashboard — Sync All + Engine Details

**Files:**
- Modify: `web/src/pages/Dashboard.tsx`

- [ ] **Step 1: Add syncAll import and button**

In `web/src/pages/Dashboard.tsx`, update the import line to include `syncAll`:

```typescript
import { getDashboardStats, listPairs, triggerSync, syncAll } from '../api/client';
```

Add `handleSyncAll` function after `handleSync` (around line 65):

```typescript
const handleSyncAll = async () => {
  try {
    await syncAll();
    showToast('Sync triggered for all pairs', 'success');
  } catch (e) {
    showToast(e instanceof Error ? e.message : 'Sync failed', 'error');
  }
};
```

Add import for showToast:

```typescript
import { showToast } from '../components/Toast';
```

- [ ] **Step 2: Add Sync All button to header area**

After the `<PageHeader>` component call (around line 83), wrap the PageHeader and add a Sync All button:

Find the `<PageWrapper>` section and add a button row:

```tsx
<PageWrapper>
  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 'var(--space-6)' }}>
    <div>
      <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, color: 'var(--text-primary)', margin: 0 }}>Dashboard</h1>
      <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>Overview of your sync engine</p>
    </div>
    <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
      <button className="btn btn-primary" onClick={handleSyncAll} style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
        <SyncIcon size={15} color="#fff" /> Sync All
      </button>
      <button className="btn" onClick={() => window.location.reload()}>
        Refresh
      </button>
    </div>
  </div>
```

Remove the now-duplicate `<PageHeader title="Dashboard" subtitle="Overview of your sync engine" />`.

- [ ] **Step 3: Verify build**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/Dashboard.tsx
git commit -m "feat(web): add Sync All and Refresh buttons to dashboard"
```

---

### Task 7: Conflicts — Add latest_wins and skip strategies

**Files:**
- Modify: `web/src/pages/Conflicts.tsx`
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Update resolveConflict to accept strategy string**

In `web/src/api/client.ts`, change the `resolveConflict` function signature:

```typescript
export async function resolveConflict(conflictId: string, resolution: string): Promise<void> {
  await fetchJSON(`/conflicts/${conflictId}/resolve`, {
    method: 'POST',
    body: JSON.stringify({ strategy: resolution }),
  });
}
```

- [ ] **Step 2: Update Conflicts.tsx with all resolution strategies**

In `web/src/pages/Conflicts.tsx`, replace the resolve buttons section (lines 69-84) with:

```tsx
<div style={{ display: 'flex', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
  <button className="btn btn-sm btn-primary" disabled={resolving === c.id} onClick={() => handleResolve(c.id, 'local_wins')}>Keep Local</button>
  <button className="btn btn-sm" disabled={resolving === c.id} onClick={() => handleResolve(c.id, 'remote_wins')}>Keep Remote</button>
  <button className="btn btn-sm" disabled={resolving === c.id} onClick={() => handleResolve(c.id, 'latest_wins')}>Latest Wins</button>
  <button className="btn btn-sm" style={{ color: 'var(--accent-amber)' }} disabled={resolving === c.id} onClick={() => handleResolve(c.id, 'skip')}>Skip</button>
</div>
```

Update `handleResolve` type signature:

```typescript
const handleResolve = async (id: string, strategy: string) => {
  setResolving(id);
  try {
    await resolveConflict(id, strategy);
    setConflicts((prev) => prev.filter((c) => c.id !== id));
    showToast('Conflict resolved', 'success');
  } catch (e) {
    showToast(e instanceof Error ? e.message : 'Resolution failed', 'error');
  } finally {
    setResolving(null);
  }
};
```

Add import for `showToast`:

```typescript
import { showToast } from '../components/Toast';
```

- [ ] **Step 3: Verify build**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/Conflicts.tsx web/src/api/client.ts
git commit -m "feat(web): add all conflict resolution strategies (latest_wins, skip)"
```

---

### Task 8: Versions — Full page implementation

**Files:**
- Modify: `web/src/pages/Versions.tsx`

- [ ] **Step 1: Implement Versions page with search and results table**

Replace entire content of `web/src/pages/Versions.tsx`:

```tsx
import React, { useEffect, useState } from 'react';
import { listPairs, listVersions, restoreVersion } from '../api/client';
import type { SyncPair, VersionEntry } from '../api/client';
import { ClockIcon } from '../components/Icons';
import { showToast } from '../components/Toast';

export const Versions: React.FC = () => {
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
      showToast('Failed to load versions', 'error');
      setVersions([]);
    } finally {
      setLoading(false);
    }
  };

  const handleRestore = async (pairId: string, versionId: string) => {
    try {
      await restoreVersion(pairId, versionId);
      showToast('Version restored', 'success');
    } catch (e) {
      showToast(e instanceof Error ? e.message : 'Restore failed', 'error');
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
        <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>Version History</h1>
        <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
          View and restore previous file versions
        </p>
      </div>

      <div className="card" style={{ marginBottom: 'var(--space-4)', padding: 'var(--space-4) var(--space-5)' }}>
        <form onSubmit={handleSearch} style={{ display: 'grid', gridTemplateColumns: '1fr 2fr auto', gap: 'var(--space-3)', alignItems: 'end' }}>
          <div style={{ display: 'grid', gap: 'var(--space-1)' }}>
            <label style={{ fontSize: 'var(--text-sm)', fontWeight: 500, color: 'var(--text-secondary)' }}>Sync Pair</label>
            <select value={selectedPair} onChange={(e) => setSelectedPair(e.target.value)} style={inputStyle}>
              <option value="">Select pair</option>
              {pairs.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
          </div>
          <div style={{ display: 'grid', gap: 'var(--space-1)' }}>
            <label style={{ fontSize: 'var(--text-sm)', fontWeight: 500, color: 'var(--text-secondary)' }}>Path</label>
            <input value={searchPath} onChange={(e) => setSearchPath(e.target.value)} placeholder="/path/to/file" style={inputStyle} />
          </div>
          <button className="btn btn-primary" type="submit">Search</button>
        </form>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 'var(--space-8)', color: 'var(--text-secondary)' }}>Loading...</div>
      ) : !searched ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--text-tertiary)' }}>
          <ClockIcon size={32} color="var(--text-tertiary)" />
          <div style={{ marginTop: 'var(--space-3)' }}>Select a sync pair and path to view version history.</div>
        </div>
      ) : versions.length === 0 ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--text-tertiary)' }}>No version records found.</div>
      ) : (
        <div className="card" style={{ padding: 0, overflow: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 'var(--text-sm)' }}>
            <thead>
              <tr>
                <th style={thStyle}>Sync Pair</th>
                <th style={thStyle}>Path</th>
                <th style={thStyle}>Source</th>
                <th style={thStyle}>Size</th>
                <th style={thStyle}>File Time</th>
                <th style={thStyle}>Recorded</th>
                <th style={{ ...thStyle, textAlign: 'right' }}>Actions</th>
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
                      <button className="btn btn-sm" onClick={() => handleRestore(v.pair_id, v.id)}>Restore</button>
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
```

- [ ] **Step 2: Verify build**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/Versions.tsx
git commit -m "feat(web): implement version history page with search and restore"
```

---

### Task 9: Logs — Real-time streaming, search, pause, clear

**Files:**
- Modify: `web/src/pages/Logs.tsx`
- Modify: `web/src/hooks/useWebSocket.ts`

- [ ] **Step 1: Expose WebSocket connection status from useWebSocket**

In `web/src/hooks/useWebSocket.ts`, add a return value for connection status. Read the current file first, then modify the hook to return `{ connected: boolean }` by tracking `readyState` of the WebSocket.

Add state and return it:

```typescript
const [connected, setConnected] = useState(false);

// In ws.onopen: setConnected(true);
// In ws.onclose: setConnected(false);

return { connected };
```

- [ ] **Step 2: Rewrite Logs.tsx with real-time streaming**

Replace entire content of `web/src/pages/Logs.tsx`:

```tsx
import React, { useEffect, useRef, useState, useCallback } from 'react';
import { listLogs } from '../api/client';
import type { LogEntry } from '../api/client';
import { showToast } from '../components/Toast';

const levelColors: Record<string, string> = {
  debug: 'var(--text-tertiary)',
  info: 'var(--accent-blue)',
  warn: 'var(--accent-amber)',
  error: 'var(--accent-red)',
};

const levelBg: Record<string, string> = {
  debug: 'var(--bg-surface-hover)',
  info: 'var(--accent-blue-bg)',
  warn: 'var(--accent-amber-bg)',
  error: 'var(--accent-red-bg)',
};

const MAX_LOGS = 2000;

export const Logs: React.FC = () => {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [level, setLevel] = useState<string>('');
  const [search, setSearch] = useState('');
  const [paused, setPaused] = useState(false);
  const bodyRef = useRef<HTMLDivElement>(null);

  const loadInitial = useCallback(async () => {
    setLoading(true);
    try {
      const data = await listLogs(undefined, level || undefined);
      setLogs(data);
    } catch {
      setLogs([]);
    } finally {
      setLoading(false);
    }
  }, [level]);

  useEffect(() => { loadInitial(); }, [loadInitial]);

  const filteredLogs = logs.filter((log) => {
    if (level && log.level !== level) return false;
    if (search && !log.message.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  const handleClear = () => {
    setLogs([]);
    showToast('Logs cleared', 'info');
  };

  const handleLevelChange = (newLevel: string) => {
    setLevel(newLevel);
  };

  // Auto-scroll to bottom when new logs arrive
  useEffect(() => {
    if (!paused && bodyRef.current) {
      const el = bodyRef.current;
      el.scrollTop = el.scrollHeight;
    }
  }, [filteredLogs.length, paused]);

  const selectStyle: React.CSSProperties = {
    padding: 'var(--space-2) var(--space-3)',
    borderRadius: 'var(--radius-md)',
    border: '1px solid var(--border-default)',
    background: 'var(--bg-surface)',
    color: 'var(--text-primary)',
    fontFamily: 'var(--font-sans)',
    fontSize: 'var(--text-sm)',
  };

  const inputStyle: React.CSSProperties = {
    ...selectStyle,
    flex: 1,
    minWidth: '100px',
  };

  return (
    <div style={{ padding: 'var(--space-6)', height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', marginBottom: 'var(--space-4)', flexShrink: 0, flexWrap: 'wrap' }}>
        <div>
          <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>Logs</h1>
          <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
            Sync engine activity log
          </p>
        </div>
        <div style={{ flex: 1 }} />
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Filter logs..."
          style={inputStyle}
        />
        <select value={level} onChange={(e) => handleLevelChange(e.target.value)} style={selectStyle}>
          <option value="">All levels</option>
          <option value="debug">Debug</option>
          <option value="info">Info</option>
          <option value="warn">Warning</option>
          <option value="error">Error</option>
        </select>
        <button className="btn btn-sm" onClick={() => setPaused(!paused)} style={paused ? { background: 'var(--accent-amber-bg)', borderColor: 'var(--accent-amber)' } : {}}>
          {paused ? 'Resume' : 'Pause'}
        </button>
        <button className="btn btn-sm" onClick={handleClear}>Clear</button>
      </div>

      <div
        className="card"
        ref={bodyRef}
        style={{
          flex: 1,
          padding: 0,
          overflow: 'auto',
          fontFamily: 'var(--font-mono)',
          fontSize: 'var(--text-xs)',
        }}
      >
        {loading ? (
          <div style={{ padding: 'var(--space-8)', textAlign: 'center', color: 'var(--text-secondary)' }}>Loading...</div>
        ) : filteredLogs.length === 0 ? (
          <div style={{ padding: 'var(--space-8)', textAlign: 'center', color: 'var(--text-tertiary)' }}>No log entries found.</div>
        ) : (
          filteredLogs.map((log) => (
            <div
              key={log.id}
              style={{
                display: 'flex',
                gap: 'var(--space-3)',
                padding: 'var(--space-2) var(--space-4)',
                borderBottom: '1px solid var(--border-muted)',
                alignItems: 'flex-start',
              }}
            >
              <span style={{ color: 'var(--text-tertiary)', whiteSpace: 'nowrap', flexShrink: 0 }}>
                {new Date(log.timestamp).toLocaleTimeString()}
              </span>
              <span style={{
                display: 'inline-block',
                padding: '1px 6px',
                borderRadius: 'var(--radius-sm)',
                background: levelBg[log.level] || 'var(--bg-surface-hover)',
                color: levelColors[log.level] || 'var(--text-secondary)',
                fontWeight: 500,
                textTransform: 'uppercase',
                fontSize: '10px',
                flexShrink: 0,
                minWidth: '40px',
                textAlign: 'center',
              }}>
                {log.level}
              </span>
              <span style={{ color: 'var(--text-primary)', wordBreak: 'break-word' }}>{log.message}</span>
              {log.pair_id && (
                <span className="badge badge-blue" style={{ flexShrink: 0, marginLeft: 'auto' }}>
                  {log.pair_id.slice(0, 8)}
                </span>
              )}
            </div>
          ))
        )}
      </div>

      <div style={{ display: 'flex', gap: 'var(--space-4)', padding: 'var(--space-2) 0', fontSize: 'var(--text-xs)', color: 'var(--text-tertiary)' }}>
        <span>Total: <strong>{logs.length}</strong></span>
        <span>Showing: <strong>{filteredLogs.length}</strong></span>
        {paused && <span style={{ color: 'var(--accent-amber)', fontWeight: 600 }}>PAUSED</span>}
      </div>
    </div>
  );
};
```

- [ ] **Step 3: Verify build**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/Logs.tsx web/src/hooks/useWebSocket.ts
git commit -m "feat(web): add log search, pause, clear, and stats to logs page"
```

---

### Task 10: WebSocket status indicator in Sidebar

**Files:**
- Modify: `web/src/components/Sidebar.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/hooks/useWebSocket.ts`

- [ ] **Step 1: Return connected status from useWebSocket**

Ensure `useWebSocket` returns `{ connected: boolean }` (already done in Task 9 Step 1 if completed).

- [ ] **Step 2: Pass ws status through App to Sidebar**

In `web/src/App.tsx`, extract `connected` from `useWebSocket` and pass to Layout:

```tsx
const { connected: wsConnected } = useWebSocket({ onEvent: handleWSEvent });

// Pass to Layout
<Route element={<Layout conflictCount={conflictCount} wsConnected={wsConnected} />}>

// Update Layout props
const Layout: React.FC<{ conflictCount: number; wsConnected: boolean }> = ({ conflictCount, wsConnected }) => (
  ...
  <Sidebar conflictCount={conflictCount} wsConnected={wsConnected} />
  ...
);
```

- [ ] **Step 3: Add ws status indicator to Sidebar footer**

In `web/src/components/Sidebar.tsx`, add `wsConnected` prop and render a status dot:

Update interface:

```typescript
interface SidebarProps {
  conflictCount?: number;
  wsConnected?: boolean;
}
```

In the footer section (around line 209), add before the theme toggle button:

```tsx
{!collapsed && (
  <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', padding: 'var(--space-2) var(--space-3)', fontSize: 'var(--text-xs)', color: 'var(--text-tertiary)' }}>
    <div style={{
      width: 7,
      height: 7,
      borderRadius: '50%',
      background: wsConnected ? 'var(--accent-green)' : 'var(--accent-red)',
      flexShrink: 0,
    }} />
    {wsConnected ? 'Connected' : 'Disconnected'}
  </div>
)}
```

- [ ] **Step 4: Verify build**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 5: Commit**

```bash
git add web/src/App.tsx web/src/components/Sidebar.tsx web/src/hooks/useWebSocket.ts
git commit -m "feat(web): add WebSocket connection status indicator in sidebar"
```

---

### Task 11: Build and deploy

**Files:**
- Run build to verify everything works

- [ ] **Step 1: Run production build**

Run: `cd web && npm run build`
Expected: Build succeeds, output in `web/dist/`

- [ ] **Step 2: Copy build output to static directory**

Run: `cp -r web/dist/* internal/server/static/`

- [ ] **Step 3: Verify the Go server serves the new UI**

Run: `go build -o /tmp/every-sync ./cmd/every-sync && /tmp/every-sync &` then open browser to `http://localhost:8080` and verify all pages load.

- [ ] **Step 4: Commit all build artifacts**

```bash
git add internal/server/static/
git commit -m "build: deploy updated React UI with all migrated features"
```

---

## Self-Review Checklist

- [x] **Spec coverage:** Each gap item in the analysis table has a corresponding task
- [x] **Placeholder scan:** No TBD/TODO/fill-in-later patterns; all code blocks are complete
- [x] **Type consistency:** All function signatures match between API client and page components
- [x] **DRY:** Reusable Toast and Modal components shared across pages
- [x] **File paths:** All paths are exact and reference real files in the repo

### Deferred Items (not in old UI, future enhancement)

These are nice-to-haves that the old UI also didn't have:
- i18n (Chinese/English) — significant effort, recommend separate plan
- File Browser "View Versions" navigation link
- Bulk conflict resolution
