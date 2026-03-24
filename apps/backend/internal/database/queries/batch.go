package queries

import (
	"context"
	"fmt"

	"github.com/homelab-game/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FullGameData holds all data needed to process a tick or action for a single user.
type FullGameData struct {
	GameState      *models.GameState
	Hardware       []models.Hardware
	Services       []models.Service
	Upgrades       []models.Upgrade
	Customers      []models.Customer
	Expenses       []models.Expense
	ColoRacks      []models.ColoRack
	ComponentUps   []models.ComponentUpgrade
	ResearchLevels []models.ResearchLevel
}

// LoadFullGameState loads all game state data for a user in 2 DB round-trips:
// 1. A single-row lookup of game_states by user_id (returns the game_state_id).
// 2. A pgx.Batch of 8 queries for all child tables using the game_state_id
//    (plus colo_racks by user_id).
//
// This replaces 9 sequential queries that each required a separate pool acquire/release.
func LoadFullGameState(ctx context.Context, pool *pgxpool.Pool, userID string) (*FullGameData, error) {
	// Phase 1: Get game_state by user_id (single-row lookup by unique index, <1ms)
	var gs models.GameState
	err := pool.QueryRow(ctx,
		`SELECT `+gsColumns+` FROM game_states WHERE user_id = $1`, userID,
	).Scan(gsFields(&gs)...)
	if err != nil {
		return nil, fmt.Errorf("batch: failed to load game_state: %w", err)
	}

	// Phase 2: Batch all 8 child table reads in a single network round-trip.
	// Queries are added in a fixed order; results MUST be read in the same order.
	batch := &pgx.Batch{}

	// 1. hardware
	batch.Queue(
		`SELECT id, game_state_id, name, type, tier, slots_used, rack_units_used, power_draw, compute_per_tick, purchased_at
		 FROM hardware WHERE game_state_id = $1`,
		gs.ID,
	)

	// 2. services
	batch.Queue(
		`SELECT id, game_state_id, name, type, tier, compute_per_tick, reputation_per_tick, money_per_tick, deployed_at
		 FROM services WHERE game_state_id = $1`,
		gs.ID,
	)

	// 3. upgrades
	batch.Queue(
		`SELECT id, game_state_id, name, type, tier, persistent, purchased_at
		 FROM upgrades WHERE game_state_id = $1`,
		gs.ID,
	)

	// 4. customers
	batch.Queue(
		`SELECT id, game_state_id, name, service_type, monthly_revenue, satisfaction, signed_up_at
		 FROM customers WHERE game_state_id = $1`,
		gs.ID,
	)

	// 5. expenses
	batch.Queue(
		`SELECT id, game_state_id, name, type, cost_per_tick, created_at
		 FROM expenses WHERE game_state_id = $1`,
		gs.ID,
	)

	// 6. colo_racks (keyed by user_id, not game_state_id)
	batch.Queue(
		`SELECT id, user_id, datacenter_tier, rack_size, compute_per_tick, reputation_per_tick, money_per_tick, colo_at
		 FROM colo_racks WHERE user_id = $1 ORDER BY colo_at`,
		userID,
	)

	// 7. component_upgrades (joins through hardware)
	batch.Queue(
		`SELECT cu.id, cu.hardware_id, cu.component, cu.level, cu.compute_bonus, cu.power_reduction, cu.upgraded_at
		 FROM component_upgrades cu
		 JOIN hardware h ON h.id = cu.hardware_id
		 WHERE h.game_state_id = $1`,
		gs.ID,
	)

	// 8. research_levels
	batch.Queue(
		`SELECT id, game_state_id, research_node, level, updated_at
		 FROM research_levels WHERE game_state_id = $1`,
		gs.ID,
	)

	br := pool.SendBatch(ctx, batch)
	defer br.Close()

	data := &FullGameData{GameState: &gs}

	// 1. Parse hardware
	rows, err := br.Query()
	if err != nil {
		return nil, fmt.Errorf("batch: failed to query hardware: %w", err)
	}
	for rows.Next() {
		var h models.Hardware
		if err := rows.Scan(&h.ID, &h.GameStateID, &h.Name, &h.Type, &h.Tier,
			&h.SlotsUsed, &h.RackUnitsUsed, &h.PowerDraw, &h.ComputePerTick, &h.PurchasedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("batch: failed to scan hardware: %w", err)
		}
		data.Hardware = append(data.Hardware, h)
	}
	rows.Close()

	// 2. Parse services
	rows, err = br.Query()
	if err != nil {
		return nil, fmt.Errorf("batch: failed to query services: %w", err)
	}
	for rows.Next() {
		var s models.Service
		if err := rows.Scan(&s.ID, &s.GameStateID, &s.Name, &s.Type, &s.Tier,
			&s.ComputePerTick, &s.ReputationPerTick, &s.MoneyPerTick, &s.DeployedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("batch: failed to scan services: %w", err)
		}
		data.Services = append(data.Services, s)
	}
	rows.Close()

	// 3. Parse upgrades
	rows, err = br.Query()
	if err != nil {
		return nil, fmt.Errorf("batch: failed to query upgrades: %w", err)
	}
	for rows.Next() {
		var u models.Upgrade
		if err := rows.Scan(&u.ID, &u.GameStateID, &u.Name, &u.Type, &u.Tier,
			&u.Persistent, &u.PurchasedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("batch: failed to scan upgrades: %w", err)
		}
		data.Upgrades = append(data.Upgrades, u)
	}
	rows.Close()

	// 4. Parse customers
	rows, err = br.Query()
	if err != nil {
		return nil, fmt.Errorf("batch: failed to query customers: %w", err)
	}
	for rows.Next() {
		var c models.Customer
		if err := rows.Scan(&c.ID, &c.GameStateID, &c.Name, &c.ServiceType,
			&c.MonthlyRevenue, &c.Satisfaction, &c.SignedUpAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("batch: failed to scan customers: %w", err)
		}
		data.Customers = append(data.Customers, c)
	}
	rows.Close()

	// 5. Parse expenses
	rows, err = br.Query()
	if err != nil {
		return nil, fmt.Errorf("batch: failed to query expenses: %w", err)
	}
	for rows.Next() {
		var e models.Expense
		if err := rows.Scan(&e.ID, &e.GameStateID, &e.Name, &e.Type,
			&e.CostPerTick, &e.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("batch: failed to scan expenses: %w", err)
		}
		data.Expenses = append(data.Expenses, e)
	}
	rows.Close()

	// 6. Parse colo_racks
	rows, err = br.Query()
	if err != nil {
		return nil, fmt.Errorf("batch: failed to query colo_racks: %w", err)
	}
	for rows.Next() {
		var cr models.ColoRack
		if err := rows.Scan(&cr.ID, &cr.UserID, &cr.DatacenterTier, &cr.RackSize,
			&cr.ComputePerTick, &cr.ReputationPerTick, &cr.MoneyPerTick, &cr.ColoAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("batch: failed to scan colo_racks: %w", err)
		}
		data.ColoRacks = append(data.ColoRacks, cr)
	}
	rows.Close()

	// 7. Parse component_upgrades
	rows, err = br.Query()
	if err != nil {
		return nil, fmt.Errorf("batch: failed to query component_upgrades: %w", err)
	}
	for rows.Next() {
		var cu models.ComponentUpgrade
		if err := rows.Scan(&cu.ID, &cu.HardwareID, &cu.Component, &cu.Level,
			&cu.ComputeBonus, &cu.PowerReduction, &cu.UpgradedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("batch: failed to scan component_upgrades: %w", err)
		}
		data.ComponentUps = append(data.ComponentUps, cu)
	}
	rows.Close()

	// 8. Parse research_levels
	rows, err = br.Query()
	if err != nil {
		return nil, fmt.Errorf("batch: failed to query research_levels: %w", err)
	}
	for rows.Next() {
		var rl models.ResearchLevel
		if err := rows.Scan(&rl.ID, &rl.GameStateID, &rl.ResearchNode, &rl.Level,
			&rl.UpdatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("batch: failed to scan research_levels: %w", err)
		}
		data.ResearchLevels = append(data.ResearchLevels, rl)
	}
	rows.Close()

	return data, nil
}
