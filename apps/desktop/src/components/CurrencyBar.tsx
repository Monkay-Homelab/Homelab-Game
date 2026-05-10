import type { GameState } from '../api';
import { useIdleTick } from '../hooks/useIdleTick';
import { useConfig, tierLabel } from '../hooks/useConfig';
import { useGameStore } from '../stores/gameStore';
import { CURRENCY_COLORS, formatNumber } from '../utils/currencyColors';

export function CurrencyBar({ state }: { state: GameState }) {
  const config = useConfig();
  const isRack = state.rack_units !== null;
  const currencies = useIdleTick(state, config);
  const resolveEvent = useGameStore((s) => s.resolveEvent);
  const throttled = state.throttled || false;
  const throttleTicks = state.throttle_ticks_remaining || 0;

  return (
    <div className="panel px-4 py-3 flex flex-wrap items-center gap-x-6 gap-y-2">
      <div
        className="font-mono text-xs px-2 py-1 rounded font-semibold"
        style={{
          background: 'rgba(34,197,94,0.1)',
          color: 'var(--accent-green)',
          border: '1px solid rgba(34,197,94,0.2)',
        }}
      >
        {tierLabel(config, state.tier)}
      </div>

      <Stat
        label="CU"
        value={formatNumber(Math.floor(currencies.computeUnits))}
        rate={
          currencies.computePerSecond > 0
            ? `+${formatNumber(currencies.computePerSecond)}/s`
            : undefined
        }
        color={CURRENCY_COLORS.cu.color}
        rateColor={CURRENCY_COLORS.cu.color}
      />
      <Stat
        label="REP"
        value={formatNumber(Math.floor(currencies.reputation))}
        color={CURRENCY_COLORS.rep.color}
      />
      <Stat
        label="PWR"
        value={`${state.power_watts}/${state.power_limit}W`}
        color={CURRENCY_COLORS.pwr.color}
      />
      {currencies.money > 0 && (
        <Stat
          label="USD"
          value={`$${formatNumber(Math.floor(currencies.money))}`}
          color={CURRENCY_COLORS.money.color}
        />
      )}
      {state.bitcoin_balance > 0 && (
        <Stat
          label="BTC"
          value={`${state.bitcoin_balance} ($${formatNumber(state.bitcoin_balance * state.bitcoin_price)})`}
          color={CURRENCY_COLORS.btc.color}
        />
      )}
      <Stat
        label={isRack ? 'RACK' : 'SLOTS'}
        value={
          isRack
            ? `${state.used_rack_units}/${state.rack_units}U`
            : `${state.used_slots}/${state.hardware_slots}`
        }
        color="var(--accent-cyan)"
      />
      {isRack &&
        (() => {
          const shelves = (state.hardware || []).filter((h) => h.type === 'shelf').length;
          if (shelves === 0) return null;
          const totalSlots = shelves * config.gameplay.shelf_slots;
          const usedSlots = (state.hardware || [])
            .filter((h) => h.rack_units_used === null && h.slots_used > 0)
            .reduce((s, h) => s + h.slots_used, 0);
          return (
            <Stat label="SHELF" value={`${usedSlots}/${totalSlots}`} color="var(--accent-cyan)" />
          );
        })()}
      {state.colo_count > 0 && (
        <Stat
          label="COLO"
          value={`${state.colo_count} (${state.colo_multiplier.toFixed(1)}x)`}
          color="var(--accent-cyan)"
        />
      )}
      {state.group_bonus > 1 && (
        <Stat
          label="GROUP"
          value={`${state.group_members} (${((state.group_bonus - 1) * 100).toFixed(0)}% bonus)`}
          color="#22c55e"
        />
      )}

      {throttled && (
        <button
          onClick={resolveEvent}
          className="btn px-3 py-1 text-xs animate-gentle-pulse"
          style={{
            background: 'rgba(239,68,68,0.15)',
            color: 'var(--accent-red)',
            border: '1px solid rgba(239,68,68,0.3)',
          }}
        >
          THROTTLED ({throttleTicks}) — Fix{' '}
          {throttleTicks * config.gameplay.throttle_resolve_cost_per_tick} CU
        </button>
      )}

      {state.overheating && (
        <span
          className="font-mono text-xs animate-gentle-pulse"
          style={{ color: 'var(--accent-red)' }}
        >
          OVERHEAT {state.heat_generated}/{state.cooling_capacity}
        </span>
      )}
    </div>
  );
}

function Stat({
  label,
  value,
  rate,
  color,
  rateColor,
}: {
  label: string;
  value: string;
  rate?: string;
  color: string;
  rateColor?: string;
}) {
  return (
    <div className="flex items-baseline gap-1.5">
      <span className="font-mono text-xs uppercase" style={{ color: 'var(--text-muted)' }}>
        {label}
      </span>
      <span className="stat-value text-sm" style={{ color }}>
        {value}
      </span>
      {rate && (
        <span
          className="font-mono text-xs"
          style={{
            color: rateColor || 'var(--text-secondary)',
            opacity: rateColor ? 0.7 : undefined,
          }}
        >
          {rate}
        </span>
      )}
    </div>
  );
}
