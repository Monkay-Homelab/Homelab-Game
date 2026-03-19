package engine

import "github.com/homelab-game/backend/internal/models"

// --- Package-level maps for hardware bonuses (used by both ProcessIdleProgress and GetConfig) ---

var UpsComputeBonus = map[string]int64{
	"APC Back-UPS 600VA":    3,
	"CyberPower UPS 1500VA": 8,
	"APC UPS 3000VA":        20,
}

var NetworkIncomeBonus = map[string]float64{
	"Unmanaged Switch 8-port":  0.10,
	"Unmanaged Switch 24-port": 0.14,
	"Managed Switch 24-port":   0.20,
	"10GbE Switch":             0.25,
	"Fiber Switch 48-port":     0.30,
}

var StorageRepBonus = map[string]float64{
	"Synology NAS":          0.10,
	"2U JBOD Storage Shelf": 0.15,
	"Synology RackStation":  0.25,
}

const PatchPanelBonusValue = 0.05

// --- Config types ---

type GameConfig struct {
	Tiers           []TierConfig        `json:"tiers"`
	HardwareBonuses HardwareBonusConfig `json:"hardware_bonuses"`
	Prestige        PrestigeConfig      `json:"prestige"`
	SaasUnlock      SaasUnlockConfig    `json:"saas_unlock"`
	Datacenter      DatacenterConfig    `json:"datacenter"`
	Gameplay        GameplayConfig      `json:"gameplay"`
	Leaderboard     LeaderboardConfig   `json:"leaderboard"`
	Group           GroupConfig         `json:"group"`
}

type TierConfig struct {
	ID              string   `json:"id"`
	Label           string   `json:"label"`
	Rank            int      `json:"rank"`
	BaseUpgradeCost int64    `json:"base_upgrade_cost"`
	JobReward       int64    `json:"job_reward"`
	PowerLimit      int      `json:"power_limit"`
	CoolingBonus    int      `json:"cooling_bonus"`
	Jobs            []string `json:"jobs"`
}

type HardwareBonusConfig struct {
	UpsCompute      map[string]int64   `json:"ups_compute"`
	NetworkIncome   map[string]float64 `json:"network_income"`
	StorageRep      map[string]float64 `json:"storage_rep"`
	PatchPanelBonus float64            `json:"patch_panel_bonus"`
}

type PrestigeConfig struct {
	LinearCap       int     `json:"linear_cap"`
	LinearIncrement float64 `json:"linear_increment"`
	Base            float64 `json:"base"`
	ExponentialBase float64 `json:"exponential_base"`
}

type SaasUnlockConfig struct {
	BaseCost           int64 `json:"base_cost"`
	ReputationRequired int64 `json:"reputation_required"`
}

type DatacenterConfig struct {
	BuildMoneyCost       int64          `json:"build_money_cost"`
	BuildComputeCost     int64          `json:"build_compute_cost"`
	UpgradeMoneyBase     int64          `json:"upgrade_money_base"`
	UpgradeComputeBase   int64          `json:"upgrade_compute_base"`
	MinColoCount         int            `json:"min_colo_count"`
	MaxLevel             int            `json:"max_level"`
	IncomeMultiplierStep float64        `json:"income_multiplier_step"`
	TierNames            map[int]string `json:"tier_names"`
	LevelNames           map[int]string `json:"level_names"`
}

type GameplayConfig struct {
	ShelfSlots                int     `json:"shelf_slots"`
	ThrottleResolveCostPerTick int64  `json:"throttle_resolve_cost_per_tick"`
	HeatPenalty               float64 `json:"heat_penalty"`
	KnowledgeBoostDivisor     float64 `json:"knowledge_boost_divisor"`
	ColoRackDecay             float64 `json:"colo_rack_decay"`
	SellRefundPercent         float64 `json:"sell_refund_percent"`
	MaxColoCount              int     `json:"max_colo_count"`
	BaseCooling               int     `json:"base_cooling"`
}

type LeaderboardConfig struct {
	Categories []LeaderboardCategory `json:"categories"`
}

type LeaderboardCategory struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type GroupConfig struct {
	BonusPerMember float64 `json:"bonus_per_member"`
	MaxBonus       float64 `json:"max_bonus"`
	Description    string  `json:"description"`
}

// Tier metadata including labels and flavor text
var tierMeta = []struct {
	tier  models.Tier
	label string
	jobs  []string
}{
	{models.TierCoffeeTable, "Coffee Table", []string{"Compiling a script...", "Running apt update...", "Pinging localhost...", "Downloading ISO..."}},
	{models.TierClosetFloor, "Closet Floor", []string{"Transcoding video...", "Building Docker image...", "Running backup...", "Indexing media library..."}},
	{models.TierRack12U, "12U Rack", []string{"Deploying containers...", "Running Ansible playbook...", "Syncing NAS...", "Processing logs..."}},
	{models.TierRack24U, "24U Rack", []string{"CI/CD pipeline running...", "Swarm service scaling...", "Mail queue processing...", "Camera feed analyzing..."}},
	{models.TierRack36U, "36U Rack", []string{"K8s pod scheduling...", "ELK ingesting logs...", "DB cluster rebalancing...", "DNS zone transfer..."}},
	{models.TierRack48U, "48U Rack", []string{"Training ML model...", "CDN cache warming...", "Federation sync...", "Terraform applying..."}},
}

// GetConfig builds the full game configuration from authoritative sources.
func GetConfig() *GameConfig {
	// Build tier configs from the engine's own functions
	tiers := make([]TierConfig, 0, len(tierMeta))
	for _, tm := range tierMeta {
		// Look up base upgrade cost: this is the cost TO UPGRADE FROM this tier
		var baseCost int64
		_, cost, ok := nextTier(tm.tier)
		if ok {
			baseCost = cost
		}

		tiers = append(tiers, TierConfig{
			ID:              string(tm.tier),
			Label:           tm.label,
			Rank:            tierToRank(tm.tier),
			BaseUpgradeCost: baseCost,
			JobReward:       tierJobReward(tm.tier),
			PowerLimit:      tierPowerLimit(tm.tier),
			CoolingBonus:    tierCoolingBonus(tm.tier),
			Jobs:            tm.jobs,
		})
	}

	return &GameConfig{
		Tiers: tiers,
		HardwareBonuses: HardwareBonusConfig{
			UpsCompute:      UpsComputeBonus,
			NetworkIncome:   NetworkIncomeBonus,
			StorageRep:      StorageRepBonus,
			PatchPanelBonus: PatchPanelBonusValue,
		},
		Prestige: PrestigeConfig{
			LinearCap:       5,
			LinearIncrement: 0.5,
			Base:            3.5,
			ExponentialBase: 1.5,
		},
		SaasUnlock: SaasUnlockConfig{
			BaseCost:           10000,
			ReputationRequired: 100,
		},
		Datacenter: DatacenterConfig{
			BuildMoneyCost:       500000,
			BuildComputeCost:     5000000,
			UpgradeMoneyBase:     250000,
			UpgradeComputeBase:   2000000,
			MinColoCount:         5,
			MaxLevel:             5,
			IncomeMultiplierStep: 0.25,
			TierNames: map[int]string{
				0: "None",
				1: "Tier 1 — Basic",
				2: "Tier 2 — Redundant Power",
				3: "Tier 3 — Concurrently Maintainable",
				4: "Tier 4 — Fault Tolerant",
			},
			LevelNames: map[int]string{
				1: "Small Facility",
				2: "Medium Facility",
				3: "Large Facility",
				4: "Campus",
				5: "Hyperscale",
			},
		},
		Gameplay: GameplayConfig{
			ShelfSlots:                 8,
			ThrottleResolveCostPerTick: 100,
			HeatPenalty:               0.5,
			KnowledgeBoostDivisor:     100.0,
			ColoRackDecay:             0.9,
			SellRefundPercent:         0.6,
			MaxColoCount:              100,
			BaseCooling:               50,
		},
		Leaderboard: LeaderboardConfig{
			Categories: []LeaderboardCategory{
				{ID: "compute", Label: "Compute"},
				{ID: "reputation", Label: "Reputation"},
				{ID: "colo_count", Label: "Prestiges"},
				{ID: "money", Label: "Money"},
				{ID: "group", Label: "Groups"},
			},
		},
		Group: GroupConfig{
			BonusPerMember: 0.05,
			MaxBonus:       0.50,
			Description:    "+5% compute bonus per member, up to +50%.",
		},
	}
}

// tierToRank is the local version used by config builder (catalog.TierToRank is in another package)
func tierToRank(tier models.Tier) int {
	switch tier {
	case models.TierCoffeeTable:
		return 0
	case models.TierClosetFloor:
		return 1
	case models.TierRack12U:
		return 2
	case models.TierRack24U:
		return 3
	case models.TierRack36U:
		return 4
	case models.TierRack48U:
		return 5
	default:
		return 0
	}
}
