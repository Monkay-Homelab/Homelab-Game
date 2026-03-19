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

export const api = {
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
  promoteMemeber: (userId: string) => request<any>('/api/social/group/promote', { method: 'POST', body: JSON.stringify({ user_id: userId }) }),
  kickMember: (userId: string) => request<any>('/api/social/group/kick', { method: 'POST', body: JSON.stringify({ user_id: userId }) }),
  getLeaderboard: (category: string) => request<LeaderboardData>(`/api/social/leaderboard?category=${category}`),
  updateLeaderboard: () => request<{ ok: boolean }>('/api/social/leaderboard/update', { method: 'POST' }),
};
