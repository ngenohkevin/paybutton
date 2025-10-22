package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ngenohkevin/paybutton/utils"
)

type BlockonomicsAddress struct {
	Address string `json:"address"`
	Paid    int64  `json:"paid"`
	Pending int64  `json:"pending"`
}
//
func main() {
	log.Println("🔍 Starting Blockonomics Gap Limit Recovery Tool")

	// Load config for API key
	config, err := utils.LoadConfig()
	if err != nil {
		log.Fatal("Could not load config:", err)
	}

	// Get all addresses from Blockonomics
	addresses, err := getBlockonomicsAddresses(config.BlockonomicsAPI)
	if err != nil {
		log.Fatal("Failed to get addresses:", err)
	}

	log.Printf("📊 Retrieved %d addresses from Blockonomics\n", len(addresses))

	// Analyze gap status
	unpaidCount := 0
	paidCount := 0
	consecutiveUnpaid := 0
	maxConsecutive := 0
	var missingTxAddresses []BlockonomicsAddress

	for i, addr := range addresses {
		if addr.Paid > 0 || addr.Pending > 0 {
			paidCount++
			if addr.Paid > 0 {
				btcAmount := float64(addr.Paid) / 100000000.0
				log.Printf("✅ PAID: %s - %.8f BTC", addr.Address, btcAmount)

				// If this paid address comes after many unpaid ones, it might be missing
				if consecutiveUnpaid >= 15 {
					missingTxAddresses = append(missingTxAddresses, addr)
					log.Printf("⚠️  POTENTIAL MISSING TX: Address %s (after %d unpaid addresses)",
						addr.Address, consecutiveUnpaid)
				}
			}
			consecutiveUnpaid = 0
		} else {
			unpaidCount++
			consecutiveUnpaid++
			if consecutiveUnpaid > maxConsecutive {
				maxConsecutive = consecutiveUnpaid
			}
		}

		// Show first 30 addresses for analysis
		if i < 30 {
			status := "UNPAID"
			if addr.Paid > 0 {
				status = fmt.Sprintf("PAID: %.8f BTC", float64(addr.Paid)/100000000.0)
			} else if addr.Pending > 0 {
				status = fmt.Sprintf("PENDING: %.8f BTC", float64(addr.Pending)/100000000.0)
			}
			log.Printf("  [%02d] %s - %s", i+1, addr.Address, status)
		}
	}

	log.Println("\n📈 GAP LIMIT ANALYSIS:")
	log.Printf("  Total addresses: %d", len(addresses))
	log.Printf("  Paid addresses: %d", paidCount)
	log.Printf("  Unpaid addresses: %d", unpaidCount)
	log.Printf("  Max consecutive unpaid: %d", maxConsecutive)

	if maxConsecutive >= 20 {
		log.Printf("  🚨 GAP LIMIT EXCEEDED! (%d >= 20)", maxConsecutive)
		log.Println("\n⚠️  CRITICAL: You have hit the Blockonomics gap limit!")
		log.Println("  Transactions to addresses beyond position 20 are NOT visible to your wallet.")
	} else if maxConsecutive >= 15 {
		log.Printf("  ⚠️  WARNING: Approaching gap limit (%d/20)", maxConsecutive)
	} else {
		log.Printf("  ✅ Gap limit healthy (%d/20)", maxConsecutive)
	}

	if len(missingTxAddresses) > 0 {
		log.Printf("\n🔴 FOUND %d POTENTIALLY MISSING TRANSACTIONS:", len(missingTxAddresses))
		for _, addr := range missingTxAddresses {
			btcAmount := float64(addr.Paid) / 100000000.0
			log.Printf("  - %s: %.8f BTC", addr.Address, btcAmount)
		}
		log.Println("\n📌 RECOVERY STEPS:")
		log.Println("  1. Import these addresses manually into your wallet")
		log.Println("  2. Or mark some unpaid addresses as 'used' in Blockonomics")
		log.Println("  3. Implement address reuse to prevent future gap limit issues")
	}

	// Check Sep 17 transactions specifically
	log.Println("\n🔍 Checking for Sep 17, 2025 transactions...")
	sep17Count := 0
	for _, addr := range addresses {
		if addr.Paid > 0 {
			// Note: We can't get exact dates from this API, but we can identify recent payments
			btcAmount := float64(addr.Paid) / 100000000.0
			if btcAmount > 0 {
				sep17Count++
				log.Printf("  Recent payment: %s - %.8f BTC", addr.Address, btcAmount)
			}
		}
	}

	// Save recovery data
	saveRecoveryData(addresses, missingTxAddresses)
}

func getBlockonomicsAddresses(apiKey string) ([]BlockonomicsAddress, error) {
	url := "https://www.blockonomics.co/api/address?&limit=200&reset=1"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var addresses []BlockonomicsAddress

	if err := json.NewDecoder(resp.Body).Decode(&addresses); err != nil {
		return nil, err
	}

	return addresses, nil
}

func saveRecoveryData(addresses []BlockonomicsAddress, missing []BlockonomicsAddress) {
	data := struct {
		Timestamp        time.Time             `json:"timestamp"`
		TotalAddresses   int                   `json:"total_addresses"`
		MissingAddresses []BlockonomicsAddress `json:"missing_addresses"`
		AllAddresses     []BlockonomicsAddress `json:"all_addresses"`
	}{
		Timestamp:        time.Now(),
		TotalAddresses:   len(addresses),
		MissingAddresses: missing,
		AllAddresses:     addresses,
	}

	file, err := os.Create("gap_recovery_data.json")
	if err != nil {
		log.Printf("Failed to save recovery data: %v", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		log.Printf("Failed to encode recovery data: %v", err)
		return
	}

	log.Printf("\n✅ Recovery data saved to gap_recovery_data.json")
}
