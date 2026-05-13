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
  GlobeIcon,
  MenuIcon,
} from './Icons';
import { useI18n } from '../i18n';

interface SidebarProps {
  conflictCount?: number;
  wsConnected?: boolean;
}

interface NavItem {
  to: string;
  labelKey: string;
  icon: React.FC<{ size?: number; color?: string; className?: string }>;
}

const navItems: NavItem[] = [
  { to: '/', labelKey: 'sidebar.dashboard', icon: GridIcon },
  { to: '/files', labelKey: 'sidebar.files', icon: FolderIcon },
  { to: '/pairs', labelKey: 'sidebar.pairs', icon: LayersIcon },
  { to: '/providers', labelKey: 'sidebar.providers', icon: GearIcon },
  { to: '/conflicts', labelKey: 'sidebar.conflicts', icon: WarningIcon },
  { to: '/versions', labelKey: 'sidebar.versions', icon: ClockIcon },
  { to: '/recent', labelKey: 'sidebar.recent', icon: DocumentIcon },
  { to: '/logs', labelKey: 'sidebar.logs', icon: DocumentIcon },
];

export const Sidebar: React.FC<SidebarProps> = ({ conflictCount = 0, wsConnected = false }) => {
  const location = useLocation();
  const { t, toggleLang } = useI18n();
  const [collapsed, setCollapsed] = useState(() => {
    if (typeof window === 'undefined') return false;
    return window.innerWidth < 700;
  });
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
        {/* Header: hamburger toggle + brand */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 'var(--space-3)',
            padding: 'var(--space-2) var(--space-3)',
            borderBottom: '1px solid var(--border-muted)',
            minHeight: '52px',
          }}
        >
          <button
            onClick={() => setCollapsed(!collapsed)}
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              width: '32px',
              height: '32px',
              borderRadius: 'var(--radius-md)',
              border: 'none',
              background: 'transparent',
              color: 'var(--text-secondary)',
              cursor: 'pointer',
              flexShrink: 0,
              transition: 'all var(--transition-fast)',
            }}
            title={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
            onMouseEnter={(e) => {
              (e.currentTarget as HTMLElement).style.background = 'var(--bg-surface-hover)';
            }}
            onMouseLeave={(e) => {
              (e.currentTarget as HTMLElement).style.background = 'transparent';
            }}
          >
            <MenuIcon size={20} />
          </button>
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
                title={collapsed ? t(item.labelKey) : undefined}
              >
                <item.icon
                  size={18}
                  color={isActive ? 'var(--accent-blue)' : 'var(--text-secondary)'}
                />
                {!collapsed && <span>{t(item.labelKey)}</span>}
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
              {wsConnected ? t('sidebar.connected') : t('sidebar.disconnected')}
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
            title={darkMode ? t('sidebar.lightMode') : t('sidebar.darkMode')}
          >
            {darkMode ? <SunIcon size={18} /> : <MoonIcon size={18} />}
            {!collapsed && <span>{darkMode ? t('sidebar.lightMode') : t('sidebar.darkMode')}</span>}
          </button>
          <button
            onClick={toggleLang}
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
            title={t('sidebar.switchLang')}
          >
            <GlobeIcon size={18} />
            {!collapsed && <span>{t('sidebar.switchLang')}</span>}
          </button>
        </div>
      </aside>
    </>
  );
};
