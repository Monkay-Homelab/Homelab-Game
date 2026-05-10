import { CURRENCY_COLORS, formatNumber } from '../../utils/currencyColors';
import type { CurrencyId } from '../../utils/currencyColors';

interface CurrencyValueProps {
  currency: CurrencyId;
  value: number | string;
  prefix?: string;
  suffix?: string;
  showLabel?: boolean;
  format?: boolean;
  rateStyle?: boolean;
  className?: string;
}

/** Human-readable label for each currency. */
const CURRENCY_LABELS: Record<CurrencyId, string> = {
  cu: 'CU',
  money: 'USD',
  rep: 'REP',
  kp: 'KP',
  btc: 'BTC',
  pwr: 'PWR',
};

/**
 * Renders a single currency value with the correct currency color.
 *
 * Presentation-only — no state, no side effects.
 */
export function CurrencyValue({
  currency,
  value,
  prefix,
  suffix,
  showLabel = false,
  format = true,
  rateStyle = false,
  className,
}: CurrencyValueProps) {
  const color = CURRENCY_COLORS[currency].color;

  const formattedValue =
    typeof value === 'string' ? value : format ? formatNumber(value) : String(value);

  const valueStyle: React.CSSProperties = {
    color,
    ...(rateStyle ? { opacity: 0.7 } : undefined),
  };

  if (showLabel) {
    return (
      <span className={className}>
        <span style={{ color: 'var(--text-muted)' }}>{CURRENCY_LABELS[currency]} </span>
        <span style={valueStyle}>
          {prefix}
          {formattedValue}
          {suffix}
        </span>
      </span>
    );
  }

  return (
    <span className={className} style={valueStyle}>
      {prefix}
      {formattedValue}
      {suffix}
    </span>
  );
}
