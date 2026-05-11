import React, { useEffect, useState } from 'react';
import { listPairs, triggerSync } from '../api/client';
import type { SyncPair } from '../api/client';
import { StatusIcon } from '../components/StatusIcon';
import { SyncIcon, PlayIcon } from '../components/Icons';

export const SyncPairs: React.FC = () => {
  const [pairs, setPairs] = useState<SyncPair[]>([]);
  const [loading, setLoading] = useState(true);
  const [syncingId, setSyncingId] = useState<string | null>(null);

  useEffect(() => {
    listPairs()
      .then(setPairs)
      .catch(() => setPairs([]))
      .finally(() => setLoading(false));
  }, []);

  const handleSync = async (id: string) => {
    setSyncingId(id);
    try {
      await triggerSync(id);
    } catch { /* ignore */ }
    finally { setSyncingId(null); }
  };

  return (
    <div style={{ padding: 'var(--space-6)', maxWidth: '1000px', margin: '0 auto' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 'var(--space-6)' }}>
        <div>
          <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>Sync Pairs</h1>
          <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
            Configure and manage your sync pairs
          </p>
        </div>
        <button className="btn btn-primary">+ New Pair</button>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 'var(--space-12)', color: 'var(--text-secondary)' }}>
          Loading...
        </div>
      ) : pairs.length === 0 ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--text-tertiary)' }}>
          No sync pairs configured. Click "New Pair" to create one.
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          {pairs.map((pair) => (
            <div
              key={pair.id}
              className="card"
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 'var(--space-4)',
                padding: 'var(--space-4) var(--space-5)',
              }}
            >
              <StatusIcon status={pair.status} size={20} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontWeight: 600, marginBottom: '2px' }}>{pair.name}</div>
                <div style={{ fontSize: 'var(--text-xs)', color: 'var(--text-secondary)' }}>
                  {pair.local_path} <SyncIcon size={12} color="var(--accent-blue)" /> {pair.remote_path}
                </div>
              </div>
              <span className="badge badge-blue">{pair.mode}</span>
              <span className="badge badge-blue">{pair.direction}</span>
              <button
                className="btn btn-sm btn-primary"
                onClick={() => handleSync(pair.id)}
                disabled={syncingId === pair.id}
                style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}
              >
                {syncingId === pair.id ? <SyncIcon size={14} color="#fff" spinning /> : <PlayIcon size={14} color="#fff" />}
                Sync
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
