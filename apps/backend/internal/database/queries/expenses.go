package queries

import (
	"context"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ExpenseQueries struct {
	pool *pgxpool.Pool
}

func NewExpenseQueries(pool *pgxpool.Pool) *ExpenseQueries {
	return &ExpenseQueries{pool: pool}
}

func (q *ExpenseQueries) Create(ctx context.Context, e *models.Expense) error {
	return q.pool.QueryRow(ctx,
		`INSERT INTO expenses (game_state_id, name, type, cost_per_tick)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		e.GameStateID, e.Name, e.Type, e.CostPerTick,
	).Scan(&e.ID, &e.CreatedAt)
}

func (q *ExpenseQueries) GetByGameStateID(ctx context.Context, gameStateID string) ([]models.Expense, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, game_state_id, name, type, cost_per_tick, created_at
		 FROM expenses WHERE game_state_id = $1`,
		gameStateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var expenses []models.Expense
	for rows.Next() {
		var e models.Expense
		if err := rows.Scan(&e.ID, &e.GameStateID, &e.Name, &e.Type,
			&e.CostPerTick, &e.CreatedAt); err != nil {
			return nil, err
		}
		expenses = append(expenses, e)
	}
	return expenses, nil
}

func (q *ExpenseQueries) DeleteByGameStateID(ctx context.Context, gameStateID string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM expenses WHERE game_state_id = $1`, gameStateID)
	return err
}
