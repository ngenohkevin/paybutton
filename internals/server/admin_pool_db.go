package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ngenohkevin/paybutton/internals/database"
	"github.com/ngenohkevin/paybutton/internals/db"
)

// PoolStatsDB represents database-backed pool statistics
type PoolStatsDB struct {
	// Aggregate counts
	TotalAddresses         int64 `json:"total_addresses"`
	AvailableCount         int64 `json:"available_count"`
	ReservedCount          int64 `json:"reserved_count"`
	UsedCount              int64 `json:"used_count"`
	SkippedCount           int64 `json:"skipped_count"`
	ExpiredCount           int64 `json:"expired_count"`
	ReusedAddresses        int64 `json:"reused_addresses"`
	RecycledAddresses      int64 `json:"recycled_addresses"`
	TotalPaymentsProcessed int64 `json:"total_payments_processed"`

	// Current pool size (available + reserved)
	CurrentPoolSize int `json:"current_pool_size"`

	// Metrics for compatibility with in-memory PoolStats
	TotalGenerated int `json:"total_generated"`
	TotalUsed      int `json:"total_used"`
	TotalRecycled  int `json:"total_recycled"`
}

// SitePoolStatsDB represents pool statistics for a specific site
type SitePoolStatsDB struct {
	Site               string `json:"site"`
	TotalAddresses     int64  `json:"total_addresses"`
	AvailableCount     int64  `json:"available_count"`
	ReservedCount      int64  `json:"reserved_count"`
	UsedCount          int64  `json:"used_count"`
	SkippedCount       int64  `json:"skipped_count"`
	ExpiredCount       int64  `json:"expired_count"`
	CurrentPoolSize    int    `json:"current_pool_size"`
}

// GetPoolStatsFromDB retrieves pool statistics from the database
func GetPoolStatsFromDB(ctx context.Context) (*PoolStatsDB, error) {
	if database.Queries == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Get aggregated stats
	stats, err := database.Queries.GetAllPoolStats(ctx)
	if err != nil {
		log.Printf("Error fetching pool stats from database: %v", err)
		return nil, fmt.Errorf("failed to fetch pool stats: %w", err)
	}

	// Convert interface{} to int64 for total_payments_processed
	totalPayments := int64(0)
	if stats.TotalPaymentsProcessed != nil {
		switch v := stats.TotalPaymentsProcessed.(type) {
		case int64:
			totalPayments = v
		case int:
			totalPayments = int64(v)
		case float64:
			totalPayments = int64(v)
		}
	}

	poolStats := &PoolStatsDB{
		TotalAddresses:         stats.TotalAddresses,
		AvailableCount:         stats.AvailableCount,
		ReservedCount:          stats.ReservedCount,
		UsedCount:              stats.UsedCount,
		SkippedCount:           stats.SkippedCount,
		ExpiredCount:           stats.ExpiredCount,
		ReusedAddresses:        stats.ReusedAddresses,
		RecycledAddresses:      stats.RecycledAddresses,
		TotalPaymentsProcessed: totalPayments,
		CurrentPoolSize:        int(stats.AvailableCount + stats.ReservedCount),
		TotalGenerated:         int(stats.TotalAddresses),
		TotalUsed:              int(stats.UsedCount),
		TotalRecycled:          int(stats.RecycledAddresses),
	}

	return poolStats, nil
}

// GetSitePoolStatsFromDB retrieves pool statistics grouped by site
func GetSitePoolStatsFromDB(ctx context.Context) ([]SitePoolStatsDB, error) {
	if database.Queries == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	siteStats, err := database.Queries.GetAllSitePoolStats(ctx)
	if err != nil {
		log.Printf("Error fetching site pool stats from database: %v", err)
		return nil, fmt.Errorf("failed to fetch site pool stats: %w", err)
	}

	result := make([]SitePoolStatsDB, len(siteStats))
	for i, stat := range siteStats {
		result[i] = SitePoolStatsDB{
			Site:            stat.Site,
			TotalAddresses:  stat.TotalAddresses,
			AvailableCount:  stat.AvailableCount,
			ReservedCount:   stat.ReservedCount,
			UsedCount:       stat.UsedCount,
			SkippedCount:    stat.SkippedCount,
			ExpiredCount:    stat.ExpiredCount,
			CurrentPoolSize: int(stat.AvailableCount + stat.ReservedCount),
		}
	}

	return result, nil
}

// GetDetailedPoolInfoFromDB retrieves detailed pool information from the database
func GetDetailedPoolInfoFromDB(ctx context.Context, site string, limit int) (map[string]interface{}, error) {
	if database.Queries == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := make(map[string]interface{})

	// Get available addresses
	availableAddrs, err := database.Queries.ListAddressesBySiteAndStatus(ctx, db.ListAddressesBySiteAndStatusParams{
		Site:   site,
		Status: "available",
	})
	if err != nil {
		log.Printf("Error fetching available addresses: %v", err)
	} else {
		available := make([]map[string]interface{}, 0, len(availableAddrs))
		for i, addr := range availableAddrs {
			if limit > 0 && i >= limit {
				break
			}
			available = append(available, map[string]interface{}{
				"address":       addr.Address,
				"address_index": addr.AddressIndex,
				"created_at":    addr.CreatedAt,
				"last_checked":  addr.LastChecked,
				"payment_count": addr.PaymentCount,
			})
		}
		result["available"] = available
		result["available_count"] = len(availableAddrs)
	}

	// Get reserved addresses
	reservedAddrs, err := database.Queries.ListAddressesBySiteAndStatus(ctx, db.ListAddressesBySiteAndStatusParams{
		Site:   site,
		Status: "reserved",
	})
	if err != nil {
		log.Printf("Error fetching reserved addresses: %v", err)
	} else {
		reserved := make([]map[string]interface{}, 0, len(reservedAddrs))
		for i, addr := range reservedAddrs {
			if limit > 0 && i >= limit {
				break
			}

			var reservedAt *time.Time
			if addr.ReservedAt.Valid {
				t := addr.ReservedAt.Time
				reservedAt = &t
			}

			var ageHours int
			if reservedAt != nil {
				ageHours = int(time.Since(*reservedAt).Hours())
			}

			reserved = append(reserved, map[string]interface{}{
				"address":       addr.Address,
				"address_index": addr.AddressIndex,
				"email":         addr.Email,
				"reserved_at":   reservedAt,
				"age_hours":     ageHours,
				"payment_count": addr.PaymentCount,
			})
		}
		result["reserved"] = reserved
		result["reserved_count"] = len(reservedAddrs)
	}

	// Get used addresses (limited sample)
	usedAddrs, err := database.Queries.ListAddressesBySiteAndStatus(ctx, db.ListAddressesBySiteAndStatusParams{
		Site:   site,
		Status: "used",
	})
	if err != nil {
		log.Printf("Error fetching used addresses: %v", err)
	} else {
		used := make([]map[string]interface{}, 0, len(usedAddrs))
		maxUsed := 10
		if limit > 0 && limit < maxUsed {
			maxUsed = limit
		}
		for i, addr := range usedAddrs {
			if i >= maxUsed {
				break
			}

			var usedAt *time.Time
			if addr.UsedAt.Valid {
				t := addr.UsedAt.Time
				usedAt = &t
			}

			used = append(used, map[string]interface{}{
				"address":       addr.Address,
				"address_index": addr.AddressIndex,
				"email":         addr.Email,
				"used_at":       usedAt,
				"payment_count": addr.PaymentCount,
			})
		}
		result["used"] = used
		result["used_count"] = len(usedAddrs)
	}

	// Get recycling stats
	recyclingStats, err := database.Queries.GetRecyclingStats(ctx, site)
	if err != nil {
		log.Printf("Error fetching recycling stats: %v", err)
	} else {
		result["recycling_stats"] = map[string]interface{}{
			"reused_addresses":         recyclingStats.ReusedAddresses,
			"recycled_addresses":       recyclingStats.RecycledAddresses,
			"recent_reservations":      recyclingStats.RecentReservations,
			"recent_payments":          recyclingStats.RecentPayments,
			"total_payments_processed": recyclingStats.TotalPaymentsProcessed,
			"max_reuse_count":          recyclingStats.MaxReuseCount,
		}
	}

	return result, nil
}

// GetDetailedPoolInfoAllSites retrieves detailed pool information across all sites
func GetDetailedPoolInfoAllSites(ctx context.Context, limit int) (map[string]interface{}, error) {
	if database.Queries == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := make(map[string]interface{})

	// Use the aggregate query to get stats across all sites
	stats, err := database.Queries.GetAllPoolStats(ctx)
	if err != nil {
		log.Printf("Error fetching pool stats: %v", err)
		return nil, fmt.Errorf("failed to fetch pool stats: %w", err)
	}

	// Set counts from aggregate stats
	result["available_count"] = stats.AvailableCount
	result["reserved_count"] = stats.ReservedCount
	result["used_count"] = stats.UsedCount

	// For now, return empty lists for the actual addresses since we're showing aggregate
	result["available"] = []map[string]interface{}{}
	result["reserved"] = []map[string]interface{}{}
	result["used"] = []map[string]interface{}{}

	// Get recycling stats (sum across all sites)
	result["recycling_stats"] = map[string]interface{}{
		"reused_addresses":    stats.ReusedAddresses,
		"recycled_addresses":  stats.RecycledAddresses,
		"total_payments_processed": stats.TotalPaymentsProcessed,
	}

	return result, nil
}

// GetRecentAddressActivityFromDB retrieves recent address activity from the database
func GetRecentAddressActivityFromDB(ctx context.Context, site string) ([]map[string]interface{}, error) {
	if database.Queries == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	activities, err := database.Queries.GetRecentAddressActivity(ctx, site)
	if err != nil {
		log.Printf("Error fetching recent address activity: %v", err)
		return nil, fmt.Errorf("failed to fetch recent activity: %w", err)
	}

	result := make([]map[string]interface{}, len(activities))
	for i, activity := range activities {
		var reservedAt *time.Time
		if activity.ReservedAt.Valid {
			t := activity.ReservedAt.Time
			reservedAt = &t
		}

		var usedAt *time.Time
		if activity.UsedAt.Valid {
			t := activity.UsedAt.Time
			usedAt = &t
		}

		result[i] = map[string]interface{}{
			"address":       activity.Address,
			"site":          activity.Site,
			"status":        activity.Status,
			"email":         activity.Email,
			"payment_count": activity.PaymentCount,
			"reserved_at":   reservedAt,
			"used_at":       usedAt,
			"last_checked":  activity.LastChecked,
			"created_at":    activity.CreatedAt,
			"activity_type": activity.ActivityType,
		}
	}

	return result, nil
}
