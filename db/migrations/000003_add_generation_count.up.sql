-- Add generation_count column to track address reuse
ALTER TABLE payments ADD COLUMN IF NOT EXISTS generation_count INTEGER DEFAULT 1;

-- Create index for efficient grouping queries
CREATE INDEX IF NOT EXISTS idx_payments_email_address ON payments(email, address);

-- Update existing records to set generation_count
-- This counts how many times each email+address combination appears
WITH generation_counts AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY email, address, site
            ORDER BY created_at ASC
        ) as gen_num
    FROM payments
    WHERE email IS NOT NULL
)
UPDATE payments p
SET generation_count = gc.gen_num
FROM generation_counts gc
WHERE p.id = gc.id;
