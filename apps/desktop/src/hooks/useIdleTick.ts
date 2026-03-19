import { useEffect, useRef, useState } from 'react';
import type { GameState } from '../api';

interface InterpolatedCurrencies {
  computeUnits: number;
  reputation: number;
  money: number;
  computePerSecond: number;
}

export function useIdleTick(state: GameState | null): InterpolatedCurrencies {
  const [currencies, setCurrencies] = useState<InterpolatedCurrencies>({
    computeUnits: 0,
    reputation: 0,
    money: 0,
    computePerSecond: 0,
  });
  const lastFrameTime = useRef(Date.now());
  const rafId = useRef<number>(0);

  const rates = useRef({ compute: 0, reputation: 0, money: 0, multiplier: 1 });

  useEffect(() => {
    if (!state) return;

    setCurrencies(prev => ({
      ...prev,
      computeUnits: state.compute_units,
      reputation: state.reputation,
      money: state.money,
    }));
    lastFrameTime.current = Date.now();

    // Recalculate rates from hardware + services + colo racks
    let computeRate = 0;
    let repRate = 0;
    let moneyRate = 0;

    if (state.hardware) {
      for (const h of state.hardware) {
        computeRate += h.compute_per_tick;
      }
    }
    if (state.services) {
      for (const s of state.services) {
        computeRate += s.compute_per_tick;
        repRate += s.reputation_per_tick;
        moneyRate += s.money_per_tick;
      }
    }
    if (state.colo_racks) {
      for (const r of state.colo_racks) {
        computeRate += r.compute_per_tick;
        repRate += r.reputation_per_tick;
        moneyRate += r.money_per_tick;
      }
    }

    const multiplier = state.colo_multiplier * state.idle_multiplier *
      (state.overheating ? 0.5 : 1.0) *
      (state.throttled ? state.throttle_multiplier : 1.0) *
      (state.group_bonus > 1 ? state.group_bonus : 1.0);

    rates.current = { compute: computeRate, reputation: repRate, money: moneyRate, multiplier };

    setCurrencies(prev => ({
      ...prev,
      computePerSecond: Math.floor(computeRate * multiplier),
    }));
  }, [state]);

  useEffect(() => {
    if (!state) return;

    const tick = () => {
      const now = Date.now();
      const elapsed = (now - lastFrameTime.current) / 1000;
      lastFrameTime.current = now;

      const r = rates.current;
      if (r.compute > 0 || r.reputation > 0 || r.money > 0) {
        setCurrencies(prev => ({
          ...prev,
          computeUnits: prev.computeUnits + r.compute * elapsed * r.multiplier,
          reputation: prev.reputation + r.reputation * elapsed,
          money: prev.money + r.money * elapsed,
        }));
      }

      rafId.current = requestAnimationFrame(tick);
    };

    rafId.current = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(rafId.current);
  }, [state]);

  return currencies;
}
