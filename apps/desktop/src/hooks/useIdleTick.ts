import { useEffect, useRef, useState } from 'react';
import type { GameState, GameConfig } from '../api';

interface InterpolatedCurrencies {
  computeUnits: number;
  reputation: number;
  money: number;
  computePerSecond: number;
}

export function useIdleTick(state: GameState | null, config: GameConfig | null): InterpolatedCurrencies {
  const [currencies, setCurrencies] = useState<InterpolatedCurrencies>({
    computeUnits: 0,
    reputation: 0,
    money: 0,
    computePerSecond: 0,
  });
  const rafId = useRef<number>(0);
  const serverBase = useRef({ compute: 0, reputation: 0, money: 0 });
  const serverTime = useRef(Date.now());
  const rates = useRef({ compute: 0, reputation: 0, money: 0 });
  const initialized = useRef(false);

  // Update refs when server state arrives — no rAF restart needed
  useEffect(() => {
    if (!state || !config) return;

    const hw = config.hardware_bonuses;
    const gp = config.gameplay;

    // Always trust the server value — no client-side prediction drift
    serverBase.current = {
      compute: state.compute_units,
      reputation: state.reputation,
      money: state.money,
    };
    serverTime.current = Date.now();

    // === Rate calculation matching server engine exactly ===

    // Hardware compute (with component upgrades) + UPS bonus + network/storage/patch bonuses
    let hardwareCompute = 0;
    let upsCompute = 0;
    let networkBonus = 0;
    let storageBonus = 0;
    let patchPanelBonus = 0;

    if (state.hardware) {
      const compUps = state.component_upgrades || [];
      for (const h of state.hardware) {
        let bonus = 0;
        for (const cu of compUps) {
          if (cu.hardware_id === h.id) bonus += Math.floor(h.compute_per_tick * cu.compute_bonus / 100);
        }
        hardwareCompute += h.compute_per_tick + bonus;

        if (hw.ups_compute[h.name]) upsCompute += hw.ups_compute[h.name];
        if (hw.network_income[h.name]) networkBonus += hw.network_income[h.name];
        if (hw.storage_rep[h.name]) storageBonus += hw.storage_rep[h.name];
        if (h.type === 'patch_panel') patchPanelBonus += hw.patch_panel_bonus;
      }
    }
    // Network and storage bonuses stack with no cap
    hardwareCompute += upsCompute;

    // Service compute/rep/money
    let serviceCompute = 0;
    let serviceRep = 0;
    let serviceMoney = 0;
    if (state.services) {
      for (const s of state.services) {
        serviceCompute += s.compute_per_tick;
        serviceRep += s.reputation_per_tick;
        serviceMoney += s.money_per_tick;
      }
    }

    const totalCompute = hardwareCompute + serviceCompute;

    // Multipliers matching server's ProcessIdleProgress (defensive defaults for missing fields)
    const heatPenalty = state.overheating ? gp.heat_penalty : 1.0;
    const throttle = state.throttled ? (state.throttle_multiplier || 0) : 1.0;
    const knowledgeBoost = 1.0 + (state.knowledge_points || 0) / gp.knowledge_boost_divisor;
    const netMult = 1.0 + networkBonus;
    const repMult = 1.0 + storageBonus + patchPanelBonus;
    const coloMult = state.colo_multiplier || 1.0;
    const idleMult = state.idle_multiplier || 1.0;
    // Overclock multiplier: defensive check ensures it never reduces income (matches server guard)
    const overclockMult = (state.overclocked && state.overclock_multiplier > 1)
      ? state.overclock_multiplier
      : 1.0;
    const baseMultiplier = coloMult * idleMult * heatPenalty * throttle * overclockMult;

    // Research bonuses: aggregate by effect type (additive within type, multiplicative across)
    let researchIdleMult = 1.0;
    let researchRepMult = 1.0;
    let researchMoneyMult = 1.0;
    if (state.research_levels && config.research?.nodes) {
      for (const rl of state.research_levels) {
        const node = config.research.nodes.find(n => n.id === rl.research_node);
        if (node) {
          const bonus = rl.level * node.effect_value;
          if (node.effect_type === 'idle_income') researchIdleMult += bonus;
          else if (node.effect_type === 'reputation_gain') researchRepMult += bonus;
          else if (node.effect_type === 'money_income') researchMoneyMult += bonus;
        }
      }
    }

    // Base compute rate (hw + svc with all multipliers)
    const baseComputeRate = totalCompute * baseMultiplier * knowledgeBoost * netMult * researchIdleMult;

    // Colo rack income — server only applies datacenterIncomeMultiplier, NOT the other multipliers
    let coloComputeRate = 0;
    let coloRepRate = 0;
    let coloMoneyRate = 0;
    const dcMult = Math.max(state.datacenter_income_multiplier || 0, 1.0);
    if (state.colo_racks) {
      for (let i = 0; i < state.colo_racks.length; i++) {
        const cr = state.colo_racks[i];
        const decay = Math.pow(gp.colo_rack_decay, i);
        coloComputeRate += cr.compute_per_tick * dcMult * decay;
        coloRepRate += cr.reputation_per_tick * dcMult * decay;
        coloMoneyRate += cr.money_per_tick * dcMult * decay;
      }
    }

    // Group bonus — server applies additively on raw hw+svc compute, NOT multiplicatively
    let groupComputeRate = 0;
    if ((state.group_bonus || 0) > 1) {
      let rawHwCompute = 0;
      if (state.hardware) {
        for (const h of state.hardware) {
          rawHwCompute += h.compute_per_tick;
        }
      }
      groupComputeRate = (rawHwCompute + serviceCompute) * ((state.group_bonus || 1) - 1.0);
    }

    // Total rates
    const computeRate = baseComputeRate + coloComputeRate + groupComputeRate;
    const repRate = serviceRep * heatPenalty * throttle * repMult * researchRepMult + coloRepRate;

    // Money: service income minus expenses
    let totalExpenses = 0;
    if (state.expenses) {
      for (const e of state.expenses) {
        totalExpenses += e.cost_per_tick;
      }
    }
    const moneyRate = serviceMoney * heatPenalty * throttle * researchMoneyMult + coloMoneyRate - totalExpenses;

    // Guard against NaN propagation — fall back to 0 if any calculation produced NaN
    rates.current = {
      compute: isFinite(computeRate) ? computeRate : 0,
      reputation: isFinite(repRate) ? repRate : 0,
      money: isFinite(moneyRate) ? moneyRate : 0,
    };
    initialized.current = true;
  }, [state, config]);

  // Single rAF loop — starts once, never restarts, reads from refs
  useEffect(() => {
    const tick = () => {
      if (initialized.current) {
        const elapsed = (Date.now() - serverTime.current) / 1000;
        const r = rates.current;
        const base = serverBase.current;

        setCurrencies({
          computeUnits: base.compute + r.compute * elapsed,
          reputation: base.reputation + r.reputation * elapsed,
          money: base.money + r.money * elapsed,
          computePerSecond: Math.floor(r.compute),
        });
      }

      rafId.current = requestAnimationFrame(tick);
    };

    rafId.current = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(rafId.current);
  }, []);

  return currencies;
}
