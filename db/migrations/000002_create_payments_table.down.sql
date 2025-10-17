-- Drop trigger
DROP TRIGGER IF EXISTS payments_updated_at_trigger ON payments;

-- Drop function
DROP FUNCTION IF EXISTS update_payments_updated_at();

-- Drop indexes
DROP INDEX IF EXISTS idx_payments_status_created;
DROP INDEX IF EXISTS idx_payments_site_created;
DROP INDEX IF EXISTS idx_payments_site_status;
DROP INDEX IF EXISTS idx_payments_created_at;
DROP INDEX IF EXISTS idx_payments_payment_id;
DROP INDEX IF EXISTS idx_payments_tx_hash;
DROP INDEX IF EXISTS idx_payments_status;
DROP INDEX IF EXISTS idx_payments_email;
DROP INDEX IF EXISTS idx_payments_site;
DROP INDEX IF EXISTS idx_payments_address;

-- Drop table
DROP TABLE IF EXISTS payments;
