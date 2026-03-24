-- 011_overclock_mode.sql
-- Add overclock mode fields (temporary compute boost)

ALTER TABLE game_states ADD COLUMN overclock_multiplier DECIMAL(5,2) NOT NULL DEFAULT 1.0;
ALTER TABLE game_states ADD COLUMN overclock_ticks_remaining INT NOT NULL DEFAULT 0;
