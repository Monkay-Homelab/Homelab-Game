package catalog

import "github.com/homelab-game/backend/internal/models"

// ResearchNode defines a single node in the research tree.
// Each node has infinite levels with exponential cost scaling.
// Research levels persist through prestige.
type ResearchNode struct {
	ID          string      `json:"id"`           // unique identifier (snake_case)
	Name        string      `json:"name"`         // display name
	Branch      string      `json:"branch"`       // branch grouping for UI
	MinTier     models.Tier `json:"min_tier"`      // tier gate
	BaseCost    int64       `json:"base_cost"`     // CU cost at level 0
	CostScale   float64     `json:"cost_scale"`    // exponential multiplier per level
	EffectType  string      `json:"effect_type"`   // which bonus this applies to
	EffectValue float64     `json:"effect_value"`  // bonus per level (e.g., 0.02 = +2%)
	Description string      `json:"description"`   // flavor text
}

// Research branches (UI grouping only, no mechanical interaction)
const (
	BranchEfficiency     = "efficiency"
	BranchReputation     = "reputation"
	BranchInfrastructure = "infrastructure"
	BranchMastery        = "mastery"
)

// Research effect types
const (
	EffectIdleIncome    = "idle_income"
	EffectReputationGain = "reputation_gain"
	EffectMoneyIncome   = "money_income"
	EffectJobReward     = "job_reward"
)

// ResearchNodes defines all research nodes in the tree.
var ResearchNodes = []ResearchNode{
	// Efficiency branch — boosts compute/idle income
	{
		ID:          "read_the_docs",
		Name:        "Read the Docs",
		Branch:      BranchEfficiency,
		MinTier:     models.TierCoffeeTable,
		BaseCost:    500,
		CostScale:   1.8,
		EffectType:  EffectIdleIncome,
		EffectValue: 0.02,
		Description: "Study the documentation to improve your efficiency",
	},
	{
		ID:          "lab_notebook",
		Name:        "Lab Notebook",
		Branch:      BranchEfficiency,
		MinTier:     models.TierClosetFloor,
		BaseCost:    2000,
		CostScale:   2.0,
		EffectType:  EffectIdleIncome,
		EffectValue: 0.03,
		Description: "Keep detailed notes on your lab configuration",
	},
	{
		ID:          "automated_testing",
		Name:        "Automated Testing",
		Branch:      BranchEfficiency,
		MinTier:     models.TierRack12U,
		BaseCost:    10000,
		CostScale:   2.2,
		EffectType:  EffectIdleIncome,
		EffectValue: 0.05,
		Description: "Automated testing catches problems before they cost you",
	},

	// Reputation branch — boosts reputation gain
	{
		ID:          "blog_writing",
		Name:        "Blog Writing",
		Branch:      BranchReputation,
		MinTier:     models.TierCoffeeTable,
		BaseCost:    500,
		CostScale:   1.8,
		EffectType:  EffectReputationGain,
		EffectValue: 0.03,
		Description: "Share your homelab journey with the community",
	},
	{
		ID:          "conference_talks",
		Name:        "Conference Talks",
		Branch:      BranchReputation,
		MinTier:     models.TierRack12U,
		BaseCost:    8000,
		CostScale:   2.0,
		EffectType:  EffectReputationGain,
		EffectValue: 0.05,
		Description: "Present your work at tech conferences",
	},
	{
		ID:          "open_source_contrib",
		Name:        "Open Source Contributions",
		Branch:      BranchReputation,
		MinTier:     models.TierRack24U,
		BaseCost:    30000,
		CostScale:   2.2,
		EffectType:  EffectReputationGain,
		EffectValue: 0.08,
		Description: "Contribute to the open source projects you depend on",
	},

	// Infrastructure branch — boosts money income
	{
		ID:          "chaos_engineering",
		Name:        "Chaos Engineering",
		Branch:      BranchInfrastructure,
		MinTier:     models.TierRack24U,
		BaseCost:    25000,
		CostScale:   2.0,
		EffectType:  EffectMoneyIncome,
		EffectValue: 0.04,
		Description: "Break things on purpose to build resilient infrastructure",
	},
	{
		ID:          "master_bgp_routing",
		Name:        "Master BGP Routing",
		Branch:      BranchInfrastructure,
		MinTier:     models.TierRack36U,
		BaseCost:    80000,
		CostScale:   2.2,
		EffectType:  EffectMoneyIncome,
		EffectValue: 0.06,
		Description: "Master the routing protocol that runs the internet",
	},

	// Mastery branch — boosts job click rewards
	{
		ID:          "scripting_mastery",
		Name:        "Scripting Mastery",
		Branch:      BranchMastery,
		MinTier:     models.TierCoffeeTable,
		BaseCost:    300,
		CostScale:   1.6,
		EffectType:  EffectJobReward,
		EffectValue: 0.03,
		Description: "Automate the boring stuff with scripting",
	},
	{
		ID:          "system_optimization",
		Name:        "System Optimization",
		Branch:      BranchMastery,
		MinTier:     models.TierRack12U,
		BaseCost:    5000,
		CostScale:   2.0,
		EffectType:  EffectJobReward,
		EffectValue: 0.05,
		Description: "Squeeze every drop of performance from your systems",
	},
}

// researchIndex is a pre-built map for O(1) lookups by ID.
var researchIndex map[string]*ResearchNode

func init() {
	researchIndex = make(map[string]*ResearchNode, len(ResearchNodes))
	for i := range ResearchNodes {
		researchIndex[ResearchNodes[i].ID] = &ResearchNodes[i]
	}
}

// GetResearchNode returns the research node with the given ID, or nil if not found.
func GetResearchNode(id string) *ResearchNode {
	return researchIndex[id]
}

// GetAvailableResearchNodes returns all research nodes with MinTier <= the given tier.
func GetAvailableResearchNodes(tier models.Tier) []ResearchNode {
	tierRank := TierToRank(tier)
	var available []ResearchNode
	for _, n := range ResearchNodes {
		if TierToRank(n.MinTier) <= tierRank {
			available = append(available, n)
		}
	}
	return available
}
