package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type BlockCypherBalance struct {
	Address            string `json:"address"`
	TotalReceived      int64  `json:"total_received"`
	TotalSent          int64  `json:"total_sent"`
	Balance            int64  `json:"balance"`
	UnconfirmedBalance int64  `json:"unconfirmed_balance"`
	FinalBalance       int64  `json:"final_balance"`
}

type BlockChainBalanceResponse struct {
	Address       string `json:"address"`
	FinalBalance  int64  `json:"final_balance"`
	TotalReceived int64  `json:"total_received"`
}

type BlockStreamResponse struct {
	Address    string `json:"address"`
	ChainStats struct {
		FundedTxoSum int64 `json:"funded_txo_sum"`
		SpentTxoSum  int64 `json:"spent_txo_sum"`
	} `json:"chain_stats"`
	MempoolStats struct {
		FundedTxoSum int64 `json:"funded_txo_sum"`
		SpentTxoSum  int64 `json:"spent_txo_sum"`
	} `json:"mempool_stats"`
}

type MempoolSpaceResponse struct {
	Address    string `json:"address"`
	ChainStats struct {
		FundedTxoSum  int64 `json:"funded_txo_sum"`
		SpentTxoSum   int64 `json:"spent_txo_sum"`
		TxCount       int   `json:"tx_count"`
		FundedTxoCount int  `json:"funded_txo_count"`
		SpentTxoCount  int  `json:"spent_txo_count"`
	} `json:"chain_stats"`
	MempoolStats struct {
		FundedTxoSum int64 `json:"funded_txo_sum"`
		SpentTxoSum  int64 `json:"spent_txo_sum"`
		TxCount      int   `json:"tx_count"`
	} `json:"mempool_stats"`
}

type BlockBookResponse struct {
	Address       string `json:"address"`
	Balance       string `json:"balance"`
	TotalReceived string `json:"totalReceived"`
}

func GetBitcoinAddressBalanceWithBlockCypher(address, token string) (int64, error) {
	url := fmt.Sprintf("https://api.blockcypher.com/v1/btc/main/addrs/%s/balance?token=%s", address, token)

	// Wait for rate limiter permission
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rateLimiter := GetRateLimiter()
	if err := rateLimiter.WaitForPermission(ctx, "blockcypher"); err != nil {
		return 0, fmt.Errorf("rate limiter timeout: %w", err)
	}

	var balanceResponse BlockCypherBalance
	var err error
	var resp *http.Response

	retries := 3
	for i := 0; i < retries; i++ {
		resp, err = http.Get(url)
		if err != nil {
			return 0, err
		}

		if resp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&balanceResponse); err != nil {
				err := resp.Body.Close()
				if err != nil {
					return 0, err
				}
				return 0, err
			}
			err := resp.Body.Close()
			if err != nil {
				return 0, err
			}
			break
		} else if resp.StatusCode == 403 || resp.StatusCode == 429 || resp.StatusCode == 503 {
			body, _ := io.ReadAll(resp.Body)
			err := resp.Body.Close()
			if err != nil {
				return 0, err
			}
			err = fmt.Errorf("error fetching balance, status code: %v, response: %s", resp.StatusCode, body)
			fmt.Println(err)
			if i == retries-1 {
				return 0, err
			}
			time.Sleep(time.Duration(2<<i) * time.Second) // Exponential backoff: 2, 4, 8 seconds
		} else {
			body, _ := io.ReadAll(resp.Body)
			err := resp.Body.Close()
			if err != nil {
				return 0, err
			}
			err = fmt.Errorf("error fetching balance, status code: %v, response: %s", resp.StatusCode, body)
			return 0, err
		}
	}

	totalBalance := balanceResponse.Balance + balanceResponse.UnconfirmedBalance
	return totalBalance, nil
}

func GetBitcoinAddressBalanceWithBlockChain(address string) (int64, error) {
	url := fmt.Sprintf("https://blockchain.info/rawaddr/%s", address)

	// Wait for rate limiter permission
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rateLimiter := GetRateLimiter()
	if err := rateLimiter.WaitForPermission(ctx, "blockchain"); err != nil {
		return 0, fmt.Errorf("rate limiter timeout: %w", err)
	}

	// Create an HTTP client with a timeout to prevent hanging
	client := &http.Client{Timeout: 15 * time.Second}

	retries := 3
	baseDelay := 2 * time.Second

	for attempt := 0; attempt < retries; attempt++ {
		// Create request with User-Agent to avoid blocking
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return 0, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("User-Agent", "PayButton/1.0 (Bitcoin Balance Checker)")

		resp, err := client.Do(req)
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

		// Check if the response status is OK
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return 0, fmt.Errorf("error fetching balance, status code: %v, response: %s", resp.StatusCode, body)
		}

		// Parse the JSON response
		var balanceResponse BlockChainBalanceResponse
		if err := json.NewDecoder(resp.Body).Decode(&balanceResponse); err != nil {
			return 0, fmt.Errorf("failed to parse balance response: %w", err)
		}

		// Return the final balance, including unconfirmed transactions
		return balanceResponse.FinalBalance, nil
	}

	return 0, fmt.Errorf("all retry attempts failed")
}

func GetBitcoinAddressBalanceWithBlockStream(address string) (int64, error) {
	url := fmt.Sprintf("https://blockstream.info/api/address/%s", address)

	// Wait for rate limiter permission
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rateLimiter := GetRateLimiter()
	if err := rateLimiter.WaitForPermission(ctx, "blockstream"); err != nil {
		return 0, fmt.Errorf("rate limiter timeout: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	retries := 3
	baseDelay := 2 * time.Second

	for attempt := 0; attempt < retries; attempt++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return 0, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("User-Agent", "PayButton/1.0 (Bitcoin Balance Checker)")

		resp, err := client.Do(req)
		if err != nil {
			if attempt == retries-1 {
				return 0, fmt.Errorf("failed to fetch balance after %d attempts: %w", retries, err)
			}
			time.Sleep(baseDelay * time.Duration(1<<attempt))
			continue
		}

		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				return
			}
		}(resp.Body)

		if resp.StatusCode == 429 || resp.StatusCode == 503 {
			if attempt == retries-1 {
				body, _ := io.ReadAll(resp.Body)
				return 0, fmt.Errorf("error fetching balance, status code: %v, response: %s", resp.StatusCode, body)
			}
			time.Sleep(baseDelay * time.Duration(1<<attempt))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return 0, fmt.Errorf("error fetching balance, status code: %v, response: %s", resp.StatusCode, body)
		}

		var balanceResponse BlockStreamResponse
		if err := json.NewDecoder(resp.Body).Decode(&balanceResponse); err != nil {
			return 0, fmt.Errorf("failed to parse balance response: %w", err)
		}

		// Calculate balance: (funded - spent) + mempool
		confirmedBalance := balanceResponse.ChainStats.FundedTxoSum - balanceResponse.ChainStats.SpentTxoSum
		mempoolBalance := balanceResponse.MempoolStats.FundedTxoSum - balanceResponse.MempoolStats.SpentTxoSum
		totalBalance := confirmedBalance + mempoolBalance

		return totalBalance, nil
	}

	return 0, fmt.Errorf("all retry attempts failed")
}

// CheckAddressHistoryWithMempoolSpace checks if an address has transaction history
// Returns: (balance, txCount, error)
func CheckAddressHistoryWithMempoolSpace(address string) (int64, int, error) {
	url := fmt.Sprintf("https://mempool.space/api/address/%s", address)

	// Wait for rate limiter permission
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rateLimiter := GetRateLimiter()
	if err := rateLimiter.WaitForPermission(ctx, "mempoolspace"); err != nil {
		return 0, 0, fmt.Errorf("rate limiter timeout: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	retries := 3
	baseDelay := 2 * time.Second

	for attempt := 0; attempt < retries; attempt++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("User-Agent", "PayButton/1.0 (Bitcoin Balance Checker)")

		resp, err := client.Do(req)
		if err != nil {
			if attempt == retries-1 {
				return 0, 0, fmt.Errorf("failed to fetch balance after %d attempts: %w", retries, err)
			}
			time.Sleep(baseDelay * time.Duration(1<<attempt))
			continue
		}

		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				return
			}
		}(resp.Body)

		if resp.StatusCode == 429 || resp.StatusCode == 503 {
			if attempt == retries-1 {
				body, _ := io.ReadAll(resp.Body)
				return 0, 0, fmt.Errorf("error fetching balance, status code: %v, response: %s", resp.StatusCode, body)
			}
			time.Sleep(baseDelay * time.Duration(1<<attempt))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return 0, 0, fmt.Errorf("error fetching balance, status code: %v, response: %s", resp.StatusCode, body)
		}

		var balanceResponse MempoolSpaceResponse
		if err := json.NewDecoder(resp.Body).Decode(&balanceResponse); err != nil {
			return 0, 0, fmt.Errorf("failed to parse balance response: %w", err)
		}

		// Calculate balance: (funded - spent) + mempool
		confirmedBalance := balanceResponse.ChainStats.FundedTxoSum - balanceResponse.ChainStats.SpentTxoSum
		mempoolBalance := balanceResponse.MempoolStats.FundedTxoSum - balanceResponse.MempoolStats.SpentTxoSum
		totalBalance := confirmedBalance + mempoolBalance

		// Get total transaction count (confirmed + mempool)
		totalTxCount := balanceResponse.ChainStats.TxCount + balanceResponse.MempoolStats.TxCount

		return totalBalance, totalTxCount, nil
	}

	return 0, 0, fmt.Errorf("all retry attempts failed")
}

// GetBitcoinAddressBalanceWithMempoolSpace returns only balance (for backward compatibility)
func GetBitcoinAddressBalanceWithMempoolSpace(address string) (int64, error) {
	balance, _, err := CheckAddressHistoryWithMempoolSpace(address)
	return balance, err
}

func GetBitcoinAddressBalanceWithTrezor(address string) (int64, error) {
	url := fmt.Sprintf("https://btc1.trezor.io/api/v2/address/%s", address)

	// Wait for rate limiter permission
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rateLimiter := GetRateLimiter()
	if err := rateLimiter.WaitForPermission(ctx, "trezor"); err != nil {
		return 0, fmt.Errorf("rate limiter timeout: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	retries := 3
	baseDelay := 2 * time.Second

	for attempt := 0; attempt < retries; attempt++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return 0, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("User-Agent", "PayButton/1.0 (Bitcoin Balance Checker)")

		resp, err := client.Do(req)
		if err != nil {
			if attempt == retries-1 {
				return 0, fmt.Errorf("failed to fetch balance after %d attempts: %w", retries, err)
			}
			time.Sleep(baseDelay * time.Duration(1<<attempt))
			continue
		}

		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				return
			}
		}(resp.Body)

		if resp.StatusCode == 429 || resp.StatusCode == 503 {
			if attempt == retries-1 {
				body, _ := io.ReadAll(resp.Body)
				return 0, fmt.Errorf("error fetching balance, status code: %v, response: %s", resp.StatusCode, body)
			}
			time.Sleep(baseDelay * time.Duration(1<<attempt))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return 0, fmt.Errorf("error fetching balance, status code: %v, response: %s", resp.StatusCode, body)
		}

		var balanceResponse BlockBookResponse
		if err := json.NewDecoder(resp.Body).Decode(&balanceResponse); err != nil {
			return 0, fmt.Errorf("failed to parse balance response: %w", err)
		}

		// Parse balance from string to int64 (BlockBook returns balance as string in satoshis)
		balance, err := strconv.ParseInt(balanceResponse.Balance, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse balance: %w", err)
		}

		return balance, nil
	}

	return 0, fmt.Errorf("all retry attempts failed")
}
