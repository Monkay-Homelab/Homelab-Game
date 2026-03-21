---
project: "project"
maturity: "experimental"
last_updated: "2026-03-20"
updated_by: "@staff-engineer"
scope: "System architecture of Homelab the Game -- components, boundaries, data flow, and integration points"
owner: "@staff-engineer"
dependencies: []
---

# Architecture Specification

## 1. System Overview

Homelab the Game is a server-authoritative AFK/clicker simulation game where players build and manage a homelab, progressing from a single server on a coffee table through rack tiers (12U-48U) to colocation and datacenter ownership. The system is structured as a monorepo with three application components and one shared library.

**Maturity rationale:** The project is classified as "experimental" -- the backend and desktop client are functional with core game mechanics working end-to-end, but there are no automated tests, the mobile client does not exist, OAuth is modeled but not implemented, several planned features (ads, IAP, push notifications) are absent, and the shared types package has drifted out of sync with the actual API contract. The architecture is sound for a single-developer project but lacks the operational hardening expected for production.

## 2. Repository Structure

```
/
  package.json            # Root monorepo manifest (pnpm 10.32.1)
  pnpm-workspace.yaml     # Workspace: packages/*, apps/*
  apps/
    backend/              # Go 1.25 HTTP server (game engine, auth, API)
    desktop/              # Tauri 2.x + React 19 + Vite 8 desktop client
    mobile/               # Planned React Native client (DOES NOT EXIST)
  packages/
    shared/               # @homelab-game/shared TypeScript types/constants
  stress-tests/           # Go-based load testing tool
  docs/
    spec/                 # Project specifications (this directory)
```

### Component Inventory

| Component | Language/Runtime | Status | Lines (est.) |
|-----------|-----------------|--------|-------------|
| Backend API | Go 1.25 | Functional | ~3,500 |
| Desktop Client | TypeScript/React 19 | Functional | ~2,500 |
| Tauri Shell | Rust (Tauri 2.x) | Minimal scaffold | ~20 |
| Shared Package | TypeScript | Stale/drifted | ~50 |
| Mobile Client | React Native | Not started | 0 |
| Stress Tests | Go | Functional | ~500 |

## 3. Backend Architecture

### 3.1 Technology Stack

- **Language:** Go 1.25
- **HTTP:** `net/http` (stdlib) with Go 1.22+ pattern routing (`"GET /path"` syntax)
- **Database driver:** `pgx/v5` with connection pooling (`pgxpool`)
- **WebSocket:** `gorilla/websocket`
- **Auth:** `golang-jwt/jwt/v5` for JWT, `golang.org/x/crypto/bcrypt` for password hashing
- **Framework:** None -- pure stdlib `http.ServeMux`, custom middleware chain

### 3.2 Process Architecture

The backend is a single-process Go HTTP server. There are no background workers, message queues, or separate microservices. All game logic executes synchronously within HTTP request handlers. The WebSocket hub runs as goroutines within the same process.

```
main.go
  |-- loadEnvFile()          # Custom .env loader (not dotenv)
  |-- config.Load()          # Env-based configuration
  |-- database.Connect()     # pgxpool connection (max 20, min 2)
  |-- queries.New*()         # 10 query structs instantiated
  |-- engine.New()           # Stateless game engine
  |-- ws.NewHub()            # WebSocket connection manager
  |-- handlers.New*()        # 3 handler structs (Auth, Game, Social)
  |-- routes.Setup()         # Route registration + middleware chain
  |-- http.ListenAndServe()  # Blocking listen on :PORT
```

### 3.3 Module Structure

```
internal/
  config/           # Environment-based config (Config struct)
  database/
    db.go           # Connection pool factory
    migrations/     # 9 SQL migration files (manually applied)
    queries/        # 10 query files (hand-written SQL via pgx)
  auth/
    jwt.go          # Token generation/validation (HS256, 24h expiry)
    password.go     # bcrypt hash/check
  api/
    handlers/
      auth.go       # Register, Login (email/password only)
      game.go       # GetState, PerformAction, GetConfig
      social.go     # Groups, Leaderboards
    middleware/
      auth.go       # JWT Bearer token extraction
      cors.go       # Allowlist-based CORS
      ratelimit.go  # In-memory sliding window (IP + user buckets)
      json.go       # Content-Type header
      bodylimit.go  # 64KB max request body
    routes/
      routes.go     # Route table and middleware composition
    ws/
      hub.go        # WebSocket hub (gorilla/websocket)
  game/
    engine/
      engine.go     # Core game loop: idle progress + action processing
      config.go     # Server-side game config (tier data, bonus maps)
    catalog/
      hardware.go   # Hardware item templates (23 items)
      services.go   # Service templates (26 services)
      upgrades.go   # Upgrade templates (cooling, networking, automation, knowledge, components)
      saas.go       # SaaS service templates (9 services), customer names, business expenses
    events/
      events.go     # Random event system (6 event categories, ~25 events)
    state/          # EMPTY directory (no files)
  models/
    user.go         # User struct
    game_state.go   # GameState, Hardware, Service, Upgrade, Customer, Expense, ColoRack, ComponentUpgrade
    group.go        # Group, GroupMember, LeaderboardEntry
```

### 3.4 Request Processing Model

The backend uses a request-response model where every GET or POST processes the full game state cycle. There is no background tick loop -- idle progress is calculated lazily on each request.

**GET /api/game/state flow:**
1. Load all game data from DB (game state + 7 related tables)
2. Run `ProcessIdleProgress()` -- calculates resources earned since `last_tick_at`
3. Calculate group bonus and colo rack passive income
4. Process SaaS customer growth
5. Persist updated game state and customers to DB
6. Push any triggered events via WebSocket
7. Build and return full state response (including available catalog items)

**POST /api/game/action flow:**
1. Acquire per-user mutex lock (in-memory `sync.Mutex` map)
2. Load all game data from DB
3. Run `ProcessIdleProgress()` (same as GET)
4. Run `ProcessAction()` -- validate and apply the player action
5. Persist all new/updated/deleted records to DB
6. Handle prestige reset if applicable (delete non-persistent records)
7. Release lock, return full state response

**Key observation:** Every request loads the entire game state (8 DB queries), processes idle progress, and returns the full state. There is no incremental/delta protocol. The per-user mutex prevents concurrent action processing but only within a single server process.

### 3.5 Game Engine Design

The engine (`engine.Engine`) is a stateless struct with no stored state. All game logic operates on passed-in models, mutates them in place, and returns results as `ActionResult` structs.

**Idle Progress Calculation** (`ProcessIdleProgress`):
- Computes hardware income (with component upgrade bonuses as percentage of base)
- Recalculates heat, cooling capacity, power limits, and used slots from current data
- Applies multiplier stack: `coloMultiplier * idleMultiplier * heatPenalty * throttleMultiplier`
- Adds knowledge boost (`1 + knowledgePoints/100`)
- Adds hardware-specific bonuses (UPS flat compute, switch income%, NAS reputation%, patch panel reputation%)
- Deducts business expenses
- Decays customer satisfaction when overheating or throttled
- Rolls for random events (~2% chance per poll, weighted by severity)

**Action Processing** (`ProcessAction`):
- 18 action types supported, including 4 bulk actions
- Each action validates preconditions (tier, cost, capacity) and mutates game state
- Returns an `ActionResult` describing DB writes needed (handler persists them)

**Game Mechanics Constants:**
- 6 progression tiers: coffee_table -> closet_floor -> rack_12u -> rack_24u -> rack_36u -> rack_48u
- Tier upgrade costs: 500 / 5,000 / 25,000 / 100,000 / 500,000 CU (scaled by prestige count)
- Power limits per tier: 500W / 1,250W / 3,750W / 7,500W / 12,500W / 20,000W
- Prestige (colo): available at 48U tier with SaaS unlocked, max 100 colocations
- Prestige multiplier formula: `1 + sum(0.5 / (1 + i*0.1))` for each colo
- Prestige cost scaling: linear (1 + count*0.5) for count <= 5, then exponential (3.5 * 1.5^(count-5))

### 3.6 Catalog System

Game items are defined as Go structs in `internal/game/catalog/`. These are static, compile-time definitions -- there is no admin interface or database-driven catalog.

| Catalog | Count | Key fields |
|---------|-------|------------|
| Hardware | 23 items | Name, Type, MinTier, SlotsUsed/RackUnitsUsed, PowerDraw, ComputePerTick, Cost |
| Services | 26 items | Name, Type, MinTier, Compute/Rep/Money per tick, PowerRequired, Cost |
| SaaS Services | 9 items | DeployCost, ReputationRequired, RevenuePerCustomer, MaxCustomers |
| Cooling Upgrades | 7 items | Cooling capacity values from 75 to 3,750 |
| Network Upgrades | 4 items | Network tiers 1-4 |
| Automation Upgrades | 4 items | Idle multipliers 1.2x to 3.0x |
| Knowledge Upgrades | 6 items | Knowledge points 10 to 40 (cost in money, not compute) |
| Component Upgrades | 4 types | CPU/RAM/Storage/NIC, max level 3-5, exponential cost scaling |
| Random Events | ~25 events | 6 categories, severity-weighted, some mitigatable |

### 3.7 API Surface

All routes use the Go 1.22+ `"METHOD /path"` routing syntax.

**Public routes:**
| Method | Path | Handler | Rate Limit |
|--------|------|---------|-----------|
| GET | `/health` | Inline | None |
| POST | `/api/auth/register` | AuthHandler.Register | 10/min/IP |
| POST | `/api/auth/login` | AuthHandler.Login | 10/min/IP |
| GET | `/api/game/config` | GameHandler.GetConfig | None (1h cache) |
| GET | `/ws` | Hub.HandleConnect | None (token in query param) |

**Authenticated routes (JWT Bearer):**
| Method | Path | Handler | Rate Limit |
|--------|------|---------|-----------|
| GET | `/api/game/state` | GameHandler.GetState | None |
| POST | `/api/game/action` | GameHandler.PerformAction | 7,200/min/user |
| GET | `/api/social/group` | SocialHandler.GetMyGroup | None |
| GET | `/api/social/groups` | SocialHandler.ListGroups | None |
| POST | `/api/social/group/create` | SocialHandler.CreateGroup | 180/min/user |
| POST | `/api/social/group/join` | SocialHandler.JoinGroup | 180/min/user |
| POST | `/api/social/group/leave` | SocialHandler.LeaveGroup | 180/min/user |
| POST | `/api/social/group/promote` | SocialHandler.PromoteMember | 180/min/user |
| POST | `/api/social/group/kick` | SocialHandler.KickMember | 180/min/user |
| GET | `/api/social/leaderboard` | SocialHandler.GetLeaderboard | None |
| POST | `/api/social/leaderboard/update` | SocialHandler.UpdateLeaderboards | None |

**Global middleware chain** (applied in order): CORS -> JSON Content-Type -> MaxBodySize (64KB)

**Action types** accepted by POST `/api/game/action`:
`run_job`, `buy_hardware`, `sell_hardware`, `deploy_service`, `buy_upgrade`, `upgrade_component`, `resolve_event`, `unlock_saas`, `deploy_saas`, `upgrade_tier`, `colo`, `bulk_upgrade_components`, `bulk_deploy_services`, `bulk_buy_upgrades`, `bulk_deploy_saas`, `donate_cu`, `build_datacenter`, `upgrade_datacenter`

### 3.8 Authentication

- **Implemented:** Email/password only (bcrypt hashing, 8-128 char passwords)
- **Not implemented:** OAuth2 (Google, Apple, Discord) -- the `User` model has `OAuthProvider`/`OAuthID` fields and the schema supports them, but no OAuth handlers or flows exist
- **Token:** JWT HS256, 24-hour expiry, contains only `user_id`
- **Display name validation:** Alphanumeric only, 2-20 chars, profanity word blocklist, URL pattern blocking
- **Session management:** Stateless JWT; no refresh tokens, no session revocation, no token rotation

### 3.9 WebSocket System

The WebSocket system is server-push only -- used to deliver real-time event notifications from the server to connected clients. Clients do not send game-state messages over the WebSocket.

- **Authentication:** JWT token passed as query parameter during upgrade
- **Connection management:** Single connection per user (new connections close old ones)
- **Keepalive:** Server pings every 30s, expects pong within 45s
- **Reconnection:** Client-side auto-reconnect after 5s delay
- **Message format:** `{ "type": "event", "payload": <JSON> }`
- **Origin checking:** Hardcoded allowlist (production domains + localhost:3000 + local IP)

### 3.10 Rate Limiting

In-memory sliding window implementation. NOT distributed -- will not work correctly with multiple server instances.

- **IP-based:** Used for auth routes (10/min)
- **User-based:** Used for game actions (7,200/min = 120/sec) and social actions (180/min)
- **Cleanup:** Background goroutine purges stale entries every 60s
- **Client IP extraction:** Checks `X-Forwarded-For` (first entry), then `X-Real-IP`, then `RemoteAddr`

## 4. Database Architecture

### 4.1 Technology

- **PostgreSQL** with **TimescaleDB** extension
- **Driver:** `pgx/v5` with connection pooling (max 20 connections, min 2)
- **No ORM:** All queries are hand-written SQL in `queries/` package

### 4.2 Schema

**Core tables:**

| Table | Purpose | Key columns |
|-------|---------|-------------|
| `users` | Player accounts | UUID PK, email (unique), password_hash, display_name, oauth fields |
| `game_states` | One per user -- all game state | UUID PK, user_id (unique FK), ~33 columns covering all game state |
| `hardware` | Owned hardware items | FK to game_states, name, type, tier, compute/power stats |
| `services` | Deployed services | FK to game_states, name, type, compute/rep/money per tick |
| `upgrades` | Purchased upgrades | FK to game_states, name, type, persistent flag |
| `component_upgrades` | Per-hardware component levels | FK to hardware, component, level, bonuses. Unique(hardware_id, component) |
| `customers` | SaaS customers | FK to game_states, name, service_type, revenue, satisfaction |
| `expenses` | Recurring business costs | FK to game_states, name, type, cost_per_tick |
| `colo_racks` | Prestige'd racks | FK to users (not game_states), datacenter_tier, income rates |
| `groups` | Player groups/guilds | UUID PK, name (unique), founder_id FK |
| `group_members` | Group membership | Composite PK (group_id, user_id), role |
| `leaderboard_entries` | Score tracking | user_id FK, category, score, rank. Indexed by (category, score DESC) |

**TimescaleDB hypertables:**

| Table | Purpose | Status |
|-------|---------|--------|
| `resource_history` | Resource accumulation over time | Schema exists, NOT actively written to by application code |
| `event_log` | Event tracking | Schema exists, NOT actively written to by application code |

**Migration management:** Manual SQL files in `internal/database/migrations/`. No migration tool or version tracking table -- files are applied manually via `psql`. Nine migrations have been created (001-009), with one wipe script.

### 4.3 Data Access Patterns

All data access goes through query structs in `internal/database/queries/`. Each struct wraps a `*pgxpool.Pool` and provides typed methods.

**Per-request query load (GetState/PerformAction):**
1. `game_states` -- get by user_id
2. `hardware` -- get by game_state_id
3. `services` -- get by game_state_id
4. `upgrades` -- get by game_state_id
5. `customers` -- get by game_state_id
6. `expenses` -- get by game_state_id
7. `colo_racks` -- get by user_id
8. `component_upgrades` -- get by game_state_id

This is 8 sequential SELECT queries on every request, followed by 1-N writes. There are no JOINs in the game state loading path -- each table is queried independently.

**Notable query:** `GetGlobalDonatedCU` runs `SUM(total_donated_cu)` across all `game_states` on every game state request. This will become expensive as user count grows.

### 4.4 Concurrency Control

- **Per-user in-memory mutex:** `userMutexMap` in `game.go` prevents concurrent action processing for the same user
- **No database-level locking:** No `SELECT ... FOR UPDATE`, no advisory locks, no optimistic concurrency (version columns)
- **Single-process assumption:** The mutex is in-process only; running multiple server instances would allow concurrent state mutation for the same user

## 5. Desktop Client Architecture

### 5.1 Technology Stack

- **Shell:** Tauri 2.10.3 (Rust) -- minimal scaffold, no custom Tauri commands
- **UI:** React 19.2 + TypeScript 5.9
- **State management:** Zustand 5.0
- **Styling:** Tailwind CSS 4.2 (via Vite plugin)
- **Bundler:** Vite 8.0
- **No router:** Single-page application with tab-based navigation, no URL routing

### 5.2 Application Structure

```
src/
  main.tsx              # React root mount
  App.tsx               # Root component (auth gate + tab layout)
  api.ts                # HTTP client, all TypeScript interfaces, API methods
  vite-env.d.ts         # Vite type declarations
  styles/
    global.css          # CSS custom properties (design tokens), Tailwind imports
  stores/
    gameStore.ts        # Zustand store (single store for all state)
  hooks/
    useWebSocket.ts     # WebSocket connection management
    useIdleTick.ts      # Client-side interpolation (rAF-based smooth counter)
    useConfig.ts        # Config access helpers + prestige scaling formula
  components/
    Login.tsx           # Login/register form
    CurrencyBar.tsx     # Top status bar (compute, reputation, money, power)
    ClickArea.tsx       # "Run a job" click target
    TierProgress.tsx    # Tier upgrade display
    HardwarePanel.tsx   # Hardware catalog + owned items
    ServicePanel.tsx    # Service catalog + deployed services
    UpgradePanel.tsx    # Upgrade catalog (cooling, networking, automation, knowledge)
    SaasPanel.tsx       # SaaS services + customers
    DatacenterPanel.tsx # Datacenter ownership + colo rack display
    DonatePanel.tsx     # CU donation (global pool)
    SocialPanel.tsx     # Groups + leaderboards
    EventLog.tsx        # In-game event notifications
```

### 5.3 State Management

A single Zustand store (`gameStore.ts`) holds all application state:
- `state: GameState | null` -- the full server-returned game state
- `config: GameConfig | null` -- cached game configuration
- `token: string | null` -- JWT token (persisted to localStorage)
- `user` -- logged-in user info
- `events: GameEvent[]` -- recent in-game events (max 10)
- `loading`, `error` -- UI state

**State synchronization pattern:**
1. Client polls `GET /api/game/state` every 5 seconds
2. Server response completely replaces the client-side `state` object
3. Between polls, `useIdleTick` hook uses `requestAnimationFrame` to interpolate currency values for smooth counter animation
4. The interpolation exactly mirrors the server's `ProcessIdleProgress` math (multiplier stacks, heat penalty, colo rack decay, etc.)
5. Optimistic updates are used only for `run_job` (click action) -- the click reward is applied locally before the server confirms

### 5.4 API Client

The `api.ts` module provides a typed HTTP client built on `fetch`. Key behaviors:
- Base URL configured via `VITE_API_URL` env var, defaults to `https://api.homelab.living`
- JWT token automatically attached from `localStorage`
- 401 responses trigger automatic logout and page reload
- WebSocket URL derived by replacing `https://` with `wss://`

### 5.5 Tauri Shell

The Tauri Rust code is a minimal scaffold:
- `main.rs` -- calls `app_lib::run()`
- `lib.rs` -- default Tauri builder with debug-only logging plugin
- No custom Tauri commands, no IPC beyond the webview
- Window config: 800x600, resizable, title "Homelab the Game"
- CSP: null (disabled)
- The desktop client is effectively a web app in a native window; it could run identically in a browser

## 6. Shared Package

The `@homelab-game/shared` package (`packages/shared/`) is intended to provide shared TypeScript types between clients. In practice:

**Current state: STALE.** The types in the shared package do not match the actual API response. The frontend (`apps/desktop/src/api.ts`) defines its own interfaces that accurately reflect the server's JSON responses. The shared package's `GameState` interface uses `camelCase` field names and a `currencies` nested object, while the actual API returns `snake_case` flat fields.

**Contents:**
- `types/game.ts` -- `Tier` enum (correct), `GameState` interface (outdated), `ActionType` enum (outdated), `Currencies` interface (unused)
- `constants/index.ts` -- `HARDWARE_SLOTS`, `POWER_LIMITS` (outdated values), `RACK_SIZES`, colo multiplier constants

The shared package is listed as a workspace dependency of the desktop client but is not actually imported by any component -- the desktop client uses locally defined types in `api.ts`.

## 7. Data Flow

### 7.1 Client-Server Communication

```
Desktop Client                    Backend Server                  PostgreSQL
     |                                 |                              |
     |-- GET /api/game/config -------->|                              |
     |<-- GameConfig (cached 1hr) -----|                              |
     |                                 |                              |
     |-- POST /api/auth/login -------->|                              |
     |<-- JWT token -------------------|                              |
     |                                 |                              |
     |-- WS /ws?token=xxx ------------>|                              |
     |<== WebSocket established ======>|                              |
     |                                 |                              |
     |-- GET /api/game/state --------->|-- 8 SELECTs ---------------->|
     |   (every 5 seconds)             |<-- game data ----------------|
     |                                 |-- ProcessIdleProgress()      |
     |                                 |-- UPDATE game_states ------->|
     |<-- Full game state -------------|                              |
     |                                 |                              |
     |-- POST /api/game/action ------->|-- acquire user mutex         |
     |   { type, payload }             |-- 8 SELECTs ---------------->|
     |                                 |<-- game data ----------------|
     |                                 |-- ProcessIdleProgress()      |
     |                                 |-- ProcessAction()            |
     |                                 |-- INSERT/UPDATE/DELETE ------>|
     |<-- Full game state -------------|-- release user mutex         |
     |                                 |                              |
     |<== WS: event notification ======|  (if event triggered)        |
```

### 7.2 Server-Authoritative Model

All game state mutations are validated server-side. The client sends action requests (e.g., `buy_hardware` with a `name` payload), and the server validates preconditions (tier requirements, cost, capacity) before applying changes. The client has no ability to directly modify game state -- it can only request actions and display the server-returned state.

The client does perform local interpolation between server polls for visual smoothness, but this is display-only and always corrected by the next server response.

## 8. Infrastructure and Deployment

### 8.1 Current Deployment

Based on CORS origins and WebSocket allowlists, the system is deployed to:
- **Backend API:** `api.homelab.living` (HTTPS)
- **Frontend:** `game.homelab.living` (HTTPS)
- **Database:** PostgreSQL on `localhost:5432` (same machine, `sslmode=disable`)
- **Proxy:** Nginx is implied by the WebSocket ping/pong comments and X-Forwarded-For handling

Per project memory, everything runs on the same server -- there are no separate environments.

### 8.2 Configuration

Environment variable based, loaded from `.env` file by custom loader in `main.go`:
- `PORT` (default: 8080)
- `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`
- `JWT_SECRET` (auto-generated random if missing -- WARNING logged)
- `CORS_ORIGINS` (comma-separated additional origins)
- `ENV` (if not "production", allows localhost CORS)
- `VITE_API_URL` (frontend, defaults to `https://api.homelab.living`)

### 8.3 What Does NOT Exist

The following items are mentioned in CLAUDE.md or design documents but do not exist in the codebase:

| Planned Feature | Status |
|----------------|--------|
| Mobile client (React Native) | `apps/mobile/` directory is empty |
| OAuth2 authentication (Google, Apple, Discord) | Model fields exist, no implementation |
| Ads (AdMob) | Not started |
| In-app purchases (IAP) | Not started |
| Push notifications (FCM/APNs) | Not started |
| TimescaleDB analytics writes | Schema exists, no application code writes to hypertables |
| Game state validation / anti-cheat (`internal/game/state/`) | Directory exists but is empty |
| Canvas/Pixi.js visual rack view | Not implemented (UI is HTML/CSS panels) |
| `.documents/` directory (PLAN.md, ROADMAP.md) | Referenced in CLAUDE.md but does not exist in repository |
| Automated tests | Zero test files in the entire codebase |
| Database migration tooling | Migrations are raw SQL files applied manually |

## 9. Cross-Cutting Concerns

### 9.1 Error Handling

- Backend returns JSON error responses: `{"error": "message"}`
- No structured error types or error codes -- all errors are string messages
- Many DB query errors are silently swallowed (e.g., `hw, _ := h.hardware.GetByGameStateID(...)`)
- Client displays errors via a top-level error banner in the UI

### 9.2 Logging

- Backend uses `log.Println`/`log.Printf` (stdlib) -- no structured logging
- WebSocket connection/disconnection events are logged
- Tauri shell has debug-only log plugin
- No request logging middleware

### 9.3 Observability

- Single health check endpoint (`GET /health` returns `{"status":"ok"}`)
- `ws.Hub.ConnectedUsers()` method exists but is not exposed via any endpoint
- No metrics, no tracing, no structured logs, no dashboards
- TimescaleDB hypertables exist for historical analytics but are not written to

### 9.4 Security

- JWT authentication with 24-hour expiry
- bcrypt password hashing (default cost)
- Per-IP and per-user rate limiting (in-memory)
- CORS origin allowlist
- Body size limit (64KB)
- Display name profanity filter and URL blocking
- **Gaps:** No CSRF protection (mitigated by Bearer tokens), no refresh tokens, CSP disabled in Tauri, JWT secret auto-generated in dev, WebSocket token passed in URL query parameter (appears in server logs)

## 10. Architectural Decisions and Tradeoffs

### 10.1 Lazy Tick vs. Background Loop

The server calculates idle progress on each client request rather than running a continuous background tick loop. This eliminates timer management complexity and ensures computation only happens for active players. The tradeoff is that offline progress is calculated in a single burst when the player reconnects, which could cause a noticeable delay for very long offline periods.

### 10.2 Full State Response

Every API response returns the complete game state (including available catalog items filtered by tier). This simplifies the client -- it never needs to merge partial updates -- but increases bandwidth and serialization cost per request. For a game with relatively small state, this is an acceptable tradeoff.

### 10.3 Single-Process Architecture

All state (rate limiter, user mutexes, WebSocket connections) is in-process memory. This constrains the system to a single server instance. This is intentional for a self-hosted homelab deployment and would need significant redesign for horizontal scaling.

### 10.4 No ORM

Hand-written SQL provides full control over queries and avoids ORM abstraction leakage. The cost is verbose query code and manual schema-struct synchronization. For a project of this size, this is a reasonable choice.

### 10.5 Stateless Game Engine

The `Engine` struct has no fields and no stored state. All inputs are passed as function arguments. This makes the engine purely functional in practice -- easy to reason about and test (when tests exist). The tradeoff is large parameter lists.

## 11. Known Architectural Risks

1. **No automated tests.** Zero test coverage across the entire codebase. The game engine's math is complex (multiplier stacks, prestige scaling, component upgrade bonuses) and currently has no verification beyond manual testing and a stress test tool.

2. **Per-request query fan-out.** Every game state request performs 8 sequential database queries. With 5-second polling per connected client, this creates significant DB load per concurrent user. The `GetGlobalDonatedCU` SUM aggregation compounds this.

3. **In-memory concurrency control.** The per-user mutex map and rate limiter are process-local. Running a second server instance (or restarting the process) creates a window for concurrent mutation and rate limit reset.

4. **Silent error swallowing.** Many database operations ignore errors (e.g., customer updates, bulk persist). Failed writes are silently dropped, which could cause state drift between the in-memory model and database.

5. **Shared package drift.** The `@homelab-game/shared` package is stale and unused. Its types will mislead any developer who references them instead of the actual API contract in `api.ts`.

6. **Migration management.** No tooling, no version tracking, no rollback support. Manual `psql` execution with no audit trail.

7. **WebSocket token in URL.** The JWT is passed as a query parameter for WebSocket upgrade, which may appear in server access logs and proxy logs.

## 12. Dependency Summary

### Backend (Go)

| Dependency | Version | Purpose |
|-----------|---------|---------|
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | JWT token handling |
| `github.com/jackc/pgx/v5` | v5.8.0 | PostgreSQL driver + connection pool |
| `github.com/gorilla/websocket` | v1.5.3 | WebSocket support |
| `golang.org/x/crypto` | v0.49.0 | bcrypt password hashing |

### Desktop Client (Node.js)

| Dependency | Version | Purpose |
|-----------|---------|---------|
| `react` | ^19.2.4 | UI library |
| `react-dom` | ^19.2.4 | React DOM renderer |
| `zustand` | ^5.0.12 | State management |
| `tailwindcss` | ^4.2.1 | Utility-first CSS |
| `vite` | ^8.0.0 | Build tool + dev server |
| `@vitejs/plugin-react` | ^6.0.1 | React Vite integration |
| `typescript` | ^5.9.3 | Type checking |

### Desktop Shell (Rust)

| Dependency | Version | Purpose |
|-----------|---------|---------|
| `tauri` | 2.10.3 | Native app shell |
| `tauri-plugin-log` | 2.x | Debug logging |
| `serde` / `serde_json` | 1.0 | Serialization (Tauri requirement) |
