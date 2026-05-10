---
project: "homelab-game"
maturity: "draft"
last_updated: "2026-03-21"
updated_by: "@staff-engineer"
scope: "Research Tree: permanent percentage boosts with infinite levels and exponential cost scaling, persisting through prestige"
owner: "@staff-engineer"
dependencies:
  - ../spec/architecture.md
  - ../spec/code-quality.md
---

# Research Tree

## 1. Problem Statement

### What

Add a Research Tree to Homelab the Game -- a multi-branch system of permanent percentage
boosts that players purchase with Compute Units (CU). Each research node has infinite
levels, with each level costing exponentially more while providing the same flat percentage
bonus. Research levels persist through prestige.

This is one of three CU sink features being built (alongside Overclock Mode and Prestige
Rack Optimization) to address the late-game CU inflation problem described in
`docs/compute-unit-burn-ideas.md`.

### Why Now

The game currently has 15 distinct CU sinks, but nearly all are one-time purchases. Once a
player owns all hardware, services, upgrades, and has maxed component upgrades, the only
repeating sinks are Bitcoin trading (100K CU per tx), CU donation (pure vanity), and random
event losses (trivially small). Late-game CU generation scales exponentially via prestige
multipliers, automation, and knowledge boosts. A player with 10+ prestiges generates CU
orders of magnitude faster than they can spend it.

The Research Tree is the strongest long-term CU sink because:
- It has no cap -- exponential cost scaling absorbs arbitrarily large CU amounts
- It provides visible permanent progression ("my idle income research is level 23")
- It persists through prestige, rewarding long-term investment
- It creates meaningful decisions: which branches to invest in, how deep to go

### Constraints

- **Server-authoritative**: All research purchases validated server-side. Cost calculation
  and bonus application happen in the engine, never the client.
- **Integer-only currencies**: CU is `int64`. Cost formulas must produce integer costs.
  At very high levels, costs must not overflow `int64`.
- **No background workers**: Research is not time-based. It is an instant purchase like
  component upgrades. No timers, no queue.
- **Full state response**: Research levels are included in every game state response.
  Must be compact (bounded rows per player -- one per research node, not one per level).
- **Client interpolation must match server math**: Any new multiplier from research
  bonuses must be reflected in `useIdleTick.ts`.
- **Prestige persistence**: Research levels MUST survive colo. This is the core value
  proposition.
- **Tier-gated**: Different research nodes unlock at different progression tiers, giving
  players new nodes to discover as they progress.

### Acceptance Criteria

1. A research catalog exists with at least 8 nodes across 3+ branches, each with a
   defined effect, base cost, cost scaling factor, and minimum tier.
2. Players can purchase research levels by spending CU. Each level costs
   `floor(baseCost * costScale^currentLevel)` CU.
3. Each level of a node provides the same flat percentage bonus (e.g., +2% idle income
   per level).
4. Research levels persist through prestige/colo -- they are NOT reset.
5. Research bonuses are applied as multipliers in `ProcessIdleProgress` and reflected
   in the `useIdleTick` client-side interpolation.
6. A `buy_research` action and `bulk_buy_research` action exist following the existing
   action dispatch pattern.
7. The frontend displays a Research panel with all nodes, their current levels, costs
   for the next level, and cumulative effect.
8. The game config endpoint includes research node definitions so the client can
   display costs and effects without hardcoding.
9. Research nodes are tier-gated: nodes with `min_tier: rack_24u` are not visible or
   purchasable until the player reaches 24U rack tier.
10. Overflow protection: the engine rejects purchases when the cost calculation would
    overflow `int64`.

---

## 2. Context & Prior Art

### Existing Codebase Patterns

**ComponentUpgrade Pattern** (`catalog/upgrades.go:88-112`, `engine.go:661-719`):

The closest existing pattern. Component upgrades have per-item levels with exponential
cost scaling (`baseCost * costScale^currentLevel`). They are stored in a separate
`component_upgrades` table with a `(hardware_id, component)` unique constraint and use
an UPSERT query for level advancement. Key differences from research:
- Component upgrades are tied to specific hardware items (deleted on prestige with the
  hardware).
- They have a `MaxLevel` cap (3-5).
- They are loaded via a JOIN on `hardware.game_state_id`.

Research nodes will follow the same cost formula but differ in: tied to game state (not
hardware), no max level, and persistent through prestige.

**Upgrade Catalog Pattern** (`catalog/upgrades.go:5-125`):

Static Go structs define upgrade templates with `Name`, `Type`, `MinTier`, `Cost`, and
effect metadata. The `GetAvailableUpgrades(tier)` function filters by tier rank. Research
nodes will follow this catalog pattern but with additional fields for cost scaling and
effect type.

**Multiplier Stack in ProcessIdleProgress** (`engine.go:151-163`):

```
totalMultiplier := gs.ColoMultiplier * gs.IdleMultiplier * heatPenalty * eventThrottle
knowledgeBoost := 1.0 + float64(gs.KnowledgePoints)/100.0
netMult := 1.0 + networkBonus
repMult := 1.0 + storageBonus + patchPanelBonus

gs.ComputeUnits += int64(float64(totalCompute) * seconds * totalMultiplier * knowledgeBoost * netMult)
gs.Reputation += int64(float64(serviceRep) * seconds * heatPenalty * eventThrottle * repMult)
gs.Money += int64(float64(serviceMoney) * seconds * heatPenalty * eventThrottle)
```

Research bonuses slot into this chain as additional multipliers. The chain is already a
product of terms, so adding new multiplier terms is straightforward.

**Prestige Persistence** (`engine.go:779-806`):

The prestige function explicitly resets fields to defaults. Fields that persist are
left untouched with comments (`// Keep: KnowledgePoints`, `// Keep: BitcoinBalance`).
Research levels live in a separate table keyed by `game_state_id`, so prestige handling
needs to explicitly NOT delete them (unlike hardware, services, customers, expenses,
and non-persistent upgrades which are wiped).

**Action Dispatch** (`engine.go:232-279`):

A switch statement routes action types to handler methods. Adding `buy_research` and
`bulk_buy_research` is one new case each. The `ProcessAction` signature already accepts
all the data needed; research levels will be passed as an additional parameter.

**Handler Persist Pattern** (`game.go:600-735`):

The handler checks `ActionResult` fields and persists new records. For research, a new
`ResearchLevel` field on `ActionResult` will trigger an UPSERT to the `research_levels`
table, following the same pattern as `ComponentUpgrade`.

### Prior Art in Idle Games

Exponential-cost research trees are a standard pattern in idle/incremental games:
- **Cookie Clicker**: Heavenly Upgrades (prestige-persistent, unlocked by spending
  prestige currency)
- **Realm Grinder**: Research system with multi-branch trees and tier gating
- **Idle Champions**: Champion upgrades with exponential cost scaling

The key design insight from these games: the "same bonus per level with exponential cost"
creates natural diminishing returns without needing a cap, making it an effective infinite
sink.

---

## 3. Alternatives Considered

### Alternative A: Store research as columns on `game_states`

Add one column per research node to the `game_states` table (e.g.,
`research_idle_income INT DEFAULT 0`, `research_rep_gain INT DEFAULT 0`, etc.).

**Strengths:**
- Zero additional DB queries -- research levels loaded with the existing game state SELECT
- Simplest possible implementation
- No new table, no new query struct

**Weaknesses:**
- Schema changes for every new research node (ALTER TABLE + model + query column lists)
- The `game_states` table already has 34 columns; adding 8-12 more increases
  maintenance burden
- The `gsColumns` constant and `gsFields` function in `queries/game_state.go` must stay
  manually synchronized with the UPDATE query's 32 numbered parameters -- adding more
  columns increases the risk of positional mismatch bugs
- Does not generalize to adding more research nodes later without migrations

### Alternative B: Separate `research_levels` table (Recommended)

A new table with `(game_state_id, research_node, level)` and a unique constraint on
`(game_state_id, research_node)`.

**Strengths:**
- Adding new research nodes requires only catalog changes, no schema migration
- Clean separation of concerns
- Follows the `component_upgrades` precedent
- UPSERT handles both first purchase and level-up
- Bounded rows per player (one per research node, ~8-12 rows)

**Weaknesses:**
- One additional SELECT per request (adds to the 8-query-per-request fan-out)
- One additional write per research purchase

**Mitigation for query cost:** The query is simple (`WHERE game_state_id = $1`),
returns at most ~12 rows, and the table will be small. Adding an index on
`game_state_id` makes this negligible. The existing 8-query pattern already works at
the current scale, and one more bounded query does not change the scaling profile.

### Alternative C: Embed research levels in a JSONB column on `game_states`

Store all research levels as a JSON object in a single column.

**Strengths:**
- No new table
- No additional queries
- Flexible schema

**Weaknesses:**
- Breaks the existing pattern (all other game data uses normalized tables)
- Harder to query for leaderboard/analytics purposes
- JSONB mutations require read-modify-write at the application level
- No database-level constraints on data integrity

### Recommendation

**Alternative B** (separate table). It follows established patterns, keeps the schema
extensible, and the performance cost is negligible for the bounded row count.

---

## 4. Architecture & System Design

### 4.1 Research Node Catalog

A new file `internal/game/catalog/research.go` defines the research tree as static Go
structs, following the existing catalog pattern.

```go
type ResearchNode struct {
    ID          string     `json:"id"`          // unique identifier (snake_case)
    Name        string     `json:"name"`        // display name
    Branch      string     `json:"branch"`      // branch grouping for UI
    MinTier     models.Tier `json:"min_tier"`   // tier gate
    BaseCost    int64      `json:"base_cost"`   // CU cost at level 0
    CostScale   float64    `json:"cost_scale"`  // exponential multiplier per level
    EffectType  string     `json:"effect_type"` // which bonus this applies to
    EffectValue float64    `json:"effect_value"` // bonus per level (e.g., 0.02 = +2%)
    Description string     `json:"description"` // flavor text
}
```

**Branch: Efficiency** (boosts compute/idle income)

| ID | Name | MinTier | BaseCost | CostScale | Effect | Per Level |
|----|------|---------|----------|-----------|--------|-----------|
| `read_the_docs` | Read the Docs | coffee_table | 500 | 1.8 | idle_income | +2% |
| `lab_notebook` | Lab Notebook | closet_floor | 2,000 | 2.0 | idle_income | +3% |
| `automated_testing` | Automated Testing | rack_12u | 10,000 | 2.2 | idle_income | +5% |

**Branch: Reputation** (boosts reputation gain)

| ID | Name | MinTier | BaseCost | CostScale | Effect | Per Level |
|----|------|---------|----------|-----------|--------|-----------|
| `blog_writing` | Blog Writing | coffee_table | 500 | 1.8 | reputation_gain | +3% |
| `conference_talks` | Conference Talks | rack_12u | 8,000 | 2.0 | reputation_gain | +5% |
| `open_source_contrib` | Open Source Contributions | rack_24u | 30,000 | 2.2 | reputation_gain | +8% |

**Branch: Infrastructure** (boosts money income and reduces costs)

| ID | Name | MinTier | BaseCost | CostScale | Effect | Per Level |
|----|------|---------|----------|-----------|--------|-----------|
| `chaos_engineering` | Chaos Engineering | rack_24u | 25,000 | 2.0 | money_income | +4% |
| `master_bgp_routing` | Master BGP Routing | rack_36u | 80,000 | 2.2 | money_income | +6% |

**Branch: Mastery** (boosts job click rewards)

| ID | Name | MinTier | BaseCost | CostScale | Effect | Per Level |
|----|------|---------|----------|-----------|--------|-----------|
| `scripting_mastery` | Scripting Mastery | coffee_table | 300 | 1.6 | job_reward | +3% |
| `system_optimization` | System Optimization | rack_12u | 5,000 | 2.0 | job_reward | +5% |

**Design notes:**
- 10 nodes across 4 branches. Branches are a UI grouping only -- they have no
  mechanical interaction.
- Early-game nodes (coffee_table/closet_floor) have low base costs and gentler scaling
  (1.6-1.8x) to give new players something to sink CU into.
- Late-game nodes (rack_36u) have high base costs and aggressive scaling (2.2x) to
  absorb endgame CU generation.
- The catalog is extensible: adding a new node is one struct addition, no schema change.

**Cost examples for `read_the_docs` (base=500, scale=1.8):**

| Level | Cost | Cumulative | Bonus |
|-------|------|------------|-------|
| 1 | 500 | 500 | +2% |
| 5 | 5,249 | 14,539 | +10% |
| 10 | 90,197 | 196,492 | +20% |
| 20 | 26.6M | 52.8M | +40% |
| 50 | 6.86T | 12.1T | +100% |

At level 50 the cost per level is in the trillions -- only extreme late-game players
reach this. The bonus at +100% (2x multiplier) is significant but not game-breaking
since it applies to only one income channel.

### 4.2 Research Model

```go
// In internal/models/game_state.go
type ResearchLevel struct {
    ID           string    `json:"id"`
    GameStateID  string    `json:"game_state_id"`
    ResearchNode string    `json:"research_node"` // matches catalog ID
    Level        int       `json:"level"`
    UpdatedAt    time.Time `json:"updated_at"`
}
```

### 4.3 Engine Integration

**ProcessAction**: Add two new cases to the switch:

```
case "buy_research":
    return e.buyResearch(gs, payload, researchLevels)
case "bulk_buy_research":
    return e.bulkBuyResearch(gs, payload, researchLevels)
```

The `ProcessAction` signature needs a new parameter for research levels. This is a
breaking change to the function signature, but the single call site in `game.go`
makes it low-risk. The alternative (loading research levels inside the engine) would
violate the engine's stateless design.

**buyResearch action:**

```
Payload: { "node": "read_the_docs" }
Validation:
  1. Node exists in catalog
  2. Player tier >= node MinTier
  3. Cost = floor(baseCost * costScale^currentLevel)
  4. Overflow check: if currentLevel is high enough that the cost calculation
     overflows int64, reject with "research level too high"
  5. Player has sufficient CU
Mutation:
  1. Deduct cost from gs.ComputeUnits
  2. Increment level (or create at level 1 if first purchase)
Result:
  ActionResult.ResearchLevel = &ResearchLevel{...}
```

**bulkBuyResearch action:**

```
Payload: { "node": "read_the_docs" }  (buys max affordable levels for one node)
Logic:
  1. Validate node exists and tier gate met
  2. Loop: calculate cost for next level, check overflow, check affordability
  3. Purchase levels until CU runs out or overflow threshold reached
  4. Return final ResearchLevel
```

The bulk action buys max levels for a single specified node, not all nodes. This
gives the player control over where to invest while still supporting "pour all CU into
this node" gameplay. A no-payload variant that buys across all nodes would be complex
(which node gets priority?) and is deferred.

**Overflow protection:**

The cost formula `baseCost * costScale^level` can overflow `int64` at high levels. The
engine must check before computing:

```go
// Safe cost calculation with overflow protection
func researchCost(baseCost int64, costScale float64, level int) (int64, bool) {
    costF := float64(baseCost) * math.Pow(costScale, float64(level))
    if costF > float64(math.MaxInt64) || math.IsInf(costF, 0) || math.IsNaN(costF) {
        return 0, false // overflow
    }
    return int64(costF), true
}
```

At `costScale=2.2` and `baseCost=80000`, overflow occurs around level 42
(`80000 * 2.2^42 > 9.2e18`). This is far beyond practical gameplay but must be handled
gracefully.

### 4.4 Bonus Application in ProcessIdleProgress

Research bonuses are aggregated by effect type and applied as multipliers. Each effect
type becomes one additional multiplier term in the existing chain.

**Aggregation:** Bonuses within the same effect type stack additively. For example, if
a player has `read_the_docs` at level 5 (+10%) and `lab_notebook` at level 3 (+9%),
the total `idle_income` bonus is +19%, applied as a 1.19x multiplier.

**Application points:**

| Effect Type | Applied To | Multiplier |
|-------------|-----------|------------|
| `idle_income` | Compute income line (same line as `totalMultiplier * knowledgeBoost * netMult`) | `1.0 + sum(level * effectValue)` |
| `reputation_gain` | Reputation income line (same line as `heatPenalty * eventThrottle * repMult`) | `1.0 + sum(level * effectValue)` |
| `money_income` | Money income line (same line as `heatPenalty * eventThrottle`) | `1.0 + sum(level * effectValue)` |
| `job_reward` | Job click reward in `runJob()` (same line as `knowledgeBoost`) | `1.0 + sum(level * effectValue)` |

**Why additive within category, multiplicative across categories:**

This follows the existing pattern. Network income bonus and storage rep bonus stack
additively within their category (`1.0 + networkBonus`, `1.0 + storageBonus +
patchPanelBonus`) but multiply with other categories in the final income line.
Research bonuses follow the same convention: all `idle_income` research bonuses sum
to one multiplier that joins the product chain.

**Concrete change to ProcessIdleProgress:**

The function signature adds `researchLevels []models.ResearchLevel`. A helper
aggregates bonuses:

```go
func aggregateResearchBonuses(levels []models.ResearchLevel) map[string]float64 {
    bonuses := make(map[string]float64)
    for _, rl := range levels {
        node := catalog.GetResearchNode(rl.ResearchNode)
        if node != nil {
            bonuses[node.EffectType] += float64(rl.Level) * node.EffectValue
        }
    }
    return bonuses
}
```

Then in the income calculation:

```go
researchBonuses := aggregateResearchBonuses(researchLevels)
researchIdleMult := 1.0 + researchBonuses["idle_income"]
researchRepMult := 1.0 + researchBonuses["reputation_gain"]
researchMoneyMult := 1.0 + researchBonuses["money_income"]

gs.ComputeUnits += int64(float64(totalCompute) * seconds * totalMultiplier * knowledgeBoost * netMult * researchIdleMult)
gs.Reputation += int64(float64(serviceRep) * seconds * heatPenalty * eventThrottle * repMult * researchRepMult)
gs.Money += int64(float64(serviceMoney) * seconds * heatPenalty * eventThrottle * researchMoneyMult)
```

And in `runJob`:

```go
researchJobMult := 1.0 + researchBonuses["job_reward"]
gs.ComputeUnits += int64(float64(reward) * knowledgeBoost * researchJobMult)
```

The `runJob` function needs access to research levels. Since it currently only receives
`gs *models.GameState`, the simplest approach is to pre-compute the job reward
multiplier and store it on a non-persisted field, or pass research levels as a parameter.
Passing as a parameter is cleaner and consistent with the ProcessIdleProgress approach.

### 4.5 Prestige Persistence

Research levels are stored in a table keyed by `game_state_id`. During prestige, the
handler currently deletes hardware, services, customers, expenses, and non-persistent
upgrades (`game.go:680-697`). Research levels must NOT be deleted.

The prestige cleanup block in the handler does NOT have a blanket "delete everything
related to game_state_id" -- it explicitly calls per-table delete methods. So research
levels survive by simply not adding a delete call for them. No code change needed in
the cleanup block; just do not add `h.research.DeleteByGameStateID()`.

Component upgrades are deleted implicitly via CASCADE on hardware deletion (the
`component_upgrades` table has `ON DELETE CASCADE` from `hardware(id)`). Research
levels have no such cascade because they reference `game_states(id)`, not hardware.
This is correct -- research levels should survive even if all hardware is deleted.

The `wipe_player_progress.sql` utility script should be updated to include
`DELETE FROM research_levels;` for development resets.

---

## 5. Data Models & Storage

### 5.1 New Table: `research_levels`

```sql
-- 011_research_tree.sql
CREATE TABLE research_levels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_state_id UUID NOT NULL REFERENCES game_states(id) ON DELETE CASCADE,
    research_node VARCHAR(50) NOT NULL,
    level INT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(game_state_id, research_node)
);

CREATE INDEX idx_research_levels_game_state ON research_levels(game_state_id);
```

**Design decisions:**
- `ON DELETE CASCADE` from `game_states(id)`: if a game state is deleted (account
  wipe), research levels are cleaned up automatically. This does NOT affect prestige
  because prestige does not delete the `game_states` row -- it resets column values.
- `UNIQUE(game_state_id, research_node)`: enables UPSERT for level advancement, same
  pattern as `component_upgrades`.
- `VARCHAR(50)` for `research_node`: matches catalog node IDs like
  `"master_bgp_routing"` (20 chars). 50 chars provides ample headroom.
- No `compute_bonus` column: unlike component upgrades, research bonuses are
  calculated from `level * effectValue` using the catalog at runtime. This avoids
  storing derived data that could drift from the catalog.

### 5.2 Model Addition

```go
// In internal/models/game_state.go
type ResearchLevel struct {
    ID           string    `json:"id"`
    GameStateID  string    `json:"game_state_id"`
    ResearchNode string    `json:"research_node"`
    Level        int       `json:"level"`
    UpdatedAt    time.Time `json:"updated_at"`
}
```

### 5.3 Query Struct

New file `internal/database/queries/research.go`:

```go
type ResearchLevelQueries struct {
    pool *pgxpool.Pool
}

func NewResearchLevelQueries(pool *pgxpool.Pool) *ResearchLevelQueries { ... }

func (q *ResearchLevelQueries) Upsert(ctx, rl *ResearchLevel) error {
    // INSERT ... ON CONFLICT (game_state_id, research_node) DO UPDATE SET level = $3, updated_at = NOW()
}

func (q *ResearchLevelQueries) GetByGameStateID(ctx, gameStateID string) ([]ResearchLevel, error) {
    // SELECT id, game_state_id, research_node, level, updated_at FROM research_levels WHERE game_state_id = $1
}
```

This follows the exact pattern of `ComponentUpgradeQueries` in
`queries/upgrades.go:64-129`.

### 5.4 Grant Permissions

```sql
GRANT ALL ON research_levels TO homelab_game;
```

---

## 6. API Contracts

### 6.1 Action: `buy_research`

**Request:**
```json
{
  "type": "buy_research",
  "payload": { "node": "read_the_docs" }
}
```

**Success Response:** Standard full game state response (same as all other actions),
now including `research_levels` array.

**Error Responses:**
- `{"error": "unknown research node: xyz"}` -- 400
- `{"error": "tier too low for Read the Docs (need rack_12u)"}` -- 400
- `{"error": "not enough compute units (need 500, have 300)"}` -- 400
- `{"error": "research level too high (cost overflow)"}` -- 400

### 6.2 Action: `bulk_buy_research`

**Request:**
```json
{
  "type": "bulk_buy_research",
  "payload": { "node": "read_the_docs" }
}
```

Buys as many levels as the player can afford for the specified node. Returns the same
full state response.

### 6.3 Game State Response Addition

The `fullStateResponse` struct gains a new field:

```json
{
  "...existing fields...",
  "research_levels": [
    { "id": "uuid", "game_state_id": "uuid", "research_node": "read_the_docs", "level": 5, "updated_at": "..." },
    { "id": "uuid", "game_state_id": "uuid", "research_node": "blog_writing", "level": 3, "updated_at": "..." }
  ]
}
```

Only nodes the player has purchased appear in this array (not all 10 nodes at level 0).

### 6.4 Game Config Response Addition

The `GameConfig` struct gains a `Research` field:

```json
{
  "...existing fields...",
  "research": {
    "nodes": [
      {
        "id": "read_the_docs",
        "name": "Read the Docs",
        "branch": "efficiency",
        "min_tier": "coffee_table",
        "base_cost": 500,
        "cost_scale": 1.8,
        "effect_type": "idle_income",
        "effect_value": 0.02,
        "description": "Study documentation to improve operational efficiency"
      }
    ]
  }
}
```

This enables the client to display research node info, calculate costs, and preview
effects without hardcoding any values.

---

## 7. Frontend Changes

### 7.1 New Tab: Research

Add a `'research'` entry to the `TABS` array in `App.tsx`:

```
{ id: 'research', label: 'Research', icon: ' ? ', color: '#8b5cf6' }
```

Position it after 'upgrades' in the tab order. Show it only when the player has at
least one available research node (i.e., any node's `min_tier` is met by the current
tier).

### 7.2 ResearchPanel Component

New file `apps/desktop/src/components/ResearchPanel.tsx`.

**Layout:** Grid grouped by branch (Efficiency, Reputation, Infrastructure, Mastery),
similar to how `UpgradePanel.tsx` groups by type (cooling, networking, automation,
knowledge). Each branch is a panel card containing its nodes.

**Per-node display:**
- Node name and description
- Current level (e.g., "Lv. 5")
- Cumulative effect (e.g., "+10% idle income")
- Cost for next level (e.g., "5,249 CU")
- Buy button (disabled if insufficient CU or tier not met)
- "Buy Max" button to invoke `bulk_buy_research`
- Tier gate indicator if the node is not yet available (greyed out with "Requires 24U
  Rack" text)

**Cost calculation (client-side for display only):**
The client reads the node's `base_cost` and `cost_scale` from the config and the
current level from `state.research_levels`, then computes
`Math.floor(baseCost * Math.pow(costScale, level))` for the display. The server
is authoritative for the actual cost at purchase time.

### 7.3 GameStore Actions

Add to the Zustand store interface and implementation:

```typescript
buyResearch: (node: string) => Promise<void>;
buyMaxResearch: (node: string) => Promise<void>;
```

Following the existing action pattern (send via `wsClient.sendAction`, set state from
response).

### 7.4 Type Updates

In `apps/desktop/src/api.ts`:

```typescript
export interface ResearchLevelItem {
  id: string;
  game_state_id: string;
  research_node: string;
  level: number;
  updated_at: string;
}

export interface ResearchNodeConfig {
  id: string;
  name: string;
  branch: string;
  min_tier: string;
  base_cost: number;
  cost_scale: number;
  effect_type: string;
  effect_value: number;
  description: string;
}

export interface ResearchConfig {
  nodes: ResearchNodeConfig[];
}
```

Add to `GameState`:
```typescript
research_levels: ResearchLevelItem[];
```

Add to `GameConfig`:
```typescript
research: ResearchConfig;
```

### 7.5 useIdleTick Update

The `useIdleTick` hook must include research bonuses in its rate calculation to match
the server. After computing `baseMultiplier`, `knowledgeBoost`, `netMult`, `repMult`:

```typescript
// Research bonuses
let researchIdleMult = 1.0;
let researchRepMult = 1.0;
let researchMoneyMult = 1.0;
if (state.research_levels && config.research?.nodes) {
  for (const rl of state.research_levels) {
    const node = config.research.nodes.find(n => n.id === rl.research_node);
    if (node) {
      const bonus = rl.level * node.effect_value;
      if (node.effect_type === 'idle_income') researchIdleMult += bonus;
      else if (node.effect_type === 'reputation_gain') researchRepMult += bonus;
      else if (node.effect_type === 'money_income') researchMoneyMult += bonus;
    }
  }
}
```

Then apply in the rate calculations:
```typescript
const baseComputeRate = totalCompute * baseMultiplier * knowledgeBoost * netMult * researchIdleMult;
const repRate = serviceRep * heatPenalty * throttle * repMult * researchRepMult + coloRepRate;
const moneyRate = serviceMoney * heatPenalty * throttle * researchMoneyMult + coloMoneyRate - totalExpenses;
```

### 7.6 Shared Types Package

The `@homelab-game/shared` package is currently stale and unused (see
`docs/spec/code-quality.md` section 1.2). Adding research types there would perpetuate
drift. Skip updating the shared package -- the authoritative types live in
`apps/desktop/src/api.ts`.

---

## 8. Migration & Rollout

### 8.1 Database Migration

Migration file `011_research_tree.sql`:

```sql
-- 011_research_tree.sql
-- Research tree: permanent percentage boosts with infinite levels

CREATE TABLE research_levels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_state_id UUID NOT NULL REFERENCES game_states(id) ON DELETE CASCADE,
    research_node VARCHAR(50) NOT NULL,
    level INT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(game_state_id, research_node)
);

CREATE INDEX idx_research_levels_game_state ON research_levels(game_state_id);
```

Apply:
```bash
cat /root/project/apps/backend/internal/database/migrations/011_research_tree.sql | sudo -u postgres psql -d homelab_game
echo "GRANT ALL ON research_levels TO homelab_game;" | sudo -u postgres psql -d homelab_game
```

### 8.2 Rollout

Since everything runs on a single server with no separate environments:

1. Apply the migration (creates empty table -- zero impact on existing players)
2. Deploy the backend code (rebuild and restart)
3. Deploy the frontend (rebuild)

Existing players will see an empty `research_levels` array in their state response
and the new Research tab will appear. No data migration needed -- new players and
existing players start at level 0 for all nodes.

### 8.3 Rollback

If issues arise:
- **Backend rollback**: Revert the Go code changes, rebuild, restart. The
  `research_levels` table remains but is unused.
- **Frontend rollback**: Revert the frontend changes, rebuild. The research tab
  disappears.
- **Schema rollback** (if needed): `DROP TABLE research_levels;` -- no data loss
  for other systems since nothing else references this table.

---

## 9. Risks & Open Questions

### Risks

1. **Balance risk: research bonuses may be too strong or too weak.**
   - Mitigation: The catalog is data-only -- adjusting `EffectValue` or `CostScale`
     requires no schema changes. Start conservative (lower effect values) and tune
     based on observed player progression.
   - Mitigation: The cost examples in section 4.1 show that reaching +100% on any
     single channel requires spending trillions of CU, which bounds the maximum
     practical impact.

2. **ProcessAction signature change ripples.**
   - Adding `researchLevels` to `ProcessAction` and `ProcessIdleProgress` touches
     two widely-called functions. However, each is called from exactly one place in
     `game.go`, so the blast radius is small.

3. **Client interpolation drift if research bonuses are omitted from useIdleTick.**
   - The counter will visibly jump on each server sync. This must be addressed in
     the same deployment as the server changes.

4. **Integer overflow at extreme research levels.**
   - Mitigated by the overflow check in the cost calculation helper. The engine
     rejects purchases when cost would exceed `int64`.

### Open Questions

1. **Should the `job_reward` research effect apply to the click's knowledge boost as
   well, or only to the base reward?** The current `runJob` code is
   `reward * knowledgeBoost`. If research multiplies this result, it compounds with
   knowledge. If it multiplies only `reward`, it does not. **Recommendation:** Apply
   research multiplicatively to the full click value (after knowledge boost), matching
   how `idle_income` research multiplies the full compute income line. This is simpler
   and more intuitive for players.

2. **Should research node costs scale with prestige count (like tier upgrades and SaaS
   unlock)?** The existing `prestigeCostScale()` function scales certain costs by
   prestige count to maintain challenge. **Recommendation:** No. Research is the
   permanent power ramp -- it should feel like it gets easier to invest in over time,
   not harder. Prestige scaling on research costs would undermine the "feel like you
   get ahead" goal. The exponential cost scaling within each node provides sufficient
   natural resistance.

3. **Should the Research tab be available from the very start, or gated behind a tier?**
   **Recommendation:** Available from coffee_table tier onward, since some nodes
   (`read_the_docs`, `blog_writing`, `scripting_mastery`) are gated at coffee_table.
   Show the tab whenever the player's tier meets any node's `min_tier` requirement
   (which is always true for coffee_table nodes). Nodes the player cannot yet access
   are shown greyed out with tier requirements.

---

## 10. Testing Strategy

### Unit Tests (Engine)

- **Cost calculation:** Verify `researchCost(baseCost, costScale, level)` produces
  correct values for levels 0, 1, 5, 10, and returns overflow=false at high levels.
- **buyResearch action:** Test validation (unknown node, tier gate, insufficient CU,
  overflow), successful purchase (CU deducted, level incremented), and repeated
  purchase (level 1 -> 2 -> 3).
- **bulkBuyResearch action:** Test that it purchases max affordable levels and stops
  when CU runs out or overflow is reached.
- **aggregateResearchBonuses:** Verify additive stacking within effect type and correct
  aggregation across types.
- **ProcessIdleProgress with research:** Verify that research multipliers correctly
  affect compute, reputation, and money income lines.
- **runJob with research:** Verify job_reward research bonus applies to click reward.
- **Prestige persistence:** Verify that after calling `prestige()`, research levels
  passed to the next `ProcessIdleProgress` call still apply bonuses.

### Integration Tests (Handler)

- **Full action flow:** POST buy_research, verify response includes updated
  research_levels, verify subsequent GET state includes the levels.
- **Prestige flow:** Buy research, prestige, verify research levels survive in the
  next state response.

### Frontend Tests

- **ResearchPanel rendering:** Verify nodes display with correct levels, costs, and
  effects from mock state/config.
- **useIdleTick:** Verify rate calculation includes research bonuses (compare computed
  rate with expected rate given known research levels).

---

## 11. Observability & Operational Readiness

### Key Signals

- **Research purchase frequency:** Log each `buy_research` and `bulk_buy_research`
  action (already covered by existing action logging if added, or via the game state
  update pattern).
- **Max research level per player:** Monitor via periodic SQL query on
  `research_levels` to detect if any player reaches levels where overflow protection
  activates (early warning for balance issues).
- **Query performance:** The new `SELECT ... FROM research_levels WHERE game_state_id`
  query adds to per-request DB time. If the 9-query (was 8) fan-out becomes
  problematic, it will show up as increased p99 latency on `/api/game/state` and
  `/api/game/action`.

### 3am Diagnosability

- If a player reports research not working: check `research_levels` table for their
  `game_state_id`, verify catalog node IDs match, verify the bonus is being aggregated
  in `ProcessIdleProgress`.
- If counter interpolation drifts: verify `useIdleTick` research bonus calculation
  matches the server's `aggregateResearchBonuses` logic.

---

## 12. Implementation Phases

### Phase 1: Backend Foundation (Size: M)

**Files:** `catalog/research.go`, `models/game_state.go`, `queries/research.go`,
migration `011_research_tree.sql`

- Define the research node catalog (10 nodes, 4 branches)
- Add `ResearchLevel` model
- Create the `research_levels` table and query struct
- Add `GetResearchNode(id)` and `GetAvailableResearchNodes(tier)` catalog functions
- Apply migration and grant permissions
- Wire `ResearchLevelQueries` into `main.go` and `GameHandler`

**Dependencies:** None. Can be done first.

### Phase 2: Engine Logic (Size: M)

**Files:** `engine/engine.go`, `engine/config.go`

- Add `researchLevels` parameter to `ProcessIdleProgress` and `ProcessAction`
- Implement `buyResearch` and `bulkBuyResearch` action handlers
- Implement `aggregateResearchBonuses` helper
- Apply research multipliers to compute, reputation, and money income lines in
  `ProcessIdleProgress`
- Apply `job_reward` research bonus in `runJob`
- Add overflow-safe cost calculation helper
- Add `ResearchLevel` field to `ActionResult`
- Add `ResearchConfig` to `GameConfig` and populate in `GetConfig()`

**Dependencies:** Phase 1.

### Phase 3: Handler Integration (Size: S)

**Files:** `handlers/game.go`

- Load research levels in `runUserTick` and the action handler (add to the query
  fan-out alongside hardware, services, etc.)
- Pass research levels to `ProcessIdleProgress` and `ProcessAction`
- Persist `ActionResult.ResearchLevel` via UPSERT
- Add `ResearchLevels` to `fullStateResponse` and `buildResponse`
- Ensure prestige cleanup does NOT delete research levels (verify by absence)

**Dependencies:** Phase 1, Phase 2.

### Phase 4: Frontend (Size: M)

**Files:** `api.ts`, `gameStore.ts`, `App.tsx`, new `ResearchPanel.tsx`,
`useIdleTick.ts`

- Add TypeScript interfaces for research levels and config
- Add `research_levels` to `GameState` interface
- Add `research` to `GameConfig` interface
- Create `ResearchPanel` component with branch grouping, level display, cost
  display, buy/buy-max buttons, tier gate indicators
- Add `buyResearch` and `buyMaxResearch` actions to the Zustand store
- Add Research tab to `App.tsx`
- Update `useIdleTick` to include research bonuses in rate calculation

**Dependencies:** Phase 3 (needs the backend serving research data in responses).

### Phase 5: Polish & Balance (Size: S)

- Verify client interpolation matches server math (no counter jumps)
- Update `wipe_player_progress.sql` to include `DELETE FROM research_levels`
- Manual balance testing: play through coffee_table to 48U with research purchases,
  verify progression feels right
- Adjust catalog values (base costs, cost scales, effect values) based on testing

**Dependencies:** Phase 4.
