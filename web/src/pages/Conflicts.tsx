import React, { useEffect, useState } from 'react';
import { listConflicts, resolveConflict } from '../api/client';
import type { ConflictEntry } from '../api/client';
import { WarningIcon } from '../components/Icons';

export const Conflicts: React.FC = () => {
  const [conflicts, setConflicts] = useState<ConflictEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [resolving, setResolving] = useState<string | null>(null);

  useEffect(() => {
    listConflicts()
      .then(setConflicts)
      .catch(() => setConflicts([]))
      .finally(() => setLoading(false));
  }, []);

  const handleResolve = async (id: string, resolution: 'local' | 'remote') => {
    setResolving(id);
    try {
      await resolveConflict(id, resolution);
      setConflicts((prev) => prev.filter((c) => c.id !== id));
    } catch { /* ignore */ }
    finally { setResolving(null); }
  };

  return (
    <div style={{ padding: 'var(--space-6)', maxWidth: '1000px', margin: '0 auto' }}>
      <div style={{ marginBottom: 'var(--space-6)' }}>
        <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>Conflicts</h1>
        <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
          Resolve file synchronization conflicts
        </p>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 'var(--space-12)', color: 'var(--text-secondary)' }}>Loading...</div>
      ) : conflicts.length === 0 ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--text-tertiary)' }}>
          <WarningIcon size={32} color="var(--accent-green)" />
          <div style={{ marginTop: 'var(--space-3)' }}>No conflicts detected. All files are in sync.</div>
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
                  <div style={{ fontWeight: 500, color: 'var(--text-primary)' }}>Local</div>
                  <div>Modified: {new Date(c.local_modified).toLocaleString()}</div>
                  <div>Size: {c.local_size} bytes</div>
                </div>
                <div>
                  <div style={{ fontWeight: 500, color: 'var(--text-primary)' }}>Remote</div>
                  <div>Modified: {new Date(c.remote_modified).toLocaleString()}</div>
                  <div>Size: {c.remote_size} bytes</div>
                </div>
              </div>
              <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                <button
                  className="btn btn-sm btn-primary"
                  disabled={resolving === c.id}
                  onClick={() => handleResolve(c.id, 'local')}
                >
                  Keep Local
                </button>
                <button
                  className="btn btn-sm"
                  disabled={resolving === c.id}
                  onClick={() => handleResolve(c.id, 'remote')}
                >
                  Keep Remote
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
