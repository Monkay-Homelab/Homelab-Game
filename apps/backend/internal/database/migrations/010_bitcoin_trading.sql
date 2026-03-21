-- 010_bitcoin_trading.sql
-- Add Bitcoin trading schema: balance column on game_states, global price singleton, price history hypertable

-- Add bitcoin balance to game_states (persists through prestige)
ALTER TABLE game_states ADD COLUMN bitcoin_balance BIGINT NOT NULL DEFAULT 0;

-- Global bitcoin price state (singleton row)
CREATE TABLE bitcoin_price (
    id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    current_price BIGINT NOT NULL DEFAULT 10000,
    seed BIGINT NOT NULL DEFAULT 42,
    last_step_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Insert initial price
INSERT INTO bitcoin_price (current_price, seed) VALUES (10000, 42);

-- Price history for charting (TimescaleDB hypertable)
CREATE TABLE bitcoin_price_history (
    time TIMESTAMPTZ NOT NULL,
    price BIGINT NOT NULL
);

SELECT create_hypertable('bitcoin_price_history', 'time');

CREATE INDEX idx_bitcoin_price_history_time ON bitcoin_price_history (time DESC);

-- Automatically drop price history older than 7 days (~20K rows max)
SELECT add_retention_policy('bitcoin_price_history', INTERVAL '7 days');
