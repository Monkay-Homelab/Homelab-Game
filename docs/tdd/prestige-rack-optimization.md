---
project: "homelab-game"
maturity: "draft"
last_updated: "2026-03-21"
updated_by: "@staff-engineer"
scope: "Prestige Rack Optimization: spend CU before colo to boost the frozen colo rack income snapshot"
owner: "@staff-engineer"
dependencies:
  - ../spec/architecture.md
  - ../spec/code-quality.md
---

# Prestige Rack Optimization

## 1. Problem Statement

### What

Add a "Rack Optimization" mechanic that allows players to spend Compute Units (CU) before
prestiging to boost the income snapshot stored on the resulting Colo Rack. Each optimization
purchase adds a percentage bonus to the rack's frozen `ComputePerTick`, `ReputationPerTick`,
and `MoneyPerTick` values. The cost escalates (doubling per purchase), and the optimization
level resets after prestige -- making it a per-run CU dump.

### Why Now

This is one of three CU sink features being built to address the late-game CU inflation
problem (alongside Overclock Mode and Research Tree -- see `docs/compute-unit-burn-ideas.md`).
Late-game 48U players accumulate CU orders of magnitude faster than they can spend it. The
current finite sinks (hardware, upgrades, services) are exhausted well before the player is
ready to prestige. This feature converts idle CU surplus into a meaningful pre-prestige
investment, transforming the prestige moment from "sacrifice everything" into "invest
everything."

### Why This Feature Specifically

The prestige moment is the highest-stakes decision in the game. Currently, a player who has
200M CU and nothing left to buy just... colos. That CU vanishes. Rack Optimization gives it
purpose: every CU you pour in before colo makes your legacy rack permanently stronger. It
creates a genuine timing tension -- do I colo now, or keep earning to optimize further?

### Constraints

- **Server-authoritative**: The optimization level and cost must be computed and validated
  server-side. Clients display the state but never determine it.
- **Per-run, not permanent**: `RackOptimization` resets to 0 on prestige. This is an
  investment within a single prestige cycle, not a cross-prestige upgrade.
- **Integer currencies**: CU is `int64`. Cost calculations must use integer arithmetic to
  avoid floating-point drift.
- **Client interpolation parity**: The optimization multiplier does NOT affect idle tick
  rates -- it only modifies the snapshot at prestige time. No changes to `useIdleTick.ts`
  rate calculations are needed. The frontend only needs to display the current optimization
  level and preview the boosted snapshot values.
- **Payload size**: One new field on `GameState` (`rack_optimization`). No new tables,
  no new per-request queries. The ideation doc emphasizes storing sink state as columns on
  `game_states` to avoid connection pool pressure.
- **Existing action pattern**: Follows the established `ProcessAction` switch-case pattern
  with a new `optimize_rack` action type.

### Acceptance Criteria

1. A new `optimize_rack` action is available when the player is at 48U tier with SaaS unlocked
   (same prerequisites as colo itself).
2. Each optimization purchase deducts CU at an escalating cost (base cost * 2^level) and
   increments the optimization level by 1.
3. The current optimization bonus is displayed as a percentage (e.g., "+40% colo rack income").
4. When the player colos, the colo rack snapshot (`ComputePerTick`, `ReputationPerTick`,
   `MoneyPerTick`) is multiplied by `1.0 + (optimization_level * bonus_per_level)` before
   being written to the `colo_racks` table.
5. After prestige, `RackOptimization` resets to 0.
6. The optimization UI is accessible from the Datacenter panel, adjacent to the existing
   "Colocate Rack (Prestige)" button.
7. The UI shows: current level, current bonus percentage, cost of next optimization, and a
   preview of what the colo rack income would be with the current bonus applied.
8. Players cannot optimize when they have insufficient CU.
9. No maximum level cap -- cost escalation is the balancing mechanism.

## 2. Context & Prior Art

### Existing Codebase

**Prestige function** (`engine.go:721-806`): The `prestige()` method requires 48U tier + SaaS
unlocked. It iterates all hardware (with component upgrade bonuses) and services to compute
`totalCompute`, `totalRep`, and `totalMoney`. These are written directly to a new `ColoRack`
record. The function then resets `GameState` fields to starting values, preserving
`KnowledgePoints`, `BitcoinBalance`, datacenter ownership, and `ColoCount`.

**ColoRack model** (`models/game_state.go:117-126`): Stores `ComputePerTick`, `ReputationPerTick`,
`MoneyPerTick` as `int64`. These are frozen snapshots -- they never change after creation.

**Colo rack income** (`handlers/game.go:309-319`, `useIdleTick.ts:93-106`): Both server and
client iterate colo racks, applying `DatacenterIncomeMultiplier` and a 0.9^i decay per rack.
The optimization multiplier would be baked into the snapshot at prestige time, so it compounds
with `DatacenterIncomeMultiplier` and decay automatically -- no changes to this income
calculation are needed.

**Action dispatch** (`engine.go:234-278`): Clean switch statement with 19 action types. Each
action is a method on `Engine` that mutates `GameState` and returns an `ActionResult`. Adding
a new case requires one line in the switch and one method.

**Donate CU action** (`engine.go:930-946`): Closest precedent for a simple "deduct CU, update
state" action. Takes a payload with an amount, validates balance, mutates two fields. The
`optimize_rack` action is similarly simple but without a user-specified amount (cost is
deterministic from the current level).

**GameState persistence** (`queries/game_state.go`): All GameState fields are in a single
`UPDATE` statement with positional parameters. Adding a new column requires updating
`gsColumns`, `gsFields`, and the `Update` query. This is a well-established pattern -- Bitcoin
balance was the most recent addition via migration 010.

**Frontend Datacenter panel** (`DatacenterPanel.tsx`): Already contains the "Colocate Rack
(Prestige)" button and colo rack display. The optimization button belongs here, adjacent to
the prestige button.

### How Other Games Solve This

- **Cookie Clicker** "Permanent Upgrade Slots": Pre-ascension investments that carry into the
  next run. Similar emotional hook -- spend now to boost your legacy.
- **Idle Champions** "Modron Automation": Pre-reset investments that amplify the value of the
  reset. Creates the same "should I reset now or invest more?" tension.
- **Exponential Idle** "Supremacy": Multipliers you build before resetting that directly
  affect post-reset progression speed.

The common pattern: give players a way to feel like they are "loading up" before the reset,
making the reset feel like a launch rather than a loss.

## 3. Alternatives Considered

### Alternative A: Multiplier on GameState, Applied at Snapshot Time (Recommended)

Store a `RackOptimization int` (level, not a float multiplier) on `GameState`. At prestige
time, compute `bonus = 1.0 + float64(level) * bonusPerLevel` and multiply the snapshot values
before writing the `ColoRack`. Reset level to 0 in the prestige reset block.

**Strengths:**
- Simplest implementation -- one new column, one new action, a 3-line modification to
  `prestige()`.
- The bonus is baked into the `ColoRack` record permanently. No ongoing calculation needed.
- No changes to colo rack income processing (server or client).
- Integer level avoids floating-point accumulation issues.
- Cost calculation from level is deterministic: `baseCost * 2^level`.

**Weaknesses:**
- The bonus is invisible once baked into the colo rack record (you can't see "this rack was
  optimized +40%"). This is acceptable -- the rack's raw income numbers are higher, which is
  the visible result.

### Alternative B: Separate Multiplier Stored on ColoRack

Add an `OptimizationBonus float64` column to the `colo_racks` table. Apply it during income
calculation alongside `DatacenterIncomeMultiplier`.

**Strengths:**
- The per-rack bonus is visible and inspectable.
- Could display "optimized" badge per rack in the UI.

**Weaknesses:**
- Requires a schema change to `colo_racks` (a cross-prestige-persistent table).
- Adds a multiplication to the colo rack income loop on every tick (server + client).
- `useIdleTick.ts` must be updated to include the new multiplier -- any mismatch causes
  counter jumps.
- More complex for marginal display benefit.

### Alternative C: Float64 Multiplier on GameState (Direct)

Store `RackOptimization float64` directly as the multiplier (starts at 1.0, incremented by
+0.10 per purchase).

**Strengths:**
- Slightly simpler prestige code (`totalCompute *= gs.RackOptimization`).

**Weaknesses:**
- Floating-point accumulation after many purchases (e.g., 1.0 + 0.10 * 20 = 2.999...98).
  Int64 income values then get subtly wrong snapshots.
- Cost calculation from a float is lossy -- need to reverse-engineer the level.
- Harder to display "Level 5 (+50%)" in the UI without the level.

### Recommendation

**Alternative A**. Store the integer level. Compute the float multiplier only at the moment
it is needed (prestige time). This avoids floating-point drift entirely and gives the UI a
clean level number for display. The ideation doc suggests a `float64` field, but an `int`
level is strictly superior for the reasons above.

## 4. Architecture & System Design

### Data Flow

```
Player clicks "Optimize Rack" in Datacenter panel
  -> WebSocket action { type: "optimize_rack" }
  -> GameHandler.processAction()
  -> Engine.ProcessAction() switch -> Engine.optimizeRack()
     1. Validate: tier == 48U, SaaS unlocked
     2. Calculate cost: baseCost * 2^level
     3. Validate: ComputeUnits >= cost
     4. Deduct CU, increment RackOptimization
     5. Return ActionResult{}
  -> GameHandler persists GameState (existing Update flow)
  -> Full state response pushed via WebSocket

Player clicks "Colocate Rack (Prestige)"
  -> Engine.prestige()
     1. Compute totalCompute, totalRep, totalMoney (existing logic)
     2. NEW: Apply optimization bonus:
        bonus := 1.0 + float64(gs.RackOptimization) * BonusPerOptLevel
        totalCompute = int64(float64(totalCompute) * bonus)
        totalRep = int64(float64(totalRep) * bonus)
        totalMoney = int64(float64(totalMoney) * bonus)
     3. Create ColoRack with boosted values
     4. Reset gs.RackOptimization = 0 (in the reset block)
     5. Return ActionResult{NewColoRack, Prestige: true}
```

### Components Modified

| Component | File | Change |
|-----------|------|--------|
| Model | `models/game_state.go` | Add `RackOptimization int` field |
| Engine | `engine/engine.go` | Add `optimizeRack()` method, case in `ProcessAction`, modify `prestige()` snapshot |
| Config | `engine/config.go` | Add `RackOptimization` config section |
| DB Queries | `queries/game_state.go` | Add column to `gsColumns`, `gsFields`, `Update` |
| DB Migration | `migrations/011_rack_optimization.sql` | `ALTER TABLE game_states ADD COLUMN rack_optimization INT NOT NULL DEFAULT 0` |
| Frontend API types | `apps/desktop/src/api.ts` | Add `rack_optimization` to `GameState` interface |
| Frontend store | `stores/gameStore.ts` | Add `optimizeRack` action |
| Frontend UI | `components/DatacenterPanel.tsx` | Add optimization button, level display, cost preview, snapshot preview |
| Frontend config | `api.ts` (`GameConfig`) | Add `rack_optimization` config section |
| Shared types | `packages/shared/src/types/game.ts` | Add to `ActionType` enum |

## 5. Data Models & Storage

### GameState Addition

```go
// In models/game_state.go, add to GameState struct:
RackOptimization int `json:"rack_optimization"`
```

This is the optimization level (0 = no optimization, 1 = first purchase, etc.). Not a
multiplier -- the multiplier is computed from it.

### Database Migration (011_rack_optimization.sql)

```sql
-- 011_rack_optimization.sql
-- Add rack optimization level to game_states (resets on prestige)
ALTER TABLE game_states ADD COLUMN rack_optimization INT NOT NULL DEFAULT 0;
```

Single column, no new tables. Default 0 means existing players are unaffected.

### Query Updates

`gsColumns` in `queries/game_state.go` must include `rack_optimization` in the SELECT column
list. `gsFields` must add `&gs.RackOptimization` to the scan targets. The `Update` method
must add `rack_optimization = $N` to the SET clause and pass `gs.RackOptimization` in the
parameter list. All positional parameters after the insertion point shift by 1.

## 6. API Contracts

### Action: `optimize_rack`

**Request** (via WebSocket action):
```json
{ "type": "optimize_rack" }
```

No payload required. The cost is deterministic from the current optimization level.

**Success Response**: Standard full game state (same as all other actions). The
`rack_optimization` field will be incremented.

**Error Responses**:
- `"must be at 48U rack tier to optimize"` -- player not at max tier
- `"must have SaaS unlocked to optimize"` -- SaaS not unlocked
- `"not enough compute units (need X, have Y)"` -- insufficient CU

### Config Addition

Add a `RackOptimization` section to `GameConfig`:

```go
type RackOptimizationConfig struct {
    BaseCost      int64   `json:"base_cost"`
    CostMultiplier float64 `json:"cost_multiplier"`
    BonusPerLevel float64 `json:"bonus_per_level"`
}
```

```json
{
  "rack_optimization": {
    "base_cost": 100000,
    "cost_multiplier": 2.0,
    "bonus_per_level": 0.10
  }
}
```

This config is consumed by both the engine (for validation) and the frontend (for cost/bonus
display). Having it in the config avoids hardcoded magic numbers and enables tuning without
code changes.

### GameState Response Addition

The `rack_optimization` field appears in every full state response as part of the embedded
`GameState`:

```json
{
  "rack_optimization": 5,
  ...
}
```

The frontend computes the display values:
- **Current bonus**: `rack_optimization * bonus_per_level * 100` (e.g., "50%")
- **Next cost**: `base_cost * 2^rack_optimization` (e.g., "3,200,000 CU")
- **Snapshot preview**: Current hardware/service income * (1 + bonus) -- informational only

## 7. Cost Curve Design

### Parameters

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| `BaseCost` | 100,000 CU | Matches Bitcoin trading cost (100K CU per tx). Accessible to a freshly-at-48U player who has accumulated surplus. |
| `CostMultiplier` | 2.0 (doubling) | Standard exponential escalation. Level 10 costs ~102M CU. Level 15 costs ~3.2B CU. Creates natural diminishing returns without a hard cap. |
| `BonusPerLevel` | 0.10 (10% per level) | Level 1 = +10%, Level 5 = +50%, Level 10 = +100%. Feels meaningful at every level. Compounds well with datacenter income multiplier. |

### Cost Table (First 10 Levels)

| Level | Cost (CU) | Cumulative Cost | Bonus |
|-------|-----------|-----------------|-------|
| 1 | 100,000 | 100,000 | +10% |
| 2 | 200,000 | 300,000 | +20% |
| 3 | 400,000 | 700,000 | +30% |
| 4 | 800,000 | 1,500,000 | +40% |
| 5 | 1,600,000 | 3,100,000 | +50% |
| 6 | 3,200,000 | 6,300,000 | +60% |
| 7 | 6,400,000 | 12,700,000 | +70% |
| 8 | 12,800,000 | 25,500,000 | +80% |
| 9 | 25,600,000 | 51,100,000 | +90% |
| 10 | 51,200,000 | 102,300,000 | +100% |

### Why No Hard Cap

The cost doubling is the cap. At level 20, a single purchase costs 104B CU. At level 30,
it costs 107T CU. The exponential cost curve makes each subsequent level proportionally
harder to achieve even as player income scales. A hard cap would be arbitrary and could be
hit by dedicated players, removing the sink. The uncapped design ensures this feature absorbs
CU at every income level indefinitely.

### Overflow Protection

Cost formula: `baseCost * 2^level`. With `baseCost = 100,000` and `int64` max of
~9.2 * 10^18, overflow occurs at level ~46 (`100,000 * 2^46 = ~7 * 10^18`). The engine
must check for this:

```go
if level >= 46 {
    return nil, fmt.Errorf("optimization level too high")
}
```

Alternatively, use a safe multiplication check. In practice, reaching level 46 requires
~1.4 * 10^19 cumulative CU -- well beyond any realistic gameplay. But the guard is
necessary for correctness.

## 8. Engine Logic

### New Method: `optimizeRack`

```
func (e *Engine) optimizeRack(gs *models.GameState) (*ActionResult, error)
    1. Validate gs.Tier == TierRack48U (error: "must be at 48U rack tier to optimize")
    2. Validate gs.SaasUnlocked (error: "must have SaaS unlocked to optimize")
    3. level := gs.RackOptimization
    4. Overflow guard: if level >= 46, error "optimization level at maximum"
    5. cost := int64(100000) << uint(level)   // 100,000 * 2^level, using bit shift
    6. Validate gs.ComputeUnits >= cost (error with both values)
    7. gs.ComputeUnits -= cost
    8. gs.RackOptimization++
    9. return &ActionResult{}, nil
```

Using bit shift (`<< uint(level)`) for the power-of-2 cost avoids floating-point entirely.
This is exact for all integer levels.

### Modified Method: `prestige`

In the existing `prestige()` method, after computing `totalCompute`, `totalRep`, `totalMoney`
(lines 734-748) and before creating the `ColoRack` (line 762):

```
    // Apply rack optimization bonus to snapshot
    if gs.RackOptimization > 0 {
        bonus := 1.0 + float64(gs.RackOptimization) * 0.10
        totalCompute = int64(float64(totalCompute) * bonus)
        totalRep = int64(float64(totalRep) * bonus)
        totalMoney = int64(float64(totalMoney) * bonus)
    }
```

In the reset block (after line 802), add:

```
    gs.RackOptimization = 0
```

### ProcessAction Switch

Add one case:

```
    case "optimize_rack":
        return e.optimizeRack(gs)
```

No payload needed, so no JSON unmarshalling.

## 9. Frontend Changes

### DatacenterPanel.tsx

The optimization UI should appear in the left "Actions" column of `DatacenterPanel`, between
the datacenter info card and the "Colocate Rack (Prestige)" button. It is only visible when
`canColo` is true (48U + SaaS unlocked).

**Elements:**
1. **Level display**: "Rack Optimization: Level N (+X%)"
2. **Cost display**: "Next: [cost] CU"
3. **Optimize button**: "Optimize Rack -- [cost] CU", disabled when insufficient CU
4. **Snapshot preview**: "Colo rack income with bonus: +X CU/tick, +Y Rep/tick, +$Z/tick"
   This is a client-side calculation showing what the colo rack would look like if the player
   prestiges now, using the same hardware/service iteration as the server's `prestige()`.

**Styling**: Use the cyan accent (`var(--accent-cyan)`) to match the datacenter panel theme.
The button should follow the existing pattern: `rgba(6,182,212,0.1)` background, cyan text,
`1px solid rgba(6,182,212,0.2)` border.

### GameStore

Add `optimizeRack` action to the store, following the exact pattern of `colo`:

```typescript
optimizeRack: async () => {
    set({ error: null });
    try {
        const state = await wsClient.sendAction('optimize_rack');
        set({ state });
    } catch (e) {
        set({ error: (e as Error).message });
    }
}
```

### API Types (api.ts)

Add to `GameState` interface:

```typescript
rack_optimization: number;
```

Add to `GameConfig` interface:

```typescript
rack_optimization: RackOptimizationConfig;
```

Add new interface:

```typescript
export interface RackOptimizationConfig {
    base_cost: number;
    cost_multiplier: number;
    bonus_per_level: number;
}
```

### Shared Types (packages/shared)

Add `OptimizeRack = 'optimize_rack'` to `ActionType` enum in
`packages/shared/src/types/game.ts`.

### Snapshot Preview Calculation

The preview shows the player what their colo rack income would be if they prestige now with
the current optimization level. This requires summing hardware compute (with component upgrade
bonuses) and service income on the client side. The `DatacenterPanel` already receives the
full `state` object. The calculation mirrors the server's `prestige()` snapshot logic:

```typescript
const totalCompute = (state.hardware || []).reduce((sum, h) => {
    let compute = h.compute_per_tick;
    for (const cu of (state.component_upgrades || [])) {
        if (cu.hardware_id === h.id) compute += Math.floor(h.compute_per_tick * cu.compute_bonus / 100);
    }
    return sum + compute;
}, 0) + (state.services || []).reduce((sum, s) => sum + s.compute_per_tick, 0);

const totalRep = (state.services || []).reduce((sum, s) => sum + s.reputation_per_tick, 0);
const totalMoney = (state.services || []).reduce((sum, s) => sum + s.money_per_tick, 0);

const bonus = 1.0 + state.rack_optimization * config.rack_optimization.bonus_per_level;
const previewCompute = Math.floor(totalCompute * bonus);
const previewRep = Math.floor(totalRep * bonus);
const previewMoney = Math.floor(totalMoney * bonus);
```

## 10. Migration & Rollout

### Migration

Single SQL statement. Non-breaking (`DEFAULT 0`). Can be applied while the server is running.
No data backfill needed -- all existing players start at optimization level 0.

```bash
cat /root/project/apps/backend/internal/database/migrations/011_rack_optimization.sql | sudo -u postgres psql -d homelab_game
echo "GRANT ALL ON game_states TO homelab_game;" | sudo -u postgres psql -d homelab_game
```

The `GRANT` is technically unnecessary since `game_states` already has grants, but it is
included for the pattern (the CLAUDE.md instructions mention granting on new tables).

### Rollout Steps

1. Apply DB migration (zero downtime -- `ALTER TABLE ADD COLUMN` with default is non-blocking
   in PostgreSQL).
2. Deploy updated backend (kill old process, start new one). During the ~1 second gap, clients
   will see a WebSocket disconnect and auto-reconnect.
3. Deploy updated frontend (rebuild Vite, which serves on port 3000).
4. Verify: Create a test account at 48U with SaaS unlocked, optimize a few levels, then colo.
   Confirm the colo rack income is boosted.

### Rollback

If issues are found:
- Revert the backend code and restart. The `rack_optimization` column remains in the DB but
  is ignored (it is just an `int` column with a default of 0).
- No need to roll back the migration -- the column is harmless when unused.
- If the column must be removed: `ALTER TABLE game_states DROP COLUMN rack_optimization;`

## 11. Risks & Open Questions

### Known Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Cost tuning is wrong (too cheap or too expensive) | Medium | Low (gameplay feel) | All parameters are in `GameConfig`, tuneable without code changes. Monitor first few players' optimization levels and adjust. |
| Integer overflow in cost calculation at high levels | Low | High (panic/crash) | Guard at level 46. Use bit shift instead of `math.Pow` to avoid float intermediaries. |
| Snapshot preview disagrees with server snapshot | Medium | Low (cosmetic confusion) | The preview is informational. The server is authoritative. Document that preview is approximate. |
| Players optimize then close the game before colo | None (by design) | None | The optimization level persists on GameState until prestige. They can return and colo later. |

### Open Questions

1. **Should there be a confirmation dialog before colo that shows the optimization bonus?**
   Currently, clicking "Colocate Rack (Prestige)" immediately fires the action. A confirmation
   showing "Your rack will be optimized +50% -- Proceed?" would add safety but no such modal
   pattern exists in the codebase today. **Recommendation**: Defer to a separate UX pass.
   The optimization level is already visible in the panel.

2. **Should the bonus use the config value or a constant?** The ideation doc mentions "+10%
   per purchase." Using the config makes it tuneable. **Recommendation**: Config value,
   consistent with every other numeric parameter in the game.

3. **Should optimizing be available at earlier tiers?** The ideation doc says "before
   prestiging" which implies 48U. However, one could argue optimization should be available
   at any rack tier to give mid-tier players a CU sink. **Recommendation**: Gate on 48U +
   SaaS unlocked (same as colo) for v1. It is thematically coherent -- you are optimizing
   the rack *for colocation*. Broadening the tier requirement can be a follow-up.

### Assumptions

- The 10% per level bonus is adequate for the current game economy. This can be tuned via
  config without code changes.
- The 100K CU base cost is appropriate for a player who just reached 48U. A player at that
  stage typically has 1-10M CU.
- The doubling cost curve provides sufficient escalation. If it proves too aggressive, the
  `cost_multiplier` can be lowered (e.g., 1.5x).

## 12. Testing Strategy

### Unit Tests (Engine)

- `TestOptimizeRack_Success`: At 48U with SaaS, sufficient CU. Verify CU deducted, level
  incremented.
- `TestOptimizeRack_InsufficientCU`: Verify error and no state mutation.
- `TestOptimizeRack_WrongTier`: At 24U, verify error.
- `TestOptimizeRack_NoSaas`: At 48U without SaaS, verify error.
- `TestOptimizeRack_CostEscalation`: Optimize 3 times, verify costs are 100K, 200K, 400K.
- `TestOptimizeRack_OverflowGuard`: Set level to 46, verify error on next attempt.
- `TestPrestige_WithOptimization`: Optimize to level 5, then colo. Verify ColoRack income
  is 1.5x the base snapshot. Verify `RackOptimization` is reset to 0.
- `TestPrestige_WithoutOptimization`: Standard colo (level 0). Verify no bonus applied,
  backward compatibility with existing behavior.

### Integration Tests

- Full action flow: `optimize_rack` via WebSocket, verify state response contains incremented
  `rack_optimization` and reduced `compute_units`.
- Prestige flow: `optimize_rack` x3, then `colo`, verify the new colo rack's income values
  match expected boosted amounts.

### Manual Verification

- Play through to 48U with SaaS, optimize multiple levels, colo. Verify the colo rack in the
  Datacenter panel shows boosted income values.
- Verify the preview matches the actual colo rack values after prestige.
- Verify the optimization level is 0 after returning to coffee table tier.

## 13. Observability & Operational Readiness

### Logging

The existing action handler logs each action type. No additional logging is needed beyond what
the `processAction` handler already provides. If rack optimization tuning data is needed, the
`colo_racks` table already contains the boosted income values -- comparing early-colo racks
(pre-feature) to post-feature racks shows the impact.

### Monitoring

No new alerts. The feature is a simple CU deduction with a field increment. The existing
error handling and logging in `processAction` covers failure cases. If optimization causes
unexpected CU drain, it will be visible in the `compute_units` leaderboard data.

### Diagnosability

If a player reports unexpected colo rack income:
1. Check `game_states.rack_optimization` -- was it > 0 at prestige time?
2. Check the `colo_racks` row -- does the income match `expected_base * (1 + level * 0.10)`?
3. The optimization level resets to 0 after prestige, so the evidence is in the colo rack
   income values themselves.

## 14. Implementation Phases

### Phase 1: Backend (Size: S)

**Dependencies**: None

1. Add `RackOptimization int` to `GameState` model
2. Create migration `011_rack_optimization.sql`
3. Update `gsColumns`, `gsFields`, `Update` query in `queries/game_state.go`
4. Add `RackOptimizationConfig` struct and config values to `config.go`
5. Add `optimizeRack()` method to `engine.go`
6. Add `"optimize_rack"` case to `ProcessAction` switch
7. Modify `prestige()` to apply optimization bonus and reset the level
8. Apply migration to database

### Phase 2: Frontend (Size: S)

**Dependencies**: Phase 1

1. Add `rack_optimization` to `GameState` interface in `api.ts`
2. Add `RackOptimizationConfig` interface and field to `GameConfig` in `api.ts`
3. Add `optimizeRack` action to `gameStore.ts`
4. Add optimization UI section to `DatacenterPanel.tsx` (level display, cost, button,
   snapshot preview)

### Phase 3: Shared Types & Tests (Size: S)

**Dependencies**: Phase 1

1. Add `OptimizeRack` to `ActionType` enum in `packages/shared/src/types/game.ts`
2. Write engine unit tests for `optimizeRack` and modified `prestige`

All three phases are small. Phases 2 and 3 can proceed in parallel once Phase 1 is complete.
Total estimated effort: 2-3 hours for a single engineer familiar with the codebase.
