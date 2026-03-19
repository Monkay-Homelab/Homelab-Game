package queries

import (
	"context"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ServiceQueries struct {
	pool *pgxpool.Pool
}

func NewServiceQueries(pool *pgxpool.Pool) *ServiceQueries {
	return &ServiceQueries{pool: pool}
}

func (q *ServiceQueries) Create(ctx context.Context, s *models.Service) error {
	return q.pool.QueryRow(ctx,
		`INSERT INTO services (game_state_id, name, type, tier, compute_per_tick, reputation_per_tick, money_per_tick)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, deployed_at`,
		s.GameStateID, s.Name, s.Type, s.Tier, s.ComputePerTick, s.ReputationPerTick, s.MoneyPerTick,
	).Scan(&s.ID, &s.DeployedAt)
}

func (q *ServiceQueries) GetByGameStateID(ctx context.Context, gameStateID string) ([]models.Service, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, game_state_id, name, type, tier, compute_per_tick, reputation_per_tick, money_per_tick, deployed_at
		 FROM services WHERE game_state_id = $1`,
		gameStateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []models.Service
	for rows.Next() {
		var s models.Service
		if err := rows.Scan(&s.ID, &s.GameStateID, &s.Name, &s.Type, &s.Tier,
			&s.ComputePerTick, &s.ReputationPerTick, &s.MoneyPerTick, &s.DeployedAt); err != nil {
			return nil, err
		}
		services = append(services, s)
	}
	return services, nil
}

func (q *ServiceQueries) DeleteByGameStateID(ctx context.Context, gameStateID string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM services WHERE game_state_id = $1`, gameStateID)
	return err
}
