import React, { useEffect, useRef, useState, useCallback } from 'react';
import { listLogs } from '../api/client';
import type { LogEntry, WSEvent } from '../api/client';
import { showToast } from '../components/Toast';
import { useI18n } from '../i18n';
import { useWebSocket } from '../hooks/useWebSocket';

const levelColors: Record<string, string> = {
  debug: 'var(--text-tertiary)',
  info: 'var(--accent-blue)',
  warn: 'var(--accent-amber)',
  error: 'var(--accent-red)',
};

const levelBg: Record<string, string> = {
  debug: 'var(--bg-surface-hover)',
  info: 'var(--accent-blue-bg)',
  warn: 'var(--accent-amber-bg)',
  error: 'var(--accent-red-bg)',
};

export const Logs: React.FC = () => {
  const { t } = useI18n();
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [level, setLevel] = useState<string>('');
  const [search, setSearch] = useState('');
  const [paused, setPaused] = useState(false);
  const bodyRef = useRef<HTMLDivElement>(null);

  const appendLogEvent = useCallback((event: WSEvent) => {
    if (event.type !== 'log') return;
    setLogs((prev) => {
      const next = [
        ...prev,
        {
          id: `ws-${event.timestamp}-${prev.length}`,
          timestamp: event.timestamp || new Date().toISOString(),
          level: event.level,
          message: event.message,
          pair_id: event.pair_id,
        },
      ];
      return next.slice(-500);
    });
  }, []);

  useWebSocket({ onEvent: appendLogEvent });

  const loadInitial = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const data = await listLogs();
      setLogs(data);
    } catch (e) {
      setLoadError(e instanceof Error ? e.message : t('logs.loadFailed'));
      setLogs([]);
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => { loadInitial(); }, [loadInitial]);

  const filteredLogs = logs.filter((log) => {
    if (level && log.level !== level) return false;
    if (search && !log.message.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  const handleClear = () => {
    setLogs([]);
    showToast(t('logs.logsCleared'), 'info');
  };

  useEffect(() => {
    if (!paused && bodyRef.current) {
      const el = bodyRef.current;
      el.scrollTop = el.scrollHeight;
    }
  }, [filteredLogs.length, paused]);

  const selectStyle: React.CSSProperties = {
    padding: 'var(--space-2) var(--space-3)',
    borderRadius: 'var(--radius-md)',
    border: '1px solid var(--border-default)',
    background: 'var(--bg-surface)',
    color: 'var(--text-primary)',
    fontFamily: 'var(--font-sans)',
    fontSize: 'var(--text-sm)',
  };

  const inputStyle: React.CSSProperties = {
    ...selectStyle,
    flex: 1,
    minWidth: '100px',
  };

  return (
    <div style={{ padding: 'var(--space-6)', height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', marginBottom: 'var(--space-4)', flexShrink: 0, flexWrap: 'wrap' }}>
        <div>
          <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>{t('logs.title')}</h1>
          <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', marginTop: 'var(--space-1)' }}>
            {t('logs.subtitle')}
          </p>
        </div>
        <div style={{ flex: 1 }} />
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t('logs.filterPlaceholder')}
          style={inputStyle}
        />
        <select value={level} onChange={(e) => setLevel(e.target.value)} style={selectStyle}>
          <option value="">{t('logs.allLevels')}</option>
          <option value="debug">{t('logs.debug')}</option>
          <option value="info">{t('logs.info')}</option>
          <option value="warn">{t('logs.warning')}</option>
          <option value="error">{t('logs.error')}</option>
        </select>
        <button className="btn btn-sm" onClick={() => setPaused(!paused)} style={paused ? { background: 'var(--accent-amber-bg)', borderColor: 'var(--accent-amber)' } : {}}>
          {paused ? t('logs.resume') : t('logs.pause')}
        </button>
        <button className="btn btn-sm" onClick={handleClear}>{t('logs.clear')}</button>
      </div>

      <div
        className="card"
        ref={bodyRef}
        style={{
          flex: 1,
          padding: 0,
          overflow: 'auto',
          fontFamily: 'var(--font-mono)',
          fontSize: 'var(--text-xs)',
        }}
      >
        {loading ? (
          <div style={{ padding: 'var(--space-8)', textAlign: 'center', color: 'var(--text-secondary)' }}>{t('common.loading')}</div>
        ) : loadError ? (
          <div style={{ padding: 'var(--space-8)', textAlign: 'center', color: 'var(--accent-red)' }}>
            <div style={{ marginBottom: 'var(--space-3)' }}>{t('logs.loadFailed')}: {loadError}</div>
            <button className="btn" onClick={loadInitial}>{t('common.retry')}</button>
          </div>
        ) : filteredLogs.length === 0 ? (
          <div style={{ padding: 'var(--space-8)', textAlign: 'center', color: 'var(--text-tertiary)' }}>{t('logs.noEntries')}</div>
        ) : (
          filteredLogs.map((log) => (
            <div
              key={log.id}
              style={{
                display: 'flex',
                gap: 'var(--space-3)',
                padding: 'var(--space-2) var(--space-4)',
                borderBottom: '1px solid var(--border-muted)',
                alignItems: 'flex-start',
              }}
            >
              <span style={{ color: 'var(--text-tertiary)', whiteSpace: 'nowrap', flexShrink: 0 }}>
                {new Date(log.timestamp).toLocaleTimeString()}
              </span>
              <span style={{
                display: 'inline-block',
                padding: '1px 6px',
                borderRadius: 'var(--radius-sm)',
                background: levelBg[log.level] || 'var(--bg-surface-hover)',
                color: levelColors[log.level] || 'var(--text-secondary)',
                fontWeight: 500,
                textTransform: 'uppercase',
                fontSize: '10px',
                flexShrink: 0,
                minWidth: '40px',
                textAlign: 'center',
              }}>
                {log.level}
              </span>
              <span style={{ color: 'var(--text-primary)', wordBreak: 'break-word' }}>{log.message}</span>
              {log.pair_id && (
                <span className="badge badge-blue" style={{ flexShrink: 0, marginLeft: 'auto' }}>
                  {log.pair_id.slice(0, 8)}
                </span>
              )}
            </div>
          ))
        )}
      </div>

      <div style={{ display: 'flex', gap: 'var(--space-4)', padding: 'var(--space-2) 0', fontSize: 'var(--text-xs)', color: 'var(--text-tertiary)' }}>
        <span>{t('logs.total')}: <strong>{logs.length}</strong></span>
        <span>{t('logs.showing')}: <strong>{filteredLogs.length}</strong></span>
        {paused && <span style={{ color: 'var(--accent-amber)', fontWeight: 600 }}>{t('logs.paused')}</span>}
      </div>
    </div>
  );
};
