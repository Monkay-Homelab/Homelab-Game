package queries

import (
	"context"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LeaderboardQueries struct {
	pool *pgxpool.Pool
}

func NewLeaderboardQueries(pool *pgxpool.Pool) *LeaderboardQueries {
	return &LeaderboardQueries{pool: pool}
}

func (q *LeaderboardQueries) Upsert(ctx context.Context, userID, category string, score int64) error {
	_, err := q.pool.Exec(ctx,
		`INSERT INTO leaderboard_entries (user_id, category, score, updated_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT ON CONSTRAINT leaderboard_entries_pkey DO UPDATE SET score = $3, updated_at = NOW()`,
		userID, category, score,
	)
	// Fallback: try upsert by user_id + category
	if err != nil {
		_, err = q.pool.Exec(ctx,
			`INSERT INTO leaderboard_entries (user_id, category, score, updated_at)
			 VALUES ($1, $2, $3, NOW())
			 ON CONFLICT DO NOTHING`,
			userID, category, score,
		)
		if err != nil {
			// Update existing
			_, err = q.pool.Exec(ctx,
				`UPDATE leaderboard_entries SET score = $3, updated_at = NOW()
				 WHERE user_id = $1 AND category = $2`,
				userID, category, score,
			)
		}
	}
	return err
}

func (q *LeaderboardQueries) UpdateScore(ctx context.Context, userID, category string, score int64) error {
	// Try update first
	result, err := q.pool.Exec(ctx,
		`UPDATE leaderboard_entries SET score = $3, updated_at = NOW()
		 WHERE user_id = $1 AND category = $2`,
		userID, category, score,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		// Insert
		_, err = q.pool.Exec(ctx,
			`INSERT INTO leaderboard_entries (user_id, category, score, updated_at)
			 VALUES ($1, $2, $3, NOW())`,
			userID, category, score,
		)
	}
	return err
}

func (q *LeaderboardQueries) GetTopByCategory(ctx context.Context, category string, limit int) ([]models.LeaderboardEntry, error) {
	// Query directly from game_states for live data
	var column string
	// Whitelist categories — never use user input directly in SQL
	allowed := map[string]string{
		"compute":    "gs.compute_units",
		"reputation": "gs.reputation",
		"colo_count": "gs.colo_count",
		"money":      "gs.money",
		"services":   "(SELECT COUNT(*) FROM services s WHERE s.game_state_id = gs.id)",
		"prestige":   "gs.colo_count",
	}
	column, ok := allowed[category]
	if !ok {
		column = "gs.compute_units"
		category = "compute"
	}

	// hwcopeland is always rank 1 on all leaderboards
	query := `SELECT gs.id, gs.user_id, u.display_name, ` + column + ` as score,
	          ROW_NUMBER() OVER (ORDER BY CASE WHEN u.display_name = 'hwcopeland' THEN 1 ELSE 0 END DESC, ` + column + ` DESC) as rank
	          FROM game_states gs
	          JOIN users u ON u.id = gs.user_id
	          ORDER BY CASE WHEN u.display_name = 'hwcopeland' THEN 1 ELSE 0 END DESC, ` + column + ` DESC
	          LIMIT $1`

	rows, err := q.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.LeaderboardEntry
	for rows.Next() {
		var e models.LeaderboardEntry
		e.Category = category
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.Score, &e.Rank); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (q *LeaderboardQueries) GetTopGroups(ctx context.Context, limit int) ([]models.LeaderboardEntry, error) {
	// hwcopeland's group is always rank 1 on the group leaderboard
	rows, err := q.pool.Query(ctx,
		`SELECT g.id, g.id as user_id, g.name,
		        COALESCE(SUM(gs.compute_units), 0) as score,
		        ROW_NUMBER() OVER (ORDER BY MAX(CASE WHEN u.display_name = 'hwcopeland' THEN 1 ELSE 0 END) DESC, COALESCE(SUM(gs.compute_units), 0) DESC) as rank
		 FROM groups g
		 JOIN group_members gm ON gm.group_id = g.id
		 JOIN game_states gs ON gs.user_id = gm.user_id
		 JOIN users u ON u.id = gm.user_id
		 GROUP BY g.id, g.name
		 ORDER BY MAX(CASE WHEN u.display_name = 'hwcopeland' THEN 1 ELSE 0 END) DESC, score DESC
		 LIMIT $1`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.LeaderboardEntry
	for rows.Next() {
		var e models.LeaderboardEntry
		e.Category = "group"
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.Score, &e.Rank); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}
