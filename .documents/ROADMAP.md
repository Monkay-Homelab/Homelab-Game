# Homelab the Game — Roadmap

## Phase 1: Project Foundation

- [x] Initialize monorepo structure
- [x] Set up shared TypeScript package (game types, constants, enums)
- [x] Set up Go backend project with module structure
- [x] Set up Tauri desktop app with React + Vite + Tailwind
- [x] PostgreSQL + TimescaleDB schema design
- [x] Basic dev environment (hot reload, local DB)

## Phase 2: Backend Core

- [x] User auth (email/password)
- [x] Session management and JWT tokens
- [x] REST API scaffolding (routes, middleware, error handling)
- [x] Database models: users, game state, hardware, services
- [x] Server-side game tick system (idle progress calculation)
- [x] Action validation layer (anti-cheat foundation)

## Phase 3: Core Game Loop

- [x] Click-to-run-a-job mechanic (client sends action, server validates and returns updated state)
- [x] Compute units earning and spending
- [x] Reputation passive generation based on uptime
- [x] Power (watts) tracking and limits
- [x] Hardware slots system (pre-rack: 1–2 coffee table, 3–5 closet)
- [x] Basic hardware purchasing (spend compute, fill slots)
- [x] Idle income from deployed services
- [x] Offline progress calculation on reconnect

## Phase 4: Desktop Client MVP

- [x] Connect Tauri app to backend API
- [x] Zustand state management syncing with server
- [x] Main game screen — visual representation of current tier (coffee table / closet / rack)
- [x] Click interaction UI
- [x] Currency display (compute, reputation, power, slots)
- [x] Hardware inventory and upgrade shop
- [x] Service deployment UI

## Phase 5: Progression System

- [x] Tier progression: Coffee Table → Closet Floor → 12U → 24U → 36U → 48U
- [x] Tier-specific upgrades (coffee table, closet, rack)
- [x] Rack unit (U) management replacing slots at 12U
- [x] Service unlock tree tied to tiers
- [x] Component upgrades (CPU, RAM, storage, NIC) for individual servers
- [x] Networking progression (unmanaged → managed → 10GbE → fiber)
- [x] Cooling system and overheat mechanics
- [x] Persistent upgrades: automation and knowledge trees

## Phase 6: Events System

- [x] WebSocket server (Go) — persistent connections for server-pushed events
- [x] WebSocket client integration (desktop + mobile)
- [x] Event engine on the backend (random events on tick, weighted by tier)
- [x] Coffee table events (cat, noise, spilled drink)
- [x] Closet floor events (overheating, tripped breaker, cable spaghetti, spouse aggro)
- [x] Rack tier events (power outage, drive failure, ISP outage, firmware brick)
- [x] Software events (kernel panic, security breach, DNS, certs, dependency breakage)
- [x] Event resolution mechanics (click minigame, resource spend, mitigation check)
- [x] Real-time event notifications pushed via WebSocket
- [x] Event notification and UI on desktop client

## Phase 7: SaaS/IaaS & Monetization (In-Game)

- [x] Money ($) currency system
- [x] Customer simulation (users sign up for your services)
- [x] Revenue generation from hosted services
- [x] SaaS/IaaS events (hug of death, support tickets, enterprise inquiries, chargebacks, TOS abuse)
- [x] Business expenses system

## Phase 8: Prestige & Colo

- [x] Colo trigger conditions (48U maxed + SaaS/IaaS tier)
- [x] Prestige reset flow (rack moves to datacenter, restart at coffee table)
- [x] Permanent colo multipliers (stacking with diminishing returns)
- [x] Colo'd rack passive income
- [x] Colo events (maintenance windows, bandwidth overages, remote hands, cross-connect, lease renewal)
- [x] Datacenter tier progression (Tier 1–4)
- [x] Datacenter manager view (fleet management after multiple colos)

## Phase 9: Endgame

- [x] Build your own datacenter mechanic
- [x] Final prestige layer
- [x] Endgame balancing and pacing

## Phase 10: Social Features

- [x] Groups/collectives system (create, join, manage)
- [x] Roles (founder, admin, member) and permissions
- [x] Combined compute pools and group jobs
- [x] Shared services across group members' racks
- [x] Group leaderboard rankings and milestones
- [x] Individual leaderboards (compute, uptime streak, services, fastest prestige, colo count)

## Phase 11: Mobile App

- [ ] React Native project setup
- [ ] Shared types/constants integration from monorepo
- [ ] Core game UI adapted for mobile
- [ ] API integration (same backend)
- [ ] Push notifications (FCM / APNs) for events and idle milestones
- [ ] Mobile-specific UX (touch interactions, responsive layout)
- [ ] OAuth2 auth (Google, Apple, Discord)

## Phase 12: Monetization (Real)

- [ ] AdMob integration (mobile — rewarded ads, banner, interstitial)
- [ ] Ad reward system on backend (validate ad completion, grant bonus)
- [ ] IAP integration (Apple App Store, Google Play)
- [ ] Cosmetics store (rack skins, server themes, cable colors)
- [ ] Boosters (temporary multipliers)
- [ ] Premium content (no pay-to-win)
- [ ] Transaction logging and receipt validation

## Phase 13: Polish & Launch

- [ ] Game balancing pass (progression pacing, currency rates, event frequency)
- [ ] UI/UX polish on desktop and mobile
- [ ] Visual rack/lab rendering (Canvas or Pixi.js)
- [ ] Sound design and effects
- [ ] Tutorial / onboarding flow
- [ ] CI/CD pipeline setup
- [ ] Load testing the backend
- [ ] Beta testing
- [ ] App store submissions (iOS + Android)
- [ ] Desktop distribution (website download, auto-updates)
- [ ] Launch
