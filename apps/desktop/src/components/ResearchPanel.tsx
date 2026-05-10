import type { GameState, ResearchNodeConfig } from '../api';
import { useGameStore } from '../stores/gameStore';
import { useConfig, tierLabel } from '../hooks/useConfig';
import { CURRENCY_COLORS, formatNumber } from '../utils/currencyColors';

const BRANCH_ORDER = ['efficiency', 'reputation', 'infrastructure', 'mastery'];

const BRANCH_LABELS: Record<string, string> = {
  efficiency: 'Efficiency',
  reputation: 'Reputation',
  infrastructure: 'Infrastructure',
  mastery: 'Mastery',
};

const branchConfig: Record<string, { color: string; bg: string; border: string }> = {
  efficiency: { color: '#22c55e', bg: 'rgba(34,197,94,0.1)', border: 'rgba(34,197,94,0.25)' },
  reputation: { color: '#3b82f6', bg: 'rgba(59,130,246,0.1)', border: 'rgba(59,130,246,0.25)' },
  infrastructure: { color: '#f59e0b', bg: 'rgba(245,158,11,0.1)', border: 'rgba(245,158,11,0.25)' },
  mastery: { color: '#a855f7', bg: 'rgba(168,85,247,0.1)', border: 'rgba(168,85,247,0.25)' },
};

const EFFECT_LABELS: Record<string, string> = {
  idle_income: 'idle income',
  reputation_gain: 'reputation gain',
  money_income: 'money income',
  job_reward: 'job reward',
};

function researchCost(baseCost: number, costScale: number, level: number): number {
  return Math.floor(baseCost * Math.pow(costScale, level));
}

export function ResearchPanel({ state }: { state: GameState }) {
  const config = useConfig();
  const buyResearch = useGameStore((s) => s.buyResearch);
  const buyMaxResearch = useGameStore((s) => s.buyMaxResearch);

  const nodes = config.research?.nodes || [];
  const researchLevels = state.research_levels || [];

  // Build tier rank lookup for tier gating
  const tierRanks: Record<string, number> = {};
  for (const t of config.tiers) {
    tierRanks[t.id] = t.rank;
  }
  const playerRank = tierRanks[state.tier] ?? 0;

  // Group nodes by branch
  const grouped = nodes.reduce(
    (acc, node) => {
      if (!acc[node.branch]) acc[node.branch] = [];
      acc[node.branch].push(node);
      return acc;
    },
    {} as Record<string, ResearchNodeConfig[]>,
  );

  return (
    <div className="h-full grid grid-cols-2 gap-4 min-h-0">
      {BRANCH_ORDER.map((branch) => {
        const branchNodes = grouped[branch] || [];
        const cfg = branchConfig[branch] || {
          color: 'var(--text-secondary)',
          bg: 'transparent',
          border: 'var(--border)',
        };
        if (branchNodes.length === 0) return <div key={branch} />;

        return (
          <div key={branch} className="panel p-4 flex flex-col min-h-0 overflow-hidden">
            <h3 className="text-sm font-semibold mb-3 shrink-0" style={{ color: cfg.color }}>
              {BRANCH_LABELS[branch] || branch}
            </h3>
            <div className="space-y-2 overflow-y-auto min-h-0 flex-1">
              {branchNodes.map((node) => {
                const rl = researchLevels.find((r) => r.research_node === node.id);
                const currentLevel = rl?.level || 0;
                const nodeRank = tierRanks[node.min_tier] ?? 0;
                const isLocked = playerRank < nodeRank;
                const cost = researchCost(node.base_cost, node.cost_scale, currentLevel);
                const canAfford = state.compute_units >= cost;
                const totalBonus = currentLevel * node.effect_value * 100;
                const perLevelBonus = node.effect_value * 100;
                const effectLabel = EFFECT_LABELS[node.effect_type] || node.effect_type;

                if (isLocked) {
                  return (
                    <div key={node.id} className="panel-card p-3 opacity-50">
                      <div className="flex items-center justify-between">
                        <div className="min-w-0">
                          <div
                            className="font-medium text-sm truncate"
                            style={{ color: 'var(--text-muted)' }}
                          >
                            {node.name}
                          </div>
                          <div
                            className="text-xs truncate mt-0.5"
                            style={{ color: 'var(--text-muted)' }}
                          >
                            {node.description}
                          </div>
                        </div>
                        <span
                          className="font-mono text-xs shrink-0 px-2 py-1 rounded"
                          style={{ background: 'var(--bg-card)', color: 'var(--text-muted)' }}
                        >
                          Requires {tierLabel(config, node.min_tier)}
                        </span>
                      </div>
                    </div>
                  );
                }

                return (
                  <div key={node.id} className="panel-card p-3">
                    <div className="flex items-center justify-between">
                      <div className="min-w-0 flex-1">
                        <div className="font-medium text-sm truncate">
                          {node.name}
                          <span
                            className="font-mono text-xs ml-1.5"
                            style={{ color: cfg.color, fontWeight: 400 }}
                          >
                            Lv. {currentLevel}
                          </span>
                        </div>
                        <div
                          className="text-xs truncate mt-0.5"
                          style={{ color: 'var(--text-secondary)' }}
                        >
                          {node.description}
                        </div>
                        <div className="flex items-center gap-2 mt-1">
                          <span className="font-mono text-xs" style={{ color: cfg.color }}>
                            +{totalBonus.toFixed(0)}% {effectLabel}
                          </span>
                          <span
                            className="font-mono text-xs"
                            style={{ color: 'var(--text-muted)' }}
                          >
                            (+{perLevelBonus.toFixed(0)}%/lv)
                          </span>
                        </div>
                      </div>
                      <div className="flex items-center gap-1.5 shrink-0 ml-2">
                        <button
                          onClick={() => buyResearch(node.id)}
                          disabled={!canAfford}
                          className="btn px-3 py-1 text-xs"
                          style={{
                            background: canAfford ? CURRENCY_COLORS.cu.bg : 'var(--bg-card)',
                            color: canAfford ? CURRENCY_COLORS.cu.color : 'var(--text-muted)',
                            border: `1px solid ${canAfford ? CURRENCY_COLORS.cu.border : 'var(--border)'}`,
                          }}
                        >
                          {formatNumber(cost)} CU
                        </button>
                        <button
                          onClick={() => buyMaxResearch(node.id)}
                          disabled={!canAfford}
                          className="btn px-2 py-1 text-xs"
                          style={{
                            background: canAfford ? CURRENCY_COLORS.cu.bg : 'var(--bg-card)',
                            color: canAfford ? CURRENCY_COLORS.cu.color : 'var(--text-muted)',
                            border: `1px solid ${canAfford ? CURRENCY_COLORS.cu.border : 'var(--border)'}`,
                          }}
                        >
                          Max
                        </button>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        );
      })}
    </div>
  );
}
