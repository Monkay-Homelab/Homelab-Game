package queries

import (
	"context"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GameStateQueries struct {
	pool *pgxpool.Pool
}

func NewGameStateQueries(pool *pgxpool.Pool) *GameStateQueries {
	return &GameStateQueries{pool: pool}
}

const gsColumns = `id, user_id, tier, compute_units, reputation, power_watts, power_limit,
	money, hardware_slots, used_slots, rack_units, used_rack_units,
	colo_count, colo_multiplier, heat_generated, cooling_capacity,
	network_tier, automation_tier, knowledge_points, idle_multiplier,
	saas_unlocked, total_customers, throttle_multiplier, throttle_ticks_remaining,
	datacenter_tier, owns_datacenter, datacenter_level, datacenter_income_multiplier,
	total_donated_cu, bitcoin_balance, last_customer_growth_at, last_tick_at, created_at, updated_at`

func scanGS(dest ...any) []any { return dest }

func gsFields(gs *models.GameState) []any {
	return scanGS(&gs.ID, &gs.UserID, &gs.Tier, &gs.ComputeUnits, &gs.Reputation,
		&gs.PowerWatts, &gs.PowerLimit, &gs.Money, &gs.HardwareSlots, &gs.UsedSlots,
		&gs.RackUnits, &gs.UsedRackUnits, &gs.ColoCount, &gs.ColoMultiplier,
		&gs.HeatGenerated, &gs.CoolingCapacity, &gs.NetworkTier, &gs.AutomationTier,
		&gs.KnowledgePoints, &gs.IdleMultiplier, &gs.SaasUnlocked, &gs.TotalCustomers,
		&gs.ThrottleMultiplier, &gs.ThrottleTicksRemaining, &gs.DatacenterTier,
		&gs.OwnsDatacenter, &gs.DatacenterLevel, &gs.DatacenterIncomeMultiplier,
		&gs.TotalDonatedCU, &gs.BitcoinBalance, &gs.LastCustomerGrowthAt, &gs.LastTickAt, &gs.CreatedAt, &gs.UpdatedAt)
}

func (q *GameStateQueries) Create(ctx context.Context, userID string) (*models.GameState, error) {
	var gs models.GameState
	err := q.pool.QueryRow(ctx,
		`INSERT INTO game_states (user_id) VALUES ($1) RETURNING `+gsColumns,
		userID,
	).Scan(gsFields(&gs)...)
	if err != nil {
		return nil, err
	}
	return &gs, nil
}

func (q *GameStateQueries) GetByUserID(ctx context.Context, userID string) (*models.GameState, error) {
	var gs models.GameState
	err := q.pool.QueryRow(ctx,
		`SELECT `+gsColumns+` FROM game_states WHERE user_id = $1`, userID,
	).Scan(gsFields(&gs)...)
	if err != nil {
		return nil, err
	}
	return &gs, nil
}

func (q *GameStateQueries) Update(ctx context.Context, gs *models.GameState) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE game_states SET
		    tier = $2, compute_units = $3, reputation = $4, power_watts = $5,
		    power_limit = $6, money = $7, hardware_slots = $8, used_slots = $9,
		    rack_units = $10, used_rack_units = $11, colo_count = $12,
		    colo_multiplier = $13, heat_generated = $14, cooling_capacity = $15,
		    network_tier = $16, automation_tier = $17, knowledge_points = $18,
		    idle_multiplier = $19, saas_unlocked = $20, total_customers = $21,
		    throttle_multiplier = $22, throttle_ticks_remaining = $23,
		    datacenter_tier = $24, owns_datacenter = $25, datacenter_level = $26,
		    datacenter_income_multiplier = $27, total_donated_cu = $28,
		    bitcoin_balance = $29, last_customer_growth_at = $30,
		    last_tick_at = $31, updated_at = NOW()
		 WHERE id = $1 AND user_id = $32`,
		gs.ID, gs.Tier, gs.ComputeUnits, gs.Reputation, gs.PowerWatts,
		gs.PowerLimit, gs.Money, gs.HardwareSlots, gs.UsedSlots,
		gs.RackUnits, gs.UsedRackUnits, gs.ColoCount, gs.ColoMultiplier,
		gs.HeatGenerated, gs.CoolingCapacity, gs.NetworkTier, gs.AutomationTier,
		gs.KnowledgePoints, gs.IdleMultiplier, gs.SaasUnlocked, gs.TotalCustomers,
		gs.ThrottleMultiplier, gs.ThrottleTicksRemaining, gs.DatacenterTier,
		gs.OwnsDatacenter, gs.DatacenterLevel, gs.DatacenterIncomeMultiplier,
		gs.TotalDonatedCU, gs.BitcoinBalance, gs.LastCustomerGrowthAt, gs.LastTickAt, gs.UserID)
	return err
}

func (q *GameStateQueries) GetGlobalDonatedCU(ctx context.Context) (int64, error) {
	var total int64
	err := q.pool.QueryRow(ctx, `SELECT COALESCE(SUM(total_donated_cu), 0) FROM game_states`).Scan(&total)
	return total, err
}
