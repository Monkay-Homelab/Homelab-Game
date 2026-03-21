---
project: "project"
maturity: "proof-of-concept"
last_updated: "2026-03-20"
updated_by: "@staff-engineer"
scope: "Documents code quality practices, conventions, tooling, and gaps across the monorepo"
owner: "@staff-engineer"
dependencies:
  - architecture.md
---

# Code Quality Specification

## 1. Repository Structure and Organization

### 1.1 Monorepo Layout

The project is a pnpm workspace monorepo (`pnpm@10.32.1`) with three application targets and one shared package:

```
project/
  apps/
    backend/      # Go 1.25 server (REST API, game engine, auth)
    desktop/      # Tauri 2.x + React 19 + TypeScript 5.9 desktop client
    mobile/       # React Native (empty -- directory structure only, no source files)
  packages/
    shared/       # TypeScript types and constants
  stress-tests/   # Go-based load testing tool (standalone)
  docs/
    spec/         # Project specifications (this directory)
```

**Observation:** The `mobile/` directory contains empty subdirectories (`src/assets`, `src/components`, `src/hooks`, `src/navigation`, `src/screens`) but zero source files. It is a placeholder only.

### 1.2 Module Boundaries

**Backend (Go):** Follows Go standard project layout under `internal/`:

```
cmd/server/           # Single entrypoint
internal/
  api/
    handlers/         # HTTP handlers (auth, game, social)
    middleware/        # Auth, CORS, body limit, JSON, rate limiting
    routes/           # Route registration
    ws/               # WebSocket hub
  auth/               # JWT + bcrypt password hashing
  config/             # Environment-based config loading
  database/
    migrations/       # Sequential .sql migration files (001-009)
    queries/          # Per-entity query structs
  game/
    catalog/          # Static game data (hardware, services, upgrades, SaaS)
    engine/           # Game engine (idle progress, action processing)
    events/           # Random event system
  models/             # Domain models (game_state, user, group)
```

**Desktop (TypeScript/React):** Flat component structure:

```
src/
  components/         # 12 React components (one file each)
  hooks/              # 3 custom hooks (useConfig, useIdleTick, useWebSocket)
  stores/             # Zustand store (gameStore.ts)
  styles/             # global.css (CSS variables + Tailwind)
  api.ts              # API client with all type definitions
  App.tsx             # Root component with tab routing
  main.tsx            # Entry point
```

**Shared Package (`@homelab-game/shared`):**

- `types/game.ts` -- Tier enum, GameState interface, ActionType enum
- `constants/index.ts` -- Slot limits, power limits, colo formula constants
- Exported via `src/index.ts` barrel

**Gap:** The shared package types are largely out of sync with the backend's actual API shape. The desktop client defines its own `GameState` interface in `api.ts` with ~75 fields that does not use the shared package types. The shared `GameState` interface uses camelCase (`computeUnits`) while the backend serializes as snake_case (`compute_units`). The shared types are effectively unused.

---

## 2. Language-Specific Conventions

### 2.1 Go (Backend)

**Naming:**
- Package names: lowercase, single-word (`handlers`, `middleware`, `catalog`, `engine`)
- Struct names: PascalCase (`GameHandler`, `AuthHandler`, `ActionResult`)
- Handler methods: PascalCase, receiver on struct (`func (h *GameHandler) GetState(...)`)
- Query structs: `{Entity}Queries` pattern (`UserQueries`, `GameStateQueries`)
- Constructors: `New{Type}(...)` pattern (`NewGameHandler(...)`, `NewHub()`)
- Constants: PascalCase for exported (`PatchPanelBonusValue`), no prefix convention
- Type aliases: `type Tier string`, `type contextKey string`

**Error Handling:**
- Uses Go's standard `error` return pattern consistently
- HTTP errors written as inline JSON strings: `http.Error(w, '{"error":"..."}', statusCode)`
- No custom error types -- raw `fmt.Errorf` throughout
- Database errors from `pgx` propagated directly; the handler interprets context (e.g., "email already exists" for unique constraint violations)
- Some error returns are silently discarded (notable in `GetState` and `PerformAction` where multiple `_, _ :=` patterns skip DB fetch errors for hardware, services, etc.)

**Patterns:**
- Dependency injection via constructor structs (all query structs and the engine are injected into handlers)
- Per-user mutex map for concurrent action serialization (`userMutexMap`)
- No interfaces defined anywhere -- all dependencies are concrete types
- Middleware uses the `func(http.Handler) http.Handler` chain pattern
- Context values for authenticated user ID (`contextKey` typed key)
- No use of third-party HTTP routers -- uses Go 1.22+ `http.ServeMux` with method patterns (`"GET /api/..."`)

**Code Organization:**
- `engine.go` at 1,316 lines is the largest file, containing the full game engine (idle progress, all 19 action handlers, tier logic, prestige, bulk operations)
- `game.go` handler at 533 lines is the second largest, containing HTTP handler logic with significant business logic inline (colo rack income, group bonus calculation, customer growth)
- `config.go` in the engine package (236 lines) defines the full game config response with tier metadata
- Game catalog data is split across four files: `hardware.go`, `services.go`, `upgrades.go`, `saas.go`

**Notable Code Duplication:**
- `TierToRank()` is implemented three times: `catalog.TierToRank()`, `engine.tierToRank()` (unexported), and indirectly via `isRackTier()`
- Group bonus calculation and colo rack income logic is duplicated between `GetState` and `PerformAction` handlers (~30 duplicated lines)
- CORS allowed origins are duplicated between `middleware/cors.go` (map-based) and `ws/hub.go` (slice-based) with no shared configuration

### 2.2 TypeScript (Desktop Client)

**Naming:**
- Components: PascalCase filenames and exports (`ClickArea.tsx`, `HardwarePanel.tsx`)
- Hooks: `use` prefix, camelCase (`useConfig.ts`, `useWebSocket.ts`)
- Store: camelCase (`gameStore.ts`)
- Interfaces: PascalCase (`GameState`, `HardwareTemplate`, `AuthResponse`)
- All API type interfaces defined locally in `api.ts` rather than imported from shared

**Patterns:**
- Zustand store as the single source of truth for all game state
- API client wraps `fetch` with automatic token injection and 401 handling
- Store actions follow `set({ error: null }) -> await api -> set({ state }) | catch -> set({ error })` pattern uniformly
- Optimistic update for `runJob` only; all other actions are server-authoritative
- Polling-based state sync (`setInterval(fetchState, 5000)`)
- WebSocket used exclusively for event notifications (server push), not for state sync
- React Strict Mode compatible (WebSocket connection uses delayed init + mounted ref guard)

**TypeScript Configuration:**
- `strict: true` enabled in both `tsconfig.json` files (desktop and shared)
- Target: ES2022, Module: ESNext, Module Resolution: bundler
- `skipLibCheck: true` in both configs
- `declaration: true` in shared package for type generation

**Styling:**
- Tailwind CSS v4 via Vite plugin (`@tailwindcss/vite`)
- CSS custom properties for theme colors (dark theme only, no light mode)
- Component styling is inline via `style={{...}}` props referencing CSS variables, plus Tailwind utility classes
- Custom CSS classes in `global.css` for common patterns: `.panel`, `.panel-card`, `.btn`, `.stat-value`

### 2.3 Rust (Tauri Shell)

The Tauri shell is minimal boilerplate:
- `main.rs`: 6 lines, standard Tauri entry point
- `lib.rs`: 17 lines, sets up Tauri builder with log plugin in debug mode
- No custom Tauri commands, no Rust business logic
- Edition 2021, `rust-version = "1.77.2"`, Tauri 2.10.3

---

## 3. Static Analysis and Tooling

### 3.1 Linting

**Present:**
- TypeScript `strict: true` mode provides type checking (`pnpm typecheck` in shared package; `tsc` in desktop build step)

**Absent:**
- No ESLint configuration anywhere in the repository
- No Go linter configuration (no `.golangci.yml`, no `golangci-lint` references)
- No Clippy configuration for Rust
- No Prettier configuration
- No `.editorconfig`
- No pre-commit hooks (no `.pre-commit-config.yaml`, no Husky/lint-staged)

### 3.2 Formatting

**Present:** None. No formatter configuration exists for any language.

**Implicit:** Go code appears to follow `gofmt` conventions (consistent formatting), likely from editor auto-formatting. TypeScript code has no consistent formatting standard applied.

### 3.3 CI/CD

**Absent.** No CI/CD pipeline exists:
- No `.github/workflows/` directory for this project (one exists under `_temp_claude/` which is unrelated)
- No `Makefile`, `Taskfile`, or `justfile`
- No Dockerfile or docker-compose
- No deployment scripts

### 3.4 Type Safety

**TypeScript:** `strict: true` is the primary quality gate. However, the desktop client's `api.ts` defines all types locally rather than consuming them from `@homelab-game/shared`, creating type drift risk. The shared package exports a `GameState` interface with different field names and a subset of the actual fields.

**Go:** Standard Go type safety. No `any` usage. JSON tags on all model structs. The `authResponse` struct uses `User any` which loses type information.

---

## 4. Testing

### 4.1 Current State

**There are zero tests in the entire codebase:**
- No `*_test.go` files in the Go backend
- No test configuration, test runner, or test files in the TypeScript projects
- No Jest, Vitest, Playwright, or any test framework in `package.json` dependencies
- The stress-test tool (`stress-tests/`) is a load testing utility, not a unit/integration test suite

### 4.2 Testability Assessment

**Backend:** The architecture is partially testable:
- Query structs accept `*pgxpool.Pool` -- no interfaces, making mocking difficult without integration tests
- The `Engine` struct has no dependencies (pure functions on game state), making it the most testable component
- Handlers depend on concrete query types, not interfaces, so testing requires a real database or significant refactoring
- The `loadEnvFile()` in `main.go` is a custom implementation instead of using `godotenv`, not easily testable

**Frontend:** Components receive state via props or Zustand selectors, making them unit-testable in principle. The API client is a module-level singleton, which complicates mocking.

---

## 5. Error Handling Patterns

### 5.1 Backend HTTP Errors

All HTTP error responses are hand-crafted JSON strings:

```go
http.Error(w, `{"error":"game state not found"}`, http.StatusNotFound)
```

This pattern is consistent throughout but has issues:
- Error messages are not structured (no error codes, no correlation IDs)
- JSON is constructed via string literals, risking malformed JSON if special characters appear in dynamic messages
- The `PerformAction` handler marshals dynamic errors: `json.Marshal(map[string]string{"error": err.Error()})` -- inconsistent with the string literal approach used elsewhere

### 5.2 Silently Discarded Errors

Multiple database fetch calls discard errors with `_, _ :=`:

```go
hw, _ := h.hardware.GetByGameStateID(r.Context(), gs.ID)
svcs, _ := h.services.GetByGameStateID(r.Context(), gs.ID)
// ... 5 more similar lines
```

This means a database failure for fetching hardware or services results in nil slices being processed silently rather than returning an error to the client.

### 5.3 Frontend Error Handling

The API client throws errors on non-2xx responses. The Zustand store catches them and sets `error` state, which the App component renders as a dismissible banner. The 401 handler forces a page reload after clearing the token. Empty `catch {}` blocks exist in the WebSocket message handler.

---

## 6. Database and Migration Practices

### 6.1 Migration Strategy

Migrations are sequentially numbered SQL files (`001_initial_schema.sql` through `009_global_cu_store.sql`) applied manually via `psql`. There is no migration tool (no Flyway, golang-migrate, or similar). Migration `007` was renamed to `.APPLIED` as a manual tracking mechanism.

There is also a `wipe_player_progress.sql` utility script (not part of the numbered sequence).

### 6.2 Query Patterns

- Raw SQL using `pgx/v5` directly (no ORM, no query builder)
- Parameterized queries throughout (`$1`, `$2`, ...) -- no SQL injection risk
- The `GameStateQueries` uses a shared column string constant (`gsColumns`) and scan helper (`gsFields`) to avoid column drift between SELECT and UPDATE, though the UPDATE has 31 numbered parameters that must stay in sync manually
- `rows.Close()` is properly deferred in all query methods that use `Query()`
- UUIDs generated database-side via `gen_random_uuid()`

---

## 7. Security-Relevant Code Quality

### 7.1 Credentials in Repository

**The `.env` file at `apps/backend/.env` is tracked in git and contains actual production credentials** including the database password and JWT secret. While `.env` is listed in `.gitignore`, the file was committed before the gitignore rule was added (or was force-added). This is a significant security concern.

### 7.2 Authentication

- bcrypt with `DefaultCost` (10 rounds) for password hashing -- adequate
- JWT tokens with HS256 signing, 24-hour expiry
- JWT secret auto-generated randomly if not provided (dev convenience, with a warning log)
- WebSocket auth via query parameter token (necessary but token appears in access logs)

### 7.3 Input Validation

- Registration validates email format, password length (8-128), display name (2-20 chars, alphanumeric, profanity filter)
- Game actions validate costs, tier requirements, slot/power capacity server-side
- Request body limited to 64KB globally
- Rate limiting per IP (auth: 10/min) and per user (game: 7200/min, social: 180/min)

---

## 8. Design Patterns in Use

### 8.1 Architectural Patterns

| Pattern | Where | Notes |
|---------|-------|-------|
| Server-authoritative | Game engine | All state mutations validated server-side |
| Repository pattern (partial) | `internal/database/queries/` | Per-entity query structs, but no interface abstraction |
| Constructor injection | Handler setup in `main.go` | Dependencies passed via `New*Handler()` constructors |
| Middleware chain | HTTP handlers | Standard Go `func(http.Handler) http.Handler` |
| Catalog/registry | `internal/game/catalog/` | Static game data as Go variables |
| Optimistic UI | Click handler only | `runJob` adds reward locally before server confirms |
| Polling + WebSocket hybrid | Frontend | Polling for state (5s), WebSocket for event push |

### 8.2 Absent Patterns

- **No interfaces:** The entire Go backend defines zero interfaces. All dependencies are concrete types.
- **No dependency inversion:** Query structs, engine, and hub are concrete dependencies in handlers.
- **No structured logging:** Uses `log.Println` / `log.Printf` throughout. No log levels beyond the default, no structured fields, no request/trace ID propagation.
- **No configuration validation:** Config loads from environment variables with fallback defaults; no validation that the resulting configuration is sensible.
- **No graceful shutdown:** The server uses `http.ListenAndServe` with no signal handling or shutdown timeout.
- **No health check depth:** The `/health` endpoint returns `{"status":"ok"}` unconditionally without checking database connectivity.
- **No request/response logging middleware.**
- **No pagination:** List endpoints use hardcoded limits (e.g., `LIMIT 50` for groups and leaderboards).

---

## 9. Code Complexity Hotspots

### 9.1 Engine File (1,316 lines)

`internal/game/engine/engine.go` is the highest-complexity file in the codebase. It contains:
- `ProcessIdleProgress` (~100 lines of math with 6 different multiplier calculations)
- `ProcessAction` switch statement dispatching to 19 different action handlers
- All 19 action handler implementations inline
- Tier helper functions, prestige cost scaling, bulk operations
- The `bulkUpgradeComponents` function has a nested `for upgraded` loop iterating all hardware across all component types until no more upgrades can be afforded -- O(hardware * components * max_levels)

### 9.2 Game Handler (533 lines)

`internal/api/handlers/game.go` mixes HTTP handling with business logic:
- `GetState` and `PerformAction` both contain 40+ lines of inline business logic (colo rack income calculation, group bonus, customer growth)
- The `PerformAction` handler is 215 lines long, with 10 different `if result.New*` blocks for persisting different entity types
- Significant code duplication between `GetState` and `PerformAction`

### 9.3 Game Store (308 lines)

`stores/gameStore.ts` defines 23 nearly-identical async action methods following the same `set({ error: null }) -> await api.action -> set({ state }) -> catch -> set({ error })` pattern. This is highly repetitive.

---

## 10. Dependency Health

### 10.1 Go Dependencies (4 direct)

| Dependency | Version | Purpose | Status |
|-----------|---------|---------|--------|
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | JWT auth | Active, well-maintained |
| `github.com/jackc/pgx/v5` | v5.8.0 | PostgreSQL driver | Active, industry standard |
| `golang.org/x/crypto` | v0.49.0 | bcrypt | Active, Go team maintained |
| `github.com/gorilla/websocket` | v1.5.3 | WebSocket | Archived (maintenance mode since 2023) |

**Note:** `gorilla/websocket` is archived. While stable, it will not receive new features. The Go standard library gained `nhooyr.io/websocket` as a popular alternative, though for this use case the archived library is adequate.

### 10.2 TypeScript Dependencies (Desktop)

| Dependency | Version | Purpose |
|-----------|---------|---------|
| `react` / `react-dom` | ^19.2.4 | UI framework |
| `zustand` | ^5.0.12 | State management |
| `tailwindcss` | ^4.2.1 | CSS framework |
| `typescript` | ^5.9.3 | Type system |
| `vite` | ^8.0.0 | Bundler |
| `@vitejs/plugin-react` | ^6.0.1 | Vite React support |

All dependencies are current-generation releases with no known EOL concerns.

---

## 11. Identified Gaps (Prioritized)

### Critical

1. **Committed secrets:** `apps/backend/.env` containing database password and JWT secret is tracked in git history.
2. **Zero test coverage:** No tests exist anywhere in the codebase. The game engine's math-heavy logic (idle progress, prestige scaling, multiplier stacking) is the highest-risk untested area.

### High

3. **No CI/CD pipeline:** No automated builds, tests, or deploys. Everything is manual.
4. **No linting or formatting tools configured:** Code quality depends entirely on developer discipline.
5. **Silently discarded database errors:** Multiple `_, _ :=` patterns in handlers could mask production failures.
6. **No graceful shutdown:** Server shutdown kills in-flight requests and WebSocket connections without draining.

### Medium

7. **No interfaces in Go backend:** Makes unit testing impossible without database; prevents mocking for handler tests.
8. **Shared package type drift:** `@homelab-game/shared` types diverged from the actual API contract; the desktop client maintains its own type definitions.
9. **Code duplication:** Colo income and group bonus logic duplicated between GetState and PerformAction; TierToRank implemented three times; CORS origins duplicated between middleware and WebSocket.
10. **Large file complexity:** `engine.go` at 1,316 lines combines idle processing, action dispatch, and all action implementations.
11. **No structured logging:** `log.Println` provides no log levels, structured fields, or request correlation.
12. **Mobile app is empty:** Directory structure exists with no source files.

### Low

13. **No `.editorconfig`:** No cross-editor formatting consistency enforcement.
14. **Hardcoded pagination limits:** All list endpoints use `LIMIT 50` with no client control.
15. **`authResponse.User` typed as `any`:** Loses type information in the auth response.
16. **Manual migration tracking:** No migration tool; relies on file numbering and manual `psql` execution.
17. **Repetitive store actions:** 23 nearly-identical action methods in `gameStore.ts` could be reduced with a factory pattern.
