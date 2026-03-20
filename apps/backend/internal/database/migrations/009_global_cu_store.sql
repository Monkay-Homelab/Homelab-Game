-- 009_global_cu_store.sql
-- Add lifetime donated CU tracker to game_states (persists through prestige)

ALTER TABLE game_states ADD COLUMN total_donated_cu BIGINT NOT NULL DEFAULT 0;
