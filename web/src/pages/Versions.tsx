import React from 'react';
import { ClockIcon } from '../components/Icons';

export const Versions: React.FC = () => {
  return (
    <div style={{ padding: 'var(--space-6)', maxWidth: '1000px', margin: '0 auto' }}>
      <div style={{ marginBottom: 'var(--space-6)' }}>
        <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>Version History</h1>
        <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
          View and restore previous file versions
        </p>
      </div>

      <div className="card" style={{ padding: 'var(--space-10)', textAlign: 'center', color: 'var(--text-tertiary)' }}>
        <ClockIcon size={32} color="var(--text-tertiary)" />
        <div style={{ marginTop: 'var(--space-3)' }}>
          Select a file from the File Browser to view its version history.
        </div>
      </div>
    </div>
  );
};
