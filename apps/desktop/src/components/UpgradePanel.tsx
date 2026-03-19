import type { GameState, UpgradeTemplate } from '../api';
import { useGameStore } from '../stores/gameStore';
import { useConfig, prestigeScale } from '../hooks/useConfig';

const typeConfig: Record<string, { color: string; bg: string; border: string; label: string }> = {
  cooling: { color: '#06b6d4', bg: 'rgba(6,182,212,0.1)', border: 'rgba(6,182,212,0.25)', label: 'Cooling' },
  networking: { color: '#f97316', bg: 'rgba(249,115,22,0.1)', border: 'rgba(249,115,22,0.25)', label: 'Networking' },
  automation: { color: '#22c55e', bg: 'rgba(34,197,94,0.1)', border: 'rgba(34,197,94,0.25)', label: 'Automation' },
  knowledge: { color: '#f59e0b', bg: 'rgba(245,158,11,0.1)', border: 'rgba(245,158,11,0.25)', label: 'Knowledge' },
};

export function UpgradePanel({ state }: { state: GameState }) {
  const config = useConfig();
  const buyUpgrade = useGameStore(s => s.buyUpgrade);
  const ownedNames = new Set(state.upgrades?.map(u => u.name) || []);
  const available = state.available_upgrades || [];
  const scale = prestigeScale(config, state.colo_count);

  const grouped = available.reduce((acc, u) => {
    if (!acc[u.type]) acc[u.type] = [];
    acc[u.type].push(u);
    return acc;
  }, {} as Record<string, UpgradeTemplate[]>);

  return (
    <div className="h-full grid grid-cols-2 gap-4 min-h-0">
      {['cooling', 'networking', 'automation', 'knowledge'].map(type_ => {
        const upgrades = grouped[type_] || [];
        const cfg = typeConfig[type_] || { color: 'var(--text-secondary)', label: type_ };
        if (upgrades.length === 0) return <div key={type_} />;

        return (
          <div key={type_} className="panel p-4 flex flex-col min-h-0 overflow-hidden">
            <div className="flex items-center justify-between mb-3 shrink-0">
              <h3 className="text-sm font-semibold" style={{ color: cfg.color }}>
                {cfg.label}
                {type_ === 'cooling' && (
                  <span className="font-mono font-normal text-xs ml-1" style={{ color: state.overheating ? 'var(--accent-red)' : 'var(--text-muted)' }}>
                    {state.heat_generated}/{state.cooling_capacity}
                    {state.overheating && ' HOT'}
                  </span>
                )}
              </h3>
              {upgrades.some(u => !ownedNames.has(u.name)) && (
                <button
                  onClick={() => useGameStore.getState().buyAllUpgrades(type_)}
                  className="btn px-2 py-1 text-xs"
                  style={{ background: cfg.bg, color: cfg.color, border: `1px solid ${cfg.border}` }}
                >
                  Buy All
                </button>
              )}
            </div>
            <div className="space-y-2 overflow-y-auto min-h-0 flex-1">
              {upgrades.map(u => {
                const owned = ownedNames.has(u.name);
                return (
                  <div key={u.name} className="panel-card p-3 flex items-center justify-between">
                    <div className="min-w-0">
                      <div className="font-medium text-sm truncate">
                        {u.name}
                        <span className="font-mono text-xs ml-1.5" style={{ color: cfg.color, fontWeight: 400 }}>{u.effect}</span>
                        {u.persistent && <span className="text-xs ml-1" style={{ color: 'var(--accent-green)' }}>(kept)</span>}
                      </div>
                      <div className="text-xs truncate mt-0.5" style={{ color: 'var(--text-secondary)' }}>{u.description}</div>
                    </div>
                    {owned ? (
                      <span className="font-mono text-xs shrink-0 px-2" style={{ color: 'var(--accent-green)' }}>OWNED</span>
                    ) : (
                      <button
                        onClick={() => buyUpgrade(u.name)}
                        disabled={u.cost_type === 'money' ? state.money < u.cost : state.compute_units < Math.floor(u.cost * (u.type === 'automation' ? scale : 1))}
                        className="btn px-3 py-1 text-xs shrink-0 ml-2"
                        style={{
                          background: cfg.bg,
                          color: cfg.color,
                          border: `1px solid ${cfg.border}`,
                        }}
                      >
                        {u.cost_type === 'money' ? `$${u.cost.toLocaleString()}` : `${Math.floor(u.cost * (u.type === 'automation' ? scale : 1)).toLocaleString()} CU`}
                      </button>
                    )}
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
