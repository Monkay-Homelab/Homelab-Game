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
  buyBitcoin: (amount: number) => Promise<void>;
  sellBitcoin: (amount: number) => Promise<void>;
  addEvent: (event: GameEvent) => void;
  dismissEvent: (index: number) => void;
}

// Timestamp of the last action response to prevent stale poll data from overwriting it.
// When an action (buy_bitcoin, etc.) completes and sets state, the 5-second fetchState poll
// may return data from BEFORE the action was processed (request overlap). This guard ensures
// poll responses arriving within a short window after an action are discarded.
let _lastActionAt = 0;

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
      const requestedAt = Date.now();
      const state = await api.getState();
      // If an action completed while this poll was in-flight, discard the stale poll response.
      // The action response already set the authoritative state.
      if (_lastActionAt > requestedAt) {
        return;
      }
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
      _lastActionAt = Date.now();
      set({ state, error: null });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  buyHardware: async (name) => {
    set({ error: null });
    try {
      const state = await api.action('buy_hardware', { name });
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  sellHardware: async (id) => {
    set({ error: null });
    try {
      const state = await api.action('sell_hardware', { id });
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  deployService: async (name) => {
    set({ error: null });
    try {
      const state = await api.action('deploy_service', { name });
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  deployAllServices: async () => {
    set({ error: null });
    try {
      const state = await api.action('bulk_deploy_services');
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  buyUpgrade: async (name) => {
    set({ error: null });
    try {
      const state = await api.action('buy_upgrade', { name });
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  buyAllUpgrades: async (type) => {
    set({ error: null });
    try {
      const state = await api.action('bulk_buy_upgrades', type ? { type } : {});
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  upgradeComponent: async (hardwareId, component) => {
    set({ error: null });
    try {
      const state = await api.action('upgrade_component', { hardware_id: hardwareId, component });
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  upgradeAllComponents: async () => {
    set({ error: null });
    try {
      const state = await api.action('bulk_upgrade_components');
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  resolveEvent: async () => {
    set({ error: null });
    try {
      const state = await api.action('resolve_event');
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  unlockSaas: async () => {
    set({ error: null });
    try {
      const state = await api.action('unlock_saas');
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  deploySaas: async (name) => {
    set({ error: null });
    try {
      const state = await api.action('deploy_saas', { name });
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  deployAllSaas: async () => {
    set({ error: null });
    try {
      const state = await api.action('bulk_deploy_saas');
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  upgradeTier: async () => {
    set({ error: null });
    try {
      const state = await api.action('upgrade_tier');
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  colo: async () => {
    set({ error: null });
    try {
      const state = await api.action('colo');
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  donateCU: async (amount: number) => {
    set({ error: null });
    try {
      const state = await api.action('donate_cu', { amount });
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  buildDatacenter: async () => {
    set({ error: null });
    try {
      const state = await api.action('build_datacenter');
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  upgradeDatacenter: async () => {
    set({ error: null });
    try {
      const state = await api.action('upgrade_datacenter');
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  buyBitcoin: async (amount) => {
    set({ error: null });
    try {
      const state = await api.action('buy_bitcoin', { amount });
      _lastActionAt = Date.now();
      set({ state });
    } catch (e) {
      set({ error: (e as Error).message });
    }
  },

  sellBitcoin: async (amount) => {
    set({ error: null });
    try {
      const state = await api.action('sell_bitcoin', { amount });
      _lastActionAt = Date.now();
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
