import type { GameState } from '../api';
import { useGameStore } from '../stores/gameStore';
import { useConfig } from '../hooks/useConfig';

function formatNumber(n: number): string {
  if (n >= 1_000_000_000) return (n / 1_000_000_000).toFixed(1) + 'B';
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
  return n.toString();
}

export function DatacenterPanel({ state }: { state: GameState }) {
  const config = useConfig();
  const dc = config.datacenter;
  const colo = useGameStore(s => s.colo);
  const buildDatacenter = useGameStore(s => s.buildDatacenter);
  const upgradeDatacenter = useGameStore(s => s.upgradeDatacenter);
  const optimizeRack = useGameStore(s => s.optimizeRack);
  const coloRacks = state.colo_racks || [];
  const maxTier = config.tiers[config.tiers.length - 1];
  const canColo = state.tier === maxTier.id && state.saas_unlocked;

  const totalColoCompute = coloRacks.reduce((s, r) => s + r.compute_per_tick, 0);
  const totalColoRep = coloRacks.reduce((s, r) => s + r.reputation_per_tick, 0);
  const totalColoMoney = coloRacks.reduce((s, r) => s + r.money_per_tick, 0);
  const dcMult = state.datacenter_income_multiplier || 1;

  const buildMoneyStr = (dc.build_money_cost / 1000).toLocaleString() + 'K';
  const buildComputeStr = (dc.build_compute_cost / 1000000).toLocaleString() + 'M';

  // Rack optimization calculations
  const rackOptConfig = config.rack_optimization;
  const optLevel = state.rack_optimization || 0;
  const bonusPercent = optLevel * rackOptConfig.bonus_per_level * 100;
  const optCost = optLevel >= 46 ? Infinity : rackOptConfig.base_cost * Math.pow(2, optLevel);
  const canAffordOptimize = state.compute_units >= optCost && optCost !== Infinity;

  // Snapshot preview: what the colo rack income would look like with current optimization
  const snapshotCompute = (state.hardware || []).reduce((sum, h) => {
    let compute = h.compute_per_tick;
    for (const cu of (state.component_upgrades || [])) {
      if (cu.hardware_id === h.id) compute += Math.floor(h.compute_per_tick * cu.compute_bonus / 100);
    }
    return sum + compute;
  }, 0) + (state.services || []).reduce((sum, s) => sum + s.compute_per_tick, 0);
  const snapshotRep = (state.services || []).reduce((sum, s) => sum + s.reputation_per_tick, 0);
  const snapshotMoney = (state.services || []).reduce((sum, s) => sum + s.money_per_tick, 0);

  const bonus = 1.0 + optLevel * rackOptConfig.bonus_per_level;
  const previewCompute = Math.floor(snapshotCompute * bonus);
  const previewRep = Math.floor(snapshotRep * bonus);
  const previewMoney = Math.floor(snapshotMoney * bonus);

  return (
    <div className="h-full flex gap-4 min-h-0">
      {/* Actions */}
      <div className="w-80 shrink-0 panel p-4 flex flex-col min-h-0">
        <h3 className="text-sm font-semibold mb-3 shrink-0" style={{ color: 'var(--accent-cyan)' }}>Actions</h3>

        {state.owns_datacenter && (
          <div className="panel-card p-3 mb-3" style={{ borderColor: 'rgba(6,182,212,0.3)' }}>
            <div className="font-medium text-sm" style={{ color: 'var(--accent-cyan)' }}>
              Your Datacenter — {dc.level_names[state.datacenter_level] || `Level ${state.datacenter_level}`}
            </div>
            <div className="font-mono text-xs mt-1" style={{ color: 'var(--text-secondary)' }}>{dcMult.toFixed(2)}x income on colo racks</div>
            {state.datacenter_level < dc.max_level && (
              <button
                onClick={upgradeDatacenter}
                className="btn w-full mt-2 py-1.5 text-xs"
                style={{ background: 'rgba(6,182,212,0.1)', color: 'var(--accent-cyan)', border: '1px solid rgba(6,182,212,0.2)' }}
              >
                Upgrade — ${(dc.upgrade_money_base * (state.datacenter_level + 1)).toLocaleString()} + {(dc.upgrade_compute_base * (state.datacenter_level + 1)).toLocaleString()} CU
              </button>
            )}
          </div>
        )}

        {canColo && (
          <div className="panel-card p-3 mb-3" style={{ borderColor: 'rgba(6,182,212,0.3)' }}>
            <div className="font-medium text-sm" style={{ color: 'var(--accent-cyan)' }}>
              Rack Optimization: Level {optLevel} (+{bonusPercent.toFixed(0)}%)
            </div>
            <div className="font-mono text-xs mt-1" style={{ color: 'var(--text-secondary)' }}>
              {optCost === Infinity ? 'Maximum level reached' : `Next: ${formatNumber(optCost)} CU`}
            </div>
            {optCost !== Infinity && (
              <button
                onClick={optimizeRack}
                disabled={!canAffordOptimize}
                className="btn w-full mt-2 py-1.5 text-xs"
                style={{
                  background: canAffordOptimize ? 'rgba(6,182,212,0.1)' : 'rgba(6,182,212,0.05)',
                  color: canAffordOptimize ? 'var(--accent-cyan)' : 'var(--text-muted)',
                  border: '1px solid rgba(6,182,212,0.2)',
                }}
              >
                Optimize Rack — {formatNumber(optCost)} CU
              </button>
            )}
            <div className="mt-2 pt-2" style={{ borderTop: '1px solid rgba(6,182,212,0.1)' }}>
              <div className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>
                Colo rack preview with bonus:
              </div>
              <div className="font-mono text-xs mt-0.5" style={{ color: 'var(--text-secondary)' }}>
                +{formatNumber(previewCompute)} CU · +{formatNumber(previewRep)} Rep · +${formatNumber(previewMoney)}/tick
              </div>
              <div className="font-mono text-xs mt-0.5 italic" style={{ color: 'var(--text-muted)' }}>
                Preview is approximate — server is authoritative
              </div>
            </div>
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

          {!state.owns_datacenter && state.colo_count >= dc.min_colo_count && (
            <button
              onClick={buildDatacenter}
              className="btn w-full py-3 text-sm"
              style={{ background: 'rgba(245,158,11,0.1)', color: 'var(--accent-amber)', border: '1px solid rgba(245,158,11,0.3)' }}
            >
              Build Datacenter — ${buildMoneyStr} + {buildComputeStr} CU
            </button>
          )}

          {!canColo && (
            <p className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>
              {state.tier !== maxTier.id ? `Reach ${maxTier.label} to colo` : 'Unlock SaaS to colo'}
              {state.colo_count < dc.min_colo_count && `. ${dc.min_colo_count - state.colo_count} more colos for datacenter.`}
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
              <div className="font-mono text-xs mt-0.5" style={{ color: 'var(--accent-cyan)' }}>{dc.tier_names[r.datacenter_tier] || `Tier ${r.datacenter_tier}`}</div>
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
