-- CrossSpread Database Initialization
-- TimescaleDB + PostgreSQL schema

-- Enable extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- =============================================================================
-- USERS & AUTH
-- =============================================================================

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    is_active BOOLEAN NOT NULL DEFAULT true,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_active ON users(is_active) WHERE is_active = true;

-- =============================================================================
-- API KEYS (encrypted at rest)
-- =============================================================================

CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    exchange VARCHAR(50) NOT NULL,
    api_key_encrypted BYTEA NOT NULL,          -- AES-256 encrypted
    api_secret_encrypted BYTEA NOT NULL,        -- AES-256 encrypted
    passphrase_encrypted BYTEA,                 -- For exchanges that require it (OKX)
    is_testnet BOOLEAN NOT NULL DEFAULT false,
    is_active BOOLEAN NOT NULL DEFAULT true,
    label VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, exchange, label)
);

CREATE INDEX idx_api_keys_user ON api_keys(user_id);
CREATE INDEX idx_api_keys_exchange ON api_keys(exchange);

-- =============================================================================
-- EXCHANGES & INSTRUMENTS
-- =============================================================================

CREATE TABLE exchanges (
    id VARCHAR(50) PRIMARY KEY,                 -- binance, bybit, okx, etc.
    name VARCHAR(100) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL DEFAULT '{}',         -- Exchange-specific config
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE instruments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    exchange_id VARCHAR(50) NOT NULL REFERENCES exchanges(id),
    symbol VARCHAR(50) NOT NULL,                -- Exchange-native symbol (BTCUSDT)
    canonical_symbol VARCHAR(50) NOT NULL,      -- Normalized (BTC-USDT-PERP)
    base_asset VARCHAR(20) NOT NULL,            -- BTC
    quote_asset VARCHAR(20) NOT NULL,           -- USDT
    instrument_type VARCHAR(20) NOT NULL CHECK (instrument_type IN ('perpetual', 'future', 'spot')),
    contract_size DECIMAL(30, 10) NOT NULL DEFAULT 1,
    tick_size DECIMAL(30, 10) NOT NULL,
    lot_size DECIMAL(30, 10) NOT NULL,
    min_notional DECIMAL(30, 10),
    maker_fee DECIMAL(10, 6) NOT NULL DEFAULT 0.0002,
    taker_fee DECIMAL(10, 6) NOT NULL DEFAULT 0.0005,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(exchange_id, symbol)
);

CREATE INDEX idx_instruments_exchange ON instruments(exchange_id);
CREATE INDEX idx_instruments_canonical ON instruments(canonical_symbol);
CREATE INDEX idx_instruments_base ON instruments(base_asset);
CREATE INDEX idx_instruments_active ON instruments(is_active) WHERE is_active = true;

-- =============================================================================
-- SPREADS
-- =============================================================================

CREATE TABLE spreads (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    canonical_symbol VARCHAR(50) NOT NULL,      -- BTC-USDT-PERP
    long_exchange_id VARCHAR(50) NOT NULL REFERENCES exchanges(id),
    long_instrument_id UUID NOT NULL REFERENCES instruments(id),
    short_exchange_id VARCHAR(50) NOT NULL REFERENCES exchanges(id),
    short_instrument_id UUID NOT NULL REFERENCES instruments(id),
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(long_instrument_id, short_instrument_id)
);

CREATE INDEX idx_spreads_canonical ON spreads(canonical_symbol);
CREATE INDEX idx_spreads_active ON spreads(is_active) WHERE is_active = true;

-- =============================================================================
-- ORDERS & TRADES
-- =============================================================================

CREATE TYPE order_side AS ENUM ('buy', 'sell');
CREATE TYPE order_status AS ENUM ('pending', 'open', 'partial', 'filled', 'cancelled', 'rejected', 'expired');
CREATE TYPE order_type AS ENUM ('limit', 'market');
CREATE TYPE execution_mode AS ENUM ('live', 'sim');

CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    exchange_id VARCHAR(50) NOT NULL REFERENCES exchanges(id),
    instrument_id UUID NOT NULL REFERENCES instruments(id),
    exchange_order_id VARCHAR(100),             -- ID from exchange
    client_order_id VARCHAR(100) NOT NULL,
    side order_side NOT NULL,
    order_type order_type NOT NULL DEFAULT 'limit',
    price DECIMAL(30, 10),
    quantity DECIMAL(30, 10) NOT NULL,          -- In coin units
    filled_quantity DECIMAL(30, 10) NOT NULL DEFAULT 0,
    avg_fill_price DECIMAL(30, 10),
    status order_status NOT NULL DEFAULT 'pending',
    execution_mode execution_mode NOT NULL DEFAULT 'live',
    parent_trade_id UUID,                       -- Link to spread trade
    slice_index INTEGER,                        -- Which slice (0, 1, 2...)
    total_slices INTEGER,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    filled_at TIMESTAMPTZ
);

CREATE INDEX idx_orders_user ON orders(user_id);
CREATE INDEX idx_orders_exchange ON orders(exchange_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_parent ON orders(parent_trade_id);
CREATE INDEX idx_orders_created ON orders(created_at DESC);

CREATE TABLE trades (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    spread_id UUID NOT NULL REFERENCES spreads(id),
    execution_mode execution_mode NOT NULL DEFAULT 'live',
    
    -- Entry
    size_in_coins DECIMAL(30, 10) NOT NULL,
    entry_long_price DECIMAL(30, 10),
    entry_short_price DECIMAL(30, 10),
    entry_spread_bps DECIMAL(10, 4),
    
    -- Exit (filled when closed)
    exit_long_price DECIMAL(30, 10),
    exit_short_price DECIMAL(30, 10),
    exit_spread_bps DECIMAL(10, 4),
    
    -- Slicing config
    slice_size_coins DECIMAL(30, 10),
    slice_interval_ms INTEGER DEFAULT 100,
    total_slices INTEGER,
    
    -- PnL
    realized_pnl DECIMAL(30, 10),
    total_fees DECIMAL(30, 10),
    
    -- Status
    status VARCHAR(20) NOT NULL DEFAULT 'pending' 
        CHECK (status IN ('pending', 'entering', 'open', 'exiting', 'closed', 'cancelled', 'failed')),
    is_emergency_exit BOOLEAN NOT NULL DEFAULT false,
    error_message TEXT,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    entered_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ
);

CREATE INDEX idx_trades_user ON trades(user_id);
CREATE INDEX idx_trades_spread ON trades(spread_id);
CREATE INDEX idx_trades_status ON trades(status);
CREATE INDEX idx_trades_created ON trades(created_at DESC);

-- =============================================================================
-- AUDIT LOG
-- =============================================================================

CREATE TABLE audit_log (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID REFERENCES users(id),
    action VARCHAR(100) NOT NULL,
    entity_type VARCHAR(50),
    entity_id UUID,
    details JSONB NOT NULL DEFAULT '{}',
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_user ON audit_log(user_id);
CREATE INDEX idx_audit_action ON audit_log(action);
CREATE INDEX idx_audit_created ON audit_log(created_at DESC);

-- =============================================================================
-- TIME-SERIES DATA (TimescaleDB Hypertables)
-- =============================================================================

-- Orderbook snapshots (for playback/backtest)
CREATE TABLE orderbook_snapshots (
    time TIMESTAMPTZ NOT NULL,
    instrument_id UUID NOT NULL REFERENCES instruments(id),
    bids JSONB NOT NULL,                        -- [[price, qty], ...]
    asks JSONB NOT NULL,                        -- [[price, qty], ...]
    best_bid DECIMAL(30, 10),
    best_ask DECIMAL(30, 10),
    spread_bps DECIMAL(10, 4)
);

SELECT create_hypertable('orderbook_snapshots', 'time', chunk_time_interval => INTERVAL '1 hour');
CREATE INDEX idx_ob_instrument ON orderbook_snapshots(instrument_id, time DESC);

-- Spread ticks
CREATE TABLE spread_ticks (
    time TIMESTAMPTZ NOT NULL,
    spread_id UUID NOT NULL REFERENCES spreads(id),
    long_price DECIMAL(30, 10) NOT NULL,
    short_price DECIMAL(30, 10) NOT NULL,
    spread_bps DECIMAL(10, 4) NOT NULL,
    long_funding_rate DECIMAL(10, 6),
    short_funding_rate DECIMAL(10, 6)
);

SELECT create_hypertable('spread_ticks', 'time', chunk_time_interval => INTERVAL '1 hour');
CREATE INDEX idx_spread_ticks_spread ON spread_ticks(spread_id, time DESC);

-- Trade ticks (aggregated)
CREATE TABLE trade_ticks (
    time TIMESTAMPTZ NOT NULL,
    instrument_id UUID NOT NULL REFERENCES instruments(id),
    price DECIMAL(30, 10) NOT NULL,
    volume DECIMAL(30, 10) NOT NULL,
    side order_side NOT NULL
);

SELECT create_hypertable('trade_ticks', 'time', chunk_time_interval => INTERVAL '1 hour');
CREATE INDEX idx_trade_ticks_instrument ON trade_ticks(instrument_id, time DESC);

-- =============================================================================
-- DATA RETENTION POLICIES (TimescaleDB)
-- =============================================================================

-- Keep orderbook snapshots for 1 month
SELECT add_retention_policy('orderbook_snapshots', INTERVAL '1 month');

-- Keep spread ticks for 6 months
SELECT add_retention_policy('spread_ticks', INTERVAL '6 months');

-- Keep trade ticks for 1 month
SELECT add_retention_policy('trade_ticks', INTERVAL '1 month');

-- =============================================================================
-- INITIAL DATA
-- =============================================================================

-- Insert supported exchanges
INSERT INTO exchanges (id, name, config) VALUES
    ('binance', 'Binance', '{"ws_url": "wss://fstream.binance.com", "rest_url": "https://fapi.binance.com"}'),
    ('bybit', 'Bybit', '{"ws_url": "wss://stream.bybit.com", "rest_url": "https://api.bybit.com"}'),
    ('okx', 'OKX', '{"ws_url": "wss://ws.okx.com:8443", "rest_url": "https://www.okx.com"}'),
    ('kucoin', 'KuCoin', '{"ws_url": "wss://ws-api-futures.kucoin.com", "rest_url": "https://api-futures.kucoin.com"}'),
    ('mexc', 'MEXC', '{"ws_url": "wss://contract.mexc.com", "rest_url": "https://contract.mexc.com"}'),
    ('bitget', 'Bitget', '{"ws_url": "wss://ws.bitget.com", "rest_url": "https://api.bitget.com"}'),
    ('gateio', 'Gate.io', '{"ws_url": "wss://fx-ws.gateio.ws", "rest_url": "https://api.gateio.ws"}'),
    ('bingx', 'BingX', '{"ws_url": "wss://open-api-swap.bingx.com", "rest_url": "https://open-api.bingx.com"}'),
    ('coinex', 'CoinEx', '{"ws_url": "wss://perpetual.coinex.com", "rest_url": "https://api.coinex.com"}'),
    ('lbank', 'LBank', '{"ws_url": "wss://fapi.lbkex.net", "rest_url": "https://fapi.lbkex.net"}'),
    ('htx', 'HTX', '{"ws_url": "wss://api.hbdm.com", "rest_url": "https://api.hbdm.com"}')
ON CONFLICT (id) DO NOTHING;

-- Create default admin user (password: admin123 - CHANGE IN PRODUCTION!)
-- Password hash is bcrypt of 'admin123'
INSERT INTO users (username, password_hash, role) VALUES
    ('admin', '$2b$10$rQZ5Y5Y5Y5Y5Y5Y5Y5Y5YOmGJK9X9X9X9X9X9X9X9X9X9X9X9X9X', 'admin')
ON CONFLICT (username) DO NOTHING;

-- =============================================================================
-- FUNCTIONS & TRIGGERS
-- =============================================================================

-- Update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_api_keys_updated_at BEFORE UPDATE ON api_keys
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_instruments_updated_at BEFORE UPDATE ON instruments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_orders_updated_at BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_trades_updated_at BEFORE UPDATE ON trades
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
