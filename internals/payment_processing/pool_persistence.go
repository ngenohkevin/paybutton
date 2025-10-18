package payment_processing

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ngenohkevin/paybutton/internals/database"
	dbgen "github.com/ngenohkevin/paybutton/internals/db"
)

// PoolPersistence handles database operations for address pool
type PoolPersistence struct {
	queries *dbgen.Queries
	enabled bool
}

// NewPoolPersistence creates a new persistence layer
func NewPoolPersistence() *PoolPersistence {
	if !database.IsEnabled() {
		log.Println("⚠️ Pool persistence disabled - running in memory-only mode")
		return &PoolPersistence{enabled: false}
	}

	if database.Queries == nil {
		log.Println("⚠️ Database not initialized - pool persistence disabled")
		return &PoolPersistence{enabled: false}
	}

	return &PoolPersistence{
		queries: database.Queries,
		enabled: true,
	}
}

// IsEnabled returns whether persistence is active
func (p *PoolPersistence) IsEnabled() bool {
	return p.enabled
}

// SavePoolState saves the next index for a site
func (p *PoolPersistence) SavePoolState(ctx context.Context, site string, nextIndex int) error {
	if !p.enabled {
		return nil
	}

	err := p.queries.UpsertPoolState(ctx, dbgen.UpsertPoolStateParams{
		Site:      site,
		NextIndex: int32(nextIndex),
	})

	if err != nil {
		log.Printf("❌ Failed to save pool state for %s: %v", site, err)
		return err
	}

	return nil
}

// LoadPoolState loads the next index for a site
func (p *PoolPersistence) LoadPoolState(ctx context.Context, site string) (int, error) {
	if !p.enabled {
		return SiteRegistry[site].StartIndex, nil
	}

	state, err := p.queries.GetPoolState(ctx, site)
	if err != nil {
		if err == sql.ErrNoRows {
			// First time, return start index
			return SiteRegistry[site].StartIndex, nil
		}
		return 0, err
	}

	return int(state.NextIndex), nil
}

// SaveAddress saves or updates an address in the database
func (p *PoolPersistence) SaveAddress(ctx context.Context, addr *PooledAddress) error {
	if !p.enabled {
		return nil
	}

	// Convert time to pgtype.Timestamptz
	var reservedAt pgtype.Timestamptz
	if !addr.ReservedAt.IsZero() {
		reservedAt = pgtype.Timestamptz{
			Time:  addr.ReservedAt,
			Valid: true,
		}
	}

	// Convert email to pointer
	var email *string
	if addr.Email != "" {
		email = &addr.Email
	}

	// Convert payment count to pointer
	paymentCount := int32(addr.PaymentCount)
	var paymentCountPtr *int32
	if paymentCount > 0 {
		paymentCountPtr = &paymentCount
	}

	// Convert balance and tx count to pointers (always 0 for now)
	var balanceSats *int64
	var txCount *int32

	// Try to create first (for new addresses)
	_, err := p.queries.CreateAddress(ctx, dbgen.CreateAddressParams{
		Site:         addr.Site,
		Address:      addr.Address,
		AddressIndex: int32(addr.Index),
		Status:       string(addr.Status),
		Email:        email,
		ReservedAt:   reservedAt,
		LastChecked:  addr.LastChecked,
		PaymentCount: paymentCountPtr,
		BalanceSats:  balanceSats,
		TxCount:      txCount,
	})

	if err != nil {
		// If address already exists (duplicate key error), update it instead
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			// Address already exists, update the reservation (but NOT site to avoid constraint violations)
			// Note: Addresses keep their original site when used by different sites from global pool
			updateErr := p.queries.UpdateAddressSiteAndReservation(ctx, dbgen.UpdateAddressSiteAndReservationParams{
				Address:    addr.Address,
				Email:      email,
				ReservedAt: reservedAt,
			})
			if updateErr != nil {
				log.Printf("❌ Failed to update address %s reservation: %v", addr.Address, updateErr)
				return updateErr
			}
			log.Printf("✅ Updated address %s reservation: email='%s' (site remains '%s')", addr.Address, addr.Email, addr.Site)
			// Success - address was updated
			return nil
		}
		// Different error, log and return
		log.Printf("❌ Failed to save address %s: %v", addr.Address, err)
		return err
	}

	return nil
}

// GetAddressByAddress retrieves a single address by its address string
func (p *PoolPersistence) GetAddressByAddress(ctx context.Context, address string) (*PooledAddress, error) {
	if !p.enabled {
		return nil, nil
	}

	addr, err := p.queries.GetAddress(ctx, address)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	poolAddr := &PooledAddress{
		Site:        addr.Site,
		Address:     addr.Address,
		Index:       int(addr.AddressIndex),
		Status:      AddressStatus(addr.Status),
		LastChecked: addr.LastChecked,
	}

	// Handle nullable fields
	if addr.Email != nil {
		poolAddr.Email = *addr.Email
	}

	if addr.PaymentCount != nil {
		poolAddr.PaymentCount = int(*addr.PaymentCount)
	}

	if addr.ReservedAt.Valid {
		poolAddr.ReservedAt = addr.ReservedAt.Time
	}

	return poolAddr, nil
}

// LoadAllAddresses loads all addresses for a site
func (p *PoolPersistence) LoadAllAddresses(ctx context.Context, site string) ([]*PooledAddress, error) {
	if !p.enabled {
		return nil, nil
	}

	addresses, err := p.queries.ListAddressesBySite(ctx, site)
	if err != nil {
		return nil, err
	}

	result := make([]*PooledAddress, 0, len(addresses))
	for _, addr := range addresses {
		poolAddr := &PooledAddress{
			Site:        addr.Site,
			Address:     addr.Address,
			Index:       int(addr.AddressIndex),
			Status:      AddressStatus(addr.Status),
			LastChecked: addr.LastChecked,
		}

		// Handle nullable fields
		if addr.Email != nil {
			poolAddr.Email = *addr.Email
		}

		if addr.PaymentCount != nil {
			poolAddr.PaymentCount = int(*addr.PaymentCount)
		}

		if addr.ReservedAt.Valid {
			poolAddr.ReservedAt = addr.ReservedAt.Time
		}

		result = append(result, poolAddr)
	}

	return result, nil
}

// UpdateAddressStatus updates the status of an address
func (p *PoolPersistence) UpdateAddressStatus(ctx context.Context, address string, status AddressStatus) error {
	if !p.enabled {
		return nil
	}

	return p.queries.UpdateAddressStatus(ctx, dbgen.UpdateAddressStatusParams{
		Address: address,
		Status:  string(status),
	})
}

// MarkAddressUsed marks an address as used (payment received)
func (p *PoolPersistence) MarkAddressUsed(ctx context.Context, address string) error {
	if !p.enabled {
		return nil
	}

	return p.queries.MarkAddressUsed(ctx, address)
}

// MarkAddressUsedWithSite marks an address as used WITHOUT changing its site field
// This prevents constraint violations when addresses are used cross-site via global pool
// The site parameter is kept for logging/tracking purposes only
func (p *PoolPersistence) MarkAddressUsedWithSite(ctx context.Context, address string, site string) error {
	if !p.enabled {
		return nil
	}

	// Note: site parameter not passed to DB - addresses keep their original site
	return p.queries.MarkAddressUsedWithSite(ctx, address)
}

// AddToQueue adds an address to the available queue
func (p *PoolPersistence) AddToQueue(ctx context.Context, site, address string) error {
	if !p.enabled {
		return nil
	}

	return p.queries.AddToQueue(ctx, dbgen.AddToQueueParams{
		Site:    site,
		Address: address,
	})
}

// RemoveFromQueue removes an address from the queue
func (p *PoolPersistence) RemoveFromQueue(ctx context.Context, site, address string) error {
	if !p.enabled {
		return nil
	}

	return p.queries.RemoveFromQueue(ctx, dbgen.RemoveFromQueueParams{
		Site:    site,
		Address: address,
	})
}

// GetAvailableQueue gets all available addresses for a site
func (p *PoolPersistence) GetAvailableQueue(ctx context.Context, site string) ([]string, error) {
	if !p.enabled {
		return nil, nil
	}

	queue, err := p.queries.GetAvailableQueue(ctx, site)
	if err != nil {
		return nil, err
	}

	addresses := make([]string, len(queue))
	for i, item := range queue {
		addresses[i] = item.Address
	}

	return addresses, nil
}

// GetExpiredReservations gets all expired reservations for a site
func (p *PoolPersistence) GetExpiredReservations(ctx context.Context, site string) ([]*PooledAddress, error) {
	if !p.enabled {
		return nil, nil
	}

	expired, err := p.queries.GetExpiredReservationsBySite(ctx, site)
	if err != nil {
		return nil, err
	}

	result := make([]*PooledAddress, 0, len(expired))
	for _, addr := range expired {
		poolAddr := &PooledAddress{
			Site:        addr.Site,
			Address:     addr.Address,
			Index:       int(addr.AddressIndex),
			Status:      AddressStatus(addr.Status),
			LastChecked: addr.LastChecked,
		}

		if addr.Email != nil {
			poolAddr.Email = *addr.Email
		}

		if addr.PaymentCount != nil {
			poolAddr.PaymentCount = int(*addr.PaymentCount)
		}

		if addr.ReservedAt.Valid {
			poolAddr.ReservedAt = addr.ReservedAt.Time
		}

		result = append(result, poolAddr)
	}

	return result, nil
}

// GetPoolStats gets statistics for a site's pool
func (p *PoolPersistence) GetPoolStats(ctx context.Context, site string) (map[string]int64, error) {
	if !p.enabled {
		return nil, nil
	}

	stats, err := p.queries.GetPoolStats(ctx, site)
	if err != nil {
		return nil, err
	}

	return map[string]int64{
		"total":     stats.TotalAddresses,
		"available": stats.AvailableCount,
		"reserved":  stats.ReservedCount,
		"used":      stats.UsedCount,
		"skipped":   stats.SkippedCount,
		"expired":   stats.ExpiredCount,
	}, nil
}

// UpdateAddressReservation updates reservation details
func (p *PoolPersistence) UpdateAddressReservation(ctx context.Context, address, email string, reservedAt time.Time) error {
	if !p.enabled {
		return nil
	}

	emailPtr := &email

	return p.queries.UpdateAddressReservation(ctx, dbgen.UpdateAddressReservationParams{
		Address: address,
		Email:   emailPtr,
		ReservedAt: pgtype.Timestamptz{
			Time:  reservedAt,
			Valid: true,
		},
	})
}

// UpdateAddressBalance updates balance and transaction count
func (p *PoolPersistence) UpdateAddressBalance(ctx context.Context, address string, balanceSats int64, txCount int) error {
	if !p.enabled {
		return nil
	}

	balancePtr := &balanceSats
	txCountInt32 := int32(txCount)
	txCountPtr := &txCountInt32

	return p.queries.UpdateAddressBalance(ctx, dbgen.UpdateAddressBalanceParams{
		Address:     address,
		BalanceSats: balancePtr,
		TxCount:     txCountPtr,
	})
}

// GetRecentAddressActivity returns recent address activity for a site
func (p *PoolPersistence) GetRecentAddressActivity(ctx context.Context, site string) ([]dbgen.GetRecentAddressActivityRow, error) {
	if !p.enabled {
		return nil, nil
	}

	return p.queries.GetRecentAddressActivity(ctx, site)
}

// GetRecyclingStats returns recycling and reuse statistics for a site
func (p *PoolPersistence) GetRecyclingStats(ctx context.Context, site string) (*dbgen.GetRecyclingStatsRow, error) {
	if !p.enabled {
		return nil, nil
	}

	stats, err := p.queries.GetRecyclingStats(ctx, site)
	if err != nil {
		return nil, err
	}

	return &stats, nil
}
