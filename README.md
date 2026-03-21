# Homelab the Game

An AFK/clicker simulation game where you build and manage a homelab — starting from a single server on a coffee table, scaling through rack tiers, and eventually colocating racks in datacenters.

Server-authoritative multiplayer with groups, leaderboards, SaaS business simulation, and a prestige system.

## How It Works

**Click** to run compute jobs. **Buy hardware** to earn idle income. **Deploy services** like Pi-hole, Plex, Kubernetes, and more. **Upgrade** your rack from 12U to 48U. **Unlock SaaS** to sell hosting to customers. **Colocate** your rack for permanent multipliers and start over. **Build your own datacenter** as the endgame.

### Progression

```
Coffee Table → Closet Floor → 12U Rack → 24U Rack → 36U Rack → 48U Rack → Colo (prestige)
```

### Currencies

- **Compute Units (CU)** — primary currency from jobs and idle hardware
- **Reputation** — earned from services, unlocks SaaS features
- **Money ($)** — earned from SaaS customers, spent on knowledge and datacenter
- **Power (W)** — constrains how much hardware you can run

## Architecture

Monorepo with three apps and a shared package:

```
apps/
  backend/     Go server — game engine, REST API, auth, PostgreSQL
  desktop/     Tauri + React + TypeScript — desktop client
  mobile/      React Native + TypeScript — mobile client (planned)
packages/
  shared/      Shared TypeScript types and constants
```

### Backend (Go)

- Server-authoritative game engine with tick-based idle progress
- 29 random events with tier-weighted severity and mitigation
- SaaS business simulation with customer growth and satisfaction
- Prestige system with diminishing-return multipliers
- OAuth2 (Google, Apple, Discord) + email/password auth
- WebSocket for real-time event push
- PostgreSQL + TimescaleDB

### Desktop Client (Tauri + React)

- Zustand state management synced with server
- Client-side rate interpolation for smooth counters
- Config-driven UI — zero hardcoded game values
- Tailwind CSS styling
- Vite bundler

## Quick Start

### Prerequisites

- Go 1.21+
- Node.js 18+
- pnpm
- PostgreSQL with TimescaleDB

### Setup

```bash
# Install JS dependencies
pnpm install

# Apply database migrations
cd apps/backend/internal/database/migrations
for f in 0*.sql; do
  su - postgres -c "psql -d homelab_game -f $(pwd)/$f"
done

# Start the backend
cd apps/backend
go run ./cmd/server/            # Runs on :8080

# Start the desktop client
cd apps/desktop
pnpm dev                        # Runs on :3000
```

## Game Features

### Hardware

10 compute devices (Raspberry Pi to GPU Server), 3 storage devices, 5 network switches, 3 UPS units, plus shelves and patch panels. Each with unique power draw, compute output, and tier requirements.

### Services

34 deployable services across all tiers — from Pi-hole and personal websites to Kubernetes clusters, AI/ML training, and full IaC.

### Component Upgrades

CPU, RAM, Storage, and NIC upgrades on compute hardware. Percentage-based (+5% of base compute per level), scaling meaningfully from early game to endgame.

### SaaS Business

Unlock at rack tier to sell hosting services (email, VPN, VPS, managed databases, GPU cloud). Customers grow over time, generate money, and have satisfaction that decays during outages.

### Events

29 random events across 6 categories (coffee table, closet, rack, software, SaaS, colo). Events can be mitigated by owning specific hardware or upgrades.

### Prestige (Colo)

At 48U with SaaS unlocked, colocate your rack in a datacenter. Your rack earns passively while you restart from the coffee table with permanent multipliers.

### Datacenter

After 5+ colos, build your own datacenter. Upgrade through 5 levels (Small Facility to Hyperscale) for increasing income multipliers on all colo'd racks.

### Social

Form collectives with other players for up to +50% compute bonus. Compete on leaderboards across compute, reputation, prestiges, money, and donated CU.

### Global CU Store

Donate surplus compute units to a community pool. Lifetime donations tracked on a persistent leaderboard.

## Tech Stack

| Component | Technology |
| --------- | ---------- |
| Backend | Go, PostgreSQL, TimescaleDB |
| Desktop | Tauri, React, TypeScript, Vite, Tailwind CSS |
| Mobile | React Native, TypeScript (planned) |
| Auth | OAuth2 (Google, Apple, Discord) + email/password |
| Real-time | WebSocket |
| Package manager | pnpm (JS/TS), Go modules (backend) |

## Claude Code Agent Team

This project includes a multi-agent development team for Claude Code, located in `.claude/`. The team coordinates planning, implementation, review, testing, and UX design through specialized agents and orchestration skills.

### Agents

| Agent | Role |
| ----- | ---- |
| **Staff Engineer** | Architecture, TDDs, code review, project specs |
| **Senior Engineer** | Implementation, debugging, code quality |
| **Project Manager** | Issue planning, task decomposition, dependency management |
| **SDET** | Test infrastructure, verification, quality engineering |
| **UX Designer** | Design specs, UX review, design QA |

### Skills

| Skill | Purpose |
| ----- | ------- |
| `/dev <work>` | Orchestrates the full agent team for planning and executing development work |
| `/specs` | Bootstraps project specification files in `docs/spec/` |
| `/vote <proposal>` | PBFT-inspired consensus protocol for multi-agent decision validation |
| `/evolve-agents` | Reviews and improves agent definitions |
| `/evolve-skills` | Reviews and improves skill definitions |

All agents use GitHub Issues via `gh` CLI for issue tracking and coordination.

## Documentation

- **[GAME-MECHANICS.md](docs/GAME-MECHANICS.md)** — Complete game mechanics reference with formulas, catalogs, and balance data
- **[PLAN.md](docs/PLAN.md)** — Full game design document
- **[ROADMAP.md](docs/ROADMAP.md)** — Implementation phases

## License

All rights reserved.
