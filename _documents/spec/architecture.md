---
project: "homelab-the-game"
maturity: "experimental"
last_updated: "2026-04-04"
updated_by: "@staff-engineer"
scope: "Comprehensive architecture specification documenting the actual codebase structure, module boundaries, data flow, and integration points"
owner: "@staff-engineer"
dependencies: []
---

# Architecture Specification

## 1. System Overview

Homelab the Game is a server-authoritative AFK/clicker simulation game where players build and manage a homelab, progressing from a Raspberry Pi on a coffee table to colocating racks in datacenters. The game is live at `game.homelab.living` (frontend) and `api.homelab.living` (backend API).

### High-Level Architecture

```
+---------------------+       +--------------------+       +-----------------+
|  Desktop Client     |<----->|  Go Backend        |<----->|  PostgreSQL     |
|  (React + Vite)     | WS/   |  (net/http + gorilla| pgx  |  + TimescaleDB  |
|  Port 3000          | HTTP  |  /websocket)       |       |                 |
+---------------------+       |  Port 8080/8085    |       +-----------------+
                              |                    |
                              |                    |<----->+-----------------+
                              +--------------------+       |  Redis          |
                                                           |  (optional)     |
                                                           +-----------------+
```

The system is a monorepo (`pnpm` workspaces + Go modules) with three application directories and one shared package:

| Directory | Language | Purpose |
|-----------|----------|---------|
| `apps/backend/` | Go 1.25 | Server-authoritative game engine, REST API, WebSocket server |
| `apps/desktop/` | TypeScript + React | Desktop web client (Vite dev server or static build) |
| `apps/mobile/` | TypeScript + React Native | Mobile client (planned, not yet implemented) |
| `packages/shared/` | TypeScript | Shared types and constants (Tier enum, action types, currency interfaces) |

### Core Design Principle

**Server-authoritative**: All game state mutations are validated and executed on the backend. Clients send action requests and receive computed state responses. The client performs optimistic updates only for click feedback (`run_job`) and uses `requestAnimationFrame`-based interpolation for smooth currency counter display between server ticks.

## 2. Backend Architecture (Go)

### 2.1 Entry Point and Initialization

**File**: `apps/backend/cmd/server/main.go`

The server bootstraps in this order:

1. Load `.env` file (hand-rolled parser, not a library)
2. Load configuration from environment variables (`internal/config/config.go`)
3. Connect to PostgreSQL via `pgxpool` (max 50 connections, min 5)
4. Connect to Redis (optional -- graceful degradation if unavailable)
5. Wire Redis-backed services when available:
   - Rate limiting store
   - WebSocket message broadcaster (pub/sub)
   - Bitcoin price leader election
   - Global donated CU cache
6. Instantiate all query objects (one per table/concern)
7. Instantiate the game engine, WebSocket hub, Bitcoin price service
8. Wire hub lifecycle callbacks (`OnConnect`, `OnDisconnect`, `OnMessage`)
9. Set up HTTP routes
10. Start HTTP server with graceful shutdown (10-second grace period on SIGTERM/SIGINT)

A secondary entry point exists at `cmd/healthcheck/main.go` for container health probes.

### 2.2 Package Dependency Graph

```
cmd/server/main.go
  |
  +-- internal/config           (Config struct, env loading, Docker secrets)
  +-- internal/database          (pgxpool.Connect)
  +-- internal/database/queries  (per-table query objects + batch loader)
  +-- internal/api/handlers      (AuthHandler, GameHandler, SocialHandler)
  +-- internal/api/middleware     (CORS, Auth, RateLimit, BodyLimit, JSON)
  +-- internal/api/routes        (http.ServeMux wiring)
  +-- internal/api/ws            (Hub, Client, MessageBroadcaster)
  +-- internal/auth              (JWT generation/validation, bcrypt password hashing)
  +-- internal/game/engine       (ProcessIdleProgress, ProcessAction, GetConfig)
  +-- internal/game/catalog      (hardware/services/upgrades/saas/research definitions)
  +-- internal/game/events       (random event system)
  +-- internal/game/bitcoin      (price simulation, leader election)
  +-- internal/models            (GameState, Hardware, Service, Upgrade, etc.)
```

**Key constraints**:
- `internal/game/engine` depends on `internal/game/catalog`, `internal/game/events`, and `internal/models` -- it is pure game logic with no database or HTTP awareness.
- `internal/api/handlers` is the integration layer that orchestrates database queries, engine calls, and WebSocket pushes.
- `internal/models` has zero dependencies (leaf package).

### 2.3 Module Details

#### `internal/config/`

Single file. Reads configuration from environment variables with fallbacks. Supports Docker secrets (reads from `/run/secrets/<name>` when env var is absent). Generates random JWT secret in dev mode with a loud warning.

Key config fields: `Port`, `DBHost/DBPort/DBUser/DBPass/DBName`, `JWTSecret`, `RedisAddr/RedisPassword/RedisDB`, `RegistrationEnabled`.

#### `internal/models/`

Pure data types with JSON tags. No methods, no database awareness. The `GameState` struct has 30+ fields representing all player state. Supporting models: `Hardware`, `Service`, `Upgrade`, `ComponentUpgrade`, `Customer`, `Expense`, `ColoRack`, `ResearchLevel`, `User`, `Group`, `GroupMember`, `BitcoinPricePoint`.

The `Tier` type is a `string` enum: `coffee_table`, `closet_floor`, `rack_12u`, `rack_24u`, `rack_36u`, `rack_48u`.

#### `internal/auth/`

- `jwt.go`: Token generation and validation using `github.com/golang-jwt/jwt/v5`. Tokens contain a `user_id` claim and expire in 7 days.
- `password.go`: bcrypt hashing and verification via `golang.org/x/crypto/bcrypt`.

#### `internal/database/`

- `db.go`: Thin wrapper around `pgxpool.NewWithConfig`. Pool config: max 50 conns, min 5.
- `queries/`: One file per table concern. Each file defines a struct holding a `*pgxpool.Pool` and methods for CRUD operations. Uses raw SQL queries via `pgx` (no ORM).
  - `batch.go`: `LoadFullGameState` and `LoadFullGameStateForUpdate` -- loads the complete game state for a user in 2 database round-trips (1 single-row lookup + 1 `pgx.Batch` of 8 queries). The `ForUpdate` variant acquires a `FOR UPDATE` row lock on `game_states` for transactional safety during write operations.
  - `game_state.go`, `hardware.go`, `services.go`, `upgrades.go`, `customers.go`, `expenses.go`, `colo.go`, `groups.go`, `leaderboard.go`, `bitcoin.go`, `research.go`.

#### `internal/api/middleware/`

Standard HTTP middleware chain applied in this order (outermost first):

1. **CORS** (`cors.go`): Allows specific origins (`game.homelab.living`, `dev-game.homelab.living`, `localhost:3000` in dev). Handles preflight OPTIONS.
2. **JSON** (`json.go`): Sets `Content-Type: application/json` header.
3. **MaxBodySize** (`bodylimit.go`): Limits request body to prevent abuse.
4. **Auth** (`auth.go`): Extracts JWT from `Authorization: Bearer <token>` header, validates it, and injects `user_id` into request context via `context.Value`.
5. **RateLimit** (`ratelimit.go`): Per-IP and per-user rate limiting. Backed by in-memory sliding window or Redis when available (`redis_ratelimit.go`).

#### `internal/api/routes/`

Single file wiring a `net/http.ServeMux` with Go 1.22+ method-aware patterns. Route groups:

| Pattern | Auth | Rate Limit | Handler |
|---------|------|------------|---------|
| `GET /health` | No | No | Inline health check |
| `POST /api/auth/register` | No | 10/min/IP | `AuthHandler.Register` |
| `POST /api/auth/login` | No | 10/min/IP | `AuthHandler.Login` |
| `GET /ws` | Token query param | No | `Hub.HandleConnect` |
| `GET /api/game/config` | No | No | `GameHandler.GetConfig` |
| `GET /api/game/state` | Yes | No | `GameHandler.GetState` |
| `POST /api/game/action` | Yes | 7200/min/user | `GameHandler.PerformAction` |
| `GET /api/social/group` | Yes | No | `SocialHandler.GetMyGroup` |
| `GET /api/social/groups` | Yes | No | `SocialHandler.ListGroups` |
| `POST /api/social/group/*` | Yes | 180/min/user | `SocialHandler.Create/Join/Leave/Promote/Kick` |
| `GET /api/social/leaderboard` | Yes | No | `SocialHandler.GetLeaderboard` |
| `POST /api/social/leaderboard/update` | Yes | No | `SocialHandler.UpdateLeaderboards` |

#### `internal/api/ws/`

WebSocket subsystem using `gorilla/websocket`.

- **Hub** (`hub.go`): Manages `map[string]*Client` (userID to client). Enforces single connection per user (new connection evicts old). Lifecycle callbacks: `OnConnect(userID, done)`, `OnDisconnect(userID)`, `OnMessage(userID, data)`.
- **Client**: Each client has a `send chan []byte` (capacity 16) and a `done chan struct{}`. Two goroutines per client: `writePump` (reads from send channel, writes to conn, sends pings every 30s) and `readPump` (reads from conn, routes to `OnMessage`, detects disconnect).
- **MessageBroadcaster** (`pubsub.go`): Interface abstracting message delivery. Two implementations:
  - `LocalBroadcaster`: Delegates directly to Hub (single-replica mode).
  - `RedisBroadcaster` (`redis_broadcaster.go`): Publishes to Redis `ws:broadcast` channel. Fast path: delivers locally if user is on this replica. Slow path: publishes to Redis for cross-replica delivery.

**Connection parameters**: ping interval 30s, pong timeout 45s, write deadline 10s, max read message 64KB.

**Authentication**: WebSocket connections authenticate via JWT passed as a `?token=` query parameter (not via HTTP headers, since the browser WebSocket API does not support custom headers).

#### `internal/api/handlers/`

Three handler structs:

**AuthHandler** (`auth.go`):
- `Register`: Email/password registration with validation (email format, display name 2-20 chars alphanumeric, profanity filter, URL pattern filter). Creates user + initial game state in one flow. Returns JWT.
- `Login`: Email/password authentication. Returns JWT.

**GameHandler** (`game.go`): The largest and most complex handler. Key responsibilities:

- **State retrieval** (`GetState`, `GetConfig`): Loads full game state, runs idle progress, returns enriched response with available catalogs.
- **Action processing** (`PerformAction` for HTTP, `HandleWSAction` for WebSocket): Both paths share the same `processAction` flow -- acquire per-user mutex, load state via `LoadFullGameStateForUpdate` (transactional), run `engine.ProcessAction`, persist results, push updated state.
- **Tick system** (`OnConnect`, `runUserTick`, `runFullTick`, `runLightTick`): When a user connects via WebSocket, a per-user goroutine starts that ticks every 5 seconds (configurable via `TICK_INTERVAL_SECONDS` env var). Each tick runs idle progress and pushes the full state over WebSocket. Two tick modes:
  - **Full tick**: Loads all data from DB, runs engine, persists everything. Runs when the user has performed an action since last tick (`dirty` flag) or on first tick.
  - **Light tick**: Reuses cached child data (hardware, services, etc.), runs engine on in-memory state, persists only the `game_states` row. Runs when the user is idle.
- **Per-user mutex** (`userMutexMap`): In-memory per-user locks preventing concurrent state mutations between tick goroutines and action handlers. Stale locks cleaned up every 5 minutes.
- **Global Donated CU Cache** (`cu_cache.go`): Periodically refreshes `SUM(total_donated_cu)` from the database to avoid per-request full table scans. Uses Redis for cross-replica consistency when available. Background refresh interval: 30 seconds.

**SocialHandler** (`social.go`): Group management (create, join, leave, promote, kick) and leaderboard operations. Groups provide compute bonuses (+5% per member, capped at +50%).

### 2.4 Game Engine (`internal/game/engine/`)

The engine is a pure-logic module with no I/O dependencies.

**`engine.go`** contains two critical functions:

1. **`ProcessIdleProgress`**: Called every tick (5s) for connected users. Computes resource generation based on elapsed time, owned hardware/services, active multipliers, and research bonuses. Also rolls for random events, decays throttle/overclock timers, and recalculates derived stats (heat, cooling, power, slots). Returns triggered events.

   Multiplier stack (all multiplicative):
   - `ColoMultiplier` (prestige bonus)
   - `IdleMultiplier` (automation upgrades)
   - Heat penalty (0.5x if overheating)
   - Event throttle
   - Overclock multiplier (time-weighted average for partial periods)
   - Knowledge boost (+1% per knowledge point, additive)
   - Network bonus (from switch hardware, additive)
   - Research bonuses (per effect type: idle_income, reputation_gain, money_income)

2. **`ProcessAction`**: Dispatches game actions by type string. 27 distinct action types:

   | Action | Category | Description |
   |--------|----------|-------------|
   | `run_job` | Click | Manual compute generation (click reward) |
   | `buy_hardware` | Purchase | Buy a hardware item from the catalog |
   | `sell_hardware` | Sell | Sell owned hardware for 60% refund |
   | `deploy_service` | Purchase | Deploy a service |
   | `buy_upgrade` | Purchase | Buy an upgrade (cooling/networking/automation/knowledge/misc) |
   | `upgrade_component` | Purchase | Upgrade a hardware component (CPU/RAM/NIC/SSD) |
   | `resolve_event` | Event | Pay to resolve a throttle event |
   | `unlock_saas` | Progression | Unlock the SaaS system |
   | `deploy_saas` | Purchase | Deploy a SaaS service |
   | `upgrade_tier` | Progression | Upgrade to the next tier |
   | `colo` | Prestige | Prestige reset (colo a rack) |
   | `donate_cu` | Social | Donate compute units to the global pool |
   | `build_datacenter` | Progression | Build a datacenter |
   | `upgrade_datacenter` | Progression | Upgrade datacenter level |
   | `buy_bitcoin` / `buy_max_bitcoin` | Market | Purchase bitcoin with compute |
   | `sell_bitcoin` / `sell_all_bitcoin` | Market | Sell bitcoin for money |
   | `activate_overclock` | Boost | Temporarily boost production |
   | `buy_research` / `bulk_buy_research` | Research | Purchase research levels |
   | `optimize_rack` | Progression | Increase rack efficiency |
   | `bulk_*` variants | Convenience | Batch purchase operations |

**`config.go`**: Defines the `GameConfig` struct and the `GetConfig()` function that assembles the full game configuration from authoritative engine sources. This config is served to clients via `GET /api/game/config` so they can display costs, tier requirements, and bonus values without hardcoding them.

### 2.5 Game Catalog (`internal/game/catalog/`)

Static data definitions for all purchasable/deployable items. Each catalog file defines:
- A `[]Template` slice with all items and their stats
- Helper functions: `GetByName`, `GetAvailable(tier)` (filters by min tier)
- Lookup maps for bonus values

| File | Items | Key Mechanic |
|------|-------|--------------|
| `hardware.go` | 22 hardware items across 6 tiers | Compute generation, power draw, slot/rack-unit consumption |
| `services.go` | 22 services across 6 tiers | Compute, reputation, money generation |
| `upgrades.go` | 4 upgrade categories (cooling, networking, automation, knowledge) + misc | Capacity increases, multipliers, prestige persistence |
| `saas.go` | 9 SaaS services across 4 tiers | Customer-driven money generation |
| `research.go` | 10 research nodes in 4 branches | Infinite levels with exponential cost, multiplicative bonuses |

### 2.6 Event System (`internal/game/events/`)

Random events fire during idle progress ticks with ~2% chance per tick. Events are categorized by tier (coffee_table, closet_floor, rack, software, saas, colo) with weighted severity (minor:3, moderate:2, major:1 weight ratio).

Events can:
- Reduce resources (compute, reputation, money)
- Apply throttle multiplier for N ticks
- Be mitigated by owning specific upgrades or hardware types

31 distinct events are defined across 6 categories.

### 2.7 Bitcoin System (`internal/game/bitcoin/`)

A simulated bitcoin market using an Ornstein-Uhlenbeck mean-reverting price model.

- **PriceService** (`price.go`): Computes price steps using a seeded PRNG for determinism. Parameters: mean price 10,000, min 1,000, max 50,000, step interval 5s, theta 0.02, sigma 2,000.
- **PriceLeader** (`leader.go`): Redis-based leader election ensuring only one backend replica advances the price simulation. Uses Redis SET NX with 15-second TTL, renewed every 5 seconds.
- **PriceStore interface**: Abstracts database operations for testability. Adapter pattern used in `main.go` to bridge `queries.BitcoinQueries` to the `bitcoin.PriceStore` interface.

## 3. Frontend Architecture (Desktop Client)

### 3.1 Technology Stack

| Concern | Choice |
|---------|--------|
| Framework | React 19 |
| State management | Zustand (single store) |
| Styling | Tailwind CSS (via `@tailwindcss/vite` plugin) |
| Bundler | Vite |
| Desktop shell | Tauri (Rust) -- defined but not actively used for development |
| Package manager | pnpm |

### 3.2 Module Structure

```
apps/desktop/src/
  main.tsx                    -- React DOM root mount
  App.tsx                     -- Root component, auth gate, tab layout
  api.ts                      -- REST API client, type definitions (350+ LOC)
  wsClient.ts                 -- WebSocket client singleton
  stores/
    gameStore.ts              -- Zustand store (single source of truth)
  hooks/
    useWebSocket.ts           -- WebSocket lifecycle hook
    useIdleTick.ts            -- Client-side interpolation for smooth counters
    useConfig.ts              -- Config fetching hook
  components/
    Login.tsx                 -- Auth form
    CurrencyBar.tsx           -- Top-level currency display
    ClickArea.tsx             -- Manual job execution (click target)
    TierProgress.tsx          -- Tier upgrade progress indicator
    HardwarePanel.tsx         -- Hardware catalog and owned items
    ServicePanel.tsx          -- Service deployment
    UpgradePanel.tsx          -- Upgrade purchase
    ResearchPanel.tsx         -- Research tree
    SaasPanel.tsx             -- SaaS service management
    DatacenterPanel.tsx       -- Datacenter building/upgrading
    MarketPanel.tsx           -- Bitcoin trading
    SocialPanel.tsx           -- Groups and leaderboards
    OverclockPanel.tsx        -- Overclock activation
    DonatePanel.tsx           -- CU donation
    EventLog.tsx              -- Event notification display
    shared/
      CurrencyValue.tsx       -- Formatted currency display
      CurrencyStatLine.tsx    -- Label + value stat line
  utils/
    currencyColors.ts         -- Color theming for currency types
```

### 3.3 State Management

The `gameStore.ts` Zustand store is the single source of truth for all client state:

- `state: GameState | null` -- full game state from server
- `config: GameConfig | null` -- static game configuration
- `token: string | null` -- JWT auth token (persisted in `localStorage`)
- `user` -- authenticated user info
- `events: GameEvent[]` -- event notification queue (last 10)
- `loading`, `error` -- UI state

Every game action (30+ action methods) follows the same pattern:
1. Clear error state
2. Send action via `wsClient.sendAction(actionType, payload)` (WebSocket preferred)
3. Set returned state on success, set error on failure

**Exception**: `runJob` uses optimistic update -- it immediately adds the estimated click reward to `compute_units` locally for instant feedback, then reconciles with the server response.

### 3.4 API Client (`api.ts`)

REST client using `fetch`. All requests include `Content-Type: application/json` and the JWT `Authorization` header when available. On 401 response, automatically clears token and reloads the page.

The file also serves as the **canonical TypeScript type definitions** for the entire client. The `GameState` interface (80+ fields) and `GameConfig` interface mirror the server response shapes. These types are defined inline rather than imported from `packages/shared/` (the shared package has an older, simpler type definition that is now outdated).

### 3.5 WebSocket Client (`wsClient.ts`)

Singleton `WSClient` class managing the WebSocket connection:

- **Connection**: `wss://api.homelab.living/ws?token=<jwt>` (derived from `VITE_API_URL`)
- **Action protocol**: Client sends `{type: "action", id: <uuid>, action: <type>, payload: <data>}`. Server responds with `{type: "action_result", id: <uuid>, success: bool, state?: GameState, error?: string}`. Uses a `Map<string, PendingRequest>` for request/response correlation with 10-second timeout.
- **Server push**: Server sends `{type: "state", payload: <GameState>}` every 5 seconds (tick interval). Also sends `{type: "event", payload: <GameEvent>}` for random events.
- **Fallback**: If WebSocket is not connected, actions fall back to HTTP POST `/api/game/action`.
- **Reconnection**: Handled by `useWebSocket` hook -- 5-second reconnect delay on close.

### 3.6 Client-Side Interpolation (`useIdleTick.ts`)

The `useIdleTick` hook provides smooth currency counter animation between server ticks:

1. When server state arrives, it calculates per-second production rates by replicating the server engine's multiplier stack (heat penalty, throttle, overclock, knowledge, network/storage bonuses, research, colo rack income, group bonus, expenses).
2. A `requestAnimationFrame` loop interpolates currency values: `displayed = serverBase + rate * elapsedSinceLastPush`.
3. On each server push, the base values and rates are recalculated, preventing drift.

This is a read-only mirror of the server's `ProcessIdleProgress` logic -- it never modifies actual game state.

### 3.7 UI Layout

The App component implements a fixed layout:
- **Header**: Game title + logout button
- **Currency Bar**: Always-visible currency display (compute, reputation, money, power, heat, bitcoin)
- **Left Sidebar** (fixed 288px): Click area, tier progress, donate panel, overclock panel
- **Right Content** (flexible): Tabbed panels -- Hardware, Services, Upgrades, Research, SaaS, Datacenter, Market, Social

The Market tab conditionally appears only when the player has money or bitcoin.

## 4. Shared Package (`packages/shared/`)

Minimal shared TypeScript package defining:
- `Tier` enum (6 tiers)
- `Currencies` interface
- `GameState` interface (simplified, outdated relative to `api.ts`)
- `ActionType` enum
- `GameAction` interface

**Gap**: The shared package types have drifted from the actual types used by the desktop client. The client's `api.ts` is the de facto type authority. The shared package's `GameState` interface is missing many fields that exist in the server response (overclock, research, bitcoin, SaaS, datacenter, etc.).

## 5. Data Layer

### 5.1 Database Schema

PostgreSQL 16 with TimescaleDB extension. Database: `homelab_game`, user: `homelab_game`.

**Core tables** (14 migrations applied):

| Table | Key | Relationships | Purpose |
|-------|-----|---------------|---------|
| `users` | UUID PK | -- | Authentication (email/password or OAuth) |
| `game_states` | UUID PK, unique `user_id` | FK to `users` | All game state (30+ columns) |
| `hardware` | UUID PK | FK to `game_states` | Owned hardware items |
| `services` | UUID PK | FK to `game_states` | Deployed services |
| `upgrades` | UUID PK | FK to `game_states` | Purchased upgrades |
| `component_upgrades` | UUID PK | FK to `hardware` | Per-hardware component upgrades |
| `customers` | UUID PK | FK to `game_states` | SaaS customers |
| `expenses` | UUID PK | FK to `game_states` | Recurring costs |
| `colo_racks` | UUID PK | FK to `users` (not game_states!) | Prestige racks (persist through reset) |
| `groups` | UUID PK | FK to `users` (founder) | Player groups |
| `group_members` | Composite PK | FK to `groups` + `users` | Group membership |
| `leaderboard_entries` | UUID PK | FK to `users` | Materialized leaderboard scores |
| `resource_history` | TimescaleDB hypertable | FK to `users` | Historical resource tracking |
| `event_log` | TimescaleDB hypertable | FK to `users` | Event occurrence history |
| `bitcoin_price` | Singleton row | -- | Current global bitcoin price state |
| `bitcoin_price_history` | TimescaleDB hypertable | -- | Historical price data for charts |
| `research_levels` | UUID PK | FK to `game_states` | Per-node research levels |

**Critical design note**: `colo_racks` references `users.id` (not `game_states.id`) because colo racks persist through the prestige reset that deletes/recreates game state child rows.

### 5.2 Query Pattern

All database access uses raw SQL via `pgx`. No ORM. Two patterns:

1. **Individual queries**: Standard CRUD via per-table query objects (`queries.NewXxxQueries(pool)`).
2. **Batch loading** (`queries/batch.go`): The hot path. Loads the entire game state for a user in exactly 2 database round-trips:
   - Round-trip 1: `SELECT ... FROM game_states WHERE user_id = $1` (optionally `FOR UPDATE`)
   - Round-trip 2: `pgx.Batch` of 8 queries for all child tables, sent as a single network message

The `FOR UPDATE` variant is used during action processing to provide database-level serialization, preventing concurrent transactions from reading stale state.

### 5.3 Indexing

Migration `014_add_game_state_indexes.sql` adds performance indexes. The batch query pattern means most reads hit `game_state_id` foreign key indexes on child tables.

## 6. Real-Time Communication Protocol

### 6.1 WebSocket Message Types

**Client to Server**:
```json
{
  "type": "action",
  "id": "<uuid>",
  "action": "<action_type>",
  "payload": { ... }
}
```

**Server to Client**:

| Type | Trigger | Payload |
|------|---------|---------|
| `action_result` | Response to client action | `{type, id, success, state?, error?}` |
| `state` | Every tick (5s) | Full `GameState` object |
| `event` | Random event triggered | `GameEvent` object |

### 6.2 Connection Lifecycle

1. Client authenticates via REST (`POST /api/auth/login`)
2. Client opens WebSocket: `wss://api.homelab.living/ws?token=<jwt>`
3. Server validates JWT, registers client in Hub (evicting any prior connection for same user)
4. Server starts per-user tick goroutine (5-second interval)
5. On each tick: load state, run `ProcessIdleProgress`, persist, push full state via WebSocket
6. On client action: action processed synchronously (per-user mutex), result sent back with matching `id`
7. On disconnect: tick goroutine stopped via `done` channel, client removed from Hub

### 6.3 Multi-Replica Support

The WebSocket layer is designed for horizontal scaling via Redis:

- **MessageBroadcaster interface**: Abstracts whether messages go directly to local Hub or via Redis pub/sub
- **RedisBroadcaster**: Fast path (local delivery) + slow path (Redis publish). Subscribes to `ws:broadcast` channel
- **Bitcoin PriceLeader**: Redis-based leader election (SET NX with TTL) ensures only one replica advances the price model

The Docker Swarm stack (`docker-stack.yml`) deploys 2 backend replicas behind Traefik with start-first rolling updates.

## 7. Configuration and Deployment

### 7.1 Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `PORT` | `8080` | Backend HTTP port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `homelab_game` | Database user |
| `DB_PASSWORD` | (secret) | Database password |
| `DB_NAME` | `homelab_game` | Database name |
| `DB_SSLMODE` | `disable` | PostgreSQL SSL mode |
| `JWT_SECRET` | (random in dev) | JWT signing key |
| `REDIS_ADDR` | `redis:6379` | Redis address |
| `REDIS_PASSWORD` | (secret) | Redis password |
| `REGISTRATION_ENABLED` | `true` | Feature flag for user registration |
| `ENV` | (unset) | Set to `production` to restrict CORS origins |
| `CORS_ORIGINS` | (unset) | Additional allowed CORS origins (comma-separated) |
| `TICK_INTERVAL_SECONDS` | `5` | Server tick interval |
| `VITE_API_URL` | `https://api.homelab.living` | Frontend API base URL |

### 7.2 Infrastructure

Everything runs on a single VM (homelab server). No separate dev/staging/prod environments.

- **Reverse proxy**: External nginx (not on this VM) terminates TLS and proxies `api.homelab.living` to the backend port and `game.homelab.living` to the Vite dev server (port 3000).
- **Docker Swarm**: `docker-stack.yml` defines the containerized deployment with Traefik as internal load balancer, 2 backend replicas, PostgreSQL, Redis, and a migrations container.
- **Dev mode**: Backend runs directly via `go run ./cmd/server/`, frontend via `pnpm dev`. No containerization required.

### 7.3 Dev URLs

- Production: `game.homelab.living` / `api.homelab.living`
- Development: `dev-game.homelab.living` / `dev-api.homelab.living` (same machine, different vhost)

## 8. Cross-Cutting Concerns

### 8.1 Authentication

- **Method**: Email/password with bcrypt hashing (cost factor default)
- **Token**: JWT with 7-day expiry, `user_id` claim
- **REST API**: `Authorization: Bearer <token>` header
- **WebSocket**: `?token=<jwt>` query parameter
- **OAuth**: Schema supports `oauth_provider`/`oauth_id` fields but OAuth flows are not implemented yet

### 8.2 Anti-Cheat / Server Authority

All game state mutations go through the server:
1. Client sends action request (type + payload)
2. Server loads current state with `FOR UPDATE` lock
3. Engine validates action (checks costs, tier requirements, slot availability, etc.)
4. Server persists results in a transaction
5. Server returns the authoritative state

The per-user mutex (`userMutexMap`) prevents concurrent action processing, and the database `FOR UPDATE` lock prevents concurrent transaction reads.

### 8.3 Rate Limiting

Two layers:
- **Per-IP**: Auth endpoints limited to 10 requests/minute
- **Per-user**: Game actions limited to 7200/minute (120/second) via HTTP, plus WebSocket-level rate checking in `HandleWSAction`

Rate limit storage: in-memory sliding window by default, Redis-backed when available for cross-replica consistency.

### 8.4 Error Handling

- **Game logic errors** (insufficient resources, tier requirements not met): Returned to client with descriptive message
- **Internal errors** (database failures, unexpected panics): Masked as "internal server error" for the client, logged server-side with full details
- **WebSocket panic recovery**: `HandleWSAction` has a `defer/recover` that catches panics, releases the per-user mutex, logs the panic, and sends an error response to the client

### 8.5 Graceful Degradation

Redis is treated as optional throughout the codebase:
- If Redis is unavailable at startup: rate limiting falls back to in-memory, WebSocket broadcasting falls back to local-only, CU cache falls back to local-only, bitcoin price leader election is skipped
- All Redis operations are wrapped with nil checks and error handling that falls back to local behavior

## 9. Known Gaps and Technical Debt

1. **Shared package drift**: `packages/shared/src/types/game.ts` defines a simplified `GameState` and `ActionType` enum that is significantly out of date with the actual server response. The desktop client's `api.ts` is the real type authority. The shared package is currently unused by the desktop client.

2. **Mobile client**: `apps/mobile/` directory exists in the monorepo structure but contains no implementation.

3. **OAuth authentication**: Database schema supports OAuth (Google, Apple, Discord) but no OAuth flows are implemented. Only email/password auth exists.

4. **Resource history**: The `resource_history` TimescaleDB hypertable exists but no code writes to it. It was part of the initial schema but was never wired up.

5. **Event log persistence**: The `event_log` TimescaleDB hypertable exists in the schema but events are only delivered via WebSocket push -- they are not persisted to this table.

6. **No automated tests for game balance**: The test suite covers WebSocket protocol correctness and some unit tests (rate limiting, bitcoin price model) but does not test game balance or progression curves.

7. **Helm charts**: A `helm/homelab-game/` directory exists with Kubernetes manifests, but the actual deployment uses Docker Swarm (`docker-stack.yml`). The Helm charts may be aspirational or from an earlier design.

8. **No structured logging**: The backend uses `log.Printf` throughout. No structured logging library, log levels, or correlation IDs.

9. **No observability**: No metrics, tracing, or health check beyond the basic `GET /health` endpoint. No readiness/liveness probe differentiation.

10. **Single-file game handler**: `internal/api/handlers/game.go` is the largest file in the codebase (1300+ lines). Action processing, tick logic, customer growth, bitcoin data fetching, and state building are all in one file.
