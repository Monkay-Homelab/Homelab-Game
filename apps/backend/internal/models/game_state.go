package models

import "time"

type Tier string

const (
	TierCoffeeTable Tier = "coffee_table"
	TierClosetFloor Tier = "closet_floor"
	TierRack12U     Tier = "rack_12u"
	TierRack24U     Tier = "rack_24u"
	TierRack36U     Tier = "rack_36u"
	TierRack48U     Tier = "rack_48u"
)

type GameState struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	Tier            Tier      `json:"tier"`
	ComputeUnits    int64     `json:"compute_units"`
	Reputation      int64     `json:"reputation"`
	PowerWatts      int       `json:"power_watts"`
	PowerLimit      int       `json:"power_limit"`
	Money           int64     `json:"money"`
	HardwareSlots   int       `json:"hardware_slots"`
	UsedSlots       int       `json:"used_slots"`
	RackUnits       *int      `json:"rack_units"`
	UsedRackUnits   *int      `json:"used_rack_units"`
	ColoCount       int       `json:"colo_count"`
	ColoMultiplier  float64   `json:"colo_multiplier"`
	HeatGenerated   int       `json:"heat_generated"`
	CoolingCapacity int       `json:"cooling_capacity"`
	NetworkTier     int       `json:"network_tier"`
	AutomationTier  int       `json:"automation_tier"`
	KnowledgePoints int       `json:"knowledge_points"`
	IdleMultiplier  float64   `json:"idle_multiplier"`
	SaasUnlocked          bool      `json:"saas_unlocked"`
	TotalCustomers        int       `json:"total_customers"`
	ThrottleMultiplier    float64   `json:"throttle_multiplier"`
	ThrottleTicksRemaining int      `json:"throttle_ticks_remaining"`
	OverclockMultiplier    float64   `json:"overclock_multiplier"`
	OverclockTicksRemaining int      `json:"overclock_ticks_remaining"`
	RackOptimization       int       `json:"rack_optimization"`
	DatacenterTier           int       `json:"datacenter_tier"`
	OwnsDatacenter           bool      `json:"owns_datacenter"`
	DatacenterLevel          int       `json:"datacenter_level"`
	DatacenterIncomeMultiplier float64   `json:"datacenter_income_multiplier"`
	TotalDonatedCU             int64     `json:"total_donated_cu"`
	BitcoinBalance             int64     `json:"bitcoin_balance"`
	LastCustomerGrowthAt       time.Time `json:"last_customer_growth_at"`
	LastTickAt                 time.Time `json:"last_tick_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ComponentUpgrade struct {
	ID             string    `json:"id"`
	HardwareID     string    `json:"hardware_id"`
	Component      string    `json:"component"`
	Level          int       `json:"level"`
	ComputeBonus   int       `json:"compute_bonus"`
	PowerReduction int       `json:"power_reduction"`
	UpgradedAt     time.Time `json:"upgraded_at"`
}

type Hardware struct {
	ID             string    `json:"id"`
	GameStateID    string    `json:"game_state_id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	Tier           Tier      `json:"tier"`
	SlotsUsed      int       `json:"slots_used"`
	RackUnitsUsed  *int      `json:"rack_units_used"`
	PowerDraw      int       `json:"power_draw"`
	ComputePerTick int64     `json:"compute_per_tick"`
	PurchasedAt    time.Time `json:"purchased_at"`
}

type Service struct {
	ID               string    `json:"id"`
	GameStateID      string    `json:"game_state_id"`
	Name             string    `json:"name"`
	Type             string    `json:"type"`
	Tier             Tier      `json:"tier"`
	ComputePerTick   int64     `json:"compute_per_tick"`
	ReputationPerTick int64    `json:"reputation_per_tick"`
	MoneyPerTick     int64     `json:"money_per_tick"`
	DeployedAt       time.Time `json:"deployed_at"`
}

type Upgrade struct {
	ID          string    `json:"id"`
	GameStateID string    `json:"game_state_id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Tier        Tier      `json:"tier"`
	Persistent  bool      `json:"persistent"`
	PurchasedAt time.Time `json:"purchased_at"`
}

type Customer struct {
	ID             string    `json:"id"`
	GameStateID    string    `json:"game_state_id"`
	Name           string    `json:"name"`
	ServiceType    string    `json:"service_type"`
	MonthlyRevenue int64     `json:"monthly_revenue"`
	Satisfaction   int       `json:"satisfaction"`
	SignedUpAt     time.Time `json:"signed_up_at"`
}

type Expense struct {
	ID          string    `json:"id"`
	GameStateID string    `json:"game_state_id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	CostPerTick int64     `json:"cost_per_tick"`
	CreatedAt   time.Time `json:"created_at"`
}

type ColoRack struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	DatacenterTier   int       `json:"datacenter_tier"`
	RackSize         int       `json:"rack_size"`
	ComputePerTick   int64     `json:"compute_per_tick"`
	ReputationPerTick int64    `json:"reputation_per_tick"`
	MoneyPerTick     int64     `json:"money_per_tick"`
	ColoAt           time.Time `json:"colo_at"`
}

type ResearchLevel struct {
	ID           string    `json:"id"`
	GameStateID  string    `json:"game_state_id"`
	ResearchNode string    `json:"research_node"`
	Level        int       `json:"level"`
	UpdatedAt    time.Time `json:"updated_at"`
}
