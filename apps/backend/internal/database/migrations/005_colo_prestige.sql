-- 005_colo_prestige.sql
-- Datacenter tier tracking

ALTER TABLE game_states ADD COLUMN datacenter_tier INT NOT NULL DEFAULT 0;
