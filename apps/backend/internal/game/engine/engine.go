package engine

import (
	"encoding/json"
	"fmt"
	"math"
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
func (e *Engine) ProcessIdleProgress(gs *models.GameState, hardware []models.Hardware, services []models.Service, upgrades []models.Upgrade, expenses []models.Expense, customers []models.Customer, compUpgrades []models.ComponentUpgrade, researchLevels []models.ResearchLevel, now time.Time) []*events.GameEvent {
	elapsed := now.Sub(gs.LastTickAt)
	seconds := elapsed.Seconds()

	// === RECALCULATIONS (always run, regardless of elapsed time) ===

	// Compute from hardware (including component upgrade bonuses) and recalculate heat
	// Also calculate bonuses from network, storage, power, and patch panel hardware
	var hardwareCompute int64
	var totalHeat int
	var networkBonus float64  // idle income multiplier from switches
	var storageBonus float64  // reputation multiplier from NAS
	var upsCompute int64      // flat compute bonus from UPS
	var patchPanelBonus float64 // reputation multiplier from patch panels

	for _, h := range hardware {
		compute := h.ComputePerTick
		powerDraw := h.PowerDraw
		// Apply component upgrade bonuses for this hardware (ComputeBonus is a percentage of base)
		for _, cu := range compUpgrades {
			if cu.HardwareID == h.ID {
				compute += h.ComputePerTick * int64(cu.ComputeBonus) / 100
				powerDraw -= cu.PowerReduction
			}
		}
		if powerDraw < 0 {
			powerDraw = 0
		}
		hardwareCompute += compute
		totalHeat += powerDraw

		// Network hardware: idle income bonus
		if bonus, ok := NetworkIncomeBonus[h.Name]; ok {
			networkBonus += bonus
		}

		// Storage hardware: reputation bonus
		if bonus, ok := StorageRepBonus[h.Name]; ok {
			storageBonus += bonus
		}

		// UPS hardware: flat compute bonus
		if bonus, ok := UpsComputeBonus[h.Name]; ok {
			upsCompute += bonus
		}

		// Patch panel: reputation bonus
		if h.Type == "patch_panel" {
			patchPanelBonus += PatchPanelBonusValue
		}
	}

	// Network and storage bonuses stack additively with no cap

	hardwareCompute += upsCompute

	// Compute and reputation from services
	var serviceCompute, serviceRep, serviceMoney int64
	for _, s := range services {
		serviceCompute += s.ComputePerTick
		serviceRep += s.ReputationPerTick
		serviceMoney += s.MoneyPerTick
	}

	// Recalculate actual power draw from hardware (with component reductions) + services
	// Services don't have component upgrades so we add their power from gs.PowerWatts - totalHardwareBasePower + totalHeat
	var hardwareBasePower int
	for _, h := range hardware {
		hardwareBasePower += h.PowerDraw
	}
	servicePower := gs.PowerWatts - hardwareBasePower
	if servicePower < 0 {
		servicePower = 0
	}
	gs.PowerWatts = totalHeat + servicePower
	gs.HeatGenerated = gs.PowerWatts

	// Overclock extra heat: (multiplier - 1) * heat
	if gs.OverclockTicksRemaining > 0 && gs.OverclockMultiplier > 1.0 {
		overclockHeat := int(float64(gs.HeatGenerated) * (gs.OverclockMultiplier - 1.0))
		gs.HeatGenerated += overclockHeat
	}

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

	// Recalculate power limit from tier (ensures retroactive changes apply)
	gs.PowerLimit = tierPowerLimit(gs.Tier)

	// Recalculate used slots from actual hardware (keeps slot count accurate)
	if isRackTier(gs.Tier) {
		usedSlots := 0
		for _, h := range hardware {
			if h.RackUnitsUsed == nil && h.SlotsUsed > 0 {
				usedSlots += h.SlotsUsed
			}
		}
		gs.UsedSlots = usedSlots
	}

	totalCompute := hardwareCompute + serviceCompute

	// === INCOME & EVENTS (only run when time has elapsed) ===
	if elapsed <= 0 {
		return nil
	}

	// Decay throttle over time
	if gs.ThrottleTicksRemaining > 0 {
		gs.ThrottleTicksRemaining--
		if gs.ThrottleTicksRemaining <= 0 {
			gs.ThrottleMultiplier = 1.0
			gs.ThrottleTicksRemaining = 0
		}
	}

	// Calculate overclock weighted-average multiplier for this period (before decaying ticks)
	overclockMult := 1.0
	if gs.OverclockTicksRemaining > 0 {
		overclockDurationSec := float64(gs.OverclockTicksRemaining) * 5.0
		if seconds <= overclockDurationSec {
			// Entire period was overclocked
			overclockMult = gs.OverclockMultiplier
		} else {
			// Partial: weighted average
			overclockFraction := overclockDurationSec / seconds
			overclockMult = gs.OverclockMultiplier*overclockFraction + 1.0*(1.0-overclockFraction)
		}
	}
	// Defensive guard: overclock should never reduce income
	if overclockMult < 1.0 {
		overclockMult = 1.0
	}

	// Decay overclock over time (time-based, not per-call)
	if gs.OverclockTicksRemaining > 0 {
		elapsedTicks := int(seconds / 5.0)
		if elapsedTicks < 1 {
			elapsedTicks = 1
		}
		gs.OverclockTicksRemaining -= elapsedTicks
		if gs.OverclockTicksRemaining <= 0 {
			gs.OverclockMultiplier = 1.0
			gs.OverclockTicksRemaining = 0
		}
	}

	// Overheat penalty: if heat exceeds cooling, throttle by 50%
	heatPenalty := 1.0
	if gs.HeatGenerated > gs.CoolingCapacity {
		heatPenalty = 0.5
	}

	// Event throttle
	eventThrottle := gs.ThrottleMultiplier

	totalMultiplier := gs.ColoMultiplier * gs.IdleMultiplier * heatPenalty * eventThrottle * overclockMult

	// Knowledge points boost: +1% per knowledge point
	knowledgeBoost := 1.0 + float64(gs.KnowledgePoints)/100.0

	// Network bonus: switches boost idle compute income
	netMult := 1.0 + networkBonus

	// Storage + patch panel bonus: boost reputation income
	repMult := 1.0 + storageBonus + patchPanelBonus

	// Research bonuses: aggregate by effect type and apply as multipliers
	researchBonuses := aggregateResearchBonuses(researchLevels)
	researchIdleMult := 1.0 + researchBonuses["idle_income"]
	researchRepMult := 1.0 + researchBonuses["reputation_gain"]
	researchMoneyMult := 1.0 + researchBonuses["money_income"]

	gs.ComputeUnits += int64(float64(totalCompute) * seconds * totalMultiplier * knowledgeBoost * netMult * researchIdleMult)
	gs.Reputation += int64(float64(serviceRep) * seconds * heatPenalty * eventThrottle * repMult * researchRepMult)
	gs.Money += int64(float64(serviceMoney) * seconds * heatPenalty * eventThrottle * researchMoneyMult)

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

// countShelfCapacity returns total shelf slot capacity from owned shelves.
// Each shelf provides 8 slots for small (slot-based) items in a rack.
func countShelfCapacity(hardware []models.Hardware) int {
	total := 0
	for _, h := range hardware {
		if h.Type == "shelf" {
			total += 8
		}
	}
	return total
}

// ProcessAction validates and applies a player action.
// currentBitcoinPrice is the server-resolved Bitcoin price; only used by buy_bitcoin/sell_bitcoin actions (0 for all others).
func (e *Engine) ProcessAction(gs *models.GameState, actionType string, payload json.RawMessage, hardware []models.Hardware, services []models.Service, upgrades []models.Upgrade, compUpgrades []models.ComponentUpgrade, researchLevels []models.ResearchLevel, currentBitcoinPrice int64) (*ActionResult, error) {
	switch actionType {
	case "run_job":
		return e.runJob(gs, researchLevels)
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
		return e.prestige(gs, hardware, services, compUpgrades)
	case "bulk_upgrade_components":
		return e.bulkUpgradeComponents(gs, hardware, compUpgrades)
	case "bulk_deploy_services":
		return e.bulkDeployServices(gs, services)
	case "bulk_buy_upgrades":
		return e.bulkBuyUpgrades(gs, payload, upgrades)
	case "bulk_deploy_saas":
		return e.bulkDeploySaas(gs, services)
	case "donate_cu":
		return e.donateCU(gs, payload)
	case "build_datacenter":
		return e.buildDatacenter(gs)
	case "upgrade_datacenter":
		return e.upgradeDatacenter(gs)
	case "buy_bitcoin":
		return e.buyBitcoin(gs, payload, currentBitcoinPrice)
	case "buy_max_bitcoin":
		return e.buyMaxBitcoin(gs, currentBitcoinPrice)
	case "sell_bitcoin":
		return e.sellBitcoin(gs, payload, currentBitcoinPrice)
	case "sell_all_bitcoin":
		return e.sellAllBitcoin(gs, currentBitcoinPrice)
	case "activate_overclock":
		return e.activateOverclock(gs, payload)
	case "buy_research":
		return e.buyResearch(gs, payload, researchLevels)
	case "bulk_buy_research":
		return e.bulkBuyResearch(gs, payload, researchLevels)
	case "optimize_rack":
		return e.optimizeRack(gs)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionType)
	}
}

// ActionResult carries any new or removed DB records that need to be persisted.
type ActionResult struct {
	NewHardware       *models.Hardware
	NewService        *models.Service
	NewServices       []models.Service
	NewUpgrade        *models.Upgrade
	NewUpgrades       []models.Upgrade
	NewCustomer       *models.Customer
	NewCustomers      []models.Customer
	NewExpenses       []models.Expense
	NewColoRack       *models.ColoRack
	ComponentUpgrade  *models.ComponentUpgrade
	ComponentUpgrades []models.ComponentUpgrade
	ResearchLevel     *models.ResearchLevel
	RemoveHardware    string // hardware ID to delete
	Prestige          bool   // if true, handler should wipe non-persistent data
}

func (e *Engine) runJob(gs *models.GameState, researchLevels []models.ResearchLevel) (*ActionResult, error) {
	reward := tierJobReward(gs.Tier)
	// Clicks only get knowledge boost — colo multiplier applies to idle income only
	knowledgeBoost := 1.0 + float64(gs.KnowledgePoints)/100.0
	researchBonuses := aggregateResearchBonuses(researchLevels)
	researchJobMult := 1.0 + researchBonuses["job_reward"]
	gs.ComputeUnits += int64(float64(reward) * knowledgeBoost * researchJobMult)
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
			shelfCapacity := countShelfCapacity(hardware)
			if shelfCapacity == 0 {
				return nil, fmt.Errorf("%s requires a Rack Shelf (buy one first)", p.Name)
			}
			if gs.UsedSlots+tmpl.SlotsUsed > shelfCapacity {
				return nil, fmt.Errorf("not enough shelf space (%d/%d slots used, need %d)", gs.UsedSlots, shelfCapacity, tmpl.SlotsUsed)
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

	// Can't sell a shelf if items are on it
	if found.Type == "shelf" {
		shelfCapacity := countShelfCapacity(hardware)
		// After removing this shelf, would remaining capacity cover used slots?
		remainingCapacity := shelfCapacity - 8
		if gs.UsedSlots > remainingCapacity {
			return nil, fmt.Errorf("cannot sell shelf — %d items still on shelves (remove them first)", gs.UsedSlots)
		}
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
	next, baseCost, ok := nextTier(gs.Tier)
	if !ok {
		return nil, fmt.Errorf("already at max tier")
	}

	// Scale tier costs with prestige count
	prestigeScale := prestigeCostScale(gs.ColoCount)
	cost := int64(float64(baseCost) * prestigeScale)

	if gs.ComputeUnits < cost {
		return nil, fmt.Errorf("not enough compute units (need %d, have %d)", cost, gs.ComputeUnits)
	}

	gs.ComputeUnits -= cost
	gs.Tier = next

	// Update capacity based on new tier
	switch next {
	case models.TierClosetFloor:
		gs.HardwareSlots = 5
		gs.PowerLimit = 1250
		gs.CoolingCapacity += 100
	case models.TierRack12U:
		gs.HardwareSlots = 0 // slots no longer used
		ru := 12
		usedRU := 0
		gs.RackUnits = &ru
		gs.UsedRackUnits = &usedRU
		gs.PowerLimit = 3750
		gs.CoolingCapacity += 500
	case models.TierRack24U:
		ru := 24
		gs.RackUnits = &ru
		gs.PowerLimit = 7500
		gs.CoolingCapacity += 1000
	case models.TierRack36U:
		ru := 36
		gs.RackUnits = &ru
		gs.PowerLimit = 12500
		gs.CoolingCapacity += 2000
	case models.TierRack48U:
		ru := 48
		gs.RackUnits = &ru
		gs.PowerLimit = 20000
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
		cost := int64(float64(tmpl.Cost) * prestigeCostScale(gs.ColoCount))
		if gs.ComputeUnits < cost {
			return nil, fmt.Errorf("not enough compute units")
		}
		gs.ComputeUnits -= cost
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
		ComputeBonus:   info.ComputePercent * newLevel, // percentage of base compute (e.g. 15 = +15%)
		PowerReduction: info.PowerReduce * newLevel,
	}

	return &ActionResult{ComponentUpgrade: cu}, nil
}

func (e *Engine) optimizeRack(gs *models.GameState) (*ActionResult, error) {
	if gs.Tier != models.TierRack48U {
		return nil, fmt.Errorf("must be at 48U rack tier to optimize")
	}
	if !gs.SaasUnlocked {
		return nil, fmt.Errorf("must have SaaS unlocked to optimize")
	}
	if gs.RackOptimization >= 46 {
		return nil, fmt.Errorf("optimization level at maximum")
	}
	cost := int64(100000) << uint(gs.RackOptimization)
	if gs.ComputeUnits < cost {
		return nil, fmt.Errorf("not enough compute units (need %d, have %d)", cost, gs.ComputeUnits)
	}
	gs.ComputeUnits -= cost
	gs.RackOptimization++
	return &ActionResult{}, nil
}

func (e *Engine) prestige(gs *models.GameState, hardware []models.Hardware, services []models.Service, compUpgrades []models.ComponentUpgrade) (*ActionResult, error) {
	// Must be at 48U rack with SaaS unlocked
	if gs.Tier != models.TierRack48U {
		return nil, fmt.Errorf("must be at 48U rack tier to colo")
	}
	if !gs.SaasUnlocked {
		return nil, fmt.Errorf("must have SaaS unlocked to colo")
	}
	if gs.ColoCount >= 100 {
		return nil, fmt.Errorf("maximum colocations reached")
	}

	// Snapshot current rack's total income for the colo rack (including component upgrades)
	var totalCompute, totalRep, totalMoney int64
	for _, h := range hardware {
		compute := h.ComputePerTick
		for _, cu := range compUpgrades {
			if cu.HardwareID == h.ID {
				compute += h.ComputePerTick * int64(cu.ComputeBonus) / 100
			}
		}
		totalCompute += compute
	}
	for _, s := range services {
		totalCompute += s.ComputePerTick
		totalRep += s.ReputationPerTick
		totalMoney += s.MoneyPerTick
	}

	// Apply rack optimization bonus to snapshot
	if gs.RackOptimization > 0 {
		cfg := GetConfig()
		bonus := 1.0 + float64(gs.RackOptimization)*cfg.RackOptimization.BonusPerLevel
		totalCompute = int64(float64(totalCompute) * bonus)
		totalRep = int64(float64(totalRep) * bonus)
		totalMoney = int64(float64(totalMoney) * bonus)
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
	gs.PowerLimit = 500
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
	// Keep: BitcoinBalance (persistent cross-prestige asset)
	gs.SaasUnlocked = false
	gs.TotalCustomers = 0
	gs.ThrottleMultiplier = 1.0
	gs.ThrottleTicksRemaining = 0
	gs.OverclockMultiplier = 1.0
	gs.OverclockTicksRemaining = 0
	gs.RackOptimization = 0
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

// aggregateResearchBonuses sums research bonuses by effect type from owned research levels.
// Bonuses within the same effect type stack additively: sum(level * effectValue).
func aggregateResearchBonuses(levels []models.ResearchLevel) map[string]float64 {
	bonuses := make(map[string]float64)
	for _, rl := range levels {
		node := catalog.GetResearchNode(rl.ResearchNode)
		if node != nil {
			bonuses[node.EffectType] += float64(rl.Level) * node.EffectValue
		}
	}
	return bonuses
}

// researchCost calculates the CU cost for a research node at the given level.
// Returns the cost and true if valid, or 0 and false if the cost overflows int64.
func researchCost(baseCost int64, costScale float64, level int) (int64, bool) {
	costF := float64(baseCost) * math.Pow(costScale, float64(level))
	if costF > float64(math.MaxInt64) || math.IsInf(costF, 0) || math.IsNaN(costF) {
		return 0, false // overflow
	}
	return int64(costF), true
}

type buyResearchPayload struct {
	Node string `json:"node"`
}

func (e *Engine) buyResearch(gs *models.GameState, payload json.RawMessage, researchLevels []models.ResearchLevel) (*ActionResult, error) {
	var p buyResearchPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}

	node := catalog.GetResearchNode(p.Node)
	if node == nil {
		return nil, fmt.Errorf("unknown research node: %s", p.Node)
	}

	// Check tier requirement
	if catalog.TierToRank(gs.Tier) < catalog.TierToRank(node.MinTier) {
		return nil, fmt.Errorf("tier too low for %s (need %s)", node.Name, node.MinTier)
	}

	// Look up current level
	currentLevel := 0
	for _, rl := range researchLevels {
		if rl.ResearchNode == p.Node {
			currentLevel = rl.Level
			break
		}
	}

	// Compute cost with overflow check
	cost, ok := researchCost(node.BaseCost, node.CostScale, currentLevel)
	if !ok {
		return nil, fmt.Errorf("research level too high (cost overflow)")
	}

	if gs.ComputeUnits < cost {
		return nil, fmt.Errorf("not enough compute units (need %d, have %d)", cost, gs.ComputeUnits)
	}

	gs.ComputeUnits -= cost
	newLevel := currentLevel + 1

	rl := &models.ResearchLevel{
		GameStateID:  gs.ID,
		ResearchNode: p.Node,
		Level:        newLevel,
	}

	return &ActionResult{ResearchLevel: rl}, nil
}

func (e *Engine) bulkBuyResearch(gs *models.GameState, payload json.RawMessage, researchLevels []models.ResearchLevel) (*ActionResult, error) {
	var p buyResearchPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}

	node := catalog.GetResearchNode(p.Node)
	if node == nil {
		return nil, fmt.Errorf("unknown research node: %s", p.Node)
	}

	// Check tier requirement
	if catalog.TierToRank(gs.Tier) < catalog.TierToRank(node.MinTier) {
		return nil, fmt.Errorf("tier too low for %s (need %s)", node.Name, node.MinTier)
	}

	// Look up current level
	currentLevel := 0
	for _, rl := range researchLevels {
		if rl.ResearchNode == p.Node {
			currentLevel = rl.Level
			break
		}
	}

	// Buy as many levels as affordable
	purchased := 0
	for {
		cost, ok := researchCost(node.BaseCost, node.CostScale, currentLevel+purchased)
		if !ok {
			break // overflow threshold reached
		}
		if gs.ComputeUnits < cost {
			break // can't afford next level
		}
		gs.ComputeUnits -= cost
		purchased++
	}

	if purchased == 0 {
		return nil, fmt.Errorf("not enough compute units")
	}

	newLevel := currentLevel + purchased
	rl := &models.ResearchLevel{
		GameStateID:  gs.ID,
		ResearchNode: p.Node,
		Level:        newLevel,
	}

	return &ActionResult{ResearchLevel: rl}, nil
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
	cost := int64(float64(10000) * prestigeCostScale(gs.ColoCount))
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

	cost := int64(float64(tmpl.DeployCost) * prestigeCostScale(gs.ColoCount))
	if gs.Reputation < tmpl.ReputationRequired {
		return nil, fmt.Errorf("need %d reputation (have %d)", tmpl.ReputationRequired, gs.Reputation)
	}
	if gs.ComputeUnits < cost {
		return nil, fmt.Errorf("not enough compute units (need %d)", cost)
	}
	if gs.PowerWatts+tmpl.PowerRequired > gs.PowerLimit {
		return nil, fmt.Errorf("not enough power capacity")
	}

	gs.ComputeUnits -= cost
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

type donateCUPayload struct {
	Amount int64 `json:"amount"`
}

func (e *Engine) donateCU(gs *models.GameState, payload json.RawMessage) (*ActionResult, error) {
	var p donateCUPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}
	if p.Amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	if gs.ComputeUnits < p.Amount {
		return nil, fmt.Errorf("not enough compute units (need %d, have %d)", p.Amount, gs.ComputeUnits)
	}

	gs.ComputeUnits -= p.Amount
	gs.TotalDonatedCU += p.Amount

	return &ActionResult{}, nil
}

func (e *Engine) buildDatacenter(gs *models.GameState) (*ActionResult, error) {
	if gs.OwnsDatacenter {
		return nil, fmt.Errorf("you already own a datacenter")
	}
	if gs.ColoCount < 5 {
		return nil, fmt.Errorf("need at least 5 colocations to build a datacenter (have %d)", gs.ColoCount)
	}

	moneyCost := int64(500000)
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
	moneyCost := int64(250000) * int64(level+1)
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
	// Each level adds 0.25x to the income multiplier (base 1.25 so level 1 = 1.5x matches build)
	gs.DatacenterIncomeMultiplier = 1.25 + float64(gs.DatacenterLevel)*0.25

	return &ActionResult{}, nil
}

type bitcoinPayload struct {
	Amount int64 `json:"amount"`
}

func (e *Engine) buyBitcoin(gs *models.GameState, payload json.RawMessage, currentBitcoinPrice int64) (*ActionResult, error) {
	if currentBitcoinPrice <= 0 {
		return nil, fmt.Errorf("bitcoin market unavailable")
	}

	var p bitcoinPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}
	if p.Amount < 1 {
		return nil, fmt.Errorf("amount must be positive")
	}

	// Overflow guard: reject if multiplication would overflow int64
	if p.Amount > math.MaxInt64/currentBitcoinPrice {
		return nil, fmt.Errorf("amount too large")
	}

	totalCost := p.Amount * currentBitcoinPrice
	if gs.Money < totalCost {
		return nil, fmt.Errorf("not enough money (need $%d, have $%d)", totalCost, gs.Money)
	}

	// CU cost: 100000 CU per BTC purchased
	var cuCostPerBTC int64 = 100000
	if p.Amount > math.MaxInt64/cuCostPerBTC {
		return nil, fmt.Errorf("amount too large")
	}
	cuCost := p.Amount * cuCostPerBTC
	if gs.ComputeUnits < cuCost {
		return nil, fmt.Errorf("not enough compute units (need %d, have %d)", cuCost, gs.ComputeUnits)
	}

	gs.Money -= totalCost
	gs.ComputeUnits -= cuCost
	gs.BitcoinBalance += p.Amount

	return &ActionResult{}, nil
}

func (e *Engine) buyMaxBitcoin(gs *models.GameState, currentBitcoinPrice int64) (*ActionResult, error) {
	if currentBitcoinPrice <= 0 {
		return nil, fmt.Errorf("bitcoin market unavailable")
	}

	var cuCostPerBTC int64 = 100000
	maxByMoney := gs.Money / currentBitcoinPrice
	maxByCU := gs.ComputeUnits / cuCostPerBTC
	amount := maxByMoney
	if maxByCU < amount {
		amount = maxByCU
	}
	if amount < 1 {
		return nil, fmt.Errorf("cannot afford any bitcoin at current price ($%d)", currentBitcoinPrice)
	}

	gs.Money -= amount * currentBitcoinPrice
	gs.ComputeUnits -= amount * cuCostPerBTC
	gs.BitcoinBalance += amount

	return &ActionResult{}, nil
}

func (e *Engine) sellAllBitcoin(gs *models.GameState, currentBitcoinPrice int64) (*ActionResult, error) {
	if currentBitcoinPrice <= 0 {
		return nil, fmt.Errorf("bitcoin market unavailable")
	}

	var cuCostPerBTC int64 = 100000
	maxByCU := gs.ComputeUnits / cuCostPerBTC
	amount := gs.BitcoinBalance
	if maxByCU < amount {
		amount = maxByCU
	}
	if amount < 1 {
		return nil, fmt.Errorf("cannot sell any bitcoin (need %d CU per BTC)", cuCostPerBTC)
	}

	gs.BitcoinBalance -= amount
	gs.ComputeUnits -= amount * cuCostPerBTC
	gs.Money += amount * currentBitcoinPrice

	return &ActionResult{}, nil
}

func (e *Engine) sellBitcoin(gs *models.GameState, payload json.RawMessage, currentBitcoinPrice int64) (*ActionResult, error) {
	if currentBitcoinPrice <= 0 {
		return nil, fmt.Errorf("bitcoin market unavailable")
	}

	var p bitcoinPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}
	if p.Amount < 1 {
		return nil, fmt.Errorf("amount must be positive")
	}

	if gs.BitcoinBalance < p.Amount {
		return nil, fmt.Errorf("not enough bitcoin (need %d, have %d)", p.Amount, gs.BitcoinBalance)
	}

	// CU cost: 100000 CU per BTC sold
	var cuCostPerBTC int64 = 100000
	if p.Amount > math.MaxInt64/cuCostPerBTC {
		return nil, fmt.Errorf("amount too large")
	}
	cuCost := p.Amount * cuCostPerBTC
	if gs.ComputeUnits < cuCost {
		return nil, fmt.Errorf("not enough compute units (need %d, have %d)", cuCost, gs.ComputeUnits)
	}

	// Overflow guard: reject if multiplication would overflow int64
	if p.Amount > math.MaxInt64/currentBitcoinPrice {
		return nil, fmt.Errorf("amount too large")
	}

	gs.BitcoinBalance -= p.Amount
	gs.ComputeUnits -= cuCost
	gs.Money += p.Amount * currentBitcoinPrice

	return &ActionResult{}, nil
}

type overclockPayload struct {
	Tier int `json:"tier"`
}

func (e *Engine) activateOverclock(gs *models.GameState, payload json.RawMessage) (*ActionResult, error) {
	var p overclockPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}

	tier := getOverclockTier(p.Tier)
	if tier == nil {
		return nil, fmt.Errorf("invalid overclock tier: %d", p.Tier)
	}

	if gs.ComputeUnits < tier.Cost {
		return nil, fmt.Errorf("not enough compute units (need %d, have %d)", tier.Cost, gs.ComputeUnits)
	}

	gs.ComputeUnits -= tier.Cost
	gs.OverclockMultiplier = tier.Multiplier
	gs.OverclockTicksRemaining = tier.Duration

	return &ActionResult{}, nil
}

func prestigeCostScale(coloCount int) float64 {
	if coloCount <= 5 {
		return 1.0 + float64(coloCount)*0.5
	}
	return 3.5 * math.Pow(1.5, float64(coloCount-5))
}

func tierPowerLimit(tier models.Tier) int {
	switch tier {
	case models.TierCoffeeTable:
		return 500
	case models.TierClosetFloor:
		return 1250
	case models.TierRack12U:
		return 3750
	case models.TierRack24U:
		return 7500
	case models.TierRack36U:
		return 12500
	case models.TierRack48U:
		return 20000
	default:
		return 500
	}
}

func tierCoolingBonus(tier models.Tier) int {
	switch tier {
	case models.TierClosetFloor:
		return 250
	case models.TierRack12U:
		return 1500
	case models.TierRack24U:
		return 4000
	case models.TierRack36U:
		return 9000
	case models.TierRack48U:
		return 16500
	default:
		return 0
	}
}

func isRackTier(tier models.Tier) bool {
	return catalog.TierToRank(tier) >= 2
}

// === BULK ACTIONS ===

func (e *Engine) bulkUpgradeComponents(gs *models.GameState, hardware []models.Hardware, compUpgrades []models.ComponentUpgrade) (*ActionResult, error) {
	upgradeableTypes := map[string]bool{"server": true, "desktop": true, "sbc": true, "mini_pc": true, "gpu_server": true}
	components := []string{"cpu", "ram", "storage", "nic"}
	// Track final state per (hardware_id, component) to avoid duplicate intermediate results
	type compKey struct {
		HardwareID string
		Component  string
	}
	resultMap := make(map[compKey]models.ComponentUpgrade)

	upgraded := true
	for upgraded {
		upgraded = false
		for _, h := range hardware {
			if !upgradeableTypes[h.Type] {
				continue
			}
			for _, comp := range components {
				info := catalog.GetComponentUpgradeInfo(comp)
				if info == nil {
					continue
				}
				currentLevel := 0
				for _, cu := range compUpgrades {
					if cu.HardwareID == h.ID && cu.Component == comp {
						currentLevel = cu.Level
						break
					}
				}
				if currentLevel >= info.MaxLevel {
					continue
				}
				cost := int64(float64(info.BaseCost) * pow(info.CostScale, currentLevel))
				if gs.ComputeUnits < cost {
					continue
				}
				gs.ComputeUnits -= cost
				newLevel := currentLevel + 1
				cu := models.ComponentUpgrade{
					HardwareID:     h.ID,
					Component:      comp,
					Level:          newLevel,
					ComputeBonus:   info.ComputePercent * newLevel,
					PowerReduction: info.PowerReduce * newLevel,
				}
				// Update in-memory list
				found := false
				for i, existing := range compUpgrades {
					if existing.HardwareID == h.ID && existing.Component == comp {
						compUpgrades[i] = cu
						found = true
						break
					}
				}
				if !found {
					compUpgrades = append(compUpgrades, cu)
				}
				// Only keep the final level per component (overwrites intermediate levels)
				resultMap[compKey{h.ID, comp}] = cu
				upgraded = true
			}
		}
	}

	result := make([]models.ComponentUpgrade, 0, len(resultMap))
	for _, cu := range resultMap {
		result = append(result, cu)
	}

	return &ActionResult{ComponentUpgrades: result}, nil
}

func (e *Engine) bulkDeployServices(gs *models.GameState, deployed []models.Service) (*ActionResult, error) {
	deployedNames := make(map[string]bool)
	for _, s := range deployed {
		deployedNames[s.Name] = true
	}

	available := catalog.GetAvailableServices(gs.Tier)
	var newServices []models.Service

	for _, tmpl := range available {
		if deployedNames[tmpl.Name] {
			continue
		}
		if gs.ComputeUnits < tmpl.Cost {
			continue
		}
		if gs.PowerWatts+tmpl.PowerRequired > gs.PowerLimit {
			continue
		}
		gs.ComputeUnits -= tmpl.Cost
		gs.PowerWatts += tmpl.PowerRequired
		svc := models.Service{
			GameStateID:       gs.ID,
			Name:              tmpl.Name,
			Type:              tmpl.Type,
			Tier:              gs.Tier,
			ComputePerTick:    tmpl.ComputePerTick,
			ReputationPerTick: tmpl.ReputationPerTick,
			MoneyPerTick:      tmpl.MoneyPerTick,
		}
		newServices = append(newServices, svc)
		deployedNames[tmpl.Name] = true
	}

	return &ActionResult{NewServices: newServices}, nil
}

type bulkBuyUpgradesPayload struct {
	Type string `json:"type"` // optional: "cooling", "networking", "automation", "knowledge"
}

func (e *Engine) bulkBuyUpgrades(gs *models.GameState, payload json.RawMessage, owned []models.Upgrade) (*ActionResult, error) {
	var p bulkBuyUpgradesPayload
	json.Unmarshal(payload, &p)

	ownedNames := make(map[string]bool)
	for _, u := range owned {
		ownedNames[u.Name] = true
	}

	var newUpgrades []models.Upgrade
	allUpgrades := catalog.GetAvailableUpgrades(gs.Tier)

	for _, tmpl := range allUpgrades {
		if p.Type != "" && tmpl.Type != p.Type {
			continue
		}
		if ownedNames[tmpl.Name] {
			continue
		}
		if catalog.TierToRank(gs.Tier) < catalog.TierToRank(tmpl.MinTier) {
			continue
		}

		// Check cost based on type
		// For automation upgrades, apply prestige scaling
		actualCost := tmpl.Cost
		if tmpl.Type == "automation" {
			actualCost = int64(float64(tmpl.Cost) * prestigeCostScale(gs.ColoCount))
		}
		if tmpl.CostType == "money" {
			if gs.Money < tmpl.Cost {
				continue
			}
			gs.Money -= tmpl.Cost
		} else {
			if gs.ComputeUnits < actualCost {
				continue
			}
			gs.ComputeUnits -= actualCost
		}

		// Apply effects
		switch tmpl.Type {
		case "cooling":
			if val, ok := catalog.CoolingValues[tmpl.Name]; ok {
				gs.CoolingCapacity += val
			}
		case "networking":
			if val, ok := catalog.NetworkTierValues[tmpl.Name]; ok {
				if val > gs.NetworkTier {
					gs.NetworkTier = val
				}
			}
		case "automation":
			if mult, ok := catalog.AutomationMultipliers[tmpl.Name]; ok {
				gs.IdleMultiplier = mult
				gs.AutomationTier++
			}
		case "knowledge":
			if pts, ok := catalog.KnowledgePointValues[tmpl.Name]; ok {
				gs.KnowledgePoints += pts
			}
		}

		u := models.Upgrade{
			GameStateID: gs.ID,
			Name:        tmpl.Name,
			Type:        tmpl.Type,
			Tier:        gs.Tier,
			Persistent:  tmpl.Persistent,
		}
		newUpgrades = append(newUpgrades, u)
		ownedNames[tmpl.Name] = true
	}

	return &ActionResult{NewUpgrades: newUpgrades}, nil
}

func (e *Engine) bulkDeploySaas(gs *models.GameState, deployed []models.Service) (*ActionResult, error) {
	if !gs.SaasUnlocked {
		return &ActionResult{}, nil
	}

	deployedNames := make(map[string]bool)
	for _, s := range deployed {
		deployedNames[s.Name] = true
	}

	available := catalog.GetAvailableSaasServices(gs.Tier)
	var newServices []models.Service
	var newCustomers []models.Customer

	for _, tmpl := range available {
		if deployedNames[tmpl.Name] {
			continue
		}
		cost := int64(float64(tmpl.DeployCost) * prestigeCostScale(gs.ColoCount))
		if gs.ComputeUnits < cost {
			continue
		}
		if gs.Reputation < tmpl.ReputationRequired {
			continue
		}
		if gs.PowerWatts+tmpl.PowerRequired > gs.PowerLimit {
			continue
		}

		gs.ComputeUnits -= cost
		gs.PowerWatts += tmpl.PowerRequired

		svc := models.Service{
			GameStateID:       gs.ID,
			Name:              tmpl.Name,
			Type:              tmpl.Type,
			Tier:              gs.Tier,
			ComputePerTick:    0,
			ReputationPerTick: 5,
			MoneyPerTick:      tmpl.RevenuePerCustomer,
		}
		newServices = append(newServices, svc)

		firstName := catalog.CustomerFirstNames[gs.TotalCustomers%len(catalog.CustomerFirstNames)]
		lastName := catalog.CustomerLastNames[gs.TotalCustomers%len(catalog.CustomerLastNames)]
		customer := models.Customer{
			GameStateID:    gs.ID,
			Name:           firstName + " " + lastName,
			ServiceType:    tmpl.Type,
			MonthlyRevenue: tmpl.RevenuePerCustomer,
			Satisfaction:   100,
		}
		newCustomers = append(newCustomers, customer)
		gs.TotalCustomers++
		deployedNames[tmpl.Name] = true
	}

	return &ActionResult{NewServices: newServices, NewCustomers: newCustomers}, nil
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
