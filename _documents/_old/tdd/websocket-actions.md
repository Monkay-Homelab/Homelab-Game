---
project: "project"
maturity: "draft"
last_updated: "2026-03-21"
updated_by: "@staff-engineer"
scope: "Migrate player actions from HTTP POST to WebSocket request/response, eliminating the race condition between HTTP action submission and WebSocket state push"
owner: "@staff-engineer"
dependencies:
  - websocket-state-push.md
  - ../spec/architecture.md
  - ../spec/performance.md
  - ../spec/security.md
---

# WebSocket Actions Migration

## 1. Problem Statement

### What

Move all player game actions (buy_hardware, run_job, deploy_service, sell_bitcoin, etc.) from
HTTP POST `/api/game/action` to WebSocket request/response messages. The server processes the
action and pushes the updated state back over the same WebSocket connection, unifying state
mutation and state delivery into a single channel.

### Why Now

Users experience two related timing problems:

1. **Button activation delay.** After performing an action (e.g., buying hardware), the UI does
   not reflect the change for up to 5 seconds. The HTTP POST returns updated state, but the
   next WS tick push (every 5 seconds) may overwrite it with slightly stale state computed
   from the tick goroutine. Between the HTTP response and the next WS push, the client's state
   oscillates.

2. **Actions fail on stale state.** The client reads state from a WS push, the user clicks a
   button, the action is sent via HTTP POST, but by the time the server processes it, the
   game state has changed (the tick goroutine advanced it). The action's preconditions (e.g.,
   sufficient compute units) may no longer hold, causing the action to fail with a confusing
   error.

**Root cause:** The hybrid HTTP+WebSocket architecture creates a split-brain problem. State
mutations flow through HTTP POST, but state delivery flows through WebSocket push. These two
channels share a per-user mutex on the server side, but the client has no way to correlate
them. An HTTP action response and a WS tick push can arrive in any order, and the client
cannot distinguish "this state is the result of my action" from "this state is a periodic tick."

The prior TDD (`websocket-state-push.md`) moved state delivery to WebSocket and added a
post-action WS push from `PerformAction`. This was designed to help, but the fundamental
problem remains: the action still goes through a separate HTTP channel, and the HTTP response
and WS push are two distinct messages that can race with each other and with tick pushes.

**The fix:** Move the action itself to WebSocket. When the client sends an action over WS, the
server processes it and responds over WS with the result (success + updated state, or error).
Since both the action and the state update travel on the same connection, the client can
reliably correlate an action request with its response, and there is no cross-channel race.

### Business Context

This is a user-facing quality issue. The timing bugs make the game feel broken -- buttons
appear unresponsive, purchases seem to fail randomly, and resource counters jump unexpectedly.
For a clicker/idle game where responsiveness is a core UX pillar, this directly damages
retention.

### Constraints

- **Auth (register/login) stays on HTTP.** JWT token is needed before a WS connection can be
  established. No change to auth flow.
- **Game config stays on HTTP.** Static, fetched once, cached. No timing sensitivity.
- **Social/leaderboard stays on HTTP for now.** These endpoints do not cause timing issues
  (they read independent data, not the game state under mutation).
- **GET /api/game/state stays on HTTP.** Used as a bootstrap/fallback endpoint only.
- **POST /api/game/action stays on HTTP as a fallback.** During the migration period and for
  any non-WS clients (future mobile app, stress tester), the HTTP endpoint continues to work.
  It is not removed, only deprioritized on the desktop client.
- **Server-authoritative model is preserved.** The client sends an action request; the server
  validates, processes, and returns the authoritative state. The client cannot fabricate state.
- **Existing WS push (ticks, events) continues.** Tick pushes deliver state every 5 seconds.
  Action responses are an additional message type on the same connection.
- **Single-process deployment assumption** continues (in-memory hub, per-user mutexes).

### Acceptance Criteria

1. The desktop client sends player actions (run_job, buy_hardware, etc.) as WebSocket messages
   instead of HTTP POST requests.
2. Each action request includes a client-generated request ID. The server's response includes
   the same request ID, allowing the client to correlate request and response.
3. On successful action processing, the server responds with a WS message containing the
   request ID, a success indicator, and the full updated game state.
4. On action failure (validation error, insufficient resources, etc.), the server responds with
   a WS message containing the request ID, an error indicator, and an error message string.
5. The client resolves/rejects the action's Promise based on the correlated WS response,
   maintaining the existing async/await pattern in store actions.
6. After a successful WS action response, the client does not experience state oscillation from
   subsequent tick pushes (the tick push will contain the same or newer state).
7. The HTTP POST `/api/game/action` endpoint continues to work unchanged for backward
   compatibility.
8. Action processing on the server uses the same per-user mutex, engine, and persistence logic
   regardless of whether the action arrived via HTTP or WebSocket.
9. Rate limiting is applied to WS actions at the same rate as HTTP actions (7,200/min/user).
10. The client falls back to HTTP POST if the WebSocket is not connected (e.g., during
    reconnection).

---

## 2. Context & Prior Art

### Current Architecture (Post websocket-state-push.md)

The `websocket-state-push.md` TDD was implemented. The current state is:

**Server:**
- WebSocket hub (`ws/hub.go`) manages per-user `Client` structs with buffered `send` channels
  and dedicated `writePump`/`readPump` goroutines. Write safety is guaranteed.
- `readPump` reads and **discards** all client messages (line 227: `c.conn.ReadMessage()`
  returns are ignored). The WS is currently server-push only.
- A per-user tick goroutine (`GameHandler.OnConnect`) pushes full state every 5 seconds via
  `runUserTick`.
- `PerformAction` HTTP handler acquires the per-user mutex, processes the action, returns state
  via HTTP, and also pushes state over WS (lines 744-749 of `game.go`).
- The per-user mutex (`userMutexMap`) serializes all state mutations for a given user -- both
  tick goroutines and HTTP action handlers acquire it.

**Client:**
- `useWebSocket.ts` connects WS on login, handles `type: "state"` (calls
  `setStateFromPush`) and `type: "event"` (calls `addEvent`).
- `gameStore.ts` action methods (buyHardware, runJob, etc.) call `api.action()` which uses
  `fetch()` to POST to `/api/game/action`. The HTTP response updates the store state.
- The WS `setStateFromPush` unconditionally replaces state, which can overwrite the
  just-received HTTP action response with a tick push that was computed before the action.

### The Race Condition in Detail

Consider this sequence:

```
T=0.0s  Tick goroutine fires, acquires user lock
T=0.0s  Tick reads state from DB: compute_units = 5000
T=0.0s  Tick runs ProcessIdleProgress, compute_units = 5050
T=0.0s  Tick persists state, pushes WS {"type":"state"} with 5050 CU
T=0.0s  Tick releases user lock

T=0.1s  Client receives WS push: state.compute_units = 5050
T=0.1s  User clicks "Buy Hardware" (costs 3000 CU, user has 5050)

T=0.2s  Client sends HTTP POST /api/game/action {type: "buy_hardware"}
T=0.2s  Server receives HTTP request, acquires user lock
T=0.2s  Server reads state: compute_units = 5050
T=0.2s  Server processes action: compute_units = 2050, new hardware added
T=0.2s  Server persists state, returns HTTP response with 2050 CU
T=0.2s  Server also pushes WS {"type":"state"} with 2050 CU
T=0.2s  Server releases user lock

T=0.2s  Client receives HTTP response: state = 2050 CU + new hardware
        Store updates to 2050 CU. UI shows purchase succeeded.

T=0.3s  Client receives WS push from PerformAction: state = 2050 CU
        Store updates again (redundant but harmless).

T=5.0s  Tick goroutine fires again, acquires lock
T=5.0s  Tick reads CURRENT state: 2050 CU (correct, action was persisted)
T=5.0s  Tick pushes WS state: 2100 CU (idle progress added)
T=5.0s  Client receives WS push: state = 2100 CU. Correct.
```

In the happy path above, things work. But now consider network latency on the HTTP path:

```
T=0.0s  Tick pushes WS state: 5050 CU
T=0.1s  Client receives WS push: 5050 CU. User clicks "Buy Hardware."
T=0.2s  Client sends HTTP POST (starts flight)

T=2.0s  HTTP request arrives at server (slow network / proxy buffering)
T=2.0s  Server acquires lock, processes action: 2050 CU + hardware
T=2.0s  Server returns HTTP response (starts flight back)
T=2.0s  Server pushes WS state: 2050 CU

T=2.1s  Client receives WS push: 2050 CU + hardware. Store updates.
        UI shows hardware purchased.

T=2.5s  Client receives HTTP response: 2050 CU + hardware.
        Store updates AGAIN (same data, no visible problem here).

T=5.0s  Meanwhile, between T=0.0 and T=2.0, a tick fired at T=5.0:
        Tick reads state as it was BEFORE the action (5050 CU) -- NO!
        Actually, the tick would block on the mutex until T=2.0.
        But if the HTTP request is slow to arrive, the tick at T=5.0
        runs first, pushes 5100 CU, THEN the HTTP action arrives.
```

The real problem scenario:

```
T=0.0s  Tick pushes WS: 5050 CU
T=0.1s  User clicks buy_hardware (costs 3000)
T=0.2s  HTTP POST starts flight

T=5.0s  Tick goroutine fires, acquires lock (HTTP hasn't arrived yet)
T=5.0s  Tick reads state: 5050 CU, runs idle, pushes WS: 5100 CU
T=5.0s  Tick releases lock

T=5.0s  Client receives WS push: 5100 CU (state.hardware unchanged)
        Store replaces state. The user's pending action is still in flight.
        UI: hardware not purchased, CU jumped to 5100. Confusing.

T=5.1s  HTTP request finally arrives. Server acquires lock.
        Server reads state: 5100 CU (tick advanced it).
        Action processes: 5100 - 3000 = 2100 CU + hardware. Success.
        HTTP response sent. WS push sent.

T=5.2s  Client receives HTTP response: 2100 CU + hardware.
        Store updates. UI shows purchase. But user saw a 5-second delay.
```

In this scenario the action eventually succeeds, but the 5-second gap between click and
confirmation -- combined with the intermediate state push showing unchanged hardware -- makes
the UI feel broken. In worse cases (user is near the cost threshold), the tick's idle progress
pushes state past a boundary that makes the action succeed with unexpected math, or the user
clicks again thinking it failed, triggering a double-purchase.

**With WS actions, this race is eliminated.** The action goes over the same WS connection.
The server processes it atomically (same mutex) and the response comes back on the same
connection. No intermediate tick push can interleave, because the server controls push
ordering on the single WS send channel.

### How Others Solve This

**Standard pattern for game WebSocket RPC:** Client sends `{"id": "abc123", "type": "action",
"action": "buy_hardware", "payload": {"name": "Raspberry Pi"}}`. Server processes, responds
with `{"id": "abc123", "type": "action_result", "success": true, "state": {...}}` or
`{"id": "abc123", "type": "action_error", "error": "insufficient compute units"}`. The
client matches responses to requests by the `id` field.

This is the pattern used by:
- **Socket.IO acknowledgements** (callback-based request/response over WS)
- **JSON-RPC over WebSocket** (id-correlated request/response)
- **Phoenix Channels** (ref-correlated push/reply)
- **Colyseus** (game framework, message-id correlation)

The pattern is well-established. We adopt the simplest variant: a string request ID chosen by
the client, echoed by the server.

---

## 3. Alternatives Considered

### Alternative A: Fix by Sequencing HTTP Response and WS Push (Minimal Change)

Keep actions on HTTP but add a sequence number to state payloads. The client ignores WS pushes
with sequence numbers lower than the last HTTP response.

**Strengths:** Minimal backend change. No new WS message types. Client adds a simple
comparison.

**Weaknesses:** Adds complexity to every state consumer on the client. Does not fix the core
latency issue (HTTP roundtrip is still slower than WS message). Does not help when the HTTP
request is slow to arrive at the server (the tick still runs and pushes intermediate state).
The "ignore stale push" approach can cause the client to miss legitimate state changes that
happened concurrently (e.g., a random event during the action's flight time).

**Verdict:** Rejected. Treats the symptom (stale state overwrite) without fixing the root
cause (split-channel communication). Adds permanent complexity for a partial fix.

### Alternative B: Move Actions to WS Without Request Correlation

Send actions as fire-and-forget WS messages. The next WS push (tick or action-triggered)
carries the result. The client infers success/failure from the state change.

**Strengths:** Very simple server implementation (just route the message to `PerformAction`
logic). No request/response correlation needed.

**Weaknesses:** The client cannot distinguish "action succeeded" from "tick happened to push
similar state." Error messages are lost -- if the action fails validation, the client has no
way to display a specific error. The existing store pattern (async action methods that
resolve/reject with state or error) would need a complete rewrite. No way to show loading
spinners or disable buttons during action processing.

**Verdict:** Rejected. The inability to deliver error messages to the user makes this a
non-starter. The game shows specific error messages ("not enough compute units",
"power limit exceeded") that are essential for player understanding.

### Alternative C: Move Actions to WS with Request ID Correlation (Recommended)

Client sends an action message with a unique request ID. Server processes the action and
responds with a message containing the same request ID, the success/error status, and (on
success) the full updated state.

**Strengths:** Clean request/response semantics. The client can correlate responses to
requests, enabling the existing Promise-based async pattern. Error messages are delivered
directly. Works naturally with the single WS connection model. The action and its response
travel on the same channel, eliminating cross-channel races.

**Weaknesses:** Requires client-side request ID generation and a pending-request map. Adds a
new WS message type (client-to-server action, server-to-client action result). Slightly more
complex than Alternative B.

**Mitigations:** Request ID generation is trivial (crypto.randomUUID or incrementing counter).
The pending-request map is a simple `Map<string, {resolve, reject}>`. The complexity is modest
and well-encapsulated.

**Verdict:** Recommended. Provides the correctness guarantees needed to fix the race condition
while maintaining the existing developer experience (async/await actions in the store).

---

## 4. Architecture & System Design

### 4.1 High-Level Flow (After Migration)

```
Desktop Client                        Backend Server                    PostgreSQL
     |                                     |                                |
     |-- POST /api/auth/login ------------>|                                |
     |<-- JWT token -----------------------|                                |
     |                                     |                                |
     |-- WS /ws?token=xxx --------------->|                                |
     |<== WebSocket established ==========>|                                |
     |                                     |-- [tick goroutine starts]      |
     |<== WS: {"type":"state",...} ========|   (immediate state push)       |
     |                                     |                                |
     |   [5s tick elapses]                 |                                |
     |<== WS: {"type":"state",...} ========|-- runUserTick ----------------->|
     |                                     |                                |
     |== WS: {"type":"action",             |                                |
     |    "id":"abc", "action":"buy_hw",   |                                |
     |    "payload":{"name":"RPi"}} ======>|                                |
     |                                     |-- acquire user mutex           |
     |                                     |-- 8 SELECTs ----------------->|
     |                                     |<-- game data ------------------|
     |                                     |-- ProcessIdleProgress()        |
     |                                     |-- ProcessAction()              |
     |                                     |-- INSERT/UPDATE/DELETE -------->|
     |                                     |-- release user mutex           |
     |<== WS: {"type":"action_result",     |                                |
     |    "id":"abc", "success":true,      |                                |
     |    "state":{...}} =================|                                 |
     |                                     |                                |
     |   [UI updates from action_result]   |                                |
     |                                     |                                |
     |   [5s tick elapses]                 |                                |
     |<== WS: {"type":"state",...} ========|-- runUserTick ----------------->|
     |   [state consistent with action]    |                                |
```

### 4.2 WebSocket Message Protocol

#### Client-to-Server: Action Request

```json
{
  "type": "action",
  "id": "f47ac10b-58cc",
  "action": "buy_hardware",
  "payload": {"name": "Raspberry Pi 5"}
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Always `"action"` |
| `id` | string | Yes | Client-generated unique request ID (UUID or monotonic counter) |
| `action` | string | Yes | Action type (same values as HTTP: `run_job`, `buy_hardware`, etc.) |
| `payload` | object | No | Action-specific payload (same structure as HTTP request body `payload` field) |

#### Server-to-Client: Action Result (Success)

```json
{
  "type": "action_result",
  "id": "f47ac10b-58cc",
  "success": true,
  "state": {
    "id": "...",
    "tier": "rack_12u",
    "compute_units": 2050,
    "hardware": [...],
    ...
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"action_result"` |
| `id` | string | Echoed request ID from the client |
| `success` | bool | `true` |
| `state` | object | Full game state (same schema as `fullStateResponse` / `GET /api/game/state`) |

#### Server-to-Client: Action Result (Error)

```json
{
  "type": "action_result",
  "id": "f47ac10b-58cc",
  "success": false,
  "error": "not enough compute units"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"action_result"` |
| `id` | string | Echoed request ID from the client |
| `success` | bool | `false` |
| `error` | string | Human-readable error message (same strings as current HTTP error responses) |

#### Existing Messages (Unchanged)

- **Server-to-Client `"state"`:** Periodic tick push. Full game state. No `id` field.
- **Server-to-Client `"event"`:** Random event notification. No `id` field.

### 4.3 Component Changes

#### Backend: `ws/hub.go` -- Process Incoming Messages

Currently `readPump` discards all incoming messages. It must now parse and route them.

**Change to `readPump`:**

```
for {
    _, message, err := c.conn.ReadMessage()
    if err != nil {
        break
    }
    // Route to message handler
    if c.hub.OnMessage != nil {
        c.hub.OnMessage(c.UserID, message)
    }
}
```

**New `Hub` field:**

```
OnMessage func(userID string, data []byte)
```

This callback is set by the `GameHandler` during wiring in `main.go`, same pattern as the
existing `OnConnect` / `OnDisconnect` callbacks.

**Why a callback, not inline processing:** The hub should remain decoupled from game logic.
It handles connection lifecycle and message routing. The game handler owns action processing.

**Message size limit:** The existing `MaxBodySize` middleware caps HTTP bodies at 64KB. For
consistency, set a read limit on the WebSocket connection: `c.conn.SetReadLimit(65536)`. This
prevents a malicious client from sending oversized messages. This is a defense-in-depth
measure; action payloads are tiny (< 1KB).

#### Backend: `handlers/game.go` -- WS Action Handler

**New method: `GameHandler.HandleWSAction(userID string, data []byte)`**

This method is called by the hub's `OnMessage` callback. It:

1. Deserializes the incoming message into an action request struct.
2. Validates the `type` field is `"action"` (future-proofing for other client-to-server
   message types).
3. Validates the `id` field is non-empty.
4. Checks WS-specific rate limiting for the user.
5. Calls the existing action processing logic (same code path as `PerformAction`).
6. Sends the response (success + state, or error) over WS via `hub.SendToUser`.

**Code reuse with HTTP handler:** The core action logic in `PerformAction` (lines 503-750 of
`game.go`) must be refactored into a shared internal method that both `PerformAction` (HTTP)
and `HandleWSAction` (WS) call. This method:

```
func (h *GameHandler) processAction(ctx context.Context, userID string, actionType string,
    payload json.RawMessage) (*fullStateResponse, error)
```

- Acquires per-user mutex
- Loads state, runs ProcessIdleProgress, calculates bonuses
- Calls engine.ProcessAction
- Persists results
- Builds and returns fullStateResponse
- Releases per-user mutex

The HTTP handler (`PerformAction`) calls this method and writes the result to the HTTP
response writer. The WS handler (`HandleWSAction`) calls this method and sends the result
over the WebSocket.

**Important:** `HandleWSAction` is called from the `readPump` goroutine. Action processing
involves database I/O and mutex acquisition, which can block. This is acceptable because:

- There is exactly one `readPump` per user connection.
- Actions for the same user are naturally serialized (the `readPump` processes one message at
  a time from the single WS connection, plus the per-user mutex prevents concurrency with
  ticks).
- The `readPump` blocking on action processing is equivalent to the current model where the
  client waits for the HTTP response before sending the next action.

However, to prevent the readPump from being blocked by action processing (which would prevent
pong handling and disconnect detection), the `OnMessage` callback should dispatch action
processing to a separate goroutine:

```go
hub.OnMessage = func(userID string, data []byte) {
    go gameHandler.HandleWSAction(userID, data)
}
```

This preserves the readPump's ability to handle connection lifecycle. The per-user mutex
serializes concurrent action goroutines.

#### Backend: Rate Limiting for WS Actions

The current rate limiter is HTTP middleware. WS messages bypass it entirely. A new rate
checking mechanism is needed for WS actions.

**Approach:** Reuse the existing `checkRate` function from `middleware/ratelimit.go`. The
`HandleWSAction` method calls `checkRate("game:user:" + userID, 7200)` directly. If the
rate is exceeded, it sends an `action_result` error response with `"too many requests"` and
returns without processing the action.

**Export requirement:** The `checkRate` function in `middleware/ratelimit.go` is currently
unexported. It needs to be exported (renamed to `CheckRate`) or a public wrapper function
needs to be added. The simplest approach is to add an exported function:

```go
func CheckGameActionRate(userID string) bool {
    return checkRate("game:user:"+userID, 7200)
}
```

This keeps the rate limit configuration (7200/min) in the middleware package where all rate
limits are defined, and avoids exposing internal implementation details.

#### Backend: `PerformAction` HTTP Handler -- Remove WS Push

Once WS actions are the primary path, the redundant WS push from the HTTP `PerformAction`
handler (lines 744-749 of `game.go`) should be removed. This push was added by the prior TDD
to reduce stale-state issues, but with actions moving to WS, it becomes unnecessary for
WS-connected clients. Keeping it creates the very duplication issue this TDD solves.

**Timing:** This removal happens in Phase 3 (after the client is migrated to WS actions).
During the overlap period (Phases 1-2), the HTTP push is kept for backward compatibility.

#### Frontend: `api.ts` -- WS Action Sender

Add a new module or extend `api.ts` with a WebSocket action sender that:

1. Generates a unique request ID (using `crypto.randomUUID()`).
2. Sends the action message over the WebSocket connection.
3. Returns a Promise that resolves with the game state or rejects with an error.
4. Maintains a `Map<string, {resolve, reject, timeout}>` of pending requests.
5. On receiving an `action_result` message (from the WS `onmessage` handler), looks up the
   pending request by `id`, and resolves or rejects the Promise.
6. Implements a timeout (10 seconds) per request. If no response arrives, rejects the Promise
   with a timeout error. This handles the case where the server crashes or the WS disconnects
   during action processing.
7. Falls back to HTTP POST if the WebSocket is not connected.

**Architecture choice: Separate module vs. extending api.ts.**

The WS action sender needs access to the WebSocket connection. Currently, the WS connection
lives in `useWebSocket.ts` (a React hook). The API layer (`api.ts`) has no access to it.

Two options:

**Option A: WS connection manager as a singleton module.** Extract the WebSocket connection
from the React hook into a standalone module (`wsClient.ts`) that manages the connection
lifecycle, message routing, and the pending-request map. The `useWebSocket` hook becomes a
thin wrapper that subscribes to state/event pushes. The store actions call `wsClient.sendAction()`
directly.

**Option B: Keep WS in the hook, pass a sendAction function to the store.** The hook exposes
a `sendAction` method. The store accesses it through a ref or global.

Option A is cleaner. The WebSocket connection is infrastructure, not UI. It should not live
inside a React hook. Moving it to a singleton module makes it accessible from the store (which
is also a singleton) without prop drilling or global refs.

**Recommended: Option A.**

#### Frontend: New `wsClient.ts` Module

```typescript
// Singleton WebSocket client with action request/response correlation

class WSClient {
  private ws: WebSocket | null = null;
  private pending: Map<string, {
    resolve: (state: GameState) => void;
    reject: (error: Error) => void;
    timer: ReturnType<typeof setTimeout>;
  }> = new Map();
  private onState: ((state: GameState) => void) | null = null;
  private onEvent: ((event: GameEvent) => void) | null = null;

  connect(url: string, token: string): void { ... }
  disconnect(): void { ... }
  isConnected(): boolean { ... }

  sendAction(action: string, payload?: Record<string, unknown>): Promise<GameState> {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      // Fallback to HTTP
      return api.action(action, payload);
    }

    const id = crypto.randomUUID();
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error('Action timed out'));
      }, 10000);

      this.pending.set(id, { resolve, reject, timer });

      this.ws!.send(JSON.stringify({
        type: 'action',
        id,
        action,
        payload,
      }));
    });
  }

  // Called from onmessage handler
  private handleMessage(data: string): void {
    const msg = JSON.parse(data);
    switch (msg.type) {
      case 'action_result':
        this.handleActionResult(msg);
        break;
      case 'state':
        this.onState?.(msg.payload);
        break;
      case 'event':
        this.onEvent?.(JSON.parse(msg.payload));
        break;
    }
  }

  private handleActionResult(msg: ActionResultMessage): void {
    const pending = this.pending.get(msg.id);
    if (!pending) return; // Orphaned response (timed out)

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
```

#### Frontend: `useWebSocket.ts` -- Thin Wrapper

The hook becomes a lifecycle manager that calls `wsClient.connect()` on mount/token change
and `wsClient.disconnect()` on unmount. It registers the `onState` and `onEvent` callbacks
to forward to the Zustand store. It no longer owns the WebSocket connection directly.

#### Frontend: `gameStore.ts` -- Use wsClient

Every action method changes from:

```typescript
buyHardware: async (name) => {
  const state = await api.action('buy_hardware', { name });
  set({ state });
}
```

To:

```typescript
buyHardware: async (name) => {
  const state = await wsClient.sendAction('buy_hardware', { name });
  set({ state });
}
```

The `try/catch` pattern and `set({ error })` handling remain identical. The only change is
the transport: `wsClient.sendAction` instead of `api.action`.

The `runJob` optimistic update pattern also stays the same -- it applies the local prediction
before calling `wsClient.sendAction`, and the response overwrites it.

### 4.4 Mutex and Ordering Guarantees

**Per-user mutex:** Both the tick goroutine and the WS action handler acquire
`userLocks.Lock(userID)` before any state mutation. This guarantees:

- A tick and an action never run concurrently for the same user.
- Two concurrent WS action messages (if the client sends them rapidly) are serialized.
- An action always sees the latest persisted state (not half-written tick state).

**WS send channel ordering:** The hub's `Client.send` channel is a buffered channel. Messages
are written to it from multiple sources (tick goroutine, action handler goroutine). The
`writePump` goroutine reads from the channel and writes to the connection in FIFO order.

This means: if a tick pushes state, then an action processes and pushes a result, the client
receives them in that order. The action result always arrives after any tick push that preceded
it in real time. This is the key ordering guarantee that eliminates the race condition.

**Edge case: tick fires during action processing.** The tick goroutine is blocked on the mutex
while the action processes. After the action releases the mutex, the tick acquires it, reads
the post-action state, processes idle progress, and pushes. The client receives: (1) the
action result with post-action state, (2) the tick push with slightly newer state (idle
progress added). This is correct -- each push represents a strictly later state.

### 4.5 Tick Push After Action

When an action is processed via WS, the server sends an `action_result` message. Should it
also send a separate `"state"` push?

**No.** The `action_result` already contains the full state. Sending an additional `"state"`
push would be redundant (same data, two messages). The client handles the state from the
`action_result` message. The next regular tick push (up to 5 seconds later) will contain
the post-action state plus idle progress, which is a natural update.

The HTTP `PerformAction` handler currently pushes state over WS after returning the HTTP
response (lines 744-749). This push should eventually be removed (Phase 3) since WS-connected
clients receive state via the `action_result`. For HTTP-only clients (no WS), the HTTP
response already contains the state. The WS push from `PerformAction` becomes dead code once
no client depends on it.

### 4.6 Disconnection and Fallback

**Client WS disconnection during action flight:**

If the WebSocket disconnects while an action is in flight (message sent, no response yet):

1. The pending request's 10-second timeout fires.
2. The Promise rejects with a timeout error.
3. The store sets the error state, displaying an error to the user.
4. On reconnect, the server pushes fresh state immediately (existing behavior from
   `OnConnect`), which may or may not reflect the action (depends on whether the server
   processed it before disconnect).

This is acceptable. The user sees an error and gets fresh state on reconnect. If the action
was actually processed, the reconnect state will reflect it. If not, the user can retry.

**HTTP fallback:** If `wsClient.isConnected()` is false, `sendAction` falls back to
`api.action()` (HTTP POST). This handles:

- The brief window during reconnection (5-second retry delay).
- Any future client that does not use WebSocket.
- The stress tester, which uses HTTP directly.

### 4.7 Clean-Up of Pending Requests on Disconnect

When the WebSocket disconnects, all pending requests should be rejected immediately (not
waiting for the 10-second timeout). The `wsClient.disconnect()` method iterates the pending
map, rejects all Promises with a "connection lost" error, and clears the map.

---

## 5. Data Models & Storage

No database schema changes. No new tables. No migration.

The only new data structures are in-memory:

- **Server:** The incoming WS action message struct (parsed from JSON, not persisted).
- **Client:** The `pending` map in `wsClient.ts` (request correlation, not persisted).

---

## 6. API Contracts

### Modified: WebSocket Connection

**Before:** Server-push only. Client messages discarded.
**After:** Bidirectional. Client sends `type: "action"` messages. Server responds with
`type: "action_result"` messages. Server continues to push `type: "state"` (ticks) and
`type: "event"` (random events).

### Unchanged: HTTP Endpoints

All HTTP endpoints remain unchanged:

| Endpoint | Change |
|----------|--------|
| `POST /api/auth/register` | None |
| `POST /api/auth/login` | None |
| `GET /api/game/config` | None |
| `GET /api/game/state` | None |
| `POST /api/game/action` | None (kept for backward compatibility) |
| `GET /ws` | Connection upgrade unchanged; bidirectional messages added |
| All `/api/social/*` | None |

### Backward Compatibility

Old clients that do not send WS actions continue to work via HTTP POST. Old clients that do
not handle `type: "action_result"` messages will ignore them (unknown types fall through the
message handler's switch). No breaking changes.

---

## 7. Migration & Rollout

### Phase 1: Backend -- Enable WS Message Processing (Complexity: M)

**Dependencies:** None (the existing WS infrastructure from `websocket-state-push.md` is
already deployed).

**Files changed:** `ws/hub.go`, `handlers/game.go`, `middleware/ratelimit.go`, `cmd/server/main.go`

1. **`ws/hub.go`:** Add `OnMessage func(userID string, data []byte)` field to `Hub`. In
   `readPump`, instead of discarding messages, call `c.hub.OnMessage(c.UserID, message)` if
   the callback is set. Add `c.conn.SetReadLimit(65536)` in `readPump` initialization.

2. **`middleware/ratelimit.go`:** Export a function `CheckGameActionRate(userID string) bool`
   that calls the existing `checkRate("game:user:" + userID, 7200)`.

3. **`handlers/game.go`:**
   - Extract the core action logic from `PerformAction` into a shared method
     `processAction(ctx, userID, actionType, payload) (*fullStateResponse, error)`.
   - Refactor `PerformAction` to call `processAction` and write the result to HTTP.
   - Add `HandleWSAction(userID string, data []byte)` that:
     a. Parses the message into `{type, id, action, payload}`.
     b. Validates `type == "action"` and `id != ""`.
     c. Checks rate limit via `middleware.CheckGameActionRate(userID)`.
     d. Calls `processAction(ctx, userID, action, payload)`.
     e. Sends `action_result` (success or error) via `hub.SendToUser`.
   - Add a new struct for the WS action result message and serialization.

4. **`cmd/server/main.go`:** Wire `wsHub.OnMessage` to a function that dispatches to
   `gameHandler.HandleWSAction` in a new goroutine.

**Verification:** After this phase, send a WS action message from a test client (e.g.,
`websocat` or the stress tester) and verify the server responds with an `action_result`.
The existing desktop client is unchanged and continues to use HTTP POST.

### Phase 2: Frontend -- WS Client Module and Store Migration (Complexity: M)

**Dependencies:** Phase 1 (server must handle WS actions).

**Files changed:** New `wsClient.ts`, modified `useWebSocket.ts`, `gameStore.ts`

1. **Create `apps/desktop/src/wsClient.ts`:** Implement the singleton WS client with
   `connect`, `disconnect`, `sendAction`, and message routing. Include the pending-request
   map with timeout handling and disconnect cleanup.

2. **Modify `useWebSocket.ts`:** Replace direct WebSocket management with calls to
   `wsClient.connect()` / `wsClient.disconnect()`. Register `onState` and `onEvent` callbacks
   that forward to the Zustand store (same behavior as current `onmessage` handler).

3. **Modify `gameStore.ts`:** Change all action methods to use `wsClient.sendAction()` instead
   of `api.action()`. The try/catch pattern, optimistic updates, and error handling remain
   identical. Import `wsClient` and call `wsClient.sendAction(type, payload)`.

4. **Keep `api.action()` in `api.ts`:** Do not remove it. It serves as the HTTP fallback and
   is called by `wsClient.sendAction` when WS is disconnected.

**Verification:** Play the game normally. All actions should work via WS. Open browser devtools
Network tab -- no HTTP POST to `/api/game/action` should appear during normal play. Verify
error messages still display correctly (e.g., try to buy hardware you cannot afford). Test
WS disconnection (disable network briefly) and verify fallback to HTTP, then recovery.

### Phase 3: Cleanup (Complexity: S)

**Dependencies:** Phase 2 confirmed stable.

**Files changed:** `handlers/game.go`

1. **Remove the WS state push from `PerformAction` HTTP handler** (lines 744-749 of
   `game.go`). This push is now redundant:
   - WS-connected clients receive state via `action_result` (for WS actions) or do not use
     HTTP actions at all.
   - HTTP-only clients (no WS) receive state via the HTTP response.

2. **Consider:** Remove `GET /api/game/state` from the polling codepath in App.tsx. Verify
   that the only call to `fetchState` is the bootstrap on login (this should already be the
   case from the prior TDD, but confirm).

3. **Log cleanup:** Add structured log for WS action processing (action type, user ID,
   duration, success/error) matching the level of logging in the HTTP handler.

---

## 8. Risks & Open Questions

### Known Risks

**R1: readPump goroutine blocked by slow action dispatch.**

If the `OnMessage` callback dispatches action processing synchronously (not in a new
goroutine), the readPump blocks until the action completes. During this time, the readPump
cannot read pong frames, which could cause a false disconnect if the action takes longer than
the pong timeout (45 seconds).

- Mitigation: The `OnMessage` callback dispatches to a new goroutine (`go gameHandler.HandleWSAction(...)`). The readPump returns immediately to reading. The per-user mutex serializes the action goroutine with tick goroutines.

**R2: Client sends actions faster than the server can process them.**

If the user clicks rapidly, multiple action goroutines may be spawned. They serialize on the
per-user mutex, creating a queue. If the queue grows, responses are delayed.

- Mitigation: The existing rate limit (7200/min = 120/sec) caps throughput. Legitimate play
  generates 1-5 actions/sec. The per-user mutex means at most one action processes at a time;
  queued goroutines block on the mutex and process sequentially. The 10-second client-side
  timeout prevents indefinite waits.
- Mitigation: The client can implement local debouncing for rapid clicks (e.g., disable the
  button until the current action resolves). This is a UX concern, not an architecture concern.

**R3: Request ID collisions.**

If two requests have the same ID, the second response overwrites the first in the pending map,
causing the first request to time out.

- Mitigation: `crypto.randomUUID()` generates RFC 4122 v4 UUIDs with 122 bits of randomness.
  Collision probability is negligible. Even a monotonic counter (starting from 0 per session)
  would work since request IDs only need to be unique within a single WS connection's lifetime.

**R4: Message size limit mismatch between HTTP and WS.**

HTTP has a 64KB body limit (MaxBodySize middleware). The WS read limit should match.

- Mitigation: Set `conn.SetReadLimit(65536)` in readPump initialization. Action payloads
  are well under 1KB, so this is purely defensive.

**R5: HTTP fallback creates the same race condition the migration is designed to fix.**

When the WS is disconnected and the client falls back to HTTP POST, the old race condition
returns.

- Mitigation: This is a degraded-mode scenario (WS not connected). The user experiences it
  only during the 5-second reconnection window. The fallback is "as good as the old behavior"
  and provides a usable experience while the WS reconnects. Once reconnected, the client
  switches back to WS actions automatically.

### Open Questions

**Q1: Should the action_result message also trigger a tick reset?**

After processing an action, the server could reset the tick timer for that user so the next
tick fires 5 seconds from now (not 5 seconds from the last tick). This would prevent a tick
from firing immediately after an action (redundant state push).

- Recommendation: Yes. When `HandleWSAction` completes, it should reset the per-user tick
  timer. This avoids a redundant tick push that might arrive < 1 second after the action
  result. Implementation: the tick goroutine uses `time.NewTicker`, which does not support
  reset. Either switch to `time.NewTimer` with manual reset, or accept the occasional
  redundant tick push as harmless. Given the minor UX impact (an extra state push with
  identical-or-newer data), the simpler approach is to leave the ticker as-is and accept
  the redundant push. The client handles it correctly (state update is idempotent).

**Q2: Should pending requests be limited in count?**

If a buggy or malicious client sends thousands of actions without waiting for responses, the
server spawns thousands of goroutines (each blocking on the per-user mutex).

- Recommendation: Yes. Cap pending goroutines per user at a reasonable limit (e.g., 16,
  matching the send buffer size). If the limit is reached, respond with an error immediately
  without spawning a goroutine. Implementation: use a per-user semaphore or atomic counter
  checked in the `OnMessage` callback. This is a nice-to-have for Phase 1 hardening, not a
  blocker for initial launch.

**Q3: Should the stress tester be updated to use WS actions?**

The stress tester (`stress-tests/main.go`) currently uses HTTP POST for actions.

- Recommendation: Not immediately. The HTTP endpoint remains functional. Updating the stress
  tester to use WS actions would be useful for load testing the WS action path specifically,
  but this is independent work and not a prerequisite for the migration.

### Flagged Assumptions

**A1:** The single WS connection per user model is sufficient. If a user opens multiple tabs,
only the latest connection is active. Actions from other tabs fall back to HTTP. This is
existing behavior and is not changed by this design.

**A2:** 10 seconds is an appropriate timeout for action responses. The stress test P99 for
action processing is < 50ms at 1,000 users. A 10-second timeout provides enormous headroom.
If the server is so loaded that actions take > 10 seconds, the system has larger problems.
Revisit if the timeout fires in practice.

**A3:** The `fullStateResponse` payload in `action_result` is identical to the `state` payload
in tick pushes. The client reuses the same `GameState` TypeScript interface and `setStateFromPush`
logic for both. This simplifies the client but means action results carry the full state
(2-8KB) even when only a single field changed. Acceptable at current scale.

---

## 9. Testing Strategy

### Unit Tests (Backend)

- **WS message parsing:** Verify that `HandleWSAction` correctly parses valid action messages,
  rejects malformed messages (missing `type`, missing `id`, invalid JSON), and returns
  appropriate error responses over WS.
- **Rate limiting:** Verify that `CheckGameActionRate` correctly throttles after 7200
  requests/min and that WS action responses include the rate limit error.
- **processAction refactor:** Verify that the extracted `processAction` method produces
  identical results to the original `PerformAction` for all action types. Run with the same
  inputs and compare output state.
- **OnMessage routing:** Verify that messages with `type: "action"` are routed to
  `HandleWSAction`, and messages with unknown types are ignored (not crashed on).

### Integration Tests

- **End-to-end WS action:** Connect WS, send a `buy_hardware` action, verify the response
  contains `success: true` and the state includes the new hardware.
- **WS action error:** Send an action with insufficient resources. Verify the response contains
  `success: false` and an error message.
- **Request ID correlation:** Send two actions in rapid succession with different IDs. Verify
  each response has the correct ID.
- **Tick interaction:** Send an action, wait for the next tick push, verify the tick state
  reflects the action's changes.
- **HTTP fallback:** Disconnect WS, send an action via the client (should fall back to HTTP),
  verify the action succeeds.

### Frontend Tests

- **wsClient.sendAction:** Mock the WebSocket, call `sendAction`, simulate a response with
  the matching ID. Verify the Promise resolves with the state.
- **wsClient timeout:** Call `sendAction`, do not send a response, verify the Promise rejects
  after 10 seconds.
- **wsClient disconnect cleanup:** Call `sendAction`, disconnect the WS, verify the Promise
  rejects immediately with "connection lost."
- **Store action methods:** Verify that calling `buyHardware` etc. invokes `wsClient.sendAction`
  and updates the store on success.

### Performance Verification

- **Action latency:** Compare WS action P50/P99 latency against HTTP action P50/P99 latency
  using the stress tester (HTTP) and a manual WS client. WS should be equal or better (no
  HTTP overhead).
- **Tick push consistency:** After 100 WS actions, verify the next tick push reflects all
  changes. No state drift.
- **Goroutine count:** With 100 concurrent WS action senders per user, verify goroutine count
  stays bounded (per-user mutex serializes, no goroutine leak).

---

## 10. Observability & Operational Readiness

### Key Metrics (Log-Based)

- **WS action received:** Log `[ws-action] user=%s action=%s id=%s` on receipt.
- **WS action completed:** Log `[ws-action] user=%s action=%s id=%s duration=%dms success=%t`
  on completion.
- **WS action error:** Log `[ws-action] user=%s action=%s id=%s error=%s` on failure.
- **WS action rate limited:** Log `[ws-action] user=%s rate_limited=true` when rate limit
  is exceeded.
- **WS message parse error:** Log `[ws-action] user=%s parse_error=%s` when a client message
  cannot be parsed.

### 3am Diagnosability

**Symptom: Actions not responding (client shows timeout errors).**
1. Check server logs for `[ws-action]` entries. If no "received" logs, the message is not
   reaching the server -- check WS connection status.
2. If "received" but no "completed" logs, the action is blocked on the per-user mutex.
   Check for a stuck tick goroutine (DB connection pool exhaustion, deadlock).
3. If "completed" but client does not receive response, check for full send buffer
   ("WS push dropped" log lines).
4. Check client console for pending request map size. If growing, responses are not arriving.

**Symptom: Actions succeed but state is wrong.**
1. Check the `action_result` payload in client console (log the raw WS message).
2. Compare with direct DB state (`SELECT * FROM game_states WHERE user_id = ...`).
3. If they match, the issue is client-side state handling. If they differ, the issue is in
   `processAction` persistence.

### Production Readiness Criteria

- [ ] WS actions produce identical game state results as HTTP actions for all 19 action types
  (verified by running both paths with same inputs).
- [ ] Rate limiting works for WS actions (verified by sending > 120 actions/sec and observing
  rate limit responses).
- [ ] HTTP fallback works when WS is disconnected (verified by disabling WS and confirming
  actions still succeed via HTTP).
- [ ] Request-response correlation works under concurrent load (verified by sending 10 rapid
  actions and confirming all responses match their request IDs).
- [ ] No goroutine leaks from WS action processing (verified by 1-hour soak test with active
  play).
- [ ] Client-side timeout fires correctly when server does not respond (verified by killing
  the server while an action is in flight).

---

## 11. Implementation Phases

| Phase | Scope | Complexity | Client Impact | Rollback Risk |
|-------|-------|------------|---------------|---------------|
| 1 | Backend: WS message processing + action handler | M | None (client still uses HTTP) | Low (revert OnMessage callback to nil) |
| 2 | Frontend: wsClient module + store migration | M | Actions move from HTTP to WS | Low (revert store to use api.action) |
| 3 | Cleanup: Remove redundant HTTP WS push | S | None | Minimal |

**Total estimated effort: M (medium).** Phase 1 and Phase 2 can be developed in parallel
(backend and frontend). Phase 3 is a small cleanup after validation.

**Parallelization:** Phase 1 (backend) and Phase 2 (frontend, minus integration testing) can
be worked on simultaneously. The frontend can be developed against a mock WS server or against
the Phase 1 backend once deployed. Full integration testing requires both phases deployed.

---

## Summary

This TDD addresses a user-facing timing bug caused by the hybrid HTTP+WebSocket communication
model. By moving player actions to WebSocket with request ID correlation, we unify state
mutations and state delivery onto a single channel. The server processes an action and responds
with the updated state over the same WS connection, eliminating the race condition between HTTP
responses and WS tick pushes.

The design preserves backward compatibility (HTTP endpoint unchanged), reuses existing
infrastructure (per-user mutex, game engine, WS hub), and introduces minimal new complexity
(request ID correlation, pending-request map, exported rate limit function). Each implementation
phase is independently deployable and reversible.
