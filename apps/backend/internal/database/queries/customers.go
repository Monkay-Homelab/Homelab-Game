package queries

import (
	"context"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CustomerQueries struct {
	pool *pgxpool.Pool
}

func NewCustomerQueries(pool *pgxpool.Pool) *CustomerQueries {
	return &CustomerQueries{pool: pool}
}

func (q *CustomerQueries) Create(ctx context.Context, c *models.Customer) error {
	return q.pool.QueryRow(ctx,
		`INSERT INTO customers (game_state_id, name, service_type, monthly_revenue, satisfaction)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, signed_up_at`,
		c.GameStateID, c.Name, c.ServiceType, c.MonthlyRevenue, c.Satisfaction,
	).Scan(&c.ID, &c.SignedUpAt)
}

func (q *CustomerQueries) GetByGameStateID(ctx context.Context, gameStateID string) ([]models.Customer, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, game_state_id, name, service_type, monthly_revenue, satisfaction, signed_up_at
		 FROM customers WHERE game_state_id = $1`,
		gameStateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var customers []models.Customer
	for rows.Next() {
		var c models.Customer
		if err := rows.Scan(&c.ID, &c.GameStateID, &c.Name, &c.ServiceType,
			&c.MonthlyRevenue, &c.Satisfaction, &c.SignedUpAt); err != nil {
			return nil, err
		}
		customers = append(customers, c)
	}
	return customers, nil
}

func (q *CustomerQueries) Update(ctx context.Context, c *models.Customer) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE customers SET satisfaction = $2 WHERE id = $1`,
		c.ID, c.Satisfaction)
	return err
}

func (q *CustomerQueries) DeleteByGameStateID(ctx context.Context, gameStateID string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM customers WHERE game_state_id = $1`, gameStateID)
	return err
}

func (q *CustomerQueries) Delete(ctx context.Context, id string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM customers WHERE id = $1`, id)
	return err
}
