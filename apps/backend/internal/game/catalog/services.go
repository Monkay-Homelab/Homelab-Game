package catalog

import "github.com/homelab-game/backend/internal/models"

type ServiceTemplate struct {
	Name              string     `json:"name"`
	Type              string     `json:"type"`
	MinTier           models.Tier `json:"min_tier"`
	ComputePerTick    int64      `json:"compute_per_tick"`
	ReputationPerTick int64      `json:"reputation_per_tick"`
	MoneyPerTick      int64      `json:"money_per_tick"`
	PowerRequired     int        `json:"power_required"`
	Cost              int64      `json:"cost"`
}

var Services = []ServiceTemplate{
	// Coffee Table
	{Name: "Pi-hole", Type: "dns", MinTier: models.TierCoffeeTable, ComputePerTick: 1, ReputationPerTick: 1, PowerRequired: 5, Cost: 20},
	{Name: "Personal Website", Type: "web", MinTier: models.TierCoffeeTable, ComputePerTick: 1, ReputationPerTick: 2, PowerRequired: 5, Cost: 30},
	{Name: "File Share", Type: "storage", MinTier: models.TierCoffeeTable, ComputePerTick: 2, ReputationPerTick: 1, PowerRequired: 10, Cost: 50},

	// Closet Floor
	{Name: "Plex", Type: "media", MinTier: models.TierClosetFloor, ComputePerTick: 5, ReputationPerTick: 5, MoneyPerTick: 1, PowerRequired: 30, Cost: 200},
	{Name: "Home Assistant", Type: "automation", MinTier: models.TierClosetFloor, ComputePerTick: 3, ReputationPerTick: 4, PowerRequired: 15, Cost: 150},
	{Name: "Nextcloud", Type: "cloud", MinTier: models.TierClosetFloor, ComputePerTick: 4, ReputationPerTick: 5, PowerRequired: 25, Cost: 250},
	{Name: "Game Server", Type: "gaming", MinTier: models.TierClosetFloor, ComputePerTick: 8, ReputationPerTick: 6, MoneyPerTick: 1, PowerRequired: 40, Cost: 300},

	// 12U Rack
	{Name: "Gitea", Type: "devtools", MinTier: models.TierRack12U, ComputePerTick: 10, ReputationPerTick: 8, MoneyPerTick: 2, PowerRequired: 20, Cost: 800},
	{Name: "Grafana + Prometheus", Type: "monitoring", MinTier: models.TierRack12U, ComputePerTick: 8, ReputationPerTick: 10, PowerRequired: 30, Cost: 1000},
	{Name: "Reverse Proxy", Type: "networking", MinTier: models.TierRack12U, ComputePerTick: 5, ReputationPerTick: 12, PowerRequired: 10, Cost: 500},
	{Name: "WireGuard VPN", Type: "vpn", MinTier: models.TierRack12U, ComputePerTick: 3, ReputationPerTick: 8, MoneyPerTick: 2, PowerRequired: 5, Cost: 400},
	{Name: "TrueNAS", Type: "storage", MinTier: models.TierRack12U, ComputePerTick: 12, ReputationPerTick: 10, PowerRequired: 50, Cost: 1500},

	// 24U Rack
	{Name: "CI/CD Pipeline", Type: "devtools", MinTier: models.TierRack24U, ComputePerTick: 20, ReputationPerTick: 15, MoneyPerTick: 3, PowerRequired: 40, Cost: 3000},
	{Name: "Docker Swarm", Type: "orchestration", MinTier: models.TierRack24U, ComputePerTick: 25, ReputationPerTick: 18, PowerRequired: 50, Cost: 4000},
	{Name: "Mail Server", Type: "communication", MinTier: models.TierRack24U, ComputePerTick: 10, ReputationPerTick: 20, PowerRequired: 20, Cost: 2500},
	{Name: "Matrix/Element", Type: "communication", MinTier: models.TierRack24U, ComputePerTick: 15, ReputationPerTick: 18, PowerRequired: 30, Cost: 3000},
	{Name: "Frigate NVR", Type: "security", MinTier: models.TierRack24U, ComputePerTick: 18, ReputationPerTick: 12, PowerRequired: 35, Cost: 3500},

	// 36U Rack
	{Name: "Kubernetes Cluster", Type: "orchestration", MinTier: models.TierRack36U, ComputePerTick: 60, ReputationPerTick: 40, PowerRequired: 100, Cost: 12000},
	{Name: "ELK Stack", Type: "monitoring", MinTier: models.TierRack36U, ComputePerTick: 40, ReputationPerTick: 30, PowerRequired: 80, Cost: 8000},
	{Name: "DNS Authority", Type: "dns", MinTier: models.TierRack36U, ComputePerTick: 20, ReputationPerTick: 35, PowerRequired: 15, Cost: 5000},
	{Name: "Database Cluster", Type: "database", MinTier: models.TierRack36U, ComputePerTick: 50, ReputationPerTick: 35, PowerRequired: 90, Cost: 10000},

	// 48U Rack
	{Name: "AI/ML Training", Type: "compute", MinTier: models.TierRack48U, ComputePerTick: 150, ReputationPerTick: 60, PowerRequired: 200, Cost: 50000},
	{Name: "CDN Node", Type: "networking", MinTier: models.TierRack48U, ComputePerTick: 80, ReputationPerTick: 80, PowerRequired: 100, Cost: 30000},
	{Name: "Mastodon Instance", Type: "social", MinTier: models.TierRack48U, ComputePerTick: 60, ReputationPerTick: 100, PowerRequired: 80, Cost: 25000},
	{Name: "Full IaC", Type: "devtools", MinTier: models.TierRack48U, ComputePerTick: 100, ReputationPerTick: 50, PowerRequired: 50, Cost: 20000},
}

func GetServiceByName(name string) *ServiceTemplate {
	for _, s := range Services {
		if s.Name == name {
			return &s
		}
	}
	return nil
}

func GetAvailableServices(tier models.Tier) []ServiceTemplate {
	tierRank := TierToRank(tier)
	var available []ServiceTemplate
	for _, s := range Services {
		if TierToRank(s.MinTier) <= tierRank {
			available = append(available, s)
		}
	}
	return available
}
