-- 012_research_tree.sql
-- Add research levels table for permanent percentage boosts

CREATE TABLE research_levels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_state_id UUID NOT NULL REFERENCES game_states(id) ON DELETE CASCADE,
    research_node VARCHAR(50) NOT NULL,
    level INT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(game_state_id, research_node)
);

CREATE INDEX idx_research_levels_game_state ON research_levels(game_state_id);
