import React, { useEffect, useState } from 'react';
import { listProviders } from '../api/client';
import type { Provider } from '../api/client';
import { GearIcon, CheckIcon, CloseIcon } from '../components/Icons';

export const Providers: React.FC = () => {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    listProviders()
      .then(setProviders)
      .catch(() => setProviders([]))
      .finally(() => setLoading(false));
  }, []);

  return (
    <div style={{ padding: 'var(--space-6)', maxWidth: '800px', margin: '0 auto' }}>
      <div style={{ marginBottom: 'var(--space-6)' }}>
        <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>Providers</h1>
        <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
          Configure cloud storage providers
        </p>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 'var(--space-12)', color: 'var(--text-secondary)' }}>Loading...</div>
      ) : providers.length === 0 ? (
        <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--text-tertiary)' }}>
          No providers configured yet.
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          {providers.map((p) => (
            <div
              key={p.id}
              className="card"
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 'var(--space-4)',
                padding: 'var(--space-4) var(--space-5)',
              }}
            >
              <GearIcon size={20} color={p.configured ? 'var(--accent-green)' : 'var(--text-tertiary)'} />
              <div style={{ flex: 1 }}>
                <div style={{ fontWeight: 600 }}>{p.name}</div>
                <div style={{ fontSize: 'var(--text-xs)', color: 'var(--text-secondary)', textTransform: 'capitalize' }}>
                  {p.type}
                </div>
              </div>
              {p.configured ? (
                <span className="badge badge-green">
                  <CheckIcon size={12} color="var(--accent-green)" /> Configured
                </span>
              ) : (
                <span className="badge badge-amber">
                  <CloseIcon size={12} color="var(--accent-amber)" /> Not configured
                </span>
              )}
              <button className="btn btn-sm">
                {p.configured ? 'Configure' : 'Set up'}
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
