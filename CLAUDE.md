# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Homelab the Game — an AFK/clicker simulation game where players build and manage a homelab, progressing from a server on a coffee table to colocating racks in datacenters. Server-authoritative multiplayer game with ads, IAP, groups, and leaderboards.

See docs/PLAN.md for full game design and docs/ROADMAP.md for implementation phases.

## Architecture

Monorepo with three main components:

- **`packages/shared/`** — Shared TypeScript types and constants consumed by both clients and referenced by the backend
- **`apps/desktop/`** — Tauri (Rust shell) + React + TypeScript desktop client
- **`apps/mobile/`** — React Native + TypeScript mobile client
- **`apps/backend/`** — Go server (server-authoritative game engine, REST API, auth)

### Backend Structure (Go)

- `cmd/server/` — Application entrypoint
- `internal/api/` — HTTP handlers, middleware, route definitions
- `internal/auth/` — OAuth2 (Google, Apple, Discord) + email/password auth
- `internal/game/engine/` — Server-side tick system, idle progress calculation
- `internal/game/events/` — Random event engine (tier-weighted)
- `internal/game/state/` — Game state management and validation (anti-cheat)
- `internal/models/` — Data models
- `internal/database/` — PostgreSQL + TimescaleDB migrations and queries

### Desktop Client (Tauri + React)

- State management: Zustand (synced with server state)
- Rendering: HTML/CSS for UI, Canvas or Pixi.js for visual rack/lab view
- Styling: Tailwind CSS
- Bundler: Vite

### Mobile Client (React Native)

- AdMob native SDK for ads
- Native Apple/Google purchase APIs for IAP
- FCM (Android) / APNs (iOS) for push notifications

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
pnpm build                      # Production build
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

- **Backend:** Go, PostgreSQL, TimescaleDB
- **Desktop:** Tauri, React, TypeScript, Vite, Tailwind CSS
- **Mobile:** React Native, TypeScript
- **Package manager:** pnpm (JS/TS), Go modules (backend)

## Database

- **PostgreSQL** — users, game state, groups, leaderboards, transactions
- **TimescaleDB** — uptime tracking, resource accumulation history, analytics

## Key Design Decisions

- Server-authoritative: all game state mutations are validated server-side. Clients send actions, server validates and returns updated state.
- Game uses a tick system on the backend to calculate idle progress, process events, and update state.
- Prestige system ("Colo") resets the player to the coffee table tier but keeps automation/knowledge upgrades and colo'd racks earning passively.
- Infrastructure is self-hosted on a homelab VM.
- Live at https://game.homelab.living/ (API at https://api.homelab.living/).

## Deployment

Everything runs on this single VM. There are no separate environments (dev/staging/prod).

- **Backend**: `go run ./cmd/server/` on port 8080, run as a background process. Restart by killing the old process and starting a new one (see Common Commands above).
- **Frontend**: Vite dev server on port 3000 or built static files served directly.
- **Reverse proxy (nginx)**: Handled externally, NOT on this VM. Nginx proxies `api.homelab.living` → `:8080` and `game.homelab.living` → `:3000`. Do not look for nginx config on this machine.
- **Database**: PostgreSQL 16 + TimescaleDB running locally. DB user is `homelab_game`, database is `homelab_game`.
- **After code changes**: Rebuild and restart the backend process. No deploy pipeline — the code runs directly from this repo.
