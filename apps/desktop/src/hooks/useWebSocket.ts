import { useEffect, useRef } from 'react';
import { useGameStore } from '../stores/gameStore';
import { wsClient } from '../wsClient';

export function useWebSocket() {
  const token = useGameStore((s) => s.token);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
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

      wsClient.setOnState(useGameStore.getState().setStateFromPush);
      wsClient.setOnEvent(useGameStore.getState().addEvent);
      wsClient.setOnClose(() => {
        if (mountedRef.current) {
          reconnectTimer.current = setTimeout(connect, 5000);
        }
      });

      wsClient.connect(token!);
    }

    function cleanup() {
      clearTimeout(reconnectTimer.current);
      wsClient.setOnClose(null);
      wsClient.disconnect();
    }

    return () => {
      mountedRef.current = false;
      clearTimeout(initTimer);
      cleanup();
    };
  }, [token]);
}
