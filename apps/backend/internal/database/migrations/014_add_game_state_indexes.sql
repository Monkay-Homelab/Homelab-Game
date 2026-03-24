-- Add missing indexes on game_state_id foreign keys.
-- These tables are queried on every tick (5s) and every action for every user.
-- Without indexes, PostgreSQL does sequential scans (O(n) per table).
CREATE INDEX IF NOT EXISTS idx_hardware_game_state ON hardware(game_state_id);
CREATE INDEX IF NOT EXISTS idx_services_game_state ON services(game_state_id);
CREATE INDEX IF NOT EXISTS idx_upgrades_game_state ON upgrades(game_state_id);
CREATE INDEX IF NOT EXISTS idx_customers_game_state ON customers(game_state_id);
CREATE INDEX IF NOT EXISTS idx_expenses_game_state ON expenses(game_state_id);
CREATE INDEX IF NOT EXISTS idx_colo_racks_user ON colo_racks(user_id);
