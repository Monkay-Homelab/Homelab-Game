package catalog

import "github.com/homelab-game/backend/internal/models"

type HardwareTemplate struct {
	Name           string     `json:"name"`
	Type           string     `json:"type"`
	MinTier        models.Tier `json:"min_tier"`
	SlotsUsed      int        `json:"slots_used"`
	RackUnitsUsed  *int       `json:"rack_units_used"`
	PowerDraw      int        `json:"power_draw"`
	ComputePerTick int64      `json:"compute_per_tick"`
	Cost           int64      `json:"cost"`
}

func intPtr(i int) *int { return &i }

var Hardware = []HardwareTemplate{
	// Coffee Table tier
	{Name: "Raspberry Pi 4", Type: "sbc", MinTier: models.TierCoffeeTable, SlotsUsed: 1, PowerDraw: 15, ComputePerTick: 1, Cost: 50},
	{Name: "N100 Mini PC", Type: "mini_pc", MinTier: models.TierCoffeeTable, SlotsUsed: 1, PowerDraw: 25, ComputePerTick: 6, Cost: 200},

	// Closet Floor tier
	{Name: "HP ProDesk Mini", Type: "mini_pc", MinTier: models.TierClosetFloor, SlotsUsed: 2, PowerDraw: 45, ComputePerTick: 8, Cost: 400},
	{Name: "Lenovo ThinkCentre", Type: "desktop", MinTier: models.TierClosetFloor, SlotsUsed: 2, PowerDraw: 80, ComputePerTick: 12, Cost: 600},
	{Name: "Synology NAS", Type: "nas", MinTier: models.TierClosetFloor, SlotsUsed: 1, PowerDraw: 40, ComputePerTick: 3, Cost: 500},
	{Name: "APC Back-UPS 600VA", Type: "ups", MinTier: models.TierClosetFloor, SlotsUsed: 1, PowerDraw: 0, ComputePerTick: 0, Cost: 300},

	// 12U Rack tier
	{Name: "Dell PowerEdge R620", Type: "server", MinTier: models.TierRack12U, RackUnitsUsed: intPtr(1), PowerDraw: 200, ComputePerTick: 25, Cost: 1500},
	{Name: "HP ProLiant DL360", Type: "server", MinTier: models.TierRack12U, RackUnitsUsed: intPtr(1), PowerDraw: 220, ComputePerTick: 30, Cost: 2000},
	{Name: "Unmanaged Switch 8-port", Type: "switch", MinTier: models.TierClosetFloor, SlotsUsed: 1, PowerDraw: 10, ComputePerTick: 0, Cost: 100},
	{Name: "Unmanaged Switch 24-port", Type: "switch", MinTier: models.TierRack12U, RackUnitsUsed: intPtr(1), PowerDraw: 15, ComputePerTick: 0, Cost: 500},
	{Name: "2U JBOD Storage Shelf", Type: "nas", MinTier: models.TierRack12U, RackUnitsUsed: intPtr(2), PowerDraw: 80, ComputePerTick: 5, Cost: 1200},
	{Name: "CyberPower UPS 1500VA", Type: "ups", MinTier: models.TierRack12U, RackUnitsUsed: intPtr(2), PowerDraw: 0, ComputePerTick: 0, Cost: 800},
	{Name: "1U Patch Panel", Type: "patch_panel", MinTier: models.TierRack12U, RackUnitsUsed: intPtr(1), PowerDraw: 0, ComputePerTick: 0, Cost: 200},
	{Name: "1U Rack Shelf", Type: "shelf", MinTier: models.TierRack12U, RackUnitsUsed: intPtr(1), PowerDraw: 0, ComputePerTick: 0, Cost: 150},
	{Name: "Mac Mini M4", Type: "server", MinTier: models.TierRack12U, SlotsUsed: 1, PowerDraw: 40, ComputePerTick: 30, Cost: 6969},

	// 24U Rack tier
	{Name: "Dell PowerEdge R730", Type: "server", MinTier: models.TierRack24U, RackUnitsUsed: intPtr(2), PowerDraw: 350, ComputePerTick: 60, Cost: 5000},
	{Name: "Managed Switch 24-port", Type: "switch", MinTier: models.TierRack24U, RackUnitsUsed: intPtr(1), PowerDraw: 30, ComputePerTick: 0, Cost: 1500},
	{Name: "Synology RackStation", Type: "nas", MinTier: models.TierRack24U, RackUnitsUsed: intPtr(2), PowerDraw: 100, ComputePerTick: 10, Cost: 3000},

	// 36U Rack tier
	{Name: "Dell PowerEdge R740xd", Type: "server", MinTier: models.TierRack36U, RackUnitsUsed: intPtr(2), PowerDraw: 500, ComputePerTick: 120, Cost: 15000},
	{Name: "10GbE Switch", Type: "switch", MinTier: models.TierRack36U, RackUnitsUsed: intPtr(1), PowerDraw: 50, ComputePerTick: 0, Cost: 5000},
	{Name: "APC UPS 3000VA", Type: "ups", MinTier: models.TierRack36U, RackUnitsUsed: intPtr(2), PowerDraw: 0, ComputePerTick: 0, Cost: 4000},

	// 48U Rack tier
	{Name: "Dell PowerEdge R750", Type: "server", MinTier: models.TierRack48U, RackUnitsUsed: intPtr(2), PowerDraw: 700, ComputePerTick: 250, Cost: 40000},
	{Name: "GPU Server (4x A100)", Type: "gpu_server", MinTier: models.TierRack48U, RackUnitsUsed: intPtr(4), PowerDraw: 2000, ComputePerTick: 500, Cost: 100000},
	{Name: "Fiber Switch 48-port", Type: "switch", MinTier: models.TierRack48U, RackUnitsUsed: intPtr(1), PowerDraw: 80, ComputePerTick: 0, Cost: 15000},
	{Name: "AWS at Home", Type: "server", MinTier: models.TierRack48U, RackUnitsUsed: intPtr(48), Powerdraw: 20000, ComputePerTick: 10000, Cost: 10000000},
}

func GetHardwareByName(name string) *HardwareTemplate {
	for _, h := range Hardware {
		if h.Name == name {
			return &h
		}
	}
	return nil
}

func GetAvailableHardware(tier models.Tier) []HardwareTemplate {
	tierRank := TierToRank(tier)
	var available []HardwareTemplate
	for _, h := range Hardware {
		if TierToRank(h.MinTier) <= tierRank {
			available = append(available, h)
		}
	}
	return available
}

func TierToRank(tier models.Tier) int {
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
