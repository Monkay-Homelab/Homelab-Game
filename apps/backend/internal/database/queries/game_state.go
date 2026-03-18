package queries

import (
	"context"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GameStateQueries struct {
	pool *pgxpool.Pool
}

func NewGameStateQueries(pool *pgxpool.Pool) *GameStateQueries {
	return &GameStateQueries{pool: pool}
}

func (q *GameStateQueries) Create(ctx context.Context, userID string) (*models.GameState, error) {
	var gs models.GameState
	err := q.pool.QueryRow(ctx,
		`INSERT INTO game_states (user_id)
		 VALUES ($1)
		 RETURNING id, user_id, tier, compute_units, reputation, power_watts, power_limit,
		           money, hardware_slots, used_slots, rack_units, used_rack_units,
		           colo_count, colo_multiplier, last_tick_at, created_at, updated_at`,
		userID,
	).Scan(&gs.ID, &gs.UserID, &gs.Tier, &gs.ComputeUnits, &gs.Reputation,
		&gs.PowerWatts, &gs.PowerLimit, &gs.Money, &gs.HardwareSlots, &gs.UsedSlots,
		&gs.RackUnits, &gs.UsedRackUnits, &gs.ColoCount, &gs.ColoMultiplier,
		&gs.LastTickAt, &gs.CreatedAt, &gs.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &gs, nil
}

func (q *GameStateQueries) GetByUserID(ctx context.Context, userID string) (*models.GameState, error) {
	var gs models.GameState
	err := q.pool.QueryRow(ctx,
		`SELECT id, user_id, tier, compute_units, reputation, power_watts, power_limit,
		        money, hardware_slots, used_slots, rack_units, used_rack_units,
		        colo_count, colo_multiplier, last_tick_at, created_at, updated_at
		 FROM game_states WHERE user_id = $1`,
		userID,
	).Scan(&gs.ID, &gs.UserID, &gs.Tier, &gs.ComputeUnits, &gs.Reputation,
		&gs.PowerWatts, &gs.PowerLimit, &gs.Money, &gs.HardwareSlots, &gs.UsedSlots,
		&gs.RackUnits, &gs.UsedRackUnits, &gs.ColoCount, &gs.ColoMultiplier,
		&gs.LastTickAt, &gs.CreatedAt, &gs.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &gs, nil
}

func (q *GameStateQueries) Update(ctx context.Context, gs *models.GameState) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE game_states SET
		    tier = $2, compute_units = $3, reputation = $4, power_watts = $5,
		    power_limit = $6, money = $7, hardware_slots = $8, used_slots = $9,
		    rack_units = $10, used_rack_units = $11, colo_count = $12,
		    colo_multiplier = $13, last_tick_at = $14, updated_at = NOW()
		 WHERE id = $1`,
		gs.ID, gs.Tier, gs.ComputeUnits, gs.Reputation, gs.PowerWatts,
		gs.PowerLimit, gs.Money, gs.HardwareSlots, gs.UsedSlots,
		gs.RackUnits, gs.UsedRackUnits, gs.ColoCount, gs.ColoMultiplier,
		gs.LastTickAt)
	return err
}
