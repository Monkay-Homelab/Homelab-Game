-- 006_endgame.sql
-- Build your own datacenter endgame

ALTER TABLE game_states ADD COLUMN owns_datacenter BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE game_states ADD COLUMN datacenter_level INT NOT NULL DEFAULT 0;
ALTER TABLE game_states ADD COLUMN datacenter_income_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0;
