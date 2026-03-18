import { Tier } from '../types';

// Hardware slot limits per tier (pre-rack)
export const HARDWARE_SLOTS: Record<string, number> = {
  [Tier.CoffeeTable]: 2,
  [Tier.ClosetFloor]: 5,
};

// Rack unit capacities
export const RACK_SIZES = [12, 24, 36, 48] as const;

// Power limits per tier (watts)
export const POWER_LIMITS: Record<string, number> = {
  [Tier.CoffeeTable]: 200,
  [Tier.ClosetFloor]: 500,
  [Tier.Rack12U]: 1500,
  [Tier.Rack24U]: 3000,
  [Tier.Rack36U]: 5000,
  [Tier.Rack48U]: 8000,
};

// Colo multiplier formula: 1 + (coloCount * 0.5) / (1 + coloCount * 0.1)
// Diminishing returns
export const COLO_BASE_MULTIPLIER = 0.5;
export const COLO_DIMINISHING_FACTOR = 0.1;
