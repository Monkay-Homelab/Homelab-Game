/**
 * Currency color system — typed interface for CSS custom properties.
 *
 * Maps currency identifiers to their CSS variable references so components
 * can apply consistent colors without raw hex values or --accent-* references.
 *
 * The CSS variables in global.css are the single source of truth for color
 * values; this module provides type-safe access to those variable names.
 */

/** All currency identifiers in the game. */
export type CurrencyId = 'cu' | 'money' | 'rep' | 'kp' | 'btc' | 'pwr';

/** CSS variable references for one currency's color set. */
export type CurrencyColorSet = {
  /** CSS var() reference for text color, e.g., 'var(--currency-cu)' */
  color: string;
  /** CSS var() reference for background tint */
  bg: string;
  /** CSS var() reference for border */
  border: string;
  /** CSS box-shadow value for glow */
  glow: string;
};

/**
 * Currency color lookup table.
 *
 * Each entry maps a CurrencyId to its four CSS variable references.
 * Apply via inline styles: `style={{ color: CURRENCY_COLORS.cu.color }}`
 */
export const CURRENCY_COLORS: Record<CurrencyId, CurrencyColorSet> = {
  cu: {
    color: 'var(--currency-cu)',
    bg: 'var(--currency-cu-bg)',
    border: 'var(--currency-cu-border)',
    glow: 'var(--currency-cu-glow)',
  },
  money: {
    color: 'var(--currency-money)',
    bg: 'var(--currency-money-bg)',
    border: 'var(--currency-money-border)',
    glow: 'var(--currency-money-glow)',
  },
  rep: {
    color: 'var(--currency-rep)',
    bg: 'var(--currency-rep-bg)',
    border: 'var(--currency-rep-border)',
    glow: 'var(--currency-rep-glow)',
  },
  kp: {
    color: 'var(--currency-kp)',
    bg: 'var(--currency-kp-bg)',
    border: 'var(--currency-kp-border)',
    glow: 'var(--currency-kp-glow)',
  },
  btc: {
    color: 'var(--currency-btc)',
    bg: 'var(--currency-btc-bg)',
    border: 'var(--currency-btc-border)',
    glow: 'var(--currency-btc-glow)',
  },
  pwr: {
    color: 'var(--currency-pwr)',
    bg: 'var(--currency-pwr-bg)',
    border: 'var(--currency-pwr-border)',
    glow: 'var(--currency-pwr-glow)',
  },
};

/** Set of valid CurrencyId values for runtime validation. */
const VALID_CURRENCY_IDS: ReadonlySet<string> = new Set<CurrencyId>([
  'cu',
  'money',
  'rep',
  'kp',
  'btc',
  'pwr',
]);

/**
 * Look up the color set for a currency.
 *
 * Provides the same data as `CURRENCY_COLORS[id]` but with a function
 * signature that enforces the CurrencyId type constraint.
 */
export function getCurrencyColor(id: CurrencyId): CurrencyColorSet {
  return CURRENCY_COLORS[id];
}

/**
 * Map an API cost_type string to a CurrencyId.
 *
 * The backend sends cost_type as 'money', 'compute', etc.
 * Defaults to 'cu' for unknown values, matching the backend's default
 * behavior where unspecified cost types are treated as CU.
 */
export function getCostCurrencyId(costType: string): CurrencyId {
  if (costType === 'money') return 'money';
  if (VALID_CURRENCY_IDS.has(costType)) return costType as CurrencyId;
  // 'compute' from the backend maps to 'cu'
  return 'cu';
}

/**
 * Format a number with abbreviated suffixes for large values.
 *
 * Thresholds: T (1e12), B (1e9), M (1e6), K (1e3).
 * Extracted from ResearchPanel.tsx (the most complete version across
 * the codebase, which handles all four tiers).
 */
export function formatNumber(n: number): string {
  if (n >= 1_000_000_000_000) return (n / 1_000_000_000_000).toFixed(1) + 'T';
  if (n >= 1_000_000_000) return (n / 1_000_000_000).toFixed(1) + 'B';
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
  return n.toString();
}
