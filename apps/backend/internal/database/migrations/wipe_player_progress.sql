-- wipe_player_progress.sql
-- One-off script: wipes all player game progress while keeping users, groups, and group_members.
-- Run manually: psql -d homelab_game -f wipe_player_progress.sql

BEGIN;

-- Delete all child tables of game_states
DELETE FROM component_upgrades;
DELETE FROM hardware;
DELETE FROM services;
DELETE FROM upgrades;
DELETE FROM customers;
DELETE FROM expenses;

-- Delete colo racks (referenced by user_id, not game_state_id)
DELETE FROM colo_racks;

-- Clear leaderboards (stale after wipe)
DELETE FROM leaderboard_entries;

-- Reset all game_states to fresh Coffee Table
UPDATE game_states SET
    tier = 'coffee_table',
    compute_units = 0,
    reputation = 0,
    power_watts = 0,
    power_limit = 500,
    money = 0,
    hardware_slots = 2,
    used_slots = 0,
    rack_units = NULL,
    used_rack_units = NULL,
    colo_count = 0,
    colo_multiplier = 1.0,
    heat_generated = 0,
    cooling_capacity = 50,
    network_tier = 0,
    automation_tier = 0,
    knowledge_points = 0,
    idle_multiplier = 1.0,
    saas_unlocked = false,
    total_customers = 0,
    throttle_multiplier = 1.0,
    throttle_ticks_remaining = 0,
    datacenter_tier = 0,
    owns_datacenter = false,
    datacenter_level = 0,
    datacenter_income_multiplier = 1.0,
    last_customer_growth_at = NOW(),
    last_tick_at = NOW(),
    updated_at = NOW();

COMMIT;
