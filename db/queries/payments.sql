-- name: CreatePayment :one
INSERT INTO payments (
    payment_id,
    address,
    site,
    amount_btc,
    amount_usd,
    currency,
    email,
    order_id,
    user_agent,
    ip_address,
    required_confirmations,
    expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING *;

-- name: GetPayment :one
SELECT * FROM payments
WHERE payment_id = $1;

-- name: GetPaymentByAddress :one
SELECT * FROM payments
WHERE address = $1 AND status != 'expired'
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdatePaymentTransaction :exec
UPDATE payments
SET
    tx_hash = $2,
    status = $3,
    confirmations = $4,
    first_seen_at = COALESCE(first_seen_at, NOW()),
    updated_at = NOW()
WHERE payment_id = $1;

-- name: UpdatePaymentConfirmed :exec
UPDATE payments
SET
    status = 'confirmed',
    confirmations = $2,
    confirmed_at = COALESCE(confirmed_at, NOW()),
    updated_at = NOW()
WHERE payment_id = $1;

-- name: UpdatePaymentCompleted :exec
UPDATE payments
SET
    status = 'completed',
    completed_at = NOW(),
    updated_at = NOW()
WHERE payment_id = $1;

-- name: MarkPaymentExpired :exec
UPDATE payments
SET
    status = 'expired',
    updated_at = NOW()
WHERE payment_id = $1;

-- name: UpdatePaymentWebhookSent :exec
UPDATE payments
SET
    webhook_sent = true,
    webhook_sent_at = NOW(),
    updated_at = NOW()
WHERE payment_id = $1;

-- name: UpdatePaymentEmailSent :exec
UPDATE payments
SET
    email_sent = true,
    email_sent_at = NOW(),
    updated_at = NOW()
WHERE payment_id = $1;

-- name: UpdatePaymentTelegramSent :exec
UPDATE payments
SET
    telegram_sent = true,
    telegram_sent_at = NOW(),
    updated_at = NOW()
WHERE payment_id = $1;

-- name: ListPaymentsBySite :many
SELECT * FROM payments
WHERE site = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListPaymentsByEmail :many
SELECT * FROM payments
WHERE email = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListPaymentsByStatus :many
SELECT * FROM payments
WHERE status = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListPendingPayments :many
SELECT * FROM payments
WHERE status IN ('pending', 'detected', 'confirming')
ORDER BY created_at ASC;

-- name: ListExpiredPayments :many
SELECT * FROM payments
WHERE status = 'pending'
  AND expires_at < NOW()
ORDER BY expires_at ASC;

-- name: GetPaymentStats :one
SELECT
    COUNT(*) as total_payments,
    COUNT(*) FILTER (WHERE status = 'completed') as completed_count,
    COUNT(*) FILTER (WHERE status = 'pending') as pending_count,
    COUNT(*) FILTER (WHERE status = 'expired') as expired_count,
    COALESCE(SUM(amount_btc) FILTER (WHERE status = 'completed'), 0) as total_btc,
    COALESCE(SUM(amount_usd) FILTER (WHERE status = 'completed'), 0) as total_usd,
    COALESCE(AVG(amount_btc) FILTER (WHERE status = 'completed'), 0) as avg_btc,
    COALESCE(AVG(amount_usd) FILTER (WHERE status = 'completed'), 0) as avg_usd
FROM payments
WHERE site = $1;

-- name: GetPaymentStatsByDateRange :one
SELECT
    COUNT(*) as total_payments,
    COUNT(*) FILTER (WHERE status = 'completed') as completed_count,
    COUNT(*) FILTER (WHERE status = 'pending') as pending_count,
    COUNT(*) FILTER (WHERE status = 'expired') as expired_count,
    COALESCE(SUM(amount_btc) FILTER (WHERE status = 'completed'), 0) as total_btc,
    COALESCE(SUM(amount_usd) FILTER (WHERE status = 'completed'), 0) as total_usd,
    COALESCE(AVG(amount_btc) FILTER (WHERE status = 'completed'), 0) as avg_btc,
    COALESCE(AVG(amount_usd) FILTER (WHERE status = 'completed'), 0) as avg_usd
FROM payments
WHERE site = $1
  AND created_at BETWEEN $2 AND $3;

-- name: GetRecentPayments :many
SELECT * FROM payments
WHERE site = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: GetPaymentsByDateRange :many
SELECT * FROM payments
WHERE site = $1
  AND created_at BETWEEN $2 AND $3
ORDER BY created_at DESC;

-- name: SearchPayments :many
SELECT * FROM payments
WHERE site = $1
  AND (
    email ILIKE '%' || $2 || '%'
    OR payment_id ILIKE '%' || $2 || '%'
    OR tx_hash ILIKE '%' || $2 || '%'
    OR address ILIKE '%' || $2 || '%'
  )
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: GetPaymentCountByStatus :many
SELECT
    status,
    COUNT(*) as count
FROM payments
WHERE site = $1
GROUP BY status;

-- name: GetDailyPaymentStats :many
SELECT
    DATE(created_at) as date,
    COUNT(*) as payment_count,
    COUNT(*) FILTER (WHERE status = 'completed') as completed_count,
    COALESCE(SUM(amount_btc) FILTER (WHERE status = 'completed'), 0) as total_btc,
    COALESCE(SUM(amount_usd) FILTER (WHERE status = 'completed'), 0) as total_usd
FROM payments
WHERE site = $1
  AND created_at >= $2
GROUP BY DATE(created_at)
ORDER BY date DESC;

-- name: ListPaymentsWithFilters :many
SELECT * FROM payments
WHERE
    (sqlc.narg('site')::text IS NULL OR site = sqlc.narg('site'))
    AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
    AND (
        sqlc.narg('search')::text IS NULL
        OR email ILIKE '%' || sqlc.narg('search') || '%'
        OR payment_id ILIKE '%' || sqlc.narg('search') || '%'
        OR address ILIKE '%' || sqlc.narg('search') || '%'
    )
    AND (
        sqlc.narg('start_date')::timestamptz IS NULL
        OR created_at >= sqlc.narg('start_date')
    )
    AND (
        sqlc.narg('end_date')::timestamptz IS NULL
        OR created_at <= sqlc.narg('end_date')
    )
ORDER BY created_at DESC
LIMIT sqlc.arg('limit')
OFFSET sqlc.arg('offset');

-- name: CountPaymentsWithFilters :one
SELECT COUNT(*) FROM payments
WHERE
    (sqlc.narg('site')::text IS NULL OR site = sqlc.narg('site'))
    AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
    AND (
        sqlc.narg('search')::text IS NULL
        OR email ILIKE '%' || sqlc.narg('search') || '%'
        OR payment_id ILIKE '%' || sqlc.narg('search') || '%'
        OR address ILIKE '%' || sqlc.narg('search') || '%'
    )
    AND (
        sqlc.narg('start_date')::timestamptz IS NULL
        OR created_at >= sqlc.narg('start_date')
    )
    AND (
        sqlc.narg('end_date')::timestamptz IS NULL
        OR created_at <= sqlc.narg('end_date')
    );

-- name: DeletePayment :exec
DELETE FROM payments
WHERE payment_id = $1;

-- name: DeleteOldPayments :exec
DELETE FROM payments
WHERE created_at < $1;

-- name: DeleteExpiredPaymentsByAddress :exec
-- Delete expired payments for a specific address (used when address is recycled)
DELETE FROM payments
WHERE address = $1
  AND status = 'expired';

-- Dashboard Queries

-- name: GetDashboardOverview :one
-- Get comprehensive dashboard statistics across all sites
SELECT
    COUNT(*) as total_payments,
    COUNT(*) FILTER (WHERE status = 'completed') as completed_payments,
    COUNT(*) FILTER (WHERE status = 'pending') as pending_payments,
    COUNT(*) FILTER (WHERE status = 'confirming') as confirming_payments,
    COUNT(*) FILTER (WHERE status = 'expired') as expired_payments,
    COUNT(*) FILTER (WHERE status = 'failed') as failed_payments,
    COALESCE(SUM(amount_btc) FILTER (WHERE status = 'completed'), 0) as total_btc_received,
    COALESCE(SUM(amount_usd) FILTER (WHERE status = 'completed'), 0) as total_usd_received,
    COALESCE(AVG(amount_btc) FILTER (WHERE status = 'completed'), 0) as avg_btc_per_payment,
    COALESCE(AVG(amount_usd) FILTER (WHERE status = 'completed'), 0) as avg_usd_per_payment,
    COUNT(DISTINCT site) as total_sites,
    COUNT(DISTINCT address) as total_addresses_used,
    COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '24 hours') as payments_last_24h,
    COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '1 hour') as payments_last_hour,
    COUNT(*) FILTER (WHERE status = 'completed' AND confirmed_at >= NOW() - INTERVAL '24 hours') as completed_last_24h
FROM payments;

-- name: GetRecentPaymentsAllSites :many
-- Get recent payments across all sites with pagination
SELECT
    payment_id,
    address,
    site,
    amount_btc,
    amount_usd,
    currency,
    status,
    confirmations,
    required_confirmations,
    email,
    tx_hash,
    created_at,
    confirmed_at,
    completed_at
FROM payments
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: GetSiteBreakdown :many
-- Get payment breakdown by site for dashboard
SELECT
    site,
    COUNT(*) as total_payments,
    COUNT(*) FILTER (WHERE status = 'completed') as completed_payments,
    COUNT(*) FILTER (WHERE status = 'pending') as pending_payments,
    COALESCE(SUM(amount_btc) FILTER (WHERE status = 'completed'), 0) as total_btc,
    COALESCE(SUM(amount_usd) FILTER (WHERE status = 'completed'), 0) as total_usd,
    MAX(created_at) as last_payment_at
FROM payments
GROUP BY site
ORDER BY total_payments DESC;

-- name: GetHourlyPaymentTrend :many
-- Get hourly payment trends for the last 24 hours
SELECT
    DATE_TRUNC('hour', created_at) as hour,
    COUNT(*) as payment_count,
    COUNT(*) FILTER (WHERE status = 'completed') as completed_count,
    COUNT(*) FILTER (WHERE status = 'pending') as pending_count,
    COALESCE(SUM(amount_btc) FILTER (WHERE status = 'completed'), 0) as btc_volume,
    COALESCE(SUM(amount_usd) FILTER (WHERE status = 'completed'), 0) as usd_volume
FROM payments
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY DATE_TRUNC('hour', created_at)
ORDER BY hour DESC;

-- name: GetPaymentStatusDistribution :many
-- Get count of payments by status for dashboard pie chart
SELECT
    status,
    COUNT(*) as count,
    COALESCE(SUM(amount_usd), 0) as total_usd
FROM payments
GROUP BY status
ORDER BY count DESC;

-- name: GetTopPaymentEmails :many
-- Get top paying email addresses
SELECT
    email,
    COUNT(*) as payment_count,
    COALESCE(SUM(amount_btc) FILTER (WHERE status = 'completed'), 0) as total_btc,
    COALESCE(SUM(amount_usd) FILTER (WHERE status = 'completed'), 0) as total_usd,
    MAX(created_at) as last_payment_at
FROM payments
WHERE email IS NOT NULL AND email != ''
GROUP BY email
ORDER BY payment_count DESC
LIMIT $1;

-- name: GetPaymentConversionRate :one
-- Calculate payment success/conversion rate
SELECT
    COUNT(*) as total_initiated,
    COUNT(*) FILTER (WHERE status = 'completed') as total_completed,
    CASE
        WHEN COUNT(*) > 0 THEN
            (COUNT(*) FILTER (WHERE status = 'completed')::float / COUNT(*)::float) * 100
        ELSE 0
    END as conversion_rate,
    CASE
        WHEN COUNT(*) > 0 THEN
            (COUNT(*) FILTER (WHERE status = 'expired')::float / COUNT(*)::float) * 100
        ELSE 0
    END as expiration_rate
FROM payments
WHERE created_at >= $1;

-- name: GetAverageConfirmationTime :one
-- Get average time from payment creation to confirmation
SELECT
    COUNT(*) FILTER (WHERE confirmed_at IS NOT NULL) as confirmed_count,
    AVG(EXTRACT(EPOCH FROM (confirmed_at - first_seen_at))) as avg_confirmation_seconds,
    AVG(EXTRACT(EPOCH FROM (completed_at - created_at))) as avg_completion_seconds,
    PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (confirmed_at - first_seen_at))) as median_confirmation_seconds
FROM payments
WHERE confirmed_at IS NOT NULL
  AND first_seen_at IS NOT NULL
  AND created_at >= $1;

-- name: GetActivePendingPayments :many
-- Get currently active pending payments for monitoring
SELECT
    payment_id,
    address,
    site,
    amount_btc,
    amount_usd,
    email,
    status,
    confirmations,
    required_confirmations,
    created_at,
    expires_at,
    EXTRACT(EPOCH FROM (expires_at - NOW())) as seconds_until_expiry
FROM payments
WHERE status IN ('pending', 'detected', 'confirming')
  AND expires_at > NOW()
ORDER BY created_at DESC;

-- name: GetTodayStatistics :one
-- Get today's statistics for dashboard summary
SELECT
    COUNT(*) as total_today,
    COUNT(*) FILTER (WHERE status = 'completed') as completed_today,
    COUNT(*) FILTER (WHERE status = 'pending') as pending_today,
    COALESCE(SUM(amount_btc) FILTER (WHERE status = 'completed'), 0) as btc_today,
    COALESCE(SUM(amount_usd) FILTER (WHERE status = 'completed'), 0) as usd_today,
    COUNT(DISTINCT site) as active_sites_today,
    COUNT(DISTINCT email) FILTER (WHERE email IS NOT NULL AND email != '') as unique_customers_today
FROM payments
WHERE DATE(created_at) = CURRENT_DATE;

-- name: GetRecentCompletedPayments :many
-- Get recent completed payments only (actually paid)
SELECT
    payment_id,
    address,
    site,
    amount_btc,
    amount_usd,
    currency,
    status,
    confirmations,
    required_confirmations,
    email,
    tx_hash,
    created_at,
    confirmed_at,
    completed_at
FROM payments
WHERE status IN ('completed', 'confirmed')
ORDER BY COALESCE(completed_at, confirmed_at, created_at) DESC
LIMIT $1 OFFSET $2;

-- name: ListPaymentsGroupedByEmailAddress :many
-- Get payments grouped by email+address combination, showing only most recent with generation count
WITH grouped_payments AS (
    SELECT
        email,
        address,
        site,
        status,
        COUNT(*) as generation_count,
        MAX(id) as latest_payment_id,
        MAX(created_at) as latest_created_at,
        MIN(created_at) as first_created_at
    FROM payments
    WHERE
        (sqlc.narg('site')::text IS NULL OR site = sqlc.narg('site'))
        AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
        AND (
            sqlc.narg('search')::text IS NULL
            OR email ILIKE '%' || sqlc.narg('search') || '%'
            OR address ILIKE '%' || sqlc.narg('search') || '%'
        )
        AND (
            sqlc.narg('start_date')::timestamptz IS NULL
            OR created_at >= sqlc.narg('start_date')
        )
        AND (
            sqlc.narg('end_date')::timestamptz IS NULL
            OR created_at <= sqlc.narg('end_date')
        )
    GROUP BY email, address, site, status
)
SELECT
    p.id,
    p.payment_id,
    p.address,
    p.site,
    p.tx_hash,
    p.amount_btc,
    p.amount_usd,
    p.currency,
    p.confirmations,
    p.required_confirmations,
    p.status,
    p.email,
    p.order_id,
    p.user_agent,
    p.ip_address,
    p.payment_initiated_at,
    p.first_seen_at,
    p.confirmed_at,
    p.completed_at,
    p.expires_at,
    p.notes,
    p.webhook_sent,
    p.webhook_sent_at,
    p.email_sent,
    p.email_sent_at,
    p.telegram_sent,
    p.telegram_sent_at,
    p.created_at,
    p.updated_at,
    gp.generation_count::integer as generation_count,
    gp.first_created_at
FROM grouped_payments gp
JOIN payments p ON p.id = gp.latest_payment_id
ORDER BY gp.latest_created_at DESC
LIMIT sqlc.arg('limit')
OFFSET sqlc.arg('offset');

-- name: CountPaymentsGroupedByEmailAddress :one
-- Count unique email+address combinations for pagination
SELECT COUNT(*) FROM (
    SELECT DISTINCT email, address, site, status
    FROM payments
    WHERE
        (sqlc.narg('site')::text IS NULL OR site = sqlc.narg('site'))
        AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
        AND (
            sqlc.narg('search')::text IS NULL
            OR email ILIKE '%' || sqlc.narg('search') || '%'
            OR address ILIKE '%' || sqlc.narg('search') || '%'
        )
        AND (
            sqlc.narg('start_date')::timestamptz IS NULL
            OR created_at >= sqlc.narg('start_date')
        )
        AND (
            sqlc.narg('end_date')::timestamptz IS NULL
            OR created_at <= sqlc.narg('end_date')
        )
) AS unique_groups;
