package queries

import (
	"context"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UpgradeQueries struct {
	pool *pgxpool.Pool
}

func NewUpgradeQueries(pool *pgxpool.Pool) *UpgradeQueries {
	return &UpgradeQueries{pool: pool}
}

func (q *UpgradeQueries) Create(ctx context.Context, u *models.Upgrade) error {
	return q.pool.QueryRow(ctx,
		`INSERT INTO upgrades (game_state_id, name, type, tier, persistent)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, purchased_at`,
		u.GameStateID, u.Name, u.Type, u.Tier, u.Persistent,
	).Scan(&u.ID, &u.PurchasedAt)
}

func (q *UpgradeQueries) GetByGameStateID(ctx context.Context, gameStateID string) ([]models.Upgrade, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, game_state_id, name, type, tier, persistent, purchased_at
		 FROM upgrades WHERE game_state_id = $1`,
		gameStateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var upgrades []models.Upgrade
	for rows.Next() {
		var u models.Upgrade
		if err := rows.Scan(&u.ID, &u.GameStateID, &u.Name, &u.Type, &u.Tier,
			&u.Persistent, &u.PurchasedAt); err != nil {
			return nil, err
		}
		upgrades = append(upgrades, u)
	}
	return upgrades, nil
}

func (q *UpgradeQueries) DeleteNonPersistent(ctx context.Context, gameStateID string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM upgrades WHERE game_state_id = $1 AND persistent = false`, gameStateID)
	return err
}

func (q *UpgradeQueries) HasUpgrade(ctx context.Context, gameStateID, name string) (bool, error) {
	var count int
	err := q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM upgrades WHERE game_state_id = $1 AND name = $2`,
		gameStateID, name,
	).Scan(&count)
	return count > 0, err
}

type ComponentUpgradeQueries struct {
	pool *pgxpool.Pool
}

func NewComponentUpgradeQueries(pool *pgxpool.Pool) *ComponentUpgradeQueries {
	return &ComponentUpgradeQueries{pool: pool}
}

func (q *ComponentUpgradeQueries) Upsert(ctx context.Context, cu *models.ComponentUpgrade) error {
	return q.pool.QueryRow(ctx,
		`INSERT INTO component_upgrades (hardware_id, component, level, compute_bonus, power_reduction)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (hardware_id, component) DO UPDATE SET
		     level = $3, compute_bonus = $4, power_reduction = $5, upgraded_at = NOW()
		 RETURNING id, upgraded_at`,
		cu.HardwareID, cu.Component, cu.Level, cu.ComputeBonus, cu.PowerReduction,
	).Scan(&cu.ID, &cu.UpgradedAt)
}

func (q *ComponentUpgradeQueries) GetByGameStateID(ctx context.Context, gameStateID string) ([]models.ComponentUpgrade, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT cu.id, cu.hardware_id, cu.component, cu.level, cu.compute_bonus, cu.power_reduction, cu.upgraded_at
		 FROM component_upgrades cu
		 JOIN hardware h ON h.id = cu.hardware_id
		 WHERE h.game_state_id = $1`,
		gameStateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var upgrades []models.ComponentUpgrade
	for rows.Next() {
		var cu models.ComponentUpgrade
		if err := rows.Scan(&cu.ID, &cu.HardwareID, &cu.Component, &cu.Level,
			&cu.ComputeBonus, &cu.PowerReduction, &cu.UpgradedAt); err != nil {
			return nil, err
		}
		upgrades = append(upgrades, cu)
	}
	return upgrades, nil
}

func (q *ComponentUpgradeQueries) GetByHardwareID(ctx context.Context, hardwareID string) ([]models.ComponentUpgrade, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, hardware_id, component, level, compute_bonus, power_reduction, upgraded_at
		 FROM component_upgrades WHERE hardware_id = $1`,
		hardwareID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var upgrades []models.ComponentUpgrade
	for rows.Next() {
		var cu models.ComponentUpgrade
		if err := rows.Scan(&cu.ID, &cu.HardwareID, &cu.Component, &cu.Level,
			&cu.ComputeBonus, &cu.PowerReduction, &cu.UpgradedAt); err != nil {
			return nil, err
		}
		upgrades = append(upgrades, cu)
	}
	return upgrades, nil
}
