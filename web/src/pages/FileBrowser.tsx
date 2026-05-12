import React, { useEffect, useState, useCallback, useRef } from 'react';
import { listPairs, listFiles, materializeFile, excludePath } from '../api/client';
import type { SyncPair, FileEntry, FileSide } from '../api/client';
import { StatusIcon } from '../components/StatusIcon';
import { Breadcrumb } from '../components/Breadcrumb';
import {
  FolderIcon,
  FileIcon,
  DotsIcon,
  SyncIcon,
  ChevronRightIcon,
  CloudIcon,
  ClockIcon,
  WarningIcon,
  CloseIcon,
} from '../components/Icons';
import { useI18n } from '../i18n';

function formatSize(bytes: number): string {
  if (bytes === 0) return '—';
  const units = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const val = bytes / Math.pow(1024, i);
  return val.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
}

function formatDate(iso: string, t: (key: string, params?: Record<string, string | number>) => string): string {
  try {
    const d = new Date(iso);
    const now = new Date();
    const diff = now.getTime() - d.getTime();
    if (diff < 60000) return t('time.justNow');
    if (diff < 3600000) return t('time.minutesAgo', { n: Math.floor(diff / 60000) });
    if (diff < 86400000) return t('time.hoursAgo', { n: Math.floor(diff / 3600000) });
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  } catch {
    return iso;
  }
}

interface ActionMenuState {
  visible: boolean;
  x: number;
  y: number;
  entry: FileEntry | null;
}

export const FileBrowser: React.FC = () => {
  const { t } = useI18n();
  const [pairs, setPairs] = useState<SyncPair[]>([]);
  const [selectedPairId, setSelectedPairId] = useState<string>('');
  const [side, setSide] = useState<FileSide>('local');
  const [currentPath, setCurrentPath] = useState('/');
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionMenu, setActionMenu] = useState<ActionMenuState>({
    visible: false,
    x: 0,
    y: 0,
    entry: null,
  });
  const menuRef = useRef<HTMLDivElement>(null);

  const selectedPair = pairs.find((p) => p.id === selectedPairId);

  useEffect(() => {
    listPairs()
      .then((p) => {
        setPairs(p);
        if (p.length > 0 && !selectedPairId) {
          setSelectedPairId(p[0].id);
        }
      })
      .catch(() => {
        setPairs([]);
      });
  }, []);

  useEffect(() => {
    if (!selectedPairId) return;
    setLoading(true);
    setError(null);
    listFiles(selectedPairId, currentPath, side)
      .then((files) => setEntries(files))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [selectedPairId, currentPath, side]);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setActionMenu({ visible: false, x: 0, y: 0, entry: null });
      }
    };
    if (actionMenu.visible) {
      document.addEventListener('mousedown', handleClick);
      return () => document.removeEventListener('mousedown', handleClick);
    }
  }, [actionMenu.visible]);

  const handleNavigate = useCallback((path: string) => {
    setCurrentPath(path);
  }, []);

  const handleFolderClick = useCallback((entry: FileEntry) => {
    setCurrentPath(entry.path);
  }, []);

  const handleActionClick = useCallback((e: React.MouseEvent, entry: FileEntry) => {
    e.stopPropagation();
    const rect = (e.target as HTMLElement).getBoundingClientRect();
    setActionMenu({
      visible: true,
      x: rect.right,
      y: rect.bottom,
      entry,
    });
  }, []);

  const handleMaterialize = useCallback(async () => {
    if (!selectedPairId || !actionMenu.entry) return;
    try {
      await materializeFile(selectedPairId, actionMenu.entry.path);
      const files = await listFiles(selectedPairId, currentPath, side);
      setEntries(files);
    } catch {
      // ignore
    }
    setActionMenu({ visible: false, x: 0, y: 0, entry: null });
  }, [selectedPairId, currentPath, side, actionMenu.entry]);

  const handleExclude = useCallback(async () => {
    if (!selectedPairId || !actionMenu.entry) return;
    try {
      await excludePath(selectedPairId, actionMenu.entry.path);
      const files = await listFiles(selectedPairId, currentPath, side);
      setEntries(files);
    } catch {
      // ignore
    }
    setActionMenu({ visible: false, x: 0, y: 0, entry: null });
  }, [selectedPairId, currentPath, side, actionMenu.entry]);

  const sorted = [...entries].sort((a, b) => {
    if (a.type !== b.type) return a.type === 'folder' ? -1 : 1;
    return a.name.localeCompare(b.name);
  });

  return (
    <div style={{ padding: 'var(--space-6)', height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div style={{ marginBottom: 'var(--space-4)', flexShrink: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-4)', flexWrap: 'wrap' }}>
          <div>
            <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, color: 'var(--text-primary)', margin: 0 }}>
              {t('files.title')}
            </h1>
            <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
              {t('files.subtitle')}
            </p>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
            <select
              value={selectedPairId}
              onChange={(e) => {
                setSelectedPairId(e.target.value);
                setCurrentPath('/');
              }}
              style={{
                padding: 'var(--space-2) var(--space-3)',
                borderRadius: 'var(--radius-md)',
                border: '1px solid var(--border-default)',
                background: 'var(--bg-surface)',
                color: 'var(--text-primary)',
                fontFamily: 'var(--font-sans)',
                fontSize: 'var(--text-sm)',
                cursor: 'pointer',
                minWidth: '200px',
              }}
            >
              <option value="">{t('files.selectPair')}</option>
              {pairs.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name} ({p.mode}, {p.direction})
                </option>
              ))}
            </select>

            <div className="tab-toggle">
              <button
                className={`tab-toggle-btn ${side === 'local' ? 'active' : ''}`}
                onClick={() => setSide('local')}
              >
                {t('files.local')}
              </button>
              <button
                className={`tab-toggle-btn ${side === 'remote' ? 'active' : ''}`}
                onClick={() => setSide('remote')}
              >
                {t('files.remote')}
              </button>
            </div>
          </div>
        </div>

        {selectedPair && (
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 'var(--space-3)',
              marginTop: 'var(--space-3)',
              padding: 'var(--space-2) var(--space-3)',
              borderRadius: 'var(--radius-md)',
              background: 'var(--bg-surface)',
              border: '1px solid var(--border-muted)',
              fontSize: 'var(--text-xs)',
              color: 'var(--text-secondary)',
            }}
          >
            <StatusIcon status={selectedPair.status} size={14} />
            <span>{selectedPair.local_path}</span>
            <SyncIcon size={14} color="var(--accent-blue)" />
            <span>{selectedPair.remote_provider}: {selectedPair.remote_path}</span>
            <span className="badge badge-blue">{selectedPair.mode}</span>
            <span className="badge badge-blue">{selectedPair.direction}</span>
          </div>
        )}
      </div>

      {selectedPairId && (
        <Breadcrumb path={currentPath} onNavigate={handleNavigate} />
      )}

      <div
        className="card"
        style={{
          flex: 1,
          marginTop: 'var(--space-3)',
          padding: 0,
          overflow: 'auto',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: '32px 32px 1fr 90px 120px 32px 36px',
            gap: 'var(--space-2)',
            alignItems: 'center',
            padding: 'var(--space-2) var(--space-4)',
            borderBottom: '1px solid var(--border-default)',
            fontSize: 'var(--text-xs)',
            fontWeight: 500,
            color: 'var(--text-secondary)',
            textTransform: 'uppercase',
            letterSpacing: '0.05em',
            position: 'sticky',
            top: 0,
            background: 'var(--bg-surface)',
            zIndex: 1,
          }}
        >
          <span></span>
          <span></span>
          <span>{t('files.name')}</span>
          <span>{t('files.size')}</span>
          <span>{t('files.modified')}</span>
          <span></span>
          <span></span>
        </div>

        {!selectedPairId && (
          <div style={emptyStyle}>{t('files.selectToBrowse')}</div>
        )}
        {loading && (
          <div style={emptyStyle}>
            <SyncIcon size={20} color="var(--accent-blue)" spinning />
            <span style={{ marginLeft: 'var(--space-2)' }}>{t('common.loading')}</span>
          </div>
        )}
        {error && (
          <div style={{ ...emptyStyle, color: 'var(--accent-red)' }}>{t('files.error')}: {error}</div>
        )}
        {!loading && !error && selectedPairId && entries.length === 0 && (
          <div style={emptyStyle}>{t('files.empty')}</div>
        )}

        {!loading && !error && sorted.map((entry) => (
          <FileRow
            key={entry.path}
            entry={entry}
            side={side}
            onFolderClick={handleFolderClick}
            onActionClick={handleActionClick}
            t={t}
          />
        ))}
      </div>

      {actionMenu.visible && actionMenu.entry && (
        <div
          ref={menuRef}
          className="dropdown-menu"
          style={{
            position: 'fixed',
            left: actionMenu.x,
            top: actionMenu.y,
            zIndex: 300,
          }}
        >
          {actionMenu.entry.status === 'virtual' && (
            <div className="dropdown-item" onClick={handleMaterialize}>
              <CloudIcon size={16} color="var(--accent-violet)" />
              {t('files.materialize')}
            </div>
          )}
          <div className="dropdown-item">
            <ClockIcon size={16} color="var(--text-secondary)" />
            {t('files.viewVersions')}
          </div>
          {actionMenu.entry.status === 'conflict' && (
            <div className="dropdown-item">
              <WarningIcon size={16} color="var(--accent-red)" />
              {t('files.resolveConflict')}
            </div>
          )}
          <div className="dropdown-item" onClick={handleExclude}>
            <CloseIcon size={16} color="var(--accent-red)" />
            {t('files.exclude')}
          </div>
        </div>
      )}
    </div>
  );
};

interface FileRowProps {
  entry: FileEntry;
  side: FileSide;
  onFolderClick: (entry: FileEntry) => void;
  onActionClick: (e: React.MouseEvent, entry: FileEntry) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
}

const FileRow: React.FC<FileRowProps> = ({ entry, side, onFolderClick, onActionClick, t }) => {
  const isFolder = entry.type === 'folder';
  const [hovered, setHovered] = useState(false);

  return (
    <div
      onClick={isFolder ? () => onFolderClick(entry) : undefined}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      style={{
        display: 'grid',
        gridTemplateColumns: '32px 32px 1fr 90px 120px 32px 36px',
        gap: 'var(--space-2)',
        alignItems: 'center',
        padding: 'var(--space-1) var(--space-4)',
        borderBottom: '1px solid var(--border-muted)',
        cursor: isFolder ? 'pointer' : 'default',
        background: hovered ? 'var(--bg-surface-hover)' : 'transparent',
        transition: 'background var(--transition-fast)',
        fontSize: 'var(--text-sm)',
      }}
    >
      <span style={{ display: 'flex', justifyContent: 'center' }}>
        {isFolder && (
          <input
            type="checkbox"
            className="checkbox"
            checked={entry.selected ?? false}
            onChange={() => {/* toggle selection */}}
            onClick={(e) => e.stopPropagation()}
          />
        )}
      </span>

      {isFolder ? (
        <FolderIcon size={18} color="var(--accent-amber)" />
      ) : (
        <FileIcon size={18} color="var(--text-secondary)" />
      )}

      <span style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', overflow: 'hidden' }}>
        <span style={{ fontWeight: isFolder ? 500 : 400, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {entry.name}
        </span>
        {isFolder && entry.children_count != null && (
          <span style={{ fontSize: 'var(--text-xs)', color: 'var(--text-tertiary)', flexShrink: 0 }}>
            ({entry.children_count})
          </span>
        )}
        {isFolder && <ChevronRightIcon size={14} color="var(--text-tertiary)" />}
      </span>

      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--text-xs)', color: 'var(--text-secondary)' }}>
        {isFolder ? `${entry.children_count ?? 0} ${t('files.items')}` : formatSize(entry.size)}
      </span>

      <span style={{ fontSize: 'var(--text-xs)', color: 'var(--text-secondary)' }}>
        {formatDate(entry.modified, t)}
      </span>

      <span style={{ display: 'flex', justifyContent: 'center' }}>
        <StatusIcon status={entry.status} size={16} />
      </span>

      <span style={{ display: 'flex', justifyContent: 'center' }}>
        <button
          className="btn-icon btn-ghost"
          onClick={(e) => onActionClick(e, entry)}
          style={{
            width: '28px',
            height: '28px',
            padding: '4px',
            borderRadius: 'var(--radius-sm)',
            opacity: hovered ? 1 : 0,
            transition: 'opacity var(--transition-fast)',
          }}
        >
          <DotsIcon size={16} color="var(--text-secondary)" />
        </button>
      </span>
    </div>
  );
};

const emptyStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  padding: 'var(--space-10)',
  color: 'var(--text-tertiary)',
  fontSize: 'var(--text-sm)',
};
