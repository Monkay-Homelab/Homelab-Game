import { create } from 'zustand';
import { api, type GameState, type GameConfig } from '../api';

export interface GameEvent {
  type: string;
  name: string;
  description: string;
  severity: string;
  tier_category: string;
}

interface GameStore {
  state: GameState | null;
  config: GameConfig | null;
  loading: boolean;
  error: string | null;
  token: string | null;
  user: { id: string; email: string; display_name: string } | null;
  events: GameEvent[];

  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, displayName: string) => Promise<void>;
  logout: () => void;
  fetchConfig: () => Promise<void>;
  fetchState: () => Promise<void>;
  runJob: () => Promise<void>;
  buyHardware: (name: string) => Promise<void>;
  sellHardware: (id: string) => Promise<void>;
  deployService: (name: string) => Promise<void>;
  deployAllServices: () => Promise<void>;
  buyUpgrade: (name: string) => Promise<void>;
  buyAllUpgrades: (type?: string) => Promise<void>;
  upgradeComponent: (hardwareId: string, component: string) => Promise<void>;
  upgradeAllComponents: () => Promise<void>;
  resolveEvent: () => Promise<void>;
  unlockSaas: () => Promise<void>;
  deploySaas: (name: string) => Promise<void>;
  deployAllSaas: () => Promise<void>;
  upgradeTier: () => Promise<void>;
  colo: () => Promise<void>;
  donateCU: (amount: number) => Promise<void>;
  buildDatacenter: () => Promise<void>;
  upgradeDatacenter: () => Promise<void>;
  addEvent: (event: GameEvent) => void;
  dismissEvent: (index: number) => void;
}

export const useGameStore = create<GameStore>((set, get) => ({
  state: null,
  config: null,
  loading: false,
  error: null,
  token: localStorage.getItem('token'),
  user: null,
  events: [],

  fetchConfig: async () => {
    if (get().config) return;
    try {
      const config = await api.getConfig();
      set({ config });
    } catch {
      setTimeout(() => get().fetchConfig(), 3000);
    }
  },

  login: async (email, password) => {
    set({ loading: true, error: null });
    try {
      const res = await api.login(email, password);
      localStorage.setItem('token', res.token);
      set({ token: res.token, user: res.user, loading: false });
      get().fetchConfig();
      await get().fetchState();
    } catch (e) {
      set({ error: (e as Error).message, loading: false });
    }
  },

  register: async (email, password, displayName) => {
    set({ loading: true, error: null });
    try {
      const res = await api.register(email, password, displayName);
      localStorage.setItem('token', res.token);
      set({ token: res.token, user: res.user, loading: false });
      get().fetchConfig();
      await get().fetchState();
    } catch (e) {
      set({ error: (e as Error).message, loading: false });
    }
  },

  logout: () => {
    localStorage.removeItem('token');
    set({ token: null, user: null, state: null });
  },

  fetchState: async () => {
    try {
      const state = await api.getState();
      set({ state, error: null });
    } catch (e) {
      const msg = (e as Error).message;
      if (msg.includes('not found')) {
        localStorage.removeItem('token');
        set({ token: null, user: null, state: null, error: null });
        return;
      }
      set({ error: msg });
    }
  },

  runJob: async () => {
    // Optimistically add click reward locally for instant feedback
    const s = get().state;
    const cfg = get().config;
    if (s && cfg) {
      const tierCfg = cfg.tiers.find(t => t.id === s.tier);
      const reward = tierCfg?.job_reward || 10;
      const knowledgeBoost = 1 + s.knowledge_points / cfg.gameplay.knowledge_boost_divisor;
      set({ state: { ...s, compute_units: s.compute_units + Math.floor(reward * knowledgeBoost) } });
    }
    try {
      const state = await api.action('run_job');
      set({ state, error: null });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  buyHardware: async (name) => {
    set({ error: null });
    try {
      const state = await api.action('buy_hardware', { name });
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  sellHardware: async (id) => {
    set({ error: null });
    try {
      const state = await api.action('sell_hardware', { id });
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  deployService: async (name) => {
    set({ error: null });
    try {
      const state = await api.action('deploy_service', { name });
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  deployAllServices: async () => {
    set({ error: null });
    try {
      const state = await api.action('bulk_deploy_services');
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  buyUpgrade: async (name) => {
    set({ error: null });
    try {
      const state = await api.action('buy_upgrade', { name });
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  buyAllUpgrades: async (type) => {
    set({ error: null });
    try {
      const state = await api.action('bulk_buy_upgrades', type ? { type } : {});
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  upgradeComponent: async (hardwareId, component) => {
    set({ error: null });
    try {
      const state = await api.action('upgrade_component', { hardware_id: hardwareId, component });
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  upgradeAllComponents: async () => {
    set({ error: null });
    try {
      const state = await api.action('bulk_upgrade_components');
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  resolveEvent: async () => {
    set({ error: null });
    try {
      const state = await api.action('resolve_event');
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  unlockSaas: async () => {
    set({ error: null });
    try {
      const state = await api.action('unlock_saas');
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  deploySaas: async (name) => {
    set({ error: null });
    try {
      const state = await api.action('deploy_saas', { name });
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  deployAllSaas: async () => {
    set({ error: null });
    try {
      const state = await api.action('bulk_deploy_saas');
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  upgradeTier: async () => {
    set({ error: null });
    try {
      const state = await api.action('upgrade_tier');
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  colo: async () => {
    set({ error: null });
    try {
      const state = await api.action('colo');
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  donateCU: async (amount: number) => {
    set({ error: null });
    try {
      const state = await api.action('donate_cu', { amount });
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  buildDatacenter: async () => {
    set({ error: null });
    try {
      const state = await api.action('build_datacenter');
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  upgradeDatacenter: async () => {
    set({ error: null });
    try {
      const state = await api.action('upgrade_datacenter');
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  addEvent: (event) => {
    set(s => ({ events: [...s.events, event].slice(-10) })); // keep last 10
  },

  dismissEvent: (index) => {
    set(s => ({ events: s.events.filter((_, i) => i !== index) }));
  },
}));
