import { CurrencyValue } from './CurrencyValue';
import type { CurrencyId } from '../../utils/currencyColors';

interface StatLineItem {
  currency: CurrencyId;
  value: number | string;
  prefix?: string;
  suffix?: string;
}

interface CurrencyStatLineProps {
  items: StatLineItem[];
  separator?: string;
  className?: string;
}

/**
 * Renders a multi-currency stat line with each value in its currency color,
 * separated by muted-colored separators.
 *
 * Example: "+5 CU · +2 Rep · 30W" where each segment uses its own currency color
 * and the "·" separators use --text-muted.
 *
 * Presentation-only — no state, no side effects.
 */
export function CurrencyStatLine({
  items,
  separator = ' \u00B7 ',
  className,
}: CurrencyStatLineProps) {
  return (
    <span className={className}>
      {items.map((item, i) => (
        <span key={i}>
          {i > 0 && <span style={{ color: 'var(--text-muted)' }}>{separator}</span>}
          <CurrencyValue
            currency={item.currency}
            value={item.value}
            prefix={item.prefix}
            suffix={item.suffix}
          />
        </span>
      ))}
    </span>
  );
}
