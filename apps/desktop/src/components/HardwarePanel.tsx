import { useState } from 'react';
import type { GameState } from '../api';
import { useGameStore } from '../stores/gameStore';
import { useConfig } from '../hooks/useConfig';

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

function buildBonusDescriptions(hw: { ups_compute: Record<string, number>; network_income: Record<string, number>; storage_rep: Record<string, number>; patch_panel_bonus: number }): Record<string, string> {
  const desc: Record<string, string> = {};
  for (const [name, val] of Object.entries(hw.network_income)) {
    desc[name] = `+${Math.round(val * 100)}% idle income`;
  }
  for (const [name, val] of Object.entries(hw.storage_rep)) {
    desc[name] = `+${Math.round(val * 100)}% reputation`;
  }
  for (const [name, val] of Object.entries(hw.ups_compute)) {
    desc[name] = `+${val} CU/tick · power protection`;
  }
  desc['1U Patch Panel'] = `+${Math.round(hw.patch_panel_bonus * 100)}% reputation`;
  return desc;
}

export function HardwarePanel({ state }: { state: GameState }) {
  const config = useConfig();
  const hwBonuses = config.hardware_bonuses;
  const HARDWARE_BONUSES = buildBonusDescriptions(hwBonuses);

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
                            {h.power_draw}W
                            {h.compute_per_tick > 0 && ` · +${h.compute_per_tick}/tick`}
                            {h.rack_units_used !== null ? ` · ${h.rack_units_used}U` : ` · ${h.slots_used} slot`}
                          </div>
                          {HARDWARE_BONUSES[h.name] && (
                            <div className="text-xs mt-0.5" style={{ color: colors.text }}>{HARDWARE_BONUSES[h.name]}</div>
                          )}
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

        {/* Stats summary */}
        {state.hardware && state.hardware.length > 0 && (() => {
          const compUps = state.component_upgrades || [];
          let totalCompute = 0;
          let totalPower = 0;
          let netBonus = 0;
          let repBonus = 0;
          let items = 0;

          for (const h of state.hardware) {
            items++;
            let compute = h.compute_per_tick;
            let power = h.power_draw;
            for (const cu of compUps) {
              if (cu.hardware_id === h.id) {
                compute += Math.floor(h.compute_per_tick * cu.compute_bonus / 100);
                power -= cu.power_reduction;
              }
            }
            if (power < 0) power = 0;
            totalCompute += compute;
            if (hwBonuses.ups_compute[h.name]) totalCompute += hwBonuses.ups_compute[h.name];
            totalPower += power;
            if (hwBonuses.network_income[h.name]) netBonus += Math.round(hwBonuses.network_income[h.name] * 100);
            if (hwBonuses.storage_rep[h.name]) repBonus += Math.round(hwBonuses.storage_rep[h.name] * 100);
            if (h.type === 'patch_panel') repBonus += Math.round(hwBonuses.patch_panel_bonus * 100);
          }

          return (
            <div className="grid grid-cols-4 gap-2 mb-3 shrink-0">
              <div className="panel-card p-2 text-center">
                <div className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>Items</div>
                <div className="stat-value text-sm" style={{ color: 'var(--text-primary)' }}>{items}</div>
              </div>
              <div className="panel-card p-2 text-center">
                <div className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>CU/tick</div>
                <div className="stat-value text-sm" style={{ color: '#a855f7' }}>
                  +{Math.floor(totalCompute * (1 + netBonus / 100))}
                </div>
                {netBonus > 0 && <div className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>({totalCompute} base)</div>}
              </div>
              <div className="panel-card p-2 text-center">
                <div className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>Power</div>
                <div className="stat-value text-sm" style={{ color: '#f59e0b' }}>{totalPower}W</div>
              </div>
              <div className="panel-card p-2 text-center">
                <div className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>Bonus</div>
                <div className="stat-value text-sm" style={{ color: '#22c55e' }}>
                  {netBonus > 0 && `+${netBonus}% CU`}
                  {netBonus > 0 && repBonus > 0 && ' · '}
                  {repBonus > 0 && `+${repBonus}% Rep`}
                  {netBonus === 0 && repBonus === 0 && '—'}
                </div>
              </div>
            </div>
          );
        })()}

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
                      const totalBonus = hwCompUps.reduce((sum, cu) => sum + Math.floor(h.compute_per_tick * cu.compute_bonus / 100), 0);
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
                                {h.power_draw - totalPowerReduce}W
                                {(h.compute_per_tick + totalBonus) > 0 && ` · +${h.compute_per_tick + totalBonus}/tick`}
                                {h.rack_units_used !== null ? ` · ${h.rack_units_used}U` : ` · ${h.slots_used} slot`}
                              </div>
                              {HARDWARE_BONUSES[h.name] && (
                                <div className="text-xs mt-0.5" style={{ color: colors.text }}>{HARDWARE_BONUSES[h.name]}</div>
                              )}
                              {hwCompUps.length > 0 && (
                                <div className="flex gap-2 mt-1">
                                  {[...hwCompUps].sort((a, b) => COMPONENTS.indexOf(a.component) - COMPONENTS.indexOf(b.component)).map(cu => (
                                    <span key={`${h.id}-${cu.component}`} className="font-mono text-xs px-1.5 py-0.5 rounded" style={{ background: colors.bg, color: colors.text }}>
                                      {cu.component.toUpperCase()} Lv{cu.level}
                                      {cu.compute_bonus > 0 && ` +${cu.compute_bonus}%`}
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
                                    key={`${h.id}-${comp}`}
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
