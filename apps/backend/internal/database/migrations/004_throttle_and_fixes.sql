-- 004_throttle_and_fixes.sql
-- Add throttle tracking for events

ALTER TABLE game_states ADD COLUMN throttle_multiplier DECIMAL(5,2) NOT NULL DEFAULT 1.0;
ALTER TABLE game_states ADD COLUMN throttle_ticks_remaining INT NOT NULL DEFAULT 0;
