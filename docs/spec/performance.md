---
project: "project"
maturity: "proof-of-concept"
last_updated: "2026-03-20"
updated_by: "@staff-engineer"
scope: "Performance characteristics, bottlenecks, and optimization state of the Homelab the Game backend, frontend, and database"
owner: "@staff-engineer"
dependencies:
  - architecture.md
  - operations.md
---

# Performance Specification

This document captures the actual performance characteristics of Homelab the Game as they exist
today. It is based on direct codebase analysis and the recorded stress test results from
2026-03-19.

## 1. Architecture Overview (Performance-Relevant)

The system is a monolithic Go backend serving a React (Vite) SPA frontend. All traffic flows
through a single Go process using the standard library `net/http` server. The backend connects
to a single PostgreSQL instance via pgx connection pool. A WebSocket hub runs in the same
process for real-time event delivery. The entire stack runs on a single homelab VM.

There is no reverse proxy layer documented in the codebase itself, though the WebSocket ping
configuration references "nginx proxy" in comments (`ws/hub.go:95`), suggesting nginx sits in
front in production.

## 2. Measured Performance (Stress Test Baseline)

Source: `docs/STRESS-TEST-RESULTS.md` and `stress-tests/main.go`

The custom Go stress tester simulates concurrent players each performing actions every 200ms
(80% game actions, 20% state fetches) over a 60-second run.

### Results Summary

| Players | Actions/sec | Req/sec | P50    | P90    | P95    | P99    | Max    | Errors |
|---------|-------------|---------|--------|--------|--------|--------|--------|--------|
| 100     | 160         | 191     | 2.9ms  | 8.1ms  | 10.1ms | 14.4ms | 35.9ms | 0%     |
| 500     | 1,918       | 2,392   | 3.1ms  | 4.4ms  | 4.7ms  | 5.5ms  | 16.3ms | 0%     |
| 1,000   | 3,691       | 4,615   | 3.9ms  | 6.2ms  | 7.6ms  | 13.1ms | 46.1ms | 0%     |
| 2,500   | 4,478       | 5,592   | 431ms  | 457ms  | 461ms  | 472ms  | 491ms  | 0%     |
| 5,000   | 4,472       | 5,589   | 843ms  | 918ms  | 929ms  | 942ms  | 970ms  | 0%     |

### Key Observations

- **Throughput ceiling:** approximately 4,500 actions/sec (5,600 req/sec total). The system
  handles 1,000 concurrent players with sub-15ms P99 latency.
- **Latency cliff at 2,500+ players:** P50 jumps from 3.9ms to 431ms as the system saturates.
  Throughput only increases roughly 20% while player count increases 2.5x, indicating queue
  buildup rather than CPU saturation.
- **Zero errors at all load levels:** The server never crashes, returns 500s, or drops
  connections. This demonstrates excellent stability under pressure.
- **Registration is bcrypt-bound:** Registration scales linearly with bcrypt cost (~6s for
  1,000 players, ~30s for 5,000).

## 3. Database Performance

### Connection Pooling

**File:** `internal/database/db.go`

Uses `pgx/v5` connection pool (`pgxpool`) with:
- `MaxConns: 20`
- `MinConns: 2`
- SSL disabled (`sslmode=disable`)

This is the **primary bottleneck** identified in stress testing. At 2,500+ concurrent players,
the 20-connection pool becomes saturated. Each game action request executes 8-10 sequential
database queries (1 game state read + reads for hardware, services, upgrades, customers,
expenses, colo racks, component upgrades, and group data) plus 1-20 writes. All queries run
sequentially within a single request handler.

### Query Patterns

All database queries are raw SQL strings executed through pgx. There is no query builder or ORM.

**Read-heavy paths (per request):**

| Query | Table | Access Pattern | Called From |
|-------|-------|---------------|-------------|
| `GetByUserID` | `game_states` | Single row by `user_id` (unique index) | Every GetState and PerformAction |
| `GetByGameStateID` | `hardware` | All rows for a game state | Every GetState and PerformAction |
| `GetByGameStateID` | `services` | All rows for a game state | Every GetState and PerformAction |
| `GetByGameStateID` | `upgrades` | All rows for a game state | Every GetState and PerformAction |
| `GetByGameStateID` | `customers` | All rows for a game state | Every GetState and PerformAction |
| `GetByGameStateID` | `expenses` | All rows for a game state | Every GetState and PerformAction |
| `GetByUserID` | `colo_racks` | All rows for a user (ordered by `colo_at`) | Every GetState and PerformAction |
| `GetByGameStateID` | `component_upgrades` | Join with `hardware` table | Every GetState and PerformAction |
| `GetUserGroup` | `group_members` + `groups` | Join by `user_id` | Every GetState and PerformAction |
| `GetMembers` | `group_members` + `users` | Join by `group_id` | Only when user is in a group |
| `GetGlobalDonatedCU` | `game_states` | `SUM()` aggregate over entire table | Every GetState and PerformAction |

**Write-heavy paths:**
- `Update` on `game_states` (31 columns) runs on every request
- `Update` on all customers runs after every GetState and PerformAction (iterates all customers)
- Bulk actions (e.g., `bulk_upgrade_components`) can trigger many individual INSERT/UPSERT
  operations in a loop, each executed as a separate query

**Notable anti-patterns identified:**
1. **N+1 customer updates:** `game.go:298-300` and `game.go:519-521` iterate over ALL customers
   and call `Update` individually after every request, even if no satisfaction changed.
2. **Full table aggregate on every request:** `GetGlobalDonatedCU` runs
   `SELECT COALESCE(SUM(total_donated_cu), 0) FROM game_states` on every GetState and
   PerformAction call. As the user table grows, this becomes increasingly expensive.
3. **No batching of bulk writes:** Bulk actions like `bulkUpgradeComponents` execute individual
   `Upsert` queries in a loop (`game.go:468-484`) rather than using batch inserts or
   transactions.
4. **No database transactions:** The PerformAction handler performs multiple writes (game state
   update, new hardware insert, customer updates) without wrapping them in a transaction,
   risking partial state on failure.

### Database Indexes

Indexes present in the schema (`001_initial_schema.sql`):

| Table | Index | Type |
|-------|-------|------|
| `users` | `email` | UNIQUE |
| `users` | `(oauth_provider, oauth_id)` | UNIQUE |
| `game_states` | `user_id` | UNIQUE |
| `leaderboard_entries` | `(category, score DESC)` | B-tree |
| `resource_history` | `(user_id, time DESC)` | B-tree (TimescaleDB hypertable) |

**Missing indexes (potential performance impact):**
- `hardware.game_state_id` -- no index, queried on every request
- `services.game_state_id` -- no index, queried on every request
- `upgrades.game_state_id` -- no index, queried on every request
- `customers.game_state_id` -- no index, queried on every request
- `expenses.game_state_id` -- no index, queried on every request
- `colo_racks.user_id` -- no index, queried on every request
- `component_upgrades.hardware_id` -- no explicit index (referenced by JOIN)

These tables use UUID foreign keys. Without indexes, PostgreSQL performs sequential scans when
the tables grow beyond a few pages. For a game with fewer than a few thousand total hardware
rows, this is not yet a problem due to PostgreSQL's ability to keep small tables in shared
buffers. At scale, it will become one.

### TimescaleDB

The schema creates two hypertables (`resource_history` and `event_log`) but **neither table is
written to by any application code**. The hypertables exist in the schema but are unused. No
game handler, engine, or query module references these tables.

## 4. Concurrency Model

### Per-User Mutex

**File:** `internal/api/handlers/game.go:21-48`

A `userMutexMap` provides per-user locking for `PerformAction` requests only. This prevents
race conditions when the same user sends concurrent actions.

```
type userMutexMap struct {
    mu    sync.Mutex
    locks map[string]*sync.Mutex
}
```

**Characteristics:**
- Uses a global mutex (`mu`) to protect the map of per-user mutexes
- Mutex entries are **never cleaned up** -- the map grows monotonically with unique users
- Only `PerformAction` acquires the per-user lock; `GetState` does not, meaning state reads and
  action writes can interleave
- Two concurrent `GetState` requests for the same user will both read and write the game state
  (via `ProcessIdleProgress` + `Update`) without coordination, creating a potential lost-update
  race condition

### WebSocket Hub

**File:** `internal/api/ws/hub.go`

- Single connection per user (new connections replace old ones)
- `RWMutex` protects the `clients` map
- Ping/pong keep-alive at 30s/45s intervals
- One goroutine per connection for ping ticking, one for read loop
- No write serialization per connection: `SendToUser` calls `WriteMessage` directly, which
  is not safe if multiple goroutines send events concurrently (gorilla/websocket requires
  external synchronization for concurrent writes)

### HTTP Server

Uses `net/http.ListenAndServe` with no explicit timeouts configured:
- No `ReadTimeout`
- No `WriteTimeout`
- No `IdleTimeout`

This means slow clients can hold connections open indefinitely, potentially exhausting file
descriptors under load.

## 5. Rate Limiting

**File:** `internal/api/middleware/ratelimit.go`

### Implementation

In-memory rate limiter using a global `sync.Mutex`-protected map of visitors. Each visitor
tracks request count and last-seen time. A background goroutine evicts entries not seen in the
last minute.

### Configured Limits

| Endpoint | Type | Limit | Bucket Key |
|----------|------|-------|------------|
| Auth (register/login) | IP-based | 10/min | `auth:ip:{ip}` |
| Game actions | User-based (fallback to IP) | 7,200/min (120/sec) | `game:user:{uid}` |
| Social actions | User-based (fallback to IP) | 180/min (3/sec) | `social:user:{uid}` |
| GetState, Leaderboard, GetMyGroup | None | Unlimited | -- |

**Observations:**
- The game action rate limit of 120/sec per user is extremely permissive. The frontend polls
  every 5 seconds and sends individual actions on click, so legitimate clients generate perhaps
  1-5 req/sec. The current limit would only catch automated abuse.
- `GetState` has **no rate limit** and runs the full game engine (ProcessIdleProgress) plus
  writes the updated state back to the database. An attacker polling GetState rapidly could
  impose significant database load.
- Rate limit state is in-memory only -- not shared across processes (single instance, so this
  is fine for now).

## 6. Caching

### Current State: No Application-Level Caching

There is **no caching layer** anywhere in the application. Every request hits the database.

The only cache-like behavior:
1. **`/api/game/config`** sets `Cache-Control: public, max-age=3600` (1 hour). This endpoint
   returns a static configuration object computed from Go constants. This is the only response
   with cache headers.
2. **Frontend config caching:** `gameStore.ts:57-65` checks `if (get().config) return;` to avoid
   refetching the config once loaded. The config is fetched once per session.
3. **Frontend optimistic update:** `runJob` in the game store optimistically adds the click
   reward locally before the server roundtrip, providing instant feedback.

### Missing Caching Opportunities

- **Game state:** The full game state is read from 8+ database tables on every request. For a
  polling interval of 5 seconds, this means 8+ queries execute when nothing has changed. An
  in-memory cache with write-through would eliminate the vast majority of these reads.
- **Catalog data:** Available hardware, services, and upgrades are computed from static Go
  slices on every response (`catalog.GetAvailableHardware`, etc.). These are deterministic
  functions of the player's tier. The catalog computation is cheap but the serialization into
  the response is redundant.
- **Global donated CU:** `GetGlobalDonatedCU` runs a SUM aggregate on every request. This is
  a strong candidate for a periodic refresh (e.g., every 30 seconds) rather than per-request.
- **Leaderboard queries:** Live queries against `game_states` with JOINs run on every
  leaderboard view. The `leaderboard_entries` materialized table exists but leaderboard reads
  bypass it, querying live data directly.

## 7. Frontend Performance

### Polling Architecture

**File:** `apps/desktop/src/App.tsx:45-49`

The client polls `GET /api/game/state` every 5 seconds via `setInterval`. This is the primary
driver of server load in normal operation.

**Characteristics:**
- Fixed 5-second interval regardless of game activity or visibility
- No exponential backoff on errors
- No visibility API integration (continues polling when tab is hidden)
- No conditional GET or ETag support -- full state is transferred every time
- No long-polling or server-sent events alternative

### Client-Side Interpolation

**File:** `apps/desktop/src/hooks/useIdleTick.ts`

A `requestAnimationFrame` loop interpolates currency values between server polls using
calculated rates that mirror the server engine's math. This provides smooth visual updates
without requiring high-frequency server requests.

**Characteristics:**
- Runs a continuous rAF loop (started once, never stops while mounted)
- Calculates compute/reputation/money rates from the server state snapshot
- Applies all multipliers matching the server: colo, idle, heat penalty, throttle, knowledge
  boost, network/storage/patch panel bonuses, colo rack income with decay, group bonus
- Guards against NaN propagation with `isFinite()` checks
- Re-syncs base values and rates on every server state update (every 5 seconds)

This is a well-designed optimization that reduces the need for frequent server polling while
providing a responsive UI.

### Bundle and Build

- **Bundler:** Vite 8 with React plugin and Tailwind CSS 4
- **No code splitting** is configured. All components are statically imported in `App.tsx`.
  However, the application is small (approximately 15 components) so the impact is minimal.
- **No lazy loading** of tab panels -- all six tab components render their content eagerly
  but only the active tab is visible.
- Zustand state store is lightweight with no middleware (no persist, devtools, etc.)

### Response Payload Size

Every `GetState` and `PerformAction` response includes the full game state plus:
- All owned hardware, services, upgrades, component upgrades, customers, expenses, colo racks
- All available hardware/services/upgrades/SaaS catalogs for the player's tier
- Triggered events, group bonus info, global donated CU

For a late-game player with many items, this payload could be several KB. There is no pagination,
partial response, or delta update mechanism.

## 8. Game Engine Performance

### ProcessIdleProgress

**File:** `internal/game/engine/engine.go:22-218`

Called on every `GetState` and `PerformAction` request. Operates entirely in-memory on Go
structs (no database calls).

**Computational complexity:**
- Iterates all hardware: O(H) where H = owned hardware count
- For each hardware, iterates all component upgrades: O(H * C) where C = component upgrade count
- Iterates all services: O(S)
- Iterates all upgrades (for cooling values): O(U)
- Iterates all customers (for satisfaction decay): O(K)
- Iterates all expenses: O(E)
- Event roll: O(1) probability check, O(events) if triggered for mitigation check

For a typical late-game player this is perhaps 20-50 hardware items, 10-20 services, 10-20
upgrades, 10-30 customers. The total is well under a few hundred iterations -- trivial
computational cost.

### ProcessAction

Similar complexity to ProcessIdleProgress. The `bulkUpgradeComponents` action has a nested loop
pattern (`for upgraded { for hardware { for components ... } }`) that repeats until no more
upgrades can be afforded. In the worst case, this could iterate many times, but is bounded by
the player's finite compute units and component max levels.

### Catalog Lookups

Hardware, service, and upgrade catalogs use linear scans (`GetHardwareByName`,
`GetServiceByName`, etc.) over slices of approximately 20-30 templates each. These are fast
enough at current scale but would not scale to hundreds of catalog items.

## 9. Identified Bottlenecks (Priority-Ordered)

### Critical (Current Scale)

1. **Per-request database query count:** 8-10 reads + 1-20 writes per request, all sequential.
   This is the primary throughput limiter. The connection pool (20 max) saturates under load.

2. **GetGlobalDonatedCU on every request:** A `SUM()` aggregate across all game states runs
   on every single request. This gets worse as the user count grows.

3. **N+1 customer updates:** All customers are updated individually after every request
   regardless of whether satisfaction changed.

### Significant (Near-Term Scale)

4. **Missing foreign key indexes:** Tables queried on every request (hardware, services,
   upgrades, customers, expenses, component_upgrades) lack indexes on their `game_state_id`
   columns.

5. **No caching of game state:** Every 5-second poll re-reads the entire game state from the
   database even when nothing has changed.

6. **GetState lacks per-user locking:** Concurrent GetState requests for the same user can
   cause lost-update race conditions on game state.

7. **Unbounded rate limiter mutex map:** Per-user mutexes are allocated but never freed, causing
   gradual memory growth proportional to unique user count.

### Future Concerns

8. **Leaderboard queries are live aggregates:** The leaderboard queries join `game_states` with
   `users` and sort by various columns without the benefit of the existing
   `leaderboard_entries` table. The group leaderboard additionally performs a three-table join
   with aggregation.

9. **No HTTP server timeouts:** Slow clients can hold connections indefinitely.

10. **Fixed polling interval:** No adaptive polling, no visibility-based pause, no conditional
    requests.

11. **WebSocket write safety:** `SendToUser` is not protected against concurrent writes to the
    same connection.

## 10. Benchmarking Infrastructure

### Existing

- **Custom stress tester** at `stress-tests/main.go`: A well-built Go load testing tool that
  simulates concurrent players with configurable parameters:
  - Player count (`-players`)
  - Test duration (`-duration`)
  - Ramp-up time (`-rampup`)
  - Action rate per player (`-rate`)
  - Optional WebSocket connections (`-ws`)
  - Reports P50/P90/P95/P99/Max latencies, throughput, and error rates
  - Configured HTTP transport with `MaxIdleConns` scaled to player count
  - Uses per-player `X-Forwarded-For` to avoid IP rate limiting
  - 80/20 action/state-fetch mix matches realistic usage patterns

### Missing

- **No Go benchmarks:** No `_test.go` files exist anywhere in the backend. The game engine
  (`ProcessIdleProgress`, `ProcessAction`) and catalog lookups have no micro-benchmarks.
- **No database query benchmarks:** No measurement of individual query performance.
- **No frontend performance measurement:** No Lighthouse, Web Vitals, or bundle analysis
  tooling.
- **No continuous performance testing:** No CI integration for regression detection.
- **No profiling artifacts:** No pprof endpoints or profiling configuration.

## 11. Scaling Considerations

The system currently targets a single homelab VM deployment. The architecture has several
characteristics relevant to horizontal scaling:

**Barriers to horizontal scaling:**
- In-memory per-user mutex map (process-local)
- In-memory rate limiter state (process-local)
- In-memory WebSocket hub (process-local, single connection per user)
- No session/cache externalization (no Redis or equivalent)

**Favorable for scaling:**
- Stateless game engine (operates on data passed in, no global state)
- Database is already externalized (PostgreSQL)
- Clean separation of concerns (handlers, engine, queries)

For the stated deployment target (self-hosted homelab VM), the current architecture is
appropriate. The stress test results show the system handles 1,000 concurrent players with
excellent latency. Scaling beyond that primarily requires optimizing the database interaction
pattern (fewer queries per request, connection pool tuning, indexes, caching) rather than
architectural changes.

## 12. Optimization Roadmap (Not Yet Implemented)

The following optimizations are documented as identified but not yet implemented. They are
listed in the stress test results (`docs/STRESS-TEST-RESULTS.md`) and confirmed by this
codebase analysis:

1. **Increase connection pool size** and PostgreSQL `max_connections`
2. **Batch database queries** -- combine 8+ individual reads into fewer queries or a single
   query with multiple result sets
3. **In-memory game state cache** with write-through to database
4. **Async writes** for non-critical data (event logs, history, leaderboard updates)
5. **Add missing foreign key indexes** on frequently-queried columns
6. **Cache or periodically refresh** the global donated CU sum
7. **Batch customer updates** instead of N+1 individual updates
8. **Add HTTP server timeouts** (ReadTimeout, WriteTimeout, IdleTimeout)
9. **Implement conditional polling** (visibility API, ETag/If-None-Match, adaptive intervals)
