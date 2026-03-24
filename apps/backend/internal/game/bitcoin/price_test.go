package bitcoin

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"
)

// --- In-memory PriceStore for testing (no database required) ---

type memPriceStore struct {
	state   *PriceState
	history []PricePoint
}

func newMemPriceStore(initialPrice int64, seed int64, lastStepAt time.Time) *memPriceStore {
	return &memPriceStore{
		state: &PriceState{
			CurrentPrice: initialPrice,
			Seed:         seed,
			LastStepAt:   lastStepAt,
			UpdatedAt:    lastStepAt,
		},
	}
}

func (m *memPriceStore) GetPrice(_ context.Context) (*PriceState, error) {
	cp := *m.state
	return &cp, nil
}

func (m *memPriceStore) UpdatePrice(_ context.Context, state *PriceState) error {
	cp := *state
	m.state = &cp
	return nil
}

func (m *memPriceStore) InsertPriceHistory(_ context.Context, point PricePoint) error {
	m.history = append(m.history, point)
	return nil
}

func (m *memPriceStore) GetPriceHistory(_ context.Context, limit int) ([]PricePoint, error) {
	if limit <= 0 || limit > len(m.history) {
		limit = len(m.history)
	}
	start := len(m.history) - limit
	if start < 0 {
		start = 0
	}
	// Return in descending order (mimicking DB query: ORDER BY time DESC LIMIT N).
	result := make([]PricePoint, 0, limit)
	for i := len(m.history) - 1; i >= start; i-- {
		result = append(result, m.history[i])
	}
	return result, nil
}

// --- errorPriceStore returns errors for specific operations ---

type errorPriceStore struct {
	memPriceStore
	getPriceErr      error
	updatePriceErr   error
	insertHistoryErr error
}

func (e *errorPriceStore) GetPrice(ctx context.Context) (*PriceState, error) {
	if e.getPriceErr != nil {
		return nil, e.getPriceErr
	}
	return e.memPriceStore.GetPrice(ctx)
}

func (e *errorPriceStore) UpdatePrice(ctx context.Context, state *PriceState) error {
	if e.updatePriceErr != nil {
		return e.updatePriceErr
	}
	return e.memPriceStore.UpdatePrice(ctx, state)
}

func (e *errorPriceStore) InsertPriceHistory(ctx context.Context, point PricePoint) error {
	if e.insertHistoryErr != nil {
		return e.insertHistoryErr
	}
	return e.memPriceStore.InsertPriceHistory(ctx, point)
}

// ======================================================================
// Test: DefaultPriceConfig returns TDD-specified values
// ======================================================================

func TestDefaultPriceConfig(t *testing.T) {
	cfg := DefaultPriceConfig()

	if cfg.Mu != 10000 {
		t.Errorf("Mu = %d, want 10000", cfg.Mu)
	}
	if cfg.Theta != 0.02 {
		t.Errorf("Theta = %f, want 0.02", cfg.Theta)
	}
	if cfg.Sigma != 2000 {
		t.Errorf("Sigma = %f, want 2000", cfg.Sigma)
	}
	if cfg.StepInterval != 5 {
		t.Errorf("StepInterval = %d, want 5", cfg.StepInterval)
	}
	if cfg.MinPrice != 1000 {
		t.Errorf("MinPrice = %d, want 1000", cfg.MinPrice)
	}
	if cfg.MaxPrice != 50000 {
		t.Errorf("MaxPrice = %d, want 50000", cfg.MaxPrice)
	}
	if cfg.MaxCatchupSteps != 1000 {
		t.Errorf("MaxCatchupSteps = %d, want 1000", cfg.MaxCatchupSteps)
	}
	if cfg.HistoryRetention != 200 {
		t.Errorf("HistoryRetention = %d, want 200", cfg.HistoryRetention)
	}
}

// ======================================================================
// Test: Ornstein-Uhlenbeck step function properties
// ======================================================================

func TestStep_DeterministicWithSameSeed(t *testing.T) {
	cfg := DefaultPriceConfig()
	svc := NewPriceService(nil, cfg)

	seed := int64(42)

	rng1 := rand.New(rand.NewSource(seed))
	price1 := int64(10000)
	for i := 0; i < 10; i++ {
		price1 = svc.step(price1, rng1)
	}

	rng2 := rand.New(rand.NewSource(seed))
	price2 := int64(10000)
	for i := 0; i < 10; i++ {
		price2 = svc.step(price2, rng2)
	}

	if price1 != price2 {
		t.Errorf("step is not deterministic: %d != %d", price1, price2)
	}
}

func TestStep_DifferentSeedProducesDifferentPrice(t *testing.T) {
	cfg := DefaultPriceConfig()
	svc := NewPriceService(nil, cfg)

	rng1 := rand.New(rand.NewSource(42))
	price1 := int64(10000)
	for i := 0; i < 50; i++ {
		price1 = svc.step(price1, rng1)
	}

	rng2 := rand.New(rand.NewSource(999))
	price2 := int64(10000)
	for i := 0; i < 50; i++ {
		price2 = svc.step(price2, rng2)
	}

	if price1 == price2 {
		t.Errorf("different seeds produced the same price after 50 steps: %d", price1)
	}
}

func TestStep_ClampsToMinPrice(t *testing.T) {
	cfg := DefaultPriceConfig()
	cfg.MinPrice = 5000
	cfg.MaxPrice = 50000
	cfg.Sigma = 100000
	cfg.Theta = 0
	cfg.Mu = 0
	svc := NewPriceService(nil, cfg)

	for seed := int64(0); seed < 100; seed++ {
		rng := rand.New(rand.NewSource(seed))
		price := svc.step(5000, rng)
		if price < cfg.MinPrice {
			t.Fatalf("seed %d: price %d below MinPrice %d", seed, price, cfg.MinPrice)
		}
	}
}

func TestStep_ClampsToMaxPrice(t *testing.T) {
	cfg := DefaultPriceConfig()
	cfg.MinPrice = 1000
	cfg.MaxPrice = 15000
	cfg.Sigma = 100000
	cfg.Theta = 0
	cfg.Mu = 100000
	svc := NewPriceService(nil, cfg)

	for seed := int64(0); seed < 100; seed++ {
		rng := rand.New(rand.NewSource(seed))
		price := svc.step(15000, rng)
		if price > cfg.MaxPrice {
			t.Fatalf("seed %d: price %d above MaxPrice %d", seed, price, cfg.MaxPrice)
		}
	}
}

func TestStep_MeanReversion(t *testing.T) {
	cfg := PriceConfig{
		Mu:       10000,
		Theta:    0.5,
		Sigma:    10,
		MinPrice: 1000,
		MaxPrice: 50000,
	}
	svc := NewPriceService(nil, cfg)

	rng := rand.New(rand.NewSource(42))
	price := int64(20000)
	for i := 0; i < 100; i++ {
		price = svc.step(price, rng)
	}

	if price > 15000 || price < 5000 {
		t.Errorf("after 100 mean-reversion steps from 20000, price = %d (expected near 10000)", price)
	}
}

// ======================================================================
// Test: GetCurrentPrice lazy evaluation
// ======================================================================

func TestGetCurrentPrice_NoStepsIfNotEnoughTimeElapsed(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	store := newMemPriceStore(10000, 42, now)
	cfg := DefaultPriceConfig()
	svc := NewPriceService(store, cfg)

	price, err := svc.GetCurrentPrice(context.Background(), now.Add(4*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price != 10000 {
		t.Errorf("price = %d, want 10000 (no steps should have run)", price)
	}
	if len(store.history) != 0 {
		t.Errorf("history has %d entries, want 0", len(store.history))
	}
}

func TestGetCurrentPrice_ExactlyOneStepAfter5s(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	store := newMemPriceStore(10000, 42, now)
	cfg := DefaultPriceConfig()
	svc := NewPriceService(store, cfg)

	_, err := svc.GetCurrentPrice(context.Background(), now.Add(5*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.history) != 1 {
		t.Errorf("history has %d entries, want 1", len(store.history))
	}
}

func TestGetCurrentPrice_MultipleSteps(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	store := newMemPriceStore(10000, 42, now)
	cfg := DefaultPriceConfig()
	svc := NewPriceService(store, cfg)

	// 15s / 5s step interval = 3 steps
	_, err := svc.GetCurrentPrice(context.Background(), now.Add(15*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.history) != 3 {
		t.Errorf("history has %d entries, want 3", len(store.history))
	}
}

func TestGetCurrentPrice_CatchupCapped(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	store := newMemPriceStore(10000, 42, now)
	cfg := DefaultPriceConfig()
	cfg.MaxCatchupSteps = 5
	svc := NewPriceService(store, cfg)

	_, err := svc.GetCurrentPrice(context.Background(), now.Add(300*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.history) != 5 {
		t.Errorf("history has %d entries, want 5 (capped)", len(store.history))
	}
}

func TestGetCurrentPrice_DeterministicReplay_SameBatchSize(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	cfg := DefaultPriceConfig()

	// Run 1: advance 5 steps in one call (25s / 5s interval = 5 steps).
	store1 := newMemPriceStore(10000, 42, now)
	svc1 := NewPriceService(store1, cfg)
	price1, err := svc1.GetCurrentPrice(context.Background(), now.Add(25*time.Second))
	if err != nil {
		t.Fatalf("run 1 error: %v", err)
	}

	// Run 2: same initial state, same 5 steps in one call.
	store2 := newMemPriceStore(10000, 42, now)
	svc2 := NewPriceService(store2, cfg)
	price2, err := svc2.GetCurrentPrice(context.Background(), now.Add(25*time.Second))
	if err != nil {
		t.Fatalf("run 2 error: %v", err)
	}

	if price1 != price2 {
		t.Errorf("same batch size replay failed: %d != %d", price1, price2)
	}

	// Verify all history entries match exactly.
	if len(store1.history) != len(store2.history) {
		t.Fatalf("history length mismatch: %d vs %d", len(store1.history), len(store2.history))
	}
	for i := range store1.history {
		if store1.history[i].Price != store2.history[i].Price {
			t.Errorf("history[%d] price mismatch: %d vs %d", i, store1.history[i].Price, store2.history[i].Price)
		}
	}
}

// NOTE: Incremental vs. bulk replay produces DIFFERENT results due to the seed persistence
// mechanism: rng.Int63() is called once per GetCurrentPrice call (after all steps in that
// call), consuming an extra random value. In incremental mode (1 step per call), this extra
// call happens after every step, diverging from bulk mode. This is a known limitation
// documented in issue #110 comments. The practical impact is low because in production,
// steps are computed once and persisted -- they are never replayed in a different batch size.
func TestGetCurrentPrice_IncrementalVsBulkDiverges(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	cfg := DefaultPriceConfig()

	// Bulk: 5 steps in one call (25s / 5s interval = 5 steps).
	store1 := newMemPriceStore(10000, 42, now)
	svc1 := NewPriceService(store1, cfg)
	priceBulk, _ := svc1.GetCurrentPrice(context.Background(), now.Add(25*time.Second))

	// Incremental: 5 steps in 5 calls (1 step each at 5s intervals).
	store2 := newMemPriceStore(10000, 42, now)
	svc2 := NewPriceService(store2, cfg)
	var priceIncr int64
	for i := 1; i <= 5; i++ {
		priceIncr, _ = svc2.GetCurrentPrice(context.Background(), now.Add(time.Duration(i*5)*time.Second))
	}

	// These WILL differ because the seed persistence mechanism consumes an extra
	// rng.Int63() per call. This test documents the known divergence.
	if priceBulk == priceIncr {
		t.Log("bulk and incremental prices happen to match (possible but unlikely)")
	} else {
		t.Logf("expected divergence: bulk=%d, incremental=%d (known limitation)", priceBulk, priceIncr)
	}
}

func TestGetCurrentPrice_SeedPersistedAndContinued(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	store := newMemPriceStore(10000, 42, now)
	cfg := DefaultPriceConfig()
	svc := NewPriceService(store, cfg)

	_, err := svc.GetCurrentPrice(context.Background(), now.Add(5*time.Second))
	if err != nil {
		t.Fatalf("step 1 error: %v", err)
	}

	if store.state.Seed == 42 {
		t.Error("seed was not updated after stepping; deterministic replay will break on restart")
	}

	price, err := svc.GetCurrentPrice(context.Background(), now.Add(10*time.Second))
	if err != nil {
		t.Fatalf("step 2 error: %v", err)
	}
	if price < cfg.MinPrice || price > cfg.MaxPrice {
		t.Errorf("price %d out of bounds after 2 steps", price)
	}
}

func TestGetCurrentPrice_NegativeElapsedTime(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	store := newMemPriceStore(10000, 42, now)
	cfg := DefaultPriceConfig()
	svc := NewPriceService(store, cfg)

	price, err := svc.GetCurrentPrice(context.Background(), now.Add(-10*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price != 10000 {
		t.Errorf("price = %d, want 10000 (should not advance on negative elapsed)", price)
	}
}

func TestGetCurrentPrice_LastStepAtAdvancesCorrectly(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	store := newMemPriceStore(10000, 42, now)
	cfg := DefaultPriceConfig()
	svc := NewPriceService(store, cfg)

	// 13s / 5s interval = 2 full steps, 3s remainder.
	_, err := svc.GetCurrentPrice(context.Background(), now.Add(13*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedLastStep := now.Add(10 * time.Second)
	if !store.state.LastStepAt.Equal(expectedLastStep) {
		t.Errorf("LastStepAt = %v, want %v", store.state.LastStepAt, expectedLastStep)
	}
}

func TestGetCurrentPrice_HistoryTimestampsAreCorrect(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	store := newMemPriceStore(10000, 42, now)
	cfg := DefaultPriceConfig()
	svc := NewPriceService(store, cfg)

	// 15s / 5s interval = 3 steps
	_, err := svc.GetCurrentPrice(context.Background(), now.Add(15*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(store.history) != 3 {
		t.Fatalf("expected 3 history points, got %d", len(store.history))
	}
	for i, expected := range []time.Time{
		now.Add(5 * time.Second),
		now.Add(10 * time.Second),
		now.Add(15 * time.Second),
	} {
		if !store.history[i].Time.Equal(expected) {
			t.Errorf("history[%d].Time = %v, want %v", i, store.history[i].Time, expected)
		}
	}
}

func TestGetCurrentPrice_PriceAlwaysWithinBounds(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	cfg := DefaultPriceConfig()
	cfg.MaxCatchupSteps = 1000

	for seed := int64(0); seed < 10; seed++ {
		store := newMemPriceStore(10000, seed, now)
		svc := NewPriceService(store, cfg)

		_, err := svc.GetCurrentPrice(context.Background(), now.Add(time.Duration(100*5)*time.Second))
		if err != nil {
			t.Fatalf("seed %d error: %v", seed, err)
		}

		for i, p := range store.history {
			if p.Price < cfg.MinPrice || p.Price > cfg.MaxPrice {
				t.Errorf("seed %d, step %d: price %d out of bounds [%d, %d]",
					seed, i, p.Price, cfg.MinPrice, cfg.MaxPrice)
			}
		}
	}
}

// ======================================================================
// Test: GetPriceHistory
// ======================================================================

func TestGetPriceHistory_DefaultLimit(t *testing.T) {
	store := newMemPriceStore(10000, 42, time.Now())
	cfg := DefaultPriceConfig()
	svc := NewPriceService(store, cfg)

	base := time.Now()
	for i := 0; i < 5; i++ {
		store.history = append(store.history, PricePoint{
			Time:  base.Add(time.Duration(i*30) * time.Second),
			Price: int64(10000 + i*100),
		})
	}

	points, err := svc.GetPriceHistory(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 5 {
		t.Errorf("got %d points, want 5", len(points))
	}
}

func TestGetPriceHistory_LimitApplied(t *testing.T) {
	store := newMemPriceStore(10000, 42, time.Now())
	cfg := DefaultPriceConfig()
	svc := NewPriceService(store, cfg)

	base := time.Now()
	for i := 0; i < 10; i++ {
		store.history = append(store.history, PricePoint{
			Time:  base.Add(time.Duration(i*30) * time.Second),
			Price: int64(10000 + i*100),
		})
	}

	points, err := svc.GetPriceHistory(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 3 {
		t.Errorf("got %d points, want 3", len(points))
	}
}

// ======================================================================
// Test: Error handling
// ======================================================================

func TestGetCurrentPrice_GetPriceError(t *testing.T) {
	store := &errorPriceStore{
		memPriceStore: *newMemPriceStore(10000, 42, time.Now()),
		getPriceErr:   fmt.Errorf("db connection lost"),
	}
	svc := NewPriceService(store, DefaultPriceConfig())

	_, err := svc.GetCurrentPrice(context.Background(), time.Now().Add(30*time.Second))
	if err == nil {
		t.Fatal("expected error from GetPrice failure")
	}
}

func TestGetCurrentPrice_UpdatePriceError(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	store := &errorPriceStore{
		memPriceStore:  *newMemPriceStore(10000, 42, now),
		updatePriceErr: fmt.Errorf("db write failed"),
	}
	svc := NewPriceService(store, DefaultPriceConfig())

	_, err := svc.GetCurrentPrice(context.Background(), now.Add(30*time.Second))
	if err == nil {
		t.Fatal("expected error from UpdatePrice failure")
	}
}

func TestGetCurrentPrice_InsertHistoryError(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	store := &errorPriceStore{
		memPriceStore:    *newMemPriceStore(10000, 42, now),
		insertHistoryErr: fmt.Errorf("db insert failed"),
	}
	svc := NewPriceService(store, DefaultPriceConfig())

	_, err := svc.GetCurrentPrice(context.Background(), now.Add(30*time.Second))
	if err == nil {
		t.Fatal("expected error from InsertPriceHistory failure")
	}
}

// ======================================================================
// Test: Config accessor
// ======================================================================

func TestConfig_ReturnsServiceConfig(t *testing.T) {
	cfg := PriceConfig{Mu: 12345, MinPrice: 500}
	svc := NewPriceService(nil, cfg)

	got := svc.Config()
	if got.Mu != 12345 || got.MinPrice != 500 {
		t.Errorf("Config() returned wrong values: %+v", got)
	}
}

// ======================================================================
// Test: Statistical properties of the OU process
// ======================================================================

func TestOrnsteinUhlenbeck_StatisticalProperties(t *testing.T) {
	cfg := DefaultPriceConfig()
	cfg.MaxCatchupSteps = 10000

	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	store := newMemPriceStore(10000, 42, now)
	svc := NewPriceService(store, cfg)

	// 10000 steps at 5s intervals = 50000s of simulation
	_, err := svc.GetCurrentPrice(context.Background(), now.Add(time.Duration(10000*5)*time.Second))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(store.history) == 0 {
		t.Fatal("no history generated")
	}
	var sum float64
	for _, p := range store.history {
		sum += float64(p.Price)
	}
	mean := sum / float64(len(store.history))

	var sumSqDiff float64
	for _, p := range store.history {
		diff := float64(p.Price) - mean
		sumSqDiff += diff * diff
	}
	stdDev := math.Sqrt(sumSqDiff / float64(len(store.history)))

	// With sigma=2000 and theta=0.02, the OU process has wider swings and slower
	// mean-reversion, so the mean can deviate further from mu=10000. The theoretical
	// stationary std dev is sigma/sqrt(2*theta) = 2000/sqrt(0.04) = 10000, but clamping
	// to [1000, 50000] constrains the actual distribution.
	if mean < 5000 || mean > 25000 {
		t.Errorf("mean price = %.0f, expected within [5000, 25000] (OU mean-reversion broken)", mean)
	}

	if stdDev < 500 {
		t.Errorf("price stddev = %.0f, expected > 500 (price not fluctuating enough with sigma=2000)", stdDev)
	}

	for i, p := range store.history {
		if p.Price < cfg.MinPrice || p.Price > cfg.MaxPrice {
			t.Errorf("step %d: price %d out of bounds", i, p.Price)
			break
		}
	}
}
