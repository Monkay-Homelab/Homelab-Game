package handlers

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/homelab-game/backend/internal/api/middleware"
	"github.com/homelab-game/backend/internal/api/ws"
	"github.com/homelab-game/backend/internal/database/queries"
	"github.com/homelab-game/backend/internal/game/bitcoin"
	"github.com/homelab-game/backend/internal/game/catalog"
	"github.com/homelab-game/backend/internal/game/engine"
	"github.com/homelab-game/backend/internal/game/events"
	"github.com/homelab-game/backend/internal/models"
)

// userMutexMap provides per-user locking to prevent race conditions on concurrent actions.
type userMutexMap struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newUserMutexMap() *userMutexMap {
	return &userMutexMap{locks: make(map[string]*sync.Mutex)}
}

func (m *userMutexMap) Lock(userID string) {
	m.mu.Lock()
	l, ok := m.locks[userID]
	if !ok {
		l = &sync.Mutex{}
		m.locks[userID] = l
	}
	m.mu.Unlock()
	l.Lock()
}

func (m *userMutexMap) Unlock(userID string) {
	m.mu.Lock()
	l, ok := m.locks[userID]
	m.mu.Unlock()
	if ok {
		l.Unlock()
	}
}

// defaultTickInterval is the default server-side tick interval for computing
// idle progress and pushing state over WebSocket.
const defaultTickInterval = 5 * time.Second

type GameHandler struct {
	gameState    *queries.GameStateQueries
	hardware     *queries.HardwareQueries
	services     *queries.ServiceQueries
	upgrades     *queries.UpgradeQueries
	components   *queries.ComponentUpgradeQueries
	customers    *queries.CustomerQueries
	expenses     *queries.ExpenseQueries
	coloRacks    *queries.ColoRackQueries
	groups       *queries.GroupQueries
	engine       *engine.Engine
	hub          *ws.Hub
	userLocks    *userMutexMap
	bitcoinSvc   *bitcoin.PriceService
	tickInterval time.Duration
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
	bitcoinSvc *bitcoin.PriceService,
) *GameHandler {
	tick := defaultTickInterval
	if s := os.Getenv("TICK_INTERVAL_SECONDS"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			tick = time.Duration(v) * time.Second
		}
	}
	return &GameHandler{gameState: gameState, hardware: hardware, services: services, upgrades: upgrades, components: components, customers: customers, expenses: expenses, coloRacks: coloRacks, groups: groups, engine: eng, hub: hub, userLocks: newUserMutexMap(), bitcoinSvc: bitcoinSvc, tickInterval: tick}
}

type fullStateResponse struct {
	*models.GameState
	Hardware            []models.Hardware            `json:"hardware"`
	Services            []models.Service             `json:"services"`
	Upgrades            []models.Upgrade             `json:"upgrades"`
	ComponentUpgrades   []models.ComponentUpgrade    `json:"component_upgrades"`
	Customers           []models.Customer            `json:"customers"`
	Expenses            []models.Expense             `json:"expenses"`
	ColoRacks           []models.ColoRack            `json:"colo_racks"`
	Events              []*events.GameEvent          `json:"events,omitempty"`
	AvailableHardware   []catalog.HardwareTemplate   `json:"available_hardware"`
	AvailableServices   []catalog.ServiceTemplate    `json:"available_services"`
	AvailableUpgrades   []catalog.UpgradeTemplate    `json:"available_upgrades"`
	AvailableSaas       []catalog.SaasServiceTemplate `json:"available_saas,omitempty"`
	Overheating         bool                         `json:"overheating"`
	Throttled           bool                         `json:"throttled"`
	GroupBonus          float64                      `json:"group_bonus"`
	GroupMembers        int                          `json:"group_members"`
	GlobalDonatedCU     int64                        `json:"global_donated_cu"`
	BitcoinPrice        int64                        `json:"bitcoin_price"`
	BitcoinPriceHistory []models.BitcoinPricePoint   `json:"bitcoin_price_history"`
}

func (h *GameHandler) buildResponse(gs *models.GameState, hw []models.Hardware, svcs []models.Service, ups []models.Upgrade, compUps []models.ComponentUpgrade, custs []models.Customer, exps []models.Expense, colos []models.ColoRack, evts []*events.GameEvent, btcPrice int64, btcHistory []models.BitcoinPricePoint) fullStateResponse {
	resp := fullStateResponse{
		GameState:           gs,
		Hardware:            hw,
		Services:            svcs,
		Upgrades:            ups,
		ComponentUpgrades:   compUps,
		Customers:           custs,
		Expenses:            exps,
		ColoRacks:           colos,
		Events:              evts,
		AvailableHardware:   catalog.GetAvailableHardware(gs.Tier),
		AvailableServices:   catalog.GetAvailableServices(gs.Tier),
		AvailableUpgrades:   catalog.GetAvailableUpgrades(gs.Tier),
		Overheating:         gs.HeatGenerated > gs.CoolingCapacity,
		Throttled:           gs.ThrottleTicksRemaining > 0,
		BitcoinPrice:        btcPrice,
		BitcoinPriceHistory: btcHistory,
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

// processCustomerGrowth uses a separate timestamp (LastCustomerGrowthAt) to accumulate
// time across multiple polling intervals, so 5-second polls don't starve the 60s growth timer.
func (h *GameHandler) processCustomerGrowth(ctx context.Context, gs *models.GameState, custs []models.Customer, svcs []models.Service, now time.Time) ([]models.Customer, []models.Service) {
	customerElapsed := now.Sub(gs.LastCustomerGrowthAt).Seconds()
	if customerElapsed < 1 {
		return custs, svcs
	}

	customerCountByType := make(map[string]int)
	for _, c := range custs {
		customerCountByType[c.ServiceType]++
	}

	grew := false
	for _, s := range svcs {
		saasTemplate := catalog.GetSaasServiceByName(s.Name)
		if saasTemplate == nil {
			continue
		}

		currentCount := customerCountByType[saasTemplate.Type]
		if currentCount >= saasTemplate.MaxCustomers {
			continue
		}

		// Tiered growth: interval = 60 / (1 + currentCustomers * 0.1) seconds
		interval := 60.0 / (1.0 + float64(currentCount)*0.1)
		newCount := int(customerElapsed / interval)
		if newCount < 1 {
			continue
		}
		if currentCount+newCount > saasTemplate.MaxCustomers {
			newCount = saasTemplate.MaxCustomers - currentCount
		}

		for j := 0; j < newCount; j++ {
			firstName := catalog.CustomerFirstNames[(gs.TotalCustomers+j)%len(catalog.CustomerFirstNames)]
			lastName := catalog.CustomerLastNames[(gs.TotalCustomers+j)%len(catalog.CustomerLastNames)]
			newCust := models.Customer{
				GameStateID:    gs.ID,
				Name:           firstName + " " + lastName,
				ServiceType:    saasTemplate.Type,
				MonthlyRevenue: saasTemplate.RevenuePerCustomer,
				Satisfaction:   100,
			}
			if err := h.customers.Create(ctx, &newCust); err == nil {
				custs = append(custs, newCust)
				customerCountByType[saasTemplate.Type]++
			}
		}
		gs.TotalCustomers += newCount
		grew = true

		// Update the service's MoneyPerTick to reflect total customer revenue
		newTotal := int64(customerCountByType[saasTemplate.Type]) * saasTemplate.RevenuePerCustomer
		for i := range svcs {
			if svcs[i].ID == s.ID {
				svcs[i].MoneyPerTick = newTotal
				h.services.Update(ctx, &svcs[i])
				break
			}
		}
	}

	// Only advance the timer when customers actually grew, so partial intervals accumulate
	if grew {
		gs.LastCustomerGrowthAt = now
	}

	return custs, svcs
}

// fetchBitcoinData retrieves the current price and recent history from the bitcoin PriceService,
// converting bitcoin.PricePoint to models.BitcoinPricePoint for the response.
func (h *GameHandler) fetchBitcoinData(ctx context.Context, now time.Time) (int64, []models.BitcoinPricePoint) {
	if h.bitcoinSvc == nil {
		return 0, nil
	}

	price, err := h.bitcoinSvc.GetCurrentPrice(ctx, now)
	if err != nil {
		price = 0
	}

	points, err := h.bitcoinSvc.GetPriceHistory(ctx, 100)
	if err != nil {
		points = nil
	}

	history := make([]models.BitcoinPricePoint, len(points))
	for i, p := range points {
		history[i] = models.BitcoinPricePoint{Time: p.Time, Price: p.Price}
	}

	return price, history
}

// runUserTick computes idle progress for a single user, persists the updated
// state, and pushes the full state over WebSocket. It acquires the per-user
// mutex to prevent concurrent state mutations with PerformAction.
//
// Errors are returned to the caller for logging but are not fatal — a failed
// tick is recovered on the next tick.
func (h *GameHandler) runUserTick(ctx context.Context, userID string) error {
	h.userLocks.Lock(userID)
	defer h.userLocks.Unlock(userID)

	gs, err := h.gameState.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	hw, _ := h.hardware.GetByGameStateID(ctx, gs.ID)
	svcs, _ := h.services.GetByGameStateID(ctx, gs.ID)
	ups, _ := h.upgrades.GetByGameStateID(ctx, gs.ID)
	custs, _ := h.customers.GetByGameStateID(ctx, gs.ID)
	exps, _ := h.expenses.GetByGameStateID(ctx, gs.ID)
	colos, _ := h.coloRacks.GetByUserID(ctx, userID)
	compUps, _ := h.components.GetByGameStateID(ctx, gs.ID)

	now := time.Now()
	// Capture elapsed seconds BEFORE ProcessIdleProgress updates LastTickAt
	elapsed := now.Sub(gs.LastTickAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}

	triggered := h.engine.ProcessIdleProgress(gs, hw, svcs, ups, exps, custs, compUps, now)

	// Group bonus
	groupBonus, groupMembers := h.getGroupBonus(ctx, userID)

	// Add colo rack passive income (boosted by datacenter ownership)
	dcMult := gs.DatacenterIncomeMultiplier
	if dcMult < 1.0 {
		dcMult = 1.0
	}
	for i, cr := range colos {
		decay := math.Pow(0.9, float64(i))
		gs.ComputeUnits += int64(float64(cr.ComputePerTick) * elapsed * dcMult * decay)
		gs.Reputation += int64(float64(cr.ReputationPerTick) * elapsed * dcMult * decay)
		gs.Money += int64(float64(cr.MoneyPerTick) * elapsed * dcMult * decay)
	}

	// Apply group bonus to idle compute earned this tick
	if groupBonus > 1.0 {
		var idleCompute int64
		for _, item := range hw {
			idleCompute += int64(item.ComputePerTick)
		}
		for _, s := range svcs {
			idleCompute += int64(s.ComputePerTick)
		}
		groupExtra := int64(float64(idleCompute) * elapsed * (groupBonus - 1.0))
		gs.ComputeUnits += groupExtra
	}

	// Customer growth for SaaS services
	if gs.SaasUnlocked {
		custs, svcs = h.processCustomerGrowth(ctx, gs, custs, svcs, now)
	}

	if err := h.gameState.Update(ctx, gs); err != nil {
		return err
	}

	for i := range custs {
		h.customers.Update(ctx, &custs[i])
	}

	if len(triggered) > 0 {
		h.pushEvents(userID, triggered)
	}

	// Fetch bitcoin price and history
	btcPrice, btcHistory := h.fetchBitcoinData(ctx, now)

	resp := h.buildResponse(gs, hw, svcs, ups, compUps, custs, exps, colos, triggered, btcPrice, btcHistory)
	resp.GroupBonus = groupBonus
	resp.GroupMembers = groupMembers
	resp.GlobalDonatedCU, _ = h.gameState.GetGlobalDonatedCU(ctx)

	// Serialize the response and push as a "state" WS message
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	h.hub.SendToUser(userID, ws.Message{
		Type:    "state",
		Payload: payload,
	})

	return nil
}

// OnConnect is called when a user establishes a WebSocket connection. It spawns
// a tick goroutine that computes idle progress and pushes state at the
// configured interval. The goroutine exits when the done channel is closed
// (client disconnected).
func (h *GameHandler) OnConnect(userID string, done <-chan struct{}) {
	go func() {
		log.Printf("[tick] goroutine started for user %s (interval=%s)", userID, h.tickInterval)
		defer log.Printf("[tick] goroutine stopped for user %s", userID)

		ticker := time.NewTicker(h.tickInterval)
		defer ticker.Stop()

		// Immediate state push on connect (handles reconnection — client
		// gets state right away without waiting for the first tick).
		if err := h.runUserTick(context.Background(), userID); err != nil {
			log.Printf("[tick] initial tick error for user %s: %v", userID, err)
		}

		for {
			select {
			case <-ticker.C:
				if err := h.runUserTick(context.Background(), userID); err != nil {
					log.Printf("[tick] error for user %s: %v", userID, err)
				}
			case <-done:
				return
			}
		}
	}()
}

// OnDisconnect is called when a user's WebSocket connection is closed. The
// tick goroutine is already stopped by the done channel closure; this method
// exists for observability logging and any future cleanup needs.
func (h *GameHandler) OnDisconnect(userID string) {
	log.Printf("[tick] user %s disconnected", userID)
}

func (h *GameHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=3600")
	cfg := engine.GetConfig()
	json.NewEncoder(w).Encode(cfg)
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
	// Capture elapsed seconds BEFORE ProcessIdleProgress updates LastTickAt
	elapsed := now.Sub(gs.LastTickAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}

	triggered := h.engine.ProcessIdleProgress(gs, hw, svcs, ups, exps, custs, compUps, now)

	// Group bonus
	groupBonus, groupMembers := h.getGroupBonus(r.Context(), userID)

	// Add colo rack passive income (boosted by datacenter ownership)
	dcMult := gs.DatacenterIncomeMultiplier
	if dcMult < 1.0 {
		dcMult = 1.0
	}
	for i, cr := range colos {
		decay := math.Pow(0.9, float64(i))
		gs.ComputeUnits += int64(float64(cr.ComputePerTick) * elapsed * dcMult * decay)
		gs.Reputation += int64(float64(cr.ReputationPerTick) * elapsed * dcMult * decay)
		gs.Money += int64(float64(cr.MoneyPerTick) * elapsed * dcMult * decay)
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
		groupExtra := int64(float64(idleCompute) * elapsed * (groupBonus - 1.0))
		gs.ComputeUnits += groupExtra
	}

	// Customer growth for SaaS services (uses separate timer so 5s polling doesn't starve growth)
	if gs.SaasUnlocked {
		custs, svcs = h.processCustomerGrowth(r.Context(), gs, custs, svcs, now)
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

	// Fetch current bitcoin price and history for the response
	btcPrice, btcHistory := h.fetchBitcoinData(r.Context(), now)

	resp := h.buildResponse(gs, hw, svcs, ups, compUps, custs, exps, colos, triggered, btcPrice, btcHistory)
	resp.GroupBonus = groupBonus
	resp.GroupMembers = groupMembers
	resp.GlobalDonatedCU, _ = h.gameState.GetGlobalDonatedCU(r.Context())
	json.NewEncoder(w).Encode(resp)
}

type actionRequest struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func (h *GameHandler) PerformAction(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	// Lock per-user to prevent race conditions on concurrent actions
	h.userLocks.Lock(userID)
	defer h.userLocks.Unlock(userID)

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
	// Capture elapsed seconds BEFORE ProcessIdleProgress updates LastTickAt
	elapsed := now.Sub(gs.LastTickAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}

	triggered := h.engine.ProcessIdleProgress(gs, hw, svcs, ups, exps, custs, compUps, now)

	// Group bonus
	groupBonus, groupMembers := h.getGroupBonus(r.Context(), userID)

	// Add colo rack passive income (boosted by datacenter ownership)
	dcMult := gs.DatacenterIncomeMultiplier
	if dcMult < 1.0 {
		dcMult = 1.0
	}
	for i, cr := range colos {
		decay := math.Pow(0.9, float64(i))
		gs.ComputeUnits += int64(float64(cr.ComputePerTick) * elapsed * dcMult * decay)
		gs.Reputation += int64(float64(cr.ReputationPerTick) * elapsed * dcMult * decay)
		gs.Money += int64(float64(cr.MoneyPerTick) * elapsed * dcMult * decay)
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
		groupExtra := int64(float64(idleCompute) * elapsed * (groupBonus - 1.0))
		gs.ComputeUnits += groupExtra
	}

	// Customer growth for SaaS services (uses separate timer so 5s polling doesn't starve growth)
	if gs.SaasUnlocked {
		custs, svcs = h.processCustomerGrowth(r.Context(), gs, custs, svcs, now)
	}

	// Only resolve bitcoin price for buy/sell actions to avoid unnecessary mutex contention
	var currentBitcoinPrice int64
	if req.Type == "buy_bitcoin" || req.Type == "sell_bitcoin" {
		if h.bitcoinSvc != nil {
			if p, err := h.bitcoinSvc.GetCurrentPrice(r.Context(), now); err == nil {
				currentBitcoinPrice = p
			}
		}
	}

	result, err := h.engine.ProcessAction(gs, req.Type, req.Payload, hw, svcs, ups, compUps, currentBitcoinPrice)
	if err != nil {
		errMsg, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(errMsg), http.StatusBadRequest)
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
	// Bulk persistence
	for i := range result.NewServices {
		if err := h.services.Create(r.Context(), &result.NewServices[i]); err != nil {
			continue
		}
		svcs = append(svcs, result.NewServices[i])
	}
	for i := range result.NewUpgrades {
		if err := h.upgrades.Create(r.Context(), &result.NewUpgrades[i]); err != nil {
			continue
		}
		ups = append(ups, result.NewUpgrades[i])
	}
	for i := range result.NewCustomers {
		if err := h.customers.Create(r.Context(), &result.NewCustomers[i]); err != nil {
			continue
		}
		custs = append(custs, result.NewCustomers[i])
	}
	for i := range result.ComponentUpgrades {
		if err := h.components.Upsert(r.Context(), &result.ComponentUpgrades[i]); err != nil {
			continue
		}
		// Replace existing entry in compUps rather than appending duplicates
		found := false
		for j := range compUps {
			if compUps[j].HardwareID == result.ComponentUpgrades[i].HardwareID && compUps[j].Component == result.ComponentUpgrades[i].Component {
				compUps[j] = result.ComponentUpgrades[i]
				found = true
				break
			}
		}
		if !found {
			compUps = append(compUps, result.ComponentUpgrades[i])
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

	// Fetch bitcoin price history for the response (price already resolved above)
	var btcHistory []models.BitcoinPricePoint
	if h.bitcoinSvc != nil {
		if points, err := h.bitcoinSvc.GetPriceHistory(r.Context(), 100); err == nil {
			btcHistory = make([]models.BitcoinPricePoint, len(points))
			for i, p := range points {
				btcHistory[i] = models.BitcoinPricePoint{Time: p.Time, Price: p.Price}
			}
		}
	}

	resp := h.buildResponse(gs, hw, svcs, ups, compUps, custs, exps, colos, triggered, currentBitcoinPrice, btcHistory)
	resp.GroupBonus = groupBonus
	resp.GroupMembers = groupMembers
	resp.GlobalDonatedCU, _ = h.gameState.GetGlobalDonatedCU(r.Context())
	json.NewEncoder(w).Encode(resp)

	// Push the same state over WebSocket for immediate client refresh.
	// Fire-and-forget: if the user has no WS connection or the send buffer
	// is full, the push is silently dropped. The HTTP response above is the
	// authoritative response; this is a bonus so the client's WS-driven
	// state path doesn't wait for the next tick.
	if stateJSON, err := json.Marshal(resp); err == nil {
		h.hub.SendToUser(userID, ws.Message{
			Type:    "state",
			Payload: stateJSON,
		})
	}
}
