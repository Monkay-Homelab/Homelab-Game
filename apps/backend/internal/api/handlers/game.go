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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/homelab-game/backend/internal/game/catalog"
	"github.com/homelab-game/backend/internal/game/engine"
	"github.com/homelab-game/backend/internal/game/events"
	"github.com/homelab-game/backend/internal/models"
)

// userMutexMap provides per-user locking to prevent race conditions on concurrent actions.
type userMutexMap struct {
	mu    sync.Mutex
	locks map[string]*userLock
}

type userLock struct {
	sync.Mutex
	lastUsed time.Time
}

func newUserMutexMap() *userMutexMap {
	m := &userMutexMap{locks: make(map[string]*userLock)}
	go m.cleanup()
	return m
}

func (m *userMutexMap) Lock(userID string) {
	m.mu.Lock()
	l, ok := m.locks[userID]
	if !ok {
		l = &userLock{}
		m.locks[userID] = l
	}
	l.lastUsed = time.Now()
	m.mu.Unlock()
	l.Mutex.Lock()
}

func (m *userMutexMap) Unlock(userID string) {
	m.mu.Lock()
	l, ok := m.locks[userID]
	m.mu.Unlock()
	if ok {
		l.Mutex.Unlock()
	}
}

// cleanup removes stale entries every 5 minutes for users inactive > 10 minutes.
func (m *userMutexMap) cleanup() {
	for {
		time.Sleep(5 * time.Minute)
		m.mu.Lock()
		for id, l := range m.locks {
			if time.Since(l.lastUsed) > 10*time.Minute {
				delete(m.locks, id)
			}
		}
		m.mu.Unlock()
	}
}

// defaultTickInterval is the default server-side tick interval for computing
// idle progress and pushing state over WebSocket.
const defaultTickInterval = 5 * time.Second

// cachedChildData holds all child-table data from the last full tick.
// This is kept in-memory and reused during light ticks when the user
// has not performed any actions since the last full tick.
type cachedChildData struct {
	Hardware          []models.Hardware
	Services          []models.Service
	Upgrades          []models.Upgrade
	ComponentUpgrades []models.ComponentUpgrade
	ResearchLevels    []models.ResearchLevel
	Customers         []models.Customer
	Expenses          []models.Expense
	ColoRacks         []models.ColoRack
	GroupBonus        float64
	GroupMembers      int
}

// userTickState tracks whether a user's game state has changed since the last tick,
// and caches the last-known state for light ticks.
type userTickState struct {
	dirty       bool             // true = action occurred since last tick
	cachedData  *cachedChildData // cached child data from last full tick
	cachedGS    *models.GameState // cached game state from last full tick
	lastPayload []byte           // pre-serialized JSON of last response
}

// tickStateMap provides thread-safe per-user tick state tracking.
type tickStateMap struct {
	mu    sync.RWMutex
	state map[string]*userTickState
}

func newTickStateMap() *tickStateMap {
	return &tickStateMap{state: make(map[string]*userTickState)}
}

func (m *tickStateMap) Get(userID string) *userTickState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state[userID]
}

func (m *tickStateMap) Set(userID string, ts *userTickState) {
	m.mu.Lock()
	m.state[userID] = ts
	m.mu.Unlock()
}

func (m *tickStateMap) Delete(userID string) {
	m.mu.Lock()
	delete(m.state, userID)
	m.mu.Unlock()
}

func (m *tickStateMap) MarkDirty(userID string) {
	m.mu.RLock()
	ts := m.state[userID]
	m.mu.RUnlock()
	if ts != nil {
		ts.dirty = true // No lock needed — single writer (processAction holds per-user mutex)
	}
}

type GameHandler struct {
	pool          *pgxpool.Pool
	gameState     *queries.GameStateQueries
	hardware      *queries.HardwareQueries
	services      *queries.ServiceQueries
	upgrades      *queries.UpgradeQueries
	components    *queries.ComponentUpgradeQueries
	customers     *queries.CustomerQueries
	expenses      *queries.ExpenseQueries
	coloRacks     *queries.ColoRackQueries
	groups        *queries.GroupQueries
	research      *queries.ResearchLevelQueries
	engine        *engine.Engine
	hub           *ws.Hub
	broadcaster   ws.MessageBroadcaster
	userLocks     *userMutexMap
	tickState     *tickStateMap
	bitcoinSvc    *bitcoin.PriceService
	globalCUCache *GlobalDonatedCUCache
	tickInterval  time.Duration
}

func NewGameHandler(
	pool *pgxpool.Pool,
	gameState *queries.GameStateQueries,
	hardware *queries.HardwareQueries,
	services *queries.ServiceQueries,
	upgrades *queries.UpgradeQueries,
	components *queries.ComponentUpgradeQueries,
	customers *queries.CustomerQueries,
	expenses *queries.ExpenseQueries,
	coloRacks *queries.ColoRackQueries,
	groups *queries.GroupQueries,
	research *queries.ResearchLevelQueries,
	eng *engine.Engine,
	hub *ws.Hub,
	broadcaster ws.MessageBroadcaster,
	bitcoinSvc *bitcoin.PriceService,
	globalCUCache *GlobalDonatedCUCache,
) *GameHandler {
	tick := defaultTickInterval
	if s := os.Getenv("TICK_INTERVAL_SECONDS"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			tick = time.Duration(v) * time.Second
		}
	}
	return &GameHandler{pool: pool, gameState: gameState, hardware: hardware, services: services, upgrades: upgrades, components: components, customers: customers, expenses: expenses, coloRacks: coloRacks, groups: groups, research: research, engine: eng, hub: hub, broadcaster: broadcaster, userLocks: newUserMutexMap(), tickState: newTickStateMap(), bitcoinSvc: bitcoinSvc, globalCUCache: globalCUCache, tickInterval: tick}
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
	ResearchLevels      []models.ResearchLevel       `json:"research_levels"`
}

func (h *GameHandler) buildResponse(gs *models.GameState, hw []models.Hardware, svcs []models.Service, ups []models.Upgrade, compUps []models.ComponentUpgrade, custs []models.Customer, exps []models.Expense, colos []models.ColoRack, evts []*events.GameEvent, btcPrice int64, btcHistory []models.BitcoinPricePoint, researchLevels []models.ResearchLevel) fullStateResponse {
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
		ResearchLevels:      researchLevels,
	}
	if gs.SaasUnlocked {
		resp.AvailableSaas = catalog.GetAvailableSaasServices(gs.Tier)
	}
	return resp
}

func (h *GameHandler) pushEvents(userID string, evts []*events.GameEvent) {
	for _, evt := range evts {
		data, _ := json.Marshal(evt)
		h.broadcaster.SendToUser(userID, ws.Message{
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
// Ticks run in two modes:
// - Full tick: loads all data from DB, runs engine, persists, pushes. Runs when
//   the user is dirty (action occurred since last tick) or on first tick.
// - Light tick: reuses cached child data from the last full tick, runs engine
//   on the in-memory state, persists only the game_state row, pushes. Runs when
//   the user is idle (no actions since last tick).
//
// Errors are returned to the caller for logging but are not fatal — a failed
// tick is recovered on the next tick.
func (h *GameHandler) runUserTick(ctx context.Context, userID string) error {
	h.userLocks.Lock(userID)
	defer h.userLocks.Unlock(userID)

	ts := h.tickState.Get(userID)
	if ts == nil {
		ts = &userTickState{dirty: true}
		h.tickState.Set(userID, ts)
	}

	if ts.dirty || ts.cachedData == nil || ts.cachedGS == nil {
		return h.runFullTick(ctx, userID, ts)
	}
	return h.runLightTick(ctx, userID, ts)
}

// runFullTick loads all data from DB, runs engine processing, persists
// everything, pushes state, and caches the results for future light ticks.
func (h *GameHandler) runFullTick(ctx context.Context, userID string, ts *userTickState) error {
	data, err := queries.LoadFullGameState(ctx, h.pool, userID)
	if err != nil {
		return err
	}

	gs := data.GameState
	hw := data.Hardware
	svcs := data.Services
	ups := data.Upgrades
	custs := data.Customers
	exps := data.Expenses
	colos := data.ColoRacks
	compUps := data.ComponentUps
	researchLevels := data.ResearchLevels

	now := time.Now()
	// Capture elapsed seconds BEFORE ProcessIdleProgress updates LastTickAt
	elapsed := now.Sub(gs.LastTickAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}

	triggered := h.engine.ProcessIdleProgress(gs, hw, svcs, ups, exps, custs, compUps, researchLevels, now)

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

	resp := h.buildResponse(gs, hw, svcs, ups, compUps, custs, exps, colos, triggered, btcPrice, btcHistory, researchLevels)
	resp.GroupBonus = groupBonus
	resp.GroupMembers = groupMembers
	resp.GlobalDonatedCU = h.globalCUCache.Get()

	// Serialize the response and push as a "state" WS message
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	h.broadcaster.SendToUser(userID, ws.Message{
		Type:    "state",
		Payload: payload,
	})

	// Cache state for future light ticks
	ts.cachedGS = gs
	ts.cachedData = &cachedChildData{
		Hardware:          hw,
		Services:          svcs,
		Upgrades:          ups,
		ComponentUpgrades: compUps,
		ResearchLevels:    researchLevels,
		Customers:         custs,
		Expenses:          exps,
		ColoRacks:         colos,
		GroupBonus:        groupBonus,
		GroupMembers:      groupMembers,
	}
	ts.lastPayload = payload
	ts.dirty = false

	return nil
}

// runLightTick reuses cached child data from the last full tick. It runs
// ProcessIdleProgress on the in-memory game state, computes colo rack income
// and group bonus from cached data, handles customer growth, and persists
// only the game_state row (1 DB query instead of 14+).
func (h *GameHandler) runLightTick(ctx context.Context, userID string, ts *userTickState) error {
	gs := ts.cachedGS
	cd := ts.cachedData

	// We need a fresh copy of customers and services since processCustomerGrowth
	// may append to them (and we need the updated slices for the cache).
	custs := cd.Customers
	svcs := cd.Services

	now := time.Now()
	// Capture elapsed seconds BEFORE ProcessIdleProgress updates LastTickAt
	elapsed := now.Sub(gs.LastTickAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}

	triggered := h.engine.ProcessIdleProgress(gs, cd.Hardware, svcs, cd.Upgrades, cd.Expenses, custs, cd.ComponentUpgrades, cd.ResearchLevels, now)

	// Colo rack passive income from cached colo racks
	dcMult := gs.DatacenterIncomeMultiplier
	if dcMult < 1.0 {
		dcMult = 1.0
	}
	for i, cr := range cd.ColoRacks {
		decay := math.Pow(0.9, float64(i))
		gs.ComputeUnits += int64(float64(cr.ComputePerTick) * elapsed * dcMult * decay)
		gs.Reputation += int64(float64(cr.ReputationPerTick) * elapsed * dcMult * decay)
		gs.Money += int64(float64(cr.MoneyPerTick) * elapsed * dcMult * decay)
	}

	// Group bonus from cached values (group membership changes are infrequent)
	groupBonus := cd.GroupBonus
	groupMembers := cd.GroupMembers
	if groupBonus > 1.0 {
		var idleCompute int64
		for _, item := range cd.Hardware {
			idleCompute += int64(item.ComputePerTick)
		}
		for _, s := range svcs {
			idleCompute += int64(s.ComputePerTick)
		}
		groupExtra := int64(float64(idleCompute) * elapsed * (groupBonus - 1.0))
		gs.ComputeUnits += groupExtra
	}

	// Customer growth for SaaS services (may create new customers — these DO need DB writes)
	if gs.SaasUnlocked {
		custs, svcs = h.processCustomerGrowth(ctx, gs, custs, svcs, now)
		// Update cached slices in case customers/services grew
		cd.Customers = custs
		cd.Services = svcs
	}

	// Persist ONLY the game_state update (1 DB query)
	if err := h.gameState.Update(ctx, gs); err != nil {
		return err
	}

	if len(triggered) > 0 {
		h.pushEvents(userID, triggered)
	}

	// Fetch bitcoin price from in-memory PriceService (no DB query)
	btcPrice, btcHistory := h.fetchBitcoinData(ctx, now)

	globalDonatedCU := h.globalCUCache.Get()

	resp := h.buildResponse(gs, cd.Hardware, svcs, cd.Upgrades, cd.ComponentUpgrades, custs, cd.Expenses, cd.ColoRacks, triggered, btcPrice, btcHistory, cd.ResearchLevels)
	resp.GroupBonus = groupBonus
	resp.GroupMembers = groupMembers
	resp.GlobalDonatedCU = globalDonatedCU

	// Serialize the response and push as a "state" WS message
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	h.broadcaster.SendToUser(userID, ws.Message{
		Type:    "state",
		Payload: payload,
	})

	ts.lastPayload = payload

	return nil
}

// OnConnect is called when a user establishes a WebSocket connection. It spawns
// a tick goroutine that computes idle progress and pushes state at the
// configured interval. The goroutine exits when the done channel is closed
// (client disconnected).
func (h *GameHandler) OnConnect(userID string, done <-chan struct{}) {
	// Initialize dirty state — first tick always runs as a full tick.
	h.tickState.Set(userID, &userTickState{dirty: true})

	go func() {
		log.Printf("[tick] goroutine started for user %s (interval=%s)", userID, h.tickInterval)
		defer log.Printf("[tick] goroutine stopped for user %s", userID)

		ticker := time.NewTicker(h.tickInterval)
		defer ticker.Stop()

		// Immediate state push on connect (handles reconnection — client
		// gets state right away without waiting for the first tick).
		initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := h.runUserTick(initCtx, userID); err != nil {
			log.Printf("[tick] initial tick error for user %s: %v", userID, err)
		}
		initCancel()

		for {
			select {
			case <-ticker.C:
				tickCtx, tickCancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := h.runUserTick(tickCtx, userID); err != nil {
					log.Printf("[tick] error for user %s: %v", userID, err)
				}
				tickCancel()
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
	h.tickState.Delete(userID)
	log.Printf("[tick] user %s disconnected", userID)
}

func (h *GameHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=3600")
	cfg := engine.GetConfig()
	json.NewEncoder(w).Encode(cfg)
}

func (h *GameHandler) GetState(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	data, err := queries.LoadFullGameState(r.Context(), h.pool, userID)
	if err != nil {
		http.Error(w, `{"error":"game state not found"}`, http.StatusNotFound)
		return
	}

	gs := data.GameState
	hw := data.Hardware
	svcs := data.Services
	ups := data.Upgrades
	custs := data.Customers
	exps := data.Expenses
	colos := data.ColoRacks
	compUps := data.ComponentUps
	researchLevels := data.ResearchLevels

	now := time.Now()
	// Capture elapsed seconds BEFORE ProcessIdleProgress updates LastTickAt
	elapsed := now.Sub(gs.LastTickAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}

	triggered := h.engine.ProcessIdleProgress(gs, hw, svcs, ups, exps, custs, compUps, researchLevels, now)

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

	resp := h.buildResponse(gs, hw, svcs, ups, compUps, custs, exps, colos, triggered, btcPrice, btcHistory, researchLevels)
	resp.GroupBonus = groupBonus
	resp.GroupMembers = groupMembers
	resp.GlobalDonatedCU = h.globalCUCache.Get()
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

	data, err := queries.LoadFullGameState(r.Context(), h.pool, userID)
	if err != nil {
		http.Error(w, `{"error":"game state not found"}`, http.StatusNotFound)
		return
	}

	gs := data.GameState
	hw := data.Hardware
	svcs := data.Services
	ups := data.Upgrades
	custs := data.Customers
	exps := data.Expenses
	colos := data.ColoRacks
	compUps := data.ComponentUps
	researchLevels := data.ResearchLevels

	now := time.Now()
	// Capture elapsed seconds BEFORE ProcessIdleProgress updates LastTickAt
	elapsed := now.Sub(gs.LastTickAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}

	triggered := h.engine.ProcessIdleProgress(gs, hw, svcs, ups, exps, custs, compUps, researchLevels, now)

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

	result, err := h.engine.ProcessAction(gs, req.Type, req.Payload, hw, svcs, ups, compUps, researchLevels, currentBitcoinPrice)
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
	if result.ResearchLevel != nil {
		if err := h.research.Upsert(r.Context(), result.ResearchLevel); err != nil {
			http.Error(w, `{"error":"failed to save research level"}`, http.StatusInternalServerError)
			return
		}
		found := false
		for i := range researchLevels {
			if researchLevels[i].ResearchNode == result.ResearchLevel.ResearchNode {
				researchLevels[i] = *result.ResearchLevel
				found = true
				break
			}
		}
		if !found {
			researchLevels = append(researchLevels, *result.ResearchLevel)
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

	// Mark user as dirty so the next tick runs a full DB reload.
	// The action may have changed child tables (bought hardware, sold services,
	// prestige wipe, etc.), so the light tick's cached data would be stale.
	h.tickState.MarkDirty(userID)

	// Update global donated CU cache immediately after a successful donate_cu
	// so the response reflects the donation without waiting for periodic refresh.
	if req.Type == "donate_cu" {
		var dp struct {
			Amount int64 `json:"amount"`
		}
		if json.Unmarshal(req.Payload, &dp) == nil && dp.Amount > 0 {
			h.globalCUCache.Add(dp.Amount)
		}
	}

	// Note: customer satisfaction is recalculated by ProcessIdleProgress on
	// every state load. The per-customer Update loop was removed — it was
	// issuing N UPDATE queries with unchanged data on every action.

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

	resp := h.buildResponse(gs, hw, svcs, ups, compUps, custs, exps, colos, triggered, currentBitcoinPrice, btcHistory, researchLevels)
	resp.GroupBonus = groupBonus
	resp.GroupMembers = groupMembers
	resp.GlobalDonatedCU = h.globalCUCache.Get()
	json.NewEncoder(w).Encode(resp)

	// Push the same state over WebSocket for immediate client refresh.
	// Fire-and-forget: if the user has no WS connection or the send buffer
	// is full, the push is silently dropped. The HTTP response above is the
	// authoritative response; this is a bonus so the client's WS-driven
	// state path doesn't wait for the next tick.
	if stateJSON, err := json.Marshal(resp); err == nil {
		h.broadcaster.SendToUser(userID, ws.Message{
			Type:    "state",
			Payload: stateJSON,
		})
	}
}

// wsActionRequest is the expected JSON format for WebSocket action messages.
type wsActionRequest struct {
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// wsActionResult is the JSON response sent back over WebSocket after processing an action.
type wsActionResult struct {
	Type    string             `json:"type"`
	ID      string             `json:"id"`
	Success bool               `json:"success"`
	State   *fullStateResponse `json:"state,omitempty"`
	Error   string             `json:"error,omitempty"`
}

// actionError is a structured error type for WebSocket action processing.
// It classifies errors so HandleWSAction can mask internal details from clients
// (security: don't leak database errors, stack traces, etc.).
type actionError struct {
	msg      string // human-readable error message
	internal bool   // true = server/infrastructure error (masked for client)
	notFound bool   // true = requested resource was not found
}

func (e *actionError) Error() string {
	return e.msg
}

// HandleWSAction processes a game action received over WebSocket. It is called
// from the hub's OnMessage callback (via a goroutine in main.go).
func (h *GameHandler) HandleWSAction(userID string, data []byte) {
	var req wsActionRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return // malformed JSON — silently drop
	}

	// Only process "action" messages; ignore anything else.
	if req.Type != "action" {
		return
	}

	// Require a request ID for response correlation.
	if req.ID == "" {
		result := wsActionResult{
			Type:    "action_result",
			ID:      "",
			Success: false,
			Error:   "missing request id",
		}
		if resData, err := json.Marshal(result); err == nil {
			h.broadcaster.SendToUserBytes(userID, resData)
		}
		return
	}

	// Rate limit check
	if !middleware.CheckGameActionRate(userID) {
		result := wsActionResult{
			Type:    "action_result",
			ID:      req.ID,
			Success: false,
			Error:   "rate limited",
		}
		if resData, err := json.Marshal(result); err == nil {
			h.broadcaster.SendToUserBytes(userID, resData)
		}
		return
	}

	// Track whether the per-user mutex is held so the panic recovery can
	// release it. HandleWSAction unlocks manually at multiple exit points
	// rather than using a single defer, so a panic between Lock and Unlock
	// would deadlock all subsequent actions for this user.
	locked := false

	// Recover from panics in the action processing goroutine. An unrecovered
	// panic here would crash the entire process since HandleWSAction runs in
	// a goroutine spawned by the hub's OnMessage callback.
	defer func() {
		if r := recover(); r != nil {
			if locked {
				h.userLocks.Unlock(userID)
			}
			log.Printf("[ws-action] user=%s action=%s id=%s panic=%v", userID, req.Action, req.ID, r)
			result := wsActionResult{
				Type:    "action_result",
				ID:      req.ID,
				Success: false,
				Error:   "internal server error",
			}
			if resData, err := json.Marshal(result); err == nil {
				h.broadcaster.SendToUserBytes(userID, resData)
			}
		}
	}()

	// Lock per-user to prevent race conditions
	h.userLocks.Lock(userID)
	locked = true

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)
	tickCtx, tickCancel := context.WithTimeout(ctx, 30*time.Second)
	defer tickCancel()

	data2, err := queries.LoadFullGameState(tickCtx, h.pool, userID)
	if err != nil {
		h.userLocks.Unlock(userID)
		locked = false
		result := wsActionResult{
			Type:    "action_result",
			ID:      req.ID,
			Success: false,
			Error:   "internal server error",
		}
		if resData, err := json.Marshal(result); err == nil {
			h.broadcaster.SendToUserBytes(userID, resData)
		}
		return
	}

	gs := data2.GameState
	hw := data2.Hardware
	svcs := data2.Services
	ups := data2.Upgrades
	custs := data2.Customers
	exps := data2.Expenses
	colos := data2.ColoRacks
	compUps := data2.ComponentUps
	researchLevels := data2.ResearchLevels

	now := time.Now()
	elapsed := now.Sub(gs.LastTickAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}

	triggered := h.engine.ProcessIdleProgress(gs, hw, svcs, ups, exps, custs, compUps, researchLevels, now)

	groupBonus, groupMembers := h.getGroupBonus(tickCtx, userID)

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

	if groupBonus > 1.0 {
		idleCompute := int64(0)
		for _, item := range hw {
			idleCompute += int64(item.ComputePerTick)
		}
		for _, s := range svcs {
			idleCompute += int64(s.ComputePerTick)
		}
		groupExtra := int64(float64(idleCompute) * elapsed * (groupBonus - 1.0))
		gs.ComputeUnits += groupExtra
	}

	if gs.SaasUnlocked {
		custs, svcs = h.processCustomerGrowth(tickCtx, gs, custs, svcs, now)
	}

	currentBitcoinPrice, btcHistory := h.fetchBitcoinData(tickCtx, now)

	result, actionErr := h.engine.ProcessAction(gs, req.Action, req.Payload, hw, svcs, ups, compUps, researchLevels, currentBitcoinPrice)
	if actionErr != nil {
		h.userLocks.Unlock(userID)
		locked = false
		res := wsActionResult{
			Type:    "action_result",
			ID:      req.ID,
			Success: false,
			Error:   actionErr.Error(),
		}
		if resData, err := json.Marshal(res); err == nil {
			h.broadcaster.SendToUserBytes(userID, resData)
		}
		return
	}

	// Persist action results
	if result.NewHardware != nil {
		h.hardware.Create(tickCtx, result.NewHardware)
		hw = append(hw, *result.NewHardware)
	}
	if result.RemoveHardware != "" {
		h.hardware.DeleteByID(tickCtx, result.RemoveHardware)
		filtered := hw[:0]
		for _, item := range hw {
			if item.ID != result.RemoveHardware {
				filtered = append(filtered, item)
			}
		}
		hw = filtered
	}
	if result.NewService != nil {
		h.services.Create(tickCtx, result.NewService)
		svcs = append(svcs, *result.NewService)
	}
	if result.NewUpgrade != nil {
		h.upgrades.Create(tickCtx, result.NewUpgrade)
		ups = append(ups, *result.NewUpgrade)
	}
	if result.NewCustomer != nil {
		h.customers.Create(tickCtx, result.NewCustomer)
		custs = append(custs, *result.NewCustomer)
	}
	for i := range result.NewExpenses {
		h.expenses.Create(tickCtx, &result.NewExpenses[i])
		exps = append(exps, result.NewExpenses[i])
	}
	if result.ComponentUpgrade != nil {
		h.components.Upsert(tickCtx, result.ComponentUpgrade)
	}
	if result.ResearchLevel != nil {
		h.research.Upsert(tickCtx, result.ResearchLevel)
		found := false
		for i := range researchLevels {
			if researchLevels[i].ResearchNode == result.ResearchLevel.ResearchNode {
				researchLevels[i] = *result.ResearchLevel
				found = true
				break
			}
		}
		if !found {
			researchLevels = append(researchLevels, *result.ResearchLevel)
		}
	}
	// Bulk persistence
	for i := range result.NewServices {
		h.services.Create(tickCtx, &result.NewServices[i])
		svcs = append(svcs, result.NewServices[i])
	}
	for i := range result.NewUpgrades {
		h.upgrades.Create(tickCtx, &result.NewUpgrades[i])
		ups = append(ups, result.NewUpgrades[i])
	}
	for i := range result.NewCustomers {
		h.customers.Create(tickCtx, &result.NewCustomers[i])
		custs = append(custs, result.NewCustomers[i])
	}
	for i := range result.ComponentUpgrades {
		h.components.Upsert(tickCtx, &result.ComponentUpgrades[i])
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

	if result.Prestige {
		h.hardware.DeleteByGameStateID(tickCtx, gs.ID)
		h.services.DeleteByGameStateID(tickCtx, gs.ID)
		h.customers.DeleteByGameStateID(tickCtx, gs.ID)
		h.expenses.DeleteByGameStateID(tickCtx, gs.ID)
		h.upgrades.DeleteNonPersistent(tickCtx, gs.ID)
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
		h.coloRacks.Create(tickCtx, result.NewColoRack)
		colos = append(colos, *result.NewColoRack)
	}

	h.gameState.Update(tickCtx, gs)

	// Update global donated CU cache for donate_cu actions
	if req.Action == "donate_cu" {
		var dp struct {
			Amount int64 `json:"amount"`
		}
		if json.Unmarshal(req.Payload, &dp) == nil && dp.Amount > 0 {
			h.globalCUCache.Add(dp.Amount)
		}
	}

	// Mark dirty for next tick
	h.tickState.MarkDirty(userID)

	h.userLocks.Unlock(userID)
	locked = false

	if len(triggered) > 0 {
		h.pushEvents(userID, triggered)
	}

	resp := h.buildResponse(gs, hw, svcs, ups, compUps, custs, exps, colos, triggered, currentBitcoinPrice, btcHistory, researchLevels)
	resp.GroupBonus = groupBonus
	resp.GroupMembers = groupMembers
	resp.GlobalDonatedCU = h.globalCUCache.Get()

	res := wsActionResult{
		Type:    "action_result",
		ID:      req.ID,
		Success: true,
		State:   &resp,
	}
	if resData, err := json.Marshal(res); err == nil {
		h.broadcaster.SendToUserBytes(userID, resData)
	}
}
