package queries

import (
	"context"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HardwareQueries struct {
	pool *pgxpool.Pool
}

func NewHardwareQueries(pool *pgxpool.Pool) *HardwareQueries {
	return &HardwareQueries{pool: pool}
}

func (q *HardwareQueries) Create(ctx context.Context, h *models.Hardware) error {
	return q.pool.QueryRow(ctx,
		`INSERT INTO hardware (game_state_id, name, type, tier, slots_used, rack_units_used, power_draw, compute_per_tick)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, purchased_at`,
		h.GameStateID, h.Name, h.Type, h.Tier, h.SlotsUsed, h.RackUnitsUsed, h.PowerDraw, h.ComputePerTick,
	).Scan(&h.ID, &h.PurchasedAt)
}

func (q *HardwareQueries) GetByGameStateID(ctx context.Context, gameStateID string) ([]models.Hardware, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, game_state_id, name, type, tier, slots_used, rack_units_used, power_draw, compute_per_tick, purchased_at
		 FROM hardware WHERE game_state_id = $1`,
		gameStateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hardware []models.Hardware
	for rows.Next() {
		var h models.Hardware
		if err := rows.Scan(&h.ID, &h.GameStateID, &h.Name, &h.Type, &h.Tier,
			&h.SlotsUsed, &h.RackUnitsUsed, &h.PowerDraw, &h.ComputePerTick, &h.PurchasedAt); err != nil {
			return nil, err
		}
		hardware = append(hardware, h)
	}
	return hardware, nil
}

func (q *HardwareQueries) DeleteByID(ctx context.Context, id string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM hardware WHERE id = $1`, id)
	return err
}

func (q *HardwareQueries) DeleteByGameStateID(ctx context.Context, gameStateID string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM hardware WHERE game_state_id = $1`, gameStateID)
	return err
}
