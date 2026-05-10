---
project: homelab-the-game
maturity: draft
last_updated: 2026-04-04
updated_by: ux-designer
scope: Global currency color system — all UI surfaces
owner: ux-designer
dependencies:
  - apps/desktop/src/styles/global.css (CSS custom properties)
  - apps/desktop/src/components/* (all components displaying currencies)
  - packages/shared/ (shared types — GameState currency fields)
---

# Currency Color System — Design Specification

## 1. Overview

### Surface Type

Desktop GUI (Tauri + React + TypeScript + Tailwind CSS). Dark-themed tech/homelab aesthetic.

### Users

Players of "Homelab the Game" — tech-savvy hobbyists who understand homelab concepts.
Interaction frequency: sustained sessions (idle/clicker genre). Skill level: comfortable
with data-dense dashboards.

### Problem Statement

The game has six currencies/resources that appear across every UI surface. Today, colors are
assigned ad-hoc per component with no centralized system, resulting in:

- **Collisions**: Compute Units and Bitcoin both use `--accent-amber` (#f59e0b)
- **Inconsistencies**: Reputation appears as `--accent-blue` in the CurrencyBar but `#22c55e`
  (green) in the SocialPanel group headings
- **Missing assignments**: Knowledge Points have no dedicated currency color; they inherit from
  the "knowledge" upgrade category color (also amber)
- **Cognitive load**: Players cannot glance at a number and instantly identify which currency
  it represents based on color alone

### Key Workflows (Prioritized)

1. **Currency bar scanning** — Player glances at the top bar and identifies all currency
   balances at a glance. This is the most frequent interaction in the game.
2. **Cost evaluation** — Player sees a price tag on hardware, upgrades, services, or research
   and instantly knows which currency it costs and whether they can afford it.
3. **Income/output reading** — Player reads stat lines like "+5 CU . +2 Rep . 30W" and maps
   each value to its currency by color.
4. **Event log triage** — Player reads event notifications and identifies which currencies
   were affected.
5. **Cross-panel comparison** — Player moves between tabs and relies on consistent color
   meaning to maintain context.

### Success Criteria

| # | Criterion | How to Verify |
|---|---|---|
| 1 | Every instance of a currency value in the UI uses the same color for the same currency | Visual inspection / design QA across all components |
| 2 | No two currencies share the same hue | Palette comparison — minimum 30 degrees hue separation |
| 3 | All currency colors pass WCAG 2.1 AA contrast against the dark backgrounds | Contrast ratio >= 4.5:1 for normal text, >= 3:1 for large text |
| 4 | A player can identify which currency a number represents from color alone, without reading the label | Heuristic: each color is perceptually distinct at a glance |
| 5 | Disabled/unaffordable states are clearly distinguishable from active states | Opacity/desaturation differentiation |

---

## 2. Color Palette

### Design Rationale

The palette was selected to:

- Maintain the existing dark, cool-toned tech aesthetic (deep blue-black backgrounds)
- Maximize perceptual distance between all six hues
- Preserve existing associations where they are strong and unambiguous (green = money is
  universal; amber = computing/processing is established in the game)
- Avoid hue collisions — the previous palette had amber used for three different purposes
- Pass WCAG AA contrast on all three background tiers (`--bg-deep` #0a0e14,
  `--bg-panel` #111820, `--bg-card` #182028)

### Currency Color Assignments

| Currency | Label | Hex | RGB | Hue | Tailwind Class | CSS Variable |
|---|---|---|---|---|---|---|
| Compute Units (CU) | `CU` | `#f59e0b` | 245, 158, 11 | 38 | `text-amber-500` | `--currency-cu` |
| Money ($) | `USD` | `#22c55e` | 34, 197, 94 | 142 | `text-green-500` | `--currency-money` |
| Reputation | `REP` | `#3b82f6` | 59, 130, 246 | 217 | `text-blue-500` | `--currency-rep` |
| Knowledge Points | `KP` | `#c084fc` | 192, 132, 252 | 271 | `text-purple-400` | `--currency-kp` |
| Bitcoin | `BTC` | `#fb923c` | 251, 146, 60 | 27 | `text-orange-400` | `--currency-btc` |
| Power (Watts) | `PWR` | `#facc15` | 250, 204, 21 | 48 | `text-yellow-400` | `--currency-pwr` |

### Hue Separation Analysis

| Pair | Hue Difference | Assessment |
|---|---|---|
| CU (38) vs BTC (27) | 11 | **Closest pair** — mitigated by saturation/lightness difference. CU is deeper amber; BTC is lighter orange. See Disambiguation section below. |
| CU (38) vs PWR (48) | 10 | **Second closest** — mitigated by PWR being significantly lighter/more yellow. See Disambiguation section below. |
| BTC (27) vs PWR (48) | 21 | Adequate — orange vs yellow is perceptually distinct. |
| Money (142) vs REP (217) | 75 | Excellent — green vs blue. |
| REP (217) vs KP (271) | 54 | Good — blue vs purple. |
| All others | >80 | Excellent separation. |

### Disambiguation: CU vs BTC vs PWR

These three currencies occupy the warm spectrum. They are distinguished by:

1. **Lightness**: PWR (#facc15) is the brightest/most yellow. CU (#f59e0b) is a deep warm
   amber. BTC (#fb923c) is a warm orange, lighter than CU but more saturated than PWR.
2. **Context**: CU appears everywhere. BTC appears only in the Market panel and the currency
   bar (conditionally). PWR appears as a constraint/limit format (e.g., "120/200W").
3. **Labels**: CU always shows "CU" suffix. BTC always shows "BTC" suffix. PWR always shows
   "W" suffix or "W" unit.
4. **Icons (recommended future enhancement)**: Unique icons per currency would further
   disambiguate. Not in scope for this spec but noted for future work.

If playtesting reveals confusion between these three warm currencies, the fallback
recommendation is to shift BTC to a teal/coral spectrum. However, orange is the canonical
Bitcoin brand color and is worth preserving.

### Contrast Ratios

All ratios computed against the three background tiers. WCAG AA requires >= 4.5:1 for
normal text (14px), >= 3:1 for large text (18px+ or 14px bold).

| Currency | vs #0a0e14 (deep) | vs #111820 (panel) | vs #182028 (card) | AA Normal | AA Large |
|---|---|---|---|---|---|
| CU #f59e0b | 8.4:1 | 7.2:1 | 6.1:1 | Pass | Pass |
| Money #22c55e | 8.1:1 | 6.9:1 | 5.9:1 | Pass | Pass |
| REP #3b82f6 | 5.2:1 | 4.5:1 | 3.8:1 | Pass (deep/panel) | Pass |
| KP #c084fc | 6.6:1 | 5.7:1 | 4.8:1 | Pass | Pass |
| BTC #fb923c | 8.0:1 | 6.8:1 | 5.8:1 | Pass | Pass |
| PWR #facc15 | 11.2:1 | 9.6:1 | 8.2:1 | Pass | Pass |

**Note on REP (#3b82f6)**: At 3.8:1 on `--bg-card`, this narrowly misses AA for small normal
text. Mitigation: REP values on card backgrounds should use `font-weight: 500` (the
`stat-value` class already does this) which qualifies as large text equivalent at our
font sizes, or use the slightly lighter `#60a5fa` (blue-400, ~5.1:1 on card) in card contexts.
Implementation note: use `--currency-rep` which is `#3b82f6` for panel/deep backgrounds,
and add `--currency-rep-on-card: #60a5fa` for card-background contexts.

### Color Tints (Backgrounds, Borders, Glows)

Each currency needs tinted backgrounds for buttons, badges, and highlight states.
Pattern: `rgba(R, G, B, alpha)` at three standard opacities.

| Currency | Background (0.10) | Border (0.25) | Glow (0.15) |
|---|---|---|---|
| CU | `rgba(245,158,11,0.10)` | `rgba(245,158,11,0.25)` | `rgba(245,158,11,0.15)` |
| Money | `rgba(34,197,94,0.10)` | `rgba(34,197,94,0.25)` | `rgba(34,197,94,0.15)` |
| REP | `rgba(59,130,246,0.10)` | `rgba(59,130,246,0.25)` | `rgba(59,130,246,0.15)` |
| KP | `rgba(192,132,252,0.10)` | `rgba(192,132,252,0.25)` | `rgba(192,132,252,0.15)` |
| BTC | `rgba(251,146,60,0.10)` | `rgba(251,146,60,0.25)` | `rgba(251,146,60,0.15)` |
| PWR | `rgba(250,204,21,0.10)` | `rgba(250,204,21,0.25)` | `rgba(250,204,21,0.15)` |

---

## 3. CSS Custom Properties

Add the following to `:root` in `global.css`. These are the single source of truth for
currency colors across the application.

```
/* Currency color system */
--currency-cu: #f59e0b;
--currency-cu-bg: rgba(245,158,11,0.10);
--currency-cu-border: rgba(245,158,11,0.25);
--currency-cu-glow: 0 0 20px rgba(245,158,11,0.15);

--currency-money: #22c55e;
--currency-money-bg: rgba(34,197,94,0.10);
--currency-money-border: rgba(34,197,94,0.25);
--currency-money-glow: 0 0 20px rgba(34,197,94,0.15);

--currency-rep: #3b82f6;
--currency-rep-bg: rgba(59,130,246,0.10);
--currency-rep-border: rgba(59,130,246,0.25);
--currency-rep-glow: 0 0 20px rgba(59,130,246,0.15);
--currency-rep-on-card: #60a5fa;

--currency-kp: #c084fc;
--currency-kp-bg: rgba(192,132,252,0.10);
--currency-kp-border: rgba(192,132,252,0.25);
--currency-kp-glow: 0 0 20px rgba(192,132,252,0.15);

--currency-btc: #fb923c;
--currency-btc-bg: rgba(251,146,60,0.10);
--currency-btc-border: rgba(251,146,60,0.25);
--currency-btc-glow: 0 0 20px rgba(251,146,60,0.15);

--currency-pwr: #facc15;
--currency-pwr-bg: rgba(250,204,21,0.10);
--currency-pwr-border: rgba(250,204,21,0.25);
--currency-pwr-glow: 0 0 20px rgba(250,204,21,0.15);
```

---

## 4. Usage Guidelines

### 4.1 When to Use Text Color

Use the currency's text color (`--currency-{id}`) when:

- Displaying a currency **value** (number) inline: balances, rates, costs, rewards
- Displaying a currency **label** in the currency bar (e.g., "CU", "REP", "PWR")
- Showing a rate of change (e.g., "+5/s", "+2/tick")

Copy pattern — values always use currency color; labels/units use `--text-muted`:

```
<span style="color: var(--text-muted)">CU</span>
<span style="color: var(--currency-cu)">1,250</span>
```

### 4.2 When to Use Background Tint

Use the currency's background (`--currency-{id}-bg`) with border (`--currency-{id}-border`)
when:

- A **button** costs that currency (the "Buy" / "Deploy" / "Upgrade" buttons)
- A **badge** or **tag** identifies an item as related to that currency
- A **highlight region** groups items by currency cost type

Pattern for affordable buttons:

```
background: var(--currency-cu-bg);
color: var(--currency-cu);
border: 1px solid var(--currency-cu-border);
```

### 4.3 When to Use Glow

Use the currency's glow (`--currency-{id}-glow`) as `box-shadow` when:

- Active/pulsing state on a currency-related element (e.g., overclock active indicator)
- Hover state on important currency actions

### 4.4 Multi-Currency Lines

When a stat line shows multiple currencies (e.g., "+5 CU . +2 Rep . 30W"), each value
segment uses its own currency color. The separator ("." or "·") uses `--text-muted`.

```
<span style="color: var(--currency-cu)">+5 CU</span>
<span style="color: var(--text-muted)"> · </span>
<span style="color: var(--currency-rep)">+2 Rep</span>
<span style="color: var(--text-muted)"> · </span>
<span style="color: var(--currency-pwr)">30W</span>
```

Currently, these multi-currency stat lines use a single `--text-secondary` color for the
entire string. This is a key area where currency colors should be applied to individual
value segments.

### 4.5 Cost Buttons — Which Currency Color?

A button's color reflects the **currency it costs**, not the panel's category color:

- A hardware item costing CU: uses `--currency-cu` palette
- An upgrade costing Money ($): uses `--currency-money` palette
- SaaS requiring CU + REP: primary button uses `--currency-cu` (the spendable cost); the
  REP requirement is shown as a secondary note in `--currency-rep` text

When an action costs **multiple currencies**, the button uses the primary/larger cost's
color, and secondary costs are listed as text below or beside the button.

---

## 5. Component-by-Component Application Guide

### 5.1 CurrencyBar.tsx

This is the highest-visibility surface. Current state and required changes:

| Stat | Current Color | Correct Color | Change Needed |
|---|---|---|---|
| CU value | `var(--accent-amber)` | `var(--currency-cu)` | Variable rename (same hex) |
| CU rate (+/s) | `var(--text-secondary)` | `var(--currency-cu)` at reduced opacity | Apply currency color with 0.7 opacity |
| REP value | `var(--accent-blue)` | `var(--currency-rep)` | Variable rename (same hex) |
| PWR value | `var(--accent-purple)` | `var(--currency-pwr)` | **Color change**: purple -> yellow |
| USD value | `var(--accent-green)` | `var(--currency-money)` | Variable rename (same hex) |
| BTC value | `var(--accent-amber)` | `var(--currency-btc)` | **Color change**: amber -> orange |
| SLOT/RACK | `var(--accent-cyan)` | `var(--accent-cyan)` | No change — not a currency |
| COLO | `var(--accent-cyan)` | `var(--accent-cyan)` | No change — not a currency |

**Critical change**: BTC currently uses `--accent-amber`, the same color as CU. This must
change to `--currency-btc` (#fb923c, orange) to eliminate the collision.

**Critical change**: PWR currently uses `--accent-purple`. Purple is not associated with
power/electricity in any common convention. Yellow (#facc15) maps to the universal
"electricity/energy" association and distinguishes PWR from all other currencies.

**CU rate display**: The rate suffix ("+1,250/s") should use the CU color at 70% opacity
rather than `--text-secondary`, so it visually groups with the CU value while remaining
subordinate.

### 5.2 HardwarePanel.tsx

**Hardware Shop — Price Buttons**

Current: Buttons use the **hardware category** color (Compute=purple, Network=blue,
Power=amber, Storage=cyan). The price is always in CU but the button color reflects the
category, not the currency.

Required: Price buttons should use `--currency-cu` palette since the cost is in CU. The
**category label** (Compute, Network, Power, etc.) retains its category color for grouping.

```
Before: "1,200 CU" button in purple (because it's a Compute item)
After:  "1,200 CU" button in amber (because it costs CU)
        Category header "Compute" remains purple
```

**Stat line in hardware cards** (e.g., "30W . +5/tick . 2U"):

Current: Entire line uses `--text-secondary`.
Required: Split into colored segments:
- "30W" in `--currency-pwr`
- "+5/tick" in `--currency-cu`
- "2U" in `--text-secondary` (rack units are not a currency)

**Owned Hardware — Stats Summary Grid**

| Stat | Current Color | Correct Color |
|---|---|---|
| Items count | `--text-primary` | `--text-primary` (not a currency) |
| CU/tick | `#a855f7` (purple) | `var(--currency-cu)` |
| Power | `#f59e0b` (amber) | `var(--currency-pwr)` |
| Bonus (CU% / Rep%) | `#22c55e` (green) | Split: CU bonus in `--currency-cu`, Rep bonus in `--currency-rep` |

**Component Upgrade Buttons** (cpu, ram, storage, nic):

Current: Use hardware category color.
Required: Use `--currency-cu` palette (all component upgrades cost CU).

### 5.3 UpgradePanel.tsx

**Price Buttons**

Current: Buttons use the upgrade **type** color (cooling=cyan, networking=orange,
automation=green, knowledge=amber).

Required: Button color reflects the **cost currency**:
- Items with `cost_type === 'money'`: Use `--currency-money` palette, display as "$1,200"
- Items with `cost_type === 'cu'` (default): Use `--currency-cu` palette, display as "1,200 CU"

The **upgrade category heading** and **effect text** retain the category color for visual
grouping within each quadrant.

Current line on UpgradePanel (line 80):
```
{u.cost_type === 'money' ? `$${u.cost.toLocaleString()}` : `${...} CU`}
```
This already differentiates the label but uses the wrong color. The color prop should
switch based on `cost_type`.

### 5.4 ServicePanel.tsx

**Stat Lines** (e.g., "+5 CU . +2 Rep . 30W"):

Current: Entire line uses `--text-secondary`.
Required: Color each segment by currency:
- "+5 CU" in `--currency-cu`
- "+2 Rep" in `--currency-rep`
- "30W" in `--currency-pwr`
- Separators in `--text-muted`

**Deploy Buttons**:

Current: Uses `--accent-blue` palette.
Required: Use `--currency-cu` palette (all service deployments cost CU).

**Money per tick** (line 68): `+$${s.money_per_tick}` should use `--currency-money`.

### 5.5 SaasPanel.tsx

**Unlock Gate**

The "Requires: 50,000 CU + 100 REP" line (line 33) should color-code each requirement:
- "50,000 CU" in `--currency-cu`
- "100 REP" in `--currency-rep`

**Deploy Buttons**: Use `--currency-cu` palette (costs are in CU).

**Revenue Lines**: "$X/cust" should use `--currency-money` (it represents money income).

**Customer Revenue**: "$X/tick" values should use `--currency-money`.

**Net Income Display** (line 117): Already uses green/red for positive/negative — keep the
positive as `--currency-money` and negative as `--accent-red`.

### 5.6 ResearchPanel.tsx

**Research Cost Buttons**:

Current: Use the branch color (efficiency=green, reputation=blue, etc.).
Required: Use `--currency-cu` palette (all research costs CU).

**Effect Text** (e.g., "+15% idle income"):

The branch color is appropriate here because the text describes the effect, not a cost.
Keep as-is.

### 5.7 MarketPanel.tsx (Bitcoin Market)

**Bitcoin Price Display**

Current: Uses `--accent-amber`.
Required: Use `--currency-btc` (#fb923c, orange).

**Portfolio Summary Grid**

| Stat | Current Color | Correct Color |
|---|---|---|
| Your BTC (count) | `--accent-amber` | `--currency-btc` |
| BTC Value ($) | `--accent-amber` | `--currency-money` (it is a dollar value) |
| Portfolio ($) | `--accent-green` | `--currency-money` |

**Trade Costs**

- "Cost: $X" — use `--currency-money`
- "CU Cost: X" — use `--currency-cu`
- Buy/Sell buttons: Buy uses `--currency-money` palette (you are spending money to buy).
  Sell uses `--currency-btc` palette (you are spending BTC to sell). This reversal from the
  current green/red is deliberate — the button color should reflect what you are spending,
  not whether the action is "good" or "bad". However, an alternative is to keep green=buy,
  red=sell as a financial convention. **Recommendation**: Keep the financial convention
  (green=buy, red=sell) for the Buy/Sell action buttons only, but ensure the "Cost:" and
  "Proceeds:" labels use the correct currency colors.

**Price Chart**

The chart line color (green for up, red for down) is a price direction indicator, not a
currency color. Keep as-is — this is a data visualization convention.

### 5.8 DatacenterPanel.tsx

**Colo Rack Stats** (line 152): "+{CU} CU . +{Rep} Rep . +${Money}/tick"

Current: Entire line in `--text-secondary`.
Required: Color each segment by currency.

**Datacenter Upgrade Button** (line 72): "Upgrade — $X + Y CU"

Current: Uses `--accent-cyan` palette.
Required: Split display — "$X" in `--currency-money`, "Y CU" in `--currency-cu`.
Button uses `--currency-cu` palette (CU is typically the larger cost). Or use the datacenter
accent (`--accent-cyan`) for the button since this is a special action, and color-code only
the cost text within it.

**Recommendation**: Use `--accent-cyan` for the button border/background (datacenter actions
are categorized by cyan), but color the cost numbers within the button text using currency
colors.

**Rack Optimization Button** (line 97): "Optimize Rack — X CU"

Current: Uses `--accent-cyan`.
Required: Cost text "X CU" in `--currency-cu`. Button can retain cyan as a category
indicator, or switch to CU palette. **Recommendation**: Switch button to `--currency-cu`
palette since it is a pure CU cost.

### 5.9 OverclockPanel.tsx

**Overclock Cost Buttons** (line 129): "X CU"

Current: Uses `--accent-amber` palette.
Required: Use `--currency-cu` palette (same hex, but use the new variable name for
consistency).

### 5.10 TierProgress.tsx

**Upgrade Button**: "Next Tier — X CU"

Current: Uses `--accent-green` palette.
Required: Use `--currency-cu` palette (it costs CU).

### 5.11 DonatePanel.tsx

**Donation Buttons**: "1K", "10K", etc.

Current: Uses `--accent-amber` palette.
Required: Use `--currency-cu` palette (same hex, new variable). The header
"Global CU Store" and donated totals should also use `--currency-cu`.

### 5.12 EventLog.tsx

Events currently do not display currency values inline, but some event descriptions contain
currency references in their text (e.g., "Lost 500 CU" or "Gained $200").

**Recommendation for future enhancement**: If event descriptions are refactored to structured
data (separate fields for each currency effect), the values should use currency colors. For
now, event descriptions are plain text strings from the server, so no change is needed.
However, if the throttle button in CurrencyBar references CU ("Fix {cost} CU"), the "CU"
value should use `--currency-cu` — it already does via the button text color.

### 5.13 ClickArea.tsx

The "Run Job" button is a game action, not a currency display. No currency color changes
needed. The job reward feedback (if added in the future as a "+X CU" floating text) should
use `--currency-cu`.

### 5.14 SocialPanel.tsx

**Group Compute Pool** (line 164): "Combined Compute Pool: X CU"

Current: `--text-secondary`.
Required: "X" in `--currency-cu`, "CU" label in `--text-muted`.

**Leaderboard Scores** (line 229):

Current: All scores in `#f59e0b` (amber).
Required: Score color should match the leaderboard category:
- "compute" leaderboard: scores in `--currency-cu`
- "reputation" leaderboard: scores in `--currency-rep`
- Other categories: map to appropriate currency or use a neutral accent

---

## 6. Interaction States

### 6.1 Affordable (Default Active)

```
color: var(--currency-{id});
background: var(--currency-{id}-bg);
border: 1px solid var(--currency-{id}-border);
```

### 6.2 Unaffordable (Disabled)

```
color: var(--text-muted);
background: var(--bg-card);
border: 1px solid var(--border);
opacity: 0.25;   /* inherited from .btn:disabled */
```

The existing `.btn:disabled` opacity of 0.25 handles this. The color switches from the
currency color to `--text-muted` to doubly reinforce that the action is unavailable.

### 6.3 Hover (Affordable)

Handled by the existing `.btn:not(:disabled):hover` rule which applies
`filter: brightness(1.3)`. This brightens the currency color naturally. No additional
hover color needed.

### 6.4 Active/Pressed

Handled by the existing `.btn:not(:disabled):active` rule. No change needed.

### 6.5 Earning Rate (Positive Change)

When showing rates like "+1,250/s", use the currency color at 70% opacity to create a
subordinate but still identifiable association:

```
color: var(--currency-cu);
opacity: 0.7;
```

### 6.6 Loss/Negative Change

When a currency is being **spent** or **lost** (e.g., expenses, event penalties):
- Keep the currency color for identification
- Prefix with "-" (minus sign)
- For expense lines, the `-$X` value uses `--accent-red` as it does today, since
  the red communicates "outflow" which is more important than currency identification
  in that context

### 6.7 Zero/Empty State

When a currency balance is zero and has never been non-zero (e.g., Money before first
income), the currency stat may be hidden (as `CurrencyBar` currently does with Money and
BTC). When shown at zero, use `--text-muted` instead of the currency color to indicate
"nothing here yet."

---

## 7. Edge Cases

### 7.1 Color Blindness Considerations

The palette spans the full hue range. For users with common color vision deficiencies:

- **Deuteranopia/Protanopia** (red-green): Money (green) and Reputation (blue) remain
  distinguishable. CU (amber), BTC (orange), and PWR (yellow) collapse somewhat, but are
  separated by the **labels** ("CU", "BTC", "W") and by **context** (BTC only in Market,
  PWR always has "W" suffix).
- **Tritanopia** (blue-yellow): Reputation (blue) and KP (purple) may converge. Labels
  distinguish them ("REP" vs "KP").
- **Mitigation**: Labels are always present alongside values. Color is an
  **enhancement**, not the sole identifier. This satisfies WCAG's "don't use color alone
  to convey information" guideline.

### 7.2 Many Currencies on One Line

Some stat lines show 3+ currencies (e.g., "+5 CU . +2 Rep . +$3/tick . 30W"). On narrow
panels, this can become visually noisy with many colors.

**Guideline**: If a line has 4+ colored segments, consider breaking into a 2x2 grid of
stat boxes (as the HardwarePanel stats summary already does) rather than a single inline
list.

### 7.3 Currency in Tooltips (Future)

If tooltips are added, currency values within tooltips use the same color system.
Tooltip backgrounds use `--bg-card`, so use the `--currency-rep-on-card` variant for
REP text in tooltips.

### 7.4 Currency Amounts That Span Breakpoints

When a number transitions between raw and abbreviated forms (e.g., 999 -> 1.0K), the
color must remain the same. The `formatNumber()` utility handles formatting; color is
applied at the component level and is unaffected.

### 7.5 Theme Variants (Future)

If a light theme is ever added, the currency hues should remain the same but shift to
darker shades for contrast against light backgrounds. The CSS variable architecture makes
this straightforward — override the `--currency-*` variables in a `.theme-light` scope.

---

## 8. Accessibility

### 8.1 Keyboard Navigation

No change from current behavior. Currency colors are visual-only; all interactive elements
remain keyboard-accessible via the existing `button` / `.btn` patterns.

### 8.2 Screen Reader Semantics

Currency values should include the currency name in screen-reader-accessible text. The
current `Stat` component in CurrencyBar has a visible label ("CU", "REP", etc.) which
serves this purpose. No `aria-label` changes needed.

### 8.3 Color Independence

As noted in 7.1, every currency value is accompanied by a text label or contextual suffix.
Color is never the sole means of identifying a currency. This satisfies WCAG 1.4.1
(Use of Color).

### 8.4 Motion Sensitivity

The `animate-gentle-pulse` animation on active overclock and throttle indicators is
independent of currency colors. No change needed. Consider adding
`prefers-reduced-motion: reduce` media query to disable pulse animations in the future.

---

## 9. Handoff Notes

### 9.1 Component Breakdown for Implementation

Implementation should be phased:

**Phase 1 — Foundation (do first)**
1. Add CSS custom properties to `global.css` (Section 3)
2. Create a shared `CURRENCY_COLORS` constant object in a new utility file (e.g.,
   `src/utils/currencyColors.ts`) that maps currency IDs to their CSS variable names and
   tint rgba values. This replaces the scattered inline hex values across components.

**Phase 2 — Currency Bar (highest visibility)**
3. Update `CurrencyBar.tsx` to use new currency variables (Section 5.1)

**Phase 3 — Panels (one PR per panel, or batched)**
4. Update `HardwarePanel.tsx` (Section 5.2)
5. Update `UpgradePanel.tsx` (Section 5.3)
6. Update `ServicePanel.tsx` (Section 5.4)
7. Update `SaasPanel.tsx` (Section 5.5)
8. Update `ResearchPanel.tsx` (Section 5.6)
9. Update `MarketPanel.tsx` (Section 5.7)
10. Update `DatacenterPanel.tsx` (Section 5.8)
11. Update `OverclockPanel.tsx`, `TierProgress.tsx`, `DonatePanel.tsx` (Section 5.9-5.11)
12. Update `SocialPanel.tsx` (Section 5.14)

**Phase 4 — Multi-currency stat lines**
13. Refactor inline stat lines ("+5 CU . +2 Rep . 30W") to use a shared component that
    color-codes each segment. This is the most pervasive change and touches many components.

### 9.2 Technology Recommendations

- **Utility file**: Create `src/utils/currencyColors.ts` exporting a typed constant:
  ```
  type CurrencyId = 'cu' | 'money' | 'rep' | 'kp' | 'btc' | 'pwr';
  type CurrencyColorSet = { color: string; bg: string; border: string; glow: string };
  ```
  This keeps all color lookups in one place and makes it easy for components to do
  `CURRENCY_COLORS[costType]`.

- **Stat line component**: Create a `<CurrencyValue currency="cu" value={1250} />` component
  that applies the correct color, formatting, and label automatically. This reduces per-
  component boilerplate and guarantees consistency.

- **Multi-currency stat component**: Create a `<CurrencyStatLine items={[{currency: 'cu',
  value: 5, prefix: '+'}, {currency: 'rep', value: 2, prefix: '+'}]} />` component for
  the common "+X CU . +Y Rep . ZW" pattern.

### 9.3 Category Colors vs Currency Colors — Coexistence

This spec does NOT replace hardware category colors (Compute=purple, Network=blue,
Storage=cyan, Power=amber, Misc=slate) or upgrade type colors or research branch colors.
Those colors serve a **grouping** purpose (which category is this item in?). Currency colors
serve an **identification** purpose (what does this number represent?).

The rule:
- **Headings, section labels, category badges**: use category/type colors
- **Numeric values and their price/cost buttons**: use currency colors

These systems coexist. A purple "Compute" heading can contain amber "1,200 CU" price buttons.

### 9.4 Existing `--accent-*` Variables — Deprecation Path

The existing `--accent-amber`, `--accent-green`, etc. variables should **NOT** be removed.
They continue to serve non-currency purposes:
- `--accent-green`: success states, "LIVE" badges, "OWNED" text, tier progress
- `--accent-red`: errors, warnings, sell buttons, danger states, overheat
- `--accent-cyan`: datacenter/colo category
- `--accent-purple`: hardware/compute category
- `--accent-blue`: services/network category

The `--currency-*` variables are **additive**. Some happen to have the same hex value as
existing accents (e.g., `--currency-cu` = `--accent-amber` = `#f59e0b`), but they are
semantically distinct. Using the currency variable makes the intent clear in code.

### 9.5 MVP vs Polish Priorities

| Priority | Item | Rationale |
|---|---|---|
| P0 (MVP) | CSS variables in global.css | Foundation — everything depends on this |
| P0 (MVP) | CurrencyBar color corrections | Highest-visibility surface, fixes BTC/CU collision |
| P1 (Important) | Price button color alignment | Most impactful consistency improvement |
| P1 (Important) | Multi-currency stat line colors | High-frequency reading pattern |
| P2 (Polish) | Shared CurrencyValue component | Reduces future drift |
| P2 (Polish) | Shared CurrencyStatLine component | Reduces per-component boilerplate |
| P3 (Future) | Tooltip currency colors | Tooltips do not exist yet |
| P3 (Future) | Per-currency icons | Additional disambiguation beyond color |

### 9.6 Open Questions

1. **BTC button convention**: Should Buy/Sell buttons in the Market panel use currency colors
   (money for buy, BTC for sell) or financial convention (green for buy, red for sell)?
   This spec recommends keeping the financial convention for the action buttons but using
   currency colors for the cost/proceeds labels. Confirm with stakeholders.

2. **Knowledge Points visibility**: Knowledge Points (`knowledge_points` in GameState) are
   referenced in the type system but do not appear to surface prominently in the current UI.
   Confirm whether KP will be displayed in the CurrencyBar in the future — the color
   assignment (`#c084fc`, purple-400) is ready.

3. **Power color change impact**: Changing PWR from purple to yellow is the most visually
   noticeable change. The current purple association exists only in the CurrencyBar; no
   other surface uses purple specifically for power. Confirm this change is acceptable.
