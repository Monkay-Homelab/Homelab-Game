# Known Issues & Technical Debt

Extracted from all project specs (`_documents/spec/*.md`) on 2026-04-04.

---

## Critical

| ID  | Issue                                                                                                                       | Source Spec(s) |
| --- | --------------------------------------------------------------------------------------------------------------------------- | -------------- |
| O1  | **No automated database backups** — no pg_dump cron, no WAL archiving, no point-in-time recovery; single point of data loss | operations     |
| S1  | **No JWT token revocation** — compromised tokens valid for full 24h; no blacklist, no refresh rotation, no jti claim        | security       |

## High

| ID  | Issue                                                                                                                 | Source Spec(s)          |
| --- | --------------------------------------------------------------------------------------------------------------------- | ----------------------- |
| T1  | **Zero frontend tests** — no test files, no test framework, no test runner configured for apps/desktop/               | testing, code-quality   |
| T2  | **Core game engine untested** — ProcessIdleProgress() has no unit tests; affects every player every 5 seconds         | testing                 |
| T3  | **Most game actions untested** — 25+ action types; only bitcoin, overclock, research have tests                       | testing                 |
| T4  | **No authentication flow tests** — login, token validation, password hashing untested; only registration guard tested | testing                 |
| T5  | **No database query tests** — 13 query files (~1000 LOC) with zero coverage                                           | testing                 |
| T6  | **No integration tests** — all tests use mocks/nil deps; no database-dependent code tested in CI                      | testing, operations     |
| T10 | **No frontend build in CI** — desktop app build not verified; typecheck only runs on shared package                   | testing, code-quality   |
| O2  | **No monitoring or alerting** — zero metrics, tracing, or dashboards; outages discovered by users                     | operations, performance |
| F4  | **No branch protection rules** — direct pushes to main possible; no documented CI gate enforcement                    | review-strategy         |
| F7  | **No security scanning in CI** — no gosec, Snyk, Dependabot, or npm audit                                             | review-strategy         |

## Medium — Security

| ID  | Issue                                                                                                               | Source Spec(s)         |
| --- | ------------------------------------------------------------------------------------------------------------------- | ---------------------- |
| S2  | **No HTTP server timeouts** — ReadTimeout, WriteTimeout, IdleTimeout not set; vulnerable to slowloris               | security               |
| S3  | **WebSocket auth via query parameter** — JWT exposed in server/proxy logs and browser history                       | security               |
| S4  | **No security response headers** — missing HSTS, CSP, X-Frame-Options, X-Content-Type-Options                       | security               |
| S5  | **ENV defaults to permissive mode** — localhost CORS/WS origins enabled when ENV != "production"                    | security               |
| S6  | **Redis rate limiter fails open** — on Redis error, all rate limits bypassed entirely                               | security, operations   |
| S14 | **No request/access logging** — cannot audit endpoint hits, detect abuse patterns                                   | security, operations   |
| S16 | **No refresh token pattern** — single 24h JWT serves as both access and session token                               | security               |
| S19 | **No rate limit on WebSocket connections** — /ws endpoint unthrottled; could exhaust goroutines                     | security               |
| S20 | **Read endpoints not rate limited** — GetState, leaderboard can be polled without restriction                       | security               |
| S22 | **Internal error masking inconsistent** — WS errors masked properly; REST handlers sometimes return raw err.Error() | security, code-quality |

## Medium — Testing & Quality

| ID  | Issue                                                                                              | Source Spec(s)        |
| --- | -------------------------------------------------------------------------------------------------- | --------------------- |
| T7  | **No coverage measurement** — go test -cover not run; no thresholds or reporting                   | testing               |
| T8  | **No WebSocket lifecycle tests** — hub, readPump, writePump, connect/disconnect untested (319 LOC) | testing               |
| T9  | **No middleware tests** (except rate limit) — auth, CORS, body limit untested                      | testing               |
| T11 | **No race detection in CI** — go test -race not used; concurrent WS handling unchecked             | testing               |
| T12 | **No bulk action tests** — bulkUpgradeComponents, bulkDeployServices, etc. untested                | testing               |
| T13 | **No event system tests** — 31 event types, tier-weighted, 2% tick rate untested                   | testing               |
| T16 | **No migration validation in CI** — SQL syntax errors only discovered at deploy time               | testing, operations   |
| T17 | **No lint step in CI** — no golangci-lint, eslint, or prettier enforcement                         | testing, code-quality |

## Medium — Operations

| ID  | Issue                                                                                                        | Source Spec(s)           |
| --- | ------------------------------------------------------------------------------------------------------------ | ------------------------ |
| O3  | **Health endpoint shallow** — /health returns ok without verifying DB or Redis connectivity                  | operations               |
| O4  | **No structured logging** — log.Printf throughout; no levels, correlation IDs, or aggregation                | operations, code-quality |
| O5  | **No log rotation or size limits** — Docker json-file logs unbounded; may consume disk                       | operations               |
| O6  | **No migration rollback mechanism** — forward-only; bad migration requires manual SQL correction             | operations               |
| O15 | **No disaster recovery runbook** — no documented procedures for data corruption, VM failure, deploy rollback | operations               |

## Medium — Architecture & Code Quality

| ID  | Issue                                                                                                                 | Source Spec(s)                |
| --- | --------------------------------------------------------------------------------------------------------------------- | ----------------------------- |
| A1  | **Shared package types stale** — packages/shared/types/game.ts missing many fields; api.ts is de facto authority      | architecture, code-quality    |
| A7  | **Idle progress calculated in 5 places** — GetState, PerformAction, HandleWSAction, runFullTick, runLightTick         | code-quality, review-strategy |
| A8  | **engine.go monolith (1701 lines)** — all 25+ game actions in one switch statement                                    | code-quality, review-strategy |
| A9  | **game.go monolith (1348 lines)** — HTTP handlers, WS handlers, tick loop, caching all in one file                    | code-quality, review-strategy |
| C1  | **No Go linter configured** — no golangci-lint or .golangci.yml                                                       | code-quality                  |
| C2  | **No ESLint configured** — no linting for TypeScript frontend                                                         | code-quality                  |
| C4  | **No pre-commit hooks** — no formatting or linting enforcement before commit                                          | code-quality                  |
| C6  | **Repetitive Zustand store pattern** — ~25 near-identical async action methods with no abstraction                    | code-quality                  |
| C9  | **GameState defined in 3 places** — models/game_state.go, api.ts, packages/shared/types/game.ts                       | code-quality                  |
| C10 | **Silent error swallowing in WS persistence** — HandleWSAction doesn't check DB persist errors                        | review-strategy               |
| C12 | **No schema validation on action payloads** — raw json.RawMessage unmarshalled per-handler; no centralized validation | review-strategy               |
| S12 | **Duplicate CORS/WS origin allowlists** — hardcoded in both cors.go and hub.go; manual sync required                  | security, code-quality        |

## Medium — Performance

| ID  | Issue                                                                                                                   | Source Spec(s)          |
| --- | ----------------------------------------------------------------------------------------------------------------------- | ----------------------- |
| P1  | **N+1 customer UPDATE every full tick** — updates all customers regardless of change; 30+ unnecessary UPDATEs late-game | performance             |
| P2  | **Group membership queried every full tick** — 2 queries per tick despite group changes being rare                      | performance             |
| P3  | **Bitcoin price mutex serialization** — holds mutex during DB I/O (mitigated by Redis leader election in multi-replica) | performance             |
| P5  | **No observability instrumentation** — no metrics endpoint; only stress tests for performance data                      | performance, operations |

## Low

| ID  | Issue                                                                       | Source Spec(s)           |
| --- | --------------------------------------------------------------------------- | ------------------------ |
| S7  | No email verification on registration                                       | security                 |
| S8  | Password complexity rules minimal (8-128 chars only)                        | security                 |
| S9  | Redis without authentication in Docker stack                                | security                 |
| S10 | Database connection uses sslmode=disable                                    | security                 |
| S11 | Group names accept arbitrary Unicode (no filtering unlike display names)    | security                 |
| S13 | No authentication audit log (failed/successful logins not logged)           | security                 |
| S21 | No Retry-After header in 429 rate limit responses                           | security                 |
| O10 | No automated deployment trigger (manual docker service update)              | operations               |
| O11 | WebSocket graceful drain not implemented (hard disconnect on deploy)        | operations, architecture |
| O12 | No environment separation (dev/staging/prod on same VM)                     | operations               |
| A2  | Mobile app directory exists but not implemented                             | architecture             |
| A3  | OAuth schema exists but not implemented (Google, Apple, Discord)            | architecture             |
| A4  | resource_history hypertable exists but no code writes to it                 | architecture             |
| A5  | event_log hypertable exists but events only delivered via WS, not persisted | architecture             |
| A6  | Helm charts exist but deployment uses Docker Swarm                          | architecture             |
| C8  | formatNumber() lives in currencyColors.ts instead of dedicated utils file   | code-quality             |
| C14 | MarketPanel has its own formatCurrency() despite consolidation              | code-quality             |
| F6  | No CONTRIBUTING.md                                                          | review-strategy          |
| F5  | No PR template                                                              | review-strategy          |

---

## Summary

| Severity  | Count  |
| --------- | ------ |
| Critical  | 2      |
| High      | 10     |
| Medium    | 37     |
| Low       | 19     |
| **Total** | **68** |

| Category     | Count |
| ------------ | ----- |
| Security     | 22    |
| Testing      | 15    |
| Operations   | 12    |
| Code Quality | 10    |
| Architecture | 6     |
| Performance  | 4     |

---

## Fix Map — Optimal Execution Order

Work is organized into waves. Each wave unlocks the next. Within a wave, items can be
done in parallel. Dependencies are noted where order matters.

### Wave 0 — Foundations (do first, everything else builds on these)

These are infrastructure and tooling changes that make all subsequent work safer, faster,
and verifiable.

```
 0.1  O1   Automated database backups (pg_dump cron + WAL archiving)
            WHY FIRST: Nothing else matters if you lose the database.
            Standalone — no code dependencies.

 0.2  O4   Structured logging (adopt slog or zerolog)
      S14  Request/access logging
      S13  Authentication audit log
            WHY EARLY: Every subsequent change benefits from observable behavior.
            Do together — one logging overhaul, not three.

 0.3  C1   Configure golangci-lint
      C2   Configure ESLint + Prettier
      C4   Add pre-commit hooks (husky + lint-staged)
      T17  Add lint step to CI
            WHY EARLY: Enforces quality on all subsequent code changes.
            Do together — one tooling setup pass.

 0.4  F4   Branch protection rules on main
      F5   PR template
      F6   CONTRIBUTING.md
            WHY EARLY: Process guardrails before the volume of changes increases.
```

### Wave 1 — Security Hardening

With backups, logging, and CI gates in place, harden the attack surface.

```
 1.1  S2   Add HTTP server timeouts (ReadTimeout, WriteTimeout, IdleTimeout)
            Standalone, 5-line change in main.go. Fixes slowloris risk.

 1.2  S4   Add security response headers (HSTS, CSP, X-Frame-Options, etc.)
            New middleware, standalone.

 1.3  S19  Rate limit WebSocket connection attempts
      S20  Rate limit read endpoints (GetState, leaderboard)
            Extend existing rate limit middleware. Do together.

 1.4  S5   Set ENV=production in docker-stack.yml
      S12  Consolidate CORS/WS origin allowlists into single source
            Quick config fixes. S12 removes the drift risk that S5 masks.

 1.5  S22  Consistent internal error masking (REST handlers)
            Audit all http.Error calls; mask internal errors like WS path does.

 1.6  S1   Token revocation + refresh token rotation
      S16  Short-lived access + long-lived refresh tokens
      S17  Add jti claim to JWTs
            DEPENDS ON 0.2 (need audit logging to verify). Largest security
            change — redesigns auth flow. S1/S16/S17 are one coherent effort.
```

### Wave 2 — Test Infrastructure

With linting and CI gates from Wave 0, build out test coverage before refactoring.

```
 2.1  T10  Add frontend build + typecheck to CI
            Quick CI yaml change. Catches type errors before merge.

 2.2  T11  Enable go test -race in CI
            One flag addition. Catches data races in existing 120 tests.

 2.3  T7   Add coverage measurement to CI (go test -cover)
            Report baseline before writing new tests.

 2.4  T4   Write authentication flow tests (login, token validation, password)
      T9   Write middleware tests (auth, CORS, body limit)
            DEPENDS ON 0.3 (linter catches test quality issues). Auth is the
            security boundary — test it before touching it in Wave 1.6.

 2.5  T2   Write ProcessIdleProgress unit tests
      T13  Write event system tests
            The engine is the heart. Test it before refactoring in Wave 3.

 2.6  T3   Write game action tests (hardware, services, upgrades, tier, prestige, SaaS)
      T12  Write bulk action tests
            Broad action coverage. Can parallelize across action categories.

 2.7  T5   Write database query tests
      T6   Write integration tests (real DB in CI)
      T16  Validate migrations in CI
            DEPENDS ON CI having a test database service. One infrastructure
            setup enables all three.

 2.8  T8   Write WebSocket lifecycle tests (hub, connect/disconnect, pumps)
            DEPENDS ON 2.4 (auth middleware tests establish patterns).

 2.9  T1   Add frontend test framework (Vitest) + initial component tests
            Largest testing gap. Start with gameStore.ts (most logic),
            then CurrencyBar, then panels.
```

### Wave 3 — Refactoring (safe now that tests exist)

Tests from Wave 2 make these refactors safe. Each unlocks cleaner code for future work.

```
 3.1  A7   Extract idle progress calculation to shared function
            DEPENDS ON 2.5 (engine tests verify no regression).
            Removes 5-way duplication. Touches game.go + engine.go.

 3.2  A8   Split engine.go by action category
            DEPENDS ON 2.5 + 2.6 (action tests catch breakage).
            e.g., engine_hardware.go, engine_services.go, engine_prestige.go

 3.3  A9   Split game.go into handler + tick + response modules
            DEPENDS ON 2.8 (WS tests) + 3.1 (shared idle progress).
            e.g., game_handler.go, game_tick.go, game_response.go

 3.4  C10  Check DB persist errors in HandleWSAction
      C11  Fix manual mutex management (use defer pattern)
      C12  Centralize action payload schema validation
            DEPENDS ON 3.3 (game.go split makes these changes tractable).
            Do together — all touch the action handling path.

 3.5  C6   Abstract repetitive Zustand store action pattern
      C9   Consolidate GameState type definitions
      A1   Update packages/shared types to match api.ts
            DEPENDS ON 2.9 (frontend tests). Frontend-side cleanup.
            C9 and A1 are the same effort — pick one canonical location.

 3.6  C14  Remove MarketPanel formatCurrency() duplicate
      C8   Move formatNumber() to dedicated utils file
            Minor cleanup. Do alongside 3.5.
```

### Wave 4 — Observability & Operations

With clean code and tests, add production-grade observability.

```
 4.1  O3   Health endpoint dependency checks (DB ping, Redis ping)
            Small change with big operational value. Standalone.

 4.2  P5   Add metrics endpoint (Prometheus-compatible)
      O2   Set up monitoring dashboards (Grafana or similar)
            DEPENDS ON 0.2 (structured logging). Expose tick duration,
            request latency, pool utilization, active connections.

 4.3  O5   Configure log rotation / Docker log limits
            Quick Docker config change.

 4.4  O6   Add migration rollback mechanism (down scripts or documented rollback SQL)
      O15  Write disaster recovery runbook
            DEPENDS ON 0.1 (backups exist to recover from).

 4.5  O11  WebSocket graceful drain on shutdown
            Send close frame before stopping; let clients reconnect to another replica.
```

### Wave 5 — Performance Optimization

With metrics from Wave 4, optimize based on data, not guesses.

```
 5.1  P1   Fix N+1 customer UPDATE (batch or dirty-check)
            DEPENDS ON 4.2 (metrics to measure improvement).

 5.2  P2   Cache group membership across ticks (invalidate on group change)
            DEPENDS ON 4.2 (metrics to verify).

 5.3  P3   Reduce bitcoin price mutex contention (async refresh or read-through cache)
            Only matters at scale. Measure first.
```

### Wave 6 — Feature Gaps & Polish (when ready)

Low-priority items that improve completeness but don't block production quality.

```
 6.1  S7   Email verification flow
      S8   Password complexity rules
            Auth improvements. Do together.

 6.2  S3   Move WebSocket auth from query param to header (Sec-WebSocket-Protocol)
            Breaking change to WS client. Coordinate frontend + backend.

 6.3  A4   Write to resource_history hypertable (or drop it)
      A5   Persist events to event_log hypertable (or drop it)
            Decision: use these tables or remove dead schema.

 6.4  A3   Implement OAuth (Google, Apple, Discord)
      A2   Implement mobile app
            Major features — scope as separate projects.

 6.5  A6   Remove unused Helm charts (or migrate to K8s per TDD)
      O12  Environment separation (if needed)
            Infrastructure decisions deferred until scale demands it.

 6.6  S9   Redis authentication in Docker stack
      S10  Database SSL (if moving off localhost)
      S21  Retry-After header on 429 responses
      O10  Automated deployment trigger
            Minor hardening. Do as convenient.
```

### Dependency Graph

```
Wave 0 (Foundations)
  |
  +---> Wave 1 (Security)          Wave 2 (Testing)
  |       |                           |
  |       +--- 1.6 depends on 0.2    +--- 2.4-2.9 depend on 0.3
  |                                   |
  +-----------------------------------+
                    |
              Wave 3 (Refactoring)
                    |
              Wave 4 (Observability)
                    |
              Wave 5 (Performance)
                    |
              Wave 6 (Features & Polish)
```

### Estimated Effort per Wave

| Wave              | Items | Rough Size | Parallel Tracks                                           |
| ----------------- | ----- | ---------- | --------------------------------------------------------- |
| 0 — Foundations   | 12    | 2-3 days   | 4 (backups, logging, linting, process)                    |
| 1 — Security      | 11    | 3-4 days   | 5 (timeouts, headers, rate limits, config, auth redesign) |
| 2 — Testing       | 14    | 5-7 days   | 4 (CI config, backend tests, DB tests, frontend tests)    |
| 3 — Refactoring   | 12    | 3-4 days   | 3 (engine split, handler split, frontend cleanup)         |
| 4 — Observability | 6     | 2-3 days   | 3 (health, metrics, ops docs)                             |
| 5 — Performance   | 3     | 1-2 days   | 3 (independent optimizations)                             |
| 6 — Polish        | 12    | varies     | as-needed                                                 |
