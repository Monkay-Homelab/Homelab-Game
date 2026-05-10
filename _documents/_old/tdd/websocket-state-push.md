---
project: "homelab-game"
maturity: "draft"
last_updated: "2026-03-21"
updated_by: "@staff-engineer"
scope: "Migrate from HTTP polling to WebSocket server-push for game state delivery, including server-side tick timer and concurrent write safety"
owner: "@staff-engineer"
dependencies:
  - ../spec/architecture.md
  - ../spec/performance.md
---

# WebSocket State Push

## 1. Problem Statement

### What

Replace the 5-second HTTP polling loop with server-initiated WebSocket pushes for game state
delivery. The server pushes the full game state to connected clients after every mutation
(action response, idle progress tick, random event). A new server-side tick timer computes idle
progress for connected users on a regular interval and pushes the result, eliminating the need
for clients to request state on a timer.

### Why Now

The current polling architecture is the dominant source of server load in normal operation. Every
5-second poll executes 8+ sequential database queries, runs the full game engine
(`ProcessIdleProgress`), persists updated state, and returns the entire game state -- even when
nothing has changed. The performance spec (stress test baseline, 2026-03-19) identifies this
per-request database fan-out as the primary throughput limiter, with the connection pool (20 max)
saturating at 2,500+ concurrent players.

WebSocket push eliminates wasted work:

- **No more redundant reads.** The server only computes and sends state when something actually
  changed (tick elapsed, action processed, event triggered).
- **Removes the stale-state race condition.** The `_lastActionAt` guard in `gameStore.ts`
  exists solely because poll responses can arrive after an action response, overwriting fresh
  state with stale data. With server-push, the server controls the ordering -- there is no
  race.
- **Foundation for future optimizations.** Server-push is a prerequisite for delta updates,
  adaptive tick rates, and visibility-aware resource management.

The WebSocket infrastructure already exists (gorilla/websocket, hub, auth, keepalive) and is used
for event notifications. This design extends it to carry game state.

### Constraints

- **Actions stay on HTTP.** `POST /api/game/action` continues to work as today. The HTTP
  response still returns the full game state (backward compatibility, and the client needs
  synchronous confirmation of success/failure). The server _also_ pushes state over WS after
  processing the action.
- **GET /api/game/state remains** but becomes a bootstrap endpoint used only on login/reconnect,
  not polled.
- **Auth, config, social, leaderboard endpoints stay on HTTP.** No changes to any endpoint
  other than game state delivery.
- **The only new WS message type is "state"** (alongside existing "event"). No new client-to-
  server WS messages.
- **Client-side interpolation continues.** `useIdleTick.ts` keeps running between server
  pushes for smooth counter animation.
- **Server-side tick only runs for connected users.** No resources are consumed for users
  without an active WebSocket connection.
- **Single-process assumption** remains (in-memory hub, per-user mutexes).

### Acceptance Criteria

1. When a user has an active WebSocket connection, the server pushes a `{"type": "state",
   "payload": <fullStateJSON>}` message at a regular tick interval (every 5 seconds by
   default).
2. When `PerformAction` completes successfully, the server pushes the updated state over
   WebSocket in addition to returning it in the HTTP response.
3. The client no longer polls `GET /api/game/state` on a 5-second interval.
4. The client uses `GET /api/game/state` exactly once on login/reconnect to bootstrap state.
5. When the WebSocket disconnects and reconnects, the server pushes a full state immediately
   upon reconnection.
6. The `_lastActionAt` stale-state guard in `gameStore.ts` is removed.
7. `useIdleTick.ts` continues to interpolate currencies smoothly between server pushes.
8. Concurrent WebSocket writes to the same connection never panic or corrupt frames.
9. Server-side tick goroutines are started on WebSocket connect and stopped on disconnect --
   no leaked goroutines.
10. Bitcoin price updates are included in each server-push state payload (same as current
    `GetState` response).

---

## 2. Context & Prior Art

### Current Architecture

**Polling model** (see `docs/spec/architecture.md` section 3.4 and 7.1):

- `App.tsx:47-51` runs `setInterval(fetchState, 5000)` when a token is present.
- `fetchState` calls `GET /api/game/state`, which loads 8+ tables from the DB, runs
  `ProcessIdleProgress`, persists updated state, and returns the full response.
- `useIdleTick.ts` interpolates between poll responses using `requestAnimationFrame` and rate
  calculations that mirror the server engine math.
- The `_lastActionAt` guard in `gameStore.ts:54` prevents a stale poll response from
  overwriting a fresher action response that arrived during the poll's flight time.

**Existing WebSocket** (`ws/hub.go`):

- Server-push only. Clients never send game messages over WS.
- Single connection per user; new connections close old ones.
- Used for event notifications only: `game.go:132-140` calls `h.hub.SendToUser` with
  `type: "event"` after `ProcessIdleProgress` triggers random events.
- Ping/pong keepalive (30s/45s).
- Client reconnects after 5s delay on close (`useWebSocket.ts:59-63`).
- **No per-connection write serialization.** `SendToUser` calls `conn.WriteMessage` directly.
  gorilla/websocket documents that "connections support one concurrent reader and one concurrent
  writer" -- concurrent writes from multiple goroutines (e.g., event push + ping) are unsafe.

**PerformAction handler** (`game.go:353-588`):

- Acquires a per-user mutex before processing.
- Loads all state, runs `ProcessIdleProgress` + `ProcessAction`, persists results.
- Returns full state in the HTTP response.
- Currently does NOT push state over WebSocket after an action.

### How Others Solve This

**Game industry standard** for idle/incremental games: the server maintains a tick loop per
active session and pushes state deltas or full snapshots. Full snapshots are simpler and correct
when state is small (< 10KB); deltas are worthwhile when state is large or bandwidth is
constrained. Our full state response is roughly 2-8KB depending on game progression, making full
snapshots the pragmatic choice.

**gorilla/websocket concurrent write safety**: The library's own documentation and FAQ recommend
either (a) a per-connection write goroutine with a buffered channel, or (b) a per-connection
mutex wrapping all write calls. Option (a) is the established pattern -- it also provides
natural backpressure via channel buffering.

---

## 3. Alternatives Considered

### Alternative A: Delta Updates Over WebSocket

Push only changed fields rather than the full state.

**Strengths:** Lower bandwidth per push. Scales better as state grows.

**Weaknesses:** Significant complexity. Requires server-side change tracking, client-side merge
logic, and careful handling of missed deltas (full resync fallback). The current full state
response is 2-8KB -- delta overhead is not justified at this scale. Introduces a new class of
bugs (client state drift) that the current full-replacement model avoids.

**Verdict:** Rejected for now. Can be layered on later if state size grows significantly
(e.g., hundreds of hardware items). The current full-push model is the right starting point.

### Alternative B: Server-Sent Events (SSE) Instead of WebSocket

Use SSE for server-push instead of extending the existing WebSocket.

**Strengths:** Simpler protocol. Automatic reconnection built into the browser API. No need for
ping/pong keepalive.

**Weaknesses:** Requires maintaining two persistent connection types (SSE for state + WS for
events), or migrating events to SSE too. The WebSocket infrastructure already exists, is tested,
and works. Adding SSE would be a lateral move with no clear benefit.

**Verdict:** Rejected. Extending the existing WebSocket is simpler and avoids dual-connection
complexity.

### Alternative C: Shared Server-Side Ticker (Single Goroutine for All Users)

Run one goroutine with a single `time.Ticker` that iterates over all connected users each tick.

**Strengths:** Fewer goroutines. Deterministic tick ordering across users.

**Weaknesses:** Tick processing time grows linearly with connected users. A single slow tick
(e.g., one user's DB write blocks) delays all other users' ticks. No natural parallelism.
Requires careful locking of the connection map during iteration.

**Verdict:** Rejected. Per-user goroutines provide isolation -- one user's slow tick does not
affect others. The goroutine cost (a few KB stack each) is negligible for the expected user
count (< 5,000 concurrent).

### Alternative D: Per-User Goroutine with Dedicated Tick Timer (Recommended)

Each WebSocket connection spawns a ticker goroutine that computes idle progress and pushes state
on a fixed interval. The goroutine exits when the connection closes.

**Strengths:** Isolation between users. Clean lifecycle tied to connection. Natural fit for
Go's goroutine model. Easy to reason about (each user has exactly one ticker). Can be started
and stopped without coordination.

**Weaknesses:** More goroutines than alternative C (one per user). Slightly higher memory
footprint. All goroutines independently hit the database, which could cause connection pool
contention if many users tick simultaneously.

**Mitigations:** Stagger tick start times naturally (users connect at different times). The
5-second tick interval means DB pressure is equivalent to the current 5-second poll interval --
no worse. Connection pool contention is an existing bottleneck documented in the performance
spec and is addressable independently (pool size increase, query batching, caching).

**Verdict:** Recommended. The operational simplicity, isolation guarantees, and natural Go
idiom outweigh the modest resource overhead.

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
     |                                     |                                |
     |                                     |-- [tick goroutine starts]      |
     |                                     |                                |
     |-- GET /api/game/state ------------->|-- 8 SELECTs ----------------->|
     |   (ONE TIME: bootstrap)             |<-- game data ------------------|
     |                                     |-- ProcessIdleProgress()        |
     |                                     |-- UPDATE game_states --------->|
     |<-- Full game state -----------------|                                |
     |                                     |                                |
     |   [Client interpolates via rAF]     |   [5s tick elapses]            |
     |                                     |                                |
     |                                     |-- 8 SELECTs ----------------->|
     |                                     |<-- game data ------------------|
     |                                     |-- ProcessIdleProgress()        |
     |                                     |-- UPDATE game_states --------->|
     |<== WS: {"type":"state",...} ========|                                |
     |                                     |                                |
     |-- POST /api/game/action ----------->|-- acquire user mutex           |
     |   { type, payload }                 |-- 8 SELECTs ----------------->|
     |                                     |<-- game data ------------------|
     |                                     |-- ProcessIdleProgress()        |
     |                                     |-- ProcessAction()              |
     |                                     |-- INSERT/UPDATE/DELETE -------->|
     |<-- Full game state (HTTP) ----------|-- release user mutex           |
     |<== WS: {"type":"state",...} ========|   (push AFTER HTTP response)   |
     |                                     |                                |
     |   [WS disconnects]                  |-- [tick goroutine stops]       |
     |                                     |                                |
     |-- WS /ws?token=xxx --------------->|                                |
     |<== WebSocket re-established =======>|                                |
     |                                     |-- [tick goroutine starts]      |
     |<== WS: {"type":"state",...} ========|   (immediate state push)       |
```

### 4.2 Component Changes

#### Backend: `ws/hub.go` -- Connection Manager Refactor

The hub evolves from a simple connection map to a per-connection client abstraction that owns
a write channel and lifecycle:

**New `Client` struct:**
```
Client {
    UserID     string
    conn       *websocket.Conn
    send       chan []byte        // buffered outbound message channel
    hub        *Hub
    done       chan struct{}       // signals shutdown to ticker goroutine
}
```

**Write safety model:** The `Client` owns a single `writePump` goroutine that is the only
goroutine that calls `conn.WriteMessage` (for both data and ping frames). All other code
sends messages by writing to the `send` channel. This is the gorilla/websocket recommended
pattern and eliminates all concurrent write hazards.

**Channel buffer size:** 16 messages. If the buffer fills (slow client), the message is
dropped and a warning is logged. Dropping is preferable to blocking -- a slow client should
not back-pressure the server tick. The client will recover on the next tick push.

**Lifecycle:**

1. `HandleConnect`: Upgrade, create `Client`, register in hub, start `writePump`,
   `readPump`, and signal to start the tick goroutine.
2. `readPump`: Same as today -- reads and discards client messages, detects disconnect.
   On exit, triggers cleanup.
3. `writePump`: Reads from `send` channel, writes to conn. Also handles ping timer
   (moved from the current separate goroutine). On `done` signal or send channel close,
   writes a close frame and exits.
4. Cleanup: Close `done` channel (stops ticker), close `send` channel (stops writePump),
   remove from hub map, close conn.

**Hub map change:** `clients map[string]*Client` (was `map[string]*websocket.Conn`).

**`SendToUser` change:** Looks up `*Client`, writes serialized `[]byte` to `client.send`
channel via non-blocking send (drop if full).

#### Backend: `handlers/game.go` -- Tick Goroutine and State Push

**New method: `GameHandler.runUserTick(ctx, userID)`**

This method contains the same logic currently in `GetState`:

1. Load game state + all related tables from DB.
2. Run `ProcessIdleProgress`.
3. Calculate group bonus, colo rack income.
4. Process customer growth.
5. Persist updated state.
6. Push events over WS (if any triggered).
7. Fetch bitcoin price data.
8. Build the `fullStateResponse`.
9. Serialize to JSON.
10. Push via `hub.SendToUser(userID, stateMessage)`.

This is extracted into a shared method so both the tick goroutine and `PerformAction` can
call it without duplicating code.

**Tick goroutine lifecycle:**

The hub signals the `GameHandler` when a user connects or disconnects. On connect, a goroutine
is spawned:

```go
go func() {
    ticker := time.NewTicker(tickInterval)  // 5s default
    defer ticker.Stop()

    // Immediate state push on connect (handles reconnection)
    h.runUserTick(ctx, userID)

    for {
        select {
        case <-ticker.C:
            h.runUserTick(ctx, userID)
        case <-done:
            return
        }
    }
}()
```

The `done` channel is closed when the WebSocket disconnects, causing the goroutine to exit
cleanly.

**Per-user mutex interaction:** `runUserTick` MUST acquire the same per-user mutex
(`userLocks.Lock(userID)`) that `PerformAction` uses. This prevents a tick from reading
half-written state while an action is being processed, and prevents a tick from writing state
that overwrites an action's result.

Sequence when an action arrives during a tick:

1. Tick goroutine holds user lock, computing idle progress.
2. `PerformAction` HTTP handler blocks on `userLocks.Lock(userID)`.
3. Tick goroutine finishes, pushes state, releases lock.
4. `PerformAction` acquires lock, loads fresh state from DB, processes action.
5. `PerformAction` persists result, returns HTTP response.
6. `PerformAction` pushes updated state over WS (after releasing the lock, so the push uses
   the just-persisted data).

The lock ensures serial execution of state mutations for the same user. The tick goroutine
and `PerformAction` never run concurrently for the same user.

**PerformAction state push:** After `PerformAction` finishes processing and persists state,
it pushes the full state over WebSocket. This ensures the client's WS-driven state is
immediately updated, even before the next tick fires. The push happens after the HTTP response
is sent (or can be done concurrently -- the key invariant is that the persisted state is
consistent before the push).

**GetState handler:** No changes to the handler itself. It continues to work as before. The
only change is that the client stops calling it on a 5-second interval.

#### Backend: Bitcoin Price in Tick Pushes

The current `GetState` handler fetches bitcoin price and history on every call
(`fetchBitcoinData`). The tick goroutine does the same, so bitcoin prices are included in
every state push. The bitcoin price service is lazy-evaluated and mutex-protected internally,
so calling it from tick goroutines across multiple users is safe.

**Consideration:** If many users tick simultaneously, they all call `bitcoinSvc.GetCurrentPrice`,
which acquires the bitcoin service's internal mutex and potentially advances the price model.
This is the existing behavior (every poll did this too) and is not worse. The bitcoin price
step interval (30s) means most calls are no-ops (price already advanced). No change needed.

#### Frontend: `App.tsx` -- Remove Polling Loop

Remove the `useEffect` at lines 47-51 that runs `setInterval(fetchState, 5000)`.

Keep the `useEffect` at lines 41-45 that calls `fetchState` once when `token` is set and
`state` is null (bootstrap on login).

#### Frontend: `useWebSocket.ts` -- Handle "state" Messages

Extend the `onmessage` handler to process `type: "state"` messages:

```
if (msg.type === 'state') {
    useGameStore.getState().setStateFromPush(msg.payload);
}
```

Remove the `fetchState()` call inside the `type: "event"` handler (line 52). Events are now
included in the state push, so a separate fetch is unnecessary.

#### Frontend: `gameStore.ts` -- New `setStateFromPush` Action, Remove `_lastActionAt`

Add a new store action `setStateFromPush(state: GameState)` that directly sets the `state`
field from a WebSocket push. This is simpler than `fetchState` because there is no HTTP request,
no error handling, and no stale-state guard needed.

Remove the `_lastActionAt` variable and all references to it. The stale-state race condition
does not exist with server-push because:

- The server controls push ordering.
- After an action, the server pushes the post-action state. There is no concurrent poll
  that could return older state.
- The client's `fetchState` (used only for bootstrap) runs once before any WS pushes, so
  there is no interleaving to guard against.

#### Frontend: `useIdleTick.ts` -- No Changes Required

The idle tick hook reads from the `state` ref and recalculates rates whenever `state` changes.
Whether `state` is updated from a poll response or a WS push, the hook works identically. The
rAF loop continues to interpolate between state updates.

The push interval (5 seconds) matches the current poll interval, so the interpolation behavior
is unchanged.

### 4.3 WebSocket Message Format

**State push message:**
```json
{
  "type": "state",
  "payload": {
    "id": "...",
    "user_id": "...",
    "tier": "rack_12u",
    "compute_units": 42000,
    ...
    "hardware": [...],
    "services": [...],
    "available_hardware": [...],
    "bitcoin_price": 9850,
    "bitcoin_price_history": [...],
    "group_bonus": 1.15,
    "group_members": 4,
    "global_donated_cu": 500000
  }
}
```

The payload is identical to the current `GET /api/game/state` response body. Same JSON
structure, same fields. This means the client can reuse the existing `GameState` TypeScript
interface without changes.

**Event message (unchanged):**
```json
{
  "type": "event",
  "payload": {
    "type": "power_surge",
    "name": "Power Surge!",
    "description": "...",
    "severity": "moderate",
    "effect": {...}
  }
}
```

Events continue to be pushed separately from state. The state push includes the `events` array
in the response (same as today), and individual events are pushed as `type: "event"` for the
`EventLog` component's toast-style display.

---

## 5. Data Models & Storage

No database schema changes. The `fullStateResponse` struct and all underlying models remain
exactly as they are.

The only data model change is the in-memory `Client` struct in the hub, which is not persisted.

---

## 6. API Contracts

### Modified Endpoints

**GET /api/game/state** -- No change to request or response format. Usage changes from
"polled every 5s" to "called once on login/reconnect."

**POST /api/game/action** -- No change to request or response format. New behavior: after
returning the HTTP response, the server also pushes the state over WebSocket.

### New WebSocket Message

**Server-to-client: `type: "state"`**

Direction: server to client only.
Trigger: idle progress tick, successful action processing, reconnection.
Payload: `fullStateResponse` JSON (same schema as `GET /api/game/state` response).

No new client-to-server messages. The WebSocket remains server-push only.

---

## 7. Migration & Rollout

### Phased Approach

The migration can be done incrementally. At no point is there a "big bang" cutover.

**Phase 0 (Prerequisite):** Fix concurrent write safety. This is a pure backend change with
no client impact. Ship independently and verify stability before proceeding.

**Phase 1:** Add server-side tick and state push. Keep the client polling loop running. Both
the poll and the push update the client state -- the push will generally arrive first, and the
poll becomes a no-op (the state is already current). This is a safe overlap period for
verifying that pushed state matches polled state.

**Phase 2:** Remove the client polling loop. Add `setStateFromPush` to the store. The
`fetchState` call moves to bootstrap-only. Remove `_lastActionAt`. The state push in
`PerformAction` ensures the client sees action results immediately.

**Phase 3:** Add `PerformAction` WS push. After the action HTTP response is sent, also push
state over WS. This is additive -- the client already has the state from the HTTP response,
and the WS push confirms it (or a subsequent tick will).

### Rollback Plan

Each phase is independently reversible:

- Phase 0 rollback: Revert hub.go changes. The old code worked (just had a latent
  concurrency bug that was unlikely under low load).
- Phase 1 rollback: Stop tick goroutines. Clients are still polling, so they continue to work.
- Phase 2 rollback: Re-add the polling `setInterval` in `App.tsx`. Restore `_lastActionAt`.
- Phase 3 rollback: Remove the WS push from `PerformAction`. Clients rely on tick pushes
  (5s delay for action confirmation, degraded UX but functional).

### Breaking Changes

None. The WS message format is additive (new `type: "state"` alongside existing `type:
"event"`). All HTTP endpoints are unchanged. Older clients that do not handle `type: "state"`
messages will simply ignore them (the `onmessage` handler falls through).

---

## 8. Risks & Open Questions

### Known Risks

**R1: Tick goroutine leaks.** If the done channel is not properly closed on disconnect, tick
goroutines accumulate and consume resources indefinitely.

- Mitigation: The cleanup path in `readPump`'s deferred function closes the done channel.
  Add a log line on ticker exit for observability. Add a metric (or log) for active tick
  goroutine count.
- Mitigation: Add a context with timeout to the tick goroutine, so it self-terminates even if
  the done channel is never closed (defense in depth).

**R2: Database connection pool contention from many simultaneous ticks.** If N users all tick
within a narrow window, N*8 queries compete for 20 pool connections.

- Mitigation: Tick start times are naturally staggered (users connect at different times).
  The 5-second interval is the same as the current poll interval, so this is no worse than
  today.
- Mitigation: This is documented as the primary bottleneck in the performance spec and is
  addressable independently (pool size increase, query batching, caching).

**R3: Action latency increase due to mutex contention with tick.** If a tick is in progress
when an action arrives, the action blocks until the tick completes.

- Mitigation: Tick processing time is fast (~4ms at P50 per the stress test, which includes
  the same code path). The worst case is a 4ms delay before the action starts processing.
  This is imperceptible.

**R4: Slow WebSocket client causes dropped state pushes.** If the `send` channel buffer fills
because the client cannot read fast enough, pushes are dropped.

- Mitigation: Buffer size of 16 messages at 5-second intervals means the client would need
  to be unresponsive for 80+ seconds to overflow. At that point, the connection's pong timeout
  (45s) will have already closed it.
- Mitigation: On reconnect, the client receives a full state push immediately, so any dropped
  messages are recovered.

**R5: Double state update on action.** The client receives the state from both the HTTP
response and the WS push. If they arrive in close succession, the store updates twice.

- Mitigation: This is harmless. Both payloads contain the same state. The second `set` is
  a no-op from the user's perspective (Zustand does not re-render if the state object is
  structurally identical, though it will if it is a new object reference). The simplicity
  of not deduplicating outweighs the cost of one extra render.

### Open Questions

**Q1: Should the tick interval be configurable?** Currently hardcoded at 5 seconds. A
configuration option (env var `TICK_INTERVAL_SECONDS`) would allow tuning without recompiling.

- Recommendation: Yes. Default to 5 seconds. Allow override via environment variable. Document
  that shorter intervals increase DB load proportionally.

**Q2: Should GetState also push events over WS when called for bootstrap?** Currently,
`GetState` pushes events via `pushEvents` (line 334-336). On bootstrap, the client does not
yet have a WS connection (it connects in parallel). Events triggered during bootstrap could
be lost.

- Recommendation: No change needed. The bootstrap `GetState` returns events in the HTTP
  response body (the `events` field). The client receives them. The WS event push is
  belt-and-suspenders. If the WS is not yet connected, the events are still delivered via
  HTTP. On the next tick push, any ongoing event effects (throttle) are reflected in the
  state.

**Q3: Should the tick goroutine use a context derived from the HTTP request?**

- Recommendation: No. The tick goroutine outlives any single HTTP request. Use a background
  context (`context.Background()`) with a cancel function tied to the done channel.

### Flagged Assumptions

**A1:** The 5-second tick interval is appropriate for this game's pace. If it proves too
frequent (DB load) or too infrequent (stale state between pushes), it should be tuned.
Revisit after Phase 1 is deployed and DB load is measured.

**A2:** Full state pushes are efficient enough at current state sizes (2-8KB). If average
state size grows past 20KB, delta updates should be reconsidered. Revisit when catalog or
inventory sizes increase significantly.

**A3:** The per-user mutex used by both `PerformAction` and the tick goroutine is sufficient
for concurrency control. This assumes single-process deployment continues. If multi-process
deployment becomes necessary, this design needs revisiting.

---

## 9. Testing Strategy

### Unit Tests (Backend)

- **Hub Client lifecycle:** Verify that `Client.writePump` exits cleanly when `send` channel
  is closed. Verify that closing `done` stops the ticker and exits the goroutine.
- **Concurrent write safety:** Spawn multiple goroutines calling `SendToUser` for the same
  user simultaneously. Verify no panics and all messages are delivered (or gracefully dropped
  if buffer is full).
- **Tick goroutine:** Verify that `runUserTick` produces a valid `fullStateResponse`. Mock
  the database queries and verify the output matches expected structure.

### Integration Tests

- **End-to-end state push:** Connect a WebSocket client, wait for a state push, verify the
  message type is "state" and the payload deserializes to a valid game state.
- **Action triggers WS push:** Send an HTTP action, verify that the WS receives a state push
  within a short window.
- **Reconnection:** Connect, disconnect, reconnect. Verify that a state push arrives
  immediately after reconnection.
- **Mutex contention:** Send an action while a tick is in progress. Verify both complete
  without deadlock and the final state is consistent.

### Frontend Tests

- **Store update from WS:** Call `setStateFromPush` with a mock payload, verify the store
  updates.
- **No polling:** Verify that no `setInterval` calls `fetchState` after login.
- **useIdleTick integration:** Verify that currencies interpolate correctly between WS-pushed
  state updates (same as current behavior, just verifying no regression).

### Performance Verification

- **Stress test with WS pushes:** Run the existing stress tester with the `-ws` flag. Measure
  push latency (time from tick trigger to client receipt). Compare DB load (queries/sec) before
  and after migration.
- **Goroutine leak check:** Connect and disconnect 1,000 users in sequence. Verify goroutine
  count returns to baseline.

---

## 10. Observability & Operational Readiness

### Key Metrics (Log-Based)

Since the project has no metrics infrastructure, these are implemented as structured log lines
that can be grepped/parsed:

- **Active tick goroutines:** Log on tick goroutine start/stop. `grep` for count at any time.
- **Tick duration:** Log the time taken for each `runUserTick` call (already roughly measurable
  from the existing stress test P50/P90 numbers).
- **WS push drops:** Log when a message is dropped due to full send channel buffer. This
  indicates a slow client.
- **Connected users:** The existing `Hub.ConnectedUsers()` method should be logged periodically
  or exposed via the health endpoint.

### 3am Diagnosability

**Symptom: Clients not receiving state updates.**
1. Check WS connection status in client console (`[WS] Message received:` logs).
2. Check server logs for tick goroutine start/stop messages.
3. Check for "WS push dropped" log lines (slow client / buffer full).
4. Check for DB connection pool exhaustion (pgx pool wait timeout errors).

**Symptom: High CPU/memory on server.**
1. Check goroutine count (`/debug/pprof/goroutine` if pprof is enabled, otherwise log-based).
2. Look for tick goroutine leaks (goroutine count growing without corresponding connected
   user count growth).
3. Check tick duration logs -- if ticks take longer than the interval, goroutines pile up.

**Symptom: Stale state on client after action.**
1. Verify the action HTTP response contains updated state.
2. Check if the WS push after `PerformAction` is being sent (log line).
3. Check if the client's `onmessage` handler is processing `type: "state"` messages.

### Production Readiness Criteria

- [ ] Concurrent write safety verified by stress test (no panics under load).
- [ ] Tick goroutine count matches connected user count (no leaks after 1h soak test).
- [ ] Client state matches server state within one tick interval (5s) under normal operation.
- [ ] Reconnection results in full state push within 1 second of WS handshake.
- [ ] No regression in action latency (P50, P99) compared to pre-migration baseline.

---

## 11. Implementation Phases

### Phase 0: Concurrent Write Safety Fix (Prerequisite)

**Complexity: S**
**Dependencies: None**
**Files changed:** `ws/hub.go`

Refactor the hub to use a per-connection `Client` struct with:

1. A `send chan []byte` (buffered, capacity 16) for outbound messages.
2. A `writePump` goroutine that is the sole writer to `conn.WriteMessage`. It reads from the
   `send` channel and handles ping frames (replacing the current separate ping goroutine).
3. A `readPump` goroutine (same as current read loop, but with cleanup responsibility).
4. Update `SendToUser` to write to the `send` channel (non-blocking) instead of calling
   `conn.WriteMessage` directly.
5. Move the ping ticker into `writePump` so all writes (data + control frames) are serialized
   through a single goroutine.
6. Update `HandleConnect` to create a `Client`, register it, and start both pumps.
7. Update the hub map type from `map[string]*websocket.Conn` to `map[string]*Client`.

The `done` channel is added to `Client` in this phase (needed for Phase 1) but nothing
writes to it yet beyond the cleanup path.

This phase is a pure refactor -- external behavior is identical, but write safety is
guaranteed. Ship and soak before proceeding.

### Phase 1: Server-Side Tick Goroutine and State Push

**Complexity: M**
**Dependencies: Phase 0**
**Files changed:** `ws/hub.go`, `handlers/game.go`, `cmd/server/main.go`

1. Add a connect/disconnect callback mechanism so the hub can notify the `GameHandler` of
   connection lifecycle events. Options:
   - Pass a callback `func(userID string, connected bool)` to the hub at construction.
   - Have the hub accept an interface with `OnConnect(userID)` / `OnDisconnect(userID)`.
   - Recommended: Use a callback pair passed to `NewHub`, keeping the hub decoupled from
     game logic.

2. Extract the idle-progress-and-response logic from `GetState` into a shared method
   `runUserTick(ctx context.Context, userID string)` on `GameHandler`. This method:
   - Acquires the per-user mutex.
   - Loads state, runs ProcessIdleProgress, calculates bonuses, processes customer growth.
   - Persists state.
   - Pushes events over WS.
   - Fetches bitcoin data.
   - Builds fullStateResponse.
   - Pushes `{"type":"state", "payload": <response>}` over WS.
   - Releases the per-user mutex.

3. Implement `OnConnect(userID)`: Spawns a goroutine with a 5-second ticker that calls
   `runUserTick`. Immediately calls `runUserTick` once before entering the tick loop
   (handles reconnection bootstrap). Goroutine exits when the `done` channel is closed.

4. Implement `OnDisconnect(userID)`: Closes the `done` channel (handled by hub cleanup).

5. Refactor `GetState` handler to call `runUserTick` internally (or keep it independent --
   it is only called once on bootstrap, so code duplication is acceptable if it avoids
   coupling).

6. Wire up callbacks in `main.go` when constructing the hub and game handler.

7. Make tick interval configurable via env var `TICK_INTERVAL_SECONDS` (default: 5).

At this point, connected clients receive state pushes every 5 seconds AND still poll via
HTTP. Both work simultaneously. This is the overlap period for validation.

### Phase 2: Client Migration -- Remove Polling, Add WS State Handler

**Complexity: S**
**Dependencies: Phase 1**
**Files changed:** `App.tsx`, `useWebSocket.ts`, `gameStore.ts`

1. **`gameStore.ts`:**
   - Add `setStateFromPush: (state: GameState) => void` action that sets `state` directly.
   - Remove the `_lastActionAt` variable and all references to it in every action handler.
   - Simplify `fetchState` to remove the stale-state guard (it is now bootstrap-only).

2. **`useWebSocket.ts`:**
   - In `onmessage`, add handling for `msg.type === 'state'`:
     Parse `msg.payload` as `GameState`, call `useGameStore.getState().setStateFromPush(parsed)`.
   - Remove the `fetchState()` call in the `type: 'event'` handler (line 52).

3. **`App.tsx`:**
   - Remove the `useEffect` at lines 47-51 (`setInterval(fetchState, 5000)`).
   - Keep the `useEffect` at lines 41-45 (bootstrap `fetchState` on login).

4. No changes to `useIdleTick.ts`, `api.ts`, or any component.

### Phase 3: PerformAction WebSocket Push

**Complexity: S**
**Dependencies: Phase 1 (backend), Phase 2 (client ready to receive)**
**Files changed:** `handlers/game.go`

1. At the end of `PerformAction`, after the HTTP response has been written, push the full
   state over WebSocket:

   ```go
   // After json.NewEncoder(w).Encode(resp):
   stateData, _ := json.Marshal(ws.Message{
       Type:    "state",
       Payload: respJSON,  // the same fullStateResponse
   })
   h.hub.SendToUser(userID, stateData)
   ```

   The state pushed here is the post-action state, which the client already has from the HTTP
   response. The WS push serves as a fast update for the useIdleTick's server timestamp
   recalibration and ensures any other hypothetical WS-only consumers stay in sync.

2. Alternatively, the push can be integrated into `runUserTick` by having `PerformAction`
   simply trigger an out-of-band tick after processing. This avoids serializing the response
   twice but adds a small delay (one runUserTick cycle). The direct push is simpler.

**Note:** The push timing relative to the HTTP response is important. The push should happen
AFTER the HTTP response is sent (or at least after the state is persisted to DB), so that if
the push triggers a runUserTick on a different goroutine, it sees consistent state. Since the
per-user mutex is held during PerformAction and the push just writes to a channel, this
ordering is naturally correct.

---

## Summary

| Phase | Scope | Complexity | Client Impact | Rollback Risk |
|-------|-------|------------|---------------|---------------|
| 0     | WS write safety fix | S | None | Minimal |
| 1     | Server-side tick + state push | M | None (polling still active) | Low |
| 2     | Remove client polling, handle WS state | S | Polling removed | Low (re-add interval) |
| 3     | Action triggers WS push | S | Faster post-action state | Low (remove push) |

Total estimated effort: M (medium). Each phase can be shipped, soaked, and verified
independently. The overlap in Phase 1 (polling + push coexisting) provides a safe validation
period before the client migration in Phase 2.
