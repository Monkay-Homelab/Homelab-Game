import type { GameState } from '../api';
import { useGameStore } from '../stores/gameStore';
import { CURRENCY_COLORS } from '../utils/currencyColors';
import { CurrencyStatLine } from './shared/CurrencyStatLine';

export function ServicePanel({ state }: { state: GameState }) {
  const deployService = useGameStore((s) => s.deployService);

  return (
    <div className="h-full flex gap-4 min-h-0">
      {/* Available Services */}
      <div className="flex-1 panel p-4 flex flex-col min-h-0">
        <div className="flex justify-between items-center mb-3 shrink-0">
          <h3 className="text-sm font-semibold" style={{ color: 'var(--accent-blue)' }}>
            Service Catalog
          </h3>
          {state.available_services.some(
            (s) => !state.services?.some((svc) => svc.name === s.name),
          ) && (
            <button
              onClick={() => useGameStore.getState().deployAllServices()}
              className="btn px-2 py-1 text-xs"
              style={{
                background: CURRENCY_COLORS.cu.bg,
                color: CURRENCY_COLORS.cu.color,
                border: `1px solid ${CURRENCY_COLORS.cu.border}`,
              }}
            >
              Deploy All
            </button>
          )}
        </div>
        <div className="space-y-2 overflow-y-auto min-h-0 flex-1">
          {state.available_services.map((s) => {
            const deployed = state.services?.some((svc) => svc.name === s.name) || false;
            const canAfford = state.compute_units >= s.cost;
            return (
              <div key={s.name} className="panel-card p-3 flex items-center justify-between">
                <div>
                  <div className="font-medium text-sm">{s.name}</div>
                  <div className="font-mono text-xs mt-0.5">
                    <CurrencyStatLine
                      items={[
                        { currency: 'cu', value: s.compute_per_tick, prefix: '+', suffix: ' CU' },
                        {
                          currency: 'rep',
                          value: s.reputation_per_tick,
                          prefix: '+',
                          suffix: ' Rep',
                        },
                        { currency: 'pwr', value: s.power_required, suffix: 'W' },
                      ]}
                    />
                  </div>
                </div>
                {deployed ? (
                  <span
                    className="font-mono text-xs px-2 py-1"
                    style={{ color: 'var(--accent-green)' }}
                  >
                    LIVE
                  </span>
                ) : (
                  <button
                    onClick={() => deployService(s.name)}
                    disabled={!canAfford}
                    className="btn px-3 py-1 text-xs shrink-0"
                    style={{
                      background: canAfford ? CURRENCY_COLORS.cu.bg : 'var(--bg-card)',
                      color: canAfford ? CURRENCY_COLORS.cu.color : 'var(--text-muted)',
                      border: `1px solid ${canAfford ? CURRENCY_COLORS.cu.border : 'var(--border)'}`,
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
        <h3 className="text-sm font-semibold mb-3" style={{ color: 'var(--accent-blue)' }}>
          Running Services
        </h3>
        <div className="space-y-2 overflow-y-auto min-h-0 flex-1">
          {state.services && state.services.length > 0 ? (
            state.services.map((s) => (
              <div key={s.id} className="panel-card p-3 flex justify-between items-center">
                <div>
                  <div className="font-medium text-sm">{s.name}</div>
                  <div className="font-mono text-xs mt-0.5">
                    <CurrencyStatLine
                      items={[
                        { currency: 'cu', value: s.compute_per_tick, prefix: '+', suffix: ' CU' },
                        {
                          currency: 'rep',
                          value: s.reputation_per_tick,
                          prefix: '+',
                          suffix: ' Rep',
                        },
                        ...(s.money_per_tick > 0
                          ? [{ currency: 'money' as const, value: s.money_per_tick, prefix: '+$' }]
                          : []),
                      ]}
                    />
                  </div>
                </div>
                <span className="font-mono text-xs" style={{ color: 'var(--accent-green)' }}>
                  LIVE
                </span>
              </div>
            ))
          ) : (
            <p className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>
              No services deployed
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
