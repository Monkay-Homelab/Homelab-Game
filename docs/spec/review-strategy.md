---
project: "project"
maturity: "draft"
last_updated: "2026-03-20"
updated_by: "@staff-engineer"
scope: "Code review strategy, risk tiers, dimension weighting, and quality gate definitions for the Homelab the Game monorepo"
owner: "@staff-engineer"
dependencies:
  - security.md
  - testing.md
  - architecture.md
---

# Review Strategy

## 1. Overview

This document defines the review strategy for the Homelab the Game project -- a server-authoritative multiplayer idle/clicker game with a Go backend, React (Tauri) desktop client, React Native mobile client, and a shared TypeScript package. The project is self-hosted on a single homelab VM with no CI/CD pipeline, no automated test suite, no linter configuration, and no existing PR templates or contribution guidelines.

**Current state of review tooling (as of 2026-03-20):** None. There is no CI pipeline, no GitHub Actions, no pre-commit hooks (only default Git sample hooks), no ESLint/Prettier/golangci-lint configuration, no Dockerfile, and zero test files of any kind (no `*_test.go`, no `*.test.ts`, no `*.spec.ts`). Reviews are therefore entirely manual and judgment-driven. This spec establishes the framework for those reviews and provides a roadmap for incremental automation.

---

## 2. Risk Tiers

Every file and directory in the codebase is classified into a risk tier. The tier determines the minimum review rigor, the number of dimensions that must be evaluated, and whether blocking approval is required before merge.

### Tier 1 -- Critical (Always Block on Findings)

| Area | Files | Rationale |
|------|-------|-----------|
| **Authentication & Authorization** | `internal/auth/jwt.go`, `internal/auth/password.go`, `internal/api/middleware/auth.go` | Token generation, password hashing, and auth middleware are the security perimeter. A bug here exposes every user account. |
| **Game Engine -- State Mutation** | `internal/game/engine/engine.go` (all of `ProcessAction` and its action methods) | Server-authoritative: all game state changes flow through this file. Economy-breaking bugs (infinite currency, prestige exploits, negative overflow) originate here. |
| **Game Engine -- Idle Progress** | `internal/game/engine/engine.go` (`ProcessIdleProgress`) | Calculates offline earnings. Incorrect multiplier stacking, floating-point drift, or time-manipulation vulnerabilities directly corrupt the in-game economy. |
| **Prestige / Colo Logic** | `internal/game/engine/engine.go` (`prestige` method) | Resets game state while preserving persistent data. A bug here can wipe player progress or grant unearned bonuses. This is effectively a data migration that runs in production on player action. |
| **Database Migrations** | `internal/database/migrations/*.sql` | Schema changes on a live PostgreSQL database. Irreversible if applied without a rollback plan. |
| **Rate Limiting & Anti-Cheat** | `internal/api/middleware/ratelimit.go`, per-user mutex in `handlers/game.go` | Protects the server from abuse. The per-user lock in `GameHandler` prevents race conditions on concurrent actions (e.g., double-buy exploits). |

### Tier 2 -- High (Should Block on Non-trivial Findings)

| Area | Files | Rationale |
|------|-------|-----------|
| **Game Handler (Persistence Layer)** | `internal/api/handlers/game.go` | Orchestrates engine calls and database writes. Partial-write failures here can leave game state inconsistent (e.g., hardware created in DB but game state not updated). |
| **Social Handlers (Groups, Leaderboard)** | `internal/api/handlers/social.go` | Authorization checks for group operations (founder/admin role gating, kick/promote). Privilege escalation bugs live here. |
| **WebSocket Hub** | `internal/api/ws/hub.go` | Manages concurrent connections. Goroutine leaks, missing cleanup on disconnect, or missing write serialization can degrade server stability. |
| **Database Queries** | `internal/database/queries/*.go` | Raw SQL (not an ORM). SQL injection risk is low (parameterized queries throughout), but column mismatch bugs between Go struct scanning and SQL column lists are common and cause silent data corruption. |
| **Game Catalog (Balance Data)** | `internal/game/catalog/hardware.go`, `services.go`, `upgrades.go`, `saas.go` | Game balance constants. Incorrect values do not crash the server but destroy gameplay. These should be reviewed with game design intent in mind, not just code correctness. |
| **Frontend State Store** | `apps/desktop/src/stores/gameStore.ts` | Zustand store is the single source of truth on the client. Optimistic updates (e.g., `runJob` applies click reward locally) can desync from server if not carefully matched. |
| **Client-Side Rate Calculation** | `apps/desktop/src/hooks/useIdleTick.ts` | Must exactly mirror the server's `ProcessIdleProgress` multiplier math. A mismatch causes visual desync where the client shows different numbers than the server returns, breaking player trust. |

### Tier 3 -- Medium (Approve with Follow-up for Minor Findings)

| Area | Files | Rationale |
|------|-------|-----------|
| **API Client** | `apps/desktop/src/api.ts` | Type definitions and fetch wrappers. Incorrect types cause runtime errors but are caught quickly by testing. |
| **Config / Environment** | `internal/config/config.go`, `.env` | Server configuration. The fallback to random JWT secret in dev is already logged. Misconfig is an ops issue, not a code correctness issue. |
| **Route Definitions** | `internal/api/routes/routes.go` | Middleware chain ordering. Incorrect ordering (e.g., rate limit before auth) changes security posture but is easy to audit visually. |
| **CORS Configuration** | `internal/api/middleware/cors.go` | Origin allowlist with env override. Review changes for overly permissive origins. |
| **Event System** | `internal/game/events/events.go` | Random event definitions and roll logic. Balance-sensitive but isolated -- events cannot directly mutate persistent state in ways that bypass the engine. |
| **React Components** | `apps/desktop/src/components/*.tsx` | UI display logic. Bugs here are visible and user-reported. Low blast radius. |
| **Shared Types** | `packages/shared/src/**` | TypeScript types and constants. Currently underutilized (the desktop client defines its own types in `api.ts` rather than importing from shared). Drift between shared types and actual API responses is a known gap. |

### Tier 4 -- Low (Quick Sanity Check)

| Area | Files | Rationale |
|------|-------|-----------|
| **Styling** | `apps/desktop/src/styles/global.css` | Visual changes only. |
| **Static Assets** | `apps/desktop/src-tauri/icons/*` | Binary assets. |
| **Build Configuration** | `vite.config.ts`, `tsconfig.json`, `Cargo.toml`, `tauri.conf.json` | Toolchain config. Review for version bumps with known CVEs. |
| **Documentation** | `README.md`, `CLAUDE.md`, `docs/**` | Prose. Review for accuracy if it describes system behavior. |
| **Stress Tests** | `stress-tests/*` | Test tooling, not production code. |

---

## 3. Review Dimensions

Six dimensions are evaluated during every review. The weight given to each dimension varies by risk tier.

### Dimension Definitions

1. **Architecture** -- Does the change fit the existing patterns? Does it introduce new patterns without justification? Does it respect the server-authoritative model? Key question: "Does this change make the system harder to reason about?"

2. **Security** -- Authentication bypass, authorization escalation, input validation, SQL injection, XSS via JSON responses, timing attacks, rate limit bypass, CORS misconfiguration, secret exposure. Key question: "Can a malicious client exploit this?"

3. **Operations** -- Error handling, logging, graceful degradation, database connection handling, goroutine lifecycle, memory leaks (particularly the `userMutexMap` which grows unboundedly), WebSocket cleanup. Key question: "If this fails at 3am, can I diagnose it from logs alone?"

4. **Performance** -- N+1 queries, unnecessary full-table scans (the leaderboard queries scan all `game_states` rows on every request), lock contention (the global `rateLimiter.mu`), large response payloads (full state returned on every action). Key question: "Does this scale to 1000 concurrent players without architectural changes?"

5. **Code Quality** -- Readability, naming, duplication (the `GetState` and `PerformAction` handlers share ~40 lines of identical colo/group/customer-growth logic), error handling consistency, dead code. Key question: "Will a new contributor understand this in 6 months?"

6. **Testing** -- Test coverage (currently zero), testability of the change, whether the change makes future testing harder. Key question: "How would we verify this works without manually clicking through the game?"

### Dimension Weighting by Risk Tier

| Dimension | Tier 1 | Tier 2 | Tier 3 | Tier 4 |
|-----------|--------|--------|--------|--------|
| Architecture | Full | Full | Scan | Skip |
| Security | Full | Full | Scan | Skip |
| Operations | Full | Full | Scan | Skip |
| Performance | Full | Scan | Skip | Skip |
| Code Quality | Full | Full | Scan | Skip |
| Testing | Full | Full | Scan | Skip |

"Full" = thorough evaluation against the dimension's criteria.
"Scan" = quick check for obvious violations.
"Skip" = not evaluated unless something jumps out.

---

## 4. High-Risk Patterns Requiring Extra Scrutiny

These patterns appear in the current codebase and demand heightened attention during review because they have historically caused or will likely cause production issues.

### 4.1 Multiplier Stacking in the Game Engine

The idle progress calculation in `ProcessIdleProgress` applies six multiplicative factors to compute income:

```
totalMultiplier = ColoMultiplier * IdleMultiplier * heatPenalty * eventThrottle
knowledgeBoost = 1.0 + KnowledgePoints / 100.0
netMult = 1.0 + networkBonus
```

Any change that adds a new multiplier, modifies an existing one, or changes the order of application requires:
- Verification that the client-side `useIdleTick.ts` mirrors the change exactly.
- Analysis of the multiplicative interaction at extreme values (e.g., 100 prestige cycles with max knowledge points).
- Confirmation that no combination produces negative income or integer overflow.

### 4.2 Non-Transactional Multi-Table Writes

The `PerformAction` handler persists results across multiple tables (hardware, services, upgrades, component_upgrades, customers, expenses, colo_racks, game_states) in separate `Exec`/`QueryRow` calls with no database transaction wrapping them. A failure partway through (e.g., hardware inserted but game_state update fails) leaves the database in an inconsistent state.

Any change that adds new persistence steps to `PerformAction` should be flagged as a concern until transactional writes are implemented.

### 4.3 Per-User Mutex Without Cleanup

The `userMutexMap` in `handlers/game.go` creates a `sync.Mutex` per user ID and never removes them. Over time with many registrations, this is a memory leak. Any change touching this map should consider adding TTL-based cleanup or using `sync.Map` with periodic eviction.

### 4.4 Duplicated Logic Between GetState and PerformAction

Both `GetState` and `PerformAction` handlers contain nearly identical blocks for:
- Processing idle progress
- Calculating group bonus
- Adding colo rack passive income
- Processing customer growth
- Updating game state and customers

Any change to game state calculation logic must be applied in both handlers, or the game will behave differently on poll vs. action. This is a strong signal that a shared method should be extracted, and reviewers should flag changes that touch only one handler.

### 4.5 Client-Server Calculation Parity

The client (`useIdleTick.ts`) independently calculates income rates for smooth animation between server polls. This calculation must exactly match `ProcessIdleProgress` in the engine. The client currently reads from `GameConfig` for constants, which helps, but structural changes to the formula (new multiplier, changed order of operations) must be verified on both sides.

### 4.6 SQL Column Ordering

The `gsColumns` constant and `gsFields` function in `queries/game_state.go` must have their columns in exactly the same order. Adding a new field to `GameState` requires updating the model, the column list, the field scanner, the `UPDATE` statement, and the migration -- five coordinated changes. Missing any one of these causes silent data corruption or runtime panics.

---

## 5. Review Workflow

### 5.1 Before Reviewing

1. Read the relevant TDD or issue to understand intent.
2. Identify which risk tiers are touched by the change using the tables in Section 2.
3. Determine the maximum risk tier -- this sets the review rigor floor.

### 5.2 During Review

1. **Start with Tier 1 files.** These carry the most risk and should get the most time. If only Tier 3-4 files are touched, the review can be quick.
2. **Check for cross-cutting concerns.** A change to `engine.go` that adds a multiplier implies a required change to `useIdleTick.ts`. If the latter is missing, that is a blocker.
3. **Verify game balance changes against design intent.** Catalog changes (costs, power draw, compute per tick) should reference the game design document or issue that motivated them. Unexplained balance changes are a concern.
4. **Check error handling.** The current codebase silently ignores many database errors (e.g., `hw, _ := h.hardware.GetByGameStateID(...)`). New code should not add more silent error swallowing. Existing silent errors encountered during review should be noted as follow-up concerns, not blockers.

### 5.3 Approval Criteria

- **Approve**: No blockers, no unaddressed concerns. Suggestions noted for follow-up.
- **Approve with follow-up**: Concerns exist but are low-risk and blocking would delay important work. Follow-up issues must be filed.
- **Request changes**: Blockers found (security, data loss, breaking changes, economy exploits).
- **Request split**: Change touches multiple risk tiers (e.g., engine refactor + UI polish in one PR). The high-risk and low-risk portions should be reviewed and merged separately.

---

## 6. Critical Review Checklists

### 6.1 Game Engine Changes (Tier 1)

- [ ] Does the change preserve the server-authoritative model? (No client-side state mutations that bypass server validation.)
- [ ] Are all currency operations guarded against negative values?
- [ ] Do integer arithmetic operations stay within `int64` bounds at extreme game states (100 prestige, max knowledge, all upgrades)?
- [ ] Is the corresponding client-side `useIdleTick.ts` updated to match?
- [ ] Are action payloads validated (unknown fields rejected, required fields checked)?
- [ ] Is the change reflected in the `GameConfig` endpoint if it introduces new constants?
- [ ] Does the change interact with the prestige reset? Are the right fields preserved vs. wiped?

### 6.2 Authentication / Middleware Changes (Tier 1)

- [ ] Is the JWT signing method explicitly checked (no `alg: none` bypass)?
- [ ] Are passwords hashed with bcrypt at default cost or higher?
- [ ] Is user input validated and sanitized before use?
- [ ] Are rate limits applied before expensive operations?
- [ ] Does CORS configuration avoid wildcards (`*`) in production?
- [ ] Is the `X-Forwarded-For` header trusted only behind a known reverse proxy?
- [ ] Are error messages generic enough to avoid information leakage (e.g., "invalid credentials" not "user not found")?

### 6.3 Database Migration Changes (Tier 1)

- [ ] Is the migration idempotent (safe to run twice)?
- [ ] Is there a rollback plan (reverse migration or known-safe state)?
- [ ] Does the migration have a data backfill step if adding NOT NULL columns to existing rows?
- [ ] Is the migration numbered sequentially with no gaps or conflicts?
- [ ] Has the corresponding Go model, query, and column list been updated?

### 6.4 API / Handler Changes (Tier 2)

- [ ] Is the new endpoint added to the route table with appropriate middleware (auth, rate limit)?
- [ ] Is the response structure consistent with existing endpoints?
- [ ] Are error responses JSON-formatted (not plain text)?
- [ ] Does the handler acquire the per-user lock for state-mutating operations?
- [ ] Are all database errors checked (not silently discarded with `_`)?
- [ ] If the change adds persistence, are all writes consistent (or is the lack of transaction acknowledged)?

### 6.5 Frontend Changes (Tier 2-3)

- [ ] Does the change correctly handle loading, error, and empty states?
- [ ] Is the token correctly included in API requests?
- [ ] Does optimistic UI update revert on server error?
- [ ] Are user inputs sanitized before display (XSS prevention)?
- [ ] Does the change respect the server as source of truth (no client-side game state invention)?

---

## 7. Known Gaps and Debt

This section documents quality and safety gaps that exist in the current codebase. These are not aspirational goals -- they are real risks that inform review priorities.

### 7.1 No Automated Tests

There are zero test files in the entire repository. No unit tests for the game engine, no integration tests for the API, no frontend component tests.

**Review implication**: Every change must be mentally simulated for correctness. Complex game logic changes (multiplier math, prestige, bulk actions) should be reviewed with pen-and-paper verification of edge cases. Reviewers should note "this needs a test" as a follow-up concern on any Tier 1-2 change.

### 7.2 No CI/CD Pipeline

There is no GitHub Actions, no pre-commit hooks, no automated linting, no automated build verification. The backend binary (`apps/backend/server`) is committed directly to the repository.

**Review implication**: Reviewers must manually verify that code compiles and types check. TypeScript type errors in the desktop client can only be caught by running `pnpm build` locally.

### 7.3 No Linting or Formatting

No ESLint, Prettier, golangci-lint, or `gofmt` enforcement.

**Review implication**: Do not spend review time on style issues. Focus on correctness and safety. When linting is eventually added, it will catch these systematically.

### 7.4 No Database Transaction Boundaries

Multi-table writes in the game handler are not wrapped in transactions. A crash mid-write can leave the database in an inconsistent state.

**Review implication**: Flag any change that adds new multi-table write sequences. The eventual fix is wrapping `PerformAction` persistence in a transaction, but until then, reviewers should ensure new writes fail safely (idempotent retries, or at least no data corruption on partial failure).

### 7.5 Shared Types Package Drift

The `packages/shared` package defines TypeScript types (`GameState`, `Tier`, `ActionType`) that do not match the actual API response shape used by the desktop client (`apps/desktop/src/api.ts`). The desktop client maintains its own complete type definitions. The shared package is effectively unused.

**Review implication**: Do not assume shared types are authoritative. The desktop `api.ts` types are the ones that must match the Go response structs.

### 7.6 Silent Error Handling

Many database query results in `handlers/game.go` discard errors with `_` (e.g., `hw, _ := h.hardware.GetByGameStateID(...)`). If a query fails, the handler proceeds with nil/empty data, which may cause incorrect game state calculations without any error signal.

**Review implication**: New code should not add silent error discards. Existing ones encountered during review should be noted for follow-up.

### 7.7 Unbounded In-Memory Structures

- `userMutexMap`: grows with every new user, never shrinks.
- `rateLimiter.visitors`: cleaned up on a 1-minute TTL, but under sustained load the map can grow large between cleanup cycles.
- `ws.Hub.clients`: cleaned up on disconnect, but stale connections that never cleanly disconnect linger.

**Review implication**: Any change adding a new in-memory map or cache should include a cleanup/eviction strategy.

### 7.8 Committed Binary and Secrets

The compiled Go binary (`apps/backend/server`, 14.8MB) is committed to the repository. The `.env` file (containing database credentials and JWT secret) is also committed. The `.env` is listed in the backend directory, not in `.gitignore`.

**Review implication**: Verify that no new secrets or binaries are added. Flag any commit containing credentials, API keys, or compiled artifacts.

---

## 8. Recommended Quality Gate Roadmap

These are not current-state descriptions -- they are prioritized recommendations for incrementally adding automated quality gates. Each gate is ordered by risk-reduction-per-effort.

### Phase 1 -- Immediate (Manual)

- Adopt this review strategy document for all code changes.
- Add `.gitignore` entries for `apps/backend/server` (compiled binary) and `apps/backend/.env`.
- Move secrets out of the committed `.env` into environment variables or a secrets manager.

### Phase 2 -- Low Effort Automation

- Add `gofmt` check as a pre-commit hook or CI step. Zero configuration needed.
- Add `go vet` as a CI step. Catches common Go mistakes (printf format strings, unreachable code, struct copying).
- Add `pnpm build` (TypeScript type-check) as a CI step for the desktop client.
- Add a GitHub Actions workflow that runs `go build ./cmd/server/` on push.

### Phase 3 -- Testing Foundation

- Add unit tests for `internal/game/engine/engine.go`, specifically:
  - `ProcessIdleProgress` with known inputs and expected outputs.
  - Each action method with edge cases (insufficient funds, max level, prestige boundary).
  - Multiplier stacking at extreme values.
- Add integration tests for auth endpoints (register, login, invalid credentials).
- These tests become mandatory CI gates for Tier 1 changes.

### Phase 4 -- Full CI Pipeline

- `golangci-lint` with a curated configuration (not default-everything; focus on security, bugs, and correctness linters).
- ESLint + Prettier for TypeScript code.
- Database migration validation (dry-run against a test database).
- Automated stress testing on staging (the `stress-tests/` tool already exists).

---

## 9. Frequently Changed Files (Change Hotspots)

Based on git history analysis, these files change most frequently and should receive proportionally more review attention:

| File | Changes | Risk Tier | Note |
|------|---------|-----------|------|
| `internal/game/engine/engine.go` | 4 | Tier 1 | Largest file (1315 lines), highest risk. Every game mechanic change touches this. |
| `internal/api/handlers/game.go` | 4 | Tier 2 | Handler orchestration. Growing in complexity with each new feature. |
| `internal/api/routes/routes.go` | 4 | Tier 3 | Every new endpoint requires a route change. |
| `apps/desktop/src/api.ts` | 3 | Tier 3 | Type definitions grow with each new feature. |
| `apps/desktop/src/App.tsx` | 3 | Tier 3 | Root component, structural changes. |
| `internal/database/queries/game_state.go` | 3 | Tier 2 | Column list must stay synchronized with model. |

---

## 10. Cross-Component Review Triggers

Certain changes in one component require mandatory review of related components. If the triggering change is submitted without the dependent change, the review should block.

| Trigger | Required Co-change | Reason |
|---------|--------------------|--------|
| New field on `GameState` model | Migration SQL + `gsColumns` + `gsFields` + `UPDATE` statement + frontend `GameState` type | Five-point synchronization. Missing any one causes data corruption or runtime errors. |
| New multiplier in `ProcessIdleProgress` | `useIdleTick.ts` rate calculation | Client-server parity. |
| New game action in `ProcessAction` | Route registration + frontend store method + UI trigger | Dead code without all three. |
| New hardware/service/upgrade in catalog | Frontend display component (may need new UI) + game balance review | Balance impact. |
| CORS origin change | WebSocket `upgrader.CheckOrigin` | Two separate origin allowlists that must stay synchronized. |
| New middleware in route chain | Ordering verification against existing chain | Middleware order determines security posture. Current order: CORS -> JSON -> MaxBodySize -> (per-route: Auth -> RateLimit -> Handler). |
| Database schema change | Corresponding Go model + queries + handler + migration + rollback plan | Full-stack coordination. |
