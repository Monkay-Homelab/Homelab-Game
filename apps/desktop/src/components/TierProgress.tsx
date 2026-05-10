import { useGameStore } from '../stores/gameStore';
import { useConfig, prestigeScale } from '../hooks/useConfig';
import { CURRENCY_COLORS } from '../utils/currencyColors';

export function TierProgress({
  tier,
  computeUnits,
  coloCount,
}: {
  tier: string;
  computeUnits: number;
  coloCount: number;
}) {
  const config = useConfig();
  const upgradeTier = useGameStore((s) => s.upgradeTier);

  const tiers = config.tiers;
  const currentIdx = tiers.findIndex((t) => t.id === tier);
  const isMaxTier = currentIdx >= tiers.length - 1;
  const currentTier = tiers[currentIdx];
  const baseCost = currentTier?.base_upgrade_cost || 0;
  const scale = prestigeScale(config, coloCount);
  const upgradeCost = Math.floor(baseCost * scale);
  const canUpgrade = !isMaxTier && computeUnits >= upgradeCost;
  const nextTier = isMaxTier ? null : tiers[currentIdx + 1]?.label;

  return (
    <div className="panel p-4">
      <div className="flex justify-between items-center mb-3">
        <span className="text-xs font-medium" style={{ color: 'var(--text-secondary)' }}>
          Progression
        </span>
        <span className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>
          {currentIdx + 1}/{tiers.length}
        </span>
      </div>

      <div className="flex gap-1 mb-3">
        {tiers.map((t, i) => (
          <div
            key={t.id}
            className="flex-1 h-1.5 rounded-full transition-all"
            style={{
              background: i <= currentIdx ? 'var(--accent-green)' : 'var(--border)',
              opacity: i <= currentIdx ? 1 : 0.5,
            }}
          />
        ))}
      </div>

      {isMaxTier ? (
        <p className="font-mono text-xs text-center" style={{ color: 'var(--accent-cyan)' }}>
          Max tier — Time to Colo
        </p>
      ) : (
        <button
          onClick={upgradeTier}
          disabled={!canUpgrade}
          className="btn w-full py-2 text-sm"
          style={{
            background: canUpgrade ? CURRENCY_COLORS.cu.bg : 'var(--bg-card)',
            color: canUpgrade ? CURRENCY_COLORS.cu.color : 'var(--text-muted)',
            border: `1px solid ${canUpgrade ? CURRENCY_COLORS.cu.border : 'var(--border)'}`,
          }}
        >
          {nextTier} — {upgradeCost.toLocaleString()} CU
        </button>
      )}
    </div>
  );
}
