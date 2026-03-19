package events

import (
	"encoding/json"
	"math/rand"

	"github.com/homelab-game/backend/internal/models"
)

// GameEvent represents an event that can happen to a player.
type GameEvent struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Tier        string          `json:"tier_category"`
	Severity    string          `json:"severity"` // minor, moderate, major
	Effect      *EventEffect    `json:"effect"`
	Mitigation  *EventMitigation `json:"mitigation,omitempty"`
}

// EventEffect describes the impact of an event.
type EventEffect struct {
	ComputeLoss    int64   `json:"compute_loss,omitempty"`
	ReputationLoss int64   `json:"reputation_loss,omitempty"`
	MoneyLoss      int64   `json:"money_loss,omitempty"`
	Throttle       float64 `json:"throttle,omitempty"`       // multiplier (e.g., 0.5 = 50% slowdown)
	ThrottleTicks  int     `json:"throttle_ticks,omitempty"` // how many ticks the throttle lasts
	SlotLoss       int     `json:"slot_loss,omitempty"`      // temporarily lose hardware slots
}

// EventMitigation describes what prevents or reduces the event.
type EventMitigation struct {
	UpgradeName string `json:"upgrade_name,omitempty"` // owning this upgrade prevents the event
	HardwareType string `json:"hardware_type,omitempty"` // owning this hardware type mitigates
	Description string `json:"description"`
}

// Coffee Table Events
var coffeeTableEvents = []GameEvent{
	{
		Type: "cat_attack", Name: "Cat Knocked the Server Off!",
		Description: "Your cat jumped on the coffee table and knocked your server to the floor.",
		Tier: "coffee_table", Severity: "minor",
		Effect: &EventEffect{Throttle: 0, ThrottleTicks: 3},
	},
	{
		Type: "noise_complaint", Name: "Noise Complaint",
		Description: "Your partner/roommate is tired of the server fan noise.",
		Tier: "coffee_table", Severity: "minor",
		Effect:     &EventEffect{SlotLoss: 1, ThrottleTicks: 5},
		Mitigation: &EventMitigation{UpgradeName: "USB Fan", Description: "Quiet fan reduces noise"},
	},
	{
		Type: "spilled_drink", Name: "Spilled Drink!",
		Description: "Someone spilled a drink near the server. Close call!",
		Tier: "coffee_table", Severity: "moderate",
		Effect: &EventEffect{ComputeLoss: 50},
	},
	{
		Type: "power_flicker", Name: "Power Flickered!",
		Description: "The lights flickered and your server rebooted.",
		Tier: "coffee_table", Severity: "minor",
		Effect:     &EventEffect{Throttle: 0, ThrottleTicks: 2},
		Mitigation: &EventMitigation{HardwareType: "ups", Description: "UPS battery keeps it running"},
	},
}

// Closet Floor Events
var closetFloorEvents = []GameEvent{
	{
		Type: "overheating", Name: "Closet Overheating!",
		Description: "No airflow in the closet. Your gear is throttling.",
		Tier: "closet_floor", Severity: "moderate",
		Effect:     &EventEffect{Throttle: 0.5, ThrottleTicks: 10},
		Mitigation: &EventMitigation{UpgradeName: "Box Fan", Description: "Fan provides airflow"},
	},
	{
		Type: "tripped_breaker", Name: "Tripped Breaker!",
		Description: "Too many machines on one circuit. Everything went dark.",
		Tier: "closet_floor", Severity: "major",
		Effect:     &EventEffect{Throttle: 0, ThrottleTicks: 5, ReputationLoss: 10},
		Mitigation: &EventMitigation{UpgradeName: "Dedicated Circuit", Description: "Separate circuit prevents this"},
	},
	{
		Type: "cable_spaghetti", Name: "Cable Spaghetti",
		Description: "Can't find which cable goes where. Troubleshooting takes longer.",
		Tier: "closet_floor", Severity: "minor",
		Effect:     &EventEffect{Throttle: 0.75, ThrottleTicks: 8},
		Mitigation: &EventMitigation{UpgradeName: "Cable Organizer", Description: "Labels and velcro ties help"},
	},
	{
		Type: "power_outage_closet", Name: "Power Outage!",
		Description: "Power went out. Everything in the closet is down.",
		Tier: "closet_floor", Severity: "major",
		Effect:     &EventEffect{Throttle: 0, ThrottleTicks: 6, ReputationLoss: 20},
		Mitigation: &EventMitigation{HardwareType: "ups", Description: "UPS battery keeps servers running"},
	},
	{
		Type: "spouse_aggro", Name: "Electricity Bill Shock!",
		Description: "The electricity bill came in. Your partner is not happy.",
		Tier: "closet_floor", Severity: "moderate",
		Effect: &EventEffect{MoneyLoss: 100, SlotLoss: 1, ThrottleTicks: 10},
	},
}

// Rack Tier Events
var rackEvents = []GameEvent{
	{
		Type: "power_outage", Name: "Power Outage!",
		Description: "Power went out in the neighborhood.",
		Tier: "rack", Severity: "major",
		Effect:     &EventEffect{Throttle: 0, ThrottleTicks: 8, ReputationLoss: 50},
		Mitigation: &EventMitigation{HardwareType: "ups", Description: "UPS keeps servers running"},
	},
	{
		Type: "drive_failure", Name: "Drive Failure!",
		Description: "One of your drives just died.",
		Tier: "rack", Severity: "major",
		Effect:     &EventEffect{ComputeLoss: 500, ReputationLoss: 30},
		Mitigation: &EventMitigation{HardwareType: "nas", Description: "RAID/backup storage saves your data"},
	},
	{
		Type: "isp_outage", Name: "ISP Outage",
		Description: "Your internet provider is down. Everything is offline.",
		Tier: "rack", Severity: "major",
		Effect: &EventEffect{Throttle: 0, ThrottleTicks: 12, ReputationLoss: 40},
	},
	{
		Type: "noise_neighbors", Name: "Noise Complaint from Neighbors",
		Description: "The rack is louder than expected. Neighbors are complaining.",
		Tier: "rack", Severity: "minor",
		Effect:     &EventEffect{Throttle: 0.8, ThrottleTicks: 6},
		Mitigation: &EventMitigation{UpgradeName: "In-Rack Fans", Description: "Quieter fans reduce noise"},
	},
	{
		Type: "firmware_brick", Name: "Firmware Update Bricked a Device",
		Description: "A switch firmware update went wrong. It won't boot.",
		Tier: "rack", Severity: "moderate",
		Effect: &EventEffect{ComputeLoss: 200, ThrottleTicks: 5},
	},
}

// Software Events (any tier)
var softwareEvents = []GameEvent{
	{
		Type: "kernel_panic", Name: "Kernel Update Gone Wrong",
		Description: "Your server won't boot after a kernel update.",
		Tier: "software", Severity: "moderate",
		Effect: &EventEffect{Throttle: 0, ThrottleTicks: 6},
	},
	{
		Type: "security_breach", Name: "Security Breach!",
		Description: "An exposed service got compromised. Time to clean up.",
		Tier: "software", Severity: "major",
		Effect: &EventEffect{ComputeLoss: 300, ReputationLoss: 100},
	},
	{
		Type: "dns_misconfig", Name: "DNS Misconfiguration",
		Description: "Services are unreachable. DNS is pointing to the void.",
		Tier: "software", Severity: "moderate",
		Effect: &EventEffect{Throttle: 0.5, ThrottleTicks: 8, ReputationLoss: 20},
	},
	{
		Type: "cert_expired", Name: "Certificate Expired",
		Description: "HTTPS services are showing scary warnings.",
		Tier: "software", Severity: "minor",
		Effect:     &EventEffect{ReputationLoss: 15, ThrottleTicks: 4},
		Mitigation: &EventMitigation{UpgradeName: "Bash Scripts", Description: "Automated certbot renewal prevents this"},
	},
	{
		Type: "dependency_broke", Name: "Dependency Broke Overnight",
		Description: "A container image update broke a service.",
		Tier: "software", Severity: "minor",
		Effect: &EventEffect{Throttle: 0.75, ThrottleTicks: 5, ComputeLoss: 50},
	},
}

// SaaS/IaaS Events
var saasEvents = []GameEvent{
	{
		Type: "hug_of_death", Name: "Reddit Hug of Death!",
		Description: "One of your hosted services went viral. Massive traffic incoming!",
		Tier: "saas", Severity: "moderate",
		Effect: &EventEffect{Throttle: 0.3, ThrottleTicks: 10, ReputationLoss: 50},
	},
	{
		Type: "support_ticket", Name: "Customer Support Ticket",
		Description: "A paying customer has an urgent issue with their service.",
		Tier: "saas", Severity: "minor",
		Effect: &EventEffect{ReputationLoss: 10, MoneyLoss: 50},
	},
	{
		Type: "enterprise_inquiry", Name: "Enterprise Client Inquiry!",
		Description: "A large company wants to use your hosting. Big opportunity!",
		Tier: "saas", Severity: "minor",
		Effect: &EventEffect{}, // Positive event — handled specially
	},
	{
		Type: "chargeback", Name: "Chargeback Filed!",
		Description: "A customer is disputing a payment. Time and money to resolve.",
		Tier: "saas", Severity: "moderate",
		Effect: &EventEffect{MoneyLoss: 200, ReputationLoss: 20},
	},
	{
		Type: "tos_abuse", Name: "TOS Abuse Detected",
		Description: "A customer is using your hosting for something sketchy.",
		Tier: "saas", Severity: "minor",
		Effect: &EventEffect{ReputationLoss: 30},
	},
}

// Colo Events (only fire when player has colo'd racks)
var coloEvents = []GameEvent{
	{
		Type: "dc_maintenance", Name: "Datacenter Maintenance Window",
		Description: "Scheduled downtime at the datacenter. Your colo'd rack goes offline briefly.",
		Tier: "colo", Severity: "minor",
		Effect: &EventEffect{ReputationLoss: 20, MoneyLoss: 50},
	},
	{
		Type: "bandwidth_overage", Name: "Bandwidth Overage!",
		Description: "Your colo'd rack exceeded its bandwidth allocation.",
		Tier: "colo", Severity: "moderate",
		Effect: &EventEffect{MoneyLoss: 500},
	},
	{
		Type: "remote_hands", Name: "Remote Hands Needed",
		Description: "Hardware issue at the datacenter. Can't just walk over.",
		Tier: "colo", Severity: "moderate",
		Effect: &EventEffect{MoneyLoss: 300, Throttle: 0.8, ThrottleTicks: 5},
	},
	{
		Type: "cross_connect", Name: "Cross-Connect Request",
		Description: "Another colo tenant wants to peer. Free bandwidth boost!",
		Tier: "colo", Severity: "minor",
		Effect: &EventEffect{}, // Positive — no damage
	},
	{
		Type: "lease_renewal", Name: "Lease Renewal",
		Description: "Your colo contract is up for renewal. Negotiate or pay up.",
		Tier: "colo", Severity: "moderate",
		Effect: &EventEffect{MoneyLoss: 1000},
	},
}

// GetEventsForTier returns all possible events for a given tier.
func GetEventsForTier(tier models.Tier, saasUnlocked bool, coloCount int) []GameEvent {
	var events []GameEvent

	// Software events can happen at any tier
	events = append(events, softwareEvents...)

	switch tier {
	case models.TierCoffeeTable:
		events = append(events, coffeeTableEvents...)
	case models.TierClosetFloor:
		events = append(events, coffeeTableEvents...)
		events = append(events, closetFloorEvents...)
	default:
		// Rack tiers get rack events + closet events (not coffee table)
		events = append(events, closetFloorEvents...)
		events = append(events, rackEvents...)
	}

	if saasUnlocked {
		events = append(events, saasEvents...)
	}

	if coloCount > 0 {
		events = append(events, coloEvents...)
	}

	return events
}

// RollEvent randomly decides if an event should fire and which one.
// Returns nil if no event fires. Chance is per-tick (called from idle progress).
func RollEvent(tier models.Tier, saasUnlocked bool, coloCount int, elapsedSeconds float64) *GameEvent {
	// ~2% per poll. With 5s polling (~12 polls/min), expect one event every 1-2 minutes.
	chance := 0.02

	if rand.Float64() > chance {
		return nil
	}

	events := GetEventsForTier(tier, saasUnlocked, coloCount)
	if len(events) == 0 {
		return nil
	}

	// Weight by severity: minor=3, moderate=2, major=1
	type weighted struct {
		event  GameEvent
		weight int
	}
	var pool []weighted
	totalWeight := 0
	for _, e := range events {
		w := 3
		switch e.Severity {
		case "moderate":
			w = 2
		case "major":
			w = 1
		}
		pool = append(pool, weighted{e, w})
		totalWeight += w
	}

	roll := rand.Intn(totalWeight)
	for _, p := range pool {
		roll -= p.weight
		if roll < 0 {
			return &p.event
		}
	}

	return &events[0]
}

// IsMitigated checks if the player has the upgrade or hardware to prevent the event.
func IsMitigated(event *GameEvent, upgrades []models.Upgrade, hardware []models.Hardware) bool {
	if event.Mitigation == nil {
		return false
	}

	if event.Mitigation.UpgradeName != "" {
		for _, u := range upgrades {
			if u.Name == event.Mitigation.UpgradeName {
				return true
			}
		}
	}

	if event.Mitigation.HardwareType != "" {
		for _, h := range hardware {
			if h.Type == event.Mitigation.HardwareType {
				return true
			}
		}
	}

	return false
}

// ApplyEvent applies the event's effects to the game state. Returns the event as JSON for logging.
func ApplyEvent(gs *models.GameState, event *GameEvent) json.RawMessage {
	if event.Effect.ComputeLoss > 0 {
		gs.ComputeUnits -= event.Effect.ComputeLoss
		if gs.ComputeUnits < 0 {
			gs.ComputeUnits = 0
		}
	}
	if event.Effect.ReputationLoss > 0 {
		gs.Reputation -= event.Effect.ReputationLoss
		if gs.Reputation < 0 {
			gs.Reputation = 0
		}
	}
	if event.Effect.MoneyLoss > 0 {
		gs.Money -= event.Effect.MoneyLoss
		if gs.Money < 0 {
			gs.Money = 0
		}
	}

	data, _ := json.Marshal(event)
	return data
}
