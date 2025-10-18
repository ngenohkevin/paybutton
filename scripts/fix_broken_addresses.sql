-- Cleanup Script for Broken Addresses
-- This script fixes addresses that received payments but are stuck in "reserved" status
-- due to the constraint violation bug in MarkAddressUsedWithSite

-- First, let's see what needs to be fixed
SELECT
    'BEFORE FIX' as status,
    a.address,
    a.site,
    a.status as current_status,
    a.email,
    a.payment_count,
    COUNT(p.id) as actual_payments,
    STRING_AGG(p.tx_hash, ', ') as tx_hashes
FROM address_pool_addresses a
LEFT JOIN payments p ON p.address = a.address AND p.status = 'completed'
WHERE a.status = 'reserved'
GROUP BY a.address, a.site, a.status, a.email, a.payment_count
HAVING COUNT(p.id) > 0
ORDER BY a.site, a.address;

-- Fix addresses that have completed payments but are still marked as "reserved"
-- This handles the specific bug where MarkAddressUsedWithSite failed
UPDATE address_pool_addresses a
SET
    status = 'used',
    used_at = NOW(),
    payment_count = COALESCE(a.payment_count, 0) + subquery.payment_count_to_add,
    last_checked = NOW()
FROM (
    SELECT
        p.address,
        COUNT(*) as payment_count_to_add
    FROM payments p
    WHERE p.status = 'completed'
    AND EXISTS (
        SELECT 1
        FROM address_pool_addresses a2
        WHERE a2.address = p.address
        AND a2.status = 'reserved'
    )
    GROUP BY p.address
) AS subquery
WHERE a.address = subquery.address
AND a.status = 'reserved';

-- Verify the fix
SELECT
    'AFTER FIX' as status,
    a.address,
    a.site,
    a.status as new_status,
    a.email,
    a.payment_count,
    a.used_at,
    COUNT(p.id) as actual_payments,
    STRING_AGG(p.tx_hash, ', ') as tx_hashes
FROM address_pool_addresses a
LEFT JOIN payments p ON p.address = a.address AND p.status = 'completed'
WHERE a.used_at IS NOT NULL
  AND a.used_at > NOW() - INTERVAL '1 minute'
GROUP BY a.address, a.site, a.status, a.email, a.payment_count, a.used_at
ORDER BY a.used_at DESC;

-- Summary statistics
SELECT
    'SUMMARY' as report_type,
    COUNT(*) FILTER (WHERE status = 'used') as fixed_addresses,
    COUNT(*) FILTER (WHERE status = 'reserved' AND payment_count > 0) as still_broken
FROM address_pool_addresses;
