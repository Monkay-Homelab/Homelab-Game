package queries

import (
	"context"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GroupQueries struct {
	pool *pgxpool.Pool
}

func NewGroupQueries(pool *pgxpool.Pool) *GroupQueries {
	return &GroupQueries{pool: pool}
}

func (q *GroupQueries) Create(ctx context.Context, g *models.Group) error {
	return q.pool.QueryRow(ctx,
		`INSERT INTO groups (name, founder_id, min_contribution, profit_split)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		g.Name, g.FounderID, g.MinContribution, g.ProfitSplit,
	).Scan(&g.ID, &g.CreatedAt)
}

func (q *GroupQueries) GetByID(ctx context.Context, id string) (*models.Group, error) {
	var g models.Group
	err := q.pool.QueryRow(ctx,
		`SELECT id, name, founder_id, min_contribution, profit_split, created_at
		 FROM groups WHERE id = $1`, id,
	).Scan(&g.ID, &g.Name, &g.FounderID, &g.MinContribution, &g.ProfitSplit, &g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (q *GroupQueries) GetByName(ctx context.Context, name string) (*models.Group, error) {
	var g models.Group
	err := q.pool.QueryRow(ctx,
		`SELECT id, name, founder_id, min_contribution, profit_split, created_at
		 FROM groups WHERE name = $1`, name,
	).Scan(&g.ID, &g.Name, &g.FounderID, &g.MinContribution, &g.ProfitSplit, &g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (q *GroupQueries) List(ctx context.Context, limit int) ([]models.Group, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, name, founder_id, min_contribution, profit_split, created_at
		 FROM groups ORDER BY created_at DESC LIMIT $1`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []models.Group
	for rows.Next() {
		var g models.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.FounderID, &g.MinContribution, &g.ProfitSplit, &g.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (q *GroupQueries) AddMember(ctx context.Context, groupID, userID, role string) error {
	_, err := q.pool.Exec(ctx,
		`INSERT INTO group_members (group_id, user_id, role)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (group_id, user_id) DO NOTHING`,
		groupID, userID, role,
	)
	return err
}

func (q *GroupQueries) RemoveMember(ctx context.Context, groupID, userID string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, groupID, userID)
	return err
}

func (q *GroupQueries) GetMembers(ctx context.Context, groupID string) ([]models.GroupMember, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT gm.group_id, gm.user_id, gm.role, gm.joined_at, u.display_name
		 FROM group_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.group_id = $1
		 ORDER BY gm.joined_at`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.GroupMember
	for rows.Next() {
		var m models.GroupMember
		if err := rows.Scan(&m.GroupID, &m.UserID, &m.Role, &m.JoinedAt, &m.DisplayName); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, nil
}

func (q *GroupQueries) GetUserGroup(ctx context.Context, userID string) (*models.Group, *models.GroupMember, error) {
	var g models.Group
	var m models.GroupMember
	err := q.pool.QueryRow(ctx,
		`SELECT g.id, g.name, g.founder_id, g.min_contribution, g.profit_split, g.created_at,
		        gm.group_id, gm.user_id, gm.role, gm.joined_at
		 FROM group_members gm
		 JOIN groups g ON g.id = gm.group_id
		 WHERE gm.user_id = $1`, userID,
	).Scan(&g.ID, &g.Name, &g.FounderID, &g.MinContribution, &g.ProfitSplit, &g.CreatedAt,
		&m.GroupID, &m.UserID, &m.Role, &m.JoinedAt)
	if err != nil {
		return nil, nil, err
	}
	return &g, &m, nil
}

func (q *GroupQueries) GetGroupComputePool(ctx context.Context, groupID string) (int64, error) {
	var total int64
	err := q.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(gs.compute_units), 0)
		 FROM group_members gm
		 JOIN game_states gs ON gs.user_id = gm.user_id
		 WHERE gm.group_id = $1`, groupID,
	).Scan(&total)
	return total, err
}

func (q *GroupQueries) SetRole(ctx context.Context, groupID, userID, role string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE group_members SET role = $3 WHERE group_id = $1 AND user_id = $2`,
		groupID, userID, role,
	)
	return err
}

func (q *GroupQueries) Delete(ctx context.Context, id string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM groups WHERE id = $1`, id)
	return err
}
