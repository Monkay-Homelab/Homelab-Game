---
project: "homelab-the-game"
maturity: "experimental"
last_updated: "2026-04-04"
updated_by: "@staff-engineer"
scope: "Code quality conventions, tooling, patterns, and gaps across the Go backend and TypeScript frontend"
owner: "@staff-engineer"
dependencies: []
---

# Code Quality Specification

This document describes the code quality conventions, patterns, and tooling that **actually exist**
in the Homelab the Game codebase as of 2026-04-04. It is intended as onboarding material for a
developer contributing code. Aspirational improvements are clearly marked as gaps.

---

## 1. Project Maturity Assessment

The project is in an **experimental** state. Core gameplay works end-to-end (backend engine,
WebSocket real-time sync, React UI), but the codebase has grown organically without formal
linting, formatting, or code review gates. The CI pipeline runs `go test ./...` and
`pnpm typecheck` but enforces no lint rules, coverage thresholds, or formatting checks.

### Codebase Size (approximate)

| Area | Files | Lines of Code |
|---|---|---|
| Go backend (`apps/backend/`) | ~50 `.go` files | ~11,800 |
| TypeScript frontend (`apps/desktop/src/`) | ~27 `.ts`/`.tsx` files | ~3,700 |
| Shared package (`packages/shared/`) | 4 `.ts` files | ~80 |
| SQL migrations | 14 files | ~500 |

---

## 2. Static Analysis and Formatting Tools

### What Exists

| Tool | Configured | Enforced in CI |
|---|---|---|
| `go vet` | Implicit (runs with `go test`) | Yes (indirectly) |
| TypeScript compiler (`strict: true`) | Yes (`tsconfig.json` in desktop and shared) | Yes (`pnpm typecheck` in CI) |
| `go fmt` | Not configured as a check | No |

### What Does NOT Exist

The following common tools are **not installed, not configured, and not enforced**:

- **No Go linter**: No `golangci-lint`, no `.golangci.yml`, no lint step in CI.
- **No ESLint**: No `.eslintrc`, no `eslint.config.*`, no ESLint dependency in `package.json`.
- **No Prettier**: No `.prettierrc`, no Prettier dependency.
- **No EditorConfig**: No `.editorconfig` file.
- **No pre-commit hooks**: No `.pre-commit-config.yaml`, no `husky`, no `lint-staged`.
- **No Makefile**: No `Makefile` or task runner beyond npm scripts and direct `go` commands.

The only automated quality gate is the GitHub Actions workflow (`.github/workflows/build.yml`)
which runs `go test ./...` for the backend and `pnpm typecheck` for the shared package. The
frontend has no test step in CI at all.

---

## 3. Go Backend Conventions

### 3.1 Naming Conventions

The backend follows standard Go naming conventions:

- **Packages**: Lowercase, single-word where possible (`handlers`, `middleware`, `queries`,
  `engine`, `catalog`, `events`, `models`, `auth`, `config`, `ws`, `bitcoin`).
- **Exported types**: PascalCase (`GameHandler`, `AuthHandler`, `GameState`, `HardwareTemplate`).
- **Unexported types**: camelCase (`userMutexMap`, `userLock`, `visitor`, `cachedChildData`).
- **Constants**: PascalCase for exported (`PatchPanelBonusValue`, `TierCoffeeTable`),
  camelCase for unexported (`defaultTickInterval`, `sendBufSize`, `pingInterval`).
- **Acronyms**: Follow Go convention -- `UserID` (not `UserId`), `DB` (not `Db`),
  `JWT` (not `Jwt`), `CU` (not `Cu`).
- **Constructors**: `New<Type>` pattern consistently used (`NewHub()`, `NewAuthHandler()`,
  `NewGameHandler()`, `NewPriceService()`).
- **Interface names**: Generally suffixed with `-er` or descriptive nouns (`RateLimitStore`,
  `PriceStore`, `MessageBroadcaster`).

### 3.2 Error Handling Patterns

Two distinct error handling patterns coexist:

**HTTP handlers** use `http.Error()` with inline JSON strings:
```go
http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
```
This pattern is used consistently across `AuthHandler`, `SocialHandler`, and `GameHandler`
for REST endpoints. Errors are always JSON objects with an `error` field. Internal errors
return a generic message to avoid leaking implementation details.

**Game engine** returns `fmt.Errorf()` with descriptive messages:
```go
return nil, fmt.Errorf("not enough compute units (need %d, have %d)", cost, gs.ComputeUnits)
```
The engine never wraps errors with `%w` -- it creates new error values with user-facing
messages. The handler layer converts these into client-visible error responses.

**Database layer** uses `fmt.Errorf` with `%w` wrapping:
```go
return nil, fmt.Errorf("batch: failed to load game_state: %w", err)
```
This is the only layer that preserves the error chain for debugging.

**Custom error type** for WebSocket actions:
```go
type actionError struct {
    msg      string
    internal bool    // true = mask for client, log server-side
    notFound bool    // true = game state not found (session invalid)
}
```
The `actionError` type in `game.go` distinguishes game-logic errors (shown to client) from
internal errors (masked as "internal server error" for the client, logged server-side).

**Gaps**:
- No use of `errors.Is()` or `errors.As()` for typed error checking outside the WS handler.
- No structured logging (all logging uses `log.Printf` from stdlib).
- No sentinel error values -- errors are created inline.

### 3.3 Logging

Logging uses exclusively the standard library `log` package:

- `log.Println()` for informational messages (startup, connections).
- `log.Printf()` for formatted messages with context (Redis errors, cache timing).
- `log.Fatalf()` for fatal startup errors (database connection failure).
- Prefix conventions: `[cu-cache]`, `[ratelimit]` for subsystem identification.

There is no structured logging library (no `slog`, no `zap`, no `zerolog`). Log output is
unstructured plaintext with inconsistent formatting.

### 3.4 Code Organization

The backend follows Go's standard project layout for internal packages:

```
apps/backend/
  cmd/
    server/main.go          -- Entry point, dependency wiring
    healthcheck/main.go     -- Docker healthcheck binary
  internal/
    api/
      handlers/             -- HTTP and WebSocket request handlers
      middleware/            -- HTTP middleware (auth, CORS, rate limiting, body limit, JSON)
      routes/routes.go      -- Route registration
      ws/                   -- WebSocket hub, client management, pub/sub
    auth/                   -- JWT and password hashing
    config/                 -- Environment/secret loading
    database/
      db.go                 -- Connection pool setup
      queries/              -- Per-entity query structs (one file per entity)
      migrations/           -- SQL migration files (numbered sequentially)
    game/
      engine/               -- Tick system, action processing, game config
      events/               -- Random event definitions and triggering
      catalog/              -- Static game data (hardware, services, upgrades, research)
      bitcoin/              -- Bitcoin price simulation (Ornstein-Uhlenbeck model)
    models/                 -- Data structs shared across packages
```

Notable patterns:
- **One handler struct per domain**: `AuthHandler`, `GameHandler`, `SocialHandler` -- each
  owns its dependencies (query objects) as struct fields.
- **Query structs wrap the connection pool**: Each entity has a `queries.New<Entity>Queries(pool)`
  constructor. Queries use raw SQL with `pgx`, not an ORM.
- **Catalog as static data**: Game items (hardware, services, upgrades) are defined as Go
  package-level variables in `catalog/`. There is no database-driven catalog.
- **Engine is stateless**: `engine.Engine` has no fields -- it receives all state as function
  arguments and returns mutations as `ActionResult` structs.

**Large file concern**: `game.go` (1,348 lines) and `engine.go` (1,701 lines) are the two
largest files. The engine handles all game action validation in a single `ProcessAction`
switch statement with ~30 cases, each delegated to a private method. The handler file
manages WebSocket tick loops, state serialization, and action dispatch.

### 3.5 Concurrency Patterns

- **Per-user mutex map** (`userMutexMap`): Prevents race conditions on concurrent game
  actions from the same user. Self-cleaning every 5 minutes for users idle >10 minutes.
- **Buffered channels**: WebSocket clients use a buffered send channel (`sendBufSize = 16`)
  with non-blocking sends and drop-on-full semantics.
- **`sync.RWMutex`**: Used for the WebSocket hub client map and the tick state cache.
- **`defer recover()`**: Used in `trySend` to handle panics from sending on a closed channel
  during client disconnect races.
- **Background goroutines**: Cleanup timers, cache refresh, and tick loops all run as
  unmanaged goroutines (no context cancellation, no `errgroup`).

### 3.6 Dependency Injection

Dependencies are injected via constructors, not interfaces (except where noted). The `main.go`
function is the composition root -- it creates all query objects, the engine, the hub, and
the handlers, then wires them together.

Two interfaces exist for testability:
- `bitcoin.PriceStore` -- abstracts database operations for the bitcoin price service.
- `middleware.RateLimitStore` -- abstracts rate limit storage (in-memory vs. Redis).
- `ws.MessageBroadcaster` -- abstracts local vs. Redis-backed message broadcasting.

Most other dependencies are concrete types passed directly.

### 3.7 JSON Serialization

All models use struct tags for JSON serialization:
- `json:"snake_case"` -- consistent snake_case field naming.
- `json:"-"` for sensitive fields (`PasswordHash`).
- `json:",omitempty"` used selectively (e.g., `MemberCount` in groups, `Email` in users).
- `json.RawMessage` used for deferred payload parsing in WebSocket action requests.

---

## 4. TypeScript Frontend Conventions

### 4.1 TypeScript Configuration

Both `apps/desktop/tsconfig.json` and `packages/shared/tsconfig.json` enable:
- `"strict": true` -- Full strict mode (strictNullChecks, noImplicitAny, etc.).
- `"target": "ES2022"`, `"module": "ESNext"`, `"moduleResolution": "bundler"`.
- `"jsx": "react-jsx"` (desktop only).

This is the primary static analysis gate for the frontend.

### 4.2 Component Patterns

All React components are **function components** using hooks. No class components exist.

- **Named exports**: Every component uses `export function ComponentName()` (not default
  exports). Example: `export function HardwarePanel({ state }: { state: GameState })`.
- **Props typed inline**: Most components type props directly in the function signature
  rather than declaring a separate `Props` interface. Exception: the shared `CurrencyValue`
  component uses a declared `CurrencyValueProps` interface.
- **No prop spreading**: Components destructure specific props rather than using `{...props}`.
- **Hooks in `hooks/` directory**: Custom hooks (`useWebSocket`, `useConfig`, `useIdleTick`)
  follow the `use<Name>` convention and are colocated in `src/hooks/`.
- **Shared components in `components/shared/`**: Reusable presentational components
  (`CurrencyValue`, `CurrencyStatLine`) are separated from feature components.

### 4.3 State Management

State management uses **Zustand** with a single monolithic store (`gameStore.ts`):

- The store is created via `create<GameStore>()` with a typed interface.
- All game actions follow an identical pattern: clear error, call `wsClient.sendAction()`,
  set state on success, set error on failure. This pattern is repeated ~25 times without
  abstraction.
- Server state pushes are handled via `setStateFromPush` callback.
- Token persistence uses `localStorage` directly.
- No middleware, no devtools integration, no persistence middleware.

**Gap**: The repetitive action pattern in `gameStore.ts` (~25 near-identical async methods)
is a candidate for a generic action helper, but none exists currently.

### 4.4 Styling Approach

The project uses a **hybrid styling approach**:

1. **Tailwind CSS v4**: Used for layout, spacing, typography, and responsive utilities.
   Imported via `@import "tailwindcss"` in `global.css` and configured through
   `@tailwindcss/vite` plugin.

2. **CSS custom properties**: A comprehensive design token system is defined in `global.css`
   `:root` block. Two token families exist:
   - **UI tokens**: `--bg-deep`, `--bg-panel`, `--bg-card`, `--border`, `--text-primary`,
     `--accent-green`, etc.
   - **Currency tokens**: `--currency-cu`, `--currency-cu-bg`, `--currency-cu-border`,
     `--currency-cu-glow` (repeated for money, rep, kp, btc, pwr).

3. **Inline styles for dynamic/themed values**: Components use `style={{ color: 'var(--accent-green)' }}`
   extensively. This is the primary mechanism for applying the custom property design tokens.
   Tailwind handles structural layout; CSS vars handle theming/branding.

4. **CSS utility classes in `global.css`**: `.panel`, `.panel-card`, `.btn`, `.stat-value`,
   `.animate-slide-in`, `.animate-gentle-pulse` -- a small set of custom utility classes.

5. **Hardcoded color values in components**: Some components define color mappings as
   local constants (e.g., `CATEGORY_COLORS` in `HardwarePanel.tsx` uses raw hex/rgba values
   like `'#a855f7'` and `'rgba(168,85,247,0.1)'`). These are not referenced from the CSS
   custom property system.

**Gap**: The inline style usage is heavy and inconsistent. Some colors use CSS var references,
others use hardcoded hex values in the same file. There is no Tailwind theme configuration
extending the design tokens.

### 4.5 API Layer

The API client (`api.ts`) provides:
- A generic `request<T>()` function wrapping `fetch` with auth token injection.
- Automatic 401 handling (clear token, reload page).
- Type definitions for all API response shapes (`GameState`, `GameConfig`, etc.) -- these
  are defined in `api.ts`, NOT imported from the shared package.

The WebSocket client (`wsClient.ts`) is a singleton class (`WSClient`) that:
- Manages connection lifecycle with auto-reconnect via the `useWebSocket` hook.
- Implements request/response correlation via UUID-based message IDs.
- Falls back to HTTP API when WebSocket is disconnected.
- Uses a 10-second timeout for pending action requests.

**Gap**: The `GameState` type is defined independently in both `api.ts` (frontend) and
`models/game_state.go` (backend). The shared package (`packages/shared/src/types/game.ts`)
has its own `GameState` definition that uses `camelCase` field names and is significantly
out of date -- it lacks many fields present in the actual API response. The frontend
`api.ts` definition is the source of truth for the client, not the shared package.

### 4.6 Number Formatting

A utility function `formatNumber()` in `utils/currencyColors.ts` handles large number
abbreviation (K, M, B, T suffixes). This is used by the shared `CurrencyValue` component.
Earlier, multiple components had their own formatting logic -- this has been partially
consolidated but the function still lives in a file named `currencyColors.ts` rather than
a dedicated utilities file.

---

## 5. Shared Package (`packages/shared/`)

The shared package exports TypeScript types and constants:
- **Types**: `Tier` enum, `GameState` interface, `ActionType` enum, `GameAction` interface.
- **Constants**: `HARDWARE_SLOTS`, `RACK_SIZES`, `POWER_LIMITS`, `COLO_BASE_MULTIPLIER`.

**Current state**: The shared package is **partially stale**. The `GameState` interface in
`packages/shared/src/types/game.ts` uses `camelCase` field naming and a nested `Currencies`
object, while the actual backend API returns `snake_case` flat fields. The desktop client
imports `@homelab-game/shared` in `package.json` but the actual `api.ts` redefines
`GameState` locally with the correct shape. The shared package's type definitions are not
used at runtime.

The CI pipeline runs `pnpm typecheck` on the shared package, which verifies internal
consistency but does not validate alignment with the backend API.

---

## 6. Testing

### 6.1 Backend Tests

Six test files exist:

| File | Tests | What It Covers |
|---|---|---|
| `handlers/auth_test.go` | 2 | Registration feature flag (enabled/disabled guard) |
| `handlers/game_ws_test.go` | 11 | WebSocket action message parsing, validation, error masking, response format |
| `middleware/ratelimit_test.go` | (present) | Rate limiting logic |
| `game/bitcoin/price_test.go` | (present) | Bitcoin price simulation (Ornstein-Uhlenbeck model) |
| `game/engine/bitcoin_test.go` | (present) | Bitcoin buy/sell game actions |
| `game/engine/cu_sinks_test.go` | (present) | Compute unit sink mechanics |

**Testing patterns**:
- Standard library `testing` package only -- no testify, no gomock.
- `httptest.NewRequest` and `httptest.NewRecorder` for HTTP handler tests.
- `httptest.NewServer` with real WebSocket connections for WS handler tests.
- Table-driven tests used in some files (`TestActionError_Types`).
- Test helpers marked with `t.Helper()`.
- Test setup creates real hub/handler instances with nil query dependencies.

**Gaps**:
- No integration tests that exercise the database.
- No tests for the game engine's `ProcessIdleProgress` (the core tick calculation).
- No tests for most game actions (only bitcoin and CU sinks are tested).
- No tests for the frontend (zero `.test.tsx` or `.spec.ts` files).
- No test coverage measurement or threshold.
- CI runs `go test ./...` but does not fail on coverage regression.

### 6.2 Frontend Tests

**None exist.** There are no test files, no test runner configured, no testing libraries
in `package.json` (no Jest, no Vitest, no React Testing Library).

---

## 7. CI/CD Pipeline

The GitHub Actions workflow (`.github/workflows/build.yml`) runs on push to `main` and on
pull requests:

**Backend job**:
1. `go test ./...` -- Run all Go tests.
2. Build and push Docker image to GHCR (push to `main` only).
3. Build and push migration image (push to `main` only).

**Frontend job**:
1. `pnpm install --frozen-lockfile` -- Install dependencies.
2. `pnpm typecheck` (shared package only) -- TypeScript type checking.
3. Build and push Docker image to GHCR (push to `main` only).

**Gaps**:
- No lint step for Go or TypeScript.
- No formatting check.
- No frontend build verification in CI (no `pnpm build`).
- No frontend typecheck (only shared package is typechecked).
- No test coverage reporting.
- No security scanning (no `govulncheck`, no `npm audit`).
- No branch protection rules enforcing CI pass before merge.

---

## 8. Error Responses and API Conventions

### HTTP API

All API responses are JSON. Error responses consistently use:
```json
{"error": "human-readable error message"}
```

The `middleware.JSON` middleware sets `Content-Type: application/json` on all responses.
Error messages are lowercase, descriptive sentences. Internal errors are masked.

### WebSocket Protocol

Action responses use a structured envelope:
```json
{
  "type": "action_result",
  "id": "<request-uuid>",
  "success": true,
  "state": { ... },
  "error": ""
}
```

The `state` field uses `omitempty` (absent on error). The `error` field uses `omitempty`
(absent on success). Internal errors are masked as "internal server error".

Server-initiated pushes use:
```json
{"type": "state", "payload": { ... }}
{"type": "event", "payload": { ... }}
```

---

## 9. Design Patterns in Use

| Pattern | Where | Notes |
|---|---|---|
| **Repository** (query structs) | `database/queries/` | Each entity has a query struct wrapping `*pgxpool.Pool` |
| **Adapter** | `main.go` `bitcoinStoreAdapter` | Bridges `BitcoinQueries` to `PriceStore` interface |
| **Strategy** | `middleware/ratelimit.go` | `RateLimitStore` interface with in-memory and Redis implementations |
| **Observer/Callback** | `ws/hub.go` | `OnConnect`, `OnDisconnect`, `OnMessage` function callbacks |
| **Singleton** | `wsClient.ts` | Single `WSClient` instance exported as module-level constant |
| **Flux-like** | `gameStore.ts` | Zustand store as single source of truth, actions dispatch updates |
| **Graceful degradation** | `main.go`, `cu_cache.go` | Redis optional -- falls back to in-memory when unavailable |
| **Non-blocking send** | `ws/hub.go` | Drop messages for slow clients rather than blocking |
| **Optimistic update** | `gameStore.ts` `runJob` | Click reward applied immediately, corrected on server response |

---

## 10. Configuration Management

### Backend

Configuration is loaded from environment variables with fallbacks to Docker secrets
(`/run/secrets/<name>`) and then to hardcoded defaults. See `config/config.go`.

| Variable | Default | Secret Fallback |
|---|---|---|
| `PORT` | `8080` | -- |
| `DB_HOST` | `localhost` | -- |
| `DB_PORT` | `5432` | -- |
| `DB_USER` | `homelab_game` | -- |
| `DB_PASSWORD` | `""` | `/run/secrets/db_password` |
| `DB_NAME` | `homelab_game` | -- |
| `JWT_SECRET` | Random (dev warning) | `/run/secrets/jwt_secret` |
| `REDIS_ADDR` | `redis:6379` | -- |
| `REDIS_PASSWORD` | `""` | `/run/secrets/redis_password` |
| `REGISTRATION_ENABLED` | `true` | -- |
| `CORS_ORIGINS` | -- | -- |
| `ENV` | -- (non-production assumed) | -- |

A `.env` file is loaded manually via `loadEnvFile()` in `main.go` (custom parser, not
a library like `godotenv`).

### Frontend

Build-time configuration via Vite environment variables:
- `VITE_API_URL` -- API base URL (defaults to `https://api.homelab.living`).

Runtime configuration is fetched from `/api/game/config` after login.

---

## 11. Code Duplication and Known Debt

### Duplicated Origin Allowlists

The CORS origin allowlist is defined independently in two places:
1. `middleware/cors.go` (`allowedOrigins` map)
2. `ws/hub.go` (`upgrader.CheckOrigin` function)

Both read `CORS_ORIGINS` env var and `ENV` for dev mode, but are maintained separately.
Adding a new allowed origin requires updating both files.

### Duplicated Type Definitions

The `GameState` type is defined in three places:
1. `models/game_state.go` (Go, authoritative)
2. `apps/desktop/src/api.ts` (TypeScript, manually synchronized)
3. `packages/shared/src/types/game.ts` (TypeScript, stale)

### Repetitive Store Actions

The Zustand store has ~25 action methods that follow an identical try/catch/setState pattern.
No generic helper abstracts this.

### Large Files

- `engine.go` (1,701 lines) -- All game action logic in one file.
- `game.go` (1,348 lines) -- Handler, tick loop, state serialization, WebSocket dispatch.

These files are at the upper limit of maintainability but are internally well-organized
with clear method boundaries.

---

## 12. TODOs, FIXMEs, and Annotations

A grep for `TODO`, `FIXME`, `HACK`, and `XXX` across all Go and TypeScript source files
returns **zero results**. The codebase does not use inline task annotations.

---

## 13. Security-Relevant Quality Observations

- **Passwords**: bcrypt with `DefaultCost` (10). Adequate.
- **JWT**: HS256 with 24-hour expiry. Secret auto-generated in dev (logged warning).
- **SQL injection**: Parameterized queries throughout (`$1`, `$2`, etc.). No string
  concatenation of user input into SQL.
- **Input validation**: Display names validated (length, alphanumeric, profanity filter).
  Email validated via `net/mail.ParseAddress`. Password length 8-128.
- **Rate limiting**: Per-IP for auth (10/min), per-user for game actions (7200/min),
  per-user for social (180/min). Redis-backed when available, in-memory fallback.
- **Body size limit**: 64KB max via `MaxBytesReader`.
- **Error masking**: Internal errors are not exposed to clients over WebSocket.
- **Sensitive field suppression**: `PasswordHash` tagged `json:"-"`, `OAuthID` tagged `json:"-"`.

---

## 14. Recommendations for Contributors

When contributing code to this project, follow these observed conventions:

1. **Go naming**: Follow standard Go conventions. Use `MixedCaps` for exports, `mixedCaps`
   for unexported. Uppercase acronyms (`UserID`, `JWT`, `CU`).
2. **Error messages**: Lowercase, no trailing punctuation. Include relevant values
   (e.g., "need %d, have %d"). Use `%w` wrapping only in the database layer.
3. **HTTP errors**: Always return `{"error":"..."}` JSON. Use `http.Error()` with inline
   JSON strings for consistency with existing handlers.
4. **New game actions**: Add a case to `engine.ProcessAction()`, implement as a private
   method on `*Engine`, return an `*ActionResult`.
5. **New queries**: Create a new file in `database/queries/` with a `New<Entity>Queries(pool)`
   constructor.
6. **React components**: Named function exports, typed props inline, Tailwind for layout,
   CSS vars for colors. Use the `CURRENCY_COLORS` system for currency-related styling.
7. **State management**: Add new actions to the `GameStore` interface and implement in the
   `create<GameStore>` closure.
8. **No inline TODOs**: The codebase does not use TODO/FIXME annotations. Track work items
   externally.
9. **Testing**: Write tests using the standard library `testing` package. Use table-driven
   tests for parameterized cases. Mark test helpers with `t.Helper()`.
10. **CORS origins**: If adding a new allowed origin, update BOTH `middleware/cors.go` AND
    `ws/hub.go`.
