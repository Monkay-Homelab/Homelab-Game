# Game Balancing Reference

Current rates, costs, and progression values for tuning.

## Tier Progression

| Tier         | Cost to Next Tier | Power Limit | Cooling (base+tier) | Slots/Rack | Hardware Slots |
| ------------ | ----------------- | ----------- | ------------------- | ---------- | -------------- |
| Coffee Table | 500 CU            | 500W        | 50                  | 2 slots    | 2              |
| Closet Floor | 5,000 CU          | 1,250W      | 300                 | 5 slots    | 5              |
| 12U Rack     | 25,000 CU         | 3,750W      | 1,550               | 12U        | —              |
| 24U Rack     | 100,000 CU        | 7,500W      | 4,050               | 24U        | —              |
| 36U Rack     | 500,000 CU        | 12,500W     | 9,050               | 36U        | —              |
| 48U Rack     | — (max)           | 20,000W     | 16,550              | 48U        | —              |

Prestige cost scaling: linear +50%/colo up to 5, then ×1.5 exponential after.
Formula: `coloCount <= 5 ? 1.0 + coloCount * 0.5 : 3.5 * 1.5^(coloCount - 5)`

## Click Rewards (per click)

| Tier         | CU/click |
| ------------ | -------- |
| Coffee Table | 10       |
| Closet Floor | 50       |
| 12U Rack     | 200      |
| 24U Rack     | 800      |
| 36U Rack     | 3,000    |
| 48U Rack     | 10,000   |

## Hardware Catalog

### Compute

| Name                  | Tier         | Power  | CU/tick | Cost    | Size    |
| --------------------- | ------------ | ------ | ------- | ------- | ------- |
| Raspberry Pi 4        | Coffee Table | 15W    | 1       | 50      | 1 slot  |
| N100 Mini PC          | Coffee Table | 25W    | 6       | 200     | 1 slot  |
| HP ProDesk Mini       | Closet Floor | 45W    | 8       | 400     | 2 slots |
| Lenovo ThinkCentre    | Closet Floor | 80W    | 12      | 600     | 2 slots |
| Dell PowerEdge R620   | 12U          | 200W   | 25      | 1,500   | 1U      |
| HP ProLiant DL360     | 12U          | 220W   | 30      | 2,000   | 1U      |
| Dell PowerEdge R730   | 24U          | 350W   | 60      | 5,000   | 2U      |
| Dell PowerEdge R740xd | 36U          | 500W   | 120     | 15,000  | 2U      |
| Dell PowerEdge R750   | 48U          | 700W   | 250     | 40,000  | 2U      |
| GPU Server (4x A100)  | 48U          | 2,000W | 500     | 100,000 | 4U      |

### Storage

| Name                  | Tier         | Power | CU/tick | Cost  | Size   |
| --------------------- | ------------ | ----- | ------- | ----- | ------ |
| Synology NAS          | Closet Floor | 40W   | 3       | 500   | 1 slot |
| 2U JBOD Storage Shelf | 12U          | 80W   | 5       | 1,200 | 2U     |
| Synology RackStation  | 24U          | 100W  | 10      | 3,000 | 2U     |

### Network

| Name                     | Tier         | Power | Cost   | Size   |
| ------------------------ | ------------ | ----- | ------ | ------ |
| Unmanaged Switch 8-port  | Closet Floor | 10W   | 100    | 1 slot |
| Unmanaged Switch 24-port | 12U          | 15W   | 500    | 1U     |
| Managed Switch 24-port   | 24U          | 30W   | 1,500  | 1U     |
| 10GbE Switch             | 36U          | 50W   | 5,000  | 1U     |
| Fiber Switch 48-port     | 48U          | 80W   | 15,000 | 1U     |

### Power

| Name                  | Tier         | Cost  | Size   |
| --------------------- | ------------ | ----- | ------ |
| APC Back-UPS 600VA    | Closet Floor | 300   | 1 slot |
| CyberPower UPS 1500VA | 12U          | 800   | 2U     |
| APC UPS 3000VA        | 36U          | 4,000 | 2U     |

### Misc

| Name           | Tier | Cost | Size                    |
| -------------- | ---- | ---- | ----------------------- |
| 1U Patch Panel | 12U  | 200  | 1U                      |
| 1U Rack Shelf  | 12U  | 150  | 1U (holds 8 slot items) |

## Hardware Bonuses

Special bonuses applied by the engine on top of catalog stats. All stack additively with no cap.

### Network Switches (idle compute income bonus)

| Name                     | Bonus        |
| ------------------------ | ------------ |
| Unmanaged Switch 8-port  | +10% idle CU |
| Unmanaged Switch 24-port | +14% idle CU |
| Managed Switch 24-port   | +20% idle CU |
| 10GbE Switch             | +25% idle CU |
| Fiber Switch 48-port     | +30% idle CU |

### Storage Devices (reputation bonus)

| Name                  | Bonus           |
| --------------------- | --------------- |
| Synology NAS          | +10% reputation |
| 2U JBOD Storage Shelf | +15% reputation |
| Synology RackStation  | +25% reputation |

### UPS (flat compute bonus)

| Name                  | Bonus       |
| --------------------- | ----------- |
| APC Back-UPS 600VA    | +3 CU/tick  |
| CyberPower UPS 1500VA | +8 CU/tick  |
| APC UPS 3000VA        | +20 CU/tick |

### Patch Panel

| Name           | Bonus           |
| -------------- | --------------- |
| 1U Patch Panel | +5% reputation  |

## Sell & Event Resolution

- **Sell refund:** 60% of original CU cost
- **Event resolution cost:** 100 CU per remaining throttle tick

## Services

| Name                 | Tier         | CU/tick | Rep/tick | $/tick | Power | Cost   |
| -------------------- | ------------ | ------- | -------- | ------ | ----- | ------ |
| Pi-hole              | Coffee Table | 1       | 1        | 0      | 5W    | 20     |
| Personal Website     | Coffee Table | 1       | 2        | 0      | 5W    | 30     |
| File Share           | Coffee Table | 2       | 1        | 0      | 10W   | 50     |
| Plex                 | Closet Floor | 5       | 5        | 1      | 30W   | 200    |
| Home Assistant       | Closet Floor | 3       | 4        | 0      | 15W   | 150    |
| Nextcloud            | Closet Floor | 4       | 5        | 0      | 25W   | 250    |
| Game Server          | Closet Floor | 8       | 6        | 1      | 40W   | 300    |
| Gitea                | 12U          | 10      | 8        | 2      | 20W   | 800    |
| Grafana + Prometheus | 12U          | 8       | 10       | 0      | 30W   | 1,000  |
| Reverse Proxy        | 12U          | 5       | 12       | 0      | 10W   | 500    |
| WireGuard VPN        | 12U          | 3       | 8        | 2      | 5W    | 400    |
| TrueNAS              | 12U          | 12      | 10       | 0      | 50W   | 1,500  |
| CI/CD Pipeline       | 24U          | 20      | 15       | 3      | 40W   | 3,000  |
| Docker Swarm         | 24U          | 25      | 18       | 0      | 50W   | 4,000  |
| Mail Server          | 24U          | 10      | 20       | 0      | 20W   | 2,500  |
| Matrix/Element       | 24U          | 15      | 18       | 0      | 30W   | 3,000  |
| Frigate NVR          | 24U          | 18      | 12       | 0      | 35W   | 3,500  |
| Kubernetes Cluster   | 36U          | 60      | 40       | 0      | 100W  | 12,000 |
| ELK Stack            | 36U          | 40      | 30       | 0      | 80W   | 8,000  |
| DNS Authority        | 36U          | 20      | 35       | 0      | 15W   | 5,000  |
| Database Cluster     | 36U          | 50      | 35       | 0      | 90W   | 10,000 |
| AI/ML Training       | 48U          | 150     | 60       | 0      | 200W  | 50,000 |
| CDN Node             | 48U          | 80      | 80       | 0      | 100W  | 30,000 |
| Mastodon Instance    | 48U          | 60      | 100      | 0      | 80W   | 25,000 |
| Full IaC             | 48U          | 100     | 50       | 0      | 50W   | 20,000 |

## SaaS Services

| Name                  | Tier | Deploy Cost | Rep Required | $/customer | Max Customers | Power |
| --------------------- | ---- | ----------- | ------------ | ---------- | ------------- | ----- |
| Email Hosting         | 12U  | 5,000       | 100          | 3          | 50            | 20W   |
| Web Hosting           | 12U  | 4,000       | 80           | 2          | 100           | 15W   |
| VPN Service           | 24U  | 10,000      | 150          | 4          | 200           | 30W   |
| VPS Hosting           | 24U  | 15,000      | 200          | 8          | 50            | 60W   |
| S3-Compatible Storage | 24U  | 12,000      | 150          | 5          | 100           | 50W   |
| Managed Database      | 36U  | 30,000      | 500          | 15         | 30            | 100W  |
| Managed Kubernetes    | 36U  | 40,000      | 600          | 25         | 20            | 120W  |
| Bare Metal Hosting    | 48U  | 60,000      | 800          | 40         | 15            | 200W  |
| GPU Cloud             | 48U  | 100,000     | 1,000        | 100        | 10            | 500W  |

## Upgrades

### Cooling

| Name             | Tier         | Capacity | Cost   |
| ---------------- | ------------ | -------- | ------ |
| USB Fan          | Coffee Table | +75      | 30     |
| Box Fan          | Closet Floor | +150     | 100    |
| Blanking Panels  | 12U          | +250     | 500    |
| In-Rack Fans     | 12U          | +500     | 2,000  |
| Portable AC Unit | 24U          | +1,000   | 8,000  |
| Mini Split AC    | 36U          | +2,000   | 25,000 |
| In-Row Cooling   | 48U          | +3,750   | 80,000 |

### Networking

| Name             | Tier         | Effect | Cost   |
| ---------------- | ------------ | ------ | ------ |
| Unmanaged Switch | Closet Floor | Tier 1 | 200    |
| Managed Switch   | 12U          | Tier 2 | 2,000  |
| 10GbE Switch     | 36U          | Tier 3 | 8,000  |
| Fiber Network    | 48U          | Tier 4 | 20,000 |

### Automation (resets on prestige)

| Name              | Tier         | Idle Multiplier | Cost      |
| ----------------- | ------------ | --------------- | --------- |
| Bash Scripts      | Coffee Table | 1.2x            | 100 CU    |
| Ansible Playbooks | Closet Floor | 1.5x            | 1,000 CU  |
| Docker Compose    | 12U          | 2.0x            | 5,000 CU  |
| Kubernetes        | 36U          | 3.0x            | 50,000 CU |

Automation + SaaS costs scale with prestige (same formula as tier upgrades).

### Knowledge (persists, costs money)

| Name            | Tier         | Job Bonus | Cost    |
| --------------- | ------------ | --------- | ------- |
| CompTIA A+      | Coffee Table | +10%      | $200    |
| Linux Basics    | Coffee Table | +15%      | $300    |
| Networking CCNA | Closet Floor | +20%      | $2,000  |
| AWS/Cloud Cert  | 12U          | +25%      | $8,000  |
| RHCE            | 24U          | +30%      | $20,000 |
| CKA             | 36U          | +40%      | $60,000 |

### Component Upgrades (per hardware item)

| Component | Max Level | Base Cost | Cost Scale | Compute/level | Power Reduce/level |
| --------- | --------- | --------- | ---------- | ------------- | ------------------ |
| CPU       | 5         | 500 CU    | 2.0x       | +5            | 0                  |
| RAM       | 5         | 300 CU    | 2.0x       | +3            | 0                  |
| Storage   | 5         | 400 CU    | 2.0x       | +2            | 0                  |
| NIC       | 3         | 600 CU    | 2.5x       | +1            | -5W                |

## Prestige & Colo

- **Colo requirement:** 48U Rack + SaaS unlocked
- **Max colos:** 100
- **Multiplier formula:** 1 + sum(0.5 / (1 + i \* 0.1)) for each colo
- **Colo rack diminishing returns:** rack N produces base × 0.9^(N-1). Rack 5 = 65%, rack 10 = 39%.
- **Datacenter tiers:** Tier 1 (0-2 colos), Tier 2 (3-5), Tier 3 (6-9), Tier 4 (10+)
- **What resets:** Tier, CU, Rep, Money, Power, Hardware, Services, Customers, Expenses, Automation, Networking, SaaS unlock
- **What persists:** Knowledge points, Colo count/multiplier, Datacenter ownership

## Endgame

- **Build datacenter:** 5+ colos, $500K + 5M CU
- **Datacenter levels:** 1-5 (Small → Hyperscale)
- **DC income multiplier:** 1.5x at level 1 (hardcoded), then 1.0 + (level × 0.25) for levels 2-5 — max 2.25x at level 5
- **DC upgrade cost:** $250K × (level+1) + 2M CU × (level+1)

## SaaS Unlock

- **Requirement:** Any rack tier + 100 reputation + 10,000 CU
- **Business expenses:** $8/tick total (internet $2, domains $1, SSL $1, insurance $3, accounting $1)
- **Customer growth:** Tiered — interval = `60 / (1 + customers * 0.1)` seconds per new customer, up to `max_customers`. First customers ~60s apart, accelerates with word of mouth. At 20 customers: ~20s interval. At 50: ~10s.

## Events

- **Frequency:** ~2% chance per 5-second poll (~1 event per 1-2 minutes)
- **Severity weights:** Minor 3x, Moderate 2x, Major 1x

## Group Bonus

- **Per member:** +5% compute bonus per additional member
- **Cap:** +50% (11 members)

## Multiplier Stack

**Compute:** Colo × Idle (automation) × Heat penalty × Event throttle × Knowledge boost × Network bonus
**Reputation:** Heat penalty × Event throttle × (1 + Storage bonus + Patch Panel bonus)
**Money:** Heat penalty × Event throttle

- Colo multiplier: diminishing returns, ~1.5x first prestige → ~4.6x at 10
- Idle (automation): 1.0x-3.0x (resets on prestige)
- Heat penalty: 0.5x if overheating, 1.0x otherwise
- Event throttle: varies (0.01x-0.8x during events)
- Knowledge boost: 1 + (knowledge_points / 100) — max 2.4x with all certs (140 total points)
- Network bonus: 1 + sum of switch bonuses (0.10-0.30 per switch type)
- Storage bonus: +0.10 (Synology NAS), +0.15 (JBOD), +0.25 (RackStation)
- Patch Panel bonus: +0.05 per patch panel
- Group bonus: +5% per member, capped at +50% (applied additively on raw compute)
- Datacenter multiplier: 1.5x at level 1, up to 2.25x at level 5 (applies to colo rack income only)

## Progression Timing Estimates

### First Playthrough (no prestige bonuses, 1.0x multiplier)

Assumptions: active clicking (~2 clicks/sec) + idle income from hardware/services. No group bonus.

| Transition      | Cost    | Click/sec | Idle/sec (built up) | Total time |
| --------------- | ------- | --------- | ------------------- | ---------- |
| Coffee → Closet | 500     | 20        | ~11                 | ~30 sec    |
| Closet → 12U    | 5,000   | 100       | ~50                 | ~1-2 min   |
| 12U → 24U       | 25,000  | 400       | ~200                | ~2-3 min   |
| 24U → 36U       | 100,000 | 1,600     | ~430                | ~3-5 min   |
| 36U → 48U       | 500,000 | 6,000     | ~2,700              | ~5-8 min   |
| **Total**       |         |           |                     | ~10-15 min |

## Open Issues

### Component Upgrades Are Too Weak at Endgame

Current component upgrades use **flat additive bonuses** (+5, +3, +2, +1 per level) that don't scale with hardware base compute. At endgame a GPU Server has 500 base compute — maxing all components adds +53, only ~10% increase.

**Proposed fix:** Change `ComputeAdd` from flat values to **percentage of hardware base compute**:

| Component | Current (flat/level) | Proposed (%/level) | Max total |
| --------- | -------------------- | ------------------ | --------- |
| CPU       | +5                   | +15%               | +75%      |
| RAM       | +3                   | +10%               | +50%      |
| Storage   | +2                   | +8%                | +40%      |
| NIC       | +1                   | +5%                | +15%      |

### Money Economy Scaling

- **5 services generate money before SaaS:** Plex ($1), Game Server ($1), Gitea ($2), WireGuard VPN ($2), CI/CD Pipeline ($3) = $9/tick total if all deployed.
- **Customer growth is implemented** — tiered interval formula grows customers over time up to `max_customers` per SaaS service. Max SaaS revenue at full customers: $4,600/tick ($4,592/tick net after $8 expenses).
- **Datacenter costs $500K + 5M CU** — at full SaaS revenue (~$4,592/tick) the money portion takes ~2 minutes.
