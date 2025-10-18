-- name: GetPoolState :one
SELECT site, next_index, last_updated_at, created_at
FROM address_pool_state
WHERE site = $1;

-- name: UpsertPoolState :exec
INSERT INTO address_pool_state (site, next_index, last_updated_at)
VALUES ($1, $2, NOW())
ON CONFLICT (site)
DO UPDATE SET
    next_index = EXCLUDED.next_index,
    last_updated_at = NOW();

-- name: GetAddress :one
SELECT id, site, address, address_index, status, email,
       reserved_at, used_at, last_checked, payment_count,
       balance_sats, tx_count, notes, created_at
FROM address_pool_addresses
WHERE address = $1;

-- name: GetAddressByIndex :one
SELECT id, site, address, address_index, status, email,
       reserved_at, used_at, last_checked, payment_count,
       balance_sats, tx_count, notes, created_at
FROM address_pool_addresses
WHERE site = $1 AND address_index = $2;

-- name: ListAddressesBySite :many
SELECT id, site, address, address_index, status, email,
       reserved_at, used_at, last_checked, payment_count,
       balance_sats, tx_count, notes, created_at
FROM address_pool_addresses
WHERE site = $1
ORDER BY address_index;

-- name: ListAddressesBySiteAndStatus :many
SELECT id, site, address, address_index, status, email,
       reserved_at, used_at, last_checked, payment_count,
       balance_sats, tx_count, notes, created_at
FROM address_pool_addresses
WHERE site = $1 AND status = $2
ORDER BY address_index;

-- name: CreateAddress :one
INSERT INTO address_pool_addresses (
    site, address, address_index, status, email,
    reserved_at, last_checked, payment_count,
    balance_sats, tx_count
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, $10
)
RETURNING id, site, address, address_index, status, email,
          reserved_at, used_at, last_checked, payment_count,
          balance_sats, tx_count, notes, created_at;

-- name: UpdateAddressStatus :exec
UPDATE address_pool_addresses
SET status = $2,
    last_checked = NOW()
WHERE address = $1;

-- name: UpdateAddressReservation :exec
UPDATE address_pool_addresses
SET status = 'reserved',
    email = $2,
    reserved_at = $3,
    last_checked = NOW()
WHERE address = $1;

-- name: UpdateAddressSiteAndReservation :exec
UPDATE address_pool_addresses
SET site = $2,
    status = 'reserved',
    email = $3,
    reserved_at = $4,
    last_checked = NOW()
WHERE address = $1;

-- name: MarkAddressUsed :exec
UPDATE address_pool_addresses
SET status = 'used',
    used_at = NOW(),
    payment_count = payment_count + 1,
    last_checked = NOW()
WHERE address = $1;

-- name: MarkAddressUsedWithSite :exec
-- Mark address as used WITHOUT changing the site field
-- This prevents constraint violations when addresses are used cross-site
UPDATE address_pool_addresses
SET status = 'used',
    used_at = NOW(),
    payment_count = COALESCE(payment_count, 0) + 1,
    last_checked = NOW()
WHERE address = $1;

-- name: UpdateAddressBalance :exec
UPDATE address_pool_addresses
SET balance_sats = $2,
    tx_count = $3,
    last_checked = NOW()
WHERE address = $1;

-- name: GetAvailableQueue :many
SELECT q.address, q.added_at
FROM address_pool_queue q
WHERE q.site = $1
ORDER BY q.added_at;

-- name: AddToQueue :exec
INSERT INTO address_pool_queue (site, address, added_at)
VALUES ($1, $2, NOW())
ON CONFLICT (site, address) DO NOTHING;

-- name: RemoveFromQueue :exec
DELETE FROM address_pool_queue
WHERE site = $1 AND address = $2;

-- name: GetExpiredReservations :many
SELECT id, site, address, address_index, status, email,
       reserved_at, used_at, last_checked, payment_count,
       balance_sats, tx_count, notes, created_at
FROM address_pool_addresses
WHERE status = 'reserved'
  AND reserved_at < NOW() - INTERVAL '72 hours'
ORDER BY reserved_at;

-- name: GetExpiredReservationsBySite :many
SELECT id, site, address, address_index, status, email,
       reserved_at, used_at, last_checked, payment_count,
       balance_sats, tx_count, notes, created_at
FROM address_pool_addresses
WHERE site = $1
  AND status = 'reserved'
  AND reserved_at < NOW() - INTERVAL '72 hours'
ORDER BY reserved_at;

-- name: CountAddressesByStatus :one
SELECT COUNT(*) as count
FROM address_pool_addresses
WHERE site = $1 AND status = $2;

-- name: GetPoolStats :one
SELECT
    COUNT(*) as total_addresses,
    COUNT(*) FILTER (WHERE status = 'available') as available_count,
    COUNT(*) FILTER (WHERE status = 'reserved') as reserved_count,
    COUNT(*) FILTER (WHERE status = 'used') as used_count,
    COUNT(*) FILTER (WHERE status = 'skipped') as skipped_count,
    COUNT(*) FILTER (WHERE status = 'expired') as expired_count
FROM address_pool_addresses
WHERE site = $1;

-- name: GetRecentAddressActivity :many
-- Get recent address activities showing reuse and recycling
SELECT
    a.address,
    a.site,
    a.status,
    a.email,
    a.payment_count,
    a.reserved_at,
    a.used_at,
    a.last_checked,
    a.created_at,
    CASE
        WHEN a.payment_count > 1 THEN 'reused'
        WHEN a.status = 'available' AND a.used_at IS NOT NULL THEN 'recycled'
        WHEN a.status = 'reserved' THEN 'active'
        ELSE 'new'
    END as activity_type
FROM address_pool_addresses a
WHERE a.site = $1
  AND (
      a.reserved_at > NOW() - INTERVAL '24 hours'
      OR a.used_at > NOW() - INTERVAL '24 hours'
      OR a.last_checked > NOW() - INTERVAL '24 hours'
  )
ORDER BY GREATEST(
    COALESCE(a.reserved_at, '1970-01-01'::timestamp),
    COALESCE(a.used_at, '1970-01-01'::timestamp),
    a.last_checked
) DESC
LIMIT 50;

-- name: GetRecyclingStats :one
-- Get statistics about address recycling for dashboard
SELECT
    COUNT(*) FILTER (WHERE payment_count > 1) as reused_addresses,
    COUNT(*) FILTER (WHERE status = 'available' AND used_at IS NOT NULL) as recycled_addresses,
    COUNT(*) FILTER (WHERE reserved_at > NOW() - INTERVAL '24 hours') as recent_reservations,
    COUNT(*) FILTER (WHERE used_at > NOW() - INTERVAL '24 hours') as recent_payments,
    COALESCE(SUM(payment_count), 0) as total_payments_processed,
    COALESCE(MAX(payment_count), 0) as max_reuse_count
FROM address_pool_addresses
WHERE site = $1;

-- name: GetAllPoolStats :one
-- Get aggregated pool statistics across all sites
SELECT
    COUNT(*) as total_addresses,
    COUNT(*) FILTER (WHERE status = 'available') as available_count,
    COUNT(*) FILTER (WHERE status = 'reserved') as reserved_count,
    COUNT(*) FILTER (WHERE status = 'used') as used_count,
    COUNT(*) FILTER (WHERE status = 'skipped') as skipped_count,
    COUNT(*) FILTER (WHERE status = 'expired') as expired_count,
    COUNT(*) FILTER (WHERE payment_count > 1) as reused_addresses,
    COUNT(*) FILTER (WHERE status = 'available' AND used_at IS NOT NULL) as recycled_addresses,
    COALESCE(SUM(payment_count), 0) as total_payments_processed
FROM address_pool_addresses;

-- name: GetAllSitePoolStats :many
-- Get pool statistics grouped by site
SELECT
    site,
    COUNT(*) as total_addresses,
    COUNT(*) FILTER (WHERE status = 'available') as available_count,
    COUNT(*) FILTER (WHERE status = 'reserved') as reserved_count,
    COUNT(*) FILTER (WHERE status = 'used') as used_count,
    COUNT(*) FILTER (WHERE status = 'skipped') as skipped_count,
    COUNT(*) FILTER (WHERE status = 'expired') as expired_count
FROM address_pool_addresses
GROUP BY site
ORDER BY site;
