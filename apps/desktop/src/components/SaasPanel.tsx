import type { GameState, SaasTemplate, CustomerItem, ExpenseItem } from '../api';
import { useGameStore } from '../stores/gameStore';
import { useConfig, prestigeScale } from '../hooks/useConfig';
import { CURRENCY_COLORS } from '../utils/currencyColors';
import { CurrencyValue } from './shared/CurrencyValue';
import { CurrencyStatLine } from './shared/CurrencyStatLine';

export function SaasPanel({ state }: { state: GameState }) {
  const config = useConfig();
  const unlockSaas = useGameStore((s) => s.unlockSaas);
  const deploySaas = useGameStore((s) => s.deploySaas);

  const saasUnlocked = state.saas_unlocked || false;
  const customers: CustomerItem[] = state.customers || [];
  const availableSaas: SaasTemplate[] = state.available_saas || [];
  const deployedSaasNames = new Set(
    (state.services || [])
      .filter((s) => availableSaas.some((t) => t.name === s.name))
      .map((s) => s.name),
  );
  const isRack = state.rack_units !== null;
  const scale = prestigeScale(config, state.colo_count);

  if (!isRack) {
    return (
      <div className="h-full panel p-6 flex items-center justify-center">
        <p className="font-mono text-sm" style={{ color: 'var(--text-muted)' }}>
          Reach a rack tier to unlock SaaS
        </p>
      </div>
    );
  }

  if (!saasUnlocked) {
    const unlockCost = Math.floor(config.saas_unlock.base_cost * scale);
    const canUnlock =
      state.compute_units >= unlockCost &&
      state.reputation >= config.saas_unlock.reputation_required;
    return (
      <div className="h-full panel p-6 flex flex-col items-center justify-center">
        <h3 className="text-lg font-semibold mb-2" style={{ color: 'var(--accent-amber)' }}>
          Start a SaaS Business
        </h3>
        <p className="text-sm mb-4 text-center" style={{ color: 'var(--text-secondary)' }}>
          Sell hosting services to customers for revenue.
        </p>
        <div className="font-mono text-xs mb-4" style={{ color: 'var(--text-muted)' }}>
          Requires: <CurrencyValue currency="cu" value={unlockCost} suffix=" CU" /> +{' '}
          <CurrencyValue
            currency="rep"
            value={config.saas_unlock.reputation_required}
            suffix=" REP"
          />
        </div>
        <button
          onClick={unlockSaas}
          disabled={!canUnlock}
          className="btn px-6 py-2 text-sm"
          style={{
            background: canUnlock ? CURRENCY_COLORS.cu.bg : 'var(--bg-card)',
            color: canUnlock ? CURRENCY_COLORS.cu.color : 'var(--text-muted)',
            border: `1px solid ${canUnlock ? CURRENCY_COLORS.cu.border : 'var(--border)'}`,
          }}
        >
          Unlock SaaS
        </button>
      </div>
    );
  }

  const expenses: ExpenseItem[] = state.expenses || [];
  const totalRevenue = customers.reduce((sum, c) => sum + c.monthly_revenue, 0);
  const totalExpenses = expenses.reduce((sum, e) => sum + e.cost_per_tick, 0);
  const netIncome = totalRevenue - totalExpenses;

  const sortedSaas = [...availableSaas].sort((a, b) => a.power_required - b.power_required);
  const typePower: Record<string, number> = {};
  for (const s of availableSaas) typePower[s.type] = s.power_required;
  const sortedCustomers = [...customers].sort(
    (a, b) => (typePower[a.service_type] || 0) - (typePower[b.service_type] || 0),
  );

  return (
    <div className="h-full flex gap-4 min-h-0">
      {/* SaaS Services */}
      <div className="flex-1 panel p-4 flex flex-col min-h-0">
        <div className="flex justify-between items-center mb-3 shrink-0">
          <h3 className="text-sm font-semibold" style={{ color: 'var(--accent-amber)' }}>
            SaaS Services
          </h3>
          {sortedSaas.some((s) => !deployedSaasNames.has(s.name)) && (
            <button
              onClick={() => useGameStore.getState().deployAllSaas()}
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
          {sortedSaas.map((s) => {
            const deployed = deployedSaasNames.has(s.name);
            const scaledCost = Math.floor(s.deploy_cost * scale);
            const canAfford =
              state.compute_units >= scaledCost && state.reputation >= s.reputation_required;
            return (
              <div key={s.name} className="panel-card p-3 flex items-center justify-between">
                <div>
                  <div className="font-medium text-sm">{s.name}</div>
                  <div
                    className="font-mono text-xs mt-0.5"
                    style={{ color: 'var(--text-secondary)' }}
                  >
                    <CurrencyStatLine
                      items={[
                        {
                          currency: 'money',
                          value: s.revenue_per_customer,
                          prefix: '$',
                          suffix: '/cust',
                        },
                        { currency: 'pwr', value: s.power_required, suffix: 'W' },
                      ]}
                    />
                  </div>
                </div>
                {deployed ? (
                  <span className="font-mono text-xs" style={{ color: 'var(--accent-green)' }}>
                    LIVE
                  </span>
                ) : (
                  <button
                    onClick={() => deploySaas(s.name)}
                    disabled={!canAfford}
                    className="btn px-3 py-1 text-xs shrink-0"
                    style={{
                      background: canAfford ? CURRENCY_COLORS.cu.bg : 'var(--bg-card)',
                      color: canAfford ? CURRENCY_COLORS.cu.color : 'var(--text-muted)',
                      border: `1px solid ${canAfford ? CURRENCY_COLORS.cu.border : 'var(--border)'}`,
                    }}
                  >
                    {scaledCost.toLocaleString()} CU
                  </button>
                )}
              </div>
            );
          })}
        </div>
      </div>

      {/* Customers */}
      <div className="flex-1 panel p-4 flex flex-col min-h-0">
        <div className="flex justify-between items-center mb-3 shrink-0">
          <h3 className="text-sm font-semibold" style={{ color: 'var(--accent-amber)' }}>
            Customers
          </h3>
          <span
            className="font-mono text-xs"
            style={{ color: netIncome >= 0 ? CURRENCY_COLORS.money.color : 'var(--accent-red)' }}
          >
            ${netIncome}/tick net
          </span>
        </div>
        <div className="space-y-2 overflow-y-auto min-h-0 flex-1">
          {sortedCustomers.length > 0 ? (
            sortedCustomers.map((c) => (
              <div key={c.id} className="panel-card p-3 flex justify-between items-center">
                <div>
                  <div className="font-medium text-sm">{c.name}</div>
                  <div
                    className="font-mono text-xs mt-0.5"
                    style={{ color: 'var(--text-secondary)' }}
                  >
                    {c.service_type} <span style={{ color: 'var(--text-muted)' }}> · </span>
                    <CurrencyValue
                      currency="money"
                      value={c.monthly_revenue}
                      prefix="$"
                      suffix="/tick"
                    />
                  </div>
                </div>
                <span
                  className="font-mono text-xs"
                  style={{
                    color:
                      c.satisfaction >= 80
                        ? 'var(--accent-green)'
                        : c.satisfaction >= 50
                          ? 'var(--accent-amber)'
                          : 'var(--accent-red)',
                  }}
                >
                  {c.satisfaction}%
                </span>
              </div>
            ))
          ) : (
            <p className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>
              Deploy a service to attract customers
            </p>
          )}
        </div>
      </div>

      {/* Expenses */}
      {expenses.length > 0 && (
        <div className="w-64 shrink-0 panel p-4 flex flex-col min-h-0">
          <h3
            className="text-sm font-semibold mb-3 shrink-0"
            style={{ color: 'var(--accent-red)' }}
          >
            Expenses
          </h3>
          <div className="space-y-2 overflow-y-auto min-h-0 flex-1">
            {expenses.map((e) => (
              <div key={e.id} className="panel-card p-3 flex justify-between items-center">
                <span className="text-sm">{e.name}</span>
                <span className="font-mono text-xs" style={{ color: 'var(--accent-red)' }}>
                  -${e.cost_per_tick}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
