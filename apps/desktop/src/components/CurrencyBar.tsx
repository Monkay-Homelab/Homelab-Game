import type { GameState } from '../api';
import { useIdleTick } from '../hooks/useIdleTick';
import { useGameStore } from '../stores/gameStore';

const TIER_LABELS: Record<string, string> = {
  coffee_table: 'Coffee Table',
  closet_floor: 'Closet Floor',
  rack_12u: '12U Rack',
  rack_24u: '24U Rack',
  rack_36u: '36U Rack',
  rack_48u: '48U Rack',
};

export function CurrencyBar({ state }: { state: GameState }) {
  const isRack = state.rack_units !== null;
  const currencies = useIdleTick(state);
  const resolveEvent = useGameStore(s => s.resolveEvent);
  const throttled = state.throttled || false;
  const throttleTicks = state.throttle_ticks_remaining || 0;

  return (
    <div className="panel px-4 py-3 flex flex-wrap items-center gap-x-6 gap-y-2">
      <div className="font-mono text-xs px-2 py-1 rounded font-semibold" style={{ background: 'rgba(34,197,94,0.1)', color: 'var(--accent-green)', border: '1px solid rgba(34,197,94,0.2)' }}>
        {TIER_LABELS[state.tier] || state.tier}
      </div>

      <Stat label="CU" value={formatNumber(Math.floor(currencies.computeUnits))} rate={currencies.computePerSecond > 0 ? `+${formatNumber(currencies.computePerSecond)}/s` : undefined} color="var(--accent-amber)" />
      <Stat label="REP" value={formatNumber(Math.floor(currencies.reputation))} color="var(--accent-blue)" />
      <Stat label="PWR" value={`${state.power_watts}/${state.power_limit}W`} color="var(--accent-purple)" />
      {currencies.money > 0 && <Stat label="$" value={formatNumber(Math.floor(currencies.money))} color="var(--accent-green)" />}
      <Stat
        label={isRack ? 'RACK' : 'SLOTS'}
        value={isRack ? `${state.used_rack_units}/${state.rack_units}U` : `${state.used_slots}/${state.hardware_slots}`}
        color="var(--accent-cyan)"
      />
      {isRack && (() => {
        const shelves = (state.hardware || []).filter(h => h.type === 'shelf').length;
        if (shelves === 0) return null;
        const totalSlots = shelves * 4;
        const usedSlots = (state.hardware || []).filter(h => h.rack_units_used === null && h.slots_used > 0).reduce((s, h) => s + h.slots_used, 0);
        return <Stat label="SHELF" value={`${usedSlots}/${totalSlots}`} color="var(--accent-cyan)" />;
      })()}
      {state.colo_count > 0 && <Stat label="COLO" value={`${state.colo_count} (${state.colo_multiplier.toFixed(1)}x)`} color="var(--accent-cyan)" />}
      {state.group_bonus > 1 && <Stat label="GROUP" value={`${state.group_members} (${((state.group_bonus - 1) * 100).toFixed(0)}% bonus)`} color="#22c55e" />}

      {throttled && (
        <button
          onClick={resolveEvent}
          className="btn px-3 py-1 text-xs animate-gentle-pulse"
          style={{ background: 'rgba(239,68,68,0.15)', color: 'var(--accent-red)', border: '1px solid rgba(239,68,68,0.3)' }}
        >
          THROTTLED ({throttleTicks}) — Fix {throttleTicks * 100} CU
        </button>
      )}

      {state.overheating && (
        <span className="font-mono text-xs animate-gentle-pulse" style={{ color: 'var(--accent-red)' }}>
          OVERHEAT {state.heat_generated}/{state.cooling_capacity}
        </span>
      )}
    </div>
  );
}

function Stat({ label, value, rate, color }: { label: string; value: string; rate?: string; color: string }) {
  return (
    <div className="flex items-baseline gap-1.5">
      <span className="font-mono text-xs uppercase" style={{ color: 'var(--text-muted)' }}>{label}</span>
      <span className="stat-value text-sm" style={{ color }}>{value}</span>
      {rate && <span className="font-mono text-xs" style={{ color: 'var(--text-secondary)' }}>{rate}</span>}
    </div>
  );
}

function formatNumber(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
  return n.toString();
}
