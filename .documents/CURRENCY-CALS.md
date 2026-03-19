# Currency Calculations Reference

Complete breakdown of how currencies are calculated, generated, and spent — with exact file locations.

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

## 1. Idle Income (Per-Tick Progression)

**File:** `apps/backend/internal/game/engine/engine.go`
**Function:** `ProcessIdleProgress()` (lines 19-236)

All income is calculated per elapsed second since `LastTickAt`.

### A. Compute Units Income (line 180)

```go
gs.ComputeUnits += int64(float64(totalCompute) * seconds * totalMultiplier * knowledgeBoost * netMult)
```

**Sources of Compute:**

1. **Hardware Compute** (lines 29-94):
   - Base: `hardware[i].ComputePerTick`
   - Component upgrades add bonus: `cu.ComputeBonus` per upgrade level
   - UPS flat bonuses (line 94):
     - "APC Back-UPS 600VA": +3
     - "CyberPower UPS 1500VA": +8
     - "APC UPS 3000VA": +20

2. **Service Compute** (lines 97-102):
   - Sum of `service[i].ComputePerTick`

3. **Colo Rack Compute** (frontend: `useIdleTick.ts` lines 132-142):
   - Each colo rack: `cr.compute_per_tick * dcMult`
   - `dcMult` = datacenter income multiplier (1.5x to 2.0x)

4. **Group Bonus Compute** (`useIdleTick.ts` lines 144-154):
   - `(rawHwCompute + serviceCompute) * (groupBonus - 1.0)`
   - 5% per additional group member, capped at 50%

**Multipliers** (line 169):

```go
totalMultiplier := gs.ColoMultiplier * gs.IdleMultiplier * heatPenalty * eventThrottle
```

| Multiplier       | Source                                       | Range          |
| ---------------- | -------------------------------------------- | -------------- |
| `ColoMultiplier` | Prestige count (diminishing returns)         | 1.0x - ~3.47x+ |
| `IdleMultiplier` | Automation upgrades                          | 1.0x - 3.0x    |
| `heatPenalty`    | 0.5 if overheating, else 1.0 (lines 162-164) | 0.5x or 1.0x   |
| `eventThrottle`  | Active event effects                         | 0.01x - 1.0x   |

**Additional Bonuses:**

- **Knowledge Boost** (line 172): `1.0 + knowledgePoints/100.0`
- **Network Bonus** (line 175): `1.0 + networkBonus` where networkBonus is sum of owned switches:
  - "Unmanaged Switch 8-port": 0.10
  - "Unmanaged Switch 24-port": 0.14
  - "Managed Switch 24-port": 0.20
  - "10GbE Switch": 0.25
  - "Fiber Switch 48-port": 0.30

### B. Reputation Income (line 181)

```go
gs.Reputation += int64(float64(serviceRep) * seconds * heatPenalty * eventThrottle * repMult)
```

- `serviceRep`: Sum of `service[i].ReputationPerTick`
- Colo racks: `cr.reputation_per_tick * dcMult`
- `repMult` (line 178): `1.0 + storageBonus + patchPanelBonus`
  - Synology NAS: +0.10
  - 2U JBOD Storage Shelf: +0.15
  - Synology RackStation: +0.25
  - Patch panels: +0.05 each

### C. Money Income (line 182)

```go
gs.Money += int64(float64(serviceMoney) * seconds * heatPenalty * eventThrottle)
gs.Money -= int64(float64(totalExpenses) * seconds)
```

- `serviceMoney`: Sum of `service[i].MoneyPerTick`
- Colo racks: `cr.money_per_tick * dcMult`
- Expenses deducted per tick (lines 184-194)
- Money floor: 0 (never goes negative)

---

## 2. Click/Job Action

**File:** `apps/backend/internal/game/engine/engine.go`
**Function:** `runJob()` (lines 309-315)

```go
reward := tierJobReward(gs.Tier)
knowledgeBoost := 1.0 + float64(gs.KnowledgePoints)/100.0
gs.ComputeUnits += int64(float64(reward) * knowledgeBoost)
```

**Tier Job Rewards** (lines 1271-1288):

| Tier         | Reward |
| ------------ | ------ |
| Coffee Table | 10     |
| Closet Floor | 50     |
| 12U Rack     | 200    |
| 24U Rack     | 800    |
| 36U Rack     | 3,000  |
| 48U Rack     | 10,000 |

---

## 3. Spending: Hardware

**File:** `apps/backend/internal/game/engine/engine.go` — `buyHardware()` (lines 321-394)
**Catalog:** `apps/backend/internal/game/catalog/hardware.go`

All costs in **Compute Units**:

| Hardware               | Cost    | Tier         | Compute/Tick | Power |
| ---------------------- | ------- | ------------ | ------------ | ----- |
| Raspberry Pi 4         | 50      | Coffee Table | 2            | 15W   |
| N100 Mini PC           | 200     | Coffee Table | 5            | 25W   |
| HP ProDesk Mini        | 400     | Closet Floor | 8            | 35W   |
| Synology NAS           | 500     | Closet Floor | 3            | 40W   |
| APC Back-UPS 600VA     | 300     | Closet Floor | 0            | 0W    |
| Dell PowerEdge R620    | 1,500   | 12U Rack     | 15           | 120W  |
| Managed Switch 24-port | 1,500   | 24U Rack     | 0            | 25W   |
| Dell PowerEdge R730    | 5,000   | 24U Rack     | 30           | 200W  |
| Dell PowerEdge R740xd  | 15,000  | 36U Rack     | 50           | 300W  |
| Dell PowerEdge R750    | 40,000  | 48U Rack     | 80           | 400W  |
| GPU Server (4x A100)   | 100,000 | 48U Rack     | 200          | 1200W |

**Sell refund** (line 431): **60%** of purchase cost

---

## 4. Spending: Services

**File:** `apps/backend/internal/game/engine/engine.go` — `deployService()` (lines 453-495)
**Catalog:** `apps/backend/internal/game/catalog/services.go`

All costs in **Compute Units**:

| Service            | Cost   | Tier   | CU/Tick | Rep/Tick | $/Tick | Power |
| ------------------ | ------ | ------ | ------- | -------- | ------ | ----- |
| Pi-hole            | 20     | Coffee | 1       | 1        | 0      | 5W    |
| Personal Website   | 30     | Coffee | 1       | 2        | 0      | 5W    |
| Plex               | 200    | Closet | 5       | 5        | 0      | 30W   |
| Game Server        | 300    | Closet | 8       | 6        | 0      | 40W   |
| WireGuard VPN      | 400    | 12U    | 3       | 8        | 2      | 5W    |
| Gitea              | 800    | 12U    | 10      | 8        | 0      | 20W   |
| CI/CD Pipeline     | 3,000  | 24U    | 20      | 15       | 0      | 40W   |
| Docker Swarm       | 4,000  | 24U    | 25      | 18       | 0      | 50W   |
| Kubernetes Cluster | 12,000 | 36U    | 60      | 40       | 0      | 100W  |
| AI/ML Training     | 50,000 | 48U    | 150     | 60       | 0      | 200W  |

---

## 5. Spending: Tier Upgrades

**File:** `apps/backend/internal/game/engine/engine.go` — `upgradeTier()` (lines 497-546)

**Base costs** (lines 548-563):

| Upgrade                      | Base Cost (CU) |
| ---------------------------- | -------------- |
| Coffee Table -> Closet Floor | 500            |
| Closet Floor -> 12U Rack     | 5,000          |
| 12U Rack -> 24U Rack         | 25,000         |
| 24U Rack -> 36U Rack         | 100,000        |
| 36U Rack -> 48U Rack         | 500,000        |

**Prestige cost scaling** (lines 504-505):

```go
prestigeScale := 1.0 + float64(gs.ColoCount)*0.5
cost := int64(float64(baseCost) * prestigeScale)
```

Each prestige adds **+50%** to all tier upgrade costs.

---

## 6. Spending: Upgrades

**File:** `apps/backend/internal/game/engine/engine.go` — `buyUpgrade()` (lines 569-655)
**Catalog:** `apps/backend/internal/game/catalog/upgrades.go`

### Cooling (cost in CU)

| Name             | Cost   | Capacity Added | Tier   |
| ---------------- | ------ | -------------- | ------ |
| USB Fan          | 30     | +75W           | Coffee |
| Box Fan          | 100    | +150W          | Closet |
| Blanking Panels  | 500    | +250W          | 12U    |
| In-Rack Fans     | 2,000  | +500W          | 12U    |
| Portable AC Unit | 8,000  | +1,000W        | 24U    |
| Mini Split AC    | 25,000 | +2,000W        | 36U    |
| In-Row Cooling   | 80,000 | +3,750W        | 48U    |

### Networking (cost in CU)

| Name             | Cost   | Tier   | Network Tier Set |
| ---------------- | ------ | ------ | ---------------- |
| Unmanaged Switch | 200    | Closet | 1                |
| Managed Switch   | 2,000  | 12U    | 2                |
| 10GbE Switch     | 8,000  | 36U    | 3                |
| Fiber Network    | 20,000 | 48U    | 4                |

### Automation (cost in CU; resets on prestige)

| Name              | Cost   | Tier   | Idle Multiplier |
| ----------------- | ------ | ------ | --------------- |
| Bash Scripts      | 100    | Coffee | 1.2x            |
| Ansible Playbooks | 1,000  | Closet | 1.5x            |
| Docker Compose    | 5,000  | 12U    | 2.0x            |
| Kubernetes        | 50,000 | 36U    | 3.0x            |

### Knowledge (cost in MONEY; persists through prestige)

| Name            | Cost ($) | Tier   | Knowledge Points |
| --------------- | -------- | ------ | ---------------- |
| CompTIA A+      | 200      | Coffee | 10               |
| Linux Basics    | 300      | Coffee | 15               |
| Networking CCNA | 2,000    | Closet | 20               |
| AWS/Cloud Cert  | 8,000    | 12U    | 25               |
| RHCE            | 20,000   | 24U    | 30               |
| CKA             | 60,000   | 36U    | 40               |

---

## 7. Spending: Component Upgrades

**File:** `apps/backend/internal/game/engine/engine.go` — `upgradeComponent()` (lines 671-729)
**Catalog:** `apps/backend/internal/game/catalog/upgrades.go` (lines 98-103)

**Cost formula:** `BaseCost * (CostScale ^ currentLevel)`

| Component | BaseCost (CU) | CostScale | MaxLevel | Compute Bonus/Level | Power Reduction/Level |
| --------- | ------------- | --------- | -------- | ------------------- | --------------------- |
| CPU       | 500           | 2.0x      | 5        | +5                  | 0                     |
| RAM       | 300           | 2.0x      | 5        | +3                  | 0                     |
| Storage   | 400           | 2.0x      | 5        | +2                  | 0                     |
| NIC       | 600           | 2.5x      | 3        | +1                  | -5W                   |

Example: CPU level 0->1 = 500, level 1->2 = 1,000, level 2->3 = 2,000, etc.

---

## 8. SaaS System

**File:** `apps/backend/internal/game/engine/engine.go`

### SaaS Unlock (lines 837-866)

- **Cost:** 10,000 CU + 100 Reputation
- **Effect:** Creates business expenses (8/tick total)

### SaaS Service Deployment (lines 872-926)

**Catalog:** `apps/backend/internal/game/catalog/saas.go`

| Service            | Deploy Cost (CU) | Rep Required | Revenue/Customer | Max Customers | Power | Tier |
| ------------------ | ---------------- | ------------ | ---------------- | ------------- | ----- | ---- |
| Email Hosting      | 5,000            | 100          | 3                | 50            | 20W   | 12U  |
| Web Hosting        | 4,000            | 80           | 2                | 100           | 15W   | 12U  |
| VPN Service        | 10,000           | 150          | 4                | 200           | 30W   | 24U  |
| VPS Hosting        | 15,000           | 200          | 8                | 50            | 60W   | 24U  |
| Managed Database   | 30,000           | 500          | 15               | 30            | 100W  | 36U  |
| Bare Metal Hosting | 60,000           | 800          | 40               | 15            | 200W  | 48U  |
| GPU Cloud          | 100,000          | 1,000        | 100              | 10            | 500W  | 48U  |

### Business Expenses (created on SaaS unlock)

**Catalog:** `apps/backend/internal/game/catalog/saas.go` (lines 76-82)

| Expense              | Cost/Tick | Type           |
| -------------------- | --------- | -------------- |
| Business Internet    | 2         | infrastructure |
| Domain Registrations | 1         | infrastructure |
| SSL Certificates     | 1         | infrastructure |
| Business Insurance   | 3         | legal          |
| Accounting Software  | 1         | operations     |
| **TOTAL**            | **8**     | -              |

Deducted from Money each tick.

### Customer Satisfaction Decay (lines 196-208)

**File:** `apps/backend/internal/game/engine/engine.go`

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

---

## 9. Prestige (Colo)

**File:** `apps/backend/internal/game/engine/engine.go` — `prestige()` (lines 731-809)

**Requirements:** 48U Rack tier, SaaS unlocked, ColoCount < 100

### What Resets (lines 783-805)

- ComputeUnits -> 0
- Reputation -> 0
- Money -> 0
- Tier -> Coffee Table
- All hardware, services, non-persistent upgrades cleared

### What Persists

- KnowledgePoints
- Knowledge upgrades (persistent flag)

### Colo Multiplier Formula (lines 776-781)

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
| 5          | ~3.05x     |
| 10         | ~3.47x     |

### Datacenter Tier Assignment (lines 754-764)

- 1-2 colos: Tier 1
- 3-5 colos: Tier 2
- 6-9 colos: Tier 3
- 10+ colos: Tier 4

---

## 10. Datacenter

**File:** `apps/backend/internal/game/engine/engine.go`

### Build Datacenter (lines 928-953)

- **Requirement:** 5+ colos
- **Cost:** 1,000,000 Money + 5,000,000 CU
- **Effect:** Sets `DatacenterIncomeMultiplier = 1.5x`

### Upgrade Datacenter (lines 955-982)

- **Max Level:** 5
- **Cost per level:** `500,000 * (level+1) Money` + `2,000,000 * (level+1) CU`
- **Multiplier:** `1.0 + level * 0.25`

| Level | Multiplier | Money Cost | CU Cost    |
| ----- | ---------- | ---------- | ---------- |
| 1     | 1.25x      | 1,000,000  | 4,000,000  |
| 2     | 1.50x      | 1,500,000  | 6,000,000  |
| 3     | 1.75x      | 2,000,000  | 8,000,000  |
| 4     | 2.00x      | 2,500,000  | 10,000,000 |
| 5     | 2.25x      | 3,000,000  | 12,000,000 |

---

## 11. Event Effects on Currency

**File:** `apps/backend/internal/game/events/events.go`

### Apply Event (lines 347-369)

```go
if event.Effect.ComputeLoss > 0 {
    gs.ComputeUnits -= event.Effect.ComputeLoss
    if gs.ComputeUnits < 0 { gs.ComputeUnits = 0 }
}
// Same pattern for ReputationLoss, MoneyLoss
```

### Event Resolution Cost

**File:** `apps/backend/internal/game/engine/engine.go` — `resolveEvent()` (lines 819-835)

- **Cost:** `100 CU * throttleTicksRemaining`

### Notable Event Costs

| Event               | Compute Loss | Rep Loss | Money Loss | Throttle        |
| ------------------- | ------------ | -------- | ---------- | --------------- |
| Spilled Drink       | 50           | -        | -          | -               |
| Drive Failure       | 500          | 30       | -          | -               |
| Power Outage        | -            | 50       | -          | 8 ticks         |
| ISP Outage          | -            | 40       | -          | 12 ticks        |
| Reddit Hug of Death | -            | 50       | -          | 10 ticks @ 0.3x |
| Support Ticket      | -            | 10       | 50         | -               |
| Chargeback          | -            | 20       | 200        | -               |
| Bandwidth Overage   | -            | -        | 500        | -               |
| Lease Renewal       | -            | -        | 1,000      | -               |
| Remote Hands        | -            | -        | 300        | 5 ticks @ 0.8x  |

---

## 12. Frontend Rate Calculation (Client-Side Mirror)

**File:** `apps/desktop/src/hooks/useIdleTick.ts`

The frontend replicates the server's idle formula for smooth interpolation:

```typescript
const computeRate = baseComputeRate + coloComputeRate + groupComputeRate;
const repRate = serviceRep * heatPenalty * throttle * repMult + coloRepRate;
const moneyRate =
  serviceMoney * heatPenalty * throttle + coloMoneyRate - totalExpenses;
```

**Components** (lines 79-174):

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

## 13. Currency Flow Summary

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

## 14. Complete Multiplier Stack

| Multiplier      | Range            | Applies To            | Source               |
| --------------- | ---------------- | --------------------- | -------------------- |
| ColoMultiplier  | 1.0x - unbounded | CU idle               | Prestige count       |
| IdleMultiplier  | 1.0x - 3.0x      | CU idle               | Automation upgrades  |
| HeatPenalty     | 0.5x or 1.0x     | CU + Rep + Money idle | Power > cooling      |
| EventThrottle   | 0.01x - 1.0x     | CU + Rep + Money idle | Active events        |
| KnowledgeBoost  | 1.0x - 1.4x      | CU idle + job click   | Knowledge points     |
| NetworkBonus    | 1.0x - 1.3x      | CU idle               | Network switches     |
| StorageBonus    | 1.0x - 1.5x      | Rep idle              | Storage hardware     |
| PatchPanelBonus | +0.05 each       | Rep idle              | Patch panel hardware |
| DatacenterMult  | 1.5x - 2.25x     | Colo rack income only | Datacenter level     |
| GroupBonus      | 1.0x - 1.5x      | CU idle               | Group member count   |
