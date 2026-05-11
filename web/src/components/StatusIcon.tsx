import React from 'react';
import type { SyncStatus } from '../api/client';
import { CheckIcon, SyncIcon, CloudIcon, CloseIcon, DashIcon } from './Icons';

interface StatusIconProps {
  status: SyncStatus;
  size?: number;
}

const statusConfig: Record<SyncStatus, { color: string; bg: string }> = {
  synced: { color: 'var(--accent-green)', bg: 'var(--accent-green-bg)' },
  syncing: { color: 'var(--accent-blue)', bg: 'var(--accent-blue-bg)' },
  virtual: { color: 'var(--accent-violet)', bg: 'var(--accent-violet-bg)' },
  conflict: { color: 'var(--accent-red)', bg: 'var(--accent-red-bg)' },
  excluded: { color: 'var(--text-tertiary)', bg: 'var(--bg-surface-hover)' },
  pending: { color: 'var(--accent-amber)', bg: 'var(--accent-amber-bg)' },
  error: { color: 'var(--accent-red)', bg: 'var(--accent-red-bg)' },
};

export const StatusIcon: React.FC<StatusIconProps> = ({ status, size = 16 }) => {
  const config = statusConfig[status] || statusConfig.pending;
  const iconSize = Math.max(size - 4, 10);

  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: size + 4,
        height: size + 4,
        borderRadius: '50%',
        background: config.bg,
        flexShrink: 0,
      }}
    >
      {status === 'synced' && <CheckIcon size={iconSize} color={config.color} />}
      {status === 'syncing' && <SyncIcon size={iconSize} color={config.color} spinning />}
      {status === 'virtual' && <CloudIcon size={iconSize} color={config.color} />}
      {status === 'conflict' && <CloseIcon size={iconSize} color={config.color} />}
      {status === 'excluded' && <DashIcon size={iconSize} color={config.color} />}
      {(status === 'pending' || status === 'error') && <DashIcon size={iconSize} color={config.color} />}
    </span>
  );
};
