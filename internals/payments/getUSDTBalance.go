package payments

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// TronScanResponse matches the actual structure returned by the Tron API
type TronScanResponse struct {
	Data []struct {
		TokenID         string  `json:"tokenId"`
		Balance         string  `json:"balance"`
		TokenName       string  `json:"tokenName"`
		TokenAbbr       string  `json:"tokenAbbr"`
		TokenDecimal    int     `json:"tokenDecimal"`
		TokenCanShow    int     `json:"tokenCanShow"`
		TokenType       string  `json:"tokenType"`
		TokenLogo       string  `json:"tokenLogo,omitempty"`
		VIP             bool    `json:"vip"`
		TokenPriceInUsd float64 `json:"tokenPriceInUsd,omitempty"`
	} `json:"data"`
}

// TronGridResponse Alternate response structure for Tron API
type TronGridResponse struct {
	Data []struct {
		TokenID      string `json:"contract_address"`
		Balance      string `json:"balance"`
		TokenName    string `json:"name"`
		TokenAbbr    string `json:"symbol"`
		TokenDecimal int    `json:"decimals"`
	} `json:"data"`
}

// USDT TRC20 token ID on Tron network
const usdtTokenID = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"

// Default USDT address to use as fallback
const defaultUSDTAddress = "TBpAXWEGD8LPpx58Fjsu1ejSMJhgDUBNZK"

// IsValidTronAddress checks if an address has a valid TRON format
func IsValidTronAddress(address string) bool {
	// Basic validation for Tron addresses
	return len(address) == 34 && strings.HasPrefix(address, "T")
}

func GetUSDTBalance(address string) (float64, error) {
	// First validate the address format
	if !IsValidTronAddress(address) {
		return 0, fmt.Errorf("invalid Tron address format: %s", address)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Get API keys from environment
	tronscanApiKey := os.Getenv("TRONSCAN_API")
	tronGridApiKey := os.Getenv("TONGRID_API_KEY")

	// Try multiple API endpoints for redundancy
	endpoints := []string{
		fmt.Sprintf("https://apilist.tronscanapi.com/api/account/tokens?address=%s&type=trc20", address),
		fmt.Sprintf("https://api.trongrid.io/v1/accounts/%s/tokens?only_trc20=true", address),
		fmt.Sprintf("https://apilist.tronscan.org/api/account?address=%s", address),
	}

	// new http client with timeout
	client := &http.Client{
		Timeout: time.Second * 10,
	}

	var lastErr error
	// Try each endpoint until one works
	for _, url := range endpoints {
		// Only log failures at debug level, not every attempt
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			logger.Error("Failed to create request", "error", err, "url", url)
			lastErr = err
			continue
		}

		// Add API key header if we have one and it's a TronScan API
		if tronscanApiKey != "" && (strings.Contains(url, "tronscanapi.com") || strings.Contains(url, "tronscan.org")) {
			req.Header.Add("TRON-PRO-API-KEY", tronscanApiKey)
		}

		// For TronGrid API, we need a different API key format
		if strings.Contains(url, "trongrid.io") {
			if tronGridApiKey != "" {
				req.Header.Add("TRON-PRO-API-KEY", tronGridApiKey)
			}
		}

		//http request
		resp, err := client.Do(req)
		if err != nil {
			logger.Error("Failed to make HTTP request", "error", err, "url", url)
			lastErr = err
			continue
		}

		// validate response
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			// 404 Not Found is common for addresses with no tokens - log as debug instead of error
			if resp.StatusCode == http.StatusNotFound {
				logger.Debug("Address has no tokens (404)", "Status", resp.Status, "url", url)
			} else {
				logger.Error("Bad response code", "Status", resp.Status, "url", url)
			}
			lastErr = fmt.Errorf("bad response code: %s", resp.Status)
			continue
		}

		// decode response
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			logger.Error("Failed to read response body", "error", err, "url", url)
			lastErr = err
			continue
		}

		// Log raw response for debugging
		logger.Debug("Raw API response", "body", string(body))

		// Attempt to parse the response based on which API we're using
		if isFromTronscan(url) {
			var response TronScanResponse
			if err := json.Unmarshal(body, &response); err != nil {
				logger.Error("Failed to parse Tronscan JSON response", "error", err)
				lastErr = err
				continue
			}

			// Search for USDT token in data
			for _, token := range response.Data {
				if token.TokenID == usdtTokenID {
					//convert balance to float64
					balanceFloat, err := strconv.ParseFloat(token.Balance, 64)
					if err != nil {
						logger.Error("Failed to parse balance to float64", "error", err)
						lastErr = err
						continue
					}

					//Apply decimal places (USDT uses 6 decimals on Tron)
					tokenDecimal := token.TokenDecimal
					if tokenDecimal == 0 {
						tokenDecimal = 6 // Default decimal places for USDT on TRON
					}
					balance := balanceFloat / math.Pow10(tokenDecimal)
					logger.Info("USDT balance retrieved", "address", address, "balance", balance)
					return balance, nil
				}
			}
		} else {
			// Try parsing as TronGrid response
			var response TronGridResponse
			if err := json.Unmarshal(body, &response); err != nil {
				logger.Error("Failed to parse TronGrid JSON response", "error", err)
				lastErr = err
				continue
			}

			// Search for USDT token in data
			for _, token := range response.Data {
				if token.TokenID == usdtTokenID {
					//convert balance to float64
					balanceFloat, err := strconv.ParseFloat(token.Balance, 64)
					if err != nil {
						logger.Error("Failed to parse balance to float64", "error", err)
						lastErr = err
						continue
					}

					//Apply decimal places
					tokenDecimal := token.TokenDecimal
					if tokenDecimal == 0 {
						tokenDecimal = 6 // Default decimal places for USDT on TRON
					}
					balance := balanceFloat / math.Pow10(tokenDecimal)
					logger.Info("USDT balance retrieved", "address", address, "balance", balance)
					return balance, nil
				}
			}
		}

		// If we got here, we didn't find USDT in the response
		// No logging needed for this common case
	}

	// Fallback to hard-coded API request with manual JSON parsing
	balance, err := getUSDTBalanceWithFallback(address, client, logger)
	if err == nil && balance > 0 {
		return balance, nil
	}

	// At this point, all endpoints failed but we'll handle certain errors differently
	if lastErr != nil {
		// Check if it's a network/DNS error - this is a real error we should report
		if strings.Contains(lastErr.Error(), "dial tcp") ||
			strings.Contains(lastErr.Error(), "no such host") {
			logger.Error("Network connection error", "error", lastErr)
			return 0, lastErr
		}

		// For other errors (like 404 Not Found), we'll assume the address is valid
		// but it simply doesn't have any USDT tokens yet
		return 0, nil
	}

	// No error but also no balance found - address is valid but has no USDT
	return 0, nil
}

func isFromTronscan(url string) bool {
	return strings.Contains(url, "tronscanapi.com")
}

func getUSDTBalanceWithFallback(address string, client *http.Client, logger *slog.Logger) (float64, error) {
	// This is a more direct approach to get USDT balance using an alternative API
	url := fmt.Sprintf("https://apilist.tronscan.org/api/account?address=%s", address)

	// Create request with authentication headers
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.Error("Fallback: Failed to create request", "error", err, "url", url)
		return 0, fmt.Errorf("fallback API request creation failed: %w", err)
	}

	// Add API key header if we have one
	tronscanApiKey := os.Getenv("TRONSCAN_API")
	if tronscanApiKey != "" && strings.Contains(url, "tronscan.org") {
		req.Header.Add("TRON-PRO-API-KEY", tronscanApiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Fallback: Failed to make HTTP request", "error", err, "url", url)
		return 0, fmt.Errorf("fallback API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		logger.Error("Fallback: Bad response code", "Status", resp.Status, "url", url)
		return 0, fmt.Errorf("fallback API returned error status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Fallback: Failed to read response body", "error", err)
		return 0, fmt.Errorf("fallback API could not read body: %w", err)
	}

	// Log the response for debugging
	logger.Debug("Fallback raw API response", "body", string(body))

	// Parse the raw JSON to find the USDT token
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		logger.Error("Fallback: Failed to parse JSON response", "error", err)
		return 0, fmt.Errorf("fallback API JSON parsing failed: %w", err)
	}

	// Check if trc20token_balances exists
	tokens, ok := data["trc20token_balances"].([]interface{})
	if !ok {
		// Try alternative field names that might be used by the API
		for _, fieldName := range []string{"trc20", "tokens", "trc20_tokens"} {
			if tokensCheck, hasField := data[fieldName].([]interface{}); hasField {
				tokens = tokensCheck
				ok = true
				break
			}
		}

		if !ok {
			logger.Error("Fallback: No token balances found in response", "data_keys", fmt.Sprintf("%v", getMapKeys(data)))
			// Dump the top-level structure for debugging
			logger.Error("Fallback: Response structure", "data", fmt.Sprintf("%+v", data))
			return 0, fmt.Errorf("no token balance data found in fallback API response")
		}
	}

	// Look for the USDT token
	for _, t := range tokens {
		token, ok := t.(map[string]interface{})
		if !ok {
			continue
		}

		// Try different possible key names for token ID
		var tokenID string
		for _, key := range []string{"tokenId", "contract_address", "token_id", "address"} {
			if id, hasKey := token[key].(string); hasKey {
				tokenID = id
				break
			}
		}

		if tokenID != usdtTokenID {
			continue
		}

		// Try different possible key names for balance
		var balance string
		for _, key := range []string{"balance", "value", "amount"} {
			if bal, hasKey := token[key]; hasKey {
				switch v := bal.(type) {
				case string:
					balance = v
				case float64:
					balance = fmt.Sprintf("%f", v)
				case int:
					balance = fmt.Sprintf("%d", v)
				}
				if balance != "" {
					break
				}
			}
		}

		if balance == "" {
			logger.Error("Fallback: Balance not found for USDT token", "token_keys", fmt.Sprintf("%v", getMapKeys(token)))
			return 0, fmt.Errorf("balance not found for USDT token in fallback API")
		}

		balanceFloat, err := strconv.ParseFloat(balance, 64)
		if err != nil {
			logger.Error("Fallback: Failed to parse balance string", "balance", balance, "error", err)
			return 0, fmt.Errorf("couldn't parse balance '%s': %w", balance, err)
		}

		// Try different possible key names for decimals
		var tokenDecimal float64 = 6 // Default for USDT on TRON
		for _, key := range []string{"tokenDecimal", "decimals", "decimal"} {
			if dec, hasKey := token[key]; hasKey {
				switch v := dec.(type) {
				case float64:
					tokenDecimal = v
				case int:
					tokenDecimal = float64(v)
				case string:
					if parsed, err := strconv.ParseFloat(v, 64); err == nil {
						tokenDecimal = parsed
					}
				}
				break
			}
		}

		finalBalance := balanceFloat / math.Pow10(int(tokenDecimal))
		logger.Info("Fallback: USDT balance found", "address", address, "balance", finalBalance)
		return finalBalance, nil
	}

	// If we get here and still don't have a balance, return 0 with no error
	// This likely means the address is valid but has no USDT tokens yet
	logger.Info("No USDT balance found for this specific address after trying all endpoints", "address", address)
	return 0, nil
}

// Helper function to get keys from a map for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
