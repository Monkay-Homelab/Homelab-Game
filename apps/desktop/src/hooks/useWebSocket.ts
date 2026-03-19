import { useEffect, useRef } from 'react';
import { useGameStore } from '../stores/gameStore';

const WS_URL = (import.meta.env.VITE_API_URL || 'https://api.homelab.living')
  .replace('https://', 'wss://')
  .replace('http://', 'ws://');

export function useWebSocket() {
  const token = useGameStore(s => s.token);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>();
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;

    if (!token) {
      cleanup();
      return;
    }

    // Delay connection slightly to survive React Strict Mode double-mount
    const initTimer = setTimeout(() => {
      if (mountedRef.current) {
        connect();
      }
    }, 100);

    function connect() {
      if (!mountedRef.current) return;
      if (wsRef.current && wsRef.current.readyState <= WebSocket.OPEN) return;

      const ws = new WebSocket(`${WS_URL}/ws?token=${token}`);

      ws.onopen = () => {
        if (!mountedRef.current) {
          ws.close();
          return;
        }
        console.log('WebSocket connected');
      };

      ws.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data);
          console.log('[WS] Message received:', msg.type);
          if (msg.type === 'event') {
            const event = JSON.parse(msg.payload);
            console.log(`[EVENT via WS] ${event.severity.toUpperCase()}: ${event.name} — ${event.description}`);
            useGameStore.getState().addEvent(event);
            // Refresh state to get updated values, but skip event processing on the REST side
            useGameStore.getState().fetchState();
          }
        } catch {
          // ignore
        }
      };

      ws.onclose = () => {
        wsRef.current = null;
        if (mountedRef.current) {
          reconnectTimer.current = setTimeout(connect, 5000);
        }
      };

      wsRef.current = ws;
    }

    function cleanup() {
      clearTimeout(reconnectTimer.current);
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    }

    return () => {
      mountedRef.current = false;
      clearTimeout(initTimer);
      cleanup();
    };
  }, [token]);
}
