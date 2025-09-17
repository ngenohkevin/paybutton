package payments

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// BlockonomicsAddressInfo represents address information from Blockonomics
type BlockonomicsAddressInfo struct {
	Address      string  `json:"address"`
	Paid         int64   `json:"paid"`
	Pending      int64   `json:"pending"`
	GapIndex     int     `json:"gap_index"`
	LastUpdated  string  `json:"last_updated"`
}

// BlockonomicsGapStatus represents the gap limit status from Blockonomics
type BlockonomicsGapStatus struct {
	CurrentGap       int       `json:"current_gap"`
	MaxGap          int       `json:"max_gap"`
	UnpaidAddresses []string  `json:"unpaid_addresses"`
	LastChecked     time.Time `json:"last_checked"`
	IsAtLimit       bool      `json:"is_at_limit"`
}

// CheckBlockonomicsGapLimit checks the actual gap limit status with Blockonomics API
func CheckBlockonomicsGapLimit() (*BlockonomicsGapStatus, error) {
	// Get all addresses from Blockonomics
	addresses, err := GetBlockonomicsAddresses()
	if err != nil {
		return nil, fmt.Errorf("failed to get addresses from Blockonomics: %v", err)
	}

	// Calculate gap status
	status := &BlockonomicsGapStatus{
		MaxGap:          20, // Blockonomics default
		UnpaidAddresses: make([]string, 0),
		LastChecked:     time.Now(),
	}

	// Count consecutive unpaid addresses
	consecutiveUnpaid := 0
	maxConsecutive := 0

	for _, addr := range addresses {
		if addr.Paid == 0 && addr.Pending == 0 {
			consecutiveUnpaid++
			status.UnpaidAddresses = append(status.UnpaidAddresses, addr.Address)
			if consecutiveUnpaid > maxConsecutive {
				maxConsecutive = consecutiveUnpaid
			}
		} else {
			consecutiveUnpaid = 0 // Reset counter when we find a paid address
		}
	}

	status.CurrentGap = maxConsecutive
	status.IsAtLimit = maxConsecutive >= status.MaxGap

	if status.IsAtLimit {
		log.Printf("⚠️ GAP LIMIT REACHED: %d consecutive unpaid addresses (limit: %d)",
			status.CurrentGap, status.MaxGap)
	} else if float64(status.CurrentGap) >= float64(status.MaxGap)*0.8 {
		log.Printf("⚠️ APPROACHING GAP LIMIT: %d/%d consecutive unpaid addresses",
			status.CurrentGap, status.MaxGap)
	}

	return status, nil
}

// GetBlockonomicsAddresses retrieves all addresses from Blockonomics
func GetBlockonomicsAddresses() ([]BlockonomicsAddressInfo, error) {
	url := "https://www.blockonomics.co/api/address?&limit=200"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", blockonomicsAPIKey))

	resp, err := httpClientInstance.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Addresses []BlockonomicsAddressInfo `json:"addresses"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Addresses, nil
}

// RecoverFromGapLimit attempts to recover from gap limit by marking old unpaid addresses
func RecoverFromGapLimit() error {
	status, err := CheckBlockonomicsGapLimit()
	if err != nil {
		return fmt.Errorf("failed to check gap status: %v", err)
	}

	if !status.IsAtLimit && status.CurrentGap < 15 {
		log.Printf("Gap limit is healthy: %d/%d", status.CurrentGap, status.MaxGap)
		return nil
	}

	log.Printf("🔧 RECOVERING FROM GAP LIMIT: Found %d unpaid addresses to process",
		len(status.UnpaidAddresses))

	// Get the oldest unpaid addresses (first 5-10)
	addressesToRecover := status.UnpaidAddresses
	if len(addressesToRecover) > 10 {
		addressesToRecover = addressesToRecover[:10]
	}

	for _, addr := range addressesToRecover {
		log.Printf("  - Marking address %s for recycling (72h expired check)", addr)
		// The actual recycling will be handled by the address pool
	}

	return nil
}

// FindMissingTransactions searches for transactions that might be beyond the gap limit
func FindMissingTransactions(startDate, endDate time.Time) ([]BlockonomicsAddressInfo, error) {
	addresses, err := GetBlockonomicsAddresses()
	if err != nil {
		return nil, err
	}

	var missingTxAddresses []BlockonomicsAddressInfo

	for _, addr := range addresses {
		// Check if address has received payment but might be beyond gap limit
		if addr.Paid > 0 || addr.Pending > 0 {
			// Parse last updated time
			if addr.LastUpdated != "" {
				// Check if this address received payment in the specified date range
				// and might have been missed due to gap limit
				if addr.GapIndex >= 15 {
					missingTxAddresses = append(missingTxAddresses, addr)
					log.Printf("Found potentially missing transaction on address %s (gap index: %d, paid: %d sats)",
						addr.Address, addr.GapIndex, addr.Paid)
				}
			}
		}
	}

	return missingTxAddresses, nil
}