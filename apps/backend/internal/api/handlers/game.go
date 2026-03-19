package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/homelab-game/backend/internal/api/middleware"
	"github.com/homelab-game/backend/internal/api/ws"
	"github.com/homelab-game/backend/internal/database/queries"
	"github.com/homelab-game/backend/internal/game/catalog"
	"github.com/homelab-game/backend/internal/game/engine"
	"github.com/homelab-game/backend/internal/game/events"
	"github.com/homelab-game/backend/internal/models"
)

type GameHandler struct {
	gameState  *queries.GameStateQueries
	hardware   *queries.HardwareQueries
	services   *queries.ServiceQueries
	upgrades   *queries.UpgradeQueries
	components *queries.ComponentUpgradeQueries
	customers  *queries.CustomerQueries
	expenses   *queries.ExpenseQueries
	coloRacks  *queries.ColoRackQueries
	groups     *queries.GroupQueries
	engine     *engine.Engine
	hub        *ws.Hub
}

func NewGameHandler(
	gameState *queries.GameStateQueries,
	hardware *queries.HardwareQueries,
	services *queries.ServiceQueries,
	upgrades *queries.UpgradeQueries,
	components *queries.ComponentUpgradeQueries,
	customers *queries.CustomerQueries,
	expenses *queries.ExpenseQueries,
	coloRacks *queries.ColoRackQueries,
	groups *queries.GroupQueries,
	eng *engine.Engine,
	hub *ws.Hub,
) *GameHandler {
	return &GameHandler{gameState: gameState, hardware: hardware, services: services, upgrades: upgrades, components: components, customers: customers, expenses: expenses, coloRacks: coloRacks, groups: groups, engine: eng, hub: hub}
}

type fullStateResponse struct {
	*models.GameState
	Hardware           []models.Hardware            `json:"hardware"`
	Services           []models.Service             `json:"services"`
	Upgrades           []models.Upgrade             `json:"upgrades"`
	ComponentUpgrades  []models.ComponentUpgrade    `json:"component_upgrades"`
	Customers          []models.Customer            `json:"customers"`
	Expenses           []models.Expense             `json:"expenses"`
	ColoRacks          []models.ColoRack            `json:"colo_racks"`
	Events             []*events.GameEvent          `json:"events,omitempty"`
	AvailableHardware  []catalog.HardwareTemplate   `json:"available_hardware"`
	AvailableServices  []catalog.ServiceTemplate    `json:"available_services"`
	AvailableUpgrades  []catalog.UpgradeTemplate    `json:"available_upgrades"`
	AvailableSaas      []catalog.SaasServiceTemplate `json:"available_saas,omitempty"`
	Overheating        bool                         `json:"overheating"`
	Throttled          bool                         `json:"throttled"`
	GroupBonus         float64                      `json:"group_bonus"`
	GroupMembers       int                          `json:"group_members"`
}

func (h *GameHandler) buildResponse(gs *models.GameState, hw []models.Hardware, svcs []models.Service, ups []models.Upgrade, compUps []models.ComponentUpgrade, custs []models.Customer, exps []models.Expense, colos []models.ColoRack, evts []*events.GameEvent) fullStateResponse {
	resp := fullStateResponse{
		GameState:         gs,
		Hardware:          hw,
		Services:          svcs,
		Upgrades:          ups,
		ComponentUpgrades: compUps,
		Customers:         custs,
		Expenses:          exps,
		ColoRacks:         colos,
		Events:            evts,
		AvailableHardware: catalog.GetAvailableHardware(gs.Tier),
		AvailableServices: catalog.GetAvailableServices(gs.Tier),
		AvailableUpgrades: catalog.GetAvailableUpgrades(gs.Tier),
		Overheating:       gs.HeatGenerated > gs.CoolingCapacity,
		Throttled:         gs.ThrottleTicksRemaining > 0,
	}
	if gs.SaasUnlocked {
		resp.AvailableSaas = catalog.GetAvailableSaasServices(gs.Tier)
	}
	return resp
}

func (h *GameHandler) pushEvents(userID string, evts []*events.GameEvent) {
	for _, evt := range evts {
		data, _ := json.Marshal(evt)
		h.hub.SendToUser(userID, ws.Message{
			Type:    "event",
			Payload: data,
		})
	}
}

// getGroupBonus returns the group multiplier and member count for a user.
// +5% per member beyond yourself, plus shared service bonus.
func (h *GameHandler) getGroupBonus(ctx context.Context, userID string) (float64, int) {
	group, _, err := h.groups.GetUserGroup(ctx, userID)
	if err != nil || group == nil {
		return 1.0, 0
	}
	members, _ := h.groups.GetMembers(ctx, group.ID)
	count := len(members)
	if count <= 1 {
		return 1.0, count
	}
	// +5% per additional member, capped at +50%
	bonus := 1.0 + float64(count-1)*0.05
	if bonus > 1.5 {
		bonus = 1.5
	}
	return bonus, count
}

func (h *GameHandler) GetState(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	gs, err := h.gameState.GetByUserID(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"game state not found"}`, http.StatusNotFound)
		return
	}

	hw, _ := h.hardware.GetByGameStateID(r.Context(), gs.ID)
	svcs, _ := h.services.GetByGameStateID(r.Context(), gs.ID)
	ups, _ := h.upgrades.GetByGameStateID(r.Context(), gs.ID)
	custs, _ := h.customers.GetByGameStateID(r.Context(), gs.ID)
	exps, _ := h.expenses.GetByGameStateID(r.Context(), gs.ID)
	colos, _ := h.coloRacks.GetByUserID(r.Context(), userID)
	compUps, _ := h.components.GetByGameStateID(r.Context(), gs.ID)

	now := time.Now()
	triggered := h.engine.ProcessIdleProgress(gs, hw, svcs, ups, exps, custs, now)

	// Group bonus
	groupBonus, groupMembers := h.getGroupBonus(r.Context(), userID)

	// Add colo rack passive income (boosted by datacenter ownership)
	dcMult := gs.DatacenterIncomeMultiplier
	if dcMult < 1.0 {
		dcMult = 1.0
	}
	seconds := now.Sub(gs.LastTickAt).Seconds()
	if seconds <= 0 {
		seconds = 5
	}
	for _, cr := range colos {
		gs.ComputeUnits += int64(float64(cr.ComputePerTick) * seconds * dcMult)
		gs.Reputation += int64(float64(cr.ReputationPerTick) * seconds * dcMult)
		gs.Money += int64(float64(cr.MoneyPerTick) * seconds * dcMult)
	}

	// Apply group bonus to idle compute earned this tick
	if groupBonus > 1.0 {
		// Compute idle rate and add group bonus portion
		var idleCompute int64
		for _, hw := range hw {
			idleCompute += int64(hw.ComputePerTick)
		}
		for _, s := range svcs {
			idleCompute += int64(s.ComputePerTick)
		}
		groupExtra := int64(float64(idleCompute) * seconds * (groupBonus - 1.0))
		gs.ComputeUnits += groupExtra
	}

	if err := h.gameState.Update(r.Context(), gs); err != nil {
		http.Error(w, `{"error":"failed to update game state"}`, http.StatusInternalServerError)
		return
	}

	for i := range custs {
		h.customers.Update(r.Context(), &custs[i])
	}

	if len(triggered) > 0 {
		h.pushEvents(userID, triggered)
	}

	resp := h.buildResponse(gs, hw, svcs, ups, compUps, custs, exps, colos, triggered)
	resp.GroupBonus = groupBonus
	resp.GroupMembers = groupMembers
	json.NewEncoder(w).Encode(resp)
}

type actionRequest struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func (h *GameHandler) PerformAction(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req actionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	gs, err := h.gameState.GetByUserID(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"game state not found"}`, http.StatusNotFound)
		return
	}

	hw, _ := h.hardware.GetByGameStateID(r.Context(), gs.ID)
	svcs, _ := h.services.GetByGameStateID(r.Context(), gs.ID)
	ups, _ := h.upgrades.GetByGameStateID(r.Context(), gs.ID)
	custs, _ := h.customers.GetByGameStateID(r.Context(), gs.ID)
	exps, _ := h.expenses.GetByGameStateID(r.Context(), gs.ID)
	colos, _ := h.coloRacks.GetByUserID(r.Context(), userID)
	compUps, _ := h.components.GetByGameStateID(r.Context(), gs.ID)

	now := time.Now()
	triggered := h.engine.ProcessIdleProgress(gs, hw, svcs, ups, exps, custs, now)

	// Group bonus
	groupBonus, groupMembers := h.getGroupBonus(r.Context(), userID)

	// Add colo rack passive income (boosted by datacenter ownership)
	dcMult := gs.DatacenterIncomeMultiplier
	if dcMult < 1.0 {
		dcMult = 1.0
	}
	seconds := now.Sub(gs.LastTickAt).Seconds()
	if seconds <= 0 {
		seconds = 5
	}
	for _, cr := range colos {
		gs.ComputeUnits += int64(float64(cr.ComputePerTick) * seconds * dcMult)
		gs.Reputation += int64(float64(cr.ReputationPerTick) * seconds * dcMult)
		gs.Money += int64(float64(cr.MoneyPerTick) * seconds * dcMult)
	}

	// Apply group bonus
	if groupBonus > 1.0 {
		var idleCompute int64
		for _, h := range hw {
			idleCompute += int64(h.ComputePerTick)
		}
		for _, s := range svcs {
			idleCompute += int64(s.ComputePerTick)
		}
		groupExtra := int64(float64(idleCompute) * seconds * (groupBonus - 1.0))
		gs.ComputeUnits += groupExtra
	}

	result, err := h.engine.ProcessAction(gs, req.Type, req.Payload, hw, svcs, ups, compUps)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	// Persist new records
	if result.NewHardware != nil {
		if err := h.hardware.Create(r.Context(), result.NewHardware); err != nil {
			http.Error(w, `{"error":"failed to save hardware"}`, http.StatusInternalServerError)
			return
		}
		hw = append(hw, *result.NewHardware)
	}
	if result.RemoveHardware != "" {
		if err := h.hardware.DeleteByID(r.Context(), result.RemoveHardware); err != nil {
			http.Error(w, `{"error":"failed to remove hardware"}`, http.StatusInternalServerError)
			return
		}
		filtered := hw[:0]
		for _, item := range hw {
			if item.ID != result.RemoveHardware {
				filtered = append(filtered, item)
			}
		}
		hw = filtered
	}
	if result.NewService != nil {
		if err := h.services.Create(r.Context(), result.NewService); err != nil {
			http.Error(w, `{"error":"failed to save service"}`, http.StatusInternalServerError)
			return
		}
		svcs = append(svcs, *result.NewService)
	}
	if result.NewUpgrade != nil {
		if err := h.upgrades.Create(r.Context(), result.NewUpgrade); err != nil {
			http.Error(w, `{"error":"failed to save upgrade"}`, http.StatusInternalServerError)
			return
		}
		ups = append(ups, *result.NewUpgrade)
	}
	if result.NewCustomer != nil {
		if err := h.customers.Create(r.Context(), result.NewCustomer); err != nil {
			http.Error(w, `{"error":"failed to save customer"}`, http.StatusInternalServerError)
			return
		}
		custs = append(custs, *result.NewCustomer)
	}
	for i := range result.NewExpenses {
		if err := h.expenses.Create(r.Context(), &result.NewExpenses[i]); err != nil {
			http.Error(w, `{"error":"failed to save expense"}`, http.StatusInternalServerError)
			return
		}
		exps = append(exps, result.NewExpenses[i])
	}
	if result.ComponentUpgrade != nil {
		if err := h.components.Upsert(r.Context(), result.ComponentUpgrade); err != nil {
			http.Error(w, `{"error":"failed to save component upgrade"}`, http.StatusInternalServerError)
			return
		}
	}

	// Handle prestige — wipe non-persistent data
	if result.Prestige {
		h.hardware.DeleteByGameStateID(r.Context(), gs.ID)
		h.services.DeleteByGameStateID(r.Context(), gs.ID)
		h.customers.DeleteByGameStateID(r.Context(), gs.ID)
		h.expenses.DeleteByGameStateID(r.Context(), gs.ID)
		h.upgrades.DeleteNonPersistent(r.Context(), gs.ID)
		hw = nil
		svcs = nil
		custs = nil
		exps = nil
		var persistentUps []models.Upgrade
		for _, u := range ups {
			if u.Persistent {
				persistentUps = append(persistentUps, u)
			}
		}
		ups = persistentUps
	}

	if result.NewColoRack != nil {
		if err := h.coloRacks.Create(r.Context(), result.NewColoRack); err != nil {
			http.Error(w, `{"error":"failed to save colo rack"}`, http.StatusInternalServerError)
			return
		}
		colos = append(colos, *result.NewColoRack)
	}

	if err := h.gameState.Update(r.Context(), gs); err != nil {
		http.Error(w, `{"error":"failed to update game state"}`, http.StatusInternalServerError)
		return
	}

	for i := range custs {
		h.customers.Update(r.Context(), &custs[i])
	}

	if len(triggered) > 0 {
		h.pushEvents(userID, triggered)
	}

	resp := h.buildResponse(gs, hw, svcs, ups, compUps, custs, exps, colos, triggered)
	resp.GroupBonus = groupBonus
	resp.GroupMembers = groupMembers
	json.NewEncoder(w).Encode(resp)
}
