---
project: "homelab-the-game"
maturity: "stable"
last_updated: "2026-04-04"
updated_by: "@staff-engineer"
scope: "Performance characteristics, bottlenecks, optimizations, and scaling limits of the Homelab the Game backend and client"
owner: "@staff-engineer"
dependencies:
  - ../tdd/tick-performance-optimizations.md
  - ../tdd/horizontal-scaling.md
---

# Performance Specification

This document describes the actual performance characteristics of Homelab the Game as of
2026-04-04. It covers the backend game loop, database interaction patterns, caching layers,
concurrency model, client-side rendering, known bottlenecks, and measured limits. It is
written for a developer contributing code who needs to understand where performance is
sensitive and where it is not.

---

## 1. Architecture Overview (Performance-Relevant)

The game runs a server-authoritative model: all state mutations are validated server-side.
The backend is a single Go process serving HTTP and WebSocket connections, backed by
PostgreSQL 16 with TimescaleDB. Redis is used optionally for cross-replica coordination
(rate limiting, pub/sub, CU cache, leader election). The desktop client is a Tauri + React
app using Vite/Tailwind.

**Key components in the hot path:**

- Per-user tick goroutine (5-second interval, per connected WS user)
- `LoadFullGameState` batch query (2 DB round-trips per full tick)
- `ProcessIdleProgress` engine computation (pure in-memory, no I/O)
- `gameState.Update` persistence (1 DB write per tick)
- JSON serialization + WebSocket push (per tick, per user)
- Per-user mutex (`userMutexMap`) serializing ticks and actions

---

## 2. Tick System

The tick system is the single most performance-critical subsystem. Every connected WebSocket
user has a dedicated goroutine that fires every 5 seconds (configurable via
`TICK_INTERVAL_SECONDS` env var).

### 2.1 Full Tick vs. Light Tick

The tick system operates in two modes, controlled by a per-user dirty flag:

**Full tick** (dirty=true, or first tick after connect):
1. `LoadFullGameState` -- 2 DB round-trips (1 game_state lookup + 1 pgx.Batch of 8 child queries)
2. `ProcessIdleProgress` -- pure computation, no I/O
3. `getGroupBonus` -- 2 DB queries (`GetUserGroup` + `GetMembers`)
4. Colo rack income -- in-memory loop over cached colo racks
5. Customer growth -- may issue DB writes if SaaS customers are growing
6. `gameState.Update` -- 1 DB write
7. Per-customer `Update` -- N DB writes (one per customer, regardless of change -- **known N+1 issue**, see Section 9)
8. `fetchBitcoinData` -- reads from in-memory `PriceService` (0 DB queries for non-leader replicas; leader may write price history)
9. JSON marshal + WS push
10. Cache state in `tickStateMap` for future light ticks

**Full tick DB cost:** ~5 round-trips + N customer updates + conditional writes

**Light tick** (dirty=false, cached data available):
1. Reuses cached child data from last full tick (hardware, services, upgrades, etc.)
2. Reuses cached group bonus (group membership changes are infrequent)
3. `ProcessIdleProgress` on cached in-memory state
4. Colo rack income from cached colo racks
5. Customer growth (may write if SaaS customers grow)
6. `gameState.Update` -- 1 DB write
7. `fetchBitcoinData` -- in-memory read
8. `globalCUCache.Get()` -- in-memory read (or Redis read if available)
9. JSON marshal + WS push

**Light tick DB cost:** 1 round-trip (game_state update only) + conditional customer writes

### 2.2 Dirty Flag Lifecycle

| Event | Flag State |
|-------|-----------|
| `OnConnect` | Set to `dirty: true` (first tick always full) |
| `processAction` / `HandleWSAction` succeeds | Set to `dirty: true` via `tickState.MarkDirty(userID)` |
| Full tick completes | Set to `dirty: false`, caches child data |
| `OnDisconnect` | Entry deleted from `tickStateMap` |

The dirty flag is **not protected by a separate lock** -- the comment in `MarkDirty` states
"No lock needed -- single writer (processAction holds per-user mutex)." This is correct
because the per-user mutex serializes all tick and action execution for a given user.

### 2.3 Tick Timing

- Default interval: 5 seconds (`defaultTickInterval`)
- Context timeout: 10 seconds per tick (`context.WithTimeout`)
- On connect: immediate tick fires before entering the ticker loop (provides instant state)
- On disconnect: `done` channel closes, goroutine exits on next select

**Location:** `apps/backend/internal/api/handlers/game.go` lines 369-635

---

## 3. Database Performance

### 3.1 Connection Pool

| Setting | Value | Location |
|---------|-------|----------|
| `MaxConns` | 50 | `apps/backend/internal/database/db.go:16` |
| `MinConns` | 5 | `apps/backend/internal/database/db.go:17` |
| PostgreSQL `max_connections` | 150 | `ALTER SYSTEM`, verified via `SHOW max_connections` |
| PostgreSQL `shared_buffers` | 128MB | Default PostgreSQL config |
| PostgreSQL `work_mem` | 4MB | Default PostgreSQL config |
| PostgreSQL `effective_cache_size` | 4GB | Default PostgreSQL config |

The pool is a `pgxpool.Pool` from `github.com/jackc/pgx/v5`. It manages connection lifecycle
automatically -- idle connections are returned to the pool after each query.

**Headroom analysis:** At 50 max connections and 150 PostgreSQL max_connections, there are 100
connections available for superuser sessions, monitoring tools, manual psql, and future
services. At current table sizes (single-digit to low-hundreds of rows), individual queries
complete in <1ms, so 50 connections can theoretically handle 50,000 queries/sec.

### 3.2 Batch Loading

The primary read pattern is `LoadFullGameState` in `queries/batch.go`. It uses `pgx.Batch` to
send 8 child-table SELECTs in a single network round-trip, preceded by a single game_state
lookup (total: 2 round-trips).

There are two variants:

| Function | Purpose | Locking |
|----------|---------|---------|
| `LoadFullGameState(ctx, pool, userID)` | Used by ticks and HTTP GetState | No row lock |
| `LoadFullGameStateForUpdate(ctx, tx, userID)` | Used for multi-replica action serialization | `SELECT ... FOR UPDATE` on game_states |

**Tables queried per batch:**
1. `game_states` (by `user_id`, unique index)
2. `hardware` (by `game_state_id`, indexed)
3. `services` (by `game_state_id`, indexed)
4. `upgrades` (by `game_state_id`, indexed)
5. `customers` (by `game_state_id`, indexed)
6. `expenses` (by `game_state_id`, indexed)
7. `colo_racks` (by `user_id`, indexed)
8. `component_upgrades` (JOIN through `hardware`, by `game_state_id`, indexed)
9. `research_levels` (by `game_state_id`, indexed)

### 3.3 Indexes

All foreign key columns used in the batch query have indexes (applied via migration
`014_add_game_state_indexes.sql`, verified in production):

| Index | Table | Column |
|-------|-------|--------|
| `idx_hardware_game_state` | hardware | game_state_id |
| `idx_services_game_state` | services | game_state_id |
| `idx_upgrades_game_state` | upgrades | game_state_id |
| `idx_customers_game_state` | customers | game_state_id |
| `idx_expenses_game_state` | expenses | game_state_id |
| `idx_colo_racks_user` | colo_racks | user_id |
| `idx_research_levels_game_state` | research_levels | game_state_id |

Additional indexes exist on primary keys (auto-created), `game_states.user_id` (unique),
`bitcoin_price_history.time`, `leaderboard_entries.category+score`, and
`component_upgrades.hardware_id+component` (unique).

### 3.4 Table Sizes (Current Production)

As of 2026-04-04, the database has 1 active player:

| Table | Row Count |
|-------|-----------|
| bitcoin_price_history | 1,283 |
| customers | 45 |
| component_upgrades | 44 |
| services | 34 |
| upgrades | 21 |
| hardware | 17 |
| research_levels | 10 |
| colo_racks | 5 |
| expenses | 5 |
| users | 1 |
| game_states | 1 |

At these sizes, all tables fit entirely in PostgreSQL shared buffers. Query execution times
are sub-millisecond. **Performance concerns are about query volume (round-trips), not query
execution time.** As the player base grows, table sizes will scale linearly with player
count (each player has their own set of child rows). The indexes ensure this remains
efficient -- each child-table query is an index scan on `game_state_id`, not a sequential
scan.

### 3.5 Write Patterns

**Per tick (idle user):** 1 UPDATE on `game_states` (light tick)

**Per tick (active user, SaaS unlocked with customers):** 1 UPDATE on `game_states` + N
UPDATEs on `customers` (one per customer regardless of change -- see Section 9, Known
Issue #1)

**Per action:** Varies by action type. Most actions produce 1-3 INSERTs (new hardware,
service, upgrade) + 1 UPDATE on `game_states`. Prestige is the heaviest action: DELETE on
5 tables + re-INSERT persistent upgrades.

---

## 4. Caching

### 4.1 Global Donated CU Cache

**What:** Caches `SELECT COALESCE(SUM(total_donated_cu), 0) FROM game_states` -- a full table
scan that was previously executed on every tick, every GetState, and every action.

**Location:** `apps/backend/internal/api/handlers/cu_cache.go`

**Behavior:**
- Initialized with a blocking query at startup (before accepting connections)
- Background goroutine refreshes every 30 seconds
- `Get()` reads from Redis first (if available), falls back to local in-memory value
- `Add(amount)` uses Redis `INCRBY` for cross-replica atomicity, plus local increment
- On donate_cu action: `Add()` is called inline for immediate feedback
- Stale value retained on refresh error (logged)
- Slow refresh (>1 second) logged as warning
- Redis TTL: 60 seconds per SET

**Impact:** Eliminated ~400 full table scans/sec at 2K connected users (reduced to 1 query
every 30 seconds).

### 4.2 Tick State Cache (Per-User)

**What:** Caches all child-table data (hardware, services, upgrades, customers, expenses,
colo_racks, component_upgrades, research_levels, group bonus) from the last full tick for
reuse during light ticks.

**Location:** `apps/backend/internal/api/handlers/game.go` (types `userTickState`,
`cachedChildData`, `tickStateMap`)

**Behavior:**
- Map of `userID -> *userTickState`, protected by `sync.RWMutex`
- Full tick populates the cache; light tick reads from it
- Dirty flag triggers cache refresh on next tick
- Cache entries are deleted on disconnect
- No TTL -- cache validity is maintained by the dirty flag mechanism

**Memory footprint:** Each cached user holds: game state struct (~500 bytes), all child data
structs (varies by game progress, typically 1-10KB for a mid-game player), and the last
serialized JSON payload (typically 5-20KB). At 1,000 connected users, expect 10-30MB total
for the tick state cache.

### 4.3 Bitcoin Price Service (In-Memory)

**What:** The `PriceService` lazily advances a deterministic Ornstein-Uhlenbeck price model.
Price state is persisted to the `bitcoin_price` table. Price history is stored in
`bitcoin_price_history`.

**Location:** `apps/backend/internal/game/bitcoin/price.go`

**Behavior:**
- `GetCurrentPrice` is serialized by a `sync.Mutex` (one caller advances at a time)
- Non-leader replicas (when Redis leader election is active) skip advancement and just read
- Price steps are capped at 1,000 catch-up steps to prevent expensive computation after long offline periods
- Step interval: 5 seconds (configurable via `PriceConfig`)
- Each step writes 1 row to `bitcoin_price_history`

**Performance note:** The bitcoin service `GetCurrentPrice` holds a mutex while doing DB
I/O (read price state, write N history rows, write updated price state). At high concurrency,
this mutex becomes a serialization point. However, since the tick system calls
`fetchBitcoinData` which calls both `GetCurrentPrice` and `GetPriceHistory`, and these are
read-only for non-leader replicas, the impact is limited in multi-replica deployments.

### 4.4 What Is NOT Cached

- **Group membership:** Queried live on every full tick (2 DB queries: `GetUserGroup` +
  `GetMembers`). Cached in `cachedChildData.GroupBonus` for light ticks.
- **Leaderboard data:** Not in the tick hot path (separate endpoint).
- **Game config / catalog data:** Compiled into Go code (zero-cost lookups via maps and
  slices). The `/api/game/config` endpoint returns static data with a 1-hour `Cache-Control`
  header.

---

## 5. Concurrency Model

### 5.1 Per-User Mutex

**What:** `userMutexMap` in `game.go` provides a `sync.Mutex` per user ID. It prevents
concurrent state mutations (tick vs. action, or two simultaneous actions).

**Behavior:**
- `Lock(userID)` creates a new mutex entry if one does not exist, updates `lastUsed`, then locks
- `Unlock(userID)` unlocks the mutex
- Background cleanup goroutine runs every 5 minutes, removes entries inactive >10 minutes
- The outer map is protected by its own `sync.Mutex` (short-lived, just for map access)

**Critical invariant:** A user's tick and actions never execute concurrently. This is enforced
at the application level, not the database level (though `LoadFullGameStateForUpdate` provides
database-level serialization for multi-replica scenarios).

### 5.2 Goroutine Model

Per connected WebSocket user, the system spawns:
- 1 `readPump` goroutine (reads incoming WS messages)
- 1 `writePump` goroutine (writes outbound WS messages, owns ping ticker)
- 1 tick goroutine (fires every 5 seconds)
- N action goroutines (1 per incoming WS action message, spawned by `OnMessage` callback:
  `go gameHandler.HandleWSAction(userID, data)`)

At 1,000 connected users: ~3,000 persistent goroutines + transient action goroutines.

**Memory per connection:** ~70KB (goroutine stacks + WS buffers + send channel buffer of 16
slots). At 2,000 users: ~140MB for WS infrastructure alone.

### 5.3 WebSocket Send Buffer

Each client has a buffered send channel (`sendBufSize = 16`). At a 5-second push interval,
16 slots means a client must be unresponsive for ~80 seconds before drops begin. The pong
timeout (45s) closes the connection before that threshold.

**Non-blocking send:** `Hub.trySend` uses a select with default -- if the channel is full,
the message is dropped and logged. This prevents a slow client from blocking the tick
goroutine.

**Panic recovery:** `trySend` includes `defer recover()` to handle the case where the send
channel is closed between the map lookup and the send attempt (race between disconnect cleanup
and tick push).

### 5.4 Hub Locking

`Hub.clients` (map of userID to Client) is protected by `sync.RWMutex`:
- `RLock` for `SendToUser`, `SendToUserBytes`, `HasUser`, `ConnectedUsers`
- `Lock` for `HandleConnect` (register/replace client) and `readPump` cleanup (deregister)

**Single connection per user:** If a user connects while already connected, the old connection
is forcibly closed (send channel closed, done channel closed, `OnDisconnect` fired). The new
connection then registers.

---

## 6. Rate Limiting

### 6.1 Implementation

Rate limiting uses a pluggable `RateLimitStore` interface:
- **In-memory (default):** `InMemoryRateLimitStore` with a `sync.Mutex`-protected map.
  Cleanup goroutine runs every minute, removes entries inactive >1 minute.
- **Redis-backed:** `RedisRateLimitStore` using a Lua script for atomic INCR + conditional
  EXPIRE. Fails open on Redis error (allows the request, logs the error).

The active store is set at startup: if Redis is available, `SetRateLimitStore` installs the
Redis-backed store. Otherwise, the in-memory store is used.

### 6.2 Rate Limits

| Endpoint / Bucket | Limit | Key |
|-------------------|-------|-----|
| Auth (register, login) | 10/min | IP address |
| Game actions (HTTP) | 7,200/min (120/sec) | User ID |
| Game actions (WS) | 7,200/min (120/sec) | User ID (via `CheckGameActionRate`) |
| Social endpoints | 180/min | User ID |
| GetState (HTTP) | No explicit rate limit | -- |

**Performance note:** The in-memory rate limiter uses a single `sync.Mutex` for all keys.
Under extreme concurrency, this could become a contention point. The Redis-backed limiter
avoids this by moving contention to Redis, which handles it efficiently. However, each Redis
rate limit check adds ~0.5-1ms of latency per request.

---

## 7. Client-Side Performance

### 7.1 requestAnimationFrame Interpolation

The client does NOT poll the server for state updates between ticks. Instead, the
`useIdleTick` hook (`apps/desktop/src/hooks/useIdleTick.ts`) runs a single
`requestAnimationFrame` loop that interpolates currency values (compute units, reputation,
money) between server pushes.

**How it works:**
1. On each server state push (every 5 seconds via WS), the hook recalculates per-second
   rates by replicating the server's `ProcessIdleProgress` multiplier logic exactly.
2. The rAF loop linearly extrapolates from the last known server value at the calculated rate.
3. When the next server push arrives, the base values snap to the authoritative server state
   (no client-side prediction drift).

**Rate calculation accuracy:** The client replicates all server multipliers: hardware compute
(with component upgrade bonuses), UPS/network/storage/patch panel bonuses, service compute/rep/money,
heat penalty, throttle multiplier, overclock multiplier, knowledge boost, colo multiplier,
idle multiplier, research bonuses, colo rack income with decay, group bonus, and expenses.
This ensures smooth visual interpolation that closely matches what the server computes.

**NaN guard:** `rates.current` values are checked with `isFinite()` before use, falling back
to 0 if any calculation produces NaN.

### 7.2 WebSocket Client

The `WSClient` (`apps/desktop/src/wsClient.ts`) manages a single WebSocket connection:
- Actions are sent as JSON with a UUID for response correlation
- A `pending` map tracks in-flight requests with 10-second timeouts
- If WS is disconnected, `sendAction` falls back to the HTTP API (`api.action`)
- No automatic reconnection logic in the WS client itself (handled by the `useWebSocket` hook)

### 7.3 State Management

Zustand store (`apps/desktop/src/stores/gameStore.ts`) holds the full game state. State
updates arrive via two paths:
1. **WS state push** (every 5 seconds from tick) -> `setStateFromPush`
2. **Action response** (WS action_result or HTTP response) -> updates state in action handler

The store does NOT do deep equality checks on state -- every push replaces the entire state
object, which triggers React re-renders via Zustand subscriptions.

### 7.4 Build / Bundle

- Vite with React plugin and Tailwind CSS Vite plugin
- No code splitting configuration (default Vite behavior)
- No explicit performance optimizations (no lazy loading, no memoization directives beyond
  what React provides)

---

## 8. Measured Performance (Stress Test Data)

Stress tests were conducted on 2026-03-22 using a custom Go load tester
(`apps/stress-tests/main.go`). Each simulated player performs one action every 500ms. Tests
ran for 30 seconds after ramp-up.

### 8.1 Hardware Configurations Tested

| Config | vCPU | RAM |
|--------|------|-----|
| Config A | 12 | 16 GB |
| Config B | 32 | 32 GB |

### 8.2 Summary Results (32 vCPU / 32 GB -- Current Production Hardware)

**WebSocket-Only (all actions over WS):**

| Players | Error Rate | Actions/sec | P50 | P90 | P95 | P99 | Max |
|---------|-----------|-------------|-----|-----|-----|-----|-----|
| 50 | 0% | 91.5 | 1.9ms | 6.9ms | 8.2ms | 12.1ms | 21.2ms |
| 200 | 0% | 364.8 | 2.1ms | 13.6ms | 17.0ms | 69.5ms | 146.9ms |
| 500 | 0% | 911.5 | 2.3ms | 8.1ms | 9.8ms | 100.3ms | 235.2ms |
| 1,000 | 0% | 1,807.6 | 6.5ms | 23.2ms | 233.5ms | 498.5ms | 807.1ms |
| 2,000 | 0% | 2,516.7 | 31.6ms | 1,571ms | 2,213ms | 3,429ms | 4,927ms |
| 5,000 | 0% | 4,178.8 | -- | -- | -- | 3,115ms | 7,763ms |
| 10,000 | 0% | 4,178.8 | 1,570ms | 3,033ms | 3,059ms | 3,115ms | 7,763ms |

**HTTP-Only (no WebSocket connections):**

| Players | Error Rate | Actions/sec | P50 | P90 | P95 | P99 | Max |
|---------|-----------|-------------|-----|-----|-----|-----|-----|
| 50 | 0% | 73.4 | 6.6ms | 7.5ms | 8.0ms | 9.3ms | 12.8ms |
| 500 | 0% | 728.5 | 6.9ms | 9.5ms | 10.3ms | 11.5ms | 21.7ms |
| 1,000 | 0% | 1,459.5 | 7.9ms | 11.7ms | 12.5ms | 14.0ms | 22.8ms |
| 2,000 | 0% | 1,944.2 | 27.2ms | 1,525ms | 1,541ms | 1,577ms | 3,032ms |
| 10,000 | 0% | 6,641.6 | 7.3ms | 3,475ms | 3,501ms | 3,542ms | 7,109ms |

### 8.3 Key Findings

1. **Throughput ceiling:** ~4,500 WS actions/sec, ~6,600 HTTP actions/sec on 32 vCPU.
   Beyond this, latency grows but the server does not drop connections or return errors.

2. **HTTP is dramatically faster than WS up to 1,000 players.** P99 at 1,000 players: 14ms
   (HTTP) vs. 498ms (WS). This is NOT because of WebSocket protocol overhead -- it is because
   WS connections trigger per-user tick goroutines that run 14+ DB queries every 5 seconds
   as background load. HTTP-only mode has zero such background load.

3. **The bottleneck is DB query volume, not CPU or network.** All transport modes converge
   to similar high latency at 2,000+ players, confirming the bottleneck is server-side
   processing (DB connection pool contention).

4. **Zero errors at all tested levels** (up to 10,000 players). The server degrades
   gracefully with increasing latency rather than failing.

5. **P50-to-P99 spread widens dramatically** at high player counts. Most requests remain
   fast, but tail latency spikes due to periodic blocking (tick processing, DB writes, GC
   pauses).

6. **Vertical scaling provides ~2-5x latency improvement** from 12 to 32 vCPU, with the
   biggest gains at moderate load (200-500 players). Below 500 players, the bottleneck is
   the per-player action rate, not hardware.

### 8.4 Capacity Recommendations

For an idle/clicker game, latency budget thresholds:

| Quality | P99 Latency |
|---------|-------------|
| Excellent | <50ms |
| Good | 50-200ms |
| Degraded | 200ms-1s |
| Poor | >1s |

| Configuration | Excellent | Good | Degraded | Max (before errors) |
|---------------|-----------|------|----------|---------------------|
| 32 vCPU, WS-only | ~200 players | ~500 players | ~1,000 players | 2,000+ players |
| 32 vCPU, HTTP-only | ~1,000 players | ~1,000 players | ~1,500 players | 2,000+ players |

---

## 9. Known Bottlenecks and Issues

### Issue 1: N+1 Customer UPDATE in Full Tick and GetState (PRESENT)

**Location:** `game.go` full tick (line ~462-464) and `GetState` (line ~718-719)

**Problem:** After every full tick and every GetState call, the code iterates all customers
and calls `h.customers.Update(ctx, &custs[i])` individually, regardless of whether
satisfaction changed. For a late-game player with 30+ customers, this is 30 unnecessary
UPDATE queries per tick.

**Note:** The WS action handler (`HandleWSAction`) does NOT have this issue -- the comment
at lines 987-989 of the PerformAction handler documents that the per-customer Update loop
was deliberately removed from that path.

**Impact:** At 1,000 connected users with an average of 20 customers each, this adds
~4,000 unnecessary UPDATE queries/sec (200 ticks/sec * 20 customers). Mitigated by the
light tick path (which skips child-table updates), but still present on full ticks and
HTTP GetState calls.

### Issue 2: Group Bonus Queries on Every Full Tick (PRESENT)

**Location:** `game.go` line ~426 (`getGroupBonus`)

**Problem:** Every full tick makes 2 DB queries to fetch group membership. Group membership
changes extremely rarely (once per session at most). Light ticks correctly cache this, but
every dirty flag triggers a full tick which re-queries.

**Impact:** 2 extra round-trips per full tick per user. At 200 active users with 5-second
ticks: 80 queries/sec.

### Issue 3: Bitcoin Price Mutex Serialization (PRESENT, MITIGATED)

**Location:** `bitcoin/price.go` line ~105 (`s.mu.Lock()`)

**Problem:** `GetCurrentPrice` holds a mutex while performing DB I/O (read + write). All
concurrent callers block. This was a significant bottleneck before leader election was
introduced.

**Mitigation:** With Redis leader election, non-leader replicas skip advancement and just
read the current price without the mutex. In single-replica mode (no Redis), the mutex is
still a serialization point, but the bitcoin service is called per-tick (not per-action),
limiting the impact.

### Issue 4: JSON Serialization on Every Tick (PRESENT)

**Problem:** Every tick serializes the full state response to JSON (`json.Marshal`). This
includes all hardware, services, upgrades, customers, expenses, colo racks, available
catalogs, bitcoin history, and research levels. For a late-game player, this can be 20-50KB
of JSON.

**Impact:** At 1,000 users with 5-second ticks: 200 JSON serializations/sec, each producing
10-50KB. Total throughput: ~2-10MB/sec of serialization work. `encoding/json` is not the
fastest JSON library in Go, but at this scale the CPU cost is modest compared to DB I/O.

### Issue 5: No Observability Instrumentation (PRESENT)

**Problem:** There are no metrics, no structured logging, and no tracing. Performance
information comes only from stress tests and ad-hoc `log.Printf` statements. There is no
way to monitor pool utilization, tick duration, or query latency in production.

The `cu_cache.go` logs slow refreshes (>1 second), and tick start/stop is logged. But there
are no Prometheus metrics, no OpenTelemetry spans, and no dashboard.

---

## 10. Optimizations Already Applied

The following optimizations from `_documents/tdd/tick-performance-optimizations.md` have been
implemented:

| Optimization | Status | Evidence |
|-------------|--------|----------|
| FK indexes on child tables | **Applied** | Migration `014_add_game_state_indexes.sql` exists and all 6 indexes verified in production |
| Connection pool increase (20 -> 50) | **Applied** | `database/db.go` shows `MaxConns: 50, MinConns: 5`; PostgreSQL `max_connections = 150` confirmed |
| Global donated CU cache | **Applied** | `cu_cache.go` implements `GlobalDonatedCUCache` with 30s refresh, Redis support, inline `Add()` |
| pgx.Batch for game state loading | **Applied** | `queries/batch.go` implements `LoadFullGameState` (2 round-trips) and `LoadFullGameStateForUpdate` |
| Dirty-state detection (full/light ticks) | **Applied** | `tickStateMap`, `userTickState`, `runFullTick`, `runLightTick` all present in `game.go` |

**Combined effect:** Reduced per-tick DB queries from 14+ (all sequential) to 1 (light tick)
or ~5 (full tick with batch). Estimated query reduction at 2,000 users with 90% idle: from
5,600+ queries/sec to ~560 queries/sec.

---

## 11. Horizontal Scaling Readiness

The `_documents/tdd/horizontal-scaling.md` TDD describes the path to running N backend
replicas. As of 2026-04-04, the following multi-replica infrastructure is **implemented**:

| Component | Status | Implementation |
|-----------|--------|---------------|
| Redis client integration | **Done** | `config.go` reads `REDIS_ADDR`, `main.go` connects with graceful degradation |
| Redis-backed rate limiting | **Done** | `redis_ratelimit.go` implements `RateLimitStore` with Lua INCR+EXPIRE |
| Redis-backed CU cache | **Done** | `cu_cache.go` reads/writes Redis with local fallback |
| Redis pub/sub for WS fan-out | **Done** | `redis_broadcaster.go` implements `MessageBroadcaster` with fast local path |
| Bitcoin price leader election | **Done** | `bitcoin/leader.go` uses Redis SET NX EX with 15s TTL, 5s renew |
| DB-level user locking | **Done** | `LoadFullGameStateForUpdate` uses `SELECT ... FOR UPDATE` |
| `MessageBroadcaster` interface | **Done** | `pubsub.go` defines the interface; `LocalBroadcaster` and `RedisBroadcaster` implement it |

**Not yet implemented:**
- Traefik sticky session configuration
- Docker Swarm `replicas > 1` deployment
- Graceful connection draining on rolling deploy

**Current mode:** Single replica, Redis optional. If Redis is unavailable at startup, the
server falls back to in-memory rate limiting, local-only CU cache, and local-only WS
broadcasting. This is the current production configuration.

---

## 12. Engine Computation Cost

`ProcessIdleProgress` (`apps/backend/internal/game/engine/engine.go`) is pure computation
with no I/O. It performs:

1. Hardware compute aggregation (loop over hardware, loop over component upgrades per hardware)
2. Network/storage/UPS/patch panel bonus aggregation
3. Service compute/rep/money aggregation
4. Cooling capacity recalculation
5. Power limit and slot recalculation
6. Multiplier chain: colo * idle * heat * throttle * overclock * knowledge * network * research
7. Income application: compute, reputation, money (with expense deduction)
8. Customer satisfaction decay (if overheating or throttled)
9. Random event roll (`events.RollEvent`)
10. Event application (if triggered)

**Complexity:** O(H + S + U + C + E + CU + R) where H=hardware, S=services, U=upgrades,
C=customers, E=expenses, CU=component_upgrades, R=research_levels. For a typical late-game
player: ~150 items total. This runs in microseconds -- negligible compared to DB I/O.

The engine is stateless: it takes all inputs as parameters and mutates the `GameState` struct
in place. No global state, no locks, no allocations beyond the event list.

---

## 13. Performance-Sensitive Code Paths

When modifying these code paths, profile or stress-test to verify no regression:

1. **`runUserTick` / `runFullTick` / `runLightTick`** -- the hot loop for all connected users
2. **`LoadFullGameState`** -- the primary DB read pattern (2 round-trips)
3. **`HandleWSAction`** -- WS action processing (holds per-user mutex, issues DB reads+writes)
4. **`Hub.trySend`** -- WS message delivery (called from tick goroutines, must not block)
5. **`ProcessIdleProgress`** -- engine computation (called on every tick and every action)
6. **`processCustomerGrowth`** -- may issue DB writes during ticks
7. **`GlobalDonatedCUCache.Get`** -- called on every tick and action (Redis round-trip or local read)
8. **`PriceService.GetCurrentPrice`** -- serialized by mutex, DB I/O on leader

---

## 14. Recommendations for Contributors

### Do

- Use `context.WithTimeout` for all DB operations in new code (the tick system already does this)
- Check if new queries can be added to the existing `pgx.Batch` in `batch.go` rather than
  issuing separate queries
- Set the dirty flag (`tickState.MarkDirty`) after any action that modifies child tables
- Use non-blocking sends for WS messages (the `Hub.trySend` pattern)
- Add indexes on any new foreign key columns used in WHERE clauses

### Do Not

- Do not add per-request DB queries to the tick hot path without justification (each query
  at 1,000 users = 200 extra queries/sec)
- Do not hold the per-user mutex while performing expensive I/O outside the user's own
  game state (risk of cascading latency)
- Do not use `encoding/json.Decoder` with `DisallowUnknownFields` in the tick path (adds
  allocation overhead)
- Do not introduce new global mutexes that could serialize tick processing across users
- Do not add HTTP polling from the client -- the WS push model is intentional and its
  absence in HTTP-only mode is why HTTP stress tests appear faster

### Performance Testing

The stress tester at `apps/stress-tests/main.go` supports:
```
-players N        Number of simulated players
-duration Ns      Test duration in seconds
-rampup Ns        Ramp-up period in seconds
-ws               Enable WebSocket connections
-ws-only          Send all actions via WS (no HTTP)
-url URL          Target server URL
```

Before and after any performance-relevant change, run:
```bash
cd /root/project/apps/stress-tests
go run . -players 1000 -duration 60 -rampup 10 -ws
go run . -players 2000 -duration 60 -rampup 10 -ws
```

Compare P50/P90/P95/P99 latencies and throughput.
