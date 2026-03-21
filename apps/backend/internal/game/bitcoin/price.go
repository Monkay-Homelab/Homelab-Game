package bitcoin

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

// PriceState represents the current global bitcoin price state (singleton row in DB).
type PriceState struct {
	CurrentPrice int64     `json:"current_price"`
	Seed         int64     `json:"seed"`
	LastStepAt   time.Time `json:"last_step_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PricePoint represents a single historical price data point for charting.
type PricePoint struct {
	Time  time.Time `json:"time"`
	Price int64     `json:"price"`
}

// PriceStore abstracts the database operations needed by PriceService.
// This interface allows the service to be built and tested without a real database.
type PriceStore interface {
	// GetPrice returns the current singleton bitcoin price state.
	GetPrice(ctx context.Context) (*PriceState, error)

	// UpdatePrice persists the updated price state (current price, seed, timestamps).
	UpdatePrice(ctx context.Context, state *PriceState) error

	// InsertPriceHistory records a single price point in the history table.
	InsertPriceHistory(ctx context.Context, point PricePoint) error

	// GetPriceHistory returns the most recent price points, ordered by time descending.
	GetPriceHistory(ctx context.Context, limit int) ([]PricePoint, error)
}

// PriceConfig holds tunable parameters for the Ornstein-Uhlenbeck price model.
type PriceConfig struct {
	Mu               int64   // Long-term mean price (default 10000)
	Theta            float64 // Mean-reversion speed (default 0.05)
	Sigma            float64 // Volatility (default 500)
	StepInterval     int     // Seconds between price steps (default 30)
	MinPrice         int64   // Hard floor (default 1000)
	MaxPrice         int64   // Hard ceiling (default 50000)
	MaxCatchupSteps  int     // Max steps to compute on catch-up (default 1000)
	HistoryRetention int     // Number of price points to keep for charting (default 200)
}

// DefaultPriceConfig returns a PriceConfig with the default values from the TDD.
func DefaultPriceConfig() PriceConfig {
	return PriceConfig{
		Mu:               10000,
		Theta:            0.02,
		Sigma:            2000,
		StepInterval:     5,
		MinPrice:         1000,
		MaxPrice:         50000,
		MaxCatchupSteps:  1000,
		HistoryRetention: 200,
	}
}

// PriceService encapsulates the Ornstein-Uhlenbeck price model with lazy evaluation.
// It is safe for concurrent use — a global mutex serializes price advancement so
// concurrent callers do not produce duplicate steps.
type PriceService struct {
	store  PriceStore
	mu     sync.Mutex
	config PriceConfig
}

// NewPriceService creates a PriceService with the given store and config.
func NewPriceService(store PriceStore, config PriceConfig) *PriceService {
	return &PriceService{
		store:  store,
		config: config,
	}
}

// GetCurrentPrice returns the current bitcoin price, lazily advancing the model
// forward by however many steps have elapsed since the last update.
//
// The price is advanced deterministically using a seeded PRNG read from the database,
// ensuring that the same sequence of steps always produces the same price trajectory
// (important for replay and testing).
//
// Concurrent calls are serialized: the first caller advances the price and writes it
// back; subsequent callers see the already-advanced price.
func (s *PriceService) GetCurrentPrice(ctx context.Context, now time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.store.GetPrice(ctx)
	if err != nil {
		return 0, fmt.Errorf("get bitcoin price state: %w", err)
	}

	// Compute how many steps have elapsed since the last price step.
	elapsedSeconds := now.Sub(state.LastStepAt).Seconds()
	if elapsedSeconds < 0 {
		// Clock skew or stale timestamp — no steps to advance.
		return state.CurrentPrice, nil
	}

	steps := int(math.Floor(elapsedSeconds / float64(s.config.StepInterval)))
	if steps <= 0 {
		return state.CurrentPrice, nil
	}

	// Cap steps to prevent expensive catch-up after long offline periods.
	if steps > s.config.MaxCatchupSteps {
		steps = s.config.MaxCatchupSteps
	}

	// Create a deterministic PRNG from the stored seed.
	rng := rand.New(rand.NewSource(state.Seed))

	price := state.CurrentPrice
	stepTime := state.LastStepAt

	for i := 0; i < steps; i++ {
		price = s.step(price, rng)
		stepTime = stepTime.Add(time.Duration(s.config.StepInterval) * time.Second)

		// Record each step in price history.
		if err := s.store.InsertPriceHistory(ctx, PricePoint{
			Time:  stepTime,
			Price: price,
		}); err != nil {
			return 0, fmt.Errorf("insert bitcoin price history (step %d): %w", i+1, err)
		}
	}

	// Persist the updated price state with the new seed.
	// The seed is derived from the PRNG's current state by reading one more value,
	// so the next call continues the deterministic sequence.
	state.CurrentPrice = price
	state.Seed = rng.Int63()
	state.LastStepAt = stepTime
	state.UpdatedAt = now

	if err := s.store.UpdatePrice(ctx, state); err != nil {
		return 0, fmt.Errorf("update bitcoin price state: %w", err)
	}

	return price, nil
}

// GetPriceHistory returns the last N price points for charting, ordered by time
// descending. If limit is <= 0, defaults to 100.
func (s *PriceService) GetPriceHistory(ctx context.Context, limit int) ([]PricePoint, error) {
	if limit <= 0 {
		limit = 100
	}
	points, err := s.store.GetPriceHistory(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("get bitcoin price history: %w", err)
	}
	return points, nil
}

// step applies a single Ornstein-Uhlenbeck step to the price:
//
//	price_next = price_current + theta * (mu - price_current) * dt + sigma * sqrt(dt) * N(0,1)
//
// where dt = 1.0 (normalized single step) and N(0,1) is drawn from the seeded PRNG.
// The result is clamped to [MinPrice, MaxPrice].
func (s *PriceService) step(price int64, rng *rand.Rand) int64 {
	cfg := s.config
	dt := 1.0

	// Standard normal via Box-Muller (NormFloat64 from the seeded source).
	z := rng.NormFloat64()

	// Ornstein-Uhlenbeck discrete step.
	pFloat := float64(price)
	drift := cfg.Theta * (float64(cfg.Mu) - pFloat) * dt
	diffusion := cfg.Sigma * math.Sqrt(dt) * z

	next := pFloat + drift + diffusion

	// Round to nearest integer and clamp to bounds.
	nextInt := int64(math.Round(next))
	if nextInt < cfg.MinPrice {
		nextInt = cfg.MinPrice
	}
	if nextInt > cfg.MaxPrice {
		nextInt = cfg.MaxPrice
	}

	return nextInt
}

// Config returns the service's price configuration (useful for exposing
// config values via the /api/game/config endpoint).
func (s *PriceService) Config() PriceConfig {
	return s.config
}
