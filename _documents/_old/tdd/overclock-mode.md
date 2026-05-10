---
project: "homelab-game"
maturity: "draft"
last_updated: "2026-03-21"
updated_by: "@staff-engineer"
scope: "Overclock Mode -- temporary CU-purchased compute multiplier as a repeatable CU sink"
owner: "@staff-engineer"
dependencies:
  - ../spec/architecture.md
  - ../spec/code-quality.md
---

# Overclock Mode

## 1. Problem Statement

### What

Add Overclock Mode to Homelab the Game -- a repeatable CU sink where players spend Compute
Units to activate a time-limited multiplier on all compute income. Multiple tiers offer
increasing multipliers (2x, 3x, 5x) at escalating costs. The overclock generates extra heat,
creating a feedback loop with the cooling system.

### Why Now

The game's CU economy is inflationary. Once a player owns all available hardware, services,
and upgrades, the only repeating CU sinks are Bitcoin trading (100K CU per transaction),
CU donation (pure vanity), and random event losses (trivially small). Late-game players
accumulate CU with nothing meaningful to spend it on. Overclock Mode is the first of three
planned CU sinks (alongside Research Tree and Prestige Rack Optimization, per
`docs/compute-unit-burn-ideas.md`) and was identified as the highest-impact, lowest-effort
quick win because it piggybacks on the existing throttle multiplier pattern.

### Constraints

- **Server-authoritative**: The overclock multiplier and remaining ticks are stored and
  decremented server-side. The client displays the timer but never controls the countdown.
- **No background workers**: Overclock ticks must be decremented lazily inside
  `ProcessIdleProgress`, exactly like `ThrottleTicksRemaining` (engine.go lines 134-140).
  Offline periods must consume overclock ticks correctly.
- **Integer-only currencies**: CU is `int64`. Overclock costs must be integer values.
- **Full state response**: New fields are added to `GameState` and serialized in every
  response. Two new columns on `game_states` -- minimal payload impact.
- **Client interpolation must match server math**: The overclock multiplier must be added
  to the `useIdleTick.ts` rate calculation so counters do not jump on sync.
- **Prestige resets overclock**: Overclock is a per-run buff. It resets on colo/prestige,
  same as throttle.

### Acceptance Criteria

1. A player can activate overclock by sending an `activate_overclock` action with a `tier`
   payload (1, 2, or 3).
2. Activation deducts the tier's CU cost from the player's balance.
3. While overclock is active, all compute income (hardware + services, after all existing
   multipliers) is multiplied by the overclock multiplier.
4. The overclock timer decrements by 1 each server tick (5 seconds). When it reaches 0,
   the multiplier resets to 1.0.
5. If the player activates overclock while one is already active, the new tier **replaces**
   the existing overclock (full cost, timer resets). No stacking, no refunds.
6. Overclock generates additional heat equal to `(multiplier - 1) * current_heat_generated`.
   This heat is applied during `ProcessIdleProgress` and interacts with the existing
   overheat penalty system.
7. Overclock multiplier and remaining ticks reset to defaults (1.0 / 0) on prestige.
8. The client displays: overclock tier label, remaining ticks as a countdown, and the
   boosted compute-per-second rate.
9. The `useIdleTick.ts` interpolation includes the overclock multiplier so counters are
   smooth between server pushes.
10. Overclock state is included in the `GameState` response (`overclock_multiplier`,
    `overclock_ticks_remaining`).
11. Overclock tiers and costs are exposed in the `GameConfig` response so the client
    does not hardcode them.
12. If the player is offline for longer than the remaining overclock ticks, the overclock
    expires (ticks go to 0, multiplier resets) -- but CU earned during the overlapping
    online portion before the overclock expired includes the boost.

## 2. Context & Prior Art

### Existing Throttle Pattern

The throttle system (`ThrottleMultiplier` / `ThrottleTicksRemaining`) is the direct
architectural precedent. It implements a timed multiplier effect on game state:

- **Storage**: Two columns on `game_states` (`DECIMAL(5,2)` and `INT`), two fields on
  the `GameState` Go struct.
- **Tick processing** (engine.go lines 134-140): Each tick decrements
  `ThrottleTicksRemaining`. When it reaches 0, `ThrottleMultiplier` resets to 1.0.
- **Income application** (engine.go line 151): The throttle multiplier is part of the
  `totalMultiplier` chain: `coloMultiplier * idleMultiplier * heatPenalty * eventThrottle`.
- **Client interpolation** (useIdleTick.ts line 82): Reads `throttle_multiplier` and
  applies it as the `throttle` variable in the rate calculation.
- **Prestige reset** (engine.go lines 801-802): Both fields reset to 1.0 / 0.
- **Response** (game.go line 137): `Throttled` boolean derived from
  `ThrottleTicksRemaining > 0`.

Overclock Mode is structurally identical but positive: the multiplier is > 1.0 instead of
< 1.0, it is player-initiated instead of event-triggered, and it costs CU instead of being
free.

### Idle Game Precedent

Temporary boost mechanics are standard in idle/clicker games. Cookie Clicker's "Frenzy"
(7x for 77 seconds), Adventure Capitalist's "Angel Boost" (3x for 4 hours), and Realm
Grinder's "Tax Collection" are all examples of CU-in-for-multiplier-out loops that serve
as effective currency sinks.

## 3. Alternatives Considered

### Alternative A: Separate Overclock Engine (rejected)

Run overclock as an independent subsystem with its own tick counter, separate from the
idle progress loop. This would allow more complex mechanics (stacking, decay curves,
partial-tick resolution) but introduces a second timer system, complicates offline
calculation, and is unnecessary for the simple "multiply and count down" behavior needed.

**Verdict**: Over-engineered. The throttle pattern already solves this problem.

### Alternative B: Stacking Overclocks (rejected)

Allow players to activate multiple overclock tiers simultaneously, with multipliers
stacking multiplicatively (e.g., 2x + 3x = 6x). This creates a more interesting
decision space but is balance-dangerous: a player could trivially stack 5x * 5x * 5x
to break the economy. It also complicates the data model (need an array or multiple
field sets).

**Verdict**: High balance risk, data model complexity. Replacement (new overclock
replaces old) is simpler and prevents abuse. If stacking is desired later, the fields
can be extended.

### Alternative C: Duration Scales with Tier (considered, deferred)

Higher tiers could have shorter durations (e.g., 5x for only 20 ticks vs. 2x for 120
ticks), creating a "burst vs. sustain" tradeoff. This is a good mechanic but adds tuning
complexity for the first release. The initial implementation uses a fixed 60 ticks for all
tiers; duration-per-tier can be added by changing the `OverclockTier` config without any
structural changes.

**Verdict**: Good future enhancement. The data model supports it; only the tier config
changes. Defer to balance tuning after launch.

### Recommended Approach: Throttle-Mirrored with Replacement Semantics

Two new fields on `GameState` (`OverclockMultiplier float64`, `OverclockTicksRemaining int`),
one new action (`activate_overclock`), one multiplication in `ProcessIdleProgress`, and heat
generation proportional to the multiplier. Activating while active replaces the current
overclock (full cost, timer resets). This is the simplest approach that delivers all
acceptance criteria.

## 4. Architecture & System Design

### 4.1 Overclock Tiers

Three tiers, defined as configuration (not hardcoded in engine logic):

| Tier | Multiplier | Cost (CU) | Duration (ticks) | Extra Heat Factor |
|------|-----------|-----------|------------------|------------------|
| 1    | 2.0x      | 50,000    | 60 (5 min)       | 1.0x current heat |
| 2    | 3.0x      | 200,000   | 60 (5 min)       | 2.0x current heat |
| 3    | 5.0x      | 1,000,000 | 60 (5 min)       | 4.0x current heat |

**Cost rationale**: Tier 1 costs 50K (accessible to mid-game players at closet/12U tier).
Tier 2 at 200K targets late-rack players. Tier 3 at 1M targets post-prestige players with
high CU generation. The cost-to-multiplier ratio increases: Tier 1 = 25K per 1x boost,
Tier 2 = 100K per 1x boost, Tier 3 = 250K per 1x boost. This makes higher tiers
proportionally more expensive, which is correct for a CU sink.

**Duration**: All tiers use 60 ticks (300 seconds = 5 minutes at default 5s tick interval).
This is long enough to feel meaningful but short enough to be repeatable. It can be tuned
per-tier via config without code changes.

**Heat factor**: The extra heat is `(multiplier - 1.0) * current_heat_generated`. For a
Tier 3 overclock on a player generating 500W of heat, this adds 2000W of extra heat (4.0 *
500). If their cooling capacity cannot handle it, they trigger the overheat penalty (50%
income reduction), partially negating the overclock benefit. This creates a natural ceiling:
players must invest in cooling upgrades to sustain high-tier overclocks.

### 4.2 Component Changes

```
                    +-----------------+
                    |  GameState      |
                    |  (models)       |
                    +-----------------+
                    | + OverclockMultiplier float64    (new)
                    | + OverclockTicksRemaining int    (new)
                    +-----------------+
                           |
              +------------+------------+
              |                         |
    +---------v---------+     +---------v---------+
    |  Engine            |     |  game_states DB   |
    |  (engine.go)       |     |  (migration 011)  |
    +--------------------+     +-------------------+
    | ProcessIdleProgress|     | overclock_multiplier DECIMAL(5,2) DEFAULT 1.0
    |   - decrement ticks|     | overclock_ticks_remaining INT DEFAULT 0
    |   - apply multiplier|    +-------------------+
    |   - add heat       |
    | ProcessAction      |
    |   - activate_overclock (new case)
    | prestige()         |
    |   - reset overclock fields
    +--------------------+
              |
    +---------v---------+
    |  Config            |
    |  (config.go)       |
    +--------------------+
    | OverclockConfig    |
    |  - Tiers[]         |
    |    {Mult, Cost,    |
    |     Duration,      |
    |     HeatFactor}    |
    +--------------------+
              |
    +---------v---------+          +---------v---------+
    |  GameHandler       |          |  Frontend          |
    |  (game.go)         |          |  (api.ts)          |
    +--------------------+          +--------------------+
    | fullStateResponse  |          | GameState interface |
    |   + Overclocked bool|         |   + overclock_multiplier
    +--------------------+          |   + overclock_ticks_remaining
                                    | GameConfig interface |
                                    |   + overclock: OverclockConfig
                                    +--------------------+
                                              |
                                    +---------v---------+
                                    |  useIdleTick.ts    |
                                    +--------------------+
                                    | + overclockMult in |
                                    |   rate calculation |
                                    +--------------------+
                                              |
                                    +---------v---------+
                                    |  gameStore.ts      |
                                    +--------------------+
                                    | + activateOverclock|
                                    |   action method    |
                                    +--------------------+
```

### 4.3 Engine Logic

**ProcessIdleProgress changes** (engine.go, inside the "INCOME & EVENTS" section):

After the existing throttle decay block (lines 134-140), add overclock tick decay:

```
// Decay overclock over time
if gs.OverclockTicksRemaining > 0 {
    gs.OverclockTicksRemaining--
    if gs.OverclockTicksRemaining <= 0 {
        gs.OverclockMultiplier = 1.0
        gs.OverclockTicksRemaining = 0
    }
}
```

In the multiplier chain (line 151), add the overclock multiplier:

```
overclockMult := gs.OverclockMultiplier
if overclockMult < 1.0 {
    overclockMult = 1.0 // defensive: never let overclock reduce income
}

totalMultiplier := gs.ColoMultiplier * gs.IdleMultiplier * heatPenalty * eventThrottle * overclockMult
```

For heat generation, after the heat recalculation block (line 97), add overclock heat:

```
// Overclock extra heat: (multiplier - 1) * heat
if gs.OverclockTicksRemaining > 0 && gs.OverclockMultiplier > 1.0 {
    overclockHeat := int(float64(gs.HeatGenerated) * (gs.OverclockMultiplier - 1.0))
    gs.HeatGenerated += overclockHeat
}
```

**Important ordering note**: The overclock heat must be added BEFORE the `heatPenalty`
calculation (line 143-146) so that the overheat check includes overclock-generated heat.
This means the overclock heat addition goes in the RECALCULATIONS section, after
`gs.HeatGenerated = gs.PowerWatts` (line 97), not in the INCOME section. The overclock
tick decrement stays in the INCOME section (after elapsed > 0 guard) since it should only
decrement when time passes.

**activate_overclock action** (new method in engine.go):

```go
type overclockPayload struct {
    Tier int `json:"tier"`
}

func (e *Engine) activateOverclock(gs *models.GameState, payload json.RawMessage) (*ActionResult, error) {
    var p overclockPayload
    if err := json.Unmarshal(payload, &p); err != nil {
        return nil, fmt.Errorf("invalid payload")
    }

    tier := getOverclockTier(p.Tier)
    if tier == nil {
        return nil, fmt.Errorf("invalid overclock tier: %d", p.Tier)
    }

    if gs.ComputeUnits < tier.Cost {
        return nil, fmt.Errorf("not enough compute units (need %d, have %d)", tier.Cost, gs.ComputeUnits)
    }

    gs.ComputeUnits -= tier.Cost
    gs.OverclockMultiplier = tier.Multiplier
    gs.OverclockTicksRemaining = tier.Duration

    return &ActionResult{}, nil
}
```

Add to ProcessAction switch:
```go
case "activate_overclock":
    return e.activateOverclock(gs, payload)
```

**Prestige reset** (engine.go, prestige() function, after line 802):

```go
gs.OverclockMultiplier = 1.0
gs.OverclockTicksRemaining = 0
```

### 4.4 Offline Handling

When a player is offline for N seconds (e.g., 2 hours = 1440 ticks at 5s interval), the
lazy tick evaluation in `ProcessIdleProgress` fires once with a large `elapsed` value.
However, the current implementation only decrements `ThrottleTicksRemaining` by 1 per call,
not by the elapsed tick count. This means a throttle that should expire in 60 ticks persists
for 60 *requests*, not 60 *tick-intervals*.

**This is an existing bug in the throttle system.** Overclock should NOT replicate this bug.

The correct approach for overclock is:

```go
if gs.OverclockTicksRemaining > 0 {
    elapsedTicks := int(seconds / 5.0) // ticks elapsed based on real time
    if elapsedTicks < 1 {
        elapsedTicks = 1
    }
    gs.OverclockTicksRemaining -= elapsedTicks
    if gs.OverclockTicksRemaining <= 0 {
        gs.OverclockMultiplier = 1.0
        gs.OverclockTicksRemaining = 0
    }
}
```

**Design decision**: Use elapsed-time-based decrement for overclock. This ensures a 5-minute
overclock actually lasts 5 minutes of real time, not 5 minutes of server-tick-time. If the
player goes AFK for 10 minutes with 60 ticks remaining, the overclock is correctly expired
when they return. The CU earned during the first 5 minutes of the offline period should
include the overclock boost.

**Partial-period offline CU calculation**: When the player reconnects after being offline
longer than the overclock duration, the ideal calculation is:

1. Apply overclock multiplier for the first N ticks (where N = remaining ticks)
2. Apply base multiplier for the remaining ticks

However, this would require splitting the idle progress calculation into two phases, which
adds significant complexity to `ProcessIdleProgress`. The pragmatic approach is:

- If `OverclockTicksRemaining > 0` and the elapsed time exceeds the remaining overclock
  duration, calculate the fraction of time the overclock was active and apply a weighted
  average multiplier. This is approximately correct and avoids restructuring the idle
  progress loop.

```go
overclockMult := 1.0
if gs.OverclockTicksRemaining > 0 {
    overclockDurationSec := float64(gs.OverclockTicksRemaining) * 5.0
    if seconds <= overclockDurationSec {
        // Entire period was overclocked
        overclockMult = gs.OverclockMultiplier
    } else {
        // Partial: weighted average
        overclockFraction := overclockDurationSec / seconds
        overclockMult = gs.OverclockMultiplier*overclockFraction + 1.0*(1.0-overclockFraction)
    }
}
```

This weighted-average approach is a reasonable approximation. The error is bounded: in the
worst case (5x overclock, 5 min active out of 60 min offline), the player gets a ~1.33x
average instead of exact 5x-for-5min-then-1x-for-55min, which would yield the same total
CU. The weighted average is mathematically equivalent for the CU sum.

**Note**: This weighted-average calculation IS actually exact, not an approximation. If
`rate * T_overclock * mult + rate * T_base * 1.0 = rate * T_total * weighted_avg`, then
`weighted_avg = (T_overclock * mult + T_base) / T_total`, which is exactly what the
formula computes.

## 5. Data Models & Storage

### 5.1 GameState Model Changes

In `internal/models/game_state.go`, add two fields to the `GameState` struct:

```go
OverclockMultiplier    float64   `json:"overclock_multiplier"`
OverclockTicksRemaining int      `json:"overclock_ticks_remaining"`
```

Place them after the `ThrottleTicksRemaining` field (line 40) for logical grouping with the
existing timed-multiplier fields.

### 5.2 Database Migration

File: `internal/database/migrations/011_overclock_mode.sql`

```sql
-- 011_overclock_mode.sql
-- Add overclock mode fields (temporary compute boost)

ALTER TABLE game_states ADD COLUMN overclock_multiplier DECIMAL(5,2) NOT NULL DEFAULT 1.0;
ALTER TABLE game_states ADD COLUMN overclock_ticks_remaining INT NOT NULL DEFAULT 0;
```

This mirrors the exact pattern used in `004_throttle_and_fixes.sql` for throttle fields.
`DECIMAL(5,2)` supports values up to 999.99, which is more than sufficient for a 5.0x
multiplier.

### 5.3 Query Changes

In `internal/database/queries/game_state.go`:

1. Add `overclock_multiplier, overclock_ticks_remaining` to the `gsColumns` constant
   (after `throttle_ticks_remaining`).
2. Add `&gs.OverclockMultiplier, &gs.OverclockTicksRemaining` to the `gsFields` scan list
   (after `&gs.ThrottleTicksRemaining`).
3. Add `overclock_multiplier = $N, overclock_ticks_remaining = $N+1` to the UPDATE query
   (after `throttle_ticks_remaining`). Adjust all subsequent parameter indices.
4. Add `gs.OverclockMultiplier, gs.OverclockTicksRemaining` to the UPDATE exec parameter
   list.

This is the same mechanical change pattern used when `bitcoin_balance` was added.

### 5.4 Wipe Script Update

Add to `wipe_player_progress.sql`:

```sql
overclock_multiplier = 1.0,
overclock_ticks_remaining = 0,
```

## 6. API Contracts

### 6.1 Action Request

**Endpoint**: POST `/api/game/action` (or WebSocket action message, if WS actions TDD is
implemented first)

```json
{
  "type": "activate_overclock",
  "payload": {
    "tier": 2
  }
}
```

**Validation**:
- `tier` must be 1, 2, or 3. Any other value returns `400 {"error":"invalid overclock tier: N"}`.
- Player must have sufficient CU. Otherwise: `400 {"error":"not enough compute units (need N, have N)"}`.

**Success response**: Standard full game state response with updated `overclock_multiplier`,
`overclock_ticks_remaining`, and reduced `compute_units`.

### 6.2 GameState Response (new fields)

The following fields are added to every game state response (embedded in the `GameState`
struct via JSON serialization):

```json
{
  "overclock_multiplier": 3.0,
  "overclock_ticks_remaining": 45,
  "overclocked": true
}
```

The `overclocked` boolean is a derived convenience field (same pattern as `throttled` and
`overheating`), set in `buildResponse`:

```go
Overclocked: gs.OverclockTicksRemaining > 0,
```

### 6.3 GameConfig Response (new section)

Add an `overclock` section to the `GameConfig` response so the client can render tier
options without hardcoding values:

```json
{
  "overclock": {
    "tiers": [
      { "tier": 1, "multiplier": 2.0, "cost": 50000, "duration": 60, "label": "2x Boost" },
      { "tier": 2, "multiplier": 3.0, "cost": 200000, "duration": 60, "label": "3x Boost" },
      { "tier": 3, "multiplier": 5.0, "cost": 1000000, "duration": 60, "label": "5x Boost" }
    ],
    "tick_interval_seconds": 5
  }
}
```

The `tick_interval_seconds` field tells the client how to convert ticks to a human-readable
countdown (remaining_ticks * tick_interval_seconds = seconds remaining).

## 7. Frontend Changes

### 7.1 TypeScript Types (api.ts)

Add to `GameState` interface:

```typescript
overclock_multiplier: number;
overclock_ticks_remaining: number;
overclocked: boolean;
```

Add to `GameConfig` interface:

```typescript
overclock: OverclockConfig;
```

New interface:

```typescript
export interface OverclockConfig {
  tiers: OverclockTierConfig[];
  tick_interval_seconds: number;
}

export interface OverclockTierConfig {
  tier: number;
  multiplier: number;
  cost: number;
  duration: number;
  label: string;
}
```

### 7.2 useIdleTick.ts Rate Calculation

In the multiplier calculation section (around line 88), add overclock:

```typescript
const overclockMult = (state.overclocked && state.overclock_multiplier > 1)
  ? state.overclock_multiplier
  : 1.0;
const baseMultiplier = coloMult * idleMult * heatPenalty * throttle * overclockMult;
```

This exactly mirrors the server-side multiplication order. The defensive check ensures
the overclock multiplier never reduces income (same guard as the server-side code).

**Heat interaction note**: The client's `heatPenalty` is already computed from
`state.overheating` (line 81), which the server sets based on `HeatGenerated > CoolingCapacity`.
Since the server includes overclock heat in `HeatGenerated` before computing `overheating`,
the client automatically reflects the correct penalty without additional logic.

### 7.3 gameStore.ts

Add a new action method:

```typescript
activateOverclock: async (tier: number) => {
  set({ error: null });
  try {
    const state = await wsClient.sendAction('activate_overclock', { tier });
    set({ state });
  } catch (e) {
    set({ error: (e as Error).message });
  }
},
```

Add `activateOverclock` to the `GameStore` interface definition.

### 7.4 UI Component

A new `OverclockPanel` component (or section within an existing panel) should display:

1. **Tier buttons**: One per overclock tier, showing multiplier, cost, and duration. Disable
   buttons the player cannot afford.
2. **Active overclock indicator**: When overclocked, show the current multiplier and a
   countdown timer. The countdown is computed client-side:
   `remaining_seconds = overclock_ticks_remaining * tick_interval_seconds`.
3. **Boosted rate display**: Show the overclock-boosted CU/sec alongside the base rate.
4. **Heat warning**: If activating the overclock would push heat above cooling capacity,
   display a warning (computed client-side from config).

The specific UI layout is @ux-designer's responsibility. This TDD specifies the data
contract and available fields; the visual design is deferred to a UX spec.

## 8. Migration & Rollout

### 8.1 Database Migration

Apply migration 011 before deploying the new backend code:

```bash
cat /root/project/apps/backend/internal/database/migrations/011_overclock_mode.sql | sudo -u postgres psql -d homelab_game
echo "GRANT ALL ON game_states TO homelab_game;" | sudo -u postgres psql -d homelab_game
```

The migration adds columns with `NOT NULL DEFAULT` values, so it is safe to apply while the
old code is running. The old code will ignore the new columns (they are not in its SELECT
list), and the defaults (1.0 / 0) represent "no overclock active," which is correct.

**Note**: The GRANT statement is likely unnecessary since `game_states` already exists and
the `homelab_game` user already has permissions on it, but it is harmless and follows the
project convention documented in CLAUDE.md.

### 8.2 Rollback Plan

If overclock needs to be removed:

1. Deploy code without overclock (revert the backend changes).
2. The overclock columns remain in the DB but are ignored by the old code.
3. Optionally, run: `ALTER TABLE game_states DROP COLUMN overclock_multiplier, DROP COLUMN overclock_ticks_remaining;`

No data loss risk. Players who had an active overclock simply lose the remaining duration
-- the CU was already spent and the boost CU already earned. No refund mechanism is needed
for a temporary buff.

### 8.3 Deployment Order

1. Apply DB migration (011)
2. Deploy backend with engine + handler + query changes
3. Deploy frontend with type + store + interpolation + UI changes

Steps 2 and 3 can be deployed simultaneously since the backend changes are backward-
compatible (new fields have safe defaults, new action is additive).

## 9. Risks & Open Questions

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| Overclock + prestige multiplier + knowledge boost creates runaway CU inflation | Medium | High | The overclock multiplier stacks multiplicatively with existing multipliers. A player with 5x colo mult, 3x idle mult, 1.5x knowledge, and 5x overclock gets 112.5x base income. This is intentional -- it is a temporary burst, not permanent. Monitor via leaderboard score velocity. |
| Heat feedback makes Tier 3 unusable without max cooling | Low | Low | This is intentional design. Tier 3 is meant for late-game players with strong cooling. If it proves too punishing, the heat factor can be tuned down in config. |
| Offline weighted-average multiplier confuses players | Low | Low | The math is exact (see section 4.4). Players see the correct total CU earned. The only visible artifact is that the overclock appears "expired" when they return, which is correct behavior. |
| Throttle offline bug creates inconsistency | Medium | Medium | Overclock uses time-based decrement while throttle uses per-call decrement. This means overclock expires correctly offline but throttle does not. This inconsistency should be documented and the throttle bug should be filed as a follow-up fix. |

### Open Questions

1. **Should overclock affect reputation and money income, or only compute?** The current
   design applies it only to compute income (via the `totalMultiplier` chain on line 162,
   which feeds CU calculation). Reputation (line 163) and Money (line 164) use
   `heatPenalty * eventThrottle` but not the full `totalMultiplier`. The ideation document
   says "all compute income." **Recommendation**: Compute only for v1. Adding
   reputation/money boost is a one-line change each if desired later.

2. **Should there be a cooldown between overclocks?** The current design allows immediate
   re-activation (at full cost). A cooldown would prevent spamming but reduces the CU-sink
   effectiveness. **Recommendation**: No cooldown for v1. The cost IS the cooldown -- a
   player spamming Tier 3 overclocks is burning 1M CU every 5 minutes, which is the desired
   behavior for a CU sink.

3. **Should the existing throttle offline bug be fixed in the same PR?** Fixing it would
   make both timed effects consistent. However, changing throttle behavior is a separate
   concern with its own edge cases (events that apply throttle assume per-call decrement).
   **Recommendation**: File as a separate follow-up issue. Do not couple it with the
   overclock feature.

## 10. Testing Strategy

### Unit Tests (engine)

| Test Case | Input | Expected Output |
|-----------|-------|----------------|
| Activate overclock tier 1 | CU >= 50000, tier=1 | CU -= 50000, mult=2.0, ticks=60 |
| Activate overclock tier 3 | CU >= 1000000, tier=3 | CU -= 1000000, mult=5.0, ticks=60 |
| Insufficient CU | CU = 10000, tier=1 | Error "not enough compute units" |
| Invalid tier | tier=0 or tier=4 | Error "invalid overclock tier" |
| Replace active overclock | Already overclocked, activate new | Old replaced, full cost charged |
| Overclock income boost | Hardware producing 100/tick, 2x overclock | 200/tick compute |
| Overclock tick decrement | ticks=5, 1 tick elapses | ticks=4 |
| Overclock expiry | ticks=1, 1 tick elapses | ticks=0, mult=1.0 |
| Offline expiry | ticks=10, 30 ticks elapsed | ticks=0, mult=1.0 |
| Partial offline | ticks=30, 60 ticks elapsed | Weighted avg multiplier applied |
| Heat generation | 500W heat, 3x overclock | HeatGenerated = 500 + 1000 = 1500 |
| Heat triggers penalty | Overclock heat > cooling | heatPenalty = 0.5 applied |
| Prestige resets overclock | Prestige with active overclock | mult=1.0, ticks=0 |

### Integration Tests (handler)

- POST action `activate_overclock` with tier=2 returns updated state with correct fields
- State response includes `overclocked: true` when active
- State response includes `overclocked: false` when not active
- Repeated activation replaces previous overclock

### Client Tests

- `useIdleTick` rate calculation includes overclock multiplier when `overclocked` is true
- `useIdleTick` rate calculation uses 1.0 when `overclocked` is false
- `gameStore.activateOverclock()` sends correct WebSocket action

## 11. Implementation Phases

### Phase 1: Backend Core (Size: S)

**Files modified:**
- `internal/models/game_state.go` -- add 2 fields
- `internal/database/migrations/011_overclock_mode.sql` -- new file
- `internal/database/queries/game_state.go` -- add columns to SELECT, scan, UPDATE
- `internal/game/engine/engine.go` -- overclock tick decay, multiplier in income calc,
  heat generation, new `activateOverclock` action, prestige reset
- `internal/game/engine/config.go` -- `OverclockConfig` type, tier definitions, add to
  `GetConfig()`
- `internal/database/migrations/wipe_player_progress.sql` -- add overclock reset

**Dependencies:** None (can start immediately).

**Estimated effort:** 1-2 hours. All changes follow established patterns. The largest
piece is the `ProcessIdleProgress` changes (offline handling).

### Phase 2: Handler & Response (Size: S)

**Files modified:**
- `internal/api/handlers/game.go` -- add `Overclocked` field to `fullStateResponse`,
  set in `buildResponse`

**Dependencies:** Phase 1 (engine changes).

**Estimated effort:** 30 minutes. Mechanical addition to response struct.

### Phase 3: Frontend Integration (Size: S)

**Files modified:**
- `apps/desktop/src/api.ts` -- add TypeScript interfaces
- `apps/desktop/src/hooks/useIdleTick.ts` -- add overclock multiplier to rate calc
- `apps/desktop/src/stores/gameStore.ts` -- add `activateOverclock` action

**Dependencies:** Phase 2 (response includes overclock fields).

**Estimated effort:** 30-45 minutes. The `useIdleTick.ts` change is a single line in the
multiplier chain. The store method follows the existing action pattern exactly.

### Phase 4: UI Component (Size: M)

**Files created:**
- New overclock UI component or section within an existing panel

**Dependencies:** Phase 3 (store action available), UX spec from @ux-designer.

**Estimated effort:** 1-2 hours. Depends on UX design complexity. The data plumbing is
complete from Phase 3; this is pure visual work.

### Phase 5: Testing (Size: S-M)

**Files created:**
- Engine unit tests for overclock logic
- Handler integration test for the action endpoint

**Dependencies:** Phase 1-2 (code to test exists).

**Estimated effort:** 1-2 hours. @sdet responsibility.

---

**Total estimated effort:** 4-7 hours across all phases. Phases 1-3 can be completed in
a single session by @senior-engineer. Phase 4 depends on UX design. Phase 5 can be
parallelized with Phase 4.
