-- 002_progression_system.sql
-- Add cooling, networking, automation, and component upgrade tracking

-- Add progression fields to game_states
ALTER TABLE game_states ADD COLUMN heat_generated INT NOT NULL DEFAULT 0;
ALTER TABLE game_states ADD COLUMN cooling_capacity INT NOT NULL DEFAULT 50;
ALTER TABLE game_states ADD COLUMN network_tier INT NOT NULL DEFAULT 0;
ALTER TABLE game_states ADD COLUMN automation_tier INT NOT NULL DEFAULT 0;
ALTER TABLE game_states ADD COLUMN knowledge_points INT NOT NULL DEFAULT 0;
ALTER TABLE game_states ADD COLUMN idle_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0;

-- Component upgrades for individual hardware (CPU, RAM, storage, NIC)
CREATE TABLE component_upgrades (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hardware_id UUID NOT NULL REFERENCES hardware(id) ON DELETE CASCADE,
    component VARCHAR(20) NOT NULL,
    level INT NOT NULL DEFAULT 1,
    compute_bonus INT NOT NULL DEFAULT 0,
    power_reduction INT NOT NULL DEFAULT 0,
    upgraded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(hardware_id, component)
);
