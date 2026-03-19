import { create } from 'zustand';
import { api, type GameState } from '../api';

export interface GameEvent {
  type: string;
  name: string;
  description: string;
  severity: string;
  tier_category: string;
}

interface GameStore {
  state: GameState | null;
  loading: boolean;
  error: string | null;
  token: string | null;
  user: { id: string; email: string; display_name: string } | null;
  events: GameEvent[];

  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, displayName: string) => Promise<void>;
  logout: () => void;
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
  buildDatacenter: () => Promise<void>;
  upgradeDatacenter: () => Promise<void>;
  addEvent: (event: GameEvent) => void;
  dismissEvent: (index: number) => void;
}

export const useGameStore = create<GameStore>((set, get) => ({
  state: null,
  loading: false,
  error: null,
  token: localStorage.getItem('token'),
  user: null,
  events: [],

  login: async (email, password) => {
    set({ loading: true, error: null });
    try {
      const res = await api.login(email, password);
      localStorage.setItem('token', res.token);
      set({ token: res.token, user: res.user, loading: false });
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
      set({ error: (e as Error).message });
    }
  },

  runJob: async () => {
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
    const s = get().state;
    if (!s) return;
    const deployed = new Set(s.services?.map(svc => svc.name) || []);
    const available = (s.available_services || []).filter(svc => !deployed.has(svc.name));

    for (const svc of available) {
      try {
        const state = await api.action('deploy_service', { name: svc.name });
        set({ state, error: null });
      } catch {
        break;
      }
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
    const s = get().state;
    if (!s) return;
    const owned = new Set(s.upgrades?.map(u => u.name) || []);
    const available = (s.available_upgrades || []).filter(u => !owned.has(u.name) && (!type || u.type === type));

    for (const u of available) {
      try {
        const state = await api.action('buy_upgrade', { name: u.name });
        set({ state, error: null });
      } catch {
        break;
      }
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
    const s = get().state;
    if (!s) return;
    const upgradeableTypes = ['server', 'desktop', 'sbc', 'mini_pc', 'gpu_server'];
    const components = ['cpu', 'ram', 'storage', 'nic'];
    const upgradeableHardware = (s.hardware || []).filter(h => upgradeableTypes.includes(h.type));

    for (const h of upgradeableHardware) {
      for (const comp of components) {
        // Keep upgrading until it fails (max level or out of CU)
        let success = true;
        while (success) {
          try {
            const state = await api.action('upgrade_component', { hardware_id: h.id, component: comp });
            set({ state, error: null });
          } catch {
            success = false;
          }
        }
      }
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
    const s = get().state;
    if (!s) return;
    const availableSaas = s.available_saas || [];
    const deployed = new Set((s.services || []).filter(svc => availableSaas.some(t => t.name === svc.name)).map(svc => svc.name));
    const undeployed = availableSaas.filter(svc => !deployed.has(svc.name));

    for (const svc of undeployed) {
      try {
        const state = await api.action('deploy_saas', { name: svc.name });
        set({ state, error: null });
      } catch {
        break;
      }
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
