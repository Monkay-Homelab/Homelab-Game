import { useEffect, useRef, useState } from 'react';
import type { GameState } from '../api';
import { useGameStore } from '../stores/gameStore';
import { useConfig } from '../hooks/useConfig';
import { CURRENCY_COLORS, formatNumber } from '../utils/currencyColors';

function formatTime(totalSeconds: number): string {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = Math.floor(totalSeconds % 60);
  return `${minutes}:${seconds.toString().padStart(2, '0')}`;
}

export function OverclockPanel({ state }: { state: GameState }) {
  const config = useConfig();
  const activateOverclock = useGameStore((s) => s.activateOverclock);
  const overclockConfig = config.overclock;
  const tickInterval = overclockConfig.tick_interval_seconds;

  // Client-side countdown timer that interpolates between server pushes.
  // displaySeconds is the interpolated countdown shown in the UI. It is
  // derived from server ticks (synced via refs) and a 250ms interval that
  // subtracts elapsed wall-clock time.
  const lastSyncTime = useRef(0);
  const lastSyncTicks = useRef(state.overclock_ticks_remaining);
  const [countdown, setCountdown] = useState(state.overclock_ticks_remaining * tickInterval);

  // Sync refs when server state changes (effect avoids impure render calls)
  useEffect(() => {
    lastSyncTime.current = Date.now();
    lastSyncTicks.current = state.overclock_ticks_remaining;
  }, [state.overclock_ticks_remaining]);

  // Derive the display value: use the interval-driven countdown when
  // active, otherwise compute directly from props (no effect needed).
  const displaySeconds = state.overclocked
    ? countdown
    : state.overclock_ticks_remaining * tickInterval;

  // Client-side countdown interpolation
  useEffect(() => {
    if (!state.overclocked) return;

    // Seed initial countdown value synchronously via ref reset
    lastSyncTime.current = Date.now();
    lastSyncTicks.current = state.overclock_ticks_remaining;

    const interval = setInterval(() => {
      const elapsedMs = Date.now() - lastSyncTime.current;
      const remainingSec = lastSyncTicks.current * tickInterval - elapsedMs / 1000;
      setCountdown(Math.max(0, remainingSec));
    }, 250);

    return () => clearInterval(interval);
  }, [state.overclocked, tickInterval, state.overclock_ticks_remaining]);

  const isActive = state.overclocked && state.overclock_ticks_remaining > 0;

  return (
    <div className="panel p-4">
      <div className="flex justify-between items-center mb-2">
        <span className="text-xs font-medium" style={{ color: 'var(--text-secondary)' }}>
          Overclock
        </span>
        {isActive && (
          <span
            className="font-mono text-xs px-2 py-0.5 rounded animate-gentle-pulse"
            style={{
              background: CURRENCY_COLORS.cu.bg,
              color: CURRENCY_COLORS.cu.color,
              border: `1px solid ${CURRENCY_COLORS.cu.border}`,
            }}
          >
            {state.overclock_multiplier}x ACTIVE
          </span>
        )}
      </div>

      {/* Active overclock countdown */}
      {isActive && (
        <div
          className="mb-3 p-2 rounded"
          style={{
            background: CURRENCY_COLORS.cu.bg,
            border: `1px solid ${CURRENCY_COLORS.cu.border}`,
          }}
        >
          <div className="flex justify-between items-center">
            <span className="font-mono text-xs" style={{ color: CURRENCY_COLORS.cu.color }}>
              {state.overclock_multiplier}x OVERCLOCK
            </span>
            <span className="font-mono text-xs" style={{ color: 'var(--text-secondary)' }}>
              {formatTime(displaySeconds)} remaining
            </span>
          </div>
          {/* Progress bar */}
          <div
            className="mt-1.5 h-1 rounded-full overflow-hidden"
            style={{ background: 'var(--bg-card)' }}
          >
            <div
              className="h-full rounded-full transition-all"
              style={{
                background: CURRENCY_COLORS.cu.color,
                width: `${(() => {
                  const tierCfg = overclockConfig.tiers.find(
                    (t) => t.multiplier === state.overclock_multiplier,
                  );
                  const totalDuration = (tierCfg?.duration || 60) * tickInterval;
                  return Math.max(0, (displaySeconds / totalDuration) * 100);
                })()}%`,
              }}
            />
          </div>
        </div>
      )}

      {/* Tier buttons */}
      <div className="space-y-1.5">
        {overclockConfig.tiers.map((tier) => {
          const canAfford = state.compute_units >= tier.cost;
          const duration = tier.duration * tickInterval;

          // Heat warning: estimate if overclock would push heat above cooling
          const estimatedHeat =
            state.heat_generated + state.heat_generated * (tier.multiplier - 1) * tier.heat_factor;
          const wouldOverheat = estimatedHeat > state.cooling_capacity && !state.overheating;

          return (
            <div key={tier.tier} className="panel-card p-2.5">
              <div className="flex items-center justify-between">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-sm">{tier.label}</span>
                    <span
                      className="font-mono text-xs px-1.5 py-0.5 rounded"
                      style={{ background: CURRENCY_COLORS.cu.bg, color: CURRENCY_COLORS.cu.color }}
                    >
                      {formatTime(duration)}
                    </span>
                  </div>
                  {wouldOverheat && (
                    <div className="font-mono text-xs mt-1" style={{ color: 'var(--accent-red)' }}>
                      Heat warning: {Math.floor(estimatedHeat)}W / {state.cooling_capacity}W cooling
                    </div>
                  )}
                </div>
                <button
                  onClick={() => activateOverclock(tier.tier)}
                  disabled={!canAfford}
                  className="btn px-3 py-1.5 text-xs shrink-0 ml-2"
                  style={{
                    background: canAfford ? CURRENCY_COLORS.cu.bg : 'var(--bg-card)',
                    color: canAfford ? CURRENCY_COLORS.cu.color : 'var(--text-muted)',
                    border: `1px solid ${canAfford ? CURRENCY_COLORS.cu.border : 'var(--border)'}`,
                  }}
                >
                  {formatNumber(tier.cost)} CU
                </button>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
