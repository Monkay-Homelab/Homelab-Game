package queries

import (
	"context"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserQueries struct {
	pool *pgxpool.Pool
}

func NewUserQueries(pool *pgxpool.Pool) *UserQueries {
	return &UserQueries{pool: pool}
}

func (q *UserQueries) Create(ctx context.Context, email, passwordHash, displayName string) (*models.User, error) {
	var user models.User
	err := q.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, display_name)
		 VALUES ($1, $2, $3)
		 RETURNING id, email, display_name, created_at, updated_at`,
		email, passwordHash, displayName,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (q *UserQueries) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := q.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, display_name, oauth_provider, oauth_id, created_at, updated_at
		 FROM users WHERE email = $1`,
		email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.DisplayName,
		&user.OAuthProvider, &user.OAuthID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (q *UserQueries) GetByID(ctx context.Context, id string) (*models.User, error) {
	var user models.User
	err := q.pool.QueryRow(ctx,
		`SELECT id, email, display_name, oauth_provider, created_at, updated_at
		 FROM users WHERE id = $1`,
		id,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.OAuthProvider, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
