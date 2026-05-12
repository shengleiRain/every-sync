import React, { useState } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import {
  GridIcon,
  FolderIcon,
  LayersIcon,
  GearIcon,
  WarningIcon,
  ClockIcon,
  DocumentIcon,
  MoonIcon,
  SunIcon,
  ChevronRightIcon,
} from './Icons';

interface SidebarProps {
  conflictCount?: number;
  wsConnected?: boolean;
}

interface NavItem {
  to: string;
  label: string;
  icon: React.FC<{ size?: number; color?: string; className?: string }>;
}

const navItems: NavItem[] = [
  { to: '/', label: 'Dashboard', icon: GridIcon },
  { to: '/files', label: 'File Browser', icon: FolderIcon },
  { to: '/pairs', label: 'Sync Pairs', icon: LayersIcon },
  { to: '/providers', label: 'Providers', icon: GearIcon },
  { to: '/conflicts', label: 'Conflicts', icon: WarningIcon },
  { to: '/versions', label: 'Versions', icon: ClockIcon },
  { to: '/logs', label: 'Logs', icon: DocumentIcon },
];

export const Sidebar: React.FC<SidebarProps> = ({ conflictCount = 0, wsConnected = false }) => {
  const location = useLocation();
  const [collapsed, setCollapsed] = useState(false);
  const [darkMode, setDarkMode] = useState(() => {
    if (typeof window === 'undefined') return false;
    return document.documentElement.getAttribute('data-theme') === 'dark' ||
      (!document.documentElement.getAttribute('data-theme') &&
        window.matchMedia('(prefers-color-scheme: dark)').matches);
  });

  const toggleTheme = () => {
    const next = !darkMode;
    setDarkMode(next);
    document.documentElement.setAttribute('data-theme', next ? 'dark' : 'light');
  };

  return (
    <>
      {/* Mobile overlay */}
      <div
        className="sidebar-overlay"
        style={{
          display: 'none',
          position: 'fixed',
          inset: 0,
          background: 'var(--bg-overlay)',
          zIndex: 200,
        }}
      />
      <aside
        style={{
          display: 'flex',
          flexDirection: 'column',
          width: collapsed ? 'var(--sidebar-collapsed-width)' : 'var(--sidebar-width)',
          height: '100vh',
          background: 'var(--bg-sidebar)',
          borderRight: '1px solid var(--border-default)',
          flexShrink: 0,
          transition: 'width var(--transition-normal)',
          overflow: 'hidden',
          position: 'relative',
          zIndex: 210,
        }}
      >
        {/* Logo area */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 'var(--space-3)',
            padding: 'var(--space-4) var(--space-4)',
            borderBottom: '1px solid var(--border-muted)',
            minHeight: '52px',
          }}
        >
          <div
            style={{
              width: '28px',
              height: '28px',
              borderRadius: 'var(--radius-md)',
              background: 'var(--accent-blue)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              flexShrink: 0,
            }}
          >
            <SyncMiniIcon />
          </div>
          {!collapsed && (
            <span
              style={{
                fontSize: 'var(--text-lg)',
                fontWeight: 700,
                color: 'var(--text-primary)',
                whiteSpace: 'nowrap',
              }}
            >
              Every-Sync
            </span>
          )}
        </div>

        {/* Navigation */}
        <nav
          style={{
            flex: 1,
            padding: 'var(--space-2)',
            overflowY: 'auto',
          }}
        >
          {navItems.map((item) => {
            const isActive =
              item.to === '/'
                ? location.pathname === '/'
                : location.pathname.startsWith(item.to);

            return (
              <NavLink
                key={item.to}
                to={item.to}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 'var(--space-3)',
                  padding: collapsed
                    ? 'var(--space-2) var(--space-2)'
                    : 'var(--space-2) var(--space-3)',
                  borderRadius: 'var(--radius-md)',
                  color: isActive ? 'var(--accent-blue)' : 'var(--text-secondary)',
                  background: isActive ? 'var(--accent-blue-bg)' : 'transparent',
                  textDecoration: 'none',
                  fontWeight: isActive ? 600 : 400,
                  fontSize: 'var(--text-sm)',
                  transition: 'all var(--transition-fast)',
                  marginBottom: '2px',
                  whiteSpace: 'nowrap',
                  justifyContent: collapsed ? 'center' : 'flex-start',
                  position: 'relative',
                }}
                onMouseEnter={(e) => {
                  if (!isActive) {
                    (e.currentTarget as HTMLElement).style.background =
                      'var(--bg-surface-hover)';
                  }
                }}
                onMouseLeave={(e) => {
                  if (!isActive) {
                    (e.currentTarget as HTMLElement).style.background = 'transparent';
                  }
                }}
                title={collapsed ? item.label : undefined}
              >
                <item.icon
                  size={18}
                  color={isActive ? 'var(--accent-blue)' : 'var(--text-secondary)'}
                />
                {!collapsed && <span>{item.label}</span>}
                {!collapsed && item.to === '/conflicts' && conflictCount > 0 && (
                  <span
                    style={{
                      marginLeft: 'auto',
                      display: 'inline-flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      minWidth: '18px',
                      height: '18px',
                      padding: '0 5px',
                      borderRadius: '100px',
                      background: 'var(--accent-red)',
                      color: '#FFFFFF',
                      fontSize: '11px',
                      fontWeight: 600,
                      lineHeight: 1,
                    }}
                  >
                    {conflictCount}
                  </span>
                )}
              </NavLink>
            );
          })}
        </nav>

        {/* Footer actions */}
        <div
          style={{
            padding: 'var(--space-2)',
            borderTop: '1px solid var(--border-muted)',
            display: 'flex',
            flexDirection: 'column',
            gap: '2px',
          }}
        >
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
          <button
            onClick={toggleTheme}
            className="btn-ghost"
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 'var(--space-3)',
              padding: collapsed ? 'var(--space-2)' : 'var(--space-2) var(--space-3)',
              borderRadius: 'var(--radius-md)',
              border: 'none',
              background: 'transparent',
              color: 'var(--text-secondary)',
              cursor: 'pointer',
              fontFamily: 'inherit',
              fontSize: 'var(--text-sm)',
              width: '100%',
              justifyContent: collapsed ? 'center' : 'flex-start',
            }}
            title={darkMode ? 'Switch to light mode' : 'Switch to dark mode'}
          >
            {darkMode ? <SunIcon size={18} /> : <MoonIcon size={18} />}
            {!collapsed && <span>{darkMode ? 'Light Mode' : 'Dark Mode'}</span>}
          </button>
          <button
            onClick={() => setCollapsed(!collapsed)}
            className="btn-ghost"
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 'var(--space-3)',
              padding: collapsed ? 'var(--space-2)' : 'var(--space-2) var(--space-3)',
              borderRadius: 'var(--radius-md)',
              border: 'none',
              background: 'transparent',
              color: 'var(--text-secondary)',
              cursor: 'pointer',
              fontFamily: 'inherit',
              fontSize: 'var(--text-sm)',
              width: '100%',
              justifyContent: collapsed ? 'center' : 'flex-start',
            }}
            title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            <ChevronRightIcon
              size={18}
              className={collapsed ? '' : undefined}
            />
            {!collapsed && <span>Collapse</span>}
          </button>
        </div>
      </aside>
    </>
  );
};

/** Small sync icon for the logo */
const SyncMiniIcon: React.FC = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
    <path
      d="M3 8C3 5.24 5.24 3 8 3C9.77 3 11.29 3.95 12.09 5.36L13 4.45C11.93 2.78 10.1 1.7 8 1.7C4.52 1.7 1.7 4.52 1.7 8H3Z"
      fill="white"
    />
    <path
      d="M13 8C13 10.76 10.76 13 8 13C6.23 13 4.71 12.05 3.91 10.64L3 11.55C4.07 13.22 5.9 14.3 8 14.3C11.48 14.3 14.3 11.48 14.3 8H13Z"
      fill="white"
    />
  </svg>
);
