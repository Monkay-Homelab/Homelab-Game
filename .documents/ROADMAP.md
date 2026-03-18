# Homelab the Game — Roadmap

## Phase 1: Project Foundation

- [x] Initialize monorepo structure
- [x] Set up shared TypeScript package (game types, constants, enums)
- [x] Set up Go backend project with module structure
- [x] Set up Tauri desktop app with React + Vite + Tailwind
- [x] PostgreSQL + TimescaleDB schema design
- [x] Basic dev environment (hot reload, local DB)

## Phase 2: Backend Core

- [x] User auth (email/password, OAuth2 with Google/Apple/Discord)
- [x] Session management and JWT tokens
- [x] REST API scaffolding (routes, middleware, error handling)
- [x] Database models: users, game state, hardware, services
- [x] Server-side game tick system (idle progress calculation)
- [x] Action validation layer (anti-cheat foundation)

## Phase 3: Core Game Loop

- [ ] Click-to-run-a-job mechanic (client sends action, server validates and returns updated state)
- [ ] Compute units earning and spending
- [ ] Reputation passive generation based on uptime
- [ ] Power (watts) tracking and limits
- [ ] Hardware slots system (pre-rack: 1–2 coffee table, 3–5 closet)
- [ ] Basic hardware purchasing (spend compute, fill slots)
- [ ] Idle income from deployed services
- [ ] Offline progress calculation on reconnect

## Phase 4: Desktop Client MVP

- [ ] Connect Tauri app to backend API
- [ ] Zustand state management syncing with server
- [ ] Main game screen — visual representation of current tier (coffee table / closet / rack)
- [ ] Click interaction UI
- [ ] Currency display (compute, reputation, power, slots)
- [ ] Hardware inventory and upgrade shop
- [ ] Service deployment UI
- [ ] Basic notifications for events

## Phase 5: Progression System

- [ ] Tier progression: Coffee Table → Closet Floor → 12U → 24U → 36U → 48U
- [ ] Tier-specific upgrades (coffee table, closet, rack)
- [ ] Rack unit (U) management replacing slots at 12U
- [ ] Service unlock tree tied to tiers
- [ ] Component upgrades (CPU, RAM, storage, NIC) for individual servers
- [ ] Networking progression (unmanaged → managed → 10GbE → fiber)
- [ ] Cooling system and overheat mechanics
- [ ] Persistent upgrades: automation and knowledge trees

## Phase 6: Events System

- [ ] Event engine on the backend (random events on tick, weighted by tier)
- [ ] Coffee table events (cat, noise, spilled drink)
- [ ] Closet floor events (overheating, tripped breaker, cable spaghetti, spouse aggro)
- [ ] Rack tier events (power outage, drive failure, ISP outage, firmware brick)
- [ ] Software events (kernel panic, security breach, DNS, certs, dependency breakage)
- [ ] Event resolution mechanics (click minigame, resource spend, mitigation check)
- [ ] Event notification and UI on desktop client

## Phase 7: SaaS/IaaS & Monetization (In-Game)

- [ ] Money ($) currency system
- [ ] Customer simulation (users sign up for your services)
- [ ] Revenue generation from hosted services
- [ ] SaaS/IaaS events (hug of death, support tickets, enterprise inquiries, chargebacks, TOS abuse)
- [ ] Business expenses system

## Phase 8: Prestige & Colo

- [ ] Colo trigger conditions (48U maxed + SaaS/IaaS tier)
- [ ] Prestige reset flow (rack moves to datacenter, restart at coffee table)
- [ ] Permanent colo multipliers (stacking with diminishing returns)
- [ ] Colo'd rack passive income
- [ ] Colo events (maintenance windows, bandwidth overages, remote hands, cross-connect, lease renewal)
- [ ] Datacenter tier progression (Tier 1–4)
- [ ] Datacenter manager view (fleet management after multiple colos)

## Phase 9: Endgame

- [ ] Build your own datacenter mechanic
- [ ] Final prestige layer
- [ ] Endgame balancing and pacing

## Phase 10: Mobile App

- [ ] React Native project setup
- [ ] Shared types/constants integration from monorepo
- [ ] Core game UI adapted for mobile
- [ ] API integration (same backend)
- [ ] Push notifications (FCM / APNs) for events and idle milestones
- [ ] Mobile-specific UX (touch interactions, responsive layout)

## Phase 11: Social Features

- [ ] Groups/collectives system (create, join, manage)
- [ ] Roles (founder, admin, member) and permissions
- [ ] Combined compute pools and group jobs
- [ ] Shared services across group members' racks
- [ ] Group leaderboard rankings and milestones
- [ ] Individual leaderboards (compute, uptime streak, services, fastest prestige, colo count)

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
