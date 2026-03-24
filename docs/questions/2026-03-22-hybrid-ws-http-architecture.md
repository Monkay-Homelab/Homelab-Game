# Question: Would a hybrid WS/HTTP architecture be beneficial?

**Date:** 2026-03-22
**Agents consulted:** 6 of 6 (staff-engineer, senior-engineer, devops-engineer, sdet, security-engineer, data-engineer)
**Agents declined:** 0

---

## Summary

**No — a hybrid approach would not produce the performance gains observed in the HTTP-only stress test, and it would reintroduce a solved UX race condition.** The 35x latency advantage of HTTP-only (P99 14ms vs 498ms at 1,000 players) is not caused by WebSocket protocol overhead — it is caused by the server-side tick system, which spawns a goroutine per connected WS user that runs 14+ DB queries every 5 seconds. In a hybrid architecture where WS is kept for push, those tick goroutines still run, so performance would remain close to the WS+HTTP stress test results (Set 2), not the HTTP-only results (Set 4). The real optimization target is the tick system and database query patterns, not the transport layer.

## Detailed Analysis

### Architecture (Staff Engineer)

The stress test comparison is **not apples-to-apples**. HTTP-only mode never establishes WebSocket connections, so no `OnConnect` callback fires, no tick goroutines spawn, and no background DB work occurs. WS modes spawn a dedicated tick goroutine per user that runs `runUserTick` every 5 seconds — performing 14+ DB queries, ProcessIdleProgress, colo calculations, group bonuses, bitcoin data fetch, full state serialization, and a WS push. At 1,000 players, this adds ~200 tick operations/second as background load that competes with player actions for DB connections and the per-user mutex. The HTTP-only mode has zero such background load.

Three architectural options were assessed:

| Option                                               | Risk                               | Impact                                                            | Recommendation           |
| ---------------------------------------------------- | ---------------------------------- | ----------------------------------------------------------------- | ------------------------ |
| **A: Keep current WS architecture + optimize ticks** | Low                                | High (addresses actual bottleneck)                                | **Preferred**            |
| **B: Hybrid (WS push + HTTP actions)**               | High (reintroduces race condition) | Low (doesn't address tick bottleneck)                             | Not recommended          |
| **C: HTTP-only (remove WS entirely)**                | Medium (loses real-time push)      | Mixed (eliminates tick overhead but transfers DB load to polling) | Viable as simplification |

The existing TDD (`docs/tdd/websocket-actions.md`) documents a genuine race condition that motivated moving actions to WS: HTTP action responses and WS tick pushes can arrive at the client in any order, causing UI oscillation where buttons appear to "undo" themselves. Reverting actions to HTTP would reintroduce this bug.

### Implementation (Senior Engineer)

The client already has complete infrastructure for HTTP actions — `api.ts` has the `action()` method, and `wsClient.ts:sendAction()` already falls back to HTTP when WS is disconnected. The implementation effort for a hybrid switch would be ~2 hours (remove `OnMessage` wiring, change ~25 store actions from `wsClient.sendAction` to `api.action`, simplify wsClient).

However, **the hybrid does NOT produce the HTTP-only stress test performance gains**. In the hybrid, the WS connection remains open, so:

- Per-user tick goroutine still runs (the dominant cost)
- 14+ DB queries per user per tick still execute
- Hub RWMutex contention still exists
- Per-connection goroutines (writePump, readPump) still exist
- Full state serialization and push every 5s still happens

The only savings from moving actions to HTTP are eliminating WS message correlation overhead (UUID generation, pending map, timeout timer) — microseconds per action. The `processAction` code path is literally the same function called by both WS and HTTP handlers.

**Critical finding: the client no longer does HTTP polling.** The only `fetchState()` call is at initial load. All ongoing state updates come exclusively from WS `state` pushes. Going hybrid would still require WS for state delivery (or re-adding HTTP polling, which was not tested in the stress test).

### Infrastructure (DevOps Engineer)

Per-WS connection overhead: ~70KB memory, 3 goroutines (readPump + writePump + tick). At 2,000 players: ~140MB + 6,000 goroutines just for WS infrastructure. Neither memory nor file descriptors are the constraint — the bottleneck is the DB connection pool.

**Connection pool saturation analysis:**

- Pool size: 20 connections
- WS tick at 2,000 users: 400 ticks/sec × 14+ queries = 5,600+ pool.Acquire() calls/second
- Plus user actions competing for the same pool
- HTTP polling would generate similar query volume but with natural client jitter spreading the load more evenly

**The hybrid approach does NOT reduce goroutine count materially** — you still need readPump + writePump + tick goroutine per connection. The only way to reduce goroutines is to remove the tick goroutine entirely.

Ranking of approaches by infrastructure impact:

1. **Best: Remove server-side tick entirely** (eliminates 5,600+ background queries/sec)
2. **Second: Keep ticks but batch DB reads** (reduces per-tick queries by ~75%)
3. **Third: Increase pool size to 50-80** (treats symptom, not cause)
4. **Least impactful: Change transport layer** (stress test gains are an artifact of missing tick load)

### Testing / Stress Test Methodology (SDET)

**The stress tester cannot currently validate a hybrid approach.** The existing modes are:

- `ws-only`: WS connections + all actions over WS
- `ws+http`: WS connections + 80% WS actions + 20% HTTP state fetches
- `http-only`: No WS connections, all traffic over HTTP

None test "WS connections for push + all actions via HTTP." The closest analog (Test Set 2: WS+HTTP) showed WS P99 at 1,000 players of 430ms — close to WS-only's 498ms, confirming that having WS connections active (with tick goroutines) dominates the latency regardless of action transport.

**Confounding variables in the stress test:**

| Variable                  | Impact                                                                                                     |
| ------------------------- | ---------------------------------------------------------------------------------------------------------- |
| Tick goroutines (biggest) | 1,000 goroutines × 14+ queries every 5s. Absent in HTTP-only. Accounts for most of the latency difference. |
| Per-user mutex contention | Actions block behind ticks. HTTP-only has no ticks, so actions never wait.                                 |
| DB pool pressure          | WS: ~200 tick queries/sec + action queries. HTTP: action queries only.                                     |
| Action mix difference     | WS-only: 100% actions. HTTP-only: 80% actions + 20% state fetches.                                         |

**Recommended validation steps before any transport change:**

1. Add a `-ws-push-only` mode to the stress tester
2. Test WS-only with different `TICK_INTERVAL_SECONDS` (10s, 15s, 30s) to quantify tick frequency impact
3. Test WS-only with ticks disabled to isolate pure WS transport overhead
4. Add server-side instrumentation for per-user mutex wait time

### Security (Security Engineer)

**The hybrid approach would modestly improve security posture:**

| Finding                                 | Severity | Current (WS Actions)                     | Hybrid (HTTP Actions)              |
| --------------------------------------- | -------- | ---------------------------------------- | ---------------------------------- |
| JWT in URL query param                  | Medium   | Used for all WS communication            | Reduced to push-only (read access) |
| No token expiry on established WS       | Medium   | Expired tokens can still perform actions | Expired WS only receives push data |
| Rate limit applied after JSON parsing   | Low      | CPU wasted parsing before rate check     | Middleware rejects before parsing  |
| No rate limit on WS upgrade             | Medium   | Same regardless of hybrid                | Same (WS still needed for push)    |
| Goroutine amplification via WS messages | Medium   | Each message spawns goroutine            | Eliminable if WS becomes push-only |

If WS becomes push-only, the `readPump` could be simplified to only process pong frames, eliminating VULN-EXISTING-05 entirely. Per-request JWT validation on HTTP actions provides stronger continuous auth than the one-time WS upgrade check.

### Data Layer (Data Engineer)

**DB access patterns are identical regardless of transport.** The 14+ sequential queries per state sync run the same code path whether triggered by a WS tick, HTTP poll, or action handler.

Key DB findings:

- **`GetGlobalDonatedCU` full table scan** runs on every tick/poll/action for every user (400 scans/sec at 2,000 users = 800,000 row reads/sec from this single query)
- **HTTP polling is actually worse for writes** — the GetState HTTP handler still has the N+1 customer UPDATE loop (removed from the tick path but not from GetState/processAction)
- **No dirty-state detection** — ticks execute all 14 queries even when nothing changed (90%+ of ticks for truly idle users are wasted)
- **No query batching** — 14 sequential round-trips instead of pipelined/batched queries

## Cross-Cutting Observations

1. **All 6 agents independently identified the tick system as the real bottleneck** — not one recommended the hybrid approach as a primary optimization. The stress test data is misleading because HTTP-only mode tested a fundamentally different server workload (no tick goroutines).

2. **Test Set 2 (WS+HTTP) already partially validates the hybrid answer** — having WS connections active with HTTP state fetches showed P99 of 430-538ms at 1,000 players, close to WS-only (498ms) and far from HTTP-only (14ms). This confirms the tick system dominates.

3. **The `docs/spec/architecture.md` is outdated** — it states "no background tick loop" and "client polls every 5s," but the current code has per-user tick goroutines and no HTTP polling. Any architecture decision based on the spec would be misinformed.

4. **An ironic finding:** HTTP polling would reintroduce the N+1 customer UPDATE bug that was already fixed in the tick path, potentially making HTTP polling slower than the tick system for write-heavy users.

## Recommendations

**Priority 1 (High impact, addresses actual bottleneck):**

1. **Cache `GetGlobalDonatedCU`** — replace per-request SUM() with a periodic refresh (every 30s). Eliminates 400 full table scans/sec at 2K users.
2. **Batch the 14 sequential reads** using `pgx.Batch` or a single SQL function/CTE. Reduces per-tick DB round-trips by ~10x.
3. **Add dirty-state detection to ticks** — skip the full query cycle when no action has occurred since the last tick. Eliminates 90%+ of idle-user tick overhead.
4. **Increase DB connection pool** from 20 to 50-80 (with matching PostgreSQL `max_connections` increase).
5. **Add missing foreign key indexes** on `game_state_id` columns (hardware, services, upgrades, customers, expenses, component_upgrades).

**Priority 2 (Correctness / reliability):** 6. Fix the N+1 customer UPDATE in GetState and processAction (already removed from tick path). 7. Use `context.WithTimeout` instead of `context.Background()` for tick queries. 8. Wrap multi-write action paths in database transactions. 9. Add HTTP server timeouts (ReadTimeout, WriteTimeout, IdleTimeout).

**Priority 3 (If transport change is still desired after above optimizations):** 10. Add a `-ws-push-only` mode to the stress tester and validate the hybrid approach with real data. 11. If validated, move actions to HTTP and simplify the WS readPump to pong-only. 12. Address the action-result/tick-push race condition in the client (sequence numbers or client-side dedup).

## Files Referenced

- `apps/backend/internal/api/ws/hub.go` — WebSocket hub, connection lifecycle
- `apps/backend/internal/api/handlers/game.go` — Game handler (HandleWSAction, PerformAction, processAction, runUserTick, OnConnect)
- `apps/backend/cmd/server/main.go` — Server wiring, OnMessage callback
- `apps/backend/internal/database/db.go` — Connection pool configuration
- `apps/backend/internal/database/queries/game_state.go` — GetGlobalDonatedCU query
- `apps/backend/internal/api/middleware/ratelimit.go` — Rate limiting
- `apps/backend/internal/api/middleware/auth.go` — JWT auth middleware
- `apps/backend/internal/api/routes/routes.go` — Route registration
- `apps/backend/internal/game/engine/engine.go` — Game engine
- `apps/desktop/src/stores/gameStore.ts` — Client state management
- `apps/desktop/src/wsClient.ts` — WS client with HTTP fallback
- `apps/desktop/src/api.ts` — HTTP client
- `apps/desktop/src/hooks/useWebSocket.ts` — WS connection management
- `apps/desktop/src/hooks/useIdleTick.ts` — Client-side interpolation
- `apps/desktop/src/App.tsx` — Application root
- `apps/stress-tests/main.go` — Stress test implementation
- `docs/stress-tests/stress-test-report.md` — Stress test results
- `docs/tdd/websocket-actions.md` — TDD documenting WS action migration
- `docs/tdd/websocket-state-push.md` — TDD for WS state push
- `docs/spec/architecture.md` — Architecture spec (outdated on WS/polling)
- `docs/spec/performance.md` — Performance spec
