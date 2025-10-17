-- Remove generation_count column and index
DROP INDEX IF EXISTS idx_payments_email_address;
ALTER TABLE payments DROP COLUMN IF EXISTS generation_count;
