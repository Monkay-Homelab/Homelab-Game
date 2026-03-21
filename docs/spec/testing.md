---
project: "project"
maturity: "proof-of-concept"
last_updated: "2026-03-20"
updated_by: "@staff-engineer"
scope: "Documents the current state of testing infrastructure, coverage, and gaps across the Homelab the Game monorepo"
owner: "@staff-engineer"
dependencies: []
---

# Testing Specification

## 1. Executive Summary

The project has **no automated test suite**. There are zero unit tests, zero integration tests,
zero end-to-end tests, and zero test configuration files across all three application layers
(Go backend, React desktop client, shared TypeScript package). There is no CI/CD pipeline
configured. There is no test runner, no coverage tooling, and no mocking infrastructure.

The only testing artifact is a custom **stress test tool** (`stress-tests/`) that performs
black-box load testing against the live backend API. This tool is manually invoked and produces
throughput/latency metrics but does not validate correctness.

This spec documents exactly what exists today and identifies the highest-risk gaps.

## 2. Current State by Component

### 2.1 Backend (Go) -- `apps/backend/`

**Test files:** None. Zero `*_test.go` files exist anywhere in the backend.

**Test runner:** Go's built-in `go test` is available via the Go toolchain (Go 1.25.0), but
there are no tests to run. `go test ./...` would report zero test files found.

**Test configuration:** No `testdata/` directories, no test fixtures, no test helpers, no test
database configuration, no `docker-compose.test.yml` or similar.

**Mocking infrastructure:** None. All database query types (`UserQueries`, `GameStateQueries`,
`HardwareQueries`, etc.) accept a concrete `*pgxpool.Pool` rather than interfaces, making them
difficult to mock without refactoring. The `Engine` struct has no interface either. Handlers
take concrete query struct pointers, not interfaces.

**Coverage tooling:** Not configured. Go's built-in `-cover` flag is available but has never
been used (no coverage reports, no `.coverprofile` files).

**What would need testing (by risk):**

| Component | Risk Level | Why |
|---|---|---|
| `internal/game/engine/engine.go` (1,000+ lines) | **Critical** | Core game loop: idle progress, resource calculations, tier progression, prestige, events. Pure computation with complex branching -- highly testable and highest business impact. |
| `internal/game/events/events.go` | **High** | Random event engine with tier-weighted selection, mitigation logic, effect application. Correctness directly affects game balance. |
| `internal/api/handlers/game.go` (500+ lines) | **High** | All game action processing: buy hardware, deploy services, upgrade tiers, colo/prestige, donate CU. Server-authoritative validation happens here. |
| `internal/api/handlers/auth.go` | **High** | Registration validation (display name filtering, email validation, profanity filter), login flow. Security boundary. |
| `internal/auth/jwt.go`, `password.go` | **High** | JWT generation/validation and bcrypt password hashing. Security primitives. |
| `internal/api/middleware/ratelimit.go` | **Medium** | Rate limiting logic (IP-based and user-based). Correctness matters for abuse prevention. |
| `internal/game/catalog/*.go` | **Medium** | Static game data (hardware templates, service templates, upgrade templates, SaaS templates). Could use snapshot/golden-file tests. |
| `internal/database/queries/*.go` | **Medium** | Database queries are straightforward CRUD but cover 10 query files with complex column mappings. Integration tests against a real database would catch schema drift. |
| `internal/api/middleware/cors.go`, `bodylimit.go`, `json.go` | **Low** | Simple middleware wrappers. Low complexity. |
| `internal/api/ws/hub.go` | **Medium** | WebSocket hub managing player connections, broadcast, and authentication. Concurrency-sensitive. |

**Testability assessment:** The game engine (`Engine.ProcessIdleProgress`) is the most
testable component -- it is a pure function of game state, hardware, services, upgrades,
expenses, customers, component upgrades, and time. It returns events and mutates the
`GameState` struct in place. This could be tested today with no infrastructure changes.
The handlers and query layers would require interface extraction or a test database to test
effectively.

### 2.2 Desktop Client (React/TypeScript) -- `apps/desktop/`

**Test files:** None. Zero `.test.ts`, `.test.tsx`, `.spec.ts`, or `.spec.tsx` files.

**Test runner:** No test runner is configured. The `package.json` has no `test` script.
No `jest.config.*`, `vitest.config.*`, or any test framework is listed in dependencies
or devDependencies. The project uses Vite for bundling, which makes Vitest the natural
choice, but it has not been added.

**Testing libraries:** None installed. No `@testing-library/react`, no `jest`, no `vitest`,
no `msw` (mock service worker), no assertion libraries beyond what would come with a
test framework.

**Coverage tooling:** Not configured.

**Component inventory (what would need testing):**

- `src/stores/gameStore.ts` -- Zustand store managing all client-side game state, API calls,
  and state sync. Central to client correctness.
- `src/api.ts` -- API client layer with all fetch calls to the backend. Would benefit from
  integration tests with `msw` or similar.
- `src/hooks/useIdleTick.ts` -- Client-side idle tick simulation for responsive UI between
  server syncs. Contains computation logic mirroring the backend engine.
- `src/hooks/useConfig.ts` -- Config fetching hook.
- `src/hooks/useWebSocket.ts` -- WebSocket connection management.
- `src/components/*.tsx` (12 components) -- UI components. Render testing would catch
  regressions but is lower priority than logic testing.

**Testability assessment:** The Zustand store and API client are the highest-value test
targets on the frontend. The `useIdleTick` hook contains game logic that should ideally
match the backend engine -- a strong candidate for cross-layer validation tests.

### 2.3 Shared Package (TypeScript) -- `packages/shared/`

**Test files:** None.

**Test runner:** No test runner configured. The only script is `typecheck` (`tsc --noEmit`).

**What exists:** Type definitions (`src/types/game.ts`) and constants
(`src/constants/index.ts`). These are small files (50 lines of types, 26 lines of constants)
that primarily provide TypeScript type safety. Type checking via `tsc --noEmit` is the
implicit "test" here.

**Testability assessment:** Low testing need. The `typecheck` script provides compile-time
validation. Runtime tests would only be valuable if computation logic is added to this
package (currently it is pure types and constants).

### 2.4 Mobile Client (React Native) -- `apps/mobile/`

**Test files:** None.

**Project state:** The mobile app is a scaffold only. All source directories (`components/`,
`hooks/`, `navigation/`, `screens/`) are empty. There is no `package.json`, no dependencies
installed, and no runnable code. Testing is not applicable at this stage.

### 2.5 Tauri Shell (Rust) -- `apps/desktop/src-tauri/`

**Test files:** None. The Rust layer is minimal (~20 lines across `lib.rs` and `main.rs`) --
standard Tauri boilerplate with no custom commands or logic to test.

## 3. Stress Testing Infrastructure

The only testing artifact in the project is a standalone load testing tool.

**Location:** `stress-tests/`

**Technology:** Go binary (separate `go.mod`, Go 1.24.4) using `gorilla/websocket`.

**What it does:**
1. Registers N players with unique credentials (concurrent, 50 at a time)
2. Fetches initial game state for all players (warm-up)
3. Optionally opens WebSocket connections for all players
4. Runs a timed stress test where each player performs actions at a configurable rate
   (80% game actions like `run_job`, `buy_hardware`, `deploy_service`, `buy_upgrade`;
   20% state fetches)

**Metrics collected:**
- Throughput: requests/sec, actions/sec
- Latency: P50, P90, P95, P99, max (microsecond precision)
- Error rates, rate-limited count
- Live stats every 5 seconds during test

**Configuration flags:**
- `-url` (default `http://localhost:8080`)
- `-players` (default 100)
- `-duration` (default 60s)
- `-rampup` (default 5s)
- `-rate` (default 500ms between actions per player)
- `-ws` (enable WebSocket connections)
- `-verbose` (print individual request results)

**What it does NOT do:**
- Does not validate response correctness (only checks HTTP status codes)
- Does not verify game state consistency (e.g., currency calculations, progression logic)
- Does not run automatically (no CI integration)
- Does not compare results against baselines or fail on regressions
- Does not clean up test data after runs

**Recorded results:** Documented in `docs/STRESS-TEST-RESULTS.md` from 2026-03-19.
Key finding: the backend handles 1,000 concurrent players at sub-15ms P99 latency with
zero errors. Throughput ceiling is approximately 4,500 actions/sec, with latency degrading
significantly above 2,500 concurrent players (P50 jumps to 431ms).

## 4. CI/CD Pipeline

**Status:** Does not exist.

There is no `.github/workflows/` directory, no Jenkinsfile, no GitLab CI config, no
Makefile with test targets, and no build automation of any kind. The roadmap
(`docs/ROADMAP.md`, Phase 13) lists "CI/CD pipeline setup" as a future task.

All builds and deployments are currently manual:
- Backend: `go build ./cmd/server/` then run the binary
- Desktop: `pnpm build` for production, `pnpm dev` for development
- Stress tests: `go build -o stresstest .` then run manually

## 5. Test Pyramid Analysis

| Level | Count | Tools | Notes |
|---|---|---|---|
| **Unit tests** | 0 | None | No tests at any layer |
| **Integration tests** | 0 | None | No test database, no API integration tests |
| **E2E tests** | 0 | None | No Cypress, Playwright, or equivalent |
| **Load/stress tests** | 1 tool | Custom Go binary | Manual execution only, no correctness checks |
| **Type checking** | 1 script | `tsc --noEmit` | `packages/shared` only |
| **Static analysis** | 0 | None | No linters configured (no `golangci-lint`, no `eslint`) |

The test pyramid is empty. There is no automated quality gate of any kind.

## 6. Identified Risks

### 6.1 Critical

- **Game engine correctness is unverified.** The engine (`engine.go`, ~1,000 lines) performs
  all resource calculations, tier progression, prestige math, event processing, and idle
  income computation. Any bug here directly affects game balance and player experience. There
  is no test coverage.

- **No regression safety net.** Any code change anywhere in the project could introduce
  regressions that would only be caught by manual play-testing. The recent commit history
  shows active development with game mechanics overhauls, percentage upgrade changes, and
  datacenter fixes -- all landing without automated verification.

- **Server-authoritative game with no server-side validation tests.** The game's anti-cheat
  model depends entirely on server-side validation (action handlers in `game.go`). Without
  tests, it is unknown whether edge cases like negative currency, impossible tier jumps,
  or concurrent action races are properly handled.

### 6.2 High

- **Auth/security boundary is untested.** JWT generation/validation, password hashing,
  profanity filtering, input validation, and rate limiting have no test coverage. These are
  the most security-sensitive components.

- **Database schema drift risk.** With 9 migration files and 10 query files containing
  hand-written SQL column mappings, there is no automated way to detect if the Go struct
  fields, SQL column lists, and scan targets fall out of sync.

- **Client-server state divergence.** The desktop client contains a `useIdleTick` hook
  that simulates the backend's idle progress calculation locally for responsive UI. If
  backend formulas change, the client simulation could diverge, causing visual glitches
  (displayed values jumping on server sync). No cross-layer tests exist to catch this.

### 6.3 Medium

- **Stress test results are point-in-time only.** The load test was run once
  (2026-03-19) and results were manually recorded. There is no way to detect
  performance regressions automatically.

- **No static analysis.** No Go linting (`golangci-lint`, `go vet`), no TypeScript
  linting (`eslint`), and no formatting enforcement (`gofmt`, `prettier`). Code quality
  depends entirely on author discipline and manual review.

## 7. Testability Barriers

The following architectural patterns would need to change to enable comprehensive testing:

1. **Concrete dependencies in handlers.** All handler structs (`AuthHandler`,
   `GameHandler`, `SocialHandler`) accept concrete query struct pointers
   (e.g., `*queries.GameStateQueries`) rather than interfaces. This means unit testing
   handlers requires either:
   - Extracting interfaces for each query type (preferred Go pattern)
   - Standing up a real PostgreSQL database for every test run
   - Using a library like `pgxmock` to mock the connection pool

2. **Global mutable state in rate limiter.** The rate limiter uses a package-level
   `var limiter` with a goroutine started in `init()`. This makes it difficult to test
   rate limiting behavior in isolation without process-level side effects.

3. **No test database configuration.** There is no mechanism to spin up a test database,
   apply migrations, seed data, and tear down. The backend connects to a single PostgreSQL
   instance configured via environment variables.

4. **Engine mutates state in place.** `ProcessIdleProgress` mutates the passed
   `*models.GameState` struct. While this is fine for the production code path, tests would
   need to carefully construct and compare state structs. Helper builders or fixtures would
   improve test ergonomics.

## 8. Recommendations (Prioritized)

This section documents what the codebase would benefit from, ordered by risk reduction per
effort. These are observations, not commitments -- implementation decisions belong to the
team.

### P0 -- Highest Impact

1. **Unit tests for `internal/game/engine/`**: The engine is pure computation with no I/O
   dependencies. Tests can be written today with zero refactoring. Cover: idle progress
   calculation, tier progression, prestige/colo multipliers, event triggering, heat/cooling
   mechanics, customer satisfaction decay, and edge cases (zero elapsed time, negative
   values, overflow).

2. **Unit tests for `internal/auth/`**: JWT generation, validation (expired tokens, wrong
   secret, malformed tokens), and password hashing/checking. Small surface area, high
   security value, no infrastructure needed.

### P1 -- High Impact

3. **Interface extraction for query types**: Define interfaces for each query struct (e.g.,
   `GameStateStore`, `HardwareStore`) to enable handler-level unit testing with mocks.

4. **Handler unit tests with mocked dependencies**: After interface extraction, test action
   validation logic, error responses, and edge cases without a database.

5. **CI pipeline with `go test` and `tsc --noEmit`**: Even running zero tests in CI
   establishes the infrastructure for future tests to be added. Adding `go vet` and
   `go build ./...` catches compilation errors on every push.

### P2 -- Medium Impact

6. **Test database setup**: Docker-based PostgreSQL for integration tests, with migration
   application and cleanup between test runs. Enables query-layer testing.

7. **Vitest for desktop client**: Add Vitest (natural pairing with Vite), `@testing-library/react`,
   and `msw`. Priority targets: `gameStore.ts`, `api.ts`, `useIdleTick.ts`.

8. **Cross-layer formula validation**: Automated comparison between the Go engine's
   calculations and the TypeScript `useIdleTick` hook to catch divergence.

### P3 -- Lower Impact

9. **Static analysis**: `golangci-lint` for Go, `eslint` + `prettier` for TypeScript.
10. **Stress test automation**: Run stress tests in CI on a schedule, compare against
    baseline metrics, alert on regressions.
11. **Snapshot tests for game catalog data**: Golden-file tests for hardware, service,
    upgrade, and SaaS templates to catch unintended balance changes.
