package catalog

import "github.com/homelab-game/backend/internal/models"

type UpgradeTemplate struct {
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	MinTier     models.Tier `json:"min_tier"`
	Persistent  bool       `json:"persistent"`
	Cost        int64      `json:"cost"`
	CostType    string     `json:"cost_type"` // "compute" or "money"
	Description string     `json:"description"`
	Effect      string     `json:"effect"`
}

// Cooling upgrades — increase cooling_capacity
var CoolingUpgrades = []UpgradeTemplate{
	{Name: "USB Fan", Type: "cooling", MinTier: models.TierCoffeeTable, Cost: 30, Description: "A cheap USB fan pointed at your server", Effect: "+75 cooling"},
	{Name: "Box Fan", Type: "cooling", MinTier: models.TierClosetFloor, Cost: 100, Description: "Box fan propping the closet door open", Effect: "+150 cooling"},
	{Name: "Blanking Panels", Type: "cooling", MinTier: models.TierRack12U, Cost: 500, Description: "Proper airflow management in the rack", Effect: "+250 cooling"},
	{Name: "In-Rack Fans", Type: "cooling", MinTier: models.TierRack12U, Cost: 2000, Description: "Rack-mounted fan units", Effect: "+500 cooling"},
	{Name: "Portable AC Unit", Type: "cooling", MinTier: models.TierRack24U, Cost: 8000, Description: "Dedicated AC for the server room", Effect: "+1000 cooling"},
	{Name: "Mini Split AC", Type: "cooling", MinTier: models.TierRack36U, Cost: 25000, Description: "Proper HVAC for your lab", Effect: "+2000 cooling"},
	{Name: "In-Row Cooling", Type: "cooling", MinTier: models.TierRack48U, Cost: 80000, Description: "Datacenter-grade precision cooling", Effect: "+3750 cooling"},
}

// Cooling capacity values matching each upgrade
var CoolingValues = map[string]int{
	"USB Fan":          75,
	"Box Fan":          150,
	"Blanking Panels":  250,
	"In-Rack Fans":     500,
	"Portable AC Unit": 1000,
	"Mini Split AC":    2000,
	"In-Row Cooling":   3750,
}

// Networking upgrades — increase network_tier (0-4)
var NetworkUpgrades = []UpgradeTemplate{
	{Name: "Unmanaged Switch", Type: "networking", MinTier: models.TierClosetFloor, Cost: 200, Description: "Basic gigabit switch", Effect: "Network Tier 1"},
	{Name: "Managed Switch", Type: "networking", MinTier: models.TierRack12U, Cost: 2000, Description: "VLANs, QoS, monitoring", Effect: "Network Tier 2"},
	{Name: "10GbE Switch", Type: "networking", MinTier: models.TierRack36U, Cost: 8000, Description: "10 gigabit backbone", Effect: "Network Tier 3"},
	{Name: "Fiber Network", Type: "networking", MinTier: models.TierRack48U, Cost: 20000, Description: "Full fiber infrastructure", Effect: "Network Tier 4"},
}

// Network tier values
var NetworkTierValues = map[string]int{
	"Unmanaged Switch": 1,
	"Managed Switch":   2,
	"10GbE Switch":     3,
	"Fiber Network":    4,
}

// Automation upgrades — persistent through prestige, increase idle_multiplier
var AutomationUpgrades = []UpgradeTemplate{
	{Name: "Bash Scripts", Type: "automation", MinTier: models.TierCoffeeTable, Persistent: false, Cost: 100, Description: "Basic shell scripts for common tasks", Effect: "1.2x idle multiplier"},
	{Name: "Ansible Playbooks", Type: "automation", MinTier: models.TierClosetFloor, Persistent: false, Cost: 1000, Description: "Configuration management automation", Effect: "1.5x idle multiplier"},
	{Name: "Docker Compose", Type: "automation", MinTier: models.TierRack12U, Persistent: false, Cost: 5000, Description: "Containerized deployments", Effect: "2.0x idle multiplier"},
	{Name: "Kubernetes", Type: "automation", MinTier: models.TierRack36U, Persistent: false, Cost: 50000, Description: "Full container orchestration", Effect: "3.0x idle multiplier"},
}

var AutomationMultipliers = map[string]float64{
	"Bash Scripts":      1.2,
	"Ansible Playbooks": 1.5,
	"Docker Compose":    2.0,
	"Kubernetes":        3.0,
}

// Knowledge upgrades — persistent through prestige
var KnowledgeUpgrades = []UpgradeTemplate{
	{Name: "CompTIA A+", Type: "knowledge", MinTier: models.TierCoffeeTable, Persistent: true, Cost: 200, CostType: "money", Description: "Basic IT fundamentals", Effect: "+10% job reward"},
	{Name: "Linux Basics", Type: "knowledge", MinTier: models.TierCoffeeTable, Persistent: true, Cost: 300, CostType: "money", Description: "Command line proficiency", Effect: "+15% job reward"},
	{Name: "Networking CCNA", Type: "knowledge", MinTier: models.TierClosetFloor, Persistent: true, Cost: 2000, CostType: "money", Description: "Cisco networking certification", Effect: "+20% job reward"},
	{Name: "AWS/Cloud Cert", Type: "knowledge", MinTier: models.TierRack12U, Persistent: true, Cost: 8000, CostType: "money", Description: "Cloud architecture knowledge", Effect: "+25% job reward"},
	{Name: "RHCE", Type: "knowledge", MinTier: models.TierRack24U, Persistent: true, Cost: 20000, CostType: "money", Description: "Red Hat Certified Engineer", Effect: "+30% job reward"},
	{Name: "CKA", Type: "knowledge", MinTier: models.TierRack36U, Persistent: true, Cost: 60000, CostType: "money", Description: "Certified Kubernetes Administrator", Effect: "+40% job reward"},
}

var KnowledgePointValues = map[string]int{
	"CompTIA A+":       10,
	"Linux Basics":     15,
	"Networking CCNA":  20,
	"AWS/Cloud Cert":   25,
	"RHCE":             30,
	"CKA":              40,
}

// Component upgrade costs per level
type ComponentUpgradeInfo struct {
	Component   string `json:"component"`
	MaxLevel    int    `json:"max_level"`
	BaseCost    int64  `json:"base_cost"`
	CostScale   float64 `json:"cost_scale"`
	ComputeAdd  int    `json:"compute_add"`
	PowerReduce int    `json:"power_reduce"`
}

var ComponentUpgrades = []ComponentUpgradeInfo{
	{Component: "cpu", MaxLevel: 5, BaseCost: 500, CostScale: 2.0, ComputeAdd: 5, PowerReduce: 0},
	{Component: "ram", MaxLevel: 5, BaseCost: 300, CostScale: 2.0, ComputeAdd: 3, PowerReduce: 0},
	{Component: "storage", MaxLevel: 5, BaseCost: 400, CostScale: 2.0, ComputeAdd: 2, PowerReduce: 0},
	{Component: "nic", MaxLevel: 3, BaseCost: 600, CostScale: 2.5, ComputeAdd: 1, PowerReduce: 5},
}

func GetComponentUpgradeInfo(component string) *ComponentUpgradeInfo {
	for _, c := range ComponentUpgrades {
		if c.Component == component {
			return &c
		}
	}
	return nil
}

func GetAvailableUpgrades(tier models.Tier) []UpgradeTemplate {
	tierRank := TierToRank(tier)
	var all []UpgradeTemplate
	for _, lists := range [][]UpgradeTemplate{CoolingUpgrades, NetworkUpgrades, AutomationUpgrades, KnowledgeUpgrades} {
		for _, u := range lists {
			if TierToRank(u.MinTier) <= tierRank {
				all = append(all, u)
			}
		}
	}
	return all
}
