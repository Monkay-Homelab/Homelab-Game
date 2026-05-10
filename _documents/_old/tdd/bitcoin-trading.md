---
project: "homelab-game"
maturity: "draft"
last_updated: "2026-03-21"
updated_by: "@staff-engineer"
scope: "Bitcoin trading mechanic: buy/sell Bitcoin with Money ($), server-side price fluctuation, prestige persistence"
owner: "@staff-engineer"
dependencies:
  - ../spec/architecture.md
  - ../spec/security.md
---

# Bitcoin Trading

## 1. Problem Statement

### What

Add a Bitcoin trading mechanic to Homelab the Game. Players can buy and sell Bitcoin using
the Money ($) currency at a server-controlled price that fluctuates over time. Bitcoin
balance persists through prestige/colo, giving players a speculative cross-prestige
store of value.

### Why Now

The Money ($) currency currently has limited sinks beyond knowledge upgrades and datacenter
construction. Bitcoin trading adds a speculative/timing dimension to the economy that:

- Creates meaningful decisions around when to cash out Money vs. hold Bitcoin
- Provides a cross-prestige wealth transfer mechanism alongside Knowledge Points
- Introduces market-watching engagement between active play sessions
- Gives late-game players (post-SaaS unlock, when Money starts flowing) a new system to
  interact with

### Constraints

- **Server-authoritative**: The price MUST be computed and stored server-side. Clients
  display the price but never determine it. All buy/sell transactions are validated on the
  server.
- **Single global price**: All players see the same Bitcoin price at any given time. This
  is simpler to implement and reason about than per-player prices.
- **No real money**: This is entirely in-game. The term "Bitcoin" is used thematically to
  fit the homelab/tech aesthetic. There is no connection to real cryptocurrency.
- **Prestige persistence**: Bitcoin balance survives colo/prestige, like Knowledge Points
  and colo racks.
- **No fractional Bitcoin**: Players buy and sell whole Bitcoins only. This simplifies
  the data model (integer storage) and makes the mental math easier for players.

### Acceptance Criteria

1. A global Bitcoin price exists server-side that all players can query.
2. The price updates on a regular interval (configurable, default ~30 seconds) using a
   mean-reverting random walk algorithm.
3. Players can buy one or more whole Bitcoin if they have sufficient Money ($).
4. Players can sell one or more whole Bitcoin, receiving Money ($) at the current price.
5. The player's Bitcoin balance persists through colo/prestige (not reset).
6. The Bitcoin price and player balance are included in the game state response.
7. Price history is available for the client to render a price chart (last N data points).
8. The `buy_bitcoin` and `sell_bitcoin` actions follow the existing action validation
   pattern (server-side validation, error messages, per-user locking).
9. The frontend displays current price, player's Bitcoin holdings, and a buy/sell
   interface in a new "Market" tab or within an existing panel.
10. Anti-cheat: Players cannot manipulate the price. Buy/sell are validated against the
    current server-side price at transaction time.

---

## 2. Context & Prior Art

### Existing Codebase Patterns

**Game Engine Action Pattern** (`internal/game/engine/engine.go`):
All player actions flow through `ProcessAction()`, which dispatches to typed handler
methods. Each handler receives the `GameState`, validates preconditions, mutates the
state, and returns an `ActionResult`. The handler layer (`internal/api/handlers/game.go`)
persists changes and builds the response. Per-user mutex locking in `PerformAction()`
prevents race conditions on concurrent actions.

**Prestige Persistence Model** (`engine.go:prestige()`):
The prestige function explicitly resets most fields to defaults but preserves specific
fields. Currently preserved: `KnowledgePoints`, `ColoCount`, `ColoMultiplier`,
`DatacenterTier`, `OwnsDatacenter`, `DatacenterLevel`, `DatacenterIncomeMultiplier`,
`TotalDonatedCU`. Bitcoin balance will follow this same pattern -- listed in the struct,
not explicitly reset in the prestige function.

**Currency Precedent** (`models.GameState`):
Money is stored as `int64` on the GameState. Bitcoin balance follows the same pattern:
an `int64` field on GameState. The existing `Money` field uses whole units (no cents),
and Bitcoin should follow suit with whole coins.

**Config Distribution Pattern** (`engine/config.go`):
Game constants are exposed via `GET /api/game/config`, cached client-side, and used for
display calculations. Bitcoin config (price update interval, price range, etc.) should
be distributed through this same endpoint.

**Database Migration Pattern** (`internal/database/migrations/`):
Schema changes are sequential numbered SQL files with `ALTER TABLE` statements for
additive changes. Bitcoin requires a new column on `game_states` plus a new table for
price state and history.

### How Similar Games Handle This

- **Cookie Clicker / Idle Miner**: Stock market minigames with fluctuating prices are
  common in idle games. They typically use simple random walks or sine-wave-based price
  models. The key design insight is that the price model should be engaging but not
  exploitable -- pure random walks feel random, while mean-reverting models create
  recognizable "buy low, sell high" patterns that reward attention.
- **Adventure Capitalist**: Uses angel investors as a cross-prestige currency with a
  single accumulator. Bitcoin fills a similar role but with variable exchange rates.

### Architectural Constraints

- The backend is a single-process Go server with no background workers or job queues.
  Price updates must be triggered either by HTTP requests (lazy evaluation) or by a
  lightweight goroutine timer within the existing process.
- The 5-second client polling interval (`setInterval(fetchState, 5000)`) means price
  updates are visible to clients within ~5 seconds of occurring.
- WebSocket push (`ws.Hub.SendToUser`) is available for real-time price update
  notifications but currently only used for game events.

---

## 3. Alternatives Considered

### 3A: Lazy Price Evaluation (on request)

**How it works:** No background goroutine. When any player requests game state or performs
a buy/sell action, the server checks how much time has elapsed since the last price update
and steps the price forward accordingly. The price is stored in a shared singleton (or
database row) with a `last_updated_at` timestamp.

**Strengths:**
- Zero additional infrastructure -- no goroutines, no timers
- Consistent with existing engine pattern (ProcessIdleProgress does the same thing for
  player resources)
- Price only computed when needed -- zero CPU when no one is playing

**Weaknesses:**
- If no player polls for hours, the next request must compute many price steps at once,
  which could be slow (though bounded by a max-steps cap)
- Slight inconsistency: two players requesting state at nearly the same time could see
  different prices if one triggers the update and the other gets the pre-update price
  (mitigated by the global mutex)

### 3B: Background Goroutine Timer

**How it works:** A goroutine with a `time.Ticker` updates the price every N seconds in
the background, writing to an in-memory value protected by a mutex and periodically
persisting to the database.

**Strengths:**
- Price always up-to-date, no catch-up computation
- Can push price updates via WebSocket to all connected clients
- Clean separation of price computation from request handling

**Weaknesses:**
- Adds a persistent goroutine to the single-process server
- Price ticks even when no one is playing (negligible CPU, but still unnecessary work)
- Needs graceful shutdown handling
- Adds complexity for marginal benefit in a game polled every 5 seconds

### 3C: Hybrid (Lazy + Periodic Persist)

**How it works:** Lazy evaluation (like 3A) but with a lightweight goroutine that only
persists the latest computed price to the database every ~60 seconds for crash recovery.
Price computation itself is lazy on demand.

**Strengths:**
- Best of both: no unnecessary computation, but crash-safe
- Minimal goroutine overhead (just a periodic DB write, not price computation)

**Weaknesses:**
- Slightly more complex than pure lazy evaluation
- On crash, up to 60 seconds of price history is lost (acceptable for a game)

### Recommendation: 3A (Lazy Price Evaluation)

The lazy evaluation approach is the strongest fit for this codebase. The server already
uses exactly this pattern for idle progress (ProcessIdleProgress computes elapsed time
and applies accumulated changes). Bitcoin price updates are analogous: compute elapsed
time, step the price forward, persist.

A global mutex (or atomic pointer swap) on the price state prevents race conditions
between concurrent requests. The max-steps cap prevents pathological catch-up scenarios.

Price history is stored in the database as a TimescaleDB hypertable (the project already
uses TimescaleDB for resource_history), giving efficient time-series queries for charting.

---

## 4. Architecture & System Design

### 4.1 Price Model: Ornstein-Uhlenbeck (Mean-Reverting Random Walk)

The Bitcoin price uses a discrete Ornstein-Uhlenbeck process, which provides:
- **Mean reversion**: Price tends to drift back toward a central value, preventing
  runaway inflation or crashes to zero
- **Volatility**: Random noise creates trading opportunities
- **Bounded behavior**: Soft floor and ceiling prevent absurd prices

**Formula (per step):**

```
price_next = price_current + theta * (mu - price_current) * dt + sigma * sqrt(dt) * N(0,1)
```

Where:
- `mu` = 10,000 (long-term mean price, configurable)
- `theta` = 0.05 (mean-reversion speed -- higher = faster snap-back)
- `sigma` = 500 (volatility -- controls amplitude of fluctuations)
- `dt` = time since last step in units of step interval (normalized to 1.0 for a single
  step)
- `N(0,1)` = standard normal random variable

**Hard bounds:** Price is clamped to `[1000, 50000]` to prevent degenerate states.

**Step interval:** 30 seconds (configurable). When evaluating lazily, the number of
elapsed steps is `floor(elapsed_seconds / step_interval)`, capped at 1000 steps to
prevent long-offline catch-up from being expensive.

### 4.2 Component Architecture

```
                    +------------------+
                    |  GameHandler     |
                    |  (handlers/      |
                    |   game.go)       |
                    +--------+---------+
                             |
              +--------------+--------------+
              |                             |
   +----------v----------+      +-----------v-----------+
   |  Engine              |      |  BitcoinPriceService  |
   |  (game/engine/)      |      |  (game/bitcoin/)      |
   |  - buyBitcoin()      |      |  - GetCurrentPrice()  |
   |  - sellBitcoin()     |      |  - StepPrice()        |
   |  ProcessAction()     |      |  - GetPriceHistory()  |
   +-----+---------------+      +----------+------------+
         |                                  |
         |         +------------------------+
         |         |
   +-----v---------v------+
   |  PostgreSQL           |
   |  - game_states        |  (bitcoin_balance column)
   |  - bitcoin_price      |  (singleton row: current price + state)
   |  - bitcoin_price_     |
   |    history             |  (TimescaleDB hypertable)
   +------------------------+
```

### 4.3 New Package: `internal/game/bitcoin/`

A new package `bitcoin` under `internal/game/` encapsulates the price model. It exposes
a `PriceService` that the handler calls to get/update the current price.

```go
// internal/game/bitcoin/price.go

type PriceService struct {
    queries *queries.BitcoinQueries
    mu      sync.Mutex
    config  PriceConfig
}

type PriceConfig struct {
    Mu            int64   // Long-term mean price (default 10000)
    Theta         float64 // Mean-reversion speed (default 0.05)
    Sigma         float64 // Volatility (default 500)
    StepInterval  int     // Seconds between price steps (default 30)
    MinPrice      int64   // Hard floor (default 1000)
    MaxPrice      int64   // Hard ceiling (default 50000)
    MaxCatchupSteps int   // Max steps to compute on catch-up (default 1000)
    HistoryRetention int  // Number of price points to keep for charting (default 200)
}

// GetCurrentPrice returns the current price, advancing the model if needed.
func (s *PriceService) GetCurrentPrice(ctx context.Context, now time.Time) (int64, error)

// GetPriceHistory returns the last N price points for charting.
func (s *PriceService) GetPriceHistory(ctx context.Context, limit int) ([]PricePoint, error)
```

### 4.4 Engine Integration

Two new action types are added to `ProcessAction()`:

```go
case "buy_bitcoin":
    return e.buyBitcoin(gs, payload, currentPrice)
case "sell_bitcoin":
    return e.sellBitcoin(gs, payload, currentPrice)
```

The `currentPrice` is resolved by the handler before calling the engine, since the
price service is at the handler layer (it needs DB access). The engine receives the
already-resolved price as a parameter, keeping the engine stateless and testable.

**Key design decision:** The engine's `ProcessAction` signature must be extended to
accept the current Bitcoin price. Rather than polluting the general signature, the
handler will call `GetCurrentPrice()` before dispatching to the engine and pass it
through. A new field on a context struct or a direct parameter are both viable; the
simplest approach is to pass it to the specific buy/sell methods directly rather than
changing the ProcessAction signature.

Actually, looking at the existing pattern more carefully: `ProcessAction` already
dispatches to private methods and passes `gs` plus relevant data to each. The cleanest
approach is to have the handler resolve the price and store it on a field that the
engine can access. But since the engine is stateless, the handler should instead call
the buy/sell methods on the engine directly with the price, bypassing the switch
dispatch for these two actions. However, this breaks the existing pattern where all
actions go through `ProcessAction`.

**Revised approach:** Add `currentBitcoinPrice int64` as a parameter to `ProcessAction`.
For non-bitcoin actions, this is simply 0 and ignored. This keeps the dispatch
centralized. The handler resolves the price once per request and passes it through.

### 4.5 Handler Integration

In `PerformAction()` (handlers/game.go), after fetching game state and before calling
`ProcessAction`, the handler calls `bitcoinService.GetCurrentPrice()` to get the
current price. This price is passed to the engine and also included in the response.

The `GetState()` handler similarly calls `GetCurrentPrice()` to include the price in
every state poll response, and `GetPriceHistory()` to include chart data.

### 4.6 Frontend Integration

**State response additions:**
```typescript
// In GameState interface (api.ts):
bitcoin_balance: number;       // Player's BTC holdings (whole coins)
bitcoin_price: number;         // Current market price per BTC
bitcoin_price_history: { time: string; price: number }[];  // Chart data
```

**Config additions:**
```typescript
// In GameConfig / BitcoinConfig:
bitcoin: {
  min_price: number;
  max_price: number;
  step_interval: number;  // seconds between price updates
}
```

**New store actions:**
```typescript
buyBitcoin: (amount: number) => Promise<void>;
sellBitcoin: (amount: number) => Promise<void>;
```

**UI location:** A new "Market" tab alongside the existing tabs (Hardware, Services,
Upgrades, SaaS, Datacenter, Social). The Market tab contains:
- Current BTC price with directional indicator (up/down arrow)
- Player's BTC balance and its current value in Money ($)
- A simple price chart (last ~100 data points)
- Buy/Sell controls with amount input
- Total portfolio value display

---

## 5. Data Models & Storage

### 5.1 Database Schema Changes

**Migration: `010_bitcoin_trading.sql`**

```sql
-- Add bitcoin balance to game_states (persists through prestige)
ALTER TABLE game_states ADD COLUMN bitcoin_balance BIGINT NOT NULL DEFAULT 0;

-- Global bitcoin price state (singleton row)
CREATE TABLE bitcoin_price (
    id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),  -- Enforces singleton
    current_price BIGINT NOT NULL DEFAULT 10000,
    last_step_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Insert initial price
INSERT INTO bitcoin_price (current_price) VALUES (10000);

-- Price history for charting (TimescaleDB hypertable)
CREATE TABLE bitcoin_price_history (
    time TIMESTAMPTZ NOT NULL,
    price BIGINT NOT NULL
);

SELECT create_hypertable('bitcoin_price_history', 'time');

CREATE INDEX idx_bitcoin_price_history_time ON bitcoin_price_history (time DESC);
```

**Why a singleton table for price state?** The price is global (shared across all
players). A singleton row with a CHECK constraint (`id = 1`) prevents accidental
duplicate rows. The `last_step_at` field enables lazy evaluation: on each read,
compute how many steps have elapsed since `last_step_at` and advance the model.

**Why TimescaleDB for price history?** The project already uses TimescaleDB for
`resource_history` and `event_log`. Price history is a natural time-series use case.
TimescaleDB's automatic chunk management and time-based retention policies will keep
the table from growing unboundedly.

### 5.2 GameState Model Changes

```go
// In models/game_state.go, add to GameState struct:
BitcoinBalance int64 `json:"bitcoin_balance"`
```

This field is:
- Included in `gsColumns` for SELECT queries
- Included in `gsFields` for scanning
- Included in the UPDATE query
- **NOT reset** in the `prestige()` function (persistence through colo)

### 5.3 New Model: BitcoinPrice

```go
// In models/game_state.go or a new models/bitcoin.go:
type BitcoinPrice struct {
    CurrentPrice int64     `json:"current_price"`
    LastStepAt   time.Time `json:"last_step_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}

type BitcoinPricePoint struct {
    Time  time.Time `json:"time"`
    Price int64     `json:"price"`
}
```

### 5.4 Data Lifecycle

- **Price state:** Updated lazily on every read (GetState, PerformAction). Persisted
  immediately on update. The singleton row is always current.
- **Price history:** A new row is inserted into `bitcoin_price_history` each time the
  price is stepped forward. With 30-second intervals, this is ~2,880 rows/day.
  A TimescaleDB retention policy should drop data older than 7 days (~20K rows max).
- **Bitcoin balance:** Stored on `game_states`, updated atomically with other state
  changes in the existing `UPDATE game_states SET ...` query. Survives prestige.

---

## 6. API Contracts

### 6.1 State Response Changes

The existing `GET /api/game/state` and `POST /api/game/action` responses gain three
new fields in the `fullStateResponse`:

```json
{
  "bitcoin_balance": 5,
  "bitcoin_price": 12500,
  "bitcoin_price_history": [
    { "time": "2026-03-21T10:00:00Z", "price": 10200 },
    { "time": "2026-03-21T10:00:30Z", "price": 10350 },
    ...
  ],
  ... (existing fields)
}
```

**`bitcoin_price_history`** contains the most recent 100 price points (configurable).
Included in every state response. With 100 points at 30-second intervals, this covers
~50 minutes of price history -- enough for a meaningful chart without excessive payload
size (~3KB).

### 6.2 Buy Bitcoin Action

```json
POST /api/game/action
{
  "type": "buy_bitcoin",
  "payload": { "amount": 3 }
}
```

**Validation:**
- `amount` must be a positive integer (>= 1)
- `gs.Money >= amount * currentPrice` (sufficient funds)

**Effects:**
- `gs.Money -= amount * currentPrice`
- `gs.BitcoinBalance += amount`

**Error responses:**
- `400 {"error": "amount must be positive"}` -- non-positive amount
- `400 {"error": "not enough money (need $30000, have $15000)"}` -- insufficient funds

### 6.3 Sell Bitcoin Action

```json
POST /api/game/action
{
  "type": "sell_bitcoin",
  "payload": { "amount": 2 }
}
```

**Validation:**
- `amount` must be a positive integer (>= 1)
- `gs.BitcoinBalance >= amount` (sufficient BTC)

**Effects:**
- `gs.BitcoinBalance -= amount`
- `gs.Money += amount * currentPrice`

**Error responses:**
- `400 {"error": "amount must be positive"}` -- non-positive amount
- `400 {"error": "not enough bitcoin (need 2, have 1)"}` -- insufficient BTC

### 6.4 Config Endpoint Changes

`GET /api/game/config` response gains a new `bitcoin` section:

```json
{
  "bitcoin": {
    "min_price": 1000,
    "max_price": 50000,
    "step_interval": 30,
    "mean_price": 10000
  },
  ... (existing config)
}
```

---

## 7. Migration & Rollout

### 7.1 Migration Steps

1. **Apply migration 010_bitcoin_trading.sql** -- adds `bitcoin_balance` column,
   creates `bitcoin_price` and `bitcoin_price_history` tables.
2. **Deploy backend** with new Bitcoin trading code. Existing players get
   `bitcoin_balance = 0` by default. The global price starts at 10,000.
3. **Deploy frontend** with the Market tab / Bitcoin UI.

### 7.2 Backward Compatibility

- The new `bitcoin_balance` column defaults to 0, so existing game state rows are
  valid without data migration.
- The new fields in the state response (`bitcoin_balance`, `bitcoin_price`,
  `bitcoin_price_history`) are additive -- old clients that don't read these fields
  will continue to function.
- No existing fields are modified or removed.
- The prestige wipe migration (`007_wipe_player_progress.APPLIED`) should be updated
  to include `bitcoin_balance = 0` in its reset query if a future wipe is needed.

### 7.3 Rollback Plan

- **Backend rollback:** Remove the `buy_bitcoin`/`sell_bitcoin` action handlers. The
  `bitcoin_balance` column remains harmless (default 0, not displayed).
- **Database rollback:** `ALTER TABLE game_states DROP COLUMN bitcoin_balance;
  DROP TABLE bitcoin_price_history; DROP TABLE bitcoin_price;`
- **No data loss risk:** Bitcoin balance is a new currency with no dependencies on
  existing systems. Dropping it does not affect existing game state.

---

## 8. Risks & Open Questions

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| **Price model produces degenerate values** (e.g., stuck at floor/ceiling) | Low | Medium | Hard clamp + mean-reversion parameters. Monitor price distribution in first week. Tuning knobs are all in PriceConfig. |
| **Money inflation via Bitcoin arbitrage** | Medium | Medium | Mean-reverting model prevents systematic exploitation. No external oracle means no information asymmetry. Price is the same for all players. |
| **Payload size increase from price history** | Low | Low | Limit to 100 points (~3KB JSON). Can further reduce if needed. Consider a separate endpoint if history grows. |
| **Race condition on global price update** | Medium | Low | Global mutex on PriceService. Two concurrent requests may both try to step the price; the mutex serializes them. The second request sees the already-stepped price. |
| **Players hoarding Bitcoin as infinite cross-prestige wealth** | Medium | Low | This is intentionally part of the design -- Bitcoin is meant to be a cross-prestige store of value. If it proves too powerful, the mean price or volatility can be adjusted, or a transaction fee (spread) can be introduced. |

### Open Questions

1. **Should there be a transaction fee / spread?** A small percentage fee on buy/sell
   (e.g., 2%) would act as a Money sink and prevent trivial arbitrage. This is easy to
   add later but worth considering from the start. **Recommendation:** Start without a
   fee for simplicity. Add one if testing shows Bitcoin trading is too profitable.

2. **Should Bitcoin be visible/tradeable before SaaS unlock?** Money ($) is only earned
   from SaaS services and late-game sources. Players without Money cannot trade Bitcoin.
   The UI could show the price chart as a teaser before SaaS unlock, or hide the Market
   tab entirely until Money > 0. **Recommendation:** Show the Market tab once Money > 0
   (dynamically, not tier-gated). This naturally gates it behind SaaS without adding a
   hard tier check.

3. **Should events affect Bitcoin?** A "crypto crash" event could temporarily halve the
   price, or a "Bitcoin surge" event could double it. This adds flavor but couples the
   price model to the event system. **Recommendation:** Defer to a follow-up. The base
   price model should be stable before adding event-driven perturbations.

4. **Leaderboard integration?** Should there be a "Bitcoin Holdings" or "Portfolio Value"
   leaderboard category? **Recommendation:** Defer. The leaderboard system already has
   6 categories. Add Bitcoin leaderboards only if player demand exists.

### Flagged Assumptions

- **Assumption:** Whole Bitcoin only (no fractional). This simplifies implementation
  but may feel limiting if the price is high (e.g., at $50,000, buying 1 BTC requires
  $50K in Money). **Revisit checkpoint:** After initial playtesting, assess whether
  fractional Bitcoin (stored as "satoshis" -- millionths, similar to how Money works)
  is needed.

- **Assumption:** Single global price, not per-player. This is simpler but means all
  players face the same market. **Revisit checkpoint:** If the game adds competitive
  elements (PvP market manipulation), per-player or per-group markets may be needed.

---

## 9. Testing Strategy

### Unit Tests

- **Price model**: Verify Ornstein-Uhlenbeck step function produces values within
  bounds. Test with fixed random seed for determinism. Test catch-up computation
  (many steps at once). Test clamping at min/max bounds.
- **Buy/sell validation**: Test insufficient funds, insufficient BTC, zero/negative
  amounts, boundary conditions (exact balance).
- **Prestige persistence**: Test that `prestige()` does NOT reset `BitcoinBalance`.

### Integration Tests

- **End-to-end buy/sell flow**: Create game state, add Money, buy Bitcoin, verify
  balance changes, sell Bitcoin, verify Money returned.
- **Concurrent transactions**: Two simultaneous buy requests for the same user should
  not cause double-spending (already handled by per-user mutex).
- **Price advancement**: Simulate time passing, verify price has changed, verify
  history is recorded.

### Manual Testing / Playtesting

- **Economy balance**: Play through SaaS unlock, accumulate Money, trade Bitcoin
  across multiple prestiges. Assess whether Bitcoin is too powerful as a cross-prestige
  wealth store.
- **Price chart UX**: Verify the price chart is readable and the buy/sell controls
  are intuitive.
- **Edge cases**: What happens when a player has exactly enough Money for 1 BTC?
  What if the price changes between viewing it and clicking "Buy"? (Answer: the server
  validates against the current price at transaction time, which may differ from what
  the client displayed. This is acceptable and thematically appropriate -- "the market
  moved.")

---

## 10. Observability & Operational Readiness

### Key Metrics

- **Price distribution**: Log the current price on each step. Alert if price stays
  at min or max bound for >10 consecutive steps (indicates model degeneration).
- **Trade volume**: Count buy/sell actions per minute. Useful for economy balancing.
- **Bitcoin supply**: Total `bitcoin_balance` across all players. If this grows
  monotonically without sells, the economy may have a problem.

### Debugging

- The `bitcoin_price` singleton table shows the current price and when it was last
  stepped -- this is the first thing to check if players report stale prices.
- The `bitcoin_price_history` hypertable provides full price history for
  investigating "I bought at the wrong price" complaints.
- Game state `bitcoin_balance` is included in the standard state response, visible
  in any state dump.

### Alerts

- **Price stuck at boundary**: If `current_price = min_price` or `current_price =
  max_price` for more than 5 minutes, log a warning. This suggests the model
  parameters need tuning.
- No page-level alerts needed -- Bitcoin trading is a non-critical game feature.
  Degradation means trading is temporarily unavailable, not a game-breaking outage.

---

## 11. Implementation Phases

### Phase 1: Backend Core (Size: M)

**Scope:** Price model, database schema, engine actions, API integration.

**Deliverables:**
1. Database migration `010_bitcoin_trading.sql`
2. New package `internal/game/bitcoin/` with `PriceService` and Ornstein-Uhlenbeck
   step function
3. New query package `internal/database/queries/bitcoin.go` for price state CRUD
   and price history
4. `BitcoinBalance` field added to `GameState` model
5. `gsColumns`, `gsFields`, and `Update` query updated in `game_state.go`
6. `buy_bitcoin` and `sell_bitcoin` actions added to `ProcessAction` dispatch in
   `engine.go`
7. `BitcoinPrice` field added to `PriceService` return type
8. `ProcessAction` updated to accept `currentBitcoinPrice int64` parameter
9. `GameHandler` updated to instantiate `PriceService` and call `GetCurrentPrice()`
   in `GetState()` and `PerformAction()`
10. `fullStateResponse` updated with `bitcoin_balance`, `bitcoin_price`, and
    `bitcoin_price_history`
11. `GetConfig()` updated with Bitcoin config section
12. `prestige()` verified to NOT reset `BitcoinBalance`
13. Server `main.go` updated to wire up `BitcoinQueries` and `PriceService`

**Dependencies:** None (fully independent of other work).

### Phase 2: Frontend (Size: M)

**Scope:** Market tab UI, store actions, price chart.

**Deliverables:**
1. `BitcoinConfig` type added to `api.ts` (GameConfig extension)
2. `bitcoin_balance`, `bitcoin_price`, `bitcoin_price_history` added to `GameState`
   type in `api.ts`
3. `buyBitcoin` and `sellBitcoin` store actions added to `gameStore.ts`
4. New component `MarketPanel.tsx` with:
   - Current price display with up/down indicator
   - Player's BTC balance and value in Money
   - Buy/Sell controls (amount input + buttons)
   - Simple price chart (canvas or lightweight charting lib)
5. "Market" tab added to `App.tsx` tab bar (shown when `state.money > 0` or
   `state.bitcoin_balance > 0`)
6. Bitcoin balance displayed in `CurrencyBar.tsx` (when > 0)
7. Shared types updated in `packages/shared/src/types/game.ts` if applicable

**Dependencies:** Phase 1 (backend API must be deployed first).

### Phase 3: Polish & Tuning (Size: S)

**Scope:** Economy balancing, price model tuning, optional enhancements.

**Deliverables:**
1. Playtest and tune price model parameters (mu, theta, sigma)
2. Add price history retention policy (TimescaleDB `add_retention_policy`)
3. Consider transaction fee if economy testing reveals need
4. Consider WebSocket push for real-time price updates (optional, low priority)
5. Consider "crypto crash" / "Bitcoin surge" events (follow-up TDD if pursued)

**Dependencies:** Phase 2 (needs frontend to playtest).

---

## Appendix: Price Model Visualization

For reference, with the default parameters (mu=10000, theta=0.05, sigma=500, dt=1):

- The price tends to hover around $10,000
- Typical fluctuation range: ~$5,000 to ~$15,000
- Occasional excursions to ~$3,000 or ~$20,000
- Hard bounds at $1,000 and $50,000 are rarely hit
- A player watching for 10 minutes (~20 price updates) will see meaningful movement

This creates a market that feels alive and rewards attention without being so volatile
that trading feels random or so stable that there is no reason to trade.
