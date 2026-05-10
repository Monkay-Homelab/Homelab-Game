import { api, type GameState } from './api';
import { WS_URL } from './config';
import type { GameEvent } from './stores/gameStore';
import { randomId } from './utils/randomId';

const ACTION_TIMEOUT_MS = 10_000;

interface PendingRequest {
  resolve: (state: GameState) => void;
  reject: (error: Error) => void;
  timer: ReturnType<typeof setTimeout>;
}

interface ActionResultMessage {
  type: 'action_result';
  id: string;
  success: boolean;
  state: GameState;
  error: string;
}

class WSClient {
  private ws: WebSocket | null = null;
  private pending: Map<string, PendingRequest> = new Map();
  private onState: ((state: GameState) => void) | null = null;
  private onEvent: ((event: GameEvent) => void) | null = null;
  private onClose: (() => void) | null = null;

  connect(token: string): void {
    // Close any existing connection before opening a new one
    if (this.ws) {
      this.ws.onclose = null;
      this.ws.close();
      this.ws = null;
    }

    const ws = new WebSocket(`${WS_URL}/ws?token=${token}`);

    ws.onopen = () => {
      console.log('[wsClient] WebSocket connected');
    };

    ws.onmessage = (e: MessageEvent) => {
      this.handleMessage(e.data as string);
    };

    ws.onclose = () => {
      console.log('[wsClient] WebSocket closed');
      this.ws = null;
      this.onClose?.();
    };

    this.ws = ws;
  }

  disconnect(): void {
    // Reject all pending requests immediately with "connection lost"
    for (const [id, pending] of this.pending) {
      clearTimeout(pending.timer);
      pending.reject(new Error('connection lost'));
      this.pending.delete(id);
    }
    this.pending.clear();

    if (this.ws) {
      this.ws.onclose = null;
      this.ws.close();
      this.ws = null;
    }
  }

  isConnected(): boolean {
    return this.ws !== null && this.ws.readyState === WebSocket.OPEN;
  }

  sendAction(action: string, payload?: Record<string, unknown>): Promise<GameState> {
    if (!this.isConnected()) {
      // Fallback to HTTP
      return api.action(action, payload);
    }

    const id = randomId();

    return new Promise<GameState>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error('Action timed out'));
      }, ACTION_TIMEOUT_MS);

      this.pending.set(id, { resolve, reject, timer });

      this.ws!.send(
        JSON.stringify({
          type: 'action',
          id,
          action,
          payload,
        }),
      );
    });
  }

  setOnState(cb: (state: GameState) => void): void {
    this.onState = cb;
  }

  setOnEvent(cb: (event: GameEvent) => void): void {
    this.onEvent = cb;
  }

  setOnClose(cb: (() => void) | null): void {
    this.onClose = cb;
  }

  private handleMessage(data: string): void {
    try {
      const msg = JSON.parse(data);
      switch (msg.type) {
        case 'action_result':
          this.handleActionResult(msg as ActionResultMessage);
          break;
        case 'state':
          this.onState?.(msg.payload);
          break;
        case 'event': {
          const event = typeof msg.payload === 'string' ? JSON.parse(msg.payload) : msg.payload;
          this.onEvent?.(event as GameEvent);
          break;
        }
        // Unknown types: ignore silently
      }
    } catch {
      // Ignore malformed messages
    }
  }

  private handleActionResult(msg: ActionResultMessage): void {
    const pending = this.pending.get(msg.id);
    if (!pending) return; // Orphaned response (already timed out)

    clearTimeout(pending.timer);
    this.pending.delete(msg.id);

    if (msg.success) {
      pending.resolve(msg.state);
    } else {
      pending.reject(new Error(msg.error));
    }
  }
}

export const wsClient = new WSClient();
