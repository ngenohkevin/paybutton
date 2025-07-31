package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type BalanceResponse struct {
	Addr        string `json:"addr"`
	Confirmed   int64  `json:"confirmed"`
	Unconfirmed int64  `json:"unconfirmed"`
}

func GetBitcoinAddressBalanceWithBlockonomics(address string) (int64, error) {
	url := "https://www.blockonomics.co/api/balance"

	// Wait for rate limiter permission
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	rateLimiter := GetRateLimiter()
	if err := rateLimiter.WaitForPermission(ctx, "blockonomics"); err != nil {
		return 0, fmt.Errorf("rate limiter timeout: %w", err)
	}

	retries := 3
	baseDelay := 2 * time.Second

	for attempt := 0; attempt < retries; attempt++ {
		data := map[string]interface{}{
			"addr": []string{address},
		}

		payload, err := json.Marshal(data)
		if err != nil {
			return 0, err
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
		if err != nil {
			return 0, err
		}

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", blockonomicsAPIKey))
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClientInstance.client.Do(req)
		if err != nil {
			if attempt == retries-1 {
				return 0, fmt.Errorf("failed to fetch balance after %d attempts: %w", retries, err)
			}
			time.Sleep(baseDelay * time.Duration(1<<attempt)) // Exponential backoff: 2s, 4s, 8s
			continue
		}

		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				return
			}
		}(resp.Body)

		// Handle rate limiting and temporary errors with retry
		if resp.StatusCode == 429 || resp.StatusCode == 503 {
			if attempt == retries-1 {
				body, _ := io.ReadAll(resp.Body)
				return 0, fmt.Errorf("error fetching balance, status code: %v, response: %s", resp.StatusCode, body)
			}
			// Exponential backoff for rate limits: 2s, 4s, 8s
			time.Sleep(baseDelay * time.Duration(1<<attempt))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return 0, fmt.Errorf("error fetching balance, status code: %v, response: %s", resp.StatusCode, body)
		}

		var balanceResponse struct {
			Response []BalanceResponse `json:"response"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&balanceResponse); err != nil {
			return 0, err
		}

		if len(balanceResponse.Response) == 0 {
			return 0, fmt.Errorf("no balance data returned")
		}

		// Sum confirmed and unconfirmed balances
		totalBalance := balanceResponse.Response[0].Confirmed + balanceResponse.Response[0].Unconfirmed

		return totalBalance, nil
	}

	return 0, fmt.Errorf("all retry attempts failed")
}
