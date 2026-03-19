import { useGameStore } from '../stores/gameStore';

const TIER_ORDER = ['coffee_table', 'closet_floor', 'rack_12u', 'rack_24u', 'rack_36u', 'rack_48u'];
const TIER_LABELS: Record<string, string> = {
  coffee_table: 'Coffee Table',
  closet_floor: 'Closet Floor',
  rack_12u: '12U Rack',
  rack_24u: '24U Rack',
  rack_36u: '36U Rack',
  rack_48u: '48U Rack',
};
const UPGRADE_COSTS: Record<string, number> = {
  coffee_table: 500,
  closet_floor: 5000,
  rack_12u: 25000,
  rack_24u: 100000,
  rack_36u: 500000,
  rack_48u: 0,
};

export function TierProgress({ tier, computeUnits }: { tier: string; computeUnits: number }) {
  const upgradeTier = useGameStore(s => s.upgradeTier);
  const currentIdx = TIER_ORDER.indexOf(tier);
  const isMaxTier = currentIdx >= TIER_ORDER.length - 1;
  const upgradeCost = UPGRADE_COSTS[tier] || 0;
  const canUpgrade = !isMaxTier && computeUnits >= upgradeCost;
  const nextTier = isMaxTier ? null : TIER_LABELS[TIER_ORDER[currentIdx + 1]];

  return (
    <div className="panel p-4">
      <div className="flex justify-between items-center mb-3">
        <span className="text-xs font-medium" style={{ color: 'var(--text-secondary)' }}>Progression</span>
        <span className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>{currentIdx + 1}/{TIER_ORDER.length}</span>
      </div>

      <div className="flex gap-1 mb-3">
        {TIER_ORDER.map((t, i) => (
          <div
            key={t}
            className="flex-1 h-1.5 rounded-full transition-all"
            style={{
              background: i <= currentIdx ? 'var(--accent-green)' : 'var(--border)',
              opacity: i <= currentIdx ? 1 : 0.5,
            }}
          />
        ))}
      </div>

      {isMaxTier ? (
        <p className="font-mono text-xs text-center" style={{ color: 'var(--accent-cyan)' }}>Max tier — Time to Colo</p>
      ) : (
        <button
          onClick={upgradeTier}
          disabled={!canUpgrade}
          className="btn w-full py-2 text-sm"
          style={{
            background: canUpgrade ? 'rgba(34,197,94,0.1)' : 'var(--bg-card)',
            color: canUpgrade ? 'var(--accent-green)' : 'var(--text-muted)',
            border: `1px solid ${canUpgrade ? 'rgba(34,197,94,0.3)' : 'var(--border)'}`,
          }}
        >
          {nextTier} — {upgradeCost.toLocaleString()} CU
        </button>
      )}
    </div>
  );
}
