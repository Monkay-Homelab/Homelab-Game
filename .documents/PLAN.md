# Homelab the Game

## Overview

An AFK/clicker simulation game based around the concept of a homelab — the real-world hobby where enthusiasts run enterprise-grade hardware and services at home for personal use and self-hosting.

## Core Mechanics

### Idle/Clicker Loop

- **Click action:** "Run a job" — each click processes a task (compiling code, transcoding media, running a query, etc.). Thematically scales from small tasks to massive workloads as you progress. The job type and reward scale with your current tier.
- **Idle income:** Running services passively generate compute units and reputation over time.
- **Early progression:** Start with a single server on the coffee table (no rack, limited slots). Move to the closet floor for more space. Eventually buy your first 12U rack — a milestone unlock.
- **Rack size:** Once you have a rack, upgrade capacity through 12U → 24U → 36U → 48U as you progress.
- **Prestige system:** "Colo" — once your 48U rack is maxed out and you've hit the SaaS/IaaS tier, colocate it in a datacenter. Your rack moves offsite, you get permanent multipliers (better power, cooling, bandwidth), and you start fresh at home on the coffee table again. Each colo'd rack continues earning passively.

### Currencies

- **Compute Units** — primary currency, earned by running jobs and services. Used to buy hardware, deploy services, and upgrade rack size.
- **Reputation** — secondary currency, earned passively from uptime and service quality. Used to unlock advanced services and eventually qualify for colocation.
- **Power (Watts)** — a limiting resource. Every piece of hardware draws power; total draw is constrained by your home circuit. Upgrades improve efficiency or expand capacity. Resets on prestige — the datacenter handles power for colo'd racks, but your new home setup starts with the same home circuit limits again.
- **Money ($)** — real-world cash currency, unlocks at the SaaS/IaaS tier. Earned from selling services to customers. Spent on colo leases, bandwidth overages, remote hands, and business expenses. Not earned from clicking — only from running a business.
- **Hardware Slots** — how much gear you can run. On the coffee table you get 1–2 slots. On the closet floor you get 3–5 slots. Once you buy a rack, slots are replaced by rack units (U) at 12U/24U/36U/48U. Rack size upgrades cost compute units.

### Upgrades

#### Coffee Table Upgrades
- **Second-hand hardware:** Buy a better used server off eBay. Improves job throughput within your 1–2 slot limit.
- **Quiet fan:** Reduces noise, prevents the partner/roommate complaint event.
- **Surge protector:** Protects gear from power spikes. Cheap insurance.
- **Ethernet cable:** Move off Wi-Fi for better service reliability.

#### Closet Floor Upgrades
- **Extra machines:** Add more hardware to fill your 3–5 slots.
- **Power strip upgrade:** Higher-quality strip with more outlets. Reduces tripped breaker risk.
- **Closet fan:** Improves airflow, prevents overheating events.
- **Cable organizer:** Velcro ties, labels. Reduces troubleshooting time during events.
- **Dedicated circuit:** Electrician runs a new line to the closet. Big power capacity boost.

#### Rack Upgrades (12U–48U)
- **Rack size:** 12U → 24U → 36U → 48U. Costs compute units. More U means more hardware slots.
- **Hardware:** Servers, switches, patch panels, UPS units — each occupies rack units and draws power. Better hardware increases job throughput and unlocks new services.
- **Component upgrades:** CPU, RAM, storage, NIC upgrades for individual servers. Don't take extra rack space but improve performance and efficiency.
- **Cooling:** Fans, blanking panels, in-rack cooling, AC unit for the room. Required as total power draw increases — overheat and your gear throttles or crashes.
- **Networking:** Unmanaged switch → managed switch → 10GbE → fiber. Unlocks multi-server setups and boosts service throughput.

#### Persistent Upgrades (survive prestige)
- **Automation:** Bash scripts → Ansible → Docker → Kubernetes. Each tier increases idle income multiplier. Persists through prestige as learned knowledge.
- **Knowledge:** Certifications, tutorials completed. Permanent boosts to job efficiency and event resolution speed.

## Theme & Content

### Progression Tiers

1. **The Coffee Table** — a bare 1U server sitting on your coffee table. Loud, ugly, but it works. No rack, no cable management, just vibes.
2. **The Closet Floor** — you've moved the gear to a closet. A couple of machines stacked on the floor, power strip daisy-chained. Still no rack.
3. **12U Rack** — your first proper rack. A milestone purchase. Used enterprise gear, a switch, and a UPS. Cable management begins.
4. **24U Rack** — growing the lab. Room for dedicated storage, managed networking, multiple servers.
5. **36U Rack** — serious setup. Redundant power, 10GbE, NVMe storage arrays.
6. **48U Rack** — maxed out. Full-size rack, enterprise-grade everything. Ready to colo and prestige.

### Services to Deploy

- **Coffee Table:** Pi-hole, personal website, file share (SMB/NFS)
- **Closet Floor:** Plex/Jellyfin, Home Assistant, Nextcloud, game server (Minecraft/Valheim)
- **12U Rack:** Gitea, Grafana + Prometheus, reverse proxy (Traefik/Nginx), VPN (WireGuard), NAS (TrueNAS)
- **24U Rack:** CI/CD pipelines, Docker Swarm, mail server, Matrix/Element, security cameras (Frigate)
- **36U Rack:** Kubernetes cluster, centralized logging (ELK stack), DNS authority, database clusters
- **48U Rack:** AI/ML training, CDN node, Mastodon instance, hosting for friends and family, full IaC (Terraform + Ansible)
- **Pre-Colo (SaaS/IaaS):** Start selling services — VPS hosting, managed databases, S3-compatible storage, email hosting. Revenue outgrows your home internet and power. The demand justifies colocation.

### Events & Challenges

#### Coffee Table Events

- **Cat knocks the server off the table** — server goes offline, click to reboot. No damage, just downtime.
- **Partner/roommate complains about noise** — server fan is loud. Lose access for a few hours or buy a quieter fan.
- **Spilled drink** — too close to the coffee table. Small chance of hardware damage if you don't have a cover.

#### Closet Floor Events

- **Overheating** — no airflow in the closet. Gear throttles until you prop the door open or add a fan.
- **Tripped breaker** — too many machines on one outlet. Forces you to spread load or upgrade the circuit.
- **Cable spaghetti** — can't find which cable goes where. Slows down troubleshooting during other events.
- **Partner/spouse aggro** — electricity bill creeping up. Reduce power draw or invest in efficiency, or lose a hardware slot temporarily.

#### Rack Tier Events (12U–48U)

- **Power outage** — UPS buys you time. No UPS? Everything goes down and you lose uptime reputation.
- **Drive failure** — RAID saves you. No RAID? Lose data and service progress.
- **ISP outage** — everything goes offline. Business-class internet or failover LTE mitigates it.
- **Noise complaint from neighbors** — rack is louder than closet gear. Soundproofing or quiet fans required.
- **Firmware update bricked a device** — switch or server won't boot. Click minigame to recover, or restore from backup.

#### Software Events (Any Tier)

- **Kernel update gone wrong** — servers won't boot. Fix manually (click minigame) or roll back from snapshot if you have one.
- **Security breach** — didn't patch? Exposed service gets compromised. Costs reputation and compute to clean up.
- **DNS misconfiguration** — services unreachable. Lose idle income until resolved.
- **Certificate expired** — HTTPS services go down. Automation (certbot) prevents this.
- **Dependency broke overnight** — a container image update breaks a service. Pin your versions or scramble to fix.

#### SaaS/IaaS Events

- **Reddit hug of death** — one of your hosted services goes viral. Survive the traffic spike for a big reputation bonus, or crash and lose rep.
- **Customer support ticket** — a paying user has an issue. Resolve it quickly for reputation, ignore it and lose customers.
- **Enterprise client inquiry** — big reputation and compute payout if you can meet the SLA requirements.
- **Chargeback/fraud** — someone disputes a payment. Costs money and time to resolve.
- **TOS abuse** — a customer is using your hosting for something sketchy. Deal with it or risk reputation.

#### Colo Events

- **Datacenter maintenance window** — scheduled downtime, plan around it or take the hit.
- **Bandwidth overage** — your colo'd rack exceeded its allocation. Pay up or optimize.
- **Remote hands needed** — hardware issue at the datacenter. Costs money and time since you can't just walk over.
- **Cross-connect request** — another colo tenant wants to peer. Accept for bandwidth bonus or decline.
- **Lease renewal** — colo contract is up. Negotiate for better rates if your reputation is high enough.

### Post-Colo Progression

- **Colo multipliers stack** — each rack you colocate adds a permanent multiplier to all income. First colo is 1.5x, second is 2x, third is 2.5x, etc. Diminishing returns keep it from going infinite.
- **Datacenter tiers** — early colos go to a basic facility. As you accumulate colo'd racks, unlock better datacenters with higher multipliers: Tier 1 (basic) → Tier 2 (redundant power) → Tier 3 (concurrently maintainable) → Tier 4 (fault tolerant).
- **Datacenter manager** — after enough colos, unlock a management view for your datacenter fleet. Optimize placement, negotiate bulk rates, handle cross-rack networking.
- **Endgame: Build your own datacenter** — the ultimate goal. Stop paying for colo and build your own facility. Massive upfront cost, but you keep all the revenue. Final prestige layer.

## Tech Stack

### Architecture

- **Server-authoritative** — backend owns game state, validates all actions, calculates idle progress. Clients send actions and render state.
- **Monorepo** — shared game types/constants package consumed by all clients and backend.

### Desktop App (Tauri)

- **Shell:** Tauri (Rust-based, lightweight ~10MB binary)
- **Frontend:** React with TypeScript
- **State management:** Zustand (syncs with server state)
- **Rendering:** HTML/CSS for UI, Canvas or Pixi.js for the visual rack/lab view
- **Styling:** Tailwind CSS
- **Bundler:** Vite

### Mobile App (React Native)

- **Framework:** React Native with TypeScript
- **Ads:** AdMob native SDK
- **IAP:** Native Apple/Google purchase APIs
- **Push notifications:** FCM (Android) / APNs (iOS)
- **Navigation:** React Navigation

### Backend (api.homelab.living)

- **Language:** Go
- **API:** REST for actions, WebSocket for real-time server-pushed events (random events, group activity, live notifications)
- **Auth:** OAuth2 (Google, Apple, Discord) + email/password
- **Game engine:** Server-side tick system — calculates idle progress, processes events, validates client actions
- **Real-time:** Client-side interpolation for smooth counter ticking between server syncs. WebSocket for server-initiated events (drive failures, traffic spikes, group activity).
- **Anti-cheat:** All state mutations validated server-side

### Database

- **Primary:** PostgreSQL — users, game state, groups, leaderboards, transactions
- **Time-series:** TimescaleDB — uptime tracking, resource accumulation history, analytics, leaderboard snapshots

### Social & Monetization

- **Groups/Guilds:** Players form "collectives" — shared compute pools that unlock group-only benefits:
  - **Combined processing power** — members contribute idle compute toward group jobs (large contracts, distributed workloads) that pay out more than solo jobs.
  - **Shared services** — group members can deploy services across each other's racks for redundancy bonuses (e.g., distributed Nextcloud, replicated databases).
  - **Group leaderboard rank** — collective reputation unlocks group milestones (custom cosmetics, group-only events, priority colo placement).
  - **Roles:** Founder, admin, member. Founders set contribution minimums and profit splits.
- **Leaderboards:** Individual and group rankings. Categories: total compute earned, longest uptime streak, most services deployed, fastest prestige, highest colo count.
- **Ads:** Rewarded ads (watch ad → get compute bonus), banner/interstitial
- **IAP:** Cosmetics, boosters, premium content (no pay-to-win)

### Infrastructure

- **Hosting:** Self-hosted on homelab VM
- **Build & deploy:** All done locally on the VM for now. CI/CD pipeline to be added when going live.
- **Package manager:** pnpm (JS/TS), Go modules (backend)
