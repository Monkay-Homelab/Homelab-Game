# Game Mechanics Reference

Complete reference for game balancing, currency calculations, and progression — with exact file locations.

---

## Core Currency Types

**Defined in:** `apps/backend/internal/models/game_state.go`
**Shared types:** `packages/shared/src/types/game.ts`

| Currency             | Field                             | Purpose                                                               |
| -------------------- | --------------------------------- | --------------------------------------------------------------------- |
| **Compute Units**    | `ComputeUnits` (int64)            | Primary currency — buy hardware, services, upgrades, tier progression |
| **Reputation**       | `Reputation` (int64)              | Unlock SaaS, deploy SaaS services, mitigation                         |
| **Money**            | `Money` (int64)                   | Knowledge upgrades, business expenses, datacenter build/upgrade       |
| **Power**            | `PowerWatts` / `PowerLimit` (int) | Current draw vs capacity; triggers heat penalty if exceeded           |
| **Heat**             | `HeatGenerated` (int)             | Equals power draw; throttle if > cooling capacity                     |
| **Network Tier**     | `NetworkTier` (int 0-4)           | Passive idle income bonus from switches                               |
| **Knowledge Points** | `KnowledgePoints` (int)           | +1% job reward per point; persists through prestige                   |
| **Automation Tier**  | `AutomationTier` (int)            | Idle multiplier upgrade level (1.2x to 3.0x)                          |

---

## Tier Progression

| Tier         | Cost to Next Tier | Power Limit | Cooling (base+tier) | Slots/Rack | Hardware Slots |
| ------------ | ----------------- | ----------- | ------------------- | ---------- | -------------- |
| Coffee Table | 500 CU            | 500W        | 50                  | 2 slots    | 2              |
| Closet Floor | 5,000 CU          | 1,250W      | 300                 | 5 slots    | 5              |
| 12U Rack     | 25,000 CU         | 3,750W      | 1,550               | 12U        | —              |
| 24U Rack     | 100,000 CU        | 7,500W      | 4,050               | 24U        | —              |
| 36U Rack     | 500,000 CU        | 12,500W     | 9,050               | 36U        | —              |
| 48U Rack     | — (max)           | 20,000W     | 16,550              | 48U        | —              |

**File:** `apps/backend/internal/game/engine/engine.go` — `upgradeTier()`

Prestige cost scaling: linear +50%/colo up to 5, then x1.5 exponential after.
Formula: `coloCount <= 5 ? 1.0 + coloCount * 0.5 : 3.5 * 1.5^(coloCount - 5)`

```go
prestigeScale := 1.0 + float64(gs.ColoCount)*0.5
cost := int64(float64(baseCost) * prestigeScale)
```

### Tier Flavor Text (Job Click Messages)

Each tier has themed click messages returned via config:

| Tier         | Example Jobs                                                            |
| ------------ | ----------------------------------------------------------------------- |
| Coffee Table | "Compiling a script...", "Running apt update...", "Downloading ISO..."  |
| Closet Floor | "Transcoding video...", "Building Docker image...", "Running backup..." |
| 12U Rack     | "Deploying containers...", "Running Ansible playbook..."                |
| 24U Rack     | "CI/CD pipeline running...", "Swarm service scaling..."                 |
| 36U Rack     | "K8s pod scheduling...", "ELK ingesting logs..."                        |
| 48U Rack     | "Training ML model...", "CDN cache warming...", "Terraform applying..." |

---

## Slot & Shelf System

**File:** `apps/backend/internal/game/engine/engine.go`

### Pre-Rack Tiers (Coffee Table, Closet Floor)

Hardware uses **slots**. Each tier has a fixed slot count (2, 5). Hardware occupies 1-2 slots each.

### Rack Tiers (12U-48U)

Two space systems coexist:

1. **Rack Units (U)** — rack-mountable hardware (servers, switches, UPS, etc.) uses rack units directly
2. **Shelf Slots** — small items (Raspberry Pi, mini PC, etc.) need a **Rack Shelf** first
   - Each Rack Shelf provides **8 slots** for small items
   - Rack Shelf itself occupies 1U
   - Cannot sell a shelf if items are still on it (error: "cannot sell shelf — N items still on shelves")

---

## Click/Job Rewards

**File:** `apps/backend/internal/game/engine/engine.go` — `runJob()`

| Tier         | CU/click |
| ------------ | -------- |
| Coffee Table | 10       |
| Closet Floor | 50       |
| 12U Rack     | 200      |
| 24U Rack     | 800      |
| 36U Rack     | 3,000    |
| 48U Rack     | 10,000   |

```go
reward := tierJobReward(gs.Tier)
// Clicks only get knowledge boost — colo multiplier applies to idle income only
knowledgeBoost := 1.0 + float64(gs.KnowledgePoints)/100.0
gs.ComputeUnits += int64(float64(reward) * knowledgeBoost)
```

**Important:** Click rewards are NOT affected by colo multiplier, idle multiplier, heat penalty, or event throttle — only knowledge boost.

---

## Hardware Catalog

**Buy:** `apps/backend/internal/game/engine/engine.go` — `buyHardware()`
**Catalog:** `apps/backend/internal/game/catalog/hardware.go`

### Compute

| Name                  | Tier         | Type       | Power  | CU/tick | Cost    | Size    |
| --------------------- | ------------ | ---------- | ------ | ------- | ------- | ------- |
| Raspberry Pi 4        | Coffee Table | sbc        | 15W    | 1       | 50      | 1 slot  |
| N100 Mini PC          | Coffee Table | mini_pc    | 25W    | 6       | 200     | 1 slot  |
| HP ProDesk Mini       | Closet Floor | desktop    | 45W    | 8       | 400     | 2 slots |
| Lenovo ThinkCentre    | Closet Floor | desktop    | 80W    | 12      | 600     | 2 slots |
| Dell PowerEdge R620   | 12U          | server     | 200W   | 25      | 1,500   | 1U      |
| HP ProLiant DL360     | 12U          | server     | 220W   | 30      | 2,000   | 1U      |
| Dell PowerEdge R730   | 24U          | server     | 350W   | 60      | 5,000   | 2U      |
| Dell PowerEdge R740xd | 36U          | server     | 500W   | 120     | 15,000  | 2U      |
| Dell PowerEdge R750   | 48U          | server     | 700W   | 250     | 40,000  | 2U      |
| GPU Server (4x A100)  | 48U          | gpu_server | 2,000W | 500     | 100,000 | 4U      |

### Storage

| Name                  | Tier         | Type | Power | CU/tick | Cost  | Size   |
| --------------------- | ------------ | ---- | ----- | ------- | ----- | ------ |
| Synology NAS          | Closet Floor | nas  | 40W   | 3       | 500   | 1 slot |
| 2U JBOD Storage Shelf | 12U          | nas  | 80W   | 5       | 1,200 | 2U     |
| Synology RackStation  | 24U          | nas  | 100W  | 10      | 3,000 | 2U     |

### Network

| Name                     | Tier         | Type   | Power | Cost   | Size   |
| ------------------------ | ------------ | ------ | ----- | ------ | ------ |
| Unmanaged Switch 8-port  | Closet Floor | switch | 10W   | 100    | 1 slot |
| Unmanaged Switch 24-port | 12U          | switch | 15W   | 500    | 1U     |
| Managed Switch 24-port   | 24U          | switch | 30W   | 1,500  | 1U     |
| 10GbE Switch             | 36U          | switch | 50W   | 5,000  | 1U     |
| Fiber Switch 48-port     | 48U          | switch | 80W   | 15,000 | 1U     |

### Power

| Name                  | Tier         | Type | Power | Cost  | Size   |
| --------------------- | ------------ | ---- | ----- | ----- | ------ |
| APC Back-UPS 600VA    | Closet Floor | ups  | 0W    | 300   | 1 slot |
| CyberPower UPS 1500VA | 12U          | ups  | 0W    | 800   | 2U     |
| APC UPS 3000VA        | 36U          | ups  | 0W    | 4,000 | 2U     |

**Note:** UPS hardware has 0W power draw.

### Misc

| Name           | Type        | Tier | Cost | Size                    |
| -------------- | ----------- | ---- | ---- | ----------------------- |
| 1U Patch Panel | patch_panel | 12U  | 200  | 1U                      |
| 1U Rack Shelf  | shelf       | 12U  | 150  | 1U (holds 8 slot items) |

### Sell & Event Resolution

- **Sell refund:** 60% of original CU cost
- **Event resolution cost:** 100 CU per remaining throttle tick
- **Sell shelf restriction:** Cannot sell a shelf if slot-based items are still using its capacity

---

## Hardware Bonuses

**File:** `apps/backend/internal/game/engine/config.go`

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

| Name           | Bonus          |
| -------------- | -------------- |
| 1U Patch Panel | +5% reputation |

---

## Services

**Deploy:** `apps/backend/internal/game/engine/engine.go` — `deployService()`
**Catalog:** `apps/backend/internal/game/catalog/services.go`

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

**Pre-SaaS money income:** 5 services generate money — Plex ($1), Game Server ($1), Gitea ($2), WireGuard VPN ($2), CI/CD Pipeline ($3) = $9/tick total if all deployed.

---

## Upgrades

**File:** `apps/backend/internal/game/engine/engine.go` — `buyUpgrade()`
**Catalog:** `apps/backend/internal/game/catalog/upgrades.go`

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

Networking upgrades are one-way — you can only buy equal or higher tier. Lower tiers are skipped.

### Automation (resets on prestige)

| Name              | Tier         | Idle Multiplier | Cost      |
| ----------------- | ------------ | --------------- | --------- |
| Bash Scripts      | Coffee Table | 1.2x            | 100 CU    |
| Ansible Playbooks | Closet Floor | 1.5x            | 1,000 CU  |
| Docker Compose    | 12U          | 2.0x            | 5,000 CU  |
| Kubernetes        | 36U          | 3.0x            | 50,000 CU |

Automation + SaaS costs scale with prestige (same formula as tier upgrades).

### Knowledge (persists through prestige, costs money)

| Name            | Tier         | Job Bonus | Cost    | Knowledge Points |
| --------------- | ------------ | --------- | ------- | ---------------- |
| CompTIA A+      | Coffee Table | +10%      | $200    | 10               |
| Linux Basics    | Coffee Table | +15%      | $300    | 15               |
| Networking CCNA | Closet Floor | +20%      | $2,000  | 20               |
| AWS/Cloud Cert  | 12U          | +25%      | $8,000  | 25               |
| RHCE            | 24U          | +30%      | $20,000 | 30               |
| CKA             | 36U          | +40%      | $60,000 | 40               |

### Component Upgrades (per hardware item)

**File:** `apps/backend/internal/game/engine/engine.go` — `upgradeComponent()`
**Catalog:** `apps/backend/internal/game/catalog/upgrades.go`

**Upgradeable hardware types:** `server`, `desktop`, `sbc`, `mini_pc`, `gpu_server` only. Storage, switches, UPS, shelves, and patch panels cannot be upgraded.

**Cost formula:** `BaseCost * (CostScale ^ currentLevel)`

| Component | Max Level | Base Cost | Cost Scale | Compute/level       | Power Reduce/level |
| --------- | --------- | --------- | ---------- | ------------------- | ------------------ |
| CPU       | 5         | 500 CU    | 2.0x       | +5% of base compute | 0                  |
| RAM       | 5         | 300 CU    | 2.0x       | +5% of base compute | 0                  |
| Storage   | 5         | 400 CU    | 2.0x       | +5% of base compute | 0                  |
| NIC       | 3         | 600 CU    | 2.5x       | +5% of base compute | -5W                |

Example: CPU level 0->1 = 500 CU, level 1->2 = 1,000 CU, level 2->3 = 2,000 CU, etc.

Component upgrade bonuses are **percentage-based** and **cumulative** — level 3 CPU on a 500-base GPU Server gives `500 * 15% = +75` compute.

Max upgrade bonus per hardware item: `+90%` (25% CPU + 25% RAM + 25% Storage + 15% NIC).

| Hardware             | Base | Max Upgraded | Gain |
| -------------------- | ---- | ------------ | ---- |
| Raspberry Pi 4       | 1    | 1            | +0   |
| N100 Mini PC         | 6    | 11           | +5   |
| Dell PowerEdge R620  | 25   | 47           | +22  |
| Dell PowerEdge R750  | 250  | 475          | +225 |
| GPU Server (4x A100) | 500  | 950          | +450 |

---

## SaaS System

**File:** `apps/backend/internal/game/engine/engine.go`

### SaaS Unlock

- **Requirement:** Any rack tier + 100 reputation + 10,000 CU (base)
- **CU cost scales with prestige:** `10,000 * prestigeCostScale(coloCount)`
- **Business expenses:** $8/tick total (internet $2, domains $1, SSL $1, insurance $3, accounting $1)

### SaaS Services

**Catalog:** `apps/backend/internal/game/catalog/saas.go`

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

Deploy costs scale with prestige: `deployCost * prestigeCostScale(coloCount)`

### SaaS Deploy Behavior

When a SaaS service is deployed:

1. CU cost deducted (scaled by prestige)
2. Power draw added
3. A **Service** record is created with `ComputePerTick: 0` and `ReputationPerTick: 5` — SaaS services generate reputation, not compute
4. The service's `MoneyPerTick` starts at the per-customer revenue rate (1 customer)
5. An **initial Customer** is created immediately with `Satisfaction: 100`
6. `TotalCustomers` counter incremented

### Business Expenses (created on SaaS unlock)

**Catalog:** `apps/backend/internal/game/catalog/saas.go`

| Expense              | Cost/Tick | Type           |
| -------------------- | --------- | -------------- |
| Business Internet    | 2         | infrastructure |
| Domain Registrations | 1         | infrastructure |
| SSL Certificates     | 1         | infrastructure |
| Business Insurance   | 3         | legal          |
| Accounting Software  | 1         | operations     |
| **TOTAL**            | **8**     | -              |

Deducted from Money each tick.

### Customer Growth

**File:** `apps/backend/internal/api/handlers/game.go` — `processCustomerGrowth()`

Uses a separate timer (`LastCustomerGrowthAt`) independent of the idle tick timer so 5-second polls don't starve the growth timer.

**Growth formula:** `interval = 60 / (1 + customers * 0.1)` seconds per new customer, up to `max_customers`.

| Current Customers | Interval Between New Customers |
| ----------------- | ------------------------------ |
| 0                 | ~60s                           |
| 10                | ~30s                           |
| 20                | ~20s                           |
| 50                | ~10s                           |

**Timer behavior:** The growth timer only advances when customers actually grew. If the elapsed time isn't enough for a new customer, the partial time accumulates until the next poll.

**Service revenue update:** When new customers join, the SaaS service's `MoneyPerTick` is dynamically updated to reflect total revenue: `customerCount * revenuePerCustomer`.

### Customer Satisfaction Decay

When overheating or throttled by an active event, customer satisfaction decreases:

```go
// -1 satisfaction per minute of elapsed time
if len(customers) > 0 && (gs.HeatGenerated > gs.CoolingCapacity || gs.ThrottleMultiplier < 1.0) {
    decay := int(seconds / 60.0 * float64(1))
    for i := range customers {
        customers[i].Satisfaction -= decay
        if customers[i].Satisfaction < 0 { customers[i].Satisfaction = 0 }
    }
}
```

- **Rate:** -1 satisfaction per minute
- **Triggers:** Heat > cooling capacity OR throttle multiplier < 1.0
- **Floor:** Satisfaction cannot go below 0
- **Impact:** Affects SaaS customer retention/churn risk

### SaaS Revenue

- **Max SaaS revenue:** $4,600/tick at full customers ($4,592/tick net after $8 expenses).
- **Datacenter money timing:** At full SaaS revenue (~$4,592/tick) the $500K money portion takes ~2 minutes.

---

## Idle Income (Per-Tick Calculation)

**File:** `apps/backend/internal/game/engine/engine.go` — `ProcessIdleProgress()`

All income is calculated per elapsed second since `LastTickAt`.

### Recalculations (every tick, regardless of elapsed time)

Before income calculation, the engine recalculates from current state:

- **Power draw:** Sum of hardware power (with component NIC reductions) + service power
- **Heat generated:** Equals recalculated power draw
- **Cooling capacity:** `baseCooling (50) + tierCoolingBonus(tier) + coolingUpgrades`
- **Power limit:** From tier definition
- **Used slots:** Recounted from hardware in rack tiers

### Throttle Decay

Each tick, `ThrottleTicksRemaining` decrements by 1. When it reaches 0, `ThrottleMultiplier` resets to 1.0.

### Compute Units Income

```go
gs.ComputeUnits += int64(float64(totalCompute) * seconds * totalMultiplier * knowledgeBoost * netMult)
```

**Sources of Compute:**

1. **Hardware Compute:**
   - Base: `hardware[i].ComputePerTick`
   - Component upgrades add bonus: `cu.ComputeBonus` per upgrade level
   - UPS flat bonuses: +3 / +8 / +20

2. **Service Compute:**
   - Sum of `service[i].ComputePerTick`

3. **Colo Rack Compute** (calculated in handler, not engine):
   - Each colo rack: `cr.ComputePerTick * elapsed * dcMult * decay`
   - `dcMult` = datacenter income multiplier (1.0 if none, 1.5x to 2.25x)
   - `decay` = `0.9^(rackIndex)` — each subsequent rack earns 90% of the previous
   - **NOT affected** by heat penalty, throttle, knowledge boost, or network bonus

4. **Group Bonus Compute** (calculated in handler):
   - `(rawHwCompute + serviceCompute) * elapsed * (groupBonus - 1.0)`
   - Additive, not multiplicative — applied on raw base, not on final multiplied value
   - **NOT affected** by other multipliers (colo, idle, heat, throttle, knowledge, network)

**Multipliers:**

```go
totalMultiplier := gs.ColoMultiplier * gs.IdleMultiplier * heatPenalty * eventThrottle
```

| Multiplier       | Source                               | Range          |
| ---------------- | ------------------------------------ | -------------- |
| `ColoMultiplier` | Prestige count (diminishing returns) | 1.0x - ~3.47x+ |
| `IdleMultiplier` | Automation upgrades                  | 1.0x - 3.0x    |
| `heatPenalty`    | 0.5 if overheating, else 1.0         | 0.5x or 1.0x   |
| `eventThrottle`  | Active event effects                 | 0.01x - 1.0x   |

**Additional Bonuses:**

- **Knowledge Boost:** `1.0 + knowledgePoints/100.0`
- **Network Bonus:** `1.0 + networkBonus` where networkBonus is sum of owned switches' bonus values

### Reputation Income

```go
gs.Reputation += int64(float64(serviceRep) * seconds * heatPenalty * eventThrottle * repMult)
```

- `serviceRep`: Sum of `service[i].ReputationPerTick`
- Colo racks: `cr.ReputationPerTick * elapsed * dcMult * decay` (added in handler)
- `repMult`: `1.0 + storageBonus + patchPanelBonus`

### Money Income

```go
gs.Money += int64(float64(serviceMoney) * seconds * heatPenalty * eventThrottle)
gs.Money -= int64(float64(totalExpenses) * seconds)
```

- `serviceMoney`: Sum of `service[i].MoneyPerTick`
- Colo racks: `cr.MoneyPerTick * elapsed * dcMult * decay` (added in handler)
- Expenses deducted per tick
- Money floor: 0 (never goes negative)

---

## Events

**File:** `apps/backend/internal/game/events/events.go`

### Event System Overview

- **Frequency:** ~2% chance per 5-second poll (~1 event per 1-2 minutes)
- **Severity weights:** Minor 3x, Moderate 2x, Major 1x
- **Events pushed via WebSocket** to connected clients in real-time

### Event Pool Inheritance

Events cascade by tier — higher tiers inherit some lower-tier events:

| Player Tier     | Event Pools Included                              |
| --------------- | ------------------------------------------------- |
| Coffee Table    | Coffee Table + Software                           |
| Closet Floor    | Coffee Table + Closet Floor + Software            |
| 12U-48U Rack    | Closet Floor + Rack + Software (NOT Coffee Table) |
| + SaaS unlocked | Above + SaaS events                               |
| + Colo count >0 | Above + Colo events                               |

### Mitigation System

Events can be **mitigated** if the player owns specific hardware or upgrades. Mitigated events:

- Still fire and appear in the event log
- Have all effects nullified (empty effect applied)
- Description appended with "(Mitigated!)"
- Mitigation checks by upgrade name or hardware type

### Throttle Behavior

- Events set `ThrottleMultiplier` and `ThrottleTicksRemaining` on the game state
- Events that set throttle to **0** are adjusted to **0.01** (near-zero, not fully zero)
- Throttle decays by 1 tick per idle poll
- Can be manually resolved by paying `100 CU * throttleTicksRemaining`

### Complete Event Reference

#### Coffee Table Events

| Event              | Type            | Severity | Compute Loss | Rep Loss | Money Loss | Slot Loss | Throttle  | Mitigated By   |
| ------------------ | --------------- | -------- | ------------ | -------- | ---------- | --------- | --------- | -------------- |
| Cat Knocked Server | cat_attack      | Minor    | -            | -        | -          | -         | 0.01x, 3t | -              |
| Noise Complaint    | noise_complaint | Minor    | -            | -        | -          | 1         | -, 5t     | USB Fan        |
| Spilled Drink      | spilled_drink   | Moderate | 50           | -        | -          | -         | -         | -              |
| Power Flickered    | power_flicker   | Minor    | -            | -        | -          | -         | 0.01x, 3t | UPS (hardware) |

#### Closet Floor Events

| Event              | Type                | Severity | Compute Loss | Rep Loss | Money Loss | Slot Loss | Throttle  | Mitigated By      |
| ------------------ | ------------------- | -------- | ------------ | -------- | ---------- | --------- | --------- | ----------------- |
| Closet Overheating | overheating         | Moderate | -            | -        | -          | -         | 0.5x, 10t | Box Fan           |
| Tripped Breaker    | tripped_breaker     | Major    | -            | 10       | -          | -         | 0.01x, 5t | Dedicated Circuit |
| Cable Spaghetti    | cable_spaghetti     | Minor    | -            | -        | -          | -         | 0.75x, 8t | Cable Organizer   |
| Power Outage       | power_outage_closet | Major    | -            | 20       | -          | -         | 0.01x, 6t | UPS (hardware)    |
| Electricity Bill   | spouse_aggro        | Moderate | -            | -        | 100        | 1         | -, 10t    | -                 |

#### Rack Tier Events (12U-48U)

| Event                | Type            | Severity | Compute Loss | Rep Loss | Money Loss | Throttle   | Mitigated By   |
| -------------------- | --------------- | -------- | ------------ | -------- | ---------- | ---------- | -------------- |
| Power Outage         | power_outage    | Major    | -            | 50       | -          | 0.01x, 8t  | UPS (hardware) |
| Drive Failure        | drive_failure   | Major    | 500          | 30       | -          | -          | NAS (hardware) |
| ISP Outage           | isp_outage      | Major    | -            | 40       | -          | 0.01x, 12t | -              |
| Noise from Neighbors | noise_neighbors | Minor    | -            | -        | -          | 0.8x, 6t   | In-Rack Fans   |
| Firmware Brick       | firmware_brick  | Moderate | 200          | -        | -          | -, 5t      | -              |

#### Software Events (any tier)

| Event                | Type             | Severity | Compute Loss | Rep Loss | Money Loss | Throttle  | Mitigated By |
| -------------------- | ---------------- | -------- | ------------ | -------- | ---------- | --------- | ------------ |
| Kernel Update        | kernel_panic     | Moderate | -            | -        | -          | 0.01x, 6t | -            |
| Security Breach      | security_breach  | Major    | 300          | 100      | -          | -         | -            |
| DNS Misconfiguration | dns_misconfig    | Moderate | -            | 20       | -          | 0.5x, 8t  | -            |
| Certificate Expired  | cert_expired     | Minor    | -            | 15       | -          | -, 4t     | Bash Scripts |
| Dependency Broke     | dependency_broke | Minor    | 50           | -        | -          | 0.75x, 5t | -            |

#### SaaS/Customer Events (requires SaaS unlocked)

| Event               | Type               | Severity | Compute Loss | Rep Loss | Money Loss | Throttle  | Notes                      |
| ------------------- | ------------------ | -------- | ------------ | -------- | ---------- | --------- | -------------------------- |
| Reddit Hug of Death | hug_of_death       | Moderate | -            | 50       | -          | 0.3x, 10t |                            |
| Support Ticket      | support_ticket     | Minor    | -            | 10       | 50         | -         |                            |
| Enterprise Inquiry  | enterprise_inquiry | Minor    | -            | -        | -          | -         | Positive event (no damage) |
| Chargeback Filed    | chargeback         | Moderate | -            | 20       | 200        | -         |                            |
| TOS Abuse           | tos_abuse          | Minor    | -            | 30       | -          | -         |                            |

#### Colo Events (requires colo count > 0)

| Event               | Type              | Severity | Compute Loss | Rep Loss | Money Loss | Throttle | Notes                      |
| ------------------- | ----------------- | -------- | ------------ | -------- | ---------- | -------- | -------------------------- |
| DC Maintenance      | dc_maintenance    | Minor    | -            | 20       | 50         | -        |                            |
| Bandwidth Overage   | bandwidth_overage | Moderate | -            | -        | 500        | -        |                            |
| Remote Hands Needed | remote_hands      | Moderate | -            | -        | 300        | 0.8x, 5t |                            |
| Cross-Connect       | cross_connect     | Minor    | -            | -        | -          | -        | Positive event (no damage) |
| Lease Renewal       | lease_renewal     | Moderate | -            | -        | 1,000      | -        |                            |

### Apply Event

```go
if event.Effect.ComputeLoss > 0 {
    gs.ComputeUnits -= event.Effect.ComputeLoss
    if gs.ComputeUnits < 0 { gs.ComputeUnits = 0 }
}
// Same pattern for ReputationLoss, MoneyLoss
```

All currency losses are floored at 0 (never go negative).

---

## Prestige & Colo

**File:** `apps/backend/internal/game/engine/engine.go` — `prestige()`

- **Colo requirement:** 48U Rack + SaaS unlocked
- **Max colos:** 100

### Colo Multiplier Formula

```go
mult := 1.0
for i := 0; i < newColoCount; i++ {
    mult += 0.5 / (1.0 + float64(i)*0.1)
}
```

| Colo Count | Multiplier |
| ---------- | ---------- |
| 1          | 1.50x      |
| 2          | ~1.95x     |
| 3          | ~2.37x     |
| 5          | ~3.11x     |
| 10         | ~4.59x     |

### Colo Rack Snapshot

On prestige, a snapshot of the current rack's total income is captured:

- **Compute:** Sum of all `hardware[i].ComputePerTick` (including component upgrade bonuses) + `service[i].ComputePerTick`
- **Reputation:** Sum of all `service[i].ReputationPerTick`
- **Money:** Sum of all `service[i].MoneyPerTick`

Component upgrade bonuses are included in the snapshot — players who invest in upgrades get a higher colo rack income.

### Colo Rack Diminishing Returns

Rack N produces base x 0.9^(N-1). Rack 5 = 65%, rack 10 = 39%.

### Datacenter Tier Assignment

Based on colo count at time of prestige:

| Colo Count | Datacenter Tier | Name                      |
| ---------- | --------------- | ------------------------- |
| 1-2        | Tier 1          | Basic                     |
| 3-5        | Tier 2          | Redundant Power           |
| 6-9        | Tier 3          | Concurrently Maintainable |
| 10+        | Tier 4          | Fault Tolerant            |

### What Resets

Tier, CU, Rep, Money, Power, Hardware, Services, Customers, Expenses, Automation, Networking, SaaS unlock, TotalCustomers, Throttle state

### What Persists

Knowledge points, Colo count/multiplier, Datacenter ownership/level, Knowledge upgrades (persistent flag)

---

## Endgame / Datacenter

**File:** `apps/backend/internal/game/engine/engine.go`

### Build Datacenter

- **Requirement:** 5+ colos
- **Cost:** $500,000 Money + 5,000,000 CU
- **Effect:** Sets `DatacenterIncomeMultiplier = 1.5x` (Level 1 — "Small Facility")

### Upgrade Datacenter

- **Max Level:** 5
- **Cost per level:** `250,000 * (level+1) Money` + `2,000,000 * (level+1) CU` (where level = current DatacenterLevel)
- **Multiplier formula:** `1.25 + level * 0.25` (applied after level increment)

| Level | Name            | Multiplier | Money Cost | CU Cost    |
| ----- | --------------- | ---------- | ---------- | ---------- |
| 1     | Small Facility  | 1.50x      | (build)    | (build)    |
| 2     | Medium Facility | 1.75x      | 500,000    | 4,000,000  |
| 3     | Large Facility  | 2.00x      | 750,000    | 6,000,000  |
| 4     | Campus          | 2.25x      | 1,000,000  | 8,000,000  |
| 5     | Hyperscale      | 2.50x      | 1,250,000  | 10,000,000 |

---

## Group Bonus

- **Per member:** +5% compute bonus per additional member (beyond yourself)
- **Cap:** +50% (11 total members = 10 additional)
- **Calculation:** `bonus = 1.0 + (memberCount - 1) * 0.05`, capped at 1.5
- **Applied additively** to raw hardware + service compute, NOT multiplicatively with other multipliers

---

## Complete Multiplier Stack

**Compute:** Colo x Idle (automation) x Heat penalty x Event throttle x Knowledge boost x Network bonus
**Reputation:** Heat penalty x Event throttle x (1 + Storage bonus + Patch Panel bonus)
**Money:** Heat penalty x Event throttle

| Multiplier      | Range            | Applies To            | Source               |
| --------------- | ---------------- | --------------------- | -------------------- |
| ColoMultiplier  | 1.0x - unbounded | CU idle               | Prestige count       |
| IdleMultiplier  | 1.0x - 3.0x      | CU idle               | Automation upgrades  |
| HeatPenalty     | 0.5x or 1.0x     | CU + Rep + Money idle | Power > cooling      |
| EventThrottle   | 0.01x - 1.0x     | CU + Rep + Money idle | Active events        |
| KnowledgeBoost  | 1.0x - 2.4x      | CU idle + job click   | Knowledge points     |
| NetworkBonus    | 1.0x - 1.3x      | CU idle               | Network switches     |
| StorageBonus    | 1.0x - 1.5x      | Rep idle              | Storage hardware     |
| PatchPanelBonus | +0.05 each       | Rep idle              | Patch panel hardware |
| DatacenterMult  | 1.5x - 2.50x     | Colo rack income only | Datacenter level     |
| GroupBonus      | 1.0x - 1.5x      | CU idle (additive)    | Group member count   |

**What is NOT multiplied:**

- Click/job rewards: Only knowledge boost applies
- Colo rack income: Only datacenter multiplier and decay apply
- Group bonus compute: Applied additively on raw base, bypasses all other multipliers

---

## Frontend Rate Calculation (Client-Side Mirror)

**File:** `apps/desktop/src/hooks/useIdleTick.ts`

The frontend replicates the server's idle formula for smooth interpolation:

```typescript
const computeRate = baseComputeRate + coloComputeRate + groupComputeRate;
const repRate = serviceRep * heatPenalty * throttle * repMult + coloRepRate;
const moneyRate =
  serviceMoney * heatPenalty * throttle + coloMoneyRate - totalExpenses;
```

**Components:**

1. Hardware compute (with component upgrades)
2. UPS flat bonuses
3. Network bonus multiplier
4. Storage/patch panel reputation multiplier
5. Service compute/rep/money
6. Colo rack income (datacenter multiplier applied)
7. Group bonus (additive on raw hw+service compute)
8. Heat penalty
9. Event throttle
10. Knowledge boost

**Display:** `apps/desktop/src/components/CurrencyBar.tsx` — shows CU + rate/sec, REP, PWR draw/limit, Money, throttle status

---

## Currency Flow Summary

```
COMPUTE UNITS
  IN:  Hardware ticks + Service ticks + Colo racks + Group bonus + Click/job
  OUT: Hardware, Services, Tier upgrades, Cooling/Net/Auto upgrades,
       Components, SaaS unlock/deploy, Datacenter, Event resolution
  MULT: Colo * Idle * HeatPenalty * Throttle * Knowledge * Network

REPUTATION
  IN:  Service ticks (scaled by storage/patch bonus)
  OUT: SaaS unlock requirement, SaaS deploy requirement
  LOST: Events

MONEY
  IN:  Service MoneyPerTick + Colo rack money
  OUT: Knowledge upgrades, Datacenter build/upgrade, Business expenses
  LOST: Events

POWER
  IN:  Hardware draw + Service draw
  CAP: Tier PowerLimit + Cooling upgrades
  EFFECT: Overheating = 0.5x all income

KNOWLEDGE POINTS (persistent)
  IN:  Knowledge upgrades (money cost)
  EFFECT: +1% per point to job reward and idle compute
```

---

## Action Types

All supported engine actions:

| Action                    | Payload                                   | Description                                        |
| ------------------------- | ----------------------------------------- | -------------------------------------------------- |
| `run_job`                 | none                                      | Click action — earn CU based on tier               |
| `buy_hardware`            | `{"name": "..."}`                         | Purchase hardware item                             |
| `sell_hardware`           | `{"id": "..."}`                           | Sell for 60% refund                                |
| `deploy_service`          | `{"name": "..."}`                         | Deploy a service                                   |
| `buy_upgrade`             | `{"name": "..."}`                         | Buy upgrade (cooling/network/automation/knowledge) |
| `upgrade_component`       | `{"hardware_id":"...","component":"..."}` | Upgrade CPU/RAM/storage/NIC on hardware            |
| `upgrade_tier`            | none                                      | Progress to next tier                              |
| `unlock_saas`             | none                                      | Unlock SaaS features                               |
| `deploy_saas`             | `{"name": "..."}`                         | Deploy SaaS service + first customer               |
| `resolve_event`           | none                                      | Pay CU to clear throttle                           |
| `colo`                    | none                                      | Prestige — colocate rack                           |
| `build_datacenter`        | none                                      | Build own datacenter                               |
| `upgrade_datacenter`      | none                                      | Upgrade datacenter level                           |
| `bulk_upgrade_components` | none                                      | Upgrade all components on all hardware             |
| `bulk_deploy_services`    | none                                      | Deploy all available services                      |
| `bulk_buy_upgrades`       | `{"type": "..."}` (optional)              | Buy all available upgrades (optional type filter)  |
| `bulk_deploy_saas`        | none                                      | Deploy all available SaaS services                 |

---

## Bulk Actions

Convenience actions that auto-purchase/deploy until resources are exhausted.

| Action                    | Effect                                                                                        |
| ------------------------- | --------------------------------------------------------------------------------------------- |
| `bulk_upgrade_components` | Upgrades all components on all hardware until compute runs out                                |
| `bulk_deploy_services`    | Deploys all available services for current tier until out of CU/power                         |
| `bulk_buy_upgrades`       | Buys all available upgrades (optional `type` filter: cooling/networking/automation/knowledge) |
| `bulk_deploy_saas`        | Deploys all available SaaS services until out of CU/rep/power                                 |

All bulk actions respect the same requirements as their individual counterparts (tier gates, prestige cost scaling, power limits, etc.).

---

## Game Config Endpoint

**Endpoint:** `GET /api/game/config`
**Cache:** `public, max-age=3600`
**File:** `apps/backend/internal/game/engine/config.go`

Returns the complete game configuration, enabling the frontend to be fully config-driven with zero hardcoded values.

### Config Structure

```
GameConfig
├── tiers[]              — Tier ID, label, rank, base upgrade cost, job reward, power limit, cooling bonus, flavor text jobs
├── hardware_bonuses     — UPS compute map, network income map, storage rep map, patch panel bonus value
├── prestige             — LinearCap: 5, LinearIncrement: 0.5, Base: 3.5, ExponentialBase: 1.5
├── saas_unlock          — BaseCost: 10000, ReputationRequired: 100
├── datacenter           — Build/upgrade costs, min colo count, max level, income multiplier step, tier/level names
├── gameplay             — ShelfSlots: 8, ThrottleResolveCostPerTick: 100, HeatPenalty: 0.5,
│                          KnowledgeBoostDivisor: 100, ColoRackDecay: 0.9, SellRefundPercent: 0.6,
│                          MaxColoCount: 100, BaseCooling: 50
├── leaderboard          — Categories: compute, reputation, colo_count, money, group
└── group                — BonusPerMember: 0.05, MaxBonus: 0.50
```

---

## Leaderboard Categories

| ID         | Label      |
| ---------- | ---------- |
| compute    | Compute    |
| reputation | Reputation |
| colo_count | Prestiges  |
| money      | Money      |
| group      | Groups     |

---

# Ideas

## Global CU Store

A community pool where players donate surplus Compute Units. Donations are tracked on a persistent leaderboard that survives prestige (like the colo_count board).

**Concept:**
- Players can donate any amount of CU at any time
- Donations are one-way (no withdrawals)
- A global "Donated CU" leaderboard ranks all players by lifetime donations
- Leaderboard persists through prestige — lifetime total, not per-run
- Recognition mechanic: top donors could get a visible badge/title in social features

## Bitcoin Trading

Players can buy and sell a single Bitcoin using the USD (Money) currency. The price fluctuates over time server-side. Bitcoin persists through prestige/colo.

**Concept:**
- Players can hold unlimited Bitcoin
- Buy/sell at the current market price using Money ($)
- Price fluctuates on a server-side timer (random walk, mean-reverting, or event-driven)
- Bitcoin balance persists through colo (like knowledge points)
- Sell high to fund knowledge upgrades or datacenter builds across prestiges
- Adds a speculative/timing element to the money economy
