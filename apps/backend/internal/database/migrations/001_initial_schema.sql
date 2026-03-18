-- 001_initial_schema.sql
-- Initial database schema for Homelab the Game

-- Users table
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE,
    password_hash VARCHAR(255),
    display_name VARCHAR(100) NOT NULL,
    oauth_provider VARCHAR(50),
    oauth_id VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(oauth_provider, oauth_id)
);

-- Game state (one per user)
CREATE TABLE game_states (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tier VARCHAR(20) NOT NULL DEFAULT 'coffee_table',
    compute_units BIGINT NOT NULL DEFAULT 0,
    reputation BIGINT NOT NULL DEFAULT 0,
    power_watts INT NOT NULL DEFAULT 0,
    power_limit INT NOT NULL DEFAULT 200,
    money BIGINT NOT NULL DEFAULT 0,
    hardware_slots INT NOT NULL DEFAULT 2,
    used_slots INT NOT NULL DEFAULT 0,
    rack_units INT,
    used_rack_units INT,
    colo_count INT NOT NULL DEFAULT 0,
    colo_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    last_tick_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id)
);

-- Hardware owned by a player
CREATE TABLE hardware (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_state_id UUID NOT NULL REFERENCES game_states(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(50) NOT NULL,
    tier VARCHAR(20) NOT NULL,
    slots_used INT NOT NULL DEFAULT 1,
    rack_units_used INT,
    power_draw INT NOT NULL DEFAULT 0,
    compute_per_tick BIGINT NOT NULL DEFAULT 0,
    purchased_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Services deployed by a player
CREATE TABLE services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_state_id UUID NOT NULL REFERENCES game_states(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(50) NOT NULL,
    tier VARCHAR(20) NOT NULL,
    compute_per_tick BIGINT NOT NULL DEFAULT 0,
    reputation_per_tick BIGINT NOT NULL DEFAULT 0,
    money_per_tick BIGINT NOT NULL DEFAULT 0,
    deployed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Upgrades purchased by a player
CREATE TABLE upgrades (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_state_id UUID NOT NULL REFERENCES game_states(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(50) NOT NULL,
    tier VARCHAR(20) NOT NULL,
    persistent BOOLEAN NOT NULL DEFAULT FALSE,
    purchased_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Colo'd racks (persist through prestige)
CREATE TABLE colo_racks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    datacenter_tier INT NOT NULL DEFAULT 1,
    rack_size INT NOT NULL DEFAULT 48,
    compute_per_tick BIGINT NOT NULL DEFAULT 0,
    reputation_per_tick BIGINT NOT NULL DEFAULT 0,
    money_per_tick BIGINT NOT NULL DEFAULT 0,
    colo_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Groups / collectives
CREATE TABLE groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE,
    founder_id UUID NOT NULL REFERENCES users(id),
    min_contribution BIGINT NOT NULL DEFAULT 0,
    profit_split DECIMAL(5,2) NOT NULL DEFAULT 100.00,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE group_members (
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL DEFAULT 'member',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, user_id)
);

-- Leaderboards (materialized, refreshed periodically)
CREATE TABLE leaderboard_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category VARCHAR(50) NOT NULL,
    score BIGINT NOT NULL DEFAULT 0,
    rank INT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_leaderboard_category_score ON leaderboard_entries(category, score DESC);

-- TimescaleDB hypertable for tracking resource history
CREATE TABLE resource_history (
    time TIMESTAMPTZ NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    compute_units BIGINT NOT NULL DEFAULT 0,
    reputation BIGINT NOT NULL DEFAULT 0,
    money BIGINT NOT NULL DEFAULT 0,
    power_watts INT NOT NULL DEFAULT 0
);

SELECT create_hypertable('resource_history', 'time');

CREATE INDEX idx_resource_history_user ON resource_history(user_id, time DESC);

-- Event log (tracks events that happened to players)
CREATE TABLE event_log (
    time TIMESTAMPTZ NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL,
    tier VARCHAR(20) NOT NULL,
    resolved BOOLEAN NOT NULL DEFAULT FALSE,
    outcome JSONB
);

SELECT create_hypertable('event_log', 'time');
