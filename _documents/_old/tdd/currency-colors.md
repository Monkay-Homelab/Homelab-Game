---
project: homelab-the-game
maturity: draft
last_updated: 2026-04-04
updated_by: "@staff-engineer"
scope: Implement consistent currency color-coding across all desktop UI surfaces
owner: "@staff-engineer"
dependencies:
  - _documents/ux/currency-colors.md
---

# Currency Color System — Technical Design Document

## 1. Problem Statement

The game has six currencies (CU, Money, Reputation, Knowledge Points, Bitcoin, Power) that appear across 15 components. Today, colors are assigned ad-hoc per component with no centralized system, causing three specific problems:

1. **Color collisions**: CU and BTC both use `--accent-amber` (`#f59e0b`). A player cannot distinguish CU from BTC values by color alone in the CurrencyBar (lines 20 and 24 of `CurrencyBar.tsx`).
2. **Cross-component inconsistency**: The CU/tick stat in HardwarePanel's summary grid uses `#a855f7` (purple) while CurrencyBar uses `--accent-amber` for CU values. Power uses `--accent-purple` in CurrencyBar but `#f59e0b` (amber) in HardwarePanel's stats.
3. **Semantic color misuse**: All cost buttons use their panel's **category** color (hardware category, upgrade type, research branch) rather than the **currency** being spent. A CU cost button appears purple in HardwarePanel (Compute category), orange in UpgradePanel (Networking type), green in TierProgress, and cyan in DatacenterPanel.

**Why now**: The game is live at `game.homelab.living`. As more currencies appear on more surfaces (SaaS revenue, BTC market, research costs), the inconsistency compounds. The UX designer has produced a comprehensive color spec (`_documents/ux/currency-colors.md`) that defines the target state. This TDD translates that spec into an implementation plan.

**Acceptance criteria (overall)**:
- Every instance of a currency value uses the same color for that currency across all components
- No two currencies share the same color (minimum 30 degrees hue separation, per UX spec Section 2)
- All colors pass WCAG 2.1 AA contrast on the three background tiers (verified via UX spec Section 2 contrast table)
- The currency color system coexists with existing category/type/branch colors (per UX spec Section 9.3)
- Existing `--accent-*` CSS variables are NOT removed; `--currency-*` variables are additive (per UX spec Section 9.4)

---

## 2. Context & Prior Art

### Current Architecture

Colors are managed through three mechanisms:

1. **CSS custom properties in `global.css`**: Six `--accent-*` variables (green, amber, cyan, red, purple, blue) plus two `--glow-*` variables. These serve multiple semantic purposes (category grouping, currency identification, state indication) with no distinction.

2. **Component-local color config objects**: `HardwarePanel.tsx` defines `CATEGORY_COLORS` (line 28), `UpgradePanel.tsx` defines `typeConfig` (line 5), `ResearchPanel.tsx` defines `branchConfig` (line 14). These inline objects use raw hex values, duplicating colors across files.

3. **Inline style props**: Most currency values are colored via `style={{ color: 'var(--accent-amber)' }}` or raw hex values directly in JSX. There is no shared utility or component for currency display.

### Pattern in the Codebase

The `Stat` component in `CurrencyBar.tsx` (line 59) is the only reusable currency display primitive. It accepts a `color` string prop. All other components render currency values directly in JSX with inline styles.

The `formatNumber()` utility is duplicated in four files (`CurrencyBar.tsx`, `DatacenterPanel.tsx`, `OverclockPanel.tsx`, `DonatePanel.tsx`) with slightly different thresholds. `MarketPanel.tsx` has its own `formatCurrency()` for dollar formatting. `ResearchPanel.tsx` has the most complete `formatNumber()` (handles T/B/M/K).

### How Solved Elsewhere

Design systems (Material UI, Radix Themes, Shopify Polaris) solve this with semantic CSS custom properties as a single source of truth, mapped through a typed constant in the component layer. The UX spec follows this pattern: CSS variables for runtime theming, a TypeScript utility for type-safe component access.

---

## 3. Alternatives Considered

### Option A: CSS Variables + TypeScript Utility Map (Recommended)

Add `--currency-*` CSS custom properties to `global.css`. Create a `currencyColors.ts` utility that maps currency IDs to their CSS variable names. Components reference the utility for type safety, and the CSS variables remain the runtime source of truth.

**Strengths**: Single source of truth (CSS variables), type-safe component access, supports future theming (override variables in a `.theme-light` scope), aligns with UX spec recommendation, minimal runtime overhead.

**Weaknesses**: Two places to update when adding a currency (CSS + TypeScript map). Mitigated by the fact that new currencies are a rare, high-touch change.

### Option B: TypeScript-Only Constants (No New CSS Variables)

Define all colors in a TypeScript constant and apply them via inline styles. No CSS variable changes.

**Strengths**: Single source of truth in TypeScript. Easier to grep for usage.

**Weaknesses**: Cannot be overridden via CSS (breaks future theming). Cannot be used in CSS-only contexts (`:root` fallbacks, `@media` queries). Diverges from the existing CSS variable pattern already used for `--accent-*`. UX spec explicitly defines CSS variable names and recommends this approach.

### Option C: Tailwind Extend Configuration

Add currency colors to the Tailwind config (`tailwind.config.js`) as custom color tokens, generating utility classes like `text-currency-cu`.

**Strengths**: Full Tailwind integration. Classes instead of inline styles.

**Weaknesses**: The codebase uses inline `style` props for all dynamic coloring, not Tailwind utility classes. Switching to classes would require rewriting every color application pattern. The tinted backgrounds (`rgba(...)` at specific opacities) don't map cleanly to Tailwind's opacity modifier syntax. High migration cost for no runtime benefit.

### Recommendation

**Option A**. It aligns with the existing CSS variable pattern, the UX spec's explicit recommendations, and supports the future theming path (light theme, per UX spec Section 7.5) with minimal overhead.

---

## 4. Architecture & System Design

### 4.1 Layer Architecture

```
global.css (:root)          <-- single source of truth for color values
    |
currencyColors.ts           <-- typed map: currency ID -> CSS variable names
    |
<CurrencyValue>             <-- shared component: renders a colored value
<CurrencyStatLine>          <-- shared component: renders multi-currency lines
    |
Panel components            <-- consume shared components or use utility directly
```

### 4.2 CSS Custom Properties (global.css)

Add the currency color block defined in UX spec Section 3 to the `:root` selector in `apps/desktop/src/styles/global.css`, immediately after the existing `--accent-*` / `--glow-*` variables (after line 22). This adds 25 new custom properties (4 per currency + 1 `--currency-rep-on-card` variant).

The existing `--accent-*` variables remain untouched. They continue to serve non-currency purposes (success states, error states, category grouping). Per UX spec Section 9.4, the `--currency-*` variables are additive and semantically distinct even when they share the same hex value.

### 4.3 TypeScript Utility: `currencyColors.ts`

New file: `apps/desktop/src/utils/currencyColors.ts`

Purpose: Provide a type-safe mapping from currency identifiers to their CSS variable references. This eliminates raw hex values and `--accent-*` references for currency display across all components.

**Type definitions:**

```typescript
type CurrencyId = 'cu' | 'money' | 'rep' | 'kp' | 'btc' | 'pwr';

type CurrencyColorSet = {
  /** CSS var() reference for text color, e.g., 'var(--currency-cu)' */
  color: string;
  /** CSS var() reference for background tint */
  bg: string;
  /** CSS var() reference for border */
  border: string;
  /** CSS box-shadow value for glow */
  glow: string;
};
```

**Constant:**

```typescript
const CURRENCY_COLORS: Record<CurrencyId, CurrencyColorSet> = {
  cu:    { color: 'var(--currency-cu)',    bg: 'var(--currency-cu-bg)',    border: 'var(--currency-cu-border)',    glow: 'var(--currency-cu-glow)' },
  money: { color: 'var(--currency-money)', bg: 'var(--currency-money-bg)', border: 'var(--currency-money-border)', glow: 'var(--currency-money-glow)' },
  rep:   { color: 'var(--currency-rep)',   bg: 'var(--currency-rep-bg)',   border: 'var(--currency-rep-border)',   glow: 'var(--currency-rep-glow)' },
  kp:    { color: 'var(--currency-kp)',    bg: 'var(--currency-kp-bg)',    border: 'var(--currency-kp-border)',    glow: 'var(--currency-kp-glow)' },
  btc:   { color: 'var(--currency-btc)',   bg: 'var(--currency-btc-bg)',   border: 'var(--currency-btc-border)',   glow: 'var(--currency-btc-glow)' },
  pwr:   { color: 'var(--currency-pwr)',   bg: 'var(--currency-pwr-bg)',   border: 'var(--currency-pwr-border)',   glow: 'var(--currency-pwr-glow)' },
};
```

**Helper functions** (exported):

- `getCurrencyColor(id: CurrencyId): CurrencyColorSet` -- lookup with the type constraint
- `getCostCurrencyId(costType: string): CurrencyId` -- maps `cost_type` field values from the API (`'money'`, `'cu'`, etc.) to CurrencyId. Defaults to `'cu'` for unknown values, matching the backend's default behavior.

**Design decision -- CSS `var()` references vs raw hex values**: The utility stores `var(--currency-cu)` strings, not `#f59e0b` hex values. This means:
- Theming works (override the CSS variable, all components update automatically)
- Components apply colors via `style={{ color: CURRENCY_COLORS.cu.color }}`
- The CSS variables remain the single source of truth

### 4.4 Shared Components

#### `<CurrencyValue>`

New file: `apps/desktop/src/components/shared/CurrencyValue.tsx`

Props:
```typescript
interface CurrencyValueProps {
  currency: CurrencyId;
  value: number | string;    // Pre-formatted string or raw number
  prefix?: string;           // e.g., '+', '-', '$'
  suffix?: string;           // e.g., '/s', '/tick', 'W'
  showLabel?: boolean;       // Show currency label (e.g., 'CU', 'REP')
  format?: boolean;          // Auto-format numbers (default: true)
  rateStyle?: boolean;       // Apply 0.7 opacity for rate display
  className?: string;
}
```

Renders a `<span>` with the currency's color applied. When `showLabel` is true, the label uses `--text-muted` and the value uses the currency color, matching the pattern in UX spec Section 4.1. When `format` is true, applies the `formatNumber()` utility.

This component consolidates the duplicated `formatNumber()` implementations. Import the most complete version (from `ResearchPanel.tsx` which handles T/B/M/K) into the utility file.

#### `<CurrencyStatLine>`

New file: `apps/desktop/src/components/shared/CurrencyStatLine.tsx`

Props:
```typescript
interface StatLineItem {
  currency: CurrencyId;
  value: number | string;
  prefix?: string;           // '+', '-'
  suffix?: string;           // '/tick', 'W', '/s'
}

interface CurrencyStatLineProps {
  items: StatLineItem[];
  separator?: string;        // Default: ' \u00B7 ' (middle dot with spaces)
  className?: string;
}
```

Renders each item with its currency color, separated by `--text-muted` colored separators. This replaces the pervasive pattern of `"+{cu} CU \u00B7 +{rep} Rep \u00B7 {power}W"` strings that currently use a single `--text-secondary` color.

Example current code (ServicePanel.tsx line 32):
```tsx
// Before
<div style={{ color: 'var(--text-secondary)' }}>
  +{s.compute_per_tick} CU · +{s.reputation_per_tick} Rep · {s.power_required}W
</div>

// After
<CurrencyStatLine items={[
  { currency: 'cu', value: s.compute_per_tick, prefix: '+', suffix: ' CU' },
  { currency: 'rep', value: s.reputation_per_tick, prefix: '+', suffix: ' Rep' },
  { currency: 'pwr', value: s.power_required, suffix: 'W' },
]} />
```

#### `<CurrencyCostButton>`

This is NOT a new component. Cost buttons have enough variation in layout (some show multiple costs, some have labels, some have "Buy All" behavior) that a shared button component would either be too simple (just color) or too complex (kitchen-sink props). Instead, components apply currency colors to their existing buttons by importing `CURRENCY_COLORS` and using the cost currency's color set for the button's `style` prop.

Pattern for cost buttons:
```typescript
const costColors = CURRENCY_COLORS[getCostCurrencyId(item.cost_type)];
// Then in JSX:
style={{
  background: canAfford ? costColors.bg : 'var(--bg-card)',
  color: canAfford ? costColors.color : 'var(--text-muted)',
  border: `1px solid ${canAfford ? costColors.border : 'var(--border)'}`,
}}
```

### 4.5 Component Inventory — What Changes Where

The full component-by-component guide is in UX spec Section 5. This section summarizes the **technical changes** per file, categorized by change type.

#### Type 1: Variable rename only (same hex, new semantic variable)

These are low-risk, mechanical replacements:

| File | Line(s) | Current | Target |
|---|---|---|---|
| `CurrencyBar.tsx` | 20 | `var(--accent-amber)` for CU | `CURRENCY_COLORS.cu.color` |
| `CurrencyBar.tsx` | 21 | `var(--accent-blue)` for REP | `CURRENCY_COLORS.rep.color` |
| `CurrencyBar.tsx` | 23 | `var(--accent-green)` for USD | `CURRENCY_COLORS.money.color` |
| `OverclockPanel.tsx` | 58, 67-69, 109, 124-126 | `var(--accent-amber)` / inline amber rgba | `CURRENCY_COLORS.cu` set |
| `DonatePanel.tsx` | 28, 45-47 | `var(--accent-amber)` / inline amber rgba | `CURRENCY_COLORS.cu` set |

#### Type 2: Actual color changes (different hex value)

These are the visible, breaking changes:

| File | Line(s) | Current Color | New Color | Currency |
|---|---|---|---|---|
| `CurrencyBar.tsx` | 22 | `--accent-purple` (#a855f7) | `--currency-pwr` (#facc15 yellow) | Power |
| `CurrencyBar.tsx` | 24 | `--accent-amber` (#f59e0b) | `--currency-btc` (#fb923c orange) | Bitcoin |
| `HardwarePanel.tsx` | 168 | `#a855f7` (purple) | `--currency-cu` (#f59e0b amber) | CU/tick stat |
| `HardwarePanel.tsx` | 175 | `#f59e0b` (amber) | `--currency-pwr` (#facc15 yellow) | Power stat |
| `HardwarePanel.tsx` | 179-182 | `#22c55e` (all green) | Split: CU bonus in `--currency-cu`, Rep bonus in `--currency-rep` | Bonus split |
| `MarketPanel.tsx` | 202, 212, 228-229, 251 | `--accent-amber` | `--currency-btc` | BTC values |
| `MarketPanel.tsx` | 230 | `--accent-amber` | `--currency-money` | BTC dollar value |
| `SocialPanel.tsx` | 229 | `#f59e0b` (all amber) | Dynamic per leaderboard category | Scores |
| `TierProgress.tsx` | 45-48 | `--accent-green` palette | `--currency-cu` palette | Tier upgrade cost |

#### Type 3: Cost button color source change (category -> currency)

These change which color set is used for cost buttons:

| File | Current Source | New Source | Currency |
|---|---|---|---|
| `HardwarePanel.tsx` (buy buttons) | `CATEGORY_COLORS[cat]` | `CURRENCY_COLORS.cu` | CU |
| `HardwarePanel.tsx` (component upgrade buttons) | `CATEGORY_COLORS[cat]` | `CURRENCY_COLORS.cu` | CU |
| `UpgradePanel.tsx` (buy buttons) | `typeConfig[type]` | `CURRENCY_COLORS[getCostCurrencyId(u.cost_type)]` | CU or Money |
| `ServicePanel.tsx` (deploy buttons) | `--accent-blue` inline | `CURRENCY_COLORS.cu` | CU |
| `ResearchPanel.tsx` (research buttons) | `branchConfig[branch]` | `CURRENCY_COLORS.cu` | CU |
| `SaasPanel.tsx` (deploy buttons) | `--accent-amber` inline | `CURRENCY_COLORS.cu` | CU |
| `DatacenterPanel.tsx` (optimize button) | `--accent-cyan` inline | `CURRENCY_COLORS.cu` | CU |

**Important**: Category/type/branch headings and effect text RETAIN their existing colors. Only cost buttons and cost values change. The `CATEGORY_COLORS`, `typeConfig`, and `branchConfig` objects remain in their files; they are still used for headings and category labels.

#### Type 4: Multi-currency stat line decomposition

These change a single-colored string into multiple colored segments:

| File | Line(s) | Pattern |
|---|---|---|
| `HardwarePanel.tsx` | 87-89, 222-225 | `"{power}W \u00B7 +{compute}/tick \u00B7 {units}U"` |
| `ServicePanel.tsx` | 32, 66-68 | `"+{cu} CU \u00B7 +{rep} Rep \u00B7 {power}W"` |
| `DatacenterPanel.tsx` | 104-105, 152, 161 | `"+{cu} CU \u00B7 +{rep} Rep \u00B7 +${money}/tick"` |
| `SaasPanel.tsx` | 87 | `"${revenue}/cust \u00B7 {power}W"` |
| `SaasPanel.tsx` | 126 | `"{type} \u00B7 ${revenue}/tick"` |
| `MarketPanel.tsx` | 283, 347, 286, 350 | `"Cost: ${amount}"` / `"CU Cost: {amount}"` |

#### Type 5: No change needed

These files either don't display currency values or already use appropriate colors:

| File | Reason |
|---|---|
| `EventLog.tsx` | Currency values are in plain-text event descriptions from server; no structured currency data to color |
| `ClickArea.tsx` | "Run Job" button is an action, not a currency display |
| `Login.tsx` | No currency display |
| `App.tsx` | Layout shell; no currency display |

---

## 5. Data Models & Storage

No data model or storage changes. This is a purely frontend presentation concern. Currency values are already typed in `GameState` (`api.ts`) with individual numeric fields (`compute_units`, `reputation`, `money`, `knowledge_points`, `bitcoin_balance`, `power_watts`).

The `CurrencyId` type in `currencyColors.ts` maps to these fields but does not replace them. The mapping is:

| CurrencyId | GameState Field(s) |
|---|---|
| `cu` | `compute_units`, `compute_per_tick` |
| `money` | `money`, `money_per_tick`, `monthly_revenue` |
| `rep` | `reputation`, `reputation_per_tick` |
| `kp` | `knowledge_points` |
| `btc` | `bitcoin_balance`, `bitcoin_price` |
| `pwr` | `power_watts`, `power_limit`, `power_draw`, `power_required` |

---

## 6. Migration & Rollout

### Rollout Strategy

This is an additive change with no backend impact. Deployment is a frontend rebuild.

Since there is no staging environment (per CLAUDE.md: "Everything runs on this single VM"), the rollout is:

1. Build and test locally via `pnpm dev` (Vite dev server on port 3000)
2. Visual QA across all panels
3. Build production (`pnpm build`) and deploy

### Breaking Visual Changes

Two color changes will be immediately noticeable to existing players:
- **Power**: purple -> yellow in CurrencyBar
- **Bitcoin**: amber -> orange in CurrencyBar and MarketPanel

These are intentional corrections documented in UX spec Section 5.1. There is no data-level breaking change, so no migration is needed. If player feedback on the Power color change is negative, the CSS variable `--currency-pwr` can be changed back to `#a855f7` in one line.

### Rollback Plan

All changes are in the frontend. Rollback is `git revert` of the relevant commits and a rebuild. CSS variable values can also be hot-patched individually without reverting component changes.

---

## 7. Risks & Open Questions

### Risks

| # | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| 1 | **Warm-hue confusion**: CU (amber #f59e0b), BTC (orange #fb923c), and PWR (yellow #facc15) are perceptually close at 11/10/21 degrees hue separation | Medium | Medium | UX spec Section 2 documents this and provides mitigations: lightness differentiation, context separation (BTC only in Market), labels always present. Per-currency icons are a future fallback (UX spec Section 2 Disambiguation). |
| 2 | **REP contrast on card backgrounds**: `#3b82f6` at 3.8:1 on `--bg-card` narrowly misses WCAG AA for small normal text | Low | Low | UX spec defines `--currency-rep-on-card: #60a5fa` for card contexts. The `stat-value` class already uses `font-weight: 500` which qualifies as large text at our font sizes. The implementer should use the `-on-card` variant when REP text appears on `--bg-card` backgrounds. |
| 3 | **Category color confusion on cost buttons**: Players may expect Compute hardware buttons to be purple (category color) but they will now be amber (CU currency color) | Low | Low | UX spec Section 9.3 explains the coexistence rule. Category headings retain their colors, providing visual grouping. The button color change reinforces "this costs CU" which is the actionable information. |
| 4 | **HardwarePanel bonus stat split complexity**: The bonus cell currently shows "+X% CU . +Y% Rep" as a single green string. Splitting requires conditional rendering for cases where only CU or only Rep bonus exists. | Low | Low | Straightforward conditional logic. The existing code already conditionally renders the separator (line 181). |
| 5 | **Duplicated `formatNumber()` divergence**: Four copies of `formatNumber()` exist with different thresholds. Consolidating into one shared implementation could change displayed values in some components. | Low | Medium | Use the `ResearchPanel.tsx` version (handles T/B/M/K) as the canonical implementation. Compare outputs across all thresholds before switching. MarketPanel's `formatCurrency()` (dollar prefix) is a separate concern and should remain. |

### Open Questions

1. **BTC Buy/Sell button convention** (from UX spec Section 9.6 Q1): The MarketPanel currently uses green=buy, red=sell (financial convention). The UX spec recommends keeping this convention for action buttons but using currency colors for cost/proceeds labels. **Recommendation**: Follow the UX spec -- keep green/red for Buy/Sell action buttons, apply currency colors to the "Cost:" and "Proceeds:" label values. This is the lowest-risk approach.

2. **Knowledge Points visibility** (from UX spec Section 9.6 Q2): KP appears in `GameState` (`knowledge_points` field, line 49 of `api.ts`) but is not displayed anywhere in the current UI. The color assignment (`#c084fc` purple-400) is defined and ready in the palette. **Recommendation**: Include the KP color in the CSS variables and utility map now. Do not add KP display -- that is a separate feature.

3. **Power color change acceptance** (from UX spec Section 9.6 Q3): Changing PWR from purple to yellow is the most visible change. **Recommendation**: Proceed with the change as specified. Purple has no association with power/electricity, and the current color collides with the Compute hardware category color. Yellow/energy is a widely understood association.

### Flagged Assumptions

- **Assumption**: The `cost_type` field on `UpgradeTemplate` is either `'money'` or `'cu'` (no other values). Verified by inspecting the upgrade data flow; the backend `catalog/upgrades.go` only defines these two cost types. If a new cost type is added, the `getCostCurrencyId()` fallback to `'cu'` is safe.
- **Assumption**: All hardware, service, SaaS, and research purchases cost CU. Verified from the backend game engine. If new cost currencies are introduced, the utility map is ready with all six currencies.

---

## 8. Testing Strategy

### Visual QA Checklist

Since this is a purely visual change with no logic impact, testing is visual verification:

For each component, verify:
1. Currency values use the correct color per the UX spec Section 5 table
2. Cost buttons use the currency color (not category/type/branch color)
3. Disabled/unaffordable buttons use `--text-muted` per UX spec Section 6.2
4. Hover brightness effect still works on currency-colored buttons
5. Multi-currency stat lines show each segment in its currency color

### Contrast Verification

Run WCAG contrast checks on the six currency colors against all three background tiers. The UX spec provides pre-computed ratios (Section 2, Contrast Ratios table); verify these match the implementation. Pay special attention to REP (#3b82f6) on `--bg-card` (#182028) -- confirm the `-on-card` variant (#60a5fa) is used where needed.

### Regression Checks

- Non-currency colors are unchanged: success green badges ("LIVE", "OWNED"), error red, category headings, tier progress bar
- Overclock pulse animation still works (tests that `animate-gentle-pulse` still applies correctly)
- Throttle warning button remains red
- Heat warning text remains red
- Buy/Sell buttons in MarketPanel remain green/red

### Automated Testing Opportunity (Future)

A visual regression test using a screenshot comparison tool could be added after this work to prevent future color drift. Not in scope for this TDD but worth noting.

---

## 9. Observability & Operational Readiness

This is a frontend-only visual change. No backend observability, alerting, or runbook changes are needed.

If player feedback surfaces confusion about the warm-color currencies (CU/BTC/PWR), the CSS variables can be adjusted without code changes. The CSS variable architecture makes this a config-level change.

---

## 10. Implementation Phases

Phases are ordered by the UX spec's P0/P1/P2 priorities (Section 9.5). Each phase is independently shippable.

### Phase 1: Foundation (P0) -- Size: S

**Scope**: CSS variables + TypeScript utility + `formatNumber` consolidation

**Files created**:
- `apps/desktop/src/utils/currencyColors.ts` (new)

**Files modified**:
- `apps/desktop/src/styles/global.css` (add `--currency-*` variables to `:root`)

**Tasks**:
1. Add all 25 `--currency-*` CSS custom properties to `global.css` `:root`, per UX spec Section 3
2. Create `apps/desktop/src/utils/` directory
3. Create `currencyColors.ts` with `CurrencyId` type, `CurrencyColorSet` type, `CURRENCY_COLORS` constant, and `getCostCurrencyId()` helper
4. Extract the `formatNumber()` function from `ResearchPanel.tsx` (the most complete version with T/B/M/K) into `currencyColors.ts` (or a separate `formatNumber.ts` utility)

**Acceptance criteria**:
- `--currency-cu` through `--currency-pwr-glow` are defined in `global.css`
- `--currency-rep-on-card` is defined
- `CURRENCY_COLORS` is importable and type-checks
- `getCostCurrencyId('money')` returns `'money'`; `getCostCurrencyId('cu')` returns `'cu'`; unknown returns `'cu'`
- `formatNumber()` is importable from the utility file
- No visual changes yet (this phase only adds definitions)

### Phase 2: CurrencyBar (P0) -- Size: S

**Scope**: Fix the highest-visibility surface, resolve BTC/CU collision and PWR color

**Files modified**:
- `apps/desktop/src/components/CurrencyBar.tsx`

**Tasks**:
1. Import `CURRENCY_COLORS` from utility
2. Replace `var(--accent-amber)` on CU Stat (line 20) with `CURRENCY_COLORS.cu.color`
3. Replace `var(--accent-blue)` on REP Stat (line 21) with `CURRENCY_COLORS.rep.color`
4. Replace `var(--accent-purple)` on PWR Stat (line 22) with `CURRENCY_COLORS.pwr.color` -- **visible color change** purple -> yellow
5. Replace `var(--accent-green)` on USD Stat (line 23) with `CURRENCY_COLORS.money.color`
6. Replace `var(--accent-amber)` on BTC Stat (line 24) with `CURRENCY_COLORS.btc.color` -- **visible color change** amber -> orange
7. Change rate display (line 64) from `var(--text-secondary)` to the CU currency color at 0.7 opacity, per UX spec Section 5.1

**Acceptance criteria**:
- BTC and CU are visually distinct colors in the currency bar
- PWR displays in yellow (#facc15), not purple
- CU rate suffix shows in amber at reduced opacity, not gray
- Non-currency stats (SLOTS/RACK, COLO, GROUP) are unchanged

### Phase 3: Panel Cost Buttons (P1) -- Size: M

**Scope**: Align all cost buttons to use currency color instead of category/type/branch color

**Files modified** (can be done in parallel sub-batches):
- `HardwarePanel.tsx` -- buy buttons + component upgrade buttons use `CURRENCY_COLORS.cu`
- `UpgradePanel.tsx` -- buy buttons switch color based on `cost_type`
- `ServicePanel.tsx` -- deploy buttons use `CURRENCY_COLORS.cu`
- `SaasPanel.tsx` -- deploy buttons use `CURRENCY_COLORS.cu`
- `ResearchPanel.tsx` -- research cost buttons use `CURRENCY_COLORS.cu`
- `DatacenterPanel.tsx` -- optimize/upgrade buttons; see below for dual-currency buttons
- `TierProgress.tsx` -- upgrade button uses `CURRENCY_COLORS.cu` instead of `--accent-green`
- `OverclockPanel.tsx` -- cost buttons use `CURRENCY_COLORS.cu` (same hex, new variable)
- `DonatePanel.tsx` -- donation buttons use `CURRENCY_COLORS.cu` (same hex, new variable)

**Special cases**:
- `UpgradePanel.tsx`: The "Buy All" button per category should also use the currency color of that category's cost type. Since all upgrades in a category may have different cost types, the "Buy All" button should use the dominant cost type's color, or default to CU.
- `DatacenterPanel.tsx` line 72 ("Upgrade -- $X + Y CU"): The UX spec recommends using `--accent-cyan` for the button (datacenter category) and currency colors for the cost text within it. The implementer should color the "$X" portion in `CURRENCY_COLORS.money.color` and "Y CU" in `CURRENCY_COLORS.cu.color` while the button frame stays cyan.
- `DatacenterPanel.tsx` line 97 ("Optimize Rack -- X CU"): Switch button to `CURRENCY_COLORS.cu` palette per UX spec.

**Tasks**:
1. Import `CURRENCY_COLORS` and `getCostCurrencyId` in each file
2. Replace category/type/branch color references on cost buttons with the appropriate currency color set
3. Preserve category/type/branch colors on headings, labels, and effect text
4. Fix `HardwarePanel.tsx` stats summary grid colors (CU/tick, Power, Bonus split per Type 2 changes)

**Acceptance criteria**:
- All CU cost buttons across all panels display in amber (`--currency-cu` palette)
- Money cost buttons in UpgradePanel display in green (`--currency-money` palette)
- Category headings (Compute, Network, etc.) retain their original colors
- Upgrade type headings (Cooling, Networking, etc.) retain their original colors
- Research branch headings and effect text retain their original colors
- Disabled buttons still use `--text-muted` and reduced opacity
- HardwarePanel stats grid shows CU/tick in amber, Power in yellow, bonuses split by currency

### Phase 4: Multi-Currency Stat Lines (P1) -- Size: M

**Scope**: Create shared components and apply color-coding to multi-currency stat lines

**Files created**:
- `apps/desktop/src/components/shared/CurrencyValue.tsx` (new)
- `apps/desktop/src/components/shared/CurrencyStatLine.tsx` (new)

**Files modified**:
- `HardwarePanel.tsx` -- hardware card stat lines (lines 87-89, 222-225)
- `ServicePanel.tsx` -- service stat lines (lines 32, 66-68)
- `DatacenterPanel.tsx` -- colo rack stat lines (lines 104-105, 152, 161)
- `SaasPanel.tsx` -- revenue and cost display (lines 87, 126)
- `MarketPanel.tsx` -- cost labels (lines 283, 286, 347, 350)
- `SocialPanel.tsx` -- compute pool value (line 164), leaderboard scores (line 229)

**Tasks**:
1. Create `<CurrencyValue>` component
2. Create `<CurrencyStatLine>` component
3. Replace single-color stat line strings with `<CurrencyStatLine>` in each file
4. Apply `<CurrencyValue>` for standalone currency displays (SocialPanel compute pool, MarketPanel cost labels, SaasPanel revenue)
5. In SocialPanel leaderboard, map category to currency for score colors: `compute` -> `cu`, `reputation` -> `rep`, others -> neutral accent
6. In MarketPanel portfolio grid, fix "BTC Value ($)" and "Portfolio ($)" to use `CURRENCY_COLORS.money.color` instead of `--accent-amber`

**Acceptance criteria**:
- Multi-currency stat lines show each segment in its own currency color
- Separators use `--text-muted`
- Non-currency text in stat lines (rack units "2U", service type names) uses `--text-secondary`
- `<CurrencyValue>` correctly formats numbers using the shared `formatNumber()` utility
- SocialPanel leaderboard scores are colored per their category
- MarketPanel portfolio dollar values use green (`--currency-money`)

### Phase 5: Polish & Consolidation (P2) -- Size: S

**Scope**: Clean up remaining inconsistencies and eliminate duplicated `formatNumber()` calls

**Files modified**:
- `CurrencyBar.tsx` -- replace local `formatNumber()` with shared utility import
- `DatacenterPanel.tsx` -- replace local `formatNumber()` with shared utility import
- `OverclockPanel.tsx` -- replace local `formatNumber()` with shared utility import
- `DonatePanel.tsx` -- replace local `formatNumber()` with shared utility import
- `SaasPanel.tsx` -- color-code the unlock gate requirements (line 33: "50,000 CU" in CU color, "100 REP" in REP color)

**Acceptance criteria**:
- No component defines its own `formatNumber()` -- all import from the shared utility
- `formatNumber()` thresholds are consistent across all components
- SaaS unlock gate shows currency-colored requirements

### Dependency Graph

```
Phase 1 (Foundation)
  |
  +---> Phase 2 (CurrencyBar)
  |
  +---> Phase 3 (Cost Buttons)  -- can parallel with Phase 2
  |
  +---> Phase 4 (Stat Lines)    -- can parallel with Phase 2 and 3
           |
           v
        Phase 5 (Polish)         -- depends on Phase 4 (shared components exist)
```

Phases 2, 3, and 4 can proceed in parallel after Phase 1. Phase 5 depends on Phase 4 for the shared `formatNumber()` export location.

### Complexity Estimates

| Phase | Size | Est. Files Changed | Risk |
|---|---|---|---|
| 1: Foundation | S | 2 (1 new, 1 modified) | Very low -- additive only |
| 2: CurrencyBar | S | 1 | Low -- contained to one file, 2 visible color changes |
| 3: Cost Buttons | M | 9 | Medium -- touches many files, requires testing each panel |
| 4: Stat Lines | M | 8 (2 new, 6 modified) | Medium -- new shared components, most pervasive change |
| 5: Polish | S | 5 | Low -- mechanical cleanup |
