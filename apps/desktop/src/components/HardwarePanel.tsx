import { useState } from 'react';
import type { GameState, HardwareTemplate } from '../api';
import { useGameStore } from '../stores/gameStore';

const UPGRADEABLE_TYPES = ['server', 'desktop', 'sbc', 'mini_pc', 'gpu_server'];
const COMPONENTS = ['cpu', 'ram', 'storage', 'nic'];

const CATEGORY_ORDER = ['Compute', 'Storage', 'Network', 'Power', 'Misc'];
const TYPE_TO_CATEGORY: Record<string, string> = {
  server: 'Compute',
  desktop: 'Compute',
  sbc: 'Compute',
  mini_pc: 'Compute',
  gpu_server: 'Compute',
  switch: 'Network',
  patch_panel: 'Network',
  ups: 'Power',
  nas: 'Storage',
  shelf: 'Misc',
};
const CATEGORY_COLORS: Record<string, { text: string; bg: string; border: string }> = {
  Compute: { text: '#a855f7', bg: 'rgba(168,85,247,0.1)', border: 'rgba(168,85,247,0.25)' },
  Network: { text: '#3b82f6', bg: 'rgba(59,130,246,0.1)', border: 'rgba(59,130,246,0.25)' },
  Power:   { text: '#f59e0b', bg: 'rgba(245,158,11,0.1)', border: 'rgba(245,158,11,0.25)' },
  Storage: { text: '#06b6d4', bg: 'rgba(6,182,212,0.1)', border: 'rgba(6,182,212,0.25)' },
  Misc:    { text: '#94a3b8', bg: 'rgba(148,163,184,0.1)', border: 'rgba(148,163,184,0.25)' },
};

export function HardwarePanel({ state }: { state: GameState }) {
  const buyHardware = useGameStore(s => s.buyHardware);
  const sellHardware = useGameStore(s => s.sellHardware);
  const upgradeComponent = useGameStore(s => s.upgradeComponent);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  return (
    <div className="h-full flex gap-4 min-h-0">
      {/* Hardware Shop */}
      <div className="flex-1 panel p-4 flex flex-col min-h-0">
        <h3 className="text-sm font-semibold mb-3" style={{ color: 'var(--accent-purple)' }}>Hardware Shop</h3>
        <div className="space-y-4 overflow-y-auto min-h-0 flex-1">
          {(() => {
            const grouped: Record<string, typeof state.available_hardware> = {};
            for (const h of state.available_hardware) {
              const cat = TYPE_TO_CATEGORY[h.type] || 'Other';
              if (!grouped[cat]) grouped[cat] = [];
              grouped[cat].push(h);
            }
            return CATEGORY_ORDER.filter(cat => grouped[cat]?.length).map(cat => (
              <div key={cat}>
                <div className="text-xs font-semibold mb-2 font-mono uppercase tracking-wide" style={{ color: CATEGORY_COLORS[cat].text }}>
                  {cat}
                </div>
                <div className="space-y-2">
                  {grouped[cat].map(h => {
                    const colors = CATEGORY_COLORS[cat];
                    const canAfford = state.compute_units >= h.cost;
                    return (
                      <div key={h.name} className="panel-card p-3 flex items-center justify-between">
                        <div>
                          <div className="font-medium text-sm">{h.name}</div>
                          <div className="font-mono text-xs mt-0.5" style={{ color: 'var(--text-secondary)' }}>
                            {h.power_draw}W · +{h.compute_per_tick}/tick
                            {h.rack_units_used !== null ? ` · ${h.rack_units_used}U` : ` · ${h.slots_used} slot`}
                          </div>
                        </div>
                        <button
                          onClick={() => buyHardware(h.name)}
                          disabled={!canAfford}
                          className="btn px-3 py-1 text-xs shrink-0"
                          style={{
                            background: canAfford ? colors.bg : 'var(--bg-card)',
                            color: canAfford ? colors.text : 'var(--text-muted)',
                            border: `1px solid ${canAfford ? colors.border : 'var(--border)'}`,
                          }}
                        >
                          {h.cost.toLocaleString()} CU
                        </button>
                      </div>
                    );
                  })}
                </div>
              </div>
            ));
          })()}
        </div>
      </div>

      {/* Owned Hardware */}
      <div className="flex-1 panel p-4 flex flex-col min-h-0">
        <div className="flex justify-between items-center mb-3 shrink-0">
          <h3 className="text-sm font-semibold" style={{ color: 'var(--accent-purple)' }}>Owned Hardware</h3>
          {state.hardware && state.hardware.some(h => UPGRADEABLE_TYPES.includes(h.type)) && (
            <button
              onClick={() => useGameStore.getState().upgradeAllComponents()}
              className="btn px-2 py-1 text-xs"
              style={{ background: 'rgba(168,85,247,0.1)', color: '#a855f7', border: '1px solid rgba(168,85,247,0.25)' }}
            >
              Upgrade All
            </button>
          )}
        </div>
        <div className="space-y-4 overflow-y-auto min-h-0 flex-1">
          {(() => {
            const hw = state.hardware || [];
            if (hw.length === 0) return <p className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>No hardware yet</p>;

            const grouped: Record<string, typeof hw> = {};
            for (const h of hw) {
              const cat = TYPE_TO_CATEGORY[h.type] || 'Other';
              if (!grouped[cat]) grouped[cat] = [];
              grouped[cat].push(h);
            }

            return CATEGORY_ORDER.filter(cat => grouped[cat]?.length).map(cat => {
              const colors = CATEGORY_COLORS[cat];
              return (
                <div key={cat}>
                  <div className="text-xs font-semibold mb-2 font-mono uppercase tracking-wide" style={{ color: colors.text }}>
                    {cat}
                  </div>
                  <div className="space-y-2">
                    {grouped[cat].map(h => {
                      const isUpgradeable = UPGRADEABLE_TYPES.includes(h.type);
                      const hwCompUps = (state.component_upgrades || []).filter(cu => cu.hardware_id === h.id);
                      const totalBonus = hwCompUps.reduce((sum, cu) => sum + cu.compute_bonus, 0);
                      const totalPowerReduce = hwCompUps.reduce((sum, cu) => sum + cu.power_reduction, 0);

                      return (
                        <div key={h.id} className="panel-card p-3">
                          <div className="flex items-center justify-between">
                            <div
                              className={`flex-1 ${isUpgradeable ? 'cursor-pointer' : ''}`}
                              onClick={() => isUpgradeable && setExpandedId(expandedId === h.id ? null : h.id)}
                            >
                              <div className="font-medium text-sm">{h.name}</div>
                              <div className="font-mono text-xs mt-0.5" style={{ color: 'var(--text-secondary)' }}>
                                {h.power_draw - totalPowerReduce}W · +{h.compute_per_tick + totalBonus}/tick
                                {h.rack_units_used !== null ? ` · ${h.rack_units_used}U` : ` · ${h.slots_used} slot`}
                              </div>
                              {hwCompUps.length > 0 && (
                                <div className="flex gap-2 mt-1">
                                  {[...hwCompUps].sort((a, b) => COMPONENTS.indexOf(a.component) - COMPONENTS.indexOf(b.component)).map(cu => (
                                    <span key={cu.component} className="font-mono text-xs px-1.5 py-0.5 rounded" style={{ background: colors.bg, color: colors.text }}>
                                      {cu.component.toUpperCase()} Lv{cu.level}
                                      {cu.compute_bonus > 0 && ` +${cu.compute_bonus}`}
                                      {cu.power_reduction > 0 && ` -${cu.power_reduction}W`}
                                    </span>
                                  ))}
                                </div>
                              )}
                            </div>
                            <button
                              onClick={() => sellHardware(h.id)}
                              className="btn px-2 py-1 text-xs shrink-0"
                              style={{ background: 'rgba(239,68,68,0.1)', color: 'var(--accent-red)', border: '1px solid rgba(239,68,68,0.2)' }}
                            >
                              Sell
                            </button>
                          </div>
                          {expandedId === h.id && (
                            <div className="mt-2 grid grid-cols-4 gap-1">
                              {COMPONENTS.map(comp => {
                                const existing = hwCompUps.find(cu => cu.component === comp);
                                const level = existing?.level || 0;
                                return (
                                  <button
                                    key={comp}
                                    onClick={() => upgradeComponent(h.id, comp)}
                                    className="btn px-2 py-1 text-xs"
                                    style={{ background: colors.bg, color: colors.text, border: `1px solid ${colors.border}` }}
                                  >
                                    {comp.toUpperCase()} {level > 0 ? `Lv${level}→${level + 1}` : 'Lv1'}
                                  </button>
                                );
                              })}
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </div>
              );
            });
          })()}
        </div>
      </div>

    </div>
  );
}
