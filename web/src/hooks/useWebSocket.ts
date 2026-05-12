import { useState, useEffect, useRef, useCallback } from 'react';
import type { WSEvent } from '../api/client';

interface UseWebSocketOptions {
  onEvent: (event: WSEvent) => void;
  enabled?: boolean;
}

export function useWebSocket({ onEvent, enabled = true }: UseWebSocketOptions) {
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const onEventRef = useRef(onEvent);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  onEventRef.current = onEvent;

  const connect = useCallback(() => {
    if (!enabled) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${protocol}//${window.location.host}/api/v1/events`;

    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      console.debug('[WS] Connected to event stream');
      setConnected(true);
    };

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as WSEvent;
        onEventRef.current(data);
      } catch {
        console.warn('[WS] Failed to parse message:', event.data);
      }
    };

    ws.onclose = () => {
      console.debug('[WS] Disconnected, reconnecting in 3s...');
      setConnected(false);
      wsRef.current = null;
      reconnectTimerRef.current = setTimeout(connect, 3000);
    };

    ws.onerror = (err) => {
      console.debug('[WS] Error:', err);
      ws.close();
    };
  }, [enabled]);

  useEffect(() => {
    const initialConnectTimer = setTimeout(connect, 0);

    return () => {
      clearTimeout(initialConnectTimer);
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
      }
      if (wsRef.current) {
        wsRef.current.onclose = null; // prevent reconnect on cleanup
        wsRef.current.onerror = null;
        wsRef.current.close();
      }
    };
  }, [connect]);

  return { connected };
}
