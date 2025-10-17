-- PayButton Address Pool State Management
-- Version: 1.0.0
-- Purpose: Persist address pool state for zero-downtime deployments

-- Table 1: Pool State per Site
-- Tracks the next index to generate for each site
CREATE TABLE IF NOT EXISTS address_pool_state (
    site VARCHAR(50) PRIMARY KEY,
    next_index INT NOT NULL,
    last_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT next_index_positive CHECK (next_index >= 0)
);

COMMENT ON TABLE address_pool_state IS 'Tracks next address index for each site to prevent gaps';
COMMENT ON COLUMN address_pool_state.next_index IS 'Next HD wallet index to generate';

-- Table 2: Address Storage
-- Every address generated, its state, and metadata
CREATE TABLE IF NOT EXISTS address_pool_addresses (
    id SERIAL PRIMARY KEY,
    site VARCHAR(50) NOT NULL,
    address VARCHAR(100) NOT NULL,
    address_index INT NOT NULL,
    status VARCHAR(20) NOT NULL,
    email VARCHAR(255),
    reserved_at TIMESTAMPTZ,
    used_at TIMESTAMPTZ,
    last_checked TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    payment_count INT DEFAULT 0,
    balance_sats BIGINT DEFAULT 0,
    tx_count INT DEFAULT 0,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT address_unique UNIQUE(address),
    CONSTRAINT site_index_unique UNIQUE(site, address_index),
    CONSTRAINT address_index_positive CHECK (address_index >= 0),
    CONSTRAINT payment_count_non_negative CHECK (payment_count >= 0),
    CONSTRAINT balance_non_negative CHECK (balance_sats >= 0),
    CONSTRAINT tx_count_non_negative CHECK (tx_count >= 0),
    CONSTRAINT valid_status CHECK (status IN ('available', 'reserved', 'used', 'skipped', 'expired'))
);

COMMENT ON TABLE address_pool_addresses IS 'Complete state of all generated addresses';
COMMENT ON COLUMN address_pool_addresses.status IS 'available|reserved|used|skipped|expired';
COMMENT ON COLUMN address_pool_addresses.address_index IS 'HD wallet derivation path index';
COMMENT ON COLUMN address_pool_addresses.balance_sats IS 'Last known balance in satoshis';
COMMENT ON COLUMN address_pool_addresses.tx_count IS 'Number of transactions on-chain';

-- Table 3: Available Address Queue
-- Fast FIFO queue for ready-to-use addresses
CREATE TABLE IF NOT EXISTS address_pool_queue (
    site VARCHAR(50) NOT NULL,
    address VARCHAR(100) NOT NULL,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (site, address),
    FOREIGN KEY (address) REFERENCES address_pool_addresses(address) ON DELETE CASCADE
);

COMMENT ON TABLE address_pool_queue IS 'FIFO queue of available addresses for instant assignment';

-- Indexes for Performance
CREATE INDEX idx_pool_state_updated ON address_pool_state(last_updated_at);
CREATE INDEX idx_addresses_site_status ON address_pool_addresses(site, status);
CREATE INDEX idx_addresses_site_index ON address_pool_addresses(site, address_index);
CREATE INDEX idx_addresses_address ON address_pool_addresses(address);
CREATE INDEX idx_addresses_email ON address_pool_addresses(email) WHERE email IS NOT NULL;
CREATE INDEX idx_addresses_reserved ON address_pool_addresses(reserved_at) WHERE status = 'reserved';
CREATE INDEX idx_addresses_status ON address_pool_addresses(status);
CREATE INDEX idx_queue_site_order ON address_pool_queue(site, added_at);

-- Initialize with sites (will be ignored if already exist)
INSERT INTO address_pool_state (site, next_index)
VALUES
    ('dwebstore', 0),
    ('cardershaven', 10000),
    ('ganymede', 20000)
ON CONFLICT (site) DO NOTHING;
