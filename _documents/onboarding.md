# Project Briefing: Homelab the Game

## What Is This?

An AFK/clicker simulation game where players build and manage a homelab, progressing from a single server on a coffee table through six tiers to colocation and building their own datacenter. Server-authoritative Go backend with a Tauri+React desktop client, PostgreSQL+TimescaleDB storage, optional Redis for scaling. Live at https://game.homelab.living/.

## Quick Start

```bash
# Backend (Go 1.25+, PostgreSQL 16 + TimescaleDB running locally)
cd apps/backend
go run ./cmd/server/                # Runs on :8080 (or PORT env)

# Frontend (Node.js + pnpm)
cd apps/desktop
pnpm install
pnpm dev                            # Vite dev server on :3000

# Database migrations (manual, numbered SQL files)
cat apps/backend/internal/database/migrations/NNN_name.sql | sudo -u postgres psql -d homelab_game

# Tests
cd apps/backend && go test ./...    # 120 tests across 6 files
```

## Architecture at a Glance

```
+-----------------+         +------------------------------------------+
|  Desktop Client |         |  Go Backend (:8080)                      |
|  (Tauri + React)|         |                                          |
|                 |  WS /ws |  routes.go --> middleware chain           |
|  Zustand store  |<------->|  +- auth.go      (JWT validation)        |
|  wsClient.ts    |  REST   |  +- game.go      (action handler, ticks) |
|  useIdleTick.ts |<------->|  +- social.go    (groups, leaderboards)  |
|  (rAF interp)   |         |  +- ws/hub.go    (per-user WS mgmt)     |
+-----------------+         |                                          |
                            |  engine.go   <-- Stateless game engine   |
                            |  catalog/*   <-- Static game data        |
                            |  bitcoin/*   <-- Price simulation        |
                            |  events/*    <-- Random events           |
                            +----------+-------------------------------+
                                       |
                            +----------+----------+
                            |  PostgreSQL 16      |  Redis (optional)
                            |  + TimescaleDB      |  Rate limits, pub/sub,
                            |  DB: homelab_game   |  BTC leader election,
                            |  User: homelab_game |  CU cache
                            +---------------------+
```

**Data flow**: Client connects via WebSocket (`/ws?token=JWT`). Server starts per-user tick goroutine (5s interval) that calculates idle progress and pushes state. Client sends actions with UUID correlation; server validates, processes, returns result. HTTP REST is the fallback when WS is disconnected.

## Module Map

| Directory | Purpose |
|---|---|
| `apps/backend/cmd/server/` | Server entrypoint, DI, graceful shutdown |
| `apps/backend/internal/api/` | HTTP handlers, middleware (auth, CORS, rate limit), WebSocket hub |
| `apps/backend/internal/game/engine/` | Stateless game engine -- tick processing + 30 action types (1700 lines) |
| `apps/backend/internal/game/catalog/` | Static game data: 23 hardware, 24 services, 21+ upgrades, 9 SaaS, 10 research nodes |
| `apps/backend/internal/game/bitcoin/` | BTC price simulation with Redis leader election |
| `apps/backend/internal/game/events/` | Tier-weighted random events with mitigation system |
| `apps/backend/internal/database/` | pgx connection pool, 14 SQL migrations, 13 query files with batch loading |
| `apps/backend/internal/models/` | Go data models (GameState: 37 DB columns, 54 struct fields) |
| `apps/backend/internal/auth/` | JWT (HS256, 24h) + bcrypt password hashing |
| `apps/backend/internal/config/` | Env var + Docker secrets config loader |
| `apps/desktop/src/stores/` | Zustand store -- single source of truth, 25+ actions |
| `apps/desktop/src/hooks/` | useWebSocket, useIdleTick (rAF interpolation), useConfig |
| `apps/desktop/src/components/` | 17 React components -- tab-based UI, shared currency components |
| `apps/desktop/src/utils/` | Currency color system, formatNumber utility |
| `packages/shared/` | Shared TS types (currently stale -- types live in api.ts) |
| `_documents/` | Specs, TDDs, UX specs, game design docs |

## Key Files

| File | Why It Matters |
|---|---|
| `apps/backend/internal/game/engine/engine.go` | **The game engine** -- ProcessIdleProgress() and ProcessAction() with 30 action types (1700 lines) |
| `apps/backend/internal/api/handlers/game.go` | **The glue** -- per-user mutex, tick goroutines, WS action handling, state caching (1348 lines) |
| `apps/backend/internal/api/routes/routes.go` | All HTTP routes and middleware composition (56 lines) |
| `apps/backend/internal/database/queries/batch.go` | LoadFullGameState -- batches 8 child-table queries into 2 DB round-trips |
| `apps/backend/internal/api/ws/hub.go` | WebSocket hub -- single-conn-per-user, ping/pong, send pumps |
| `apps/desktop/src/stores/gameStore.ts` | Zustand store -- all client actions, optimistic updates, WS state sync |
| `apps/desktop/src/api.ts` | REST client + all TypeScript interfaces (373 lines, canonical type definitions) |
| `apps/desktop/src/wsClient.ts` | WebSocket client -- UUID correlation, 10s timeout, HTTP fallback |
| `apps/desktop/src/hooks/useIdleTick.ts` | Client-side currency interpolation via requestAnimationFrame |
| `apps/desktop/src/styles/global.css` | CSS custom properties -- theme vars, currency colors |

## Development Workflow

**Prerequisites**: Go 1.25+, Node.js + pnpm, PostgreSQL 16 + TimescaleDB, Rust + Cargo (for Tauri builds only)

**Backend development**: Edit Go files, restart `go run ./cmd/server/`. No hot reload. Run `go test ./...` for the 120-test suite. No linter configured.

**Frontend development**: `pnpm dev` for hot-reload via Vite. `pnpm build` runs `tsc && vite build`. No frontend tests exist. TypeScript strict mode enforced.

**Database changes**: Write a numbered SQL migration file, apply via `cat | psql`. No migration framework -- manual tracking via `schema_migrations` table (populated by Docker entrypoint script).

**Deployment**: Single VM, no environments. Backend runs as background process. CI builds Docker images to GHCR. Docker Swarm stack available (`docker-stack.yml`) with Traefik, 2 backend replicas, Redis.

**Key env vars**: `PORT` (8080), `DB_*` (PostgreSQL), `JWT_SECRET`, `REDIS_ADDR` (optional), `VITE_API_URL` (frontend), `ENV` (set "production" to restrict CORS/WS origins).

## Onboarding Recommendations

**Start here**: Read `engine.go` (the game engine) and `game.go` (the handler layer) to understand the tick system and action processing. Then `routes.go` for the API surface.

**Key patterns to follow**:
- Server-authoritative: all mutations go through `ProcessAction()`, never client-side
- Per-user mutex: always acquire before reading/writing user state
- Batch DB loading: use `LoadFullGameState` pattern, not individual queries
- Currency colors: use `CURRENCY_COLORS` from `utils/currencyColors.ts`, never hardcode hex values
- Cost buttons use currency color (what it costs), not category color (where it lives)

**Known technical debt**:
- `engine.go` (1700 lines) and `game.go` (1348 lines) are monolithic -- consider splitting by action category
- `packages/shared/` types are stale -- all canonical types live in `apps/desktop/src/api.ts`
- No frontend tests, sparse backend tests (120 tests, no engine/action coverage)
- No structured logging (uses `log.Printf` throughout)
- `formatNumber()` was recently consolidated but MarketPanel still has its own `formatCurrency()`
- `resource_history` and `event_log` hypertables exist in schema but no code writes to them
- No token revocation -- compromised JWT valid for 24h
- `ENV` defaults to permissive mode (dev origins allowed in CORS/WS)
