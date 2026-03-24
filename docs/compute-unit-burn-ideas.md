---
project: "project"
type: "document"
prompt: "I want to find ideas on how players can burn compute units, but also feel like they get ahead somehow"
generated: "2026-03-21"
specs_analyzed:
  - architecture.md
  - performance.md
  - code-quality.md
---

# Compute Unit Sink Ideas — Burn CU, Feel Like You're Getting Ahead

> Generated on 2026-03-21 by analyzing 3 project specifications and the full game codebase against the prompt:
> "I want to find ideas on how players can burn compute units, but also feel like they get ahead somehow"

## Executive Summary

The game currently has **15 distinct CU sinks**, but nearly all of them are one-time purchases. Once a player owns all hardware, services, upgrades, and has maxed component upgrades, the only repeating sinks are Bitcoin trading (100K CU per tx), CU donation (pure vanity), and random event losses (trivially small). Late-game CU generation scales exponentially via prestige multipliers, automation, and knowledge boosts — a player with 10 prestiges generates CU orders of magnitude faster than they can spend it.

**The core problem: CU is inflationary and existing sinks are finite.** The game needs repeatable CU sinks that scale with player income and provide visible progression or advantage. The ideas below are ordered by impact and feasibility, informed by the current engine architecture (which makes most of these straightforward to add).

The good news: the engine's action dispatch pattern, catalog-driven design, and multiplier stack make adding new CU sinks very low-cost. Most ideas below require only new fields on `GameState`, a new `case` in the `ProcessAction` switch, and a UI button.

## Critical Ideas — High Impact, Proven Patterns

### 1. Overclock Mode (Temporary Compute Boost)

**All three analyses converged on this as the #1 quick win.**

Spend CU to activate a time-limited multiplier on all compute income — e.g., 50,000 CU for 2x income for 60 ticks. Multiple tiers (2x, 3x, 5x) at increasing costs. The player sees a visible timer and boosted income counter.

- **Why it feels like progress:** You spend CU and *immediately* see numbers go up faster. Unlike resolving events (which removes a penalty), this adds a positive buff. Creates timing decisions — overclock before going AFK? Save for a tier upgrade, or overclock to earn faster?
- **Why it works as a sink:** Repeatable, cost scales with tier/income, creates a CU-in → CU-out cycle where the player trades a lump sum for accelerated earning.
- **Architecture fit:** The `ThrottleMultiplier` + `ThrottleTicksRemaining` pattern already implements timed multiplier effects (`engine.go:134-140`). Overclock is the same pattern but positive. Two new fields on `GameState`: `OverclockMultiplier float64`, `OverclockTicksRemaining int`. One new action. The frontend `useIdleTick.ts` needs the multiplier added to its rate calculation (one-line change).
- **Performance cost:** Zero. Two columns on `game_states`, one multiplication in the existing `ProcessIdleProgress` path.
- **Bonus mechanic:** Overclock could generate extra heat, creating a feedback loop with the cooling system — players invest in cooling to sustain longer overclocks.

### 2. Research Tree (Permanent Percentage Boosts, Infinite Levels)

**The strongest long-term CU sink. Addresses the "infinite CU, finite sinks" problem directly.**

A multi-branch research tree where each node gives a small permanent bonus (+2% idle income, +5% reputation gain, -3% power draw, +1% customer satisfaction). Each level of a node costs exponentially more but gives the same flat bonus — natural diminishing returns with no cap. Themed around homelab learning: "Read the Docs," "Lab Notebook," "Automated Testing," "Chaos Engineering," "Master BGP Routing."

- **Why it feels like progress:** The player is always making their operation slightly more efficient. "My idle income research is level 23" is a visible, permanent marker of investment. Unlike hardware (which you buy once), research levels have no ceiling.
- **Why it works as a sink:** Exponential cost scaling means it absorbs increasingly large CU amounts as the player grows. A player generating 1M CU/sec still has something meaningful to pour CU into.
- **Architecture fit:** Follows the `ComponentUpgrade` pattern (`models/game_state.go:53-61`) — per-node levels with exponential cost scaling. New table `research_levels` with `(game_state_id, research_node, level)`. Bonuses applied as multipliers in `ProcessIdleProgress`.
- **Design choice — prestige persistence:** Two options:
  - **Resets on prestige** — creates a per-run CU sink, tension between research investment and saving for tier upgrades.
  - **Persists through prestige** — creates a permanent power ramp, rewards long-term investment. (Recommend this option — it's the "feel like you get ahead" angle.)
- **Performance cost:** +1 DB read per state fetch (bounded rows per player), +1 write per research action. Minimal.

### 3. Prestige Rack Optimization (Spend CU to Boost Your Legacy)

**Turns the prestige moment from "sacrifice everything" into "invest everything."**

Before prestiging, spend CU to "optimize your rack for colo" — each investment adds a percentage to the colo rack's frozen income snapshot. E.g., spend 100K CU to add +10% to the rack's permanent `ComputePerTick`. Cost escalates (doubling each purchase). Resets after prestige.

- **Why it feels like progress:** The prestige moment is already the most meaningful in the game. This makes it feel like you're powering up your legacy rack rather than throwing CU away. "I spent 2M CU optimizing, and now my colo rack earns 80% more forever."
- **Why it works as a sink:** Late-48U players often have excess CU with nothing to buy. This gives them a valuable pre-prestige dump. Cost escalation prevents trivial stacking.
- **Architecture fit:** The `prestige()` function (`engine.go:721-806`) already snapshots income into the `ColoRack` model. A `RackOptimization float64` field on `GameState` acts as a multiplier on the snapshot. New action `optimize_rack` deducts CU and increments it. Resets to 0 in the prestige reset block.
- **Performance cost:** +1 column on `game_states`. Zero additional queries.

## Important Ideas — Strong Engagement, Moderate Effort

### 4. Fill the Missing Early-Game Catalog

**PLAN.md describes 5+ coffee table/closet upgrades that aren't implemented. Pure data addition.**

Missing from the catalog: Surge Protector (event mitigation for power spikes), Ethernet Cable (+5% service reputation), Power Strip Upgrade (+100 power limit), Cable Organizer (event mitigation for cable spaghetti), Dedicated Circuit (+200 power limit). Cost range: 30–500 CU.

- **Why it matters:** The gap between "I bought everything available" and "I can afford the next tier" is currently filled only by clicking and waiting. These give early-game players more frequent "buy something" moments in the 30–500 CU range.
- **Implementation:** New entries in `catalog/upgrades.go`. No engine or handler changes — the existing `buyUpgrade` pattern handles them automatically.

### 5. CU-to-Reputation Conversion ("Host a Community Event")

**Fills a real gap — reputation is hard to accelerate, CU often accumulates faster.**

Spend CU to "host" a LAN party, hackathon, or open-source sprint that converts CU into Reputation. Ratio like 100 CU = 1 Reputation. Cooldown-gated (once per 5 minutes) to prevent spam.

- **Why it feels like progress:** Reputation gates SaaS unlock and high-tier deployments. Letting players speed this up with CU creates a satisfying "I can trade wealth for status" moment.
- **Architecture fit:** Trivially simple. New action `host_event`, validates balance, deducts CU, adds Reputation. Cooldown via `LastHostedEventAt` timestamp (same pattern as `LastCustomerGrowthAt`).
- **Performance cost:** Zero new tables, zero new queries.

### 6. Speed Up Customer Acquisition ("Run a Marketing Campaign")

**A CU-to-Money pipeline for late-game players.**

Spend CU to immediately attract 1–3 new customers to an existing SaaS service. Cost scales with existing customer count (e.g., `1000 * (total_customers + 1)`), creating natural diminishing returns.

- **Why it feels like progress:** Each customer means more $/tick. Money enables Bitcoin trading, datacenter building, and knowledge upgrades. The player is directly converting CU into a growing revenue stream.
- **Architecture fit:** The `deploySaas` action already creates customers. A `run_marketing` action would add customers to existing SaaS services. `ActionResult.NewCustomer` already supports this return type.

### 7. Prestige Accelerators (Start Next Cycle Ahead)

**Spend CU to skip early tiers on the next prestige.**

"Fast-Track Colo: spend 1,000,000 CU to start next prestige at 12U instead of Coffee Table." Or "Skip SaaS Unlock: spend 2,000,000 CU to bypass the SaaS gate next cycle." One-use-per-prestige-cycle.

- **Why it feels like progress:** Directly shortening the prestige cycle — the core progression loop. The player is paying to make their future self faster.
- **Architecture fit:** Flags on `game_states` (e.g., `prestige_start_tier`). Checked in `prestige()`. Near-zero implementation cost.
- **Balance note:** Cap the acceleration (e.g., skip at most 2 tiers, can't combine skip SaaS + start at 48U). If prestige becomes trivially fast, pacing breaks.

### 8. Bitcoin Mining (CU-Only Path to Cross-Prestige Asset)

**Gives mid-game players access to Bitcoin without needing $Money.**

A `mine_bitcoin` action: spend 500,000 CU per BTC (no money involved). Much less efficient than buying BTC with Money (which costs market price + 100K CU), but available before the SaaS tier.

- **Why it feels like progress:** Bitcoin persists through prestige. Players can convert CU surplus into a lasting store of value *before* they have Money income. Fills the gap between "I have tons of CU" and "I can do something lasting with it."
- **Architecture fit:** Follows the exact `buyBitcoin` pattern in `engine.go:1004-1085` but simpler (no money check).

## Minor Ideas — Good Flavor, Lower Priority

### 9. Emergency Repair Fund (Pre-Paid Event Insurance)

Deposit CU into a "Repair Fund." When events fire and aren't mitigated, the fund auto-covers `ComputeLoss` and auto-resolves throttle. Depletes over time as events consume it.

- **Feel:** "I invested CU so future disasters can't touch me." Converts uncertainty into peace of mind — a powerful idle game motivator.
- **Implementation:** New `RepairFund int64` on GameState. Check in `events.ApplyEvent` before deducting from main balance.

### 10. Hardware Tuning (Uncapped Component Upgrades)

Extend the component upgrade system with a "tuning" action — uncapped (or very high cap) levels with diminishing returns (+1% per tune, cost doubles each time). Turns every hardware item into a bottomless CU pit.

- **Feel:** "My Dell R740xd is now tuning level 47." Creates choices about which hardware to invest in.
- **Implementation:** Almost zero new code — raise `MaxLevel` on existing component upgrades or add a "tuning" component type. Exponential cost scaling already exists.

### 11. Compute Contracts (Time-Locked CU Investments)

Lock CU into contracts (e.g., "Enterprise Rendering Job: lock 500K CU for 30 min, receive 750K CU on completion"). CU is unavailable during the lock. If you prestige mid-contract, the contract is lost.

- **Feel:** Strategic timing decisions around prestige cycles. Risk/reward tension.
- **Performance note:** Requires a new `contracts` table. Cap at 3 concurrent contracts per player to bound query cost.

### 12. Expand Hardware Capacity (Extra Slots/U)

At coffee table/closet: "Buy a Bigger Coffee Table" for 200 CU adds 1 slot. At rack tiers: "Extra Rack Rails" adds a few U to the current rack.

- **Feel:** Helps early-game players who are stuck saving for the next tier. Gives them something tangible to buy.
- **Implementation:** `GameState` already has `HardwareSlots` and `RackUnits`. New action increments these with diminishing returns.

### 13. Redundancy Upgrades (Event Immunity per Hardware)

Spend CU to "add redundancy" to specific hardware — makes it immune to targeting events. No compute bonus, purely defensive.

- **Feel:** "My RAID-backed server can't be touched by drive failure." Thematically perfect for homelab culture.
- **Implementation:** Add "redundancy" as a 5th component type in the existing `ComponentUpgrade` system. Max level 1, high cost, zero compute bonus, event mitigation only.

## Current Strengths

- **The multiplier stack in `ProcessIdleProgress` is clean and extensible.** Adding any new CU sink that yields a multiplier bonus (overclock, research, efficiency) slots directly into the existing chain: `coloMultiplier * idleMultiplier * heatPenalty * throttleMultiplier * knowledgeBoost * netMult * repMult`.
- **Catalog-driven design** means new purchasable CU sinks (hardware, services, upgrades) require only data additions — no engine changes.
- **Action dispatch is a clean switch statement** — adding a new action type is one `case` line and one method, proven across 19 existing actions.
- **The prestige system has clear preservation rules** — the `prestige()` function explicitly lists what resets, so adding prestige-persistent benefits is safe and predictable.
- **Bulk action pattern exists** — any new per-item CU sink automatically gets a bulk variant.
- **The event mitigation system has upgrade hooks** — defensive CU sinks plug in naturally.

## Implementation Constraints

- **No background workers.** Timed effects (overclock, contracts, research) must be evaluated lazily during `ProcessIdleProgress`, same as throttle ticks. Long offline periods need careful handling.
- **Full state response on every request.** New fields increase payload size. Keep sink state compact (integers, timestamps, booleans). Avoid large arrays.
- **Integer-only currencies.** CU, Reputation, and Money are all `int64`. Conversion ratios need integer granularity.
- **Client interpolation must match server math.** Any new multiplier added server-side must also be reflected in `useIdleTick.ts`, or counters will visibly jump on sync.
- **DB queries are the bottleneck.** Prefer storing sink state as columns on `game_states` over new tables. The connection pool (20 max) limits throughput. Each new per-request query lowers the concurrent player ceiling.
- **Testing gap.** Few automated tests exist for balance-sensitive CU mutations. New sinks with multiplier interactions or exponential scaling carry risk of subtle balance bugs.
- **Marketplace/auction patterns should be deferred.** Cross-user atomicity, connection contention, and real-time price feeds are incompatible with the current single-VM, per-user-mutex architecture.
