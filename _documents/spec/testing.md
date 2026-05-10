---
project: "homelab-the-game"
maturity: "experimental"
last_updated: "2026-04-04"
updated_by: "@staff-engineer"
scope: "Testing infrastructure, coverage, patterns, and gaps across all components"
owner: "@staff-engineer"
dependencies: []
---

# Testing Specification

## 1. Overview

This document describes the current state of testing infrastructure in the Homelab the Game
project. It covers what exists, what patterns are in use, what tooling is configured, and --
critically -- what is missing. The maturity is rated **experimental**: the backend has a
meaningful test suite for recent features, but large portions of the codebase (including the
entire frontend and most of the original backend) have zero test coverage, and there is no
coverage measurement tooling in place.

### Test Inventory Summary

| Component | Test Files | Test Functions | Lines of Test Code | Lines of Source |
|---|---|---|---|---|
| Backend (`apps/backend/`) | 6 | 120 | 2,873 | 7,785 |
| Desktop (`apps/desktop/`) | 0 | 0 | 0 | 1,271 |
| Shared (`packages/shared/`) | 0 | 0 | 0 | ~100 |
| Mobile (`apps/mobile/`) | N/A (not yet created) | N/A | N/A | N/A |

---

## 2. Backend Testing (Go)

### 2.1 Test Runner and Framework

- **Runner**: Go's built-in `go test ./...`
- **Assertion style**: Standard library `testing` package only. No third-party assertion
  libraries (testify, go-cmp, etc.) are used in test code, despite `testify` appearing as a
  transitive dependency in `go.sum`.
- **Mocking framework**: None. Test doubles are hand-written (see Section 2.3).
- **Coverage tool**: None configured. `go test -cover` is not used in CI or locally.
- **Linter**: None configured (no golangci-lint config file).
- **Parallelism**: Tests do not use `t.Parallel()`. All tests run sequentially within each
  package.
- **Subtests**: Minimal usage. Only `TestActionError_Types` in `game_ws_test.go` uses
  `t.Run()` for table-driven subtests.
- **Benchmarks**: None (`Benchmark*` functions: 0).
- **Fuzz tests**: None (`Fuzz*` functions: 0).

### 2.2 Test Files and What They Cover

#### `internal/api/handlers/auth_test.go` (2 tests, 54 lines)

Tests the registration guard (enabled/disabled toggle) on the `AuthHandler`. Uses
`httptest.NewRequest` / `httptest.NewRecorder` for HTTP-level testing with nil database
dependencies.

| Test | What it verifies |
|---|---|
| `TestRegister_RegistrationDisabled` | Returns 403 when registration is off |
| `TestRegister_RegistrationEnabled_PassesGuard` | Passes guard, fails at JSON parse (400) |

**Gap**: Only tests the registration toggle. No tests for login, token validation, password
hashing, OAuth flows, JWT generation/verification, or any authenticated endpoint behavior.

#### `internal/api/handlers/game_ws_test.go` (13 tests, 488 lines)

Tests the WebSocket action handler (`HandleWSAction`). Uses a real `ws.Hub` with `httptest.Server`
and actual WebSocket connections via `gorilla/websocket`. A `wsTestEnv` helper struct manages
setup/teardown.

| Test Category | Count | What it verifies |
|---|---|---|
| Message parsing | 4 | Malformed JSON, unknown type, missing request ID, optional payload |
| Response format | 4 | Success/error JSON structure, request ID echo, serialization |
| Error handling | 3 | Internal error masking, game logic pass-through, error types |
| Integration path | 2 | Valid action reaches processAction, multi-request ID correlation |

**Gap**: All tests use nil database query objects, meaning `processAction` always panics or
returns an error. No tests verify successful action execution end-to-end through the WebSocket
handler. The handler's `runUserTick`, `OnConnect`, `OnDisconnect`, `GetState`, `PerformAction`
(REST), and `GetConfig` HTTP endpoints have zero test coverage.

#### `internal/api/middleware/ratelimit_test.go` (7 tests, 125 lines)

Tests the in-memory rate limiter (`CheckGameActionRate` and underlying `checkRate`).

| Test Category | Count | What it verifies |
|---|---|---|
| Allow within limit | 2 | Single request, 100 requests under 7200/min cap |
| Reject over limit | 1 | 7201st request rejected |
| Isolation | 2 | Per-user independence, per-key independence |
| Internal function | 2 | `checkRate` basic limit, key isolation |

**Gap**: No tests for the Redis-based rate limiter (`redis_ratelimit.go`). No tests for the
auth middleware, body limit middleware, CORS middleware, or JSON middleware.

#### `internal/game/engine/bitcoin_test.go` (28 tests, 505 lines)

Tests `buyBitcoin`, `sellBitcoin`, and related prestige behavior. Uses a `newBitcoinTestState()`
helper to create minimal `GameState` structs.

| Test Category | Count | What it verifies |
|---|---|---|
| Buy success/edge | 4 | Normal buy, exact balance, price=0, negative price |
| Buy validation | 4 | Zero amount, negative amount, overflow, invalid payload |
| Sell success/edge | 3 | Normal sell, exact balance, invalid payload |
| Sell validation | 4 | Zero amount, negative amount, price=0, negative price, overflow |
| ProcessAction dispatch | 3 | buy_bitcoin, sell_bitcoin, non-bitcoin action ignores price |
| Prestige persistence | 4 | Bitcoin survives, money resets, KP persists, CU resets |
| Round-trip | 2 | Buy/sell at same price, buy/sell with price increase |
| Boundary | 4 | Integer overflow, overflow boundary safe value |

**Gap**: None significant for the bitcoin trading feature -- this is the most thoroughly tested
area. Does not test `buyMaxBitcoin` or `sellAllBitcoin` convenience actions.

#### `internal/game/bitcoin/price_test.go` (24 tests, 631 lines)

Tests the Ornstein-Uhlenbeck bitcoin price simulation service. Uses an in-memory `PriceStore`
implementation (`memPriceStore`) and an error-injecting wrapper (`errorPriceStore`).

| Test Category | Count | What it verifies |
|---|---|---|
| Config defaults | 1 | `DefaultPriceConfig` returns TDD-specified values |
| OU step properties | 4 | Deterministic replay, seed divergence, min/max clamping, mean reversion |
| Lazy evaluation | 5 | No steps under interval, exactly 1 step at 5s, multi-step, catchup cap |
| Determinism | 2 | Same-batch replay identical, bulk vs incremental diverges (documented) |
| State persistence | 3 | Seed updated after step, negative elapsed handled, lastStepAt correct |
| History | 3 | Timestamps correct, default limit, limit applied |
| Error handling | 3 | GetPrice error, UpdatePrice error, InsertHistory error |
| Statistical | 1 | 10k-step mean within bounds, stddev minimum, all prices in range |
| Config accessor | 1 | Returns service config |

**Pattern note**: This is the only test file that uses a hand-written interface-based test
double (`memPriceStore` implementing `PriceStore`). It is the closest thing to a mocking
pattern in the codebase.

#### `internal/game/engine/cu_sinks_test.go` (46 tests, 1,070 lines)

Tests three CU-sink features: overclock mode, research tree, and rack optimization. Uses
`newTestState()` and payload helpers. This is the largest test file.

| Test Category | Count | What it verifies |
|---|---|---|
| Overclock activation | 6 | Tiers 1-3, insufficient CU, invalid tier, replace existing |
| Overclock idle progress | 6 | Decrement, expiry, multiplier applied, heat, offline expiry, partial |
| Overclock prestige | 1 | Resets overclock state |
| Research buy | 6 | Valid node, unknown node, tier-locked, insufficient CU, cost scaling, overflow |
| Research bulk buy | 2 | Max affordable, already has levels |
| Research idle effects | 3 | Idle income, reputation gain, money income |
| Research persistence | 1 | Survives prestige |
| Research job reward | 1 | Research affects job reward |
| Rack optimization | 5 | Success, wrong tier, no SaaS, insufficient CU, cost doubles |
| Rack + prestige | 3 | With optimization, without, resets on prestige |
| Rack overflow | 1 | Overflow guard |
| Cross-feature stacking | 1 | Overclock and research multiply |
| ProcessAction dispatch | 4 | Overclock, research, bulk research, optimize rack |
| Config endpoint | 3 | Overclock section, research section, rack optimization section |
| Helper functions | 3 | Research cost basic, overflow, aggregate bonuses |

### 2.3 Mocking and Test Double Patterns

The codebase uses two distinct approaches to isolate tests from dependencies:

1. **Nil dependencies with panic/error boundary testing** (most common): Handlers and engine
   functions receive nil pointers for database query objects. Tests verify behavior up to the
   point where database access would occur, then assert on the error or panic recovery. Used in
   `auth_test.go`, `game_ws_test.go`, `bitcoin_test.go`, and `cu_sinks_test.go`.

2. **Interface-based in-memory implementation** (one instance): `price_test.go` defines
   `memPriceStore` implementing the `PriceStore` interface. This provides a fully functional
   test double with in-memory state, enabling complete feature testing without a database. An
   `errorPriceStore` wrapper enables fault injection.

Three interfaces exist that could support test doubles:
- `PriceStore` (`internal/game/bitcoin/price.go`) -- used in tests
- `RateLimitStore` (`internal/api/middleware/ratelimit.go`) -- NOT used in tests
- `MessageBroadcaster` (`internal/api/ws/pubsub.go`) -- NOT used in tests

### 2.4 Test Helper Patterns

- **State factories**: `newBitcoinTestState()`, `newTestState()` create minimal `GameState`
  structs with reasonable defaults for the feature under test.
- **Payload builders**: `makePayload(amount)`, `makeResearchPayload(node)`,
  `makeOverclockPayload(tier)` create `json.RawMessage` payloads.
- **Environment struct**: `wsTestEnv` in `game_ws_test.go` bundles handler, hub, user ID,
  WebSocket connection, and test server with `close()` cleanup.
- **No shared test utilities package**: Each test file defines its own helpers inline. There
  is no `internal/testutil/` or `testdata/` directory.

### 2.5 Packages With Zero Test Coverage

The following packages have no test files at all:

| Package | Source Lines | Risk Level | Notes |
|---|---|---|---|
| `cmd/server/` | ~200 | Low | Application entrypoint, wiring |
| `cmd/healthcheck/` | ~30 | Low | Simple health check binary |
| `internal/api/routes/` | ~50 | Low | Route registration |
| `internal/api/ws/` | ~400 | **High** | WebSocket hub, pub/sub, Redis broadcaster |
| `internal/auth/` | ~200 | **High** | JWT generation/verification, password hashing |
| `internal/config/` | ~100 | Low | Configuration loading |
| `internal/database/` | ~100 | Medium | Connection pooling, migration runner |
| `internal/database/queries/` | ~1,000 | **High** | All SQL query functions (11 files) |
| `internal/game/catalog/` | ~300 | Medium | Hardware, research, SaaS, service, upgrade catalogs |
| `internal/game/events/` | ~150 | Medium | Random event engine |
| `internal/models/` | ~200 | Low | Data model structs |

### 2.6 Untested Functions in Tested Packages

Even within packages that have test files, significant functions lack coverage:

**`internal/game/engine/engine.go`** (1,701 lines) -- Tested functions cover bitcoin trading,
overclock, research, rack optimization, and prestige. Untested functions include:
- `ProcessIdleProgress` (full path with hardware/services/upgrades/expenses/customers)
- `runJob`, `buyHardware`, `sellHardware`, `deployService`, `upgradeTier`
- `buyUpgrade`, `upgradeComponent`, `resolveEvent`, `unlockSaas`, `deploySaas`
- `donateCU`, `buildDatacenter`, `upgradeDatacenter`
- `buyMaxBitcoin`, `sellAllBitcoin`
- `bulkUpgradeComponents`, `bulkDeployServices`, `bulkBuyUpgrades`, `bulkDeploySaas`
- Utility functions: `countShelfCapacity`, `nextTier`, `findUpgrade`, `pow`,
  `prestigeCostScale`, `tierPowerLimit`, `tierCoolingBonus`, `isRackTier`, `tierJobReward`

**`internal/api/handlers/game.go`** (1,348 lines) -- Only `HandleWSAction` message parsing is
tested. Untested:
- `GetState`, `PerformAction`, `GetConfig` HTTP handlers
- `runUserTick`, `runFullTick`, `runLightTick` tick system
- `buildResponse`, `pushEvents`, `getGroupBonus`, `processCustomerGrowth`, `fetchBitcoinData`
- `OnConnect`, `OnDisconnect` WebSocket lifecycle

**`internal/api/handlers/auth.go`** (170 lines) -- Only registration guard tested. Untested:
- `Login`, token refresh, all OAuth flows

**`internal/api/middleware/ratelimit.go`** (132 lines) -- In-memory limiter tested. Untested:
- Redis rate limiter (`redis_ratelimit.go`)
- Auth middleware, body limit, CORS, JSON content-type middleware

---

## 3. Frontend Testing (TypeScript/React)

### 3.1 Current State: No Tests

The desktop client (`apps/desktop/`) has **zero test files**, **zero test configuration**, and
**no test runner installed**. The `package.json` contains no test-related scripts or
dependencies (no vitest, jest, testing-library, cypress, or playwright).

The shared types package (`packages/shared/`) also has no tests. Its only validation is
TypeScript type-checking (`pnpm typecheck`).

### 3.2 Frontend Source Inventory

The desktop client consists of 25 TypeScript/React files totaling ~1,271 lines:

- **Components** (16 files): `App.tsx`, `ClickArea.tsx`, `CurrencyBar.tsx`,
  `DatacenterPanel.tsx`, `DonatePanel.tsx`, `EventLog.tsx`, `HardwarePanel.tsx`, `Login.tsx`,
  `MarketPanel.tsx`, `OverclockPanel.tsx`, `ResearchPanel.tsx`, `SaasPanel.tsx`,
  `ServicePanel.tsx`, `SocialPanel.tsx`, `TierProgress.tsx`, `UpgradePanel.tsx`,
  `shared/CurrencyValue.tsx`, `shared/CurrencyStatLine.tsx`
- **State management** (1 file): `stores/gameStore.ts` (Zustand)
- **API layer** (2 files): `api.ts`, `wsClient.ts`
- **Hooks** (3 files): `useConfig.ts`, `useWebSocket.ts`, `useIdleTick.ts`
- **Utilities** (1 file): `utils/currencyColors.ts`

### 3.3 What Would Be Testable

The frontend has clear testable units if a testing framework were added:

- **`gameStore.ts`**: Zustand store with game state management logic -- unit-testable.
- **`api.ts`**: HTTP API client -- unit-testable with fetch mocking.
- **`wsClient.ts`**: WebSocket client -- unit-testable with WS mock.
- **`useIdleTick.ts`**: Client-side idle tick calculation -- unit-testable.
- **`currencyColors.ts`**: Pure utility functions -- trivially unit-testable.
- **Components**: Could be tested with React Testing Library for interaction logic.

---

## 4. CI/CD Pipeline

### 4.1 Current CI Configuration

File: `.github/workflows/build.yml`

The pipeline runs on push to `main` and on pull requests targeting `main`. It has two jobs:

**Backend job:**
1. Checkout
2. Set up Go (version from `go.mod`, currently 1.25.0)
3. **`go test ./...`** -- runs all backend tests
4. (On push only) Build and push Docker images to GHCR

**Frontend job:**
1. Checkout
2. Set up pnpm + Node 22
3. `pnpm install --frozen-lockfile`
4. **`pnpm typecheck`** (shared package only) -- type checking, not testing
5. (On push only) Build and push Docker image to GHCR

### 4.2 What CI Does NOT Do

- **No coverage measurement** (`go test -cover` or `-coverprofile` not used)
- **No coverage thresholds or reporting** (no Codecov, Coveralls, or similar)
- **No frontend tests** (no test step in frontend job)
- **No lint step** (no golangci-lint, eslint, or prettier in pipeline)
- **No race condition detection** (`go test -race` not used)
- **No integration tests** (all tests are unit/component level with in-memory deps)
- **No database tests** (no test database provisioned in CI)
- **No build verification for Tauri** (only Docker image build)

---

## 5. Test Pyramid Assessment

```
                    /\
                   /  \          E2E Tests: NONE
                  /    \         No browser tests, no Playwright/Cypress,
                 /  E2E \        no API-level end-to-end tests
                /--------\
               /          \      Integration Tests: MINIMAL
              / Integration\     WebSocket handler tests use real Hub + WS connection
             /   (1 file)   \    but nil database dependencies
            /----------------\
           /                  \   Unit Tests: PARTIAL
          /    Unit Tests      \  120 tests across 6 files
         /   (6 files, ~2873   \  Concentrated in 3 recently-added features
        /      lines)           \ (bitcoin, overclock, research/optimization)
       /________________________\
```

### Pyramid Health: Inverted and Sparse

The test pyramid is bottom-heavy in the sense that all existing tests are unit-level, which is
correct. However, coverage is extremely uneven:

- **Well-tested** (>80% function coverage): Bitcoin trading engine, bitcoin price simulation,
  overclock mode, research tree, rack optimization, rate limiter, WS message parsing
- **Minimally tested** (<10%): Auth handler (registration guard only)
- **Not tested at all**: 11 backend packages, entire frontend, all middleware except rate
  limiter, all database queries, all game engine actions except bitcoin/overclock/research/rack,
  the tick system, WebSocket lifecycle, social features, events system

---

## 6. Database Testing

### 6.1 Current State: No Database Tests

There are no tests that interact with the database. The `internal/database/queries/` package
(11 files, ~1,000 lines of SQL query functions) has zero test coverage.

### 6.2 Migration Files

14 migration files exist in `internal/database/migrations/`. Migrations are applied manually
via `cat ... | sudo -u postgres psql`. There is no automated migration testing, no rollback
testing, and no schema validation tests.

### 6.3 Test Database Infrastructure

No test database configuration exists. CI does not provision a PostgreSQL/TimescaleDB instance.
There is no `docker-compose.test.yml` or similar for local test database setup.

---

## 7. Patterns and Conventions

### 7.1 Naming Conventions

- Test files: `<source>_test.go` (Go convention)
- Test functions: `Test<Function>_<Scenario>` (e.g., `TestBuyBitcoin_InsufficientFunds`)
- Helper functions: lowercase, descriptive (e.g., `newBitcoinTestState`, `makePayload`)
- No consistent convention for test categories within files -- some use `// ===` section
  headers, others do not.

### 7.2 Assertion Style

All assertions use explicit `if` checks with `t.Errorf` / `t.Fatalf`. Example:

```go
if gs.Money != 20000 {
    t.Errorf("Money = %d, want 20000 (50000 - 3*10000)", gs.Money)
}
```

The `t.Fatalf` pattern is used for precondition failures that should abort the test.
`t.Errorf` is used for assertions where the test can continue checking further conditions.

### 7.3 Error Message Style

Error messages follow `field = got, want expected` format with optional parenthetical
explanation. This is consistent across all test files.

### 7.4 Test Isolation

- Tests within a package share global state (rate limiter tests use unique user IDs to avoid
  cross-test pollution).
- WebSocket tests use per-test `httptest.Server` instances with explicit cleanup via
  `defer env.close()`.
- Engine tests create fresh `GameState` structs per test -- no shared mutable state.

---

## 8. Identified Gaps and Risks

### 8.1 Critical Gaps (High Risk)

| Gap | Risk | Impact |
|---|---|---|
| No auth/JWT tests | Broken auth could lock out all users or expose accounts | Security, availability |
| No database query tests | SQL bugs ship silently; data corruption possible | Data integrity |
| No game engine action handler tests (REST/WS) | Action processing bugs only caught in production | Core gameplay |
| No tick system tests | Idle progress calculation errors affect all players | Core gameplay |
| No coverage measurement | Cannot track whether coverage is improving or regressing | Process |

### 8.2 Moderate Gaps

| Gap | Risk |
|---|---|
| No frontend tests | UI regressions caught only by manual testing |
| No middleware tests (auth, CORS, body limit) | Security middleware bypasses undetected |
| No integration tests with database | Query/schema mismatches found only in production |
| No `go test -race` in CI | Data races in concurrent WebSocket handling undetected |
| No event engine tests | Random events could produce invalid state |

### 8.3 Low-Priority Gaps

| Gap | Risk |
|---|---|
| No benchmarks | Performance regressions undetected |
| No fuzz tests | Edge-case inputs in JSON parsing undetected |
| No lint step in CI | Style drift, potential bugs from common Go mistakes |
| No `t.Parallel()` usage | Test suite runs slower than necessary (~1.3s currently) |
| No shared test utilities package | Helper duplication across test files |

---

## 9. Existing Strengths

Despite the gaps, the testing that does exist demonstrates solid engineering:

1. **Thorough boundary testing**: Bitcoin tests cover overflow, negative values, zero values,
   exact boundaries, and round-trip invariants.
2. **Security-conscious testing**: Internal errors are verified to be masked from clients.
   Error types are explicitly tested.
3. **Determinism verification**: Price simulation tests verify deterministic replay with
   identical seeds and document known divergence behavior.
4. **Statistical property testing**: The OU process tests verify mean reversion, clamping,
   and distribution properties over large sample sizes.
5. **Interface-based testability**: The `PriceStore` interface demonstrates the pattern that
   could be extended to other database-dependent code.
6. **Good error message quality**: All test assertions include descriptive messages with
   expected vs. actual values.
7. **Real WebSocket testing**: The `wsTestEnv` uses actual WebSocket connections rather than
   mocking the protocol, catching real serialization and timing issues.

---

## 10. Test Execution Reference

### Running All Backend Tests

```bash
cd /root/project/apps/backend && go test ./...
```

Expected output (as of 2026-04-04): all 120 tests pass, ~1.3s total.

```
ok   github.com/homelab-game/backend/internal/api/handlers     1.320s
ok   github.com/homelab-game/backend/internal/api/middleware    0.009s
ok   github.com/homelab-game/backend/internal/game/bitcoin      0.012s
ok   github.com/homelab-game/backend/internal/game/engine       0.006s
```

Packages reporting `[no test files]`: `cmd/healthcheck`, `cmd/server`, `internal/api/routes`,
`internal/api/ws`, `internal/auth`, `internal/config`, `internal/database`,
`internal/database/queries`, `internal/game/catalog`, `internal/game/events`,
`internal/models`.

### Running With Verbose Output

```bash
cd /root/project/apps/backend && go test ./... -v -count=1
```

### Running With Coverage (Not Currently Used)

```bash
cd /root/project/apps/backend && go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

### Running With Race Detection (Not Currently Used)

```bash
cd /root/project/apps/backend && go test ./... -race
```

### Frontend Type Checking (Only Validation Available)

```bash
cd /root/project/packages/shared && pnpm typecheck
```
