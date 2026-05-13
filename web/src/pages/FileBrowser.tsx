import React, { useEffect, useState, useCallback, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { listPairs, listFiles, materializeFile, selectFolders } from '../api/client';
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
} from '../components/Icons';
import { getPairDirectionLabelKey, getPairModeLabelKey, useI18n } from '../i18n';
import { showToast } from '../components/Toast';
import { useSyncProgress } from '../hooks/useSyncProgress';
import { ProgressBar } from '../components/ProgressBar';

function formatSize(bytes: number): string {
  if (bytes === 0) return '—';
  const units = ['B', 'KB', 'MB', 'GB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const val = bytes / Math.pow(1024, i);
  return val.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
}

function formatDate(iso: string, t: (key: string, params?: Record<string, string | number>) => string): string {
  if (!iso) return t('common.notAvailable');
  try {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return t('common.notAvailable');
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
  const navigate = useNavigate();
  const [pairs, setPairs] = useState<SyncPair[]>([]);
  const [selectedPairId, setSelectedPairId] = useState<string>('');
  const [side, setSide] = useState<FileSide>('local');
  const [currentPath, setCurrentPath] = useState('/');
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [pairsLoading, setPairsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pairsError, setPairsError] = useState<string | null>(null);
  const [actionMenu, setActionMenu] = useState<ActionMenuState>({
    visible: false,
    x: 0,
    y: 0,
    entry: null,
  });
  const menuRef = useRef<HTMLDivElement>(null);

  const { getProgress } = useSyncProgress();
  const syncProgress = selectedPairId ? getProgress(selectedPairId) : undefined;
  const syncStatus = syncProgress?.status || 'idle';
  const prevSyncStatus = useRef(syncStatus);

  const selectedPair = pairs.find((p) => p.id === selectedPairId);
  const selectedFolders = React.useMemo(() => {
    if (!selectedPair?.selected_folders) return new Set<string>();
    try {
      const parsed = JSON.parse(selectedPair.selected_folders);
      return new Set(Array.isArray(parsed) ? parsed.map(String) : []);
    } catch {
      return new Set<string>();
    }
  }, [selectedPair?.selected_folders]);

  useEffect(() => {
    setPairsLoading(true);
    setPairsError(null);
    listPairs()
      .then((p) => {
        setPairs(p);
        setSelectedPairId((current) => current || p[0]?.id || '');
      })
      .catch((e) => {
        setPairsError(e instanceof Error ? e.message : t('files.loadPairsFailed'));
        setPairs([]);
      })
      .finally(() => setPairsLoading(false));
  }, [t]);

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

  // Refresh file list when sync completes
  useEffect(() => {
    if (prevSyncStatus.current === 'syncing' && syncStatus === 'completed' && selectedPairId) {
      setLoading(true);
      setError(null);
      listFiles(selectedPairId, currentPath, side)
        .then((files) => setEntries(files))
        .catch((err) => setError(err.message))
        .finally(() => setLoading(false));
    }
    prevSyncStatus.current = syncStatus;
  }, [syncStatus, selectedPairId, currentPath, side]);

  const handleNavigate = useCallback((path: string) => {
    setCurrentPath(path);
  }, []);

  const handleFolderClick = useCallback((entry: FileEntry) => {
    setCurrentPath(entry.path);
  }, []);

  const handleActionClick = useCallback((e: React.MouseEvent, entry: FileEntry) => {
    e.stopPropagation();
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
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
    } catch (e) {
      showToast(e instanceof Error ? e.message : t('files.materializeFailed'), 'error');
    }
    setActionMenu({ visible: false, x: 0, y: 0, entry: null });
  }, [selectedPairId, currentPath, side, actionMenu.entry, t]);

  const handleViewVersions = useCallback(() => {
    if (!selectedPairId || !actionMenu.entry) return;
    navigate(`/versions?pair_id=${encodeURIComponent(selectedPairId)}&path=${encodeURIComponent(actionMenu.entry.path)}`);
    setActionMenu({ visible: false, x: 0, y: 0, entry: null });
  }, [actionMenu.entry, navigate, selectedPairId]);

  const handleResolveConflict = useCallback(() => {
    if (!selectedPairId) return;
    navigate(`/conflicts?pair_id=${encodeURIComponent(selectedPairId)}`);
    setActionMenu({ visible: false, x: 0, y: 0, entry: null });
  }, [navigate, selectedPairId]);

  const handleFolderSelection = useCallback(async (entry: FileEntry, selected: boolean) => {
    if (!selectedPairId) return;
    const next = new Set(selectedFolders);
    if (selected) {
      next.add(entry.path);
    } else {
      next.delete(entry.path);
    }
    try {
      const updated = await selectFolders(selectedPairId, [...next]);
      setPairs((prev) => prev.map((pair) => pair.id === updated.id ? updated : pair));
      setEntries((prev) => prev.map((item) => item.path === entry.path ? { ...item, selected } : item));
    } catch (e) {
      showToast(e instanceof Error ? e.message : t('files.selectionFailed'), 'error');
    }
  }, [selectedFolders, selectedPairId, t]);

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
              disabled={pairsLoading}
            >
              <option value="">{t('files.selectPair')}</option>
              {pairs.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name} ({t(getPairModeLabelKey(p.mode))}, {t(getPairDirectionLabelKey(p.direction))})
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
            <span>{selectedPair.provider}: {selectedPair.remote_path}</span>
            <span className="badge badge-blue">{t(getPairModeLabelKey(selectedPair.mode))}</span>
            <span className="badge badge-blue">{t(getPairDirectionLabelKey(selectedPair.direction))}</span>
          </div>
        )}

        {selectedPairId && syncProgress?.status === 'syncing' && (() => {
          const progress = syncProgress;
          const percent = progress.filesTotal > 0 ? Math.round((progress.filesSynced / progress.filesTotal) * 100) : 0;
          return (
            <div style={{
              display: 'flex',
              alignItems: 'center',
              gap: 'var(--space-3)',
              marginTop: 'var(--space-2)',
              padding: 'var(--space-2) var(--space-3)',
              borderRadius: 'var(--radius-md)',
              background: 'var(--accent-blue-bg)',
              border: '1px solid var(--accent-blue)',
              fontSize: 'var(--text-sm)',
              color: 'var(--accent-blue)',
            }}>
              <SyncIcon size={16} spinning />
              <span style={{ flex: 1 }}>
                {progress.currentFile ? `Syncing: ${progress.currentFile.length > 60 ? '...' + progress.currentFile.slice(-57) : progress.currentFile}` : 'Processing...'}
              </span>
              {progress.filesTotal > 0 && <span>{progress.filesSynced}/{progress.filesTotal} ({percent}%)</span>}
              <div style={{ width: '120px', height: '4px', background: 'var(--border-default)', borderRadius: '2px', overflow: 'hidden' }}>
                <div style={{ width: `${percent}%`, height: '100%', background: 'var(--accent-blue)', borderRadius: '2px', transition: 'width 0.3s ease' }} />
              </div>
            </div>
          );
        })()}
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

        {pairsError && (
          <div style={{ ...emptyStyle, color: 'var(--accent-red)' }}>
            {t('files.loadPairsFailed')}: {pairsError}
          </div>
        )}
        {!pairsError && !selectedPairId && (
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
            onFolderClick={handleFolderClick}
            onActionClick={handleActionClick}
            onSelectionToggle={handleFolderSelection}
            selected={entry.type === 'folder' && (selectedFolders.has(entry.path) || Boolean(entry.selected))}
            isFileSyncing={syncProgress?.status === 'syncing' && syncProgress.currentFile === entry.path}
            activeFile={syncProgress?.activeFile}
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
          <div className="dropdown-item" onClick={handleViewVersions}>
            <ClockIcon size={16} color="var(--text-secondary)" />
            {t('files.viewVersions')}
          </div>
          {actionMenu.entry.status === 'conflict' && (
            <div className="dropdown-item" onClick={handleResolveConflict}>
              <WarningIcon size={16} color="var(--accent-red)" />
              {t('files.resolveConflict')}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

interface FileRowProps {
  entry: FileEntry;
  onFolderClick: (entry: FileEntry) => void;
  onActionClick: (e: React.MouseEvent, entry: FileEntry) => void;
  onSelectionToggle: (entry: FileEntry, selected: boolean) => void;
  selected: boolean;
  isFileSyncing: boolean;
  activeFile?: { path: string; bytesTransferred: number; bytesTotal: number; percent: number; taskType: string };
  t: (key: string, params?: Record<string, string | number>) => string;
}

const FileRow: React.FC<FileRowProps> = ({ entry, onFolderClick, onActionClick, onSelectionToggle, selected, isFileSyncing, activeFile, t }) => {
  const isFolder = entry.type === 'folder';
  const [hovered, setHovered] = useState(false);
  const fileProgressActive = Boolean(activeFile && activeFile.path === entry.path);

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
        background: isFileSyncing ? 'var(--accent-blue-bg)' : (hovered ? 'var(--bg-surface-hover)' : 'transparent'),
        borderLeft: isFileSyncing ? '3px solid var(--accent-blue)' : '3px solid transparent',
        transition: 'background var(--transition-fast)',
        fontSize: 'var(--text-sm)',
      }}
    >
      <span style={{ display: 'flex', justifyContent: 'center' }}>
        {isFolder && (
          <input
            type="checkbox"
            className="checkbox"
            checked={selected}
            onChange={(e) => onSelectionToggle(entry, e.currentTarget.checked)}
            onClick={(e) => e.stopPropagation()}
            aria-label={`${t('common.enabled')}: ${entry.name}`}
          />
        )}
      </span>

      {isFolder ? (
        <FolderIcon size={18} color="var(--accent-amber)" />
      ) : (
        <FileIcon size={18} color="var(--text-secondary)" />
      )}

      <div style={{ display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
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
        {fileProgressActive && activeFile && (
          <div style={{ marginTop: '4px', display: 'grid', gap: '3px' }}>
            <ProgressBar percent={activeFile.percent} />
            <span style={{ fontSize: 'var(--text-xs)', color: 'var(--accent-blue)' }}>
              {activeFile.taskType} · {formatSize(activeFile.bytesTransferred)} / {formatSize(activeFile.bytesTotal)} · {Math.round(activeFile.percent)}%
            </span>
          </div>
        )}
      </div>

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
            opacity: hovered ? 1 : 0.55,
            transition: 'opacity var(--transition-fast)',
          }}
          title={t('common.actions')}
          aria-label={t('common.actions')}
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
