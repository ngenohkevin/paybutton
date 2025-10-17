-- Create payments table for tracking all payment transactions
CREATE TABLE IF NOT EXISTS payments (
    id SERIAL PRIMARY KEY,
    payment_id VARCHAR(100) UNIQUE NOT NULL,

    -- Address and site info
    address VARCHAR(100) NOT NULL,
    site VARCHAR(50) NOT NULL,

    -- Transaction details
    tx_hash VARCHAR(100),
    amount_btc DECIMAL(16, 8) NOT NULL,
    amount_usd DECIMAL(10, 2),
    currency VARCHAR(10) NOT NULL DEFAULT 'BTC',

    -- Confirmation status
    confirmations INT DEFAULT 0,
    required_confirmations INT DEFAULT 1,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',

    -- User and order info
    email VARCHAR(255),
    order_id VARCHAR(100),
    user_agent TEXT,
    ip_address VARCHAR(50),

    -- Timestamps
    payment_initiated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    first_seen_at TIMESTAMPTZ,
    confirmed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,

    -- Metadata
    notes TEXT,
    webhook_sent BOOLEAN DEFAULT FALSE,
    webhook_sent_at TIMESTAMPTZ,
    email_sent BOOLEAN DEFAULT FALSE,
    email_sent_at TIMESTAMPTZ,
    telegram_sent BOOLEAN DEFAULT FALSE,
    telegram_sent_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT valid_status CHECK (status IN ('pending', 'detected', 'confirming', 'confirmed', 'completed', 'expired', 'failed')),
    CONSTRAINT valid_currency CHECK (currency IN ('BTC', 'USDT')),
    CONSTRAINT positive_amount CHECK (amount_btc > 0),

    -- Foreign key to address pool
    CONSTRAINT fk_payment_address FOREIGN KEY (address)
        REFERENCES address_pool_addresses(address)
        ON DELETE CASCADE
);

-- Indexes for fast lookups
CREATE INDEX idx_payments_address ON payments(address);
CREATE INDEX idx_payments_site ON payments(site);
CREATE INDEX idx_payments_email ON payments(email);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_tx_hash ON payments(tx_hash);
CREATE INDEX idx_payments_payment_id ON payments(payment_id);
CREATE INDEX idx_payments_created_at ON payments(created_at DESC);
CREATE INDEX idx_payments_site_status ON payments(site, status);

-- Composite index for analytics queries
CREATE INDEX idx_payments_site_created ON payments(site, created_at DESC);
CREATE INDEX idx_payments_status_created ON payments(status, created_at DESC);

-- Create function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_payments_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger to auto-update updated_at
CREATE TRIGGER payments_updated_at_trigger
    BEFORE UPDATE ON payments
    FOR EACH ROW
    EXECUTE FUNCTION update_payments_updated_at();

-- Add comment
COMMENT ON TABLE payments IS 'Tracks all payment transactions with complete history and status';
