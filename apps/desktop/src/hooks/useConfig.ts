import { useGameStore } from '../stores/gameStore';
import type { GameConfig, TierConfig } from '../api';

export function useConfig(): GameConfig {
  const config = useGameStore((s) => s.config);
  if (!config) throw new Error('Config not loaded');
  return config;
}

export function prestigeScale(config: GameConfig, coloCount: number): number {
  const p = config.prestige;
  if (coloCount <= p.linear_cap) {
    return 1.0 + coloCount * p.linear_increment;
  }
  return p.base * Math.pow(p.exponential_base, coloCount - p.linear_cap);
}

export function getTier(config: GameConfig, tierId: string): TierConfig | undefined {
  return config.tiers.find((t) => t.id === tierId);
}

export function tierLabel(config: GameConfig, tierId: string): string {
  return getTier(config, tierId)?.label || tierId;
}
