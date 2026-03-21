package queries

import (
	"context"
	"time"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BitcoinQueries struct {
	pool *pgxpool.Pool
}

func NewBitcoinQueries(pool *pgxpool.Pool) *BitcoinQueries {
	return &BitcoinQueries{pool: pool}
}

func (q *BitcoinQueries) GetPrice(ctx context.Context) (*models.BitcoinPrice, error) {
	var bp models.BitcoinPrice
	err := q.pool.QueryRow(ctx,
		`SELECT current_price, seed, last_step_at, updated_at FROM bitcoin_price WHERE id = 1`,
	).Scan(&bp.CurrentPrice, &bp.Seed, &bp.LastStepAt, &bp.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &bp, nil
}

func (q *BitcoinQueries) UpdatePrice(ctx context.Context, price int64, seed int64, lastStepAt time.Time) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE bitcoin_price SET current_price = $1, seed = $2, last_step_at = $3, updated_at = NOW() WHERE id = 1`,
		price, seed, lastStepAt,
	)
	return err
}

func (q *BitcoinQueries) InsertPriceHistory(ctx context.Context, t time.Time, price int64) error {
	_, err := q.pool.Exec(ctx,
		`INSERT INTO bitcoin_price_history (time, price) VALUES ($1, $2)`,
		t, price,
	)
	return err
}

func (q *BitcoinQueries) GetPriceHistory(ctx context.Context, limit int) ([]models.BitcoinPricePoint, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT * FROM (SELECT time, price FROM bitcoin_price_history ORDER BY time DESC LIMIT $1) sub ORDER BY time ASC`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []models.BitcoinPricePoint
	for rows.Next() {
		var pp models.BitcoinPricePoint
		if err := rows.Scan(&pp.Time, &pp.Price); err != nil {
			return nil, err
		}
		history = append(history, pp)
	}
	return history, nil
}
