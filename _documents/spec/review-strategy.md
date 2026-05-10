---
project: "homelab-the-game"
maturity: "experimental"
last_updated: "2026-04-04"
updated_by: "@staff-engineer"
scope: "Code review strategy, high-risk areas, CI quality gates, and contribution guidelines for Homelab the Game"
owner: "@staff-engineer"
dependencies: []
---

# Review Strategy

This document defines the code review strategy for Homelab the Game. It covers review
dimensions, high-risk areas requiring careful scrutiny, existing CI quality gates, gaps in
automated enforcement, and contribution workflow guidance for new developers.

The maturity is "experimental" because the project has minimal CI enforcement, no linter
gates, no coverage thresholds, no PR templates, and no CONTRIBUTING.md. Review practices
exist informally at best.

---

## 1. Review Dimensions

Every code review should evaluate changes across these six dimensions, weighted by the risk
profile of the change. Low-risk changes (docs, cosmetic) need only a quick sanity check.
High-risk changes (game engine, auth, currency, data layer) need all six.

### 1.1 Architecture

- Does the change fit the server-authoritative model? All game state mutations MUST be
  validated server-side. Clients send actions; the server validates and returns updated state.
- Does it respect module boundaries? The backend is organized as `cmd/server/`, `internal/api/`,
  `internal/auth/`, `internal/game/`, `internal/database/`, `internal/models/`.
- Does it maintain the separation between the game engine (`engine.go` -- pure game logic,
  no DB access) and the handler layer (`game.go` -- orchestration, DB I/O, WebSocket push)?
- Does it introduce new dependencies? Evaluate total cost: security surface, maintenance
  burden, license compatibility, transitive weight.

### 1.2 Security

- Auth: JWT validation via `internal/auth/jwt.go`, middleware in `internal/api/middleware/auth.go`.
  HS256 signing with a shared secret. Token passed as Bearer header for REST, query param for WS.
- Input validation: Handler-level validation exists but is inconsistent. Action payloads
  are validated in the engine via catalog lookups, but raw `json.RawMessage` payloads flow
  through without schema validation.
- Rate limiting: IP-based and user-based via `middleware/ratelimit.go`. Auth routes: 10/min/IP.
  Game actions: 7200/min/user (120/sec for WS actions). Social: 180/min/user.
- WebSocket origin check: Allowlist in `ws/hub.go` with hardcoded origins plus env-based
  extras. Dev mode allows localhost.
- Display name validation: Blocklist-based profanity filter in `handlers/auth.go`. Regex
  enforces alphanumeric only.
- Error masking: WS action handler uses `actionError` type to classify internal vs. client
  errors, preventing stack trace leaks. REST handlers are less consistent -- some return raw
  `err.Error()`.

### 1.3 Operations

- No structured logging (uses `log.Printf` throughout). No request tracing.
- No health check beyond `GET /health` returning `{"status":"ok"}` (no DB connectivity check,
  no dependency health).
- Graceful shutdown with 10-second drain period in `main.go`.
- No metrics endpoint (no Prometheus, no StatsD).
- No alerting configuration.
- Per-user tick goroutines with 5-second interval, 10-second context timeout.
- Panic recovery exists in `HandleWSAction` to prevent goroutine crashes from taking down
  the process.

### 1.4 Performance

- `batch.go` consolidates 9 sequential DB queries into 2 round-trips (1 for game_state,
  1 pgx.Batch for 8 child tables). This is the critical data path -- every tick and every
  action starts here.
- Light tick optimization: when a user has not acted since the last tick, the handler reuses
  cached child data and only persists the `game_state` row (1 DB write instead of 14+).
- `GlobalDonatedCUCache` prevents a `SUM(total_donated_cu)` full table scan on every request
  by caching with 30-second refresh. Redis-backed for cross-replica consistency.
- Per-user mutex (`userMutexMap`) serializes concurrent actions for the same user to prevent
  race conditions on game state.
- Bitcoin price leader election via Redis prevents duplicate price advancement across replicas.

### 1.5 Code Quality

- No linter configured (no golangci-lint, no eslint).
- No code formatting enforcement (no gofmt check in CI, no prettier).
- Significant code duplication: the idle progress + colo rack income + group bonus calculation
  is repeated in `GetState`, `PerformAction`, `HandleWSAction`, `runFullTick`, and
  `runLightTick` (5 copies).
- `engine.go` is 1701 lines with 25+ action handlers in a single file. `game.go` is 1348
  lines combining HTTP handlers, WS handlers, tick system, and response building.
- Test coverage is sparse: 6 test files totaling 2873 lines covering rate limiting, auth,
  WS integration, bitcoin pricing, and CU sinks. No tests for: idle progress calculation,
  prestige mechanics, tier upgrades, hardware/service/upgrade purchase validation, bulk
  actions, customer growth, or event system.
- No frontend tests exist at all.

### 1.6 Testing

- Backend tests: `go test ./...` runs in CI. Tests use table-driven patterns (e.g.,
  `cu_sinks_test.go` at 1070 lines is the most thorough).
- No integration tests against a real database (tests use mocks or in-memory constructs).
- No end-to-end tests.
- No load/stress tests in CI (manual stress test results exist in
  `_documents/STRESS-TEST-RESULTS.md`).
- No frontend test framework configured.

---

## 2. High-Risk Areas

These are the files and areas that demand the most careful review. Changes here have the
highest blast radius and are hardest to roll back.

### 2.1 CRITICAL -- Game Engine (`internal/game/engine/engine.go`, 1701 lines)

**Risk: Currency generation, game balance, exploit vectors**

This is the single most important file in the codebase. All game logic lives here:
idle progress calculation, all 25+ player actions, prestige mechanics, currency math,
and tier progression. A bug here directly affects game economy.

**Review checklist for engine changes:**

- [ ] Currency calculations: Are compute units, reputation, and money incremented/decremented
  correctly? Check for integer overflow (the file already has `math.MaxInt64` guards for
  bitcoin, but other currency math uses raw `int64` arithmetic).
- [ ] Multiplier stacking: The income formula chains 7+ multipliers:
  `totalMultiplier = coloMultiplier * idleMultiplier * heatPenalty * eventThrottle * overclockMult`,
  then `knowledgeBoost`, `netMult`, `researchIdleMult`. Verify new multipliers stack correctly
  and don't create infinite loops.
- [ ] Prestige mechanics (`prestige()`, line ~804): Resets game state but preserves
  KnowledgePoints, BitcoinBalance, ColoRacks, and persistent upgrades. Any change to the
  reset list can permanently corrupt player progression.
- [ ] Cost validation: Every purchase action must check (1) tier requirement, (2) sufficient
  currency, (3) resource capacity (power, slots, rack units). Missing any check = free items.
- [ ] `prestigeCostScale()`: Exponential scaling formula. Changes here affect the entire
  late-game economy.
- [ ] Catalog lookups: Actions validate against `catalog/` definitions. New catalog entries
  must have corresponding engine validation.
- [ ] Bulk actions: `bulkUpgradeComponents`, `bulkDeployServices`, `bulkBuyUpgrades`,
  `bulkDeploySaas`, `bulkBuyResearch` run in loops spending currency. Verify they cannot
  overspend or enter infinite loops.

### 2.2 CRITICAL -- Game Handler (`internal/api/handlers/game.go`, 1348 lines)

**Risk: State persistence, tick system correctness, race conditions**

This file orchestrates the tick system, action persistence, WebSocket state push, and
coordinates between the engine and database layers.

**Review checklist for handler changes:**

- [ ] Action persistence order: `PerformAction` and `HandleWSAction` persist results in a
  specific order (hardware, service, upgrade, customer, expense, component, research, bulk
  records, prestige wipe, colo rack, then game state update). Reordering can cause orphaned
  records or constraint violations.
- [ ] Per-user locking: Both REST `PerformAction` and WS `HandleWSAction` acquire
  `userLocks.Lock(userID)`. The WS handler manually manages lock/unlock with panic recovery.
  Verify new code paths release the lock on all exit paths.
- [ ] Tick system: `runFullTick` vs. `runLightTick` caching. The `dirty` flag in
  `tickState` determines which path runs. After any action, `MarkDirty` forces a full tick
  on the next interval. Verify that actions that modify child tables always call `MarkDirty`.
- [ ] Elapsed time calculation: `elapsed := now.Sub(gs.LastTickAt).Seconds()` is captured
  BEFORE `ProcessIdleProgress` updates `LastTickAt`. This ordering is critical -- reversing
  it causes zero-income ticks.
- [ ] Prestige wipe: Deletes hardware, services, customers, expenses, non-persistent upgrades.
  Must happen AFTER the engine returns the prestige result but BEFORE the game state update.
- [ ] WS error handling: `HandleWSAction` swallows DB persistence errors silently (no error
  returns). This is intentional (fire-and-forget for WS) but means persistence failures are
  invisible to the client.

### 2.3 HIGH -- Batch Data Loader (`internal/database/queries/batch.go`, 454 lines)

**Risk: Data corruption, query/scan mismatch, performance regression**

Every game state read flows through `LoadFullGameState` or `LoadFullGameStateForUpdate`.
A column mismatch between the SQL SELECT and the `rows.Scan()` call causes silent data
corruption or panics.

**Review checklist:**

- [ ] Column order: SQL SELECT column order MUST match `rows.Scan()` argument order. The
  batch queries 8 child tables with a fixed read order. Adding a column to any table requires
  updating both the SQL and the scan call.
- [ ] `LoadFullGameStateForUpdate`: Uses `SELECT ... FOR UPDATE` to acquire a row-level lock.
  Verify that transactions using this function commit or rollback promptly to avoid lock
  contention.
- [ ] The `gsColumns` and `gsFields` helpers (defined elsewhere in the queries package)
  must stay in sync with the `game_states` table schema.

### 2.4 HIGH -- Auth Flow (`internal/auth/`, `internal/api/middleware/auth.go`, `handlers/auth.go`)

**Risk: Authentication bypass, token leakage, account takeover**

**Review checklist:**

- [ ] JWT: HS256 with shared secret, 24-hour expiry, no refresh token mechanism. The secret
  comes from environment config. Verify it is never logged or exposed in error messages.
- [ ] Password hashing: bcrypt with `DefaultCost` (10). Acceptable but consider increasing
  cost factor over time.
- [ ] Registration: Email validation, password length (8-128), display name (2-20, alphanumeric,
  profanity filter). Registration can be disabled via config flag.
- [ ] Token in WebSocket: JWT passed as `?token=` query parameter for WS connections. Query
  params appear in server logs and browser history. This is a known tradeoff for WS auth but
  should not be worsened.
- [ ] No CSRF protection (API is JWT-only, no cookies).
- [ ] No account lockout after failed login attempts (rate limiting at 10/min/IP is the only
  protection).

### 2.5 HIGH -- WebSocket Protocol (`internal/api/ws/hub.go`, 319 lines)

**Risk: Connection leaks, goroutine leaks, denial of service**

**Review checklist:**

- [ ] Single connection per user enforced: `HandleConnect` closes the old connection when a
  new one arrives for the same user.
- [ ] `done` channel lifecycle: Closed by `readPump` cleanup or by `HandleConnect` when
  replacing. Double-close guarded with `select` on `<-done`.
- [ ] `trySend` uses `recover()` to handle panic from send-on-closed-channel race.
- [ ] Send buffer: 16 slots, non-blocking send. Full buffer = message drop (logged).
- [ ] Read limit: 64KB max incoming message size.
- [ ] Origin validation: Hardcoded allowlist plus env-based extras.

### 2.6 HIGH -- Catalog Definitions (`internal/game/catalog/`, 556 lines total)

**Risk: Game balance, economy exploits**

The catalog files define all purchasable items, their costs, and their effects. Changes here
directly affect game balance.

**Review checklist:**

- [ ] Cost values: Are they consistent with the progression curve? Cross-reference with
  `_documents/GAME-MECHANICS.md`.
- [ ] New items: Must have corresponding handling in `engine.go` for purchase, effect
  application, and prestige persistence decisions.
- [ ] Tier requirements: `MinTier` field gates when items become available. Verify the tier
  matches the intended game progression stage.
- [ ] Multiplier values: Automation multipliers, research bonuses, and hardware bonuses all
  stack. Verify new values don't create exploitable combinations.

### 2.7 HIGH -- Database Migrations (`internal/database/migrations/`)

**Risk: Data loss, irreversible schema changes**

14 migration files to date. Migrations are applied manually via `psql`.

**Review checklist:**

- [ ] Backward compatibility: Can the old code still function after the migration runs? There
  is no blue-green deployment -- migration and code deploy happen on the same machine.
- [ ] Rollback plan: Is the migration reversible? Document the rollback SQL.
- [ ] Data preservation: Does the migration drop or alter columns with existing data?
- [ ] Index impact: New indexes on large tables can lock the table during creation. Use
  `CREATE INDEX CONCURRENTLY` where possible.
- [ ] Permission grants: New tables need `GRANT ALL ON <table> TO homelab_game;`.
- [ ] Migration naming: Sequential numbering (NNN_description.sql). No gaps.

### 2.8 MEDIUM -- Frontend Components (`apps/desktop/src/components/`, 16 files)

**Risk: UI inconsistency, stale state display, misleading player information**

The frontend is a React + TypeScript + Tailwind app using Zustand for state management.
No frontend tests exist.

**Review checklist:**

- [ ] State synchronization: Verify WebSocket state updates are correctly mapped to Zustand
  store fields. Stale UI after a purchase is a common bug.
- [ ] Action payloads: Must match the exact JSON schema expected by the backend's
  `ProcessAction` switch cases.
- [ ] Currency display: Ensure numbers are formatted consistently and never show negative
  values to the player.
- [ ] Error handling: Actions can fail (rate limit, insufficient funds, capacity). Verify
  error states are shown to the player.

---

## 3. CI Quality Gates -- Current State

The project has a single GitHub Actions workflow (`.github/workflows/build.yml`) with
two jobs:

### 3.1 Backend Job

| Step | What it does | Gate? |
|---|---|---|
| Setup Go | Installs Go from `go.mod` version | No |
| `go test ./...` | Runs all backend tests | **Yes -- blocks merge** |
| Docker build & push | Builds backend + migration images, pushes to GHCR | Push to main only |

### 3.2 Frontend Job

| Step | What it does | Gate? |
|---|---|---|
| Setup pnpm + Node 22 | Installs dependencies | No |
| `pnpm install --frozen-lockfile` | Deterministic install | No |
| `pnpm typecheck` (shared) | TypeScript type checking on `packages/shared/` | **Yes -- blocks merge** |
| Docker build & push | Builds frontend image, pushes to GHCR | Push to main only |

### 3.3 What Exists

- **Go test suite**: 6 test files, 2873 lines. Covers rate limiting, auth handler basics,
  WS action integration, bitcoin price simulation, and CU sink balance verification.
- **TypeScript typecheck**: Only on the shared package. Not on the desktop app itself.
- **Docker build**: Catches compilation errors and missing dependencies.

### 3.4 What is MISSING -- Gaps in Automated Enforcement

| Gap | Risk | Priority |
|---|---|---|
| **No Go linter** (golangci-lint) | Style drift, subtle bugs (errcheck, unused vars) | High |
| **No Go coverage gate** | Regressions in untested critical paths | High |
| **No frontend lint** (eslint) | Inconsistent code, accessibility issues | Medium |
| **No frontend build** in CI | Broken builds not caught until deploy | High |
| **No frontend tests** | UI regressions undetected | Medium |
| **No desktop app typecheck** in CI | Type errors in `apps/desktop/` pass CI | High |
| **No PR template** | Inconsistent PR descriptions, missing context | Medium |
| **No CONTRIBUTING.md** | Onboarding friction, inconsistent practices | Medium |
| **No branch protection rules** documented | Direct pushes to main possible | High |
| **No security scanning** (gosec, Snyk, Dependabot) | Vulnerable dependencies, code flaws | High |
| **No database migration validation** | Schema drift, broken migrations | Medium |
| **No integration tests** | DB query correctness assumed, not verified | High |

---

## 4. Contribution Workflow

There is no `CONTRIBUTING.md` or PR template. This section documents the workflow as it
should be practiced.

### 4.1 Branch Strategy

- `main` is the production branch. All code on main runs directly on the production server.
- Feature branches should be created from `main` and merged back via pull request.
- Branch naming: `feature/description`, `fix/description`, `chore/description`.

### 4.2 Pull Request Expectations

Until a formal PR template is created, PRs should include:

1. **Summary**: What changed and why (1-3 sentences).
2. **Risk assessment**: Which high-risk areas (section 2) does this touch?
3. **Testing**: What was tested? How? Include test output for game engine changes.
4. **Migration**: Does this require a database migration? If so, include rollback SQL.
5. **Catalog changes**: If game balance numbers changed, explain the rationale.

### 4.3 Review Requirements

| Change Type | Required Review Depth | Recommended Reviewers |
|---|---|---|
| Game engine actions (engine.go) | Full 6-dimension review | Staff + Senior engineer |
| Currency/multiplier math | Full review + manual testing | Staff engineer |
| Auth/security changes | Full review, security focus | Staff + Security engineer |
| Database migrations | Full review + rollback plan | Staff + Data engineer |
| WebSocket protocol | Full review, concurrency focus | Staff + Senior engineer |
| Catalog/balance changes | Game design review | Staff engineer + Game designer |
| Frontend UI changes | Architecture + UX review | Senior engineer + UX designer |
| Documentation/config | Quick sanity check | Any engineer |

### 4.4 Pre-Merge Checklist

- [ ] `go test ./...` passes locally
- [ ] `pnpm typecheck` passes in `packages/shared/`
- [ ] No new `TODO` or `FIXME` without a linked issue
- [ ] Error paths return appropriate HTTP status codes
- [ ] New endpoints are added to `routes/routes.go` with appropriate middleware (auth, rate limit)
- [ ] New DB tables have `GRANT` statements documented
- [ ] Catalog changes have corresponding engine validation
- [ ] WebSocket message format changes are documented and backward-compatible

---

## 5. File Risk Map

Quick reference for reviewers. Files sorted by review priority.

| File | Lines | Risk | Why |
|---|---|---|---|
| `internal/game/engine/engine.go` | 1701 | CRITICAL | All game logic, currency math, 25+ actions |
| `internal/api/handlers/game.go` | 1348 | CRITICAL | Tick system, action persistence, WS handler, race conditions |
| `internal/database/queries/batch.go` | 454 | HIGH | Every state read flows through here; column/scan sync |
| `internal/game/events/events.go` | 369 | HIGH | Random event probabilities, throttle effects |
| `internal/api/ws/hub.go` | 319 | HIGH | WebSocket lifecycle, goroutine management, origin validation |
| `internal/game/engine/config.go` | 311 | HIGH | Game constants, overclock tiers, network/storage bonuses |
| `internal/api/handlers/social.go` | 277 | MEDIUM | Group management, leaderboards, authorization checks |
| `internal/game/bitcoin/price.go` | 219 | MEDIUM | Price simulation, leader election |
| `cmd/server/main.go` | 207 | MEDIUM | Wiring, initialization order, graceful shutdown |
| `internal/game/catalog/research.go` | 182 | MEDIUM | Research tree definitions, balance |
| `internal/api/handlers/auth.go` | 170 | HIGH | Registration, login, input validation, profanity filter |
| `internal/database/queries/groups.go` | 164 | MEDIUM | Group SQL queries, membership management |
| `internal/database/queries/leaderboard.go` | 139 | LOW | Read-heavy leaderboard queries |
| `internal/models/game_state.go` | 137 | HIGH | Data model -- changes here cascade to batch.go, engine.go, game.go |
| `internal/api/middleware/ratelimit.go` | 132 | MEDIUM | Rate limiting implementation |
| `internal/game/catalog/upgrades.go` | 127 | MEDIUM | Upgrade definitions, automation multipliers |
| `internal/api/handlers/cu_cache.go` | 115 | MEDIUM | Global CU cache, Redis interaction |
| `internal/game/catalog/hardware.go` | 92 | MEDIUM | Hardware catalog, cost/stat definitions |
| `internal/game/catalog/saas.go` | 82 | MEDIUM | SaaS service definitions |
| `internal/game/catalog/services.go` | 73 | MEDIUM | Service definitions |
| `internal/api/routes/routes.go` | 56 | MEDIUM | Route wiring, middleware application |
| `internal/auth/jwt.go` | 45 | HIGH | Token generation/validation, signing method |
| `internal/api/middleware/auth.go` | 48 | HIGH | Auth middleware, context key |
| `internal/auth/password.go` | 13 | LOW | bcrypt wrapper |
| `database/migrations/*.sql` | varies | HIGH | Schema changes, irreversible in production |

---

## 6. Known Technical Debt Affecting Reviews

These are patterns that make reviews harder and should be addressed over time:

1. **Code duplication in game.go**: The idle progress + colo income + group bonus
   calculation appears in 5 places. Changes to the calculation must be applied in all 5
   or the game produces inconsistent results depending on the code path (REST vs. WS vs.
   full tick vs. light tick vs. GET state).

2. **engine.go monolith**: 1701 lines with 25+ action handlers. Reviewers must read
   the entire file to understand side effects. Splitting by action category (purchases,
   bulk, prestige, bitcoin) would reduce review burden.

3. **No test coverage for core game logic**: The engine's `ProcessIdleProgress` and most
   `ProcessAction` cases have no unit tests. Reviews of engine changes rely entirely on
   manual verification, which is slow and error-prone.

4. **Manual mutex management in HandleWSAction**: The WS action handler manually tracks
   lock state with a `locked` boolean for panic recovery, rather than using `defer`. This
   pattern is fragile and easy to break during refactoring.

5. **Silent error swallowing in WS persistence**: `HandleWSAction` does not check errors
   from DB persistence calls (`h.hardware.Create`, `h.services.Create`, etc.). A failed
   persist leaves the in-memory state updated but the database stale.

6. **No schema validation on action payloads**: Raw `json.RawMessage` payloads are
   unmarshalled inside each action handler. No centralized schema validation, no max-length
   checks on string fields beyond what the engine methods enforce.

---

## 7. Review Anti-Patterns to Avoid

1. **Do not approve engine changes without verifying currency math.** Even "simple" changes
   to the multiplier chain can create exponential income exploits.

2. **Do not approve catalog changes without checking engine handling.** A new catalog entry
   without corresponding engine validation = free items for players.

3. **Do not approve migration changes without a rollback plan.** There is no staging
   environment. Migrations run directly against the production database on this machine.

4. **Do not approve changes to batch.go without verifying column/scan alignment.** A
   mismatch between SQL SELECT and `rows.Scan()` causes silent data corruption that may
   not be caught until players report incorrect state.

5. **Do not approve WebSocket changes without reviewing goroutine lifecycle.** The
   `done` channel, `send` channel close, and `readPump`/`writePump` exit sequence must
   remain correct or connections leak.
