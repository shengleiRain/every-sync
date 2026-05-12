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
  const [isAtBottom, setIsAtBottom] = useState(true);
  const bodyRef = useRef<HTMLDivElement>(null);

  const appendLogEvent = useCallback((event: WSEvent) => {
    const raw = event as {
      type: string;
      timestamp?: string;
      time?: string;
      level?: LogEntry['level'];
      message?: string;
      pair_id?: number | string;
      error?: string;
      pair_name?: string;
      path?: string;
      direction?: string;
    };
    const timestamp = raw.timestamp || raw.time || new Date().toISOString();
    const level: LogEntry['level'] = raw.type === 'log' && raw.level
      ? raw.level
      : raw.error || raw.type.includes('failed') || raw.type.includes('conflict')
        ? raw.type.includes('conflict') && !raw.error ? 'warn' : 'error'
        : 'info';
    const message = raw.type === 'log'
      ? raw.message || ''
      : formatEngineEvent(raw);
    const pairId = raw.pair_id == null ? undefined : String(raw.pair_id);

    setLogs((prev) => {
      const next = [
        ...prev,
        {
          id: `ws-${timestamp}-${prev.length}`,
          timestamp,
          level,
          message,
          pair_id: pairId,
        },
      ];
      return next.slice(-500);
    });
  }, []);

  const { connected: wsConnected } = useWebSocket({ onEvent: appendLogEvent });

  const loadInitial = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const data = await listLogs(undefined, undefined, 200);
      // Merge: API history first, then any WS logs that arrived before the API response
      setLogs((prev) => {
        if (data.length === 0) return prev;
        if (prev.length === 0) return data;
        // Keep WS logs that arrived after the latest history log
        const lastHistoryTime = data[data.length - 1]?.timestamp || '';
        const newerWsLogs = prev.filter(l => l.timestamp > lastHistoryTime);
        return [...data, ...newerWsLogs];
      });
    } catch (e) {
      setLoadError(e instanceof Error ? e.message : t('logs.loadFailed'));
      // Don't clear logs on API error - keep WS logs
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => { loadInitial(); }, [loadInitial]);

  const handleScroll = useCallback(() => {
    if (!bodyRef.current) return;
    const el = bodyRef.current;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 50;
    setIsAtBottom(atBottom);
  }, []);

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
    if (!paused && bodyRef.current && isAtBottom) {
      const el = bodyRef.current;
      el.scrollTop = el.scrollHeight;
    }
  }, [filteredLogs.length, paused, isAtBottom]);

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
    <div style={{ padding: 'var(--space-6)', height: '100%', display: 'flex', flexDirection: 'column', position: 'relative' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', marginBottom: 'var(--space-4)', flexShrink: 0, flexWrap: 'wrap' }}>
        <div>
          <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 700, margin: 0 }}>{t('logs.title')}</h1>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', marginTop: 'var(--space-1)' }}>
            <p style={{ fontSize: 'var(--text-sm)', color: 'var(--text-secondary)', margin: 0 }}>
              {t('logs.subtitle')}
            </p>
            <span style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 'var(--space-1)',
              fontSize: 'var(--text-xs)',
              color: wsConnected ? 'var(--accent-green)' : 'var(--accent-red)',
            }}>
              <span style={{
                width: '8px', height: '8px', borderRadius: '50%',
                background: wsConnected ? 'var(--accent-green)' : 'var(--accent-red)',
              }} />
              {wsConnected ? t('logs.connected') : t('logs.disconnected')}
            </span>
          </div>
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
        onScroll={handleScroll}
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

      {!isAtBottom && !paused && (
        <button
          onClick={() => {
            if (bodyRef.current) {
              bodyRef.current.scrollTop = bodyRef.current.scrollHeight;
              setIsAtBottom(true);
            }
          }}
          style={{
            position: 'absolute',
            bottom: '60px',
            right: '24px',
            width: '36px',
            height: '36px',
            borderRadius: '50%',
            background: 'var(--accent-blue)',
            color: '#fff',
            border: 'none',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            boxShadow: '0 2px 8px rgba(0,0,0,0.2)',
            zIndex: 10,
            fontSize: '18px',
          }}
          title={t('logs.scrollToBottom')}
        >
          ↓
        </button>
      )}

      <div style={{ display: 'flex', gap: 'var(--space-4)', padding: 'var(--space-2) 0', fontSize: 'var(--text-xs)', color: 'var(--text-tertiary)' }}>
        <span>{t('logs.total')}: <strong>{logs.length}</strong></span>
        <span>{t('logs.showing')}: <strong>{filteredLogs.length}</strong></span>
        {paused && <span style={{ color: 'var(--accent-amber)', fontWeight: 600 }}>{t('logs.paused')}</span>}
      </div>
    </div>
  );
};

function formatEngineEvent(event: {
  type: string;
  pair_name?: string;
  path?: string;
  direction?: string;
  message?: string;
  error?: string;
}): string {
  const parts = [event.type.split('_').join(' ')];
  if (event.pair_name) parts.push(String(event.pair_name));
  if (event.path) parts.push(String(event.path));
  if (event.direction) parts.push(`direction=${event.direction}`);
  if (event.message) parts.push(String(event.message));
  if (event.error) parts.push(String(event.error));
  return parts.join(' · ');
}
