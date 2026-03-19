package queries

import (
	"context"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ColoRackQueries struct {
	pool *pgxpool.Pool
}

func NewColoRackQueries(pool *pgxpool.Pool) *ColoRackQueries {
	return &ColoRackQueries{pool: pool}
}

func (q *ColoRackQueries) Create(ctx context.Context, cr *models.ColoRack) error {
	return q.pool.QueryRow(ctx,
		`INSERT INTO colo_racks (user_id, datacenter_tier, rack_size, compute_per_tick, reputation_per_tick, money_per_tick)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, colo_at`,
		cr.UserID, cr.DatacenterTier, cr.RackSize, cr.ComputePerTick, cr.ReputationPerTick, cr.MoneyPerTick,
	).Scan(&cr.ID, &cr.ColoAt)
}

func (q *ColoRackQueries) GetByUserID(ctx context.Context, userID string) ([]models.ColoRack, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, user_id, datacenter_tier, rack_size, compute_per_tick, reputation_per_tick, money_per_tick, colo_at
		 FROM colo_racks WHERE user_id = $1 ORDER BY colo_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var racks []models.ColoRack
	for rows.Next() {
		var cr models.ColoRack
		if err := rows.Scan(&cr.ID, &cr.UserID, &cr.DatacenterTier, &cr.RackSize,
			&cr.ComputePerTick, &cr.ReputationPerTick, &cr.MoneyPerTick, &cr.ColoAt); err != nil {
			return nil, err
		}
		racks = append(racks, cr)
	}
	return racks, nil
}
