import React from 'react';
import { ChevronRightIcon, FolderIcon } from './Icons';

interface BreadcrumbProps {
  path: string;
  onNavigate: (path: string) => void;
}

export const Breadcrumb: React.FC<BreadcrumbProps> = ({ path, onNavigate }) => {
  const segments = path.split('/').filter(Boolean);
  const items = [{ name: 'Root', path: '/' }];

  let current = '';
  for (const seg of segments) {
    current += '/' + seg;
    items.push({ name: seg, path: current });
  }

  return (
    <nav
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '2px',
        fontSize: 'var(--text-sm)',
        color: 'var(--text-secondary)',
        padding: 'var(--space-2) 0',
        overflow: 'hidden',
        flexShrink: 0,
      }}
    >
      {items.map((item, idx) => (
        <React.Fragment key={item.path}>
          {idx > 0 && (
            <ChevronRightIcon
              size={14}
              color="var(--text-tertiary)"
            />
          )}
          <button
            onClick={() => onNavigate(item.path)}
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: '4px',
              padding: '2px 6px',
              border: 'none',
              borderRadius: 'var(--radius-sm)',
              background: 'transparent',
              color: idx === items.length - 1 ? 'var(--text-primary)' : 'var(--text-secondary)',
              fontWeight: idx === items.length - 1 ? 600 : 400,
              cursor: 'pointer',
              fontFamily: 'inherit',
              fontSize: 'inherit',
              whiteSpace: 'nowrap',
              transition: 'background var(--transition-fast)',
            }}
            onMouseEnter={(e) => {
              (e.target as HTMLElement).style.background = 'var(--bg-surface-hover)';
            }}
            onMouseLeave={(e) => {
              (e.target as HTMLElement).style.background = 'transparent';
            }}
          >
            {idx === 0 && <FolderIcon size={14} color="var(--accent-amber)" />}
            {item.name}
          </button>
        </React.Fragment>
      ))}
    </nav>
  );
};
