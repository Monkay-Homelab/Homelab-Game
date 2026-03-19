package catalog

import "github.com/homelab-game/backend/internal/models"

// SaaS services that generate money from customers
type SaasServiceTemplate struct {
	Name              string     `json:"name"`
	Type              string     `json:"type"`
	MinTier           models.Tier `json:"min_tier"`
	DeployCost        int64      `json:"deploy_cost"`
	ReputationRequired int64     `json:"reputation_required"`
	RevenuePerCustomer int64     `json:"revenue_per_customer"`
	MaxCustomers      int        `json:"max_customers"`
	PowerRequired     int        `json:"power_required"`
	Description       string     `json:"description"`
}

var SaasServices = []SaasServiceTemplate{
	// 12U Rack — basic hosting
	{Name: "Email Hosting", Type: "email", MinTier: models.TierRack12U, DeployCost: 5000, ReputationRequired: 100, RevenuePerCustomer: 3, MaxCustomers: 50, PowerRequired: 20, Description: "Business email hosting"},
	{Name: "Web Hosting", Type: "web", MinTier: models.TierRack12U, DeployCost: 4000, ReputationRequired: 80, RevenuePerCustomer: 2, MaxCustomers: 100, PowerRequired: 15, Description: "Shared web hosting for small sites"},

	// 24U Rack — growing services
	{Name: "VPS Hosting", Type: "vps", MinTier: models.TierRack24U, DeployCost: 15000, ReputationRequired: 200, RevenuePerCustomer: 8, MaxCustomers: 50, PowerRequired: 60, Description: "Virtual private servers for customers"},
	{Name: "S3-Compatible Storage", Type: "storage", MinTier: models.TierRack24U, DeployCost: 12000, ReputationRequired: 150, RevenuePerCustomer: 5, MaxCustomers: 100, PowerRequired: 50, Description: "Object storage for customers"},

	// 36U Rack — enterprise services
	{Name: "Managed Database", Type: "database", MinTier: models.TierRack36U, DeployCost: 30000, ReputationRequired: 500, RevenuePerCustomer: 15, MaxCustomers: 30, PowerRequired: 100, Description: "PostgreSQL/MySQL hosting as a service"},
	{Name: "Managed Kubernetes", Type: "k8s", MinTier: models.TierRack36U, DeployCost: 40000, ReputationRequired: 600, RevenuePerCustomer: 25, MaxCustomers: 20, PowerRequired: 120, Description: "Managed K8s clusters for customers"},

	// 48U Rack — big league
	{Name: "Bare Metal Hosting", Type: "baremetal", MinTier: models.TierRack48U, DeployCost: 60000, ReputationRequired: 800, RevenuePerCustomer: 40, MaxCustomers: 15, PowerRequired: 200, Description: "Dedicated servers for enterprise clients"},
	{Name: "GPU Cloud", Type: "gpu", MinTier: models.TierRack48U, DeployCost: 100000, ReputationRequired: 1000, RevenuePerCustomer: 100, MaxCustomers: 10, PowerRequired: 500, Description: "GPU instances for AI/ML workloads"},
}

func GetSaasServiceByName(name string) *SaasServiceTemplate {
	for _, s := range SaasServices {
		if s.Name == name {
			return &s
		}
	}
	return nil
}

func GetAvailableSaasServices(tier models.Tier) []SaasServiceTemplate {
	tierRank := TierToRank(tier)
	var available []SaasServiceTemplate
	for _, s := range SaasServices {
		if TierToRank(s.MinTier) <= tierRank {
			available = append(available, s)
		}
	}
	return available
}

// Customer name pools for random generation
var CustomerFirstNames = []string{
	"Alex", "Jordan", "Sam", "Casey", "Riley", "Morgan", "Taylor", "Quinn",
	"Avery", "Blake", "Drew", "Ellis", "Frankie", "Gray", "Harper", "Indigo",
}

var CustomerLastNames = []string{
	"Tech", "Digital", "Systems", "Solutions", "Labs", "Works", "Cloud", "Net",
	"Data", "Dev", "Ops", "Stack", "Byte", "Code", "Logic", "Soft",
}

// Business expense templates
type ExpenseTemplate struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	CostPerTick int64  `json:"cost_per_tick"`
	Description string `json:"description"`
}

var BusinessExpenses = []ExpenseTemplate{
	{Name: "Business Internet", Type: "infrastructure", CostPerTick: 2, Description: "Business-class internet connection"},
	{Name: "Domain Registrations", Type: "infrastructure", CostPerTick: 1, Description: "Domain names for your services"},
	{Name: "SSL Certificates", Type: "infrastructure", CostPerTick: 1, Description: "Wildcard SSL certs"},
	{Name: "Business Insurance", Type: "legal", CostPerTick: 3, Description: "Liability insurance for hosting"},
	{Name: "Accounting Software", Type: "operations", CostPerTick: 1, Description: "Track revenue and expenses"},
}
