// Progression tiers
export enum Tier {
  CoffeeTable = 'coffee_table',
  ClosetFloor = 'closet_floor',
  Rack12U = 'rack_12u',
  Rack24U = 'rack_24u',
  Rack36U = 'rack_36u',
  Rack48U = 'rack_48u',
}

// Currency types
export interface Currencies {
  computeUnits: number;
  reputation: number;
  powerWatts: number;
  money: number;
}

// Game state returned by the server
export interface GameState {
  id: string;
  userId: string;
  tier: Tier;
  currencies: Currencies;
  hardwareSlots: number;
  usedSlots: number;
  rackUnits: number | null; // null for pre-rack tiers
  usedRackUnits: number | null;
  coloCount: number;
  coloMultiplier: number;
  createdAt: string;
  updatedAt: string;
}

// Actions the client can send
export enum ActionType {
  RunJob = 'run_job',
  BuyHardware = 'buy_hardware',
  DeployService = 'deploy_service',
  UpgradeComponent = 'upgrade_component',
  UpgradeRack = 'upgrade_rack',
  UpgradeTier = 'upgrade_tier',
  Colo = 'colo',
}

export interface GameAction {
  type: ActionType;
  payload?: Record<string, unknown>;
}
