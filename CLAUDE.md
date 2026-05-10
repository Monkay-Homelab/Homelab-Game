# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Homelab the Game — an AFK/clicker simulation game where players build and manage a homelab, progressing from a server on a coffee table to colocating racks in datacenters. Server-authoritative multiplayer game with groups and leaderboards.

See `_documents/PLAN.md` for full game design, `_documents/ROADMAP.md` for implementation phases, and `_documents/tdd/` for technical design documents.

Live at https://game.homelab.living/ (API at https://api.homelab.living/).

## Architecture

Monorepo with three main components:

- **`apps/backend/`** — Go server (game engine, REST API, WebSocket, auth)
- **`apps/desktop/`** — Tauri (Rust shell) + React + TypeScript desktop client
- **`apps/mobile/`** — React Native + TypeScript mobile client (planned)
- **`packages/shared/`** — Shared TypeScript types (Tier enum, GameState, constants)

### How They Communicate

```
Desktop Client                          Go Backend
┌─────────────┐    WebSocket /ws       ┌──────────────┐
│  Zustand     │◄──────��───────────────│  Hub          │
│  Store       │   (state pushes,      │  (per-user    │
│              │    action results,     │   connections)│
│  api.ts      │    events)            │              │
│  wsClient.ts │───────────────────────►  Handlers     │──► PostgreSQL
│              │   REST /api/game/*     │  Engine       │    + TimescaleDB
│              │   REST /api/auth/*     │  Catalog      │
│              │   REST /api/social/*   │              │──► Redis (optional)
└─────────────┘                        └──────────────┘
```

- **WebSocket** (`/ws?token=JWT`): Primary channel. Server pushes state updates every tick, delivers events. Client sends actions with UUID-based request/response matching (10s timeout). Falls back to HTTP REST if WS disconnects.
- **REST API**: Auth endpoints (login/register), game state fetch, game actions, social/group/leaderboard endpoints.
- **Auth**: JWT tokens (24h expiry) stored in `localStorage`. 401 triggers client logout+reload.

### Backend Structure (Go)

- `cmd/server/` — Entrypoint, dependency injection, graceful shutdown
- `internal/api/routes/` — HTTP route definitions with middleware chain
- `internal/api/handlers/` — AuthHandler, GameHandler, SocialHandler
- `internal/api/middleware/` — Auth (JWT), CORS, RateLimit (Redis-backed with memory fallback), BodyLimit, JSON
- `internal/api/ws/` — WebSocket hub (per-user connections), broadcaster (local or Redis pub/sub)
- `internal/auth/` — JWT generation/validation, password hashing
- `internal/config/` — Env var / Docker secrets config loader
- `internal/game/engine/` — Server-side tick system, idle progress calculation, earnings math
- `internal/game/catalog/` — Hardware, services, upgrades, SaaS, research definitions
- `internal/game/bitcoin/` — Bitcoin price simulation, leader election via Redis
- `internal/game/events/` — Random event engine (tier-weighted)
- `internal/game/state/` — Game state validation and anti-cheat
- `internal/models/` — Data models (GameState has 50+ fields)
- `internal/database/queries/` — Query objects with batch loading (`LoadFullGameState` batches 8+ queries)
- `internal/database/migrations/` — Numbered SQL migrations

### Desktop Client (Tauri + React)

- **State**: Zustand store (`stores/gameStore.ts`) — single source of truth, synced via WebSocket
- **API layer**: `api.ts` (REST client, type definitions), `wsClient.ts` (WebSocket singleton)
- **Idle ticking**: `hooks/useIdleTick.ts` — client-side currency interpolation via `requestAnimationFrame` between server ticks
- **Styling**: Tailwind CSS 4 + CSS custom properties in `styles/global.css`
- **UI**: Tab-based layout (Hardware, Services, Upgrades, Research, SaaS, Datacenter, Market, Social)
- **Shared components**: `components/shared/` — CurrencyValue, CurrencyStatLine
- **Utilities**: `utils/currencyColors.ts` — Currency color system (CURRENCY_COLORS, formatNumber)
- **Bundler**: Vite on port 3000 (host 0.0.0.0)

### Rate Limits

- Auth: 10/min per IP
- Game actions: 7200/min per user (30/sec burst)
- Social: 180/min per user

## Common Commands

### Backend (Go)

```bash
cd apps/backend
go build ./cmd/server/          # Build the server
go test ./...                   # Run all tests

# Restart the backend (kill old process, start new one)
lsof -ti:8080 | xargs kill -9; sleep 1; cd /root/project/apps/backend && go run ./cmd/server/ &>/tmp/backend.log &
tail -f /tmp/backend.log        # Watch logs
```

### Desktop (Tauri + React)

```bash
cd apps/desktop
pnpm dev                        # Vite dev server (port 3000)
pnpm build                      # Production build (tsc && vite build)
cargo tauri dev                 # Run Tauri desktop app (dev mode)
cargo tauri build               # Build Tauri desktop binary
```

### Shared Package

```bash
cd packages/shared
pnpm typecheck                  # TypeScript type checking
```

### Database

```bash
# Apply migrations (pipe to psql to avoid file permission issues)
cat /root/project/apps/backend/internal/database/migrations/NNN_name.sql | sudo -u postgres psql -d homelab_game

# Grant permissions on new tables (db user is homelab_game)
echo "GRANT ALL ON <table_name> TO homelab_game;" | sudo -u postgres psql -d homelab_game
```

### Monorepo

```bash
pnpm install                    # Install all workspace dependencies
```

## Tech Stack

- **Backend:** Go 1.25, PostgreSQL 16 + TimescaleDB, Redis (optional)
- **Desktop:** Tauri 2, React 19, TypeScript, Vite 8, Tailwind CSS 4, Zustand 5
- **Mobile:** React Native, TypeScript (planned)
- **Package manager:** pnpm (JS/TS), Go modules (backend)
- **Key Go deps:** pgx/v5 (Postgres), go-redis/v9, golang-jwt/v5

## Key Design Decisions

- **Server-authoritative**: All game state mutations are validated server-side. Clients send actions, server validates and returns updated state. Client does optimistic interpolation between ticks via `requestAnimationFrame`.
- **Tick system**: Backend calculates idle progress per tick — compute units, reputation, money per second with hardware bonuses, component upgrades, service output, throttling/overclocking multipliers.
- **Prestige ("Colo")**: Resets player to coffee table tier but keeps automation/knowledge upgrades and colo'd racks earning passively.
- **Currency color system**: 6 currencies (CU, Money, Reputation, Knowledge Points, Bitcoin, Power) each have a distinct color defined via CSS custom properties (`--currency-*` in global.css) and a TypeScript utility (`CURRENCY_COLORS` in `utils/currencyColors.ts`). See `_documents/ux/currency-colors.md`.
- **WebSocket-first with HTTP fallback**: Actions prefer WebSocket; if disconnected, `wsClient.ts` falls back to REST. Reconnects after 5s.
- **Redis optional**: Rate limiting and WebSocket pub/sub use Redis when available, gracefully fall back to in-memory alternatives.

## Deployment

Everything runs on this single VM. There are no separate environments (dev/staging/prod).

- **Backend**: `go run ./cmd/server/` on port 8080 (configurable via `PORT` env). Run as a background process.
- **Frontend**: Vite dev server on port 3000 or built static files served via nginx.
- **Reverse proxy**: Handled externally, NOT on this VM. Proxies `api.homelab.living` → `:8080` and `game.homelab.living` → `:3000`. Do not look for nginx/Traefik config on this machine.
- **Database**: PostgreSQL 16 + TimescaleDB running locally. User `homelab_game`, database `homelab_game`.
- **Dev URLs**: `dev-api.homelab.living` and `dev-game.homelab.living` also point to this machine.
- **After code changes**: Rebuild and restart the backend process. No deploy pipeline — the code runs directly from this repo.
- **Docker**: CI builds three images (backend, migrations, frontend) to GHCR via `.github/workflows/build.yml`. Backend uses distroless, frontend uses nginx with SPA fallback.

## Environment Variables

### Backend (`.env` or Docker secrets)

- `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` — PostgreSQL connection
- `JWT_SECRET` — JWT signing key (auto-generated if missing, logs warning)
- `REDIS_ADDR`, `REDIS_PASSWORD` — Optional Redis connection
- `PORT` — Server port (default: 8080)
- `REGISTRATION_ENABLED` — Toggle new user registration

### Frontend

- `VITE_API_URL` — Backend API URL (default: `https://api.homelab.living`)
