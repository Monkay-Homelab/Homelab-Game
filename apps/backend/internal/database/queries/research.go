package queries

import (
	"context"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ResearchLevelQueries struct {
	pool *pgxpool.Pool
}

func NewResearchLevelQueries(pool *pgxpool.Pool) *ResearchLevelQueries {
	return &ResearchLevelQueries{pool: pool}
}

func (q *ResearchLevelQueries) Upsert(ctx context.Context, rl *models.ResearchLevel) error {
	return q.pool.QueryRow(ctx,
		`INSERT INTO research_levels (game_state_id, research_node, level)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (game_state_id, research_node) DO UPDATE SET
		     level = $3, updated_at = NOW()
		 RETURNING id, updated_at`,
		rl.GameStateID, rl.ResearchNode, rl.Level,
	).Scan(&rl.ID, &rl.UpdatedAt)
}

func (q *ResearchLevelQueries) GetByGameStateID(ctx context.Context, gameStateID string) ([]models.ResearchLevel, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, game_state_id, research_node, level, updated_at
		 FROM research_levels WHERE game_state_id = $1`,
		gameStateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var levels []models.ResearchLevel
	for rows.Next() {
		var rl models.ResearchLevel
		if err := rows.Scan(&rl.ID, &rl.GameStateID, &rl.ResearchNode, &rl.Level,
			&rl.UpdatedAt); err != nil {
			return nil, err
		}
		levels = append(levels, rl)
	}
	return levels, nil
}
