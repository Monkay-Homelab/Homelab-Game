import { useEffect, useState } from 'react';
import { useGameStore } from './stores/gameStore';
import { Login } from './components/Login';
import { CurrencyBar } from './components/CurrencyBar';
import { ClickArea } from './components/ClickArea';
import { HardwarePanel } from './components/HardwarePanel';
import { ServicePanel } from './components/ServicePanel';
import { TierProgress } from './components/TierProgress';
import { DonatePanel } from './components/DonatePanel';
import { UpgradePanel } from './components/UpgradePanel';
import { SaasPanel } from './components/SaasPanel';
import { DatacenterPanel } from './components/DatacenterPanel';
import { SocialPanel } from './components/SocialPanel';
import { MarketPanel } from './components/MarketPanel';
import { EventLog } from './components/EventLog';
import { useWebSocket } from './hooks/useWebSocket';

type Tab = 'hardware' | 'services' | 'upgrades' | 'saas' | 'datacenter' | 'social' | 'market';

const TABS: { id: Tab; label: string; icon: string; color: string }[] = [
  { id: 'hardware', label: 'Hardware', icon: '[ ]', color: 'var(--accent-purple)' },
  { id: 'services', label: 'Services', icon: '{ }', color: 'var(--accent-blue)' },
  { id: 'upgrades', label: 'Upgrades', icon: ' ^ ', color: 'var(--accent-green)' },
  { id: 'saas', label: 'SaaS', icon: ' $ ', color: 'var(--accent-amber)' },
  { id: 'datacenter', label: 'Datacenter', icon: ' # ', color: 'var(--accent-cyan)' },
  { id: 'market', label: 'Market', icon: ' B ', color: 'var(--accent-amber)' },
  { id: 'social', label: 'Social', icon: ' @ ', color: '#22c55e' },
];

export function App() {
  const { token, state, config, error, fetchConfig, fetchState, logout } = useGameStore();
  const [activeTab, setActiveTab] = useState<Tab>('hardware');
  useWebSocket();

  useEffect(() => {
    if (token && !config) {
      fetchConfig();
    }
  }, [token, config, fetchConfig]);

  useEffect(() => {
    if (token && !state) {
      fetchState();
    }
  }, [token, state, fetchState]);

  useEffect(() => {
    if (!token) return;
    const interval = setInterval(fetchState, 5000);
    return () => clearInterval(interval);
  }, [token, fetchState]);

  if (!token) return <Login />;
  if (!state || !config) {
    return (
      <div className="h-screen flex items-center justify-center" style={{ background: 'var(--bg-deep)' }}>
        <div className="text-center">
          <div className="font-mono text-sm" style={{ color: 'var(--accent-green)' }}>
            Initializing systems...
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="h-screen flex flex-col overflow-hidden" style={{ background: 'var(--bg-deep)' }}>
      {/* Top Bar */}
      <header className="shrink-0 flex items-center justify-between px-5 py-3" style={{ borderBottom: '1px solid var(--border)' }}>
        <div className="flex items-center gap-3">
          <span className="font-mono text-xs px-2 py-1 rounded" style={{ background: 'var(--bg-card)', color: 'var(--accent-green)' }}>
            HLG
          </span>
          <h1 className="text-lg font-semibold tracking-tight">Homelab the Game</h1>
        </div>
        <button
          onClick={logout}
          className="text-xs px-3 py-1.5 rounded transition-colors"
          style={{ color: 'var(--text-secondary)', background: 'var(--bg-card)' }}
        >
          Logout
        </button>
      </header>

      {error && (
        <div className="mx-5 mt-3 px-4 py-2 rounded text-sm" style={{ background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)', color: '#fca5a5' }}>
          {error}
        </div>
      )}

      {/* Currency Bar */}
      <div className="shrink-0 px-5 py-3">
        <CurrencyBar state={state} />
      </div>

      {/* Main Content */}
      <div className="flex-1 min-h-0 flex px-5 pb-4 gap-4">
        {/* Left Sidebar — Click + Progress + Datacenter */}
        <div className="w-72 shrink-0 flex flex-col gap-3 min-h-0">
          <ClickArea tier={state.tier} />
          <TierProgress tier={state.tier} computeUnits={state.compute_units} coloCount={state.colo_count} />
          <DonatePanel state={state} />
        </div>

        {/* Right Content — Tabbed */}
        <div className="flex-1 min-h-0 flex flex-col">
          {/* Tab Bar */}
          <div className="shrink-0 flex gap-1 mb-3">
            {TABS.filter(tab => tab.id !== 'market' || state.money > 0 || state.bitcoin_balance > 0).map(tab => {
              const isActive = activeTab === tab.id;
              return (
                <button
                  key={tab.id}
                  onClick={() => setActiveTab(tab.id)}
                  className="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-t transition-all"
                  style={{
                    background: isActive ? 'var(--bg-panel)' : 'transparent',
                    color: isActive ? tab.color : 'var(--text-secondary)',
                    borderBottom: isActive ? `2px solid ${tab.color}` : '2px solid transparent',
                  }}
                >
                  <span className="font-mono text-xs opacity-60">{tab.icon}</span>
                  {tab.label}
                </button>
              );
            })}
          </div>

          {/* Tab Content */}
          <div className="flex-1 min-h-0 overflow-hidden">
            {activeTab === 'hardware' && <HardwarePanel state={state} />}
            {activeTab === 'services' && <ServicePanel state={state} />}
            {activeTab === 'upgrades' && <UpgradePanel state={state} />}
            {activeTab === 'saas' && <SaasPanel state={state} />}
            {activeTab === 'datacenter' && <DatacenterPanel state={state} />}
            {activeTab === 'market' && <MarketPanel state={state} />}
            {activeTab === 'social' && <SocialPanel />}
          </div>
        </div>
      </div>

      <EventLog />
    </div>
  );
}
