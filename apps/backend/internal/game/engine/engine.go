package engine

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/homelab-game/backend/internal/models"
)

type Engine struct{}

func New() *Engine {
	return &Engine{}
}

// ProcessIdleProgress calculates resources earned since the last tick.
func (e *Engine) ProcessIdleProgress(gs *models.GameState, now time.Time) {
	elapsed := now.Sub(gs.LastTickAt)
	if elapsed <= 0 {
		return
	}

	seconds := elapsed.Seconds()

	// Base idle compute per second scales with tier
	baseCompute := tierComputeRate(gs.Tier)
	gs.ComputeUnits += int64(float64(baseCompute) * seconds * gs.ColoMultiplier)

	// Reputation ticks passively from uptime
	baseRep := tierReputationRate(gs.Tier)
	gs.Reputation += int64(float64(baseRep) * seconds)

	gs.LastTickAt = now
}

// ProcessAction validates and applies a player action.
func (e *Engine) ProcessAction(gs *models.GameState, actionType string, payload json.RawMessage) error {
	switch actionType {
	case "run_job":
		return e.runJob(gs)
	default:
		return fmt.Errorf("unknown action: %s", actionType)
	}
}

func (e *Engine) runJob(gs *models.GameState) error {
	reward := tierJobReward(gs.Tier)
	gs.ComputeUnits += int64(float64(reward) * gs.ColoMultiplier)
	return nil
}

// Tier-based rates (per second for idle, per click for jobs)
func tierComputeRate(tier models.Tier) int64 {
	switch tier {
	case models.TierCoffeeTable:
		return 1
	case models.TierClosetFloor:
		return 5
	case models.TierRack12U:
		return 20
	case models.TierRack24U:
		return 80
	case models.TierRack36U:
		return 300
	case models.TierRack48U:
		return 1000
	default:
		return 1
	}
}

func tierReputationRate(tier models.Tier) int64 {
	switch tier {
	case models.TierCoffeeTable:
		return 0
	case models.TierClosetFloor:
		return 1
	case models.TierRack12U:
		return 3
	case models.TierRack24U:
		return 10
	case models.TierRack36U:
		return 30
	case models.TierRack48U:
		return 100
	default:
		return 0
	}
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
