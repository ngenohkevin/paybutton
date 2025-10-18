-- name: FindReservedAddressesWithPayments :many
-- Find addresses marked as "reserved" but have completed payments
SELECT DISTINCT
    a.address,
    a.site,
    a.email,
    COUNT(p.id) as payment_count
FROM address_pool_addresses a
INNER JOIN payments p ON p.address = a.address
WHERE a.status = 'reserved'
AND p.status = 'completed'
GROUP BY a.address, a.site, a.email;

-- name: FixReservedAddressWithPayment :exec
-- Mark a reserved address as used (called by health checker)
UPDATE address_pool_addresses
SET status = 'used',
    used_at = NOW(),
    payment_count = COALESCE(payment_count, 0) + 1,
    last_checked = NOW()
WHERE address = $1
AND status = 'reserved';

-- name: RemoveUsedAddressesFromQueue :exec
-- Remove all "used" addresses from the queue
DELETE FROM address_pool_queue q
WHERE EXISTS (
    SELECT 1 FROM address_pool_addresses a
    WHERE a.address = q.address
    AND a.status = 'used'
);

-- name: FixNullPaymentCounts :exec
-- Fix addresses marked as "used" but have NULL payment_count
UPDATE address_pool_addresses a
SET payment_count = COALESCE((
    SELECT COUNT(*)
    FROM payments p
    WHERE p.address = a.address
    AND p.status = 'completed'
), 0)
WHERE a.status = 'used'
AND a.payment_count IS NULL;

-- name: GetExpiredReservationsForHealthCheck :many
-- Get addresses reserved for >72 hours for health check verification
SELECT
    address,
    site,
    email,
    reserved_at,
    EXTRACT(EPOCH FROM (NOW() - reserved_at))/3600 as hours_old
FROM address_pool_addresses
WHERE status = 'reserved'
AND reserved_at < NOW() - INTERVAL '72 hours'
ORDER BY reserved_at;

-- name: GetAllReservedAddresses :many
-- Get all reserved addresses for blockchain verification
SELECT
    address,
    site,
    email,
    reserved_at
FROM address_pool_addresses
WHERE status = 'reserved'
ORDER BY reserved_at DESC;

-- name: HealthCheckSummary :one
-- Get overall health check summary
SELECT
    COUNT(*) as total_addresses,
    COUNT(*) FILTER (WHERE status = 'available') as available_count,
    COUNT(*) FILTER (WHERE status = 'reserved') as reserved_count,
    COUNT(*) FILTER (WHERE status = 'used') as used_count,
    COUNT(*) FILTER (WHERE status = 'reserved' AND reserved_at < NOW() - INTERVAL '72 hours') as expired_reservations,
    COUNT(*) FILTER (WHERE status = 'used' AND payment_count IS NULL) as null_payment_counts,
    COUNT(*) FILTER (WHERE status = 'used' AND used_at IS NULL) as null_used_timestamps,
    (SELECT COUNT(*) FROM address_pool_queue q
     JOIN address_pool_addresses a ON a.address = q.address
     WHERE a.status = 'used') as used_in_queue
FROM address_pool_addresses;

-- name: CountReservedWithPayments :one
-- Count how many reserved addresses have completed payments
SELECT COUNT(DISTINCT a.address) as count
FROM address_pool_addresses a
INNER JOIN payments p ON p.address = a.address
WHERE a.status = 'reserved'
AND p.status = 'completed';

-- name: GetAllAddressesForBlockchainCheck :many
-- Get ALL addresses to verify transaction history on blockchain
SELECT
    address,
    site,
    status,
    email,
    payment_count,
    reserved_at,
    used_at,
    last_checked
FROM address_pool_addresses
ORDER BY
    CASE status
        WHEN 'reserved' THEN 1  -- Check reserved first (most important)
        WHEN 'used' THEN 2      -- Then used (verify they're actually used)
        WHEN 'available' THEN 3 -- Then available (should have no history)
        ELSE 4
    END,
    last_checked ASC;           -- Oldest checked first
