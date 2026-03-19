import type { GameState } from '../api';
import { useGameStore } from '../stores/gameStore';

const DC_TIER_NAMES: Record<number, string> = {
  0: 'None', 1: 'Tier 1 — Basic', 2: 'Tier 2 — Redundant Power',
  3: 'Tier 3 — Concurrently Maintainable', 4: 'Tier 4 — Fault Tolerant',
};

const DC_LEVEL_NAMES: Record<number, string> = {
  1: 'Small Facility', 2: 'Medium Facility', 3: 'Large Facility', 4: 'Campus', 5: 'Hyperscale',
};

export function DatacenterPanel({ state }: { state: GameState }) {
  const colo = useGameStore(s => s.colo);
  const buildDatacenter = useGameStore(s => s.buildDatacenter);
  const upgradeDatacenter = useGameStore(s => s.upgradeDatacenter);
  const coloRacks = state.colo_racks || [];
  const canColo = state.tier === 'rack_48u' && state.saas_unlocked;

  const totalColoCompute = coloRacks.reduce((s, r) => s + r.compute_per_tick, 0);
  const totalColoRep = coloRacks.reduce((s, r) => s + r.reputation_per_tick, 0);
  const totalColoMoney = coloRacks.reduce((s, r) => s + r.money_per_tick, 0);
  const dcMult = state.datacenter_income_multiplier || 1;

  return (
    <div className="h-full flex gap-4 min-h-0">
      {/* Actions */}
      <div className="w-80 shrink-0 panel p-4 flex flex-col min-h-0">
        <h3 className="text-sm font-semibold mb-3 shrink-0" style={{ color: 'var(--accent-cyan)' }}>Actions</h3>

        {state.owns_datacenter && (
          <div className="panel-card p-3 mb-3" style={{ borderColor: 'rgba(6,182,212,0.3)' }}>
            <div className="font-medium text-sm" style={{ color: 'var(--accent-cyan)' }}>
              Your Datacenter — {DC_LEVEL_NAMES[state.datacenter_level]}
            </div>
            <div className="font-mono text-xs mt-1" style={{ color: 'var(--text-secondary)' }}>{dcMult.toFixed(2)}x income on colo racks</div>
            {state.datacenter_level < 5 && (
              <button
                onClick={upgradeDatacenter}
                className="btn w-full mt-2 py-1.5 text-xs"
                style={{ background: 'rgba(6,182,212,0.1)', color: 'var(--accent-cyan)', border: '1px solid rgba(6,182,212,0.2)' }}
              >
                Upgrade — ${(500000 * (state.datacenter_level + 1)).toLocaleString()} + {(2000000 * (state.datacenter_level + 1)).toLocaleString()} CU
              </button>
            )}
          </div>
        )}

        <div className="space-y-3 flex-1">
          {canColo && (
            <button
              onClick={colo}
              className="btn w-full py-3 text-sm"
              style={{ background: 'rgba(6,182,212,0.1)', color: 'var(--accent-cyan)', border: '1px solid rgba(6,182,212,0.3)' }}
            >
              Colocate Rack (Prestige)
            </button>
          )}

          {!state.owns_datacenter && state.colo_count >= 5 && (
            <button
              onClick={buildDatacenter}
              className="btn w-full py-3 text-sm"
              style={{ background: 'rgba(245,158,11,0.1)', color: 'var(--accent-amber)', border: '1px solid rgba(245,158,11,0.3)' }}
            >
              Build Datacenter — $1M + 5M CU
            </button>
          )}

          {!canColo && (
            <p className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>
              {state.tier !== 'rack_48u' ? 'Reach 48U to colo' : 'Unlock SaaS to colo'}
              {state.colo_count < 5 && `. ${5 - state.colo_count} more colos for datacenter.`}
            </p>
          )}
        </div>
      </div>

      {/* Colo'd Racks */}
      <div className="flex-1 panel p-4 flex flex-col min-h-0">
        <div className="flex justify-between items-center mb-3 shrink-0">
          <h3 className="text-sm font-semibold" style={{ color: 'var(--accent-cyan)' }}>Colo'd Racks</h3>
          <span className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>{state.colo_multiplier.toFixed(2)}x mult</span>
        </div>
        {coloRacks.length > 0 && (
          <div className="font-mono text-xs mb-3 shrink-0" style={{ color: 'var(--text-secondary)' }}>
            +{totalColoCompute} CU · +{totalColoRep} Rep · +${totalColoMoney}/tick passive
          </div>
        )}
        <div className="space-y-2 overflow-y-auto min-h-0 flex-1">
          {coloRacks.length > 0 ? coloRacks.map(r => (
            <div key={r.id} className="panel-card p-3">
              <div className="font-medium text-sm">{r.rack_size}U Rack</div>
              <div className="font-mono text-xs mt-0.5" style={{ color: 'var(--accent-cyan)' }}>{DC_TIER_NAMES[r.datacenter_tier]}</div>
              <div className="font-mono text-xs mt-0.5" style={{ color: 'var(--text-secondary)' }}>
                +{r.compute_per_tick} CU · +{r.reputation_per_tick} Rep · +${r.money_per_tick}/tick
              </div>
            </div>
          )) : (
            <p className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>No racks colocated yet</p>
          )}
        </div>
      </div>
    </div>
  );
}
