-- 003_saas_system.sql
-- SaaS/IaaS customer simulation and business expenses

-- Customers subscribed to player's services
CREATE TABLE customers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_state_id UUID NOT NULL REFERENCES game_states(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    service_type VARCHAR(50) NOT NULL,
    monthly_revenue BIGINT NOT NULL DEFAULT 0,
    satisfaction INT NOT NULL DEFAULT 100,
    signed_up_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Recurring business expenses
CREATE TABLE expenses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_state_id UUID NOT NULL REFERENCES game_states(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(50) NOT NULL,
    cost_per_tick BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Track SaaS tier status on game_states
ALTER TABLE game_states ADD COLUMN saas_unlocked BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE game_states ADD COLUMN total_customers INT NOT NULL DEFAULT 0;
