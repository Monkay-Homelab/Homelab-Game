---
project: "project"
maturity: "draft"
last_updated: "2026-03-22"
updated_by: "@staff-engineer"
scope: "5 Priority 1 performance optimizations for the per-user tick system to reduce DB query amplification from 5,600+ queries/sec to ~500 queries/sec at 2,000 connected users"
owner: "@staff-engineer"
dependencies:
  - websocket-actions.md
  - websocket-state-push.md
---

# Tick System Performance Optimizations

## 1. Problem Statement

### What

The per-user tick system generates unsustainable database load at scale. At 2,000 connected
WebSocket users, the tick system alone produces **5,600+ DB queries per second** through a
20-connection pool, causing P99 latency to jump from 13ms (at 1,000 users) to 472ms (at 2,500
users). This is a query amplification problem: each tick fires 14+ sequential DB round-trips,
and ticks run every 5 seconds for every connected user.

### Why Now

The deep-dive investigation (`docs/questions/2026-03-22-hybrid-ws-http-architecture.md`)
conclusively identified the tick system's DB interaction as the actual bottleneck -- all 6
consulting agents independently reached this conclusion. The stress test data shows a clear
latency cliff at 2,500 players, with throughput plateauing at ~4,500 actions/sec. The game is
live at `game.homelab.living` and user growth will hit this ceiling.

### Constraints

- Single homelab VM deployment -- no horizontal scaling, no Redis, no separate caching layer.
- Server-authoritative model must be preserved -- all game state mutations validated server-side.
- PostgreSQL 16 + TimescaleDB on the same host.
- Current `max_connections = 100` in PostgreSQL.
- Zero automated tests exist (relevant to risk assessment and rollback strategy).
- The system must remain available during the optimization rollout -- no extended downtime.

### Acceptance Criteria (Overall)

- P99 latency at 2,000 connected WS users is below 50ms (currently ~470ms).
- Throughput ceiling increases from ~4,500 to 8,000+ actions/sec.
- Zero data loss or game state corruption during and after rollout.
- Stress test validation confirms improvement with before/after comparison at 1,000 and 2,000
  player levels.

## 2. Context & Prior Art

### Current Architecture

Each WebSocket connection triggers `OnConnect` in `game.go:384`, which spawns a goroutine that
calls `runUserTick` every 5 seconds. `runUserTick` (game.go:286-378) performs:

1. Acquire per-user mutex
2. `GetByUserID` on `game_states` (1 query)
3. `GetByGameStateID` on 7 tables: hardware, services, upgrades, customers, expenses,
   component_upgrades, research_levels (7 queries, sequential)
4. `GetByUserID` on `colo_racks` (1 query)
5. `ProcessIdleProgress` (in-memory, fast)
6. `getGroupBonus` -> `GetUserGroup` + `GetMembers` (2 queries)
7. `processCustomerGrowth` (conditional writes)
8. `gameState.Update` (1 query)
9. `fetchBitcoinData` -> `GetCurrentPrice` + `GetPriceHistory` (2 queries, though bitcoin
   data is partially in-memory via PriceService)
10. `GetGlobalDonatedCU` (1 query -- full table SUM)
11. JSON marshal + WS push

**Total: 14+ DB round-trips per tick, per user.** At 2,000 users with 5-second ticks, this is
400 ticks/sec times 14 queries = 5,600+ pool.Acquire() calls/second competing for 20 connections.

The same 14-query pattern also executes in `GetState` (game.go:424-506) and `processAction`
(game.go:528-762), meaning player actions compete with tick background load for the same
connection pool.

### How Solved Elsewhere

- **Connection pooling**: PgBouncer or increasing pool size is standard for pgx-based services.
  The current 20 connections with 100 max_connections leaves significant headroom.
- **Query batching**: `pgx.Batch` is the idiomatic Go approach -- sends multiple queries in a
  single network round-trip. Used extensively in production Go services for exactly this pattern.
- **Dirty-state detection**: Game servers commonly track a "dirty" flag that marks when state
  has changed, skipping DB reads when nothing has been modified. This is standard in game tick
  architectures.
- **Cached aggregates**: Global aggregate caching (e.g., refreshing a SUM every N seconds) is
  a standard pattern for read-heavy dashboards and leaderboards.
- **FK indexes**: PostgreSQL does not auto-create indexes on FK columns. Adding them is the
  single most common "free" performance fix for PostgreSQL applications.

## 3. Alternatives Considered

### Alternative A: Optimize the Tick System (Recommended)

Apply 5 targeted optimizations to the existing tick architecture, addressing DB query
amplification at multiple levels: eliminate unnecessary queries (dirty-state), reduce
round-trips (batching), speed up remaining queries (indexes), expand connection capacity
(pool), and cache expensive aggregates (GlobalDonatedCU).

**Strengths**: Addresses the root cause identified by all 6 investigating agents. Each
optimization is independently deployable and reversible. Preserves the existing WS push
architecture and avoids reintroducing the action/tick race condition documented in
`docs/tdd/websocket-actions.md`.

**Weaknesses**: Requires touching multiple files across the query and handler layers.
Dirty-state detection adds a small amount of state tracking complexity.

### Alternative B: Remove Server-Side Ticks Entirely

Eliminate the per-user tick goroutine and return to lazy computation (calculate idle progress
only when the client requests state or performs an action).

**Strengths**: Eliminates 100% of background DB load from ticks. Simplest possible change.

**Weaknesses**: Breaks the WS state push system, which is the primary state delivery mechanism
(the client no longer polls -- all state updates come from tick pushes). Would require either
re-adding HTTP polling (reintroducing the action/tick race condition) or a fundamentally
different push trigger mechanism. The investigation explicitly ranked this as high-risk.

### Alternative C: In-Memory Game State Cache

Cache the full game state in-memory (Go maps), read from cache instead of DB, and
write-through on mutations.

**Strengths**: Eliminates nearly all DB reads from the tick path.

**Weaknesses**: Adds significant complexity (cache invalidation, memory management, crash
recovery). The single-process architecture makes this feasible but introduces a new class of
bugs (stale reads, cache/DB divergence). Overkill when batching + dirty-state detection can
achieve 90%+ of the benefit with far less risk.

### Recommendation

**Alternative A**. It addresses the actual bottleneck with 5 independently reversible changes,
each of which is well-understood and uses standard patterns. The combined effect should reduce
per-tick DB round-trips by 90%+ for idle users and 10x for active users.

## 4. Architecture & System Design

### 4.1 Optimization 1: Cache GetGlobalDonatedCU

**Current behavior**: `GetGlobalDonatedCU` executes
`SELECT COALESCE(SUM(total_donated_cu), 0) FROM game_states` on every tick, every GetState,
and every processAction. At 2,000 users, this is 400+ full table scans per second.

**Proposed behavior**: A `GlobalDonatedCUCache` struct in the `handlers` package holds the
cached value and refreshes it periodically via a background goroutine.

```
type GlobalDonatedCUCache struct {
    mu    sync.RWMutex
    value int64
    pool  *pgxpool.Pool
}

func NewGlobalDonatedCUCache(pool *pgxpool.Pool, refreshInterval time.Duration) *GlobalDonatedCUCache
func (c *GlobalDonatedCUCache) Get() int64
func (c *GlobalDonatedCUCache) refresh(ctx context.Context)  // runs in background goroutine
```

- Refresh interval: 30 seconds (the donated CU value changes rarely -- only on explicit
  `donate_cu` actions).
- The `donate_cu` action path can optionally bump the cached value inline (add the donated
  amount to the cached total) for immediate feedback, with the next periodic refresh
  correcting any drift.
- The cache is initialized with a blocking query at startup before accepting connections.
- `GameHandler` receives `*GlobalDonatedCUCache` as a constructor dependency.
- All three call sites (`runUserTick`, `GetState`, `processAction`) replace
  `h.gameState.GetGlobalDonatedCU(ctx)` with `h.globalCUCache.Get()`.

**Impact**: Eliminates 400+ queries/sec at 2K users. Reduces to 1 query every 30 seconds.

### 4.2 Optimization 2: Batch Sequential Reads with pgx.Batch

**Current behavior**: Each of `runUserTick`, `GetState`, and `processAction` executes 9
sequential SELECT queries (game_state + 7 child tables + colo_racks), each requiring a
separate connection pool acquire/release cycle and network round-trip.

**Proposed behavior**: Introduce a `LoadFullGameState` function in the `queries` package that
uses `pgx.Batch` to send all 9 SELECTs in a single network round-trip.

```
// In queries/batch.go (new file)

type FullGameData struct {
    GameState      *models.GameState
    Hardware       []models.Hardware
    Services       []models.Service
    Upgrades       []models.Upgrade
    Customers      []models.Customer
    Expenses       []models.Expense
    ColoRacks      []models.ColoRack
    ComponentUps   []models.ComponentUpgrade
    ResearchLevels []models.ResearchLevel
}

func LoadFullGameState(ctx context.Context, pool *pgxpool.Pool, userID string, gameStateID string) (*FullGameData, error)
```

**Implementation approach**:

1. Construct a `pgx.Batch` with all 9 SELECT queries.
2. Call `pool.SendBatch(ctx, &batch)` -- this sends all queries in one network round-trip.
3. Parse results in order using `br.QueryRow()` and `br.Query()` for each result set.
4. Return the assembled `FullGameData` struct.

The batch requires both `userID` (for game_state and colo_racks lookups) and `gameStateID`
(for all child table lookups). Since we need the game_state_id from the first query's result
to parameterize the child queries, we have two options:

- **Option A (two-phase)**: First query `game_states` by user_id, then batch the remaining 8
  child queries using the returned game_state_id. This reduces round-trips from 9 to 2.
- **Option B (single batch with subquery)**: Use `(SELECT id FROM game_states WHERE user_id = $1)`
  as a subquery parameter in each child query within the same batch. This sends all 9 in one
  round-trip but relies on PostgreSQL re-evaluating the subquery efficiently (it will, since
  game_states.user_id has a unique index).

**Recommendation: Option A (two-phase)**. It is simpler to implement, easier to debug, and
still achieves a ~5x reduction in round-trips (from 9 to 2). The first query is a single-row
lookup by unique index and returns in <1ms.

The group bonus queries (`GetUserGroup` + `GetMembers`) remain separate because `GetMembers`
depends on the result of `GetUserGroup`. These could be combined into a single query with a
LEFT JOIN, but this is a Priority 2 optimization that can be done later.

**Impact**: Reduces per-tick DB round-trips from 14 to ~5 (1 game_state lookup + 1 batch of 8 +
GetUserGroup + GetMembers + GetGlobalDonatedCU[cached] + gameState.Update).

With dirty-state detection (Optimization 3), idle ticks skip the batch entirely, so the
effective round-trip reduction for the common case is even greater.

### 4.3 Optimization 3: Dirty-State Detection

**Current behavior**: Every tick executes the full 14-query cycle regardless of whether
anything changed since the last tick. For truly idle users (no actions, no customer growth,
no events), 90%+ of ticks are pure waste -- they read the same state, compute zero changes,
and push an identical response.

**Proposed behavior**: Track a per-user "dirty" flag that is set when an action modifies state
and cleared after a tick processes it. When the flag is clean, the tick skips DB reads entirely
and either pushes the last-known state or skips the push.

**Design**:

```
// In handlers/game.go

type userTickState struct {
    dirty       bool                 // true = action occurred since last tick
    lastResp    *fullStateResponse   // cached last response (nil until first tick)
    lastPayload []byte               // pre-serialized JSON of lastResp
}

type tickStateMap struct {
    mu    sync.RWMutex
    state map[string]*userTickState  // userID -> tick state
}
```

**Flag lifecycle**:

1. `OnConnect`: Initialize `userTickState{dirty: true}` for the user (first tick always runs).
2. `processAction` (after successful mutation): Set `dirty = true`.
3. `runUserTick`:
   - If `dirty == false` AND `lastPayload != nil`: Push cached `lastPayload` over WS (user
     still needs to see their state periodically for bitcoin price updates and time-based
     changes like customer growth). **However**, some state changes are time-dependent -- the
     engine calculates idle progress based on elapsed time. For truly idle users, idle income
     is still being earned. Therefore:
   - **Refined approach**: Instead of skipping entirely, separate the tick into two modes:
     - **Full tick** (dirty == true or first tick): Full DB load, engine processing, persist, push.
     - **Light tick** (dirty == false): Still compute idle progress on the cached in-memory
       state (no DB read), update `last_tick_at`, persist the game_state update (1 query),
       and push updated state. This captures idle income accurately while avoiding 8 child
       table reads.
4. `OnDisconnect`: Remove the user's entry from `tickStateMap`.

**Light tick query count**: 1 (gameState.Update only -- the child tables haven't changed, so
we reuse the in-memory copies from the last full tick).

**Additionally**: The light tick must still handle:
- Bitcoin price updates (resolved from the in-memory PriceService, no DB query).
- Global donated CU (resolved from the cache, no DB query).
- Group bonus (can cache the result from the last full tick since group membership changes
  are infrequent).
- Random events (engine-computed, no DB query).
- Customer satisfaction decay (computed in-memory from cached customer list).
- Customer growth (uses cached service list, only writes if growth occurs).

The light tick effectively keeps the cached `FullGameData` in memory and re-runs
`ProcessIdleProgress` on it, only persisting the game_state row.

**Edge case -- concurrent action during tick**: The per-user mutex already serializes tick
and action execution. When an action sets dirty=true, the next tick will run a full tick and
refresh all cached data.

**Edge case -- prestige**: The prestige action wipes child tables. After prestige,
`processAction` sets dirty=true, and the next full tick reloads all data from DB, which now
reflects the wipe.

**Impact**: For a server with N connected users where P% are truly idle (no actions in the
last tick interval), this reduces total tick DB queries by roughly:
- Full tick: ~5 round-trips (with batching) for dirty users
- Light tick: 1 round-trip (game_state update) for idle users
- At 2,000 users with 90% idle: 200 full ticks * 5 + 1,800 light ticks * 1 = 2,800 queries/sec
  (down from 5,600+)

### 4.4 Optimization 4: Increase DB Connection Pool

**Current behavior**: `database/db.go` hardcodes `MaxConns: 20, MinConns: 2`. PostgreSQL
`max_connections = 100`.

**Proposed behavior**: Increase pool to 50 connections. Increase PostgreSQL `max_connections`
to 150 to provide headroom for superuser connections, monitoring tools, and manual psql
sessions.

**Sizing rationale**:
- With optimizations 1-3 applied, the peak query rate at 2,000 users drops from 5,600+/sec
  to ~2,800/sec (and further with the 90%+ idle assumption).
- At an average query duration of ~1ms (single-row indexed lookups), 50 connections can handle
  50,000 queries/sec in the theoretical maximum. Even at 5ms average, capacity is 10,000/sec.
- 50 connections leaves 50 of the 150 max_connections available for other uses (superuser,
  monitoring, manual admin queries, future services).
- The VM is a single machine -- 50 connections is well within PostgreSQL's comfort zone for
  memory usage (each connection uses ~5-10MB of RAM for work_mem and shared buffers).

**PostgreSQL configuration change** (applied via ALTER SYSTEM or postgresql.conf):
```sql
ALTER SYSTEM SET max_connections = 150;
-- Requires PostgreSQL restart to take effect
```

**Application code change** in `database/db.go`:
```go
config.MaxConns = 50
config.MinConns = 5
```

The MinConns increase from 2 to 5 ensures a warm pool on startup, avoiding the cold-start
latency of establishing connections under initial load.

**Impact**: Expands connection throughput by 2.5x, reducing queue wait times when burst
load exceeds the optimized query rate.

### 4.5 Optimization 5: Add Missing Foreign Key Indexes

**Current behavior**: The following tables lack indexes on their foreign key columns, which
are used in WHERE clauses on every tick and every action:

| Table | Column | Queries Affected |
|-------|--------|-----------------|
| `hardware` | `game_state_id` | GetByGameStateID, DeleteByGameStateID |
| `services` | `game_state_id` | GetByGameStateID, DeleteByGameStateID |
| `upgrades` | `game_state_id` | GetByGameStateID, DeleteNonPersistent |
| `customers` | `game_state_id` | GetByGameStateID, DeleteByGameStateID |
| `expenses` | `game_state_id` | GetByGameStateID, DeleteByGameStateID |
| `colo_racks` | `user_id` | GetByUserID |

Note: `component_upgrades` joins through `hardware` (which itself needs the index), and
`research_levels` already has an index (from `012_research_tree.sql`).

**Proposed migration**: Migration `014_add_game_state_indexes.sql` already exists in the
codebase with the correct CREATE INDEX statements. It needs to be applied to the database.

```sql
-- Already written at: apps/backend/internal/database/migrations/014_add_game_state_indexes.sql
CREATE INDEX IF NOT EXISTS idx_hardware_game_state ON hardware(game_state_id);
CREATE INDEX IF NOT EXISTS idx_services_game_state ON services(game_state_id);
CREATE INDEX IF NOT EXISTS idx_upgrades_game_state ON upgrades(game_state_id);
CREATE INDEX IF NOT EXISTS idx_customers_game_state ON customers(game_state_id);
CREATE INDEX IF NOT EXISTS idx_expenses_game_state ON expenses(game_state_id);
CREATE INDEX IF NOT EXISTS idx_colo_racks_user ON colo_racks(user_id);
```

`CREATE INDEX IF NOT EXISTS` is safe to run on a live database and does not acquire exclusive
locks (though it does acquire a ShareLock that blocks writes to the table during creation).
For small tables (current scale), this completes in milliseconds. For larger tables, consider
`CREATE INDEX CONCURRENTLY` (which does not block writes).

**Impact**: Converts sequential scans to index scans for all child-table lookups. The effect
is proportional to table size -- minimal at current scale (small tables fit in shared buffers)
but critical as the player base grows. This is a preventive optimization with zero downside.

## 5. Priority 2 Easy Wins

The following items are not in the core 5 optimizations but touch the same code paths and
should be considered during implementation:

### 5.1 Fix N+1 Customer UPDATE in GetState and processAction

**Location**: `game.go:490-492` (GetState) and `game.go:737-739` (processAction) both iterate
all customers and call `h.customers.Update(ctx, &custs[i])` individually, even when no
satisfaction changed.

**Note**: The tick path (`runUserTick`) already has this fixed -- the comment at line 350-353
explicitly documents that the per-customer Update loop was removed from the tick path.

**Fix**: Apply the same fix to GetState and processAction: only update customers whose
satisfaction actually changed (tracked by the engine's customer decay logic). Alternatively,
use a batch UPDATE:
```sql
UPDATE customers SET satisfaction = $2 WHERE id = $1
```
with a `pgx.Batch` for all modified customers.

**Impact**: Eliminates N unnecessary UPDATE queries per GetState/processAction call, where N
is the number of customers. For a late-game player with 30+ customers, this is significant.

### 5.2 Add context.WithTimeout for Tick Queries

**Location**: `game.go:394,401` -- `runUserTick` passes `context.Background()` to all DB
queries.

**Fix**: Use `context.WithTimeout(context.Background(), 10*time.Second)` for tick operations.
This prevents a single slow query from blocking the tick goroutine indefinitely and holding
the per-user mutex.

**Impact**: Prevents cascading latency during DB slowdowns. A timed-out tick is recovered
on the next interval (as documented in the runUserTick comment).

### 5.3 Combine Group Bonus Queries

**Location**: `game.go:165-181` -- `getGroupBonus` makes two sequential queries:
`GetUserGroup` then `GetMembers`.

**Fix**: Combine into a single query:
```sql
SELECT COUNT(*) FROM group_members WHERE group_id = (
    SELECT group_id FROM group_members WHERE user_id = $1
)
```
Or cache group membership per-tick (group changes are rare -- once per session at most).

**Impact**: Saves 1 round-trip per tick for users in groups.

## 6. Interaction Between Optimizations

The 5 optimizations are designed to layer:

```
                Before          After (combined)
Idle tick:     14 queries  ->   1 query  (light tick: just game_state.Update)
Active tick:   14 queries  ->   5 queries (batched reads + update + group)
GetState:      14 queries  ->   5 queries (batched reads + update + group)
processAction: 14+ queries ->   5+ queries (batched reads + action writes + update + group)
GlobalDonatedCU: per-request -> every 30 seconds
Index impact:   seq scan    ->   index scan (faster per-query)
Pool capacity:  20 conns    ->   50 conns  (2.5x throughput ceiling)
```

**Combined impact at 2,000 users (estimated)**:

| Metric | Before | After |
|--------|--------|-------|
| Tick queries/sec (90% idle) | 5,600+ | ~560 (200 active * 5 + 1,800 idle * 1/5 amortized) |
| Pool utilization at idle | ~112% (queuing) | ~11% |
| Per-query latency (with indexes) | 1-5ms | <1ms |
| P99 action latency at 2K users | ~470ms | <50ms (target) |

The optimizations compound: dirty-state detection eliminates most of the read load, batching
reduces the remaining reads, indexes speed up each query, the larger pool handles burst load,
and the cached aggregate removes the worst single query.

## 7. Migration & Rollout

### Phase Ordering

The optimizations are ordered by risk (lowest first) and dependency:

| Phase | Optimization | Risk | Dependencies | Estimated Size |
|-------|-------------|------|-------------|---------------|
| 1 | FK Indexes (5) | Zero | None | S |
| 2 | Pool Increase (4) | Very Low | Phase 1 (indexes make queries faster per-connection) | S |
| 3 | GlobalDonatedCU Cache (1) | Low | None (but benefits from Phase 2) | S |
| 4 | pgx.Batch Read Batching (2) | Medium | None (but phases 1-3 reduce load during testing) | M |
| 5 | Dirty-State Detection (3) | Medium | Phase 4 (light tick reuses batched data structures) | M |

### Phase 1: Apply FK Indexes

1. Apply migration 014:
   `cat /root/project/apps/backend/internal/database/migrations/014_add_game_state_indexes.sql | sudo -u postgres psql -d homelab_game`
2. Verify indexes exist:
   `echo "\\di idx_hardware_game_state; \\di idx_services_game_state;" | sudo -u postgres psql -d homelab_game`
3. No application restart needed.

**Rollback**: `DROP INDEX IF EXISTS idx_hardware_game_state, idx_services_game_state, ...`

### Phase 2: Increase Connection Pool

1. `ALTER SYSTEM SET max_connections = 150;` via psql.
2. Restart PostgreSQL: `sudo systemctl restart postgresql`.
3. Update `database/db.go`: `MaxConns = 50, MinConns = 5`.
4. Rebuild and restart the backend.

**Rollback**: Revert `db.go` changes, restart backend. PostgreSQL max_connections can remain
at 150 (no harm in the higher limit).

### Phase 3: GlobalDonatedCU Cache

1. Implement `GlobalDonatedCUCache` struct.
2. Wire into `GameHandler` constructor in `main.go`.
3. Replace 3 call sites (`runUserTick`, `GetState`, `processAction`).
4. Rebuild and restart.

**Rollback**: Revert the 3 call sites to direct DB queries.

### Phase 4: Batch Reads

1. Create `queries/batch.go` with `LoadFullGameState`.
2. Update `runUserTick`, `GetState`, `processAction` to use `LoadFullGameState` instead of 9
   individual query calls.
3. Rebuild and restart.

**Rollback**: Revert the 3 callers to individual queries. The `batch.go` file can remain
(unused code is inert).

### Phase 5: Dirty-State Detection

1. Implement `tickStateMap` and `userTickState` in `handlers/game.go`.
2. Add light tick path to `runUserTick`.
3. Set dirty flag in `processAction` after successful mutation.
4. Manage lifecycle in `OnConnect`/`OnDisconnect`.
5. Rebuild and restart.

**Rollback**: Remove the dirty-state check (always run full tick). This reverts to Phase 4
behavior.

### Stress Test After Each Phase

After each phase, run the stress tester at 1,000 and 2,000 players and record P50/P90/P95/P99
latencies and throughput. This provides a before/after comparison for each optimization and
validates that no regression occurred.

```bash
cd /root/project/apps/stress-tests
go run . -players 1000 -duration 60 -rampup 10 -ws
go run . -players 2000 -duration 60 -rampup 10 -ws
```

## 8. Risks & Open Questions

### Known Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| Dirty-state cache becomes stale (e.g., external DB modification) | Low | The cache only covers child tables (hw, svcs, etc.) which are never modified externally. The game is single-process. On restart, all caches are cleared. |
| Pool increase causes PostgreSQL OOM | Very Low | 50 connections at ~10MB each = ~500MB. The VM has far more RAM than this. Monitor with `pg_stat_activity`. |
| Batch query failure is harder to debug | Low | If any query in the batch fails, the entire batch fails. Add structured error wrapping that identifies which query failed. |
| Light tick accumulates floating-point drift | Low | The game engine already handles this (multiplier calculations are re-derived from authoritative game_state fields on each tick). A full tick every time an action occurs corrects any drift. |
| Index creation blocks writes momentarily | Very Low | At current table sizes (hundreds of rows), index creation completes in milliseconds. For future growth, use CONCURRENTLY. |

### Open Questions

1. **Should light ticks still push state?** An idle user with no changes gets the same state
   pushed every 5 seconds (with updated idle income). This is useful for bitcoin price updates
   and smooth client display, but could be optimized further by checking if the response has
   materially changed. **Resolution: Yes, push on every tick.** The client relies on regular
   state pushes for bitcoin price chart updates and idle income display.

2. **Should the batch include group queries?** The current design keeps group queries separate
   because `GetMembers` depends on `GetUserGroup` result. This is a valid Priority 2 item
   for later optimization. **Resolution: Defer to Priority 2.**

3. **Should the GlobalDonatedCU cache be application-global or per-handler?** Since there is
   only one server process and one GameHandler instance, these are equivalent. Use
   application-global (initialized in main.go and passed to GameHandler) for cleaner
   separation. **Resolution: Application-global.**

### Assumptions

- The stress tester in `apps/stress-tests/` is representative of real player behavior (80/20
  action/state-fetch mix with WS connections).
- 90% of connected users are "idle" at any given 5-second window (no action between ticks).
  This is a conservative estimate for an idle/AFK game.
- PostgreSQL shared_buffers is sufficient to keep small tables (hundreds of rows) in memory
  even without explicit indexes. The indexes are a preventive measure.

## 9. Testing Strategy

### Pre-Implementation Baseline

Before any changes, run the stress tester and record baselines:

```bash
# WS-only mode at 1K and 2K players
cd /root/project/apps/stress-tests
go run . -players 1000 -duration 60 -rampup 10 -ws
go run . -players 2000 -duration 60 -rampup 10 -ws
```

### Per-Phase Validation

After each phase deployment, rerun the same stress tests and compare:
- P50/P90/P95/P99 latencies
- Total throughput (actions/sec)
- Error rate

### Functional Verification

Since no automated tests exist, manual verification after each phase:

1. **Phase 1 (Indexes)**: Run `EXPLAIN ANALYZE` on the 6 affected queries to confirm index
   scans instead of sequential scans.
2. **Phase 2 (Pool)**: Check `pg_stat_activity` to confirm connections are being used. Run
   stress test to confirm reduced queue wait time.
3. **Phase 3 (Cache)**: Perform a `donate_cu` action and verify the global donated CU value
   updates within 30 seconds. Check that the value is present in the state response.
4. **Phase 4 (Batch)**: Verify full game state response matches previous response for the same
   user (all fields populated, no missing child data).
5. **Phase 5 (Dirty-State)**: Verify that after performing an action, the next tick reflects
   the action's effects. Verify that an idle player still receives regular state pushes with
   accumulating idle income.

### DB Query Monitoring

Use PostgreSQL statistics to verify query reduction:

```sql
-- Before/after: check query execution counts
SELECT query, calls, mean_exec_time, total_exec_time
FROM pg_stat_statements
WHERE query LIKE '%game_states%' OR query LIKE '%hardware%'
ORDER BY total_exec_time DESC
LIMIT 20;
```

Enable `pg_stat_statements` if not already active:
```sql
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
SELECT pg_stat_statements_reset();
```

## 10. Observability & Operational Readiness

### Key Signals

- **Pool utilization**: `pool.Stat().AcquiredConns()` / `pool.Stat().MaxConns()` -- log every
  30 seconds. Alert if sustained above 80%.
- **Tick duration**: Log `runUserTick` wall-clock time. Track full vs. light tick counts.
  Alert if full tick duration exceeds 1 second.
- **Batch query duration**: Log `LoadFullGameState` execution time.
- **Cache hit rate**: Log GlobalDonatedCU cache refresh count and staleness.

### Diagnosability at 3am

If the server becomes slow after deployment:

1. Check `pg_stat_activity` for connection count and wait events.
2. Check tick goroutine count (`log.Printf` already logs tick start/stop).
3. Check if dirty-state is stuck (all ticks running full path despite no actions -- indicates
   the dirty flag is not being cleared).
4. The GlobalDonatedCU cache refresh goroutine should log if the refresh query itself is slow
   (>1 second).

### Rollback Decision

If stress tests show regression at any phase, stop and investigate. Each phase can be rolled
back independently (see Migration & Rollout). The order is designed so that earlier phases
are lower risk and later phases can be skipped if the performance target is met.

## 11. Implementation Phases

| Phase | Description | Files Modified | Size | Parallelizable |
|-------|------------|---------------|------|----------------|
| 1 | Apply FK index migration to database | None (SQL migration only) | S | Yes (independent) |
| 2 | Increase pool to 50, PG max_connections to 150 | `database/db.go` | S | Yes (after Phase 1) |
| 3 | GlobalDonatedCU periodic cache | `handlers/game.go`, `cmd/server/main.go` (new cache struct, 3 call site changes) | S | Yes (after Phase 2) |
| 4 | pgx.Batch for game state loading | New `queries/batch.go`, `handlers/game.go` (3 call site changes) | M | Depends on Phase 2 (pool headroom for testing) |
| 5 | Dirty-state detection with light/full tick modes | `handlers/game.go` (tick state map, runUserTick refactor, processAction flag) | M | Depends on Phase 4 (uses batched FullGameData struct) |
| P2a | Fix N+1 customer UPDATE in GetState/processAction | `handlers/game.go` (2 call sites) | S | Parallel with any phase |
| P2b | Add context.WithTimeout to tick queries | `handlers/game.go` (runUserTick) | S | Parallel with any phase |
| Validate | Stress test after each phase | None (operational) | S | Sequential (each validates previous) |

**Estimated total effort**: M (medium) -- the individual changes are small but span multiple
files and require careful testing between phases.
