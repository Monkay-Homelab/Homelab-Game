import type { GameState, ServiceTemplate } from '../api';
import { useGameStore } from '../stores/gameStore';

export function ServicePanel({ state }: { state: GameState }) {
  const deployService = useGameStore(s => s.deployService);

  return (
    <div className="h-full flex gap-4 min-h-0">
      {/* Available Services */}
      <div className="flex-1 panel p-4 flex flex-col min-h-0">
        <div className="flex justify-between items-center mb-3 shrink-0">
          <h3 className="text-sm font-semibold" style={{ color: 'var(--accent-blue)' }}>Service Catalog</h3>
          {state.available_services.some(s => !state.services?.some(svc => svc.name === s.name)) && (
            <button
              onClick={() => useGameStore.getState().deployAllServices()}
              className="btn px-2 py-1 text-xs"
              style={{ background: 'rgba(59,130,246,0.1)', color: '#3b82f6', border: '1px solid rgba(59,130,246,0.25)' }}
            >
              Buy All
            </button>
          )}
        </div>
        <div className="space-y-2 overflow-y-auto min-h-0 flex-1">
          {state.available_services.map(s => {
            const deployed = state.services?.some(svc => svc.name === s.name) || false;
            const canAfford = state.compute_units >= s.cost;
            return (
              <div key={s.name} className="panel-card p-3 flex items-center justify-between">
                <div>
                  <div className="font-medium text-sm">{s.name}</div>
                  <div className="font-mono text-xs mt-0.5" style={{ color: 'var(--text-secondary)' }}>
                    +{s.compute_per_tick} CU · +{s.reputation_per_tick} Rep · {s.power_required}W
                  </div>
                </div>
                {deployed ? (
                  <span className="font-mono text-xs px-2 py-1" style={{ color: 'var(--accent-green)' }}>LIVE</span>
                ) : (
                  <button
                    onClick={() => deployService(s.name)}
                    disabled={!canAfford}
                    className="btn px-3 py-1 text-xs shrink-0"
                    style={{
                      background: canAfford ? 'rgba(59,130,246,0.1)' : 'var(--bg-card)',
                      color: canAfford ? 'var(--accent-blue)' : 'var(--text-muted)',
                      border: `1px solid ${canAfford ? 'rgba(59,130,246,0.2)' : 'var(--border)'}`,
                    }}
                  >
                    {s.cost.toLocaleString()} CU
                  </button>
                )}
              </div>
            );
          })}
        </div>
      </div>

      {/* Running Services */}
      <div className="flex-1 panel p-4 flex flex-col min-h-0">
        <h3 className="text-sm font-semibold mb-3" style={{ color: 'var(--accent-blue)' }}>Running Services</h3>
        <div className="space-y-2 overflow-y-auto min-h-0 flex-1">
          {state.services && state.services.length > 0 ? state.services.map(s => (
            <div key={s.id} className="panel-card p-3 flex justify-between items-center">
              <div>
                <div className="font-medium text-sm">{s.name}</div>
                <div className="font-mono text-xs mt-0.5" style={{ color: 'var(--text-secondary)' }}>
                  +{s.compute_per_tick} CU · +{s.reputation_per_tick} Rep
                  {s.money_per_tick > 0 && ` · +$${s.money_per_tick}`}
                </div>
              </div>
              <span className="font-mono text-xs" style={{ color: 'var(--accent-green)' }}>LIVE</span>
            </div>
          )) : (
            <p className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>No services deployed</p>
          )}
        </div>
      </div>
    </div>
  );
}
