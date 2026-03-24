-- 013_rack_optimization.sql
-- Add rack optimization level to game_states (resets on prestige)

ALTER TABLE game_states ADD COLUMN rack_optimization INT NOT NULL DEFAULT 0;
