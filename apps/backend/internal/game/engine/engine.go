package engine

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/homelab-game/backend/internal/game/catalog"
	"github.com/homelab-game/backend/internal/game/events"
	"github.com/homelab-game/backend/internal/models"
)

type Engine struct{}

func New() *Engine {
	return &Engine{}
}

// ProcessIdleProgress calculates resources earned since the last tick.
// Returns any events that triggered during the elapsed time.
func (e *Engine) ProcessIdleProgress(gs *models.GameState, hardware []models.Hardware, services []models.Service, upgrades []models.Upgrade, expenses []models.Expense, customers []models.Customer, now time.Time) []*events.GameEvent {
	elapsed := now.Sub(gs.LastTickAt)
	if elapsed <= 0 {
		return nil
	}

	seconds := elapsed.Seconds()

	// Decay throttle over time
	if gs.ThrottleTicksRemaining > 0 {
		gs.ThrottleTicksRemaining--
		if gs.ThrottleTicksRemaining <= 0 {
			gs.ThrottleMultiplier = 1.0
			gs.ThrottleTicksRemaining = 0
		}
	}

	// Compute from hardware and recalculate heat from actual power draw
	var hardwareCompute int64
	var totalHeat int
	for _, h := range hardware {
		hardwareCompute += h.ComputePerTick
		totalHeat += h.PowerDraw
	}

	// Compute and reputation from services
	var serviceCompute, serviceRep, serviceMoney int64
	for _, s := range services {
		serviceCompute += s.ComputePerTick
		serviceRep += s.ReputationPerTick
		serviceMoney += s.MoneyPerTick
	}

	// Heat = total power draw (recalculated every tick to stay accurate)
	gs.HeatGenerated = gs.PowerWatts

	// Recalculate cooling capacity from base + tier + owned cooling upgrades
	baseCooling := 50
	tierBonus := tierCoolingBonus(gs.Tier)
	upgradeCooling := 0
	for _, u := range upgrades {
		if u.Type == "cooling" {
			if val, ok := catalog.CoolingValues[u.Name]; ok {
				upgradeCooling += val
			}
		}
	}
	gs.CoolingCapacity = baseCooling + tierBonus + upgradeCooling

	totalCompute := hardwareCompute + serviceCompute

	// Overheat penalty: if heat exceeds cooling, throttle by 50%
	heatPenalty := 1.0
	if gs.HeatGenerated > gs.CoolingCapacity {
		heatPenalty = 0.5
	}

	// Event throttle
	eventThrottle := gs.ThrottleMultiplier

	totalMultiplier := gs.ColoMultiplier * gs.IdleMultiplier * heatPenalty * eventThrottle

	// Knowledge points boost: +1% per knowledge point
	knowledgeBoost := 1.0 + float64(gs.KnowledgePoints)/100.0

	gs.ComputeUnits += int64(float64(totalCompute) * seconds * totalMultiplier * knowledgeBoost)
	gs.Reputation += int64(float64(serviceRep) * seconds * heatPenalty * eventThrottle)
	gs.Money += int64(float64(serviceMoney) * seconds * heatPenalty * eventThrottle)

	// Deduct business expenses from money
	var totalExpenses int64
	for _, exp := range expenses {
		totalExpenses += exp.CostPerTick
	}
	if totalExpenses > 0 {
		gs.Money -= int64(float64(totalExpenses) * seconds)
		if gs.Money < 0 {
			gs.Money = 0
		}
	}

	// Customer satisfaction decay: -1 per minute of elapsed time if overheating or throttled
	if len(customers) > 0 && (gs.HeatGenerated > gs.CoolingCapacity || gs.ThrottleMultiplier < 1.0) {
		// Signal to caller that satisfaction changed (handled via returned customers)
		decayPerMin := 1
		decay := int(seconds / 60.0 * float64(decayPerMin))
		if decay > 0 {
			for i := range customers {
				customers[i].Satisfaction -= decay
				if customers[i].Satisfaction < 0 {
					customers[i].Satisfaction = 0
				}
			}
		}
	}

	// Roll for random events
	var triggered []*events.GameEvent
	event := events.RollEvent(gs.Tier, gs.SaasUnlocked, gs.ColoCount, seconds)
	if event != nil {
		if events.IsMitigated(event, upgrades, hardware) {
			mitigated := *event
			mitigated.Effect = &events.EventEffect{}
			mitigated.Description += " (Mitigated!)"
			triggered = append(triggered, &mitigated)
		} else {
			events.ApplyEvent(gs, event)
			// Apply throttle effect from event
			if event.Effect.Throttle > 0 || event.Effect.ThrottleTicks > 0 {
				gs.ThrottleMultiplier = event.Effect.Throttle
				gs.ThrottleTicksRemaining = event.Effect.ThrottleTicks
				if gs.ThrottleMultiplier == 0 && event.Effect.ThrottleTicks > 0 {
					gs.ThrottleMultiplier = 0.01 // near-zero, not fully zero
				}
			}
			triggered = append(triggered, event)
		}
	}

	gs.LastTickAt = now
	return triggered
}

// countShelfSlots returns total shelf capacity and used shelf slots from existing hardware.
// Each shelf provides 4 slots for small (slot-based) items in a rack.
func countShelfSlots(hardware []models.Hardware) (total int, used int) {
	for _, h := range hardware {
		if h.Type == "shelf" {
			total += 4
		}
		// Slot-based items in a rack are on shelves
		if h.RackUnitsUsed == nil && h.SlotsUsed > 0 {
			used += h.SlotsUsed
		}
	}
	return
}

// ProcessAction validates and applies a player action.
func (e *Engine) ProcessAction(gs *models.GameState, actionType string, payload json.RawMessage, hardware []models.Hardware, services []models.Service, upgrades []models.Upgrade, compUpgrades []models.ComponentUpgrade) (*ActionResult, error) {
	switch actionType {
	case "run_job":
		return e.runJob(gs)
	case "buy_hardware":
		return e.buyHardware(gs, payload, hardware)
	case "deploy_service":
		return e.deployService(gs, payload)
	case "sell_hardware":
		return e.sellHardware(gs, payload, hardware)
	case "buy_upgrade":
		return e.buyUpgrade(gs, payload, upgrades)
	case "upgrade_component":
		return e.upgradeComponent(gs, payload, hardware, compUpgrades)
	case "resolve_event":
		return e.resolveEvent(gs)
	case "unlock_saas":
		return e.unlockSaas(gs)
	case "deploy_saas":
		return e.deploySaas(gs, payload)
	case "upgrade_tier":
		return e.upgradeTier(gs)
	case "colo":
		return e.prestige(gs, hardware, services)
	case "build_datacenter":
		return e.buildDatacenter(gs)
	case "upgrade_datacenter":
		return e.upgradeDatacenter(gs)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionType)
	}
}

// ActionResult carries any new or removed DB records that need to be persisted.
type ActionResult struct {
	NewHardware      *models.Hardware
	NewService       *models.Service
	NewUpgrade       *models.Upgrade
	NewCustomer      *models.Customer
	NewExpenses      []models.Expense
	NewColoRack      *models.ColoRack
	ComponentUpgrade *models.ComponentUpgrade
	RemoveHardware   string // hardware ID to delete
	Prestige         bool   // if true, handler should wipe non-persistent data
}

func (e *Engine) runJob(gs *models.GameState) (*ActionResult, error) {
	reward := tierJobReward(gs.Tier)
	knowledgeBoost := 1.0 + float64(gs.KnowledgePoints)/100.0
	gs.ComputeUnits += int64(float64(reward) * gs.ColoMultiplier * knowledgeBoost)
	return &ActionResult{}, nil
}

type buyHardwarePayload struct {
	Name string `json:"name"`
}

func (e *Engine) buyHardware(gs *models.GameState, payload json.RawMessage, hardware []models.Hardware) (*ActionResult, error) {
	var p buyHardwarePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}

	tmpl := catalog.GetHardwareByName(p.Name)
	if tmpl == nil {
		return nil, fmt.Errorf("unknown hardware: %s", p.Name)
	}

	// Check tier requirement
	if catalog.TierToRank(gs.Tier) < catalog.TierToRank(tmpl.MinTier) {
		return nil, fmt.Errorf("tier too low for %s", p.Name)
	}

	// Check cost
	if gs.ComputeUnits < tmpl.Cost {
		return nil, fmt.Errorf("not enough compute units (need %d, have %d)", tmpl.Cost, gs.ComputeUnits)
	}

	// Check power
	if gs.PowerWatts+tmpl.PowerDraw > gs.PowerLimit {
		return nil, fmt.Errorf("not enough power capacity (need %dW, have %dW free)", tmpl.PowerDraw, gs.PowerLimit-gs.PowerWatts)
	}

	// Check slots/rack units
	if isRackTier(gs.Tier) {
		if tmpl.RackUnitsUsed != nil {
			// Rack-mountable item — uses rack units
			if gs.UsedRackUnits == nil || gs.RackUnits == nil {
				return nil, fmt.Errorf("rack not initialized")
			}
			if *gs.UsedRackUnits+*tmpl.RackUnitsUsed > *gs.RackUnits {
				return nil, fmt.Errorf("not enough rack space (need %dU, have %dU free)", *tmpl.RackUnitsUsed, *gs.RackUnits-*gs.UsedRackUnits)
			}
			newUsed := *gs.UsedRackUnits + *tmpl.RackUnitsUsed
			gs.UsedRackUnits = &newUsed
		} else {
			// Slot-based item in rack tier — needs a rack shelf
			shelfTotal, shelfUsed := countShelfSlots(hardware)
			if shelfTotal == 0 {
				return nil, fmt.Errorf("%s requires a Rack Shelf (buy one first)", p.Name)
			}
			if shelfUsed+tmpl.SlotsUsed > shelfTotal {
				return nil, fmt.Errorf("not enough shelf space (%d/%d slots used, need %d)", shelfUsed, shelfTotal, tmpl.SlotsUsed)
			}
			gs.UsedSlots += tmpl.SlotsUsed
		}
	} else {
		if gs.UsedSlots+tmpl.SlotsUsed > gs.HardwareSlots {
			return nil, fmt.Errorf("not enough hardware slots (need %d, have %d free)", tmpl.SlotsUsed, gs.HardwareSlots-gs.UsedSlots)
		}
		gs.UsedSlots += tmpl.SlotsUsed
	}

	// Deduct cost, add power draw and heat
	gs.ComputeUnits -= tmpl.Cost
	gs.PowerWatts += tmpl.PowerDraw
	gs.HeatGenerated += tmpl.PowerDraw // heat roughly tracks power draw

	hw := &models.Hardware{
		GameStateID:   gs.ID,
		Name:          tmpl.Name,
		Type:          tmpl.Type,
		Tier:          gs.Tier,
		SlotsUsed:     tmpl.SlotsUsed,
		RackUnitsUsed: tmpl.RackUnitsUsed,
		PowerDraw:     tmpl.PowerDraw,
		ComputePerTick: tmpl.ComputePerTick,
	}

	return &ActionResult{NewHardware: hw}, nil
}

type sellHardwarePayload struct {
	ID string `json:"id"`
}

func (e *Engine) sellHardware(gs *models.GameState, payload json.RawMessage, hardware []models.Hardware) (*ActionResult, error) {
	var p sellHardwarePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}

	// Find the hardware item
	var found *models.Hardware
	for _, h := range hardware {
		if h.ID == p.ID {
			found = &h
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("hardware not found")
	}

	// Look up original cost from catalog for 60% refund
	tmpl := catalog.GetHardwareByName(found.Name)
	if tmpl != nil {
		gs.ComputeUnits += int64(float64(tmpl.Cost) * 0.6)
	}

	// Free up power and heat
	gs.PowerWatts -= found.PowerDraw
	gs.HeatGenerated -= found.PowerDraw

	// Free up slots/rack units
	if isRackTier(gs.Tier) && found.RackUnitsUsed != nil && gs.UsedRackUnits != nil {
		newUsed := *gs.UsedRackUnits - *found.RackUnitsUsed
		gs.UsedRackUnits = &newUsed
	} else {
		gs.UsedSlots -= found.SlotsUsed
	}

	return &ActionResult{RemoveHardware: found.ID}, nil
}

type deployServicePayload struct {
	Name string `json:"name"`
}

func (e *Engine) deployService(gs *models.GameState, payload json.RawMessage) (*ActionResult, error) {
	var p deployServicePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}

	tmpl := catalog.GetServiceByName(p.Name)
	if tmpl == nil {
		return nil, fmt.Errorf("unknown service: %s", p.Name)
	}

	// Check tier requirement
	if catalog.TierToRank(gs.Tier) < catalog.TierToRank(tmpl.MinTier) {
		return nil, fmt.Errorf("tier too low for %s", p.Name)
	}

	// Check cost
	if gs.ComputeUnits < tmpl.Cost {
		return nil, fmt.Errorf("not enough compute units (need %d, have %d)", tmpl.Cost, gs.ComputeUnits)
	}

	// Check power for service
	if gs.PowerWatts+tmpl.PowerRequired > gs.PowerLimit {
		return nil, fmt.Errorf("not enough power capacity for service")
	}

	// Deduct cost, add power and heat
	gs.ComputeUnits -= tmpl.Cost
	gs.PowerWatts += tmpl.PowerRequired
	gs.HeatGenerated += tmpl.PowerRequired

	svc := &models.Service{
		GameStateID:       gs.ID,
		Name:              tmpl.Name,
		Type:              tmpl.Type,
		Tier:              gs.Tier,
		ComputePerTick:    tmpl.ComputePerTick,
		ReputationPerTick: tmpl.ReputationPerTick,
		MoneyPerTick:      tmpl.MoneyPerTick,
	}

	return &ActionResult{NewService: svc}, nil
}

func (e *Engine) upgradeTier(gs *models.GameState) (*ActionResult, error) {
	next, cost, ok := nextTier(gs.Tier)
	if !ok {
		return nil, fmt.Errorf("already at max tier")
	}

	if gs.ComputeUnits < cost {
		return nil, fmt.Errorf("not enough compute units (need %d, have %d)", cost, gs.ComputeUnits)
	}

	gs.ComputeUnits -= cost
	gs.Tier = next

	// Update capacity based on new tier
	switch next {
	case models.TierClosetFloor:
		gs.HardwareSlots = 5
		gs.PowerLimit = 500
		gs.CoolingCapacity += 100
	case models.TierRack12U:
		gs.HardwareSlots = 0 // slots no longer used
		ru := 12
		usedRU := 0
		gs.RackUnits = &ru
		gs.UsedRackUnits = &usedRU
		gs.PowerLimit = 1500
		gs.CoolingCapacity += 500
	case models.TierRack24U:
		ru := 24
		gs.RackUnits = &ru
		gs.PowerLimit = 3000
		gs.CoolingCapacity += 1000
	case models.TierRack36U:
		ru := 36
		gs.RackUnits = &ru
		gs.PowerLimit = 5000
		gs.CoolingCapacity += 2000
	case models.TierRack48U:
		ru := 48
		gs.RackUnits = &ru
		gs.PowerLimit = 8000
		gs.CoolingCapacity += 3000
	}

	return &ActionResult{}, nil
}

func nextTier(current models.Tier) (models.Tier, int64, bool) {
	switch current {
	case models.TierCoffeeTable:
		return models.TierClosetFloor, 500, true
	case models.TierClosetFloor:
		return models.TierRack12U, 5000, true
	case models.TierRack12U:
		return models.TierRack24U, 25000, true
	case models.TierRack24U:
		return models.TierRack36U, 100000, true
	case models.TierRack36U:
		return models.TierRack48U, 500000, true
	default:
		return "", 0, false
	}
}

type buyUpgradePayload struct {
	Name string `json:"name"`
}

func (e *Engine) buyUpgrade(gs *models.GameState, payload json.RawMessage, owned []models.Upgrade) (*ActionResult, error) {
	var p buyUpgradePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}

	// Check if already owned
	for _, u := range owned {
		if u.Name == p.Name {
			return nil, fmt.Errorf("already owned: %s", p.Name)
		}
	}

	// Find in cooling upgrades
	if val, ok := catalog.CoolingValues[p.Name]; ok {
		tmpl := findUpgrade(p.Name, catalog.CoolingUpgrades)
		if tmpl == nil {
			return nil, fmt.Errorf("unknown upgrade: %s", p.Name)
		}
		if catalog.TierToRank(gs.Tier) < catalog.TierToRank(tmpl.MinTier) {
			return nil, fmt.Errorf("tier too low for %s", p.Name)
		}
		if gs.ComputeUnits < tmpl.Cost {
			return nil, fmt.Errorf("not enough compute units")
		}
		gs.ComputeUnits -= tmpl.Cost
		gs.CoolingCapacity += val
		return &ActionResult{NewUpgrade: &models.Upgrade{GameStateID: gs.ID, Name: p.Name, Type: "cooling", Tier: gs.Tier}}, nil
	}

	// Find in networking upgrades
	if val, ok := catalog.NetworkTierValues[p.Name]; ok {
		tmpl := findUpgrade(p.Name, catalog.NetworkUpgrades)
		if tmpl == nil {
			return nil, fmt.Errorf("unknown upgrade: %s", p.Name)
		}
		if catalog.TierToRank(gs.Tier) < catalog.TierToRank(tmpl.MinTier) {
			return nil, fmt.Errorf("tier too low for %s", p.Name)
		}
		if gs.ComputeUnits < tmpl.Cost {
			return nil, fmt.Errorf("not enough compute units")
		}
		if val <= gs.NetworkTier {
			return nil, fmt.Errorf("already have equal or better networking")
		}
		gs.ComputeUnits -= tmpl.Cost
		gs.NetworkTier = val
		return &ActionResult{NewUpgrade: &models.Upgrade{GameStateID: gs.ID, Name: p.Name, Type: "networking", Tier: gs.Tier}}, nil
	}

	// Find in automation upgrades
	if mult, ok := catalog.AutomationMultipliers[p.Name]; ok {
		tmpl := findUpgrade(p.Name, catalog.AutomationUpgrades)
		if tmpl == nil {
			return nil, fmt.Errorf("unknown upgrade: %s", p.Name)
		}
		if catalog.TierToRank(gs.Tier) < catalog.TierToRank(tmpl.MinTier) {
			return nil, fmt.Errorf("tier too low for %s", p.Name)
		}
		if gs.ComputeUnits < tmpl.Cost {
			return nil, fmt.Errorf("not enough compute units")
		}
		gs.ComputeUnits -= tmpl.Cost
		gs.IdleMultiplier = mult
		gs.AutomationTier++
		return &ActionResult{NewUpgrade: &models.Upgrade{GameStateID: gs.ID, Name: p.Name, Type: "automation", Tier: gs.Tier, Persistent: false}}, nil
	}

	// Find in knowledge upgrades (costs money, not compute)
	if pts, ok := catalog.KnowledgePointValues[p.Name]; ok {
		tmpl := findUpgrade(p.Name, catalog.KnowledgeUpgrades)
		if tmpl == nil {
			return nil, fmt.Errorf("unknown upgrade: %s", p.Name)
		}
		if catalog.TierToRank(gs.Tier) < catalog.TierToRank(tmpl.MinTier) {
			return nil, fmt.Errorf("tier too low for %s", p.Name)
		}
		if gs.Money < tmpl.Cost {
			return nil, fmt.Errorf("not enough money (need $%d)", tmpl.Cost)
		}
		gs.Money -= tmpl.Cost
		gs.KnowledgePoints += pts
		return &ActionResult{NewUpgrade: &models.Upgrade{GameStateID: gs.ID, Name: p.Name, Type: "knowledge", Tier: gs.Tier, Persistent: true}}, nil
	}

	return nil, fmt.Errorf("unknown upgrade: %s", p.Name)
}

func findUpgrade(name string, list []catalog.UpgradeTemplate) *catalog.UpgradeTemplate {
	for _, u := range list {
		if u.Name == name {
			return &u
		}
	}
	return nil
}

type upgradeComponentPayload struct {
	HardwareID string `json:"hardware_id"`
	Component  string `json:"component"`
}

func (e *Engine) upgradeComponent(gs *models.GameState, payload json.RawMessage, hardware []models.Hardware, compUpgrades []models.ComponentUpgrade) (*ActionResult, error) {
	var p upgradeComponentPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}

	// Find the hardware
	var found *models.Hardware
	for _, h := range hardware {
		if h.ID == p.HardwareID {
			found = &h
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("hardware not found")
	}

	// Only servers can be upgraded
	if found.Type != "server" && found.Type != "desktop" && found.Type != "sbc" && found.Type != "mini_pc" && found.Type != "gpu_server" {
		return nil, fmt.Errorf("cannot upgrade components on %s", found.Type)
	}

	info := catalog.GetComponentUpgradeInfo(p.Component)
	if info == nil {
		return nil, fmt.Errorf("unknown component: %s", p.Component)
	}

	// Look up current level from existing component upgrades
	currentLevel := 0
	for _, cu := range compUpgrades {
		if cu.HardwareID == found.ID && cu.Component == p.Component {
			currentLevel = cu.Level
			break
		}
	}
	cost := int64(float64(info.BaseCost) * pow(info.CostScale, currentLevel))

	if gs.ComputeUnits < cost {
		return nil, fmt.Errorf("not enough compute units (need %d)", cost)
	}

	newLevel := currentLevel + 1
	if newLevel > info.MaxLevel {
		return nil, fmt.Errorf("component already at max level")
	}

	gs.ComputeUnits -= cost

	cu := &models.ComponentUpgrade{
		HardwareID:     found.ID,
		Component:      p.Component,
		Level:          newLevel,
		ComputeBonus:   info.ComputeAdd * newLevel,
		PowerReduction: info.PowerReduce * newLevel,
	}

	return &ActionResult{ComponentUpgrade: cu}, nil
}

func (e *Engine) prestige(gs *models.GameState, hardware []models.Hardware, services []models.Service) (*ActionResult, error) {
	// Must be at 48U rack with SaaS unlocked
	if gs.Tier != models.TierRack48U {
		return nil, fmt.Errorf("must be at 48U rack tier to colo")
	}
	if !gs.SaasUnlocked {
		return nil, fmt.Errorf("must have SaaS unlocked to colo")
	}

	// Snapshot current rack's total income for the colo rack
	var totalCompute, totalRep, totalMoney int64
	for _, h := range hardware {
		totalCompute += h.ComputePerTick
	}
	for _, s := range services {
		totalCompute += s.ComputePerTick
		totalRep += s.ReputationPerTick
		totalMoney += s.MoneyPerTick
	}

	// Determine datacenter tier based on colo count
	dcTier := 1
	if gs.ColoCount >= 3 {
		dcTier = 2
	}
	if gs.ColoCount >= 6 {
		dcTier = 3
	}
	if gs.ColoCount >= 10 {
		dcTier = 4
	}

	coloRack := &models.ColoRack{
		UserID:            gs.UserID,
		DatacenterTier:    dcTier,
		RackSize:          48,
		ComputePerTick:    totalCompute,
		ReputationPerTick: totalRep,
		MoneyPerTick:      totalMoney,
	}

	// Calculate new colo multiplier with diminishing returns
	// Formula: 1 + sum of (0.5 / (1 + i * 0.1)) for each colo
	newColoCount := gs.ColoCount + 1
	mult := 1.0
	for i := 0; i < newColoCount; i++ {
		mult += 0.5 / (1.0 + float64(i)*0.1)
	}

	// Reset game state — keep persistent fields
	gs.Tier = models.TierCoffeeTable
	gs.ComputeUnits = 0
	gs.Reputation = 0
	gs.PowerWatts = 0
	gs.PowerLimit = 200
	gs.Money = 0
	gs.HardwareSlots = 2
	gs.UsedSlots = 0
	gs.RackUnits = nil
	gs.UsedRackUnits = nil
	gs.ColoCount = newColoCount
	gs.ColoMultiplier = mult
	gs.HeatGenerated = 0
	gs.CoolingCapacity = 50
	gs.NetworkTier = 0
	gs.AutomationTier = 0
	gs.IdleMultiplier = 1.0
	// Keep: KnowledgePoints (persistent)
	gs.SaasUnlocked = false
	gs.TotalCustomers = 0
	gs.ThrottleMultiplier = 1.0
	gs.ThrottleTicksRemaining = 0
	gs.DatacenterTier = dcTier

	return &ActionResult{NewColoRack: coloRack, Prestige: true}, nil
}

func pow(base float64, exp int) float64 {
	result := 1.0
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

func (e *Engine) resolveEvent(gs *models.GameState) (*ActionResult, error) {
	if gs.ThrottleTicksRemaining <= 0 {
		return nil, fmt.Errorf("no active event to resolve")
	}

	// Cost to resolve: 100 compute per remaining tick
	cost := int64(gs.ThrottleTicksRemaining) * 100
	if gs.ComputeUnits < cost {
		return nil, fmt.Errorf("not enough compute to resolve (need %d)", cost)
	}

	gs.ComputeUnits -= cost
	gs.ThrottleMultiplier = 1.0
	gs.ThrottleTicksRemaining = 0

	return &ActionResult{}, nil
}

func (e *Engine) unlockSaas(gs *models.GameState) (*ActionResult, error) {
	if gs.SaasUnlocked {
		return nil, fmt.Errorf("SaaS already unlocked")
	}
	if !isRackTier(gs.Tier) {
		return nil, fmt.Errorf("must be at a rack tier to unlock SaaS")
	}
	if gs.Reputation < 100 {
		return nil, fmt.Errorf("need at least 100 reputation to unlock SaaS (have %d)", gs.Reputation)
	}
	cost := int64(10000)
	if gs.ComputeUnits < cost {
		return nil, fmt.Errorf("not enough compute units (need %d)", cost)
	}
	gs.ComputeUnits -= cost
	gs.SaasUnlocked = true

	// Create initial business expenses
	var expenses []models.Expense
	for _, tmpl := range catalog.BusinessExpenses {
		expenses = append(expenses, models.Expense{
			GameStateID: gs.ID,
			Name:        tmpl.Name,
			Type:        tmpl.Type,
			CostPerTick: tmpl.CostPerTick,
		})
	}

	return &ActionResult{NewExpenses: expenses}, nil
}

type deploySaasPayload struct {
	Name string `json:"name"`
}

func (e *Engine) deploySaas(gs *models.GameState, payload json.RawMessage) (*ActionResult, error) {
	if !gs.SaasUnlocked {
		return nil, fmt.Errorf("SaaS not unlocked")
	}

	var p deploySaasPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}

	tmpl := catalog.GetSaasServiceByName(p.Name)
	if tmpl == nil {
		return nil, fmt.Errorf("unknown SaaS service: %s", p.Name)
	}

	if gs.Reputation < tmpl.ReputationRequired {
		return nil, fmt.Errorf("need %d reputation (have %d)", tmpl.ReputationRequired, gs.Reputation)
	}
	if gs.ComputeUnits < tmpl.DeployCost {
		return nil, fmt.Errorf("not enough compute units (need %d)", tmpl.DeployCost)
	}
	if gs.PowerWatts+tmpl.PowerRequired > gs.PowerLimit {
		return nil, fmt.Errorf("not enough power capacity")
	}

	gs.ComputeUnits -= tmpl.DeployCost
	gs.PowerWatts += tmpl.PowerRequired
	gs.HeatGenerated += tmpl.PowerRequired

	// Deploy creates the service and attracts an initial customer
	svc := &models.Service{
		GameStateID:       gs.ID,
		Name:              tmpl.Name,
		Type:              tmpl.Type,
		Tier:              gs.Tier,
		ComputePerTick:    0,
		ReputationPerTick: 5,
		MoneyPerTick:      tmpl.RevenuePerCustomer,
	}

	// Generate first customer
	firstName := catalog.CustomerFirstNames[gs.TotalCustomers%len(catalog.CustomerFirstNames)]
	lastName := catalog.CustomerLastNames[gs.TotalCustomers%len(catalog.CustomerLastNames)]
	customer := &models.Customer{
		GameStateID:    gs.ID,
		Name:           firstName + " " + lastName,
		ServiceType:    tmpl.Type,
		MonthlyRevenue: tmpl.RevenuePerCustomer,
		Satisfaction:   100,
	}

	gs.TotalCustomers++

	return &ActionResult{NewService: svc, NewCustomer: customer}, nil
}

func (e *Engine) buildDatacenter(gs *models.GameState) (*ActionResult, error) {
	if gs.OwnsDatacenter {
		return nil, fmt.Errorf("you already own a datacenter")
	}
	if gs.ColoCount < 5 {
		return nil, fmt.Errorf("need at least 5 colocations to build a datacenter (have %d)", gs.ColoCount)
	}

	moneyCost := int64(1000000)
	computeCost := int64(5000000)

	if gs.Money < moneyCost {
		return nil, fmt.Errorf("not enough money (need $%d)", moneyCost)
	}
	if gs.ComputeUnits < computeCost {
		return nil, fmt.Errorf("not enough compute (need %d)", computeCost)
	}

	gs.Money -= moneyCost
	gs.ComputeUnits -= computeCost
	gs.OwnsDatacenter = true
	gs.DatacenterLevel = 1
	gs.DatacenterIncomeMultiplier = 1.5 // 50% bonus on all colo rack income

	return &ActionResult{}, nil
}

func (e *Engine) upgradeDatacenter(gs *models.GameState) (*ActionResult, error) {
	if !gs.OwnsDatacenter {
		return nil, fmt.Errorf("you don't own a datacenter")
	}
	if gs.DatacenterLevel >= 5 {
		return nil, fmt.Errorf("datacenter already at max level")
	}

	// Cost scales with level
	level := gs.DatacenterLevel
	moneyCost := int64(500000) * int64(level+1)
	computeCost := int64(2000000) * int64(level+1)

	if gs.Money < moneyCost {
		return nil, fmt.Errorf("not enough money (need $%d)", moneyCost)
	}
	if gs.ComputeUnits < computeCost {
		return nil, fmt.Errorf("not enough compute (need %d)", computeCost)
	}

	gs.Money -= moneyCost
	gs.ComputeUnits -= computeCost
	gs.DatacenterLevel++
	// Each level adds 0.25x to the income multiplier
	gs.DatacenterIncomeMultiplier = 1.0 + float64(gs.DatacenterLevel)*0.25

	return &ActionResult{}, nil
}

func tierCoolingBonus(tier models.Tier) int {
	switch tier {
	case models.TierClosetFloor:
		return 100
	case models.TierRack12U:
		return 600
	case models.TierRack24U:
		return 1600
	case models.TierRack36U:
		return 3600
	case models.TierRack48U:
		return 6600
	default:
		return 0
	}
}

func isRackTier(tier models.Tier) bool {
	return catalog.TierToRank(tier) >= 2
}

func tierJobReward(tier models.Tier) int64 {
	switch tier {
	case models.TierCoffeeTable:
		return 10
	case models.TierClosetFloor:
		return 50
	case models.TierRack12U:
		return 200
	case models.TierRack24U:
		return 800
	case models.TierRack36U:
		return 3000
	case models.TierRack48U:
		return 10000
	default:
		return 10
	}
}
