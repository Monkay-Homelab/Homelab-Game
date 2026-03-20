-- 008_add_customer_growth_timestamp.sql
-- Separate timestamp for customer growth so it doesn't reset on every state poll

ALTER TABLE game_states ADD COLUMN last_customer_growth_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Initialize from last_tick_at for existing rows
UPDATE game_states SET last_customer_growth_at = last_tick_at;
