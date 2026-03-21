const API_URL = import.meta.env.VITE_API_URL || 'https://api.homelab.living';

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = localStorage.getItem('token');
  const res = await fetch(`${API_URL}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers,
    },
  });

  if (res.status === 401) {
    localStorage.removeItem('token');
    window.location.reload();
    throw new Error('Session expired');
  }

  const data = await res.json();
  if (!res.ok) throw new Error(data.error || 'Request failed');
  return data;
}

export interface AuthResponse {
  token: string;
  user: { id: string; email: string; display_name: string };
}

export interface GameState {
  id: string;
  user_id: string;
  tier: string;
  compute_units: number;
  reputation: number;
  power_watts: number;
  power_limit: number;
  money: number;
  hardware_slots: number;
  used_slots: number;
  rack_units: number | null;
  used_rack_units: number | null;
  colo_count: number;
  colo_multiplier: number;
  heat_generated: number;
  cooling_capacity: number;
  network_tier: number;
  automation_tier: number;
  knowledge_points: number;
  idle_multiplier: number;
  saas_unlocked: boolean;
  total_customers: number;
  throttle_multiplier: number;
  throttle_ticks_remaining: number;
  datacenter_tier: number;
  owns_datacenter: boolean;
  datacenter_level: number;
  datacenter_income_multiplier: number;
  total_donated_cu: number;
  hardware: HardwareItem[];
  colo_racks: ColoRackItem[];
  services: ServiceItem[];
  upgrades: UpgradeItem[];
  component_upgrades: ComponentUpgradeItem[];
  customers: CustomerItem[];
  expenses: ExpenseItem[];
  available_hardware: HardwareTemplate[];
  available_services: ServiceTemplate[];
  available_upgrades: UpgradeTemplate[];
  available_saas: SaasTemplate[];
  overheating: boolean;
  throttled: boolean;
  group_bonus: number;
  group_members: number;
  global_donated_cu: number;
  bitcoin_balance: number;
  bitcoin_price: number;
  bitcoin_price_history: BitcoinPricePoint[];
}

export interface BitcoinPricePoint {
  time: string;
  price: number;
}

export interface ColoRackItem {
  id: string;
  datacenter_tier: number;
  rack_size: number;
  compute_per_tick: number;
  reputation_per_tick: number;
  money_per_tick: number;
  colo_at: string;
}

export interface ComponentUpgradeItem {
  id: string;
  hardware_id: string;
  component: string;
  level: number;
  compute_bonus: number;
  power_reduction: number;
}

export interface UpgradeItem {
  id: string;
  name: string;
  type: string;
  persistent: boolean;
}

export interface CustomerItem {
  id: string;
  name: string;
  service_type: string;
  monthly_revenue: number;
  satisfaction: number;
}

export interface ExpenseItem {
  id: string;
  name: string;
  type: string;
  cost_per_tick: number;
}

export interface UpgradeTemplate {
  name: string;
  type: string;
  min_tier: string;
  cost: number;
  cost_type: string;
  description: string;
  effect: string;
  persistent: boolean;
}

export interface SaasTemplate {
  name: string;
  type: string;
  min_tier: string;
  deploy_cost: number;
  reputation_required: number;
  revenue_per_customer: number;
  max_customers: number;
  power_required: number;
  description: string;
}

export interface HardwareItem {
  id: string;
  name: string;
  type: string;
  power_draw: number;
  compute_per_tick: number;
  slots_used: number;
  rack_units_used: number | null;
}

export interface ServiceItem {
  id: string;
  name: string;
  type: string;
  compute_per_tick: number;
  reputation_per_tick: number;
  money_per_tick: number;
}

export interface HardwareTemplate {
  name: string;
  type: string;
  min_tier: string;
  slots_used: number;
  rack_units_used: number | null;
  power_draw: number;
  compute_per_tick: number;
  cost: number;
}

export interface ServiceTemplate {
  name: string;
  type: string;
  min_tier: string;
  compute_per_tick: number;
  reputation_per_tick: number;
  money_per_tick: number;
  power_required: number;
  cost: number;
}

export interface GroupInfo {
  group: { id: string; name: string; founder_id: string; min_contribution: number; profit_split: number } | null;
  members: { group_id: string; user_id: string; role: string; display_name: string }[];
  my_role: string;
  compute_pool: number;
}

export interface LeaderboardData {
  category: string;
  entries: { id: string; user_id: string; username: string; category: string; score: number; rank: number }[];
}

export interface GameConfig {
  tiers: TierConfig[];
  hardware_bonuses: HardwareBonusConfig;
  prestige: PrestigeConfig;
  saas_unlock: SaasUnlockConfig;
  datacenter: DatacenterConfig;
  gameplay: GameplayConfig;
  leaderboard: LeaderboardConfig;
  group: GroupConfig;
  bitcoin: BitcoinConfig;
}

export interface BitcoinConfig {
  min_price: number;
  max_price: number;
  step_interval: number;
  mean_price: number;
}

export interface TierConfig {
  id: string;
  label: string;
  rank: number;
  base_upgrade_cost: number;
  job_reward: number;
  power_limit: number;
  cooling_bonus: number;
  jobs: string[];
}

export interface HardwareBonusConfig {
  ups_compute: Record<string, number>;
  network_income: Record<string, number>;
  storage_rep: Record<string, number>;
  patch_panel_bonus: number;
}

export interface PrestigeConfig {
  linear_cap: number;
  linear_increment: number;
  base: number;
  exponential_base: number;
}

export interface SaasUnlockConfig {
  base_cost: number;
  reputation_required: number;
}

export interface DatacenterConfig {
  build_money_cost: number;
  build_compute_cost: number;
  upgrade_money_base: number;
  upgrade_compute_base: number;
  min_colo_count: number;
  max_level: number;
  income_multiplier_step: number;
  tier_names: Record<number, string>;
  level_names: Record<number, string>;
}

export interface GameplayConfig {
  shelf_slots: number;
  throttle_resolve_cost_per_tick: number;
  heat_penalty: number;
  knowledge_boost_divisor: number;
  colo_rack_decay: number;
  sell_refund_percent: number;
  max_colo_count: number;
  base_cooling: number;
}

export interface LeaderboardConfig {
  categories: { id: string; label: string }[];
}

export interface GroupConfig {
  bonus_per_member: number;
  max_bonus: number;
  description: string;
}

export const api = {
  getConfig: () => request<GameConfig>('/api/game/config'),

  register: (email: string, password: string, displayName: string) =>
    request<AuthResponse>('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, password, display_name: displayName }),
    }),

  login: (email: string, password: string) =>
    request<AuthResponse>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    }),

  getState: () => request<GameState>('/api/game/state'),

  action: (type: string, payload?: Record<string, unknown>) =>
    request<GameState>('/api/game/action', {
      method: 'POST',
      body: JSON.stringify({ type, payload }),
    }),

  // Social
  getMyGroup: () => request<GroupInfo>('/api/social/group'),
  listGroups: () => request<{ groups: GroupInfo['group'][] }>('/api/social/groups'),
  createGroup: (name: string) => request<GroupInfo>('/api/social/group/create', { method: 'POST', body: JSON.stringify({ name }) }),
  joinGroup: (name: string) => request<GroupInfo>('/api/social/group/join', { method: 'POST', body: JSON.stringify({ name }) }),
  leaveGroup: () => request<{ ok: boolean }>('/api/social/group/leave', { method: 'POST' }),
  promoteMember: (userId: string) => request<{ members: GroupInfo['members'] }>('/api/social/group/promote', { method: 'POST', body: JSON.stringify({ user_id: userId }) }),
  kickMember: (userId: string) => request<{ members: GroupInfo['members'] }>('/api/social/group/kick', { method: 'POST', body: JSON.stringify({ user_id: userId }) }),
  getLeaderboard: (category: string) =>
    request<LeaderboardData>(`/api/social/leaderboard?category=${encodeURIComponent(category)}`),
  updateLeaderboard: () => request<{ ok: boolean }>('/api/social/leaderboard/update', { method: 'POST' }),
};
