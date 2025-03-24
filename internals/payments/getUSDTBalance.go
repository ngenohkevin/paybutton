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
	"time"
)

type TokenBalance struct {
	Tokens []struct {
		TokenID      string `json:"tokenId"`
		Balance      string `json:"balance"`
		TokenDecimal int    `json:"tokenDecimal"`
	} `json:"tokens"`
}

func GetUSDTBalance(address string) (float64, error) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	url := fmt.Sprintf("https://apilist.tronscanapi.com/api/accountv2?address=%s", address)

	// new http client with timeout
	client := &http.Client{
		Timeout: time.Second * 10,
	}

	//http request
	resp, err := client.Get(url)
	if err != nil {
		logger.Error("Failed to make HTTP request", "error", err)
		return 0, fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			return
		}
	}(resp.Body)

	// validate response
	if resp.StatusCode != http.StatusOK {
		logger.Error("Bad response code", "Status", resp.Status)
		return 0, fmt.Errorf("bad response code: %s", resp.Status)
	}

	// decode response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read response body", "error", err)
		return 0, fmt.Errorf("failed to read response body: %w", err)
	}

	var tokenBalance TokenBalance
	if err := json.Unmarshal(body, &tokenBalance); err != nil {
		logger.Error("failed to parse JSON response", "error", err)
		return 0, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// USDT TRC20 token ID on Tron network
	const usdtTokenID = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"

	for _, token := range tokenBalance.Tokens {
		if token.TokenID == usdtTokenID {
			//convert balance to float64
			balanceFloat, err := strconv.ParseFloat(token.Balance, 64)
			if err != nil {
				logger.Error("Failed to parse balance to float64", "error", err)
				return 0, fmt.Errorf("failed to parse balance to float64: %w", err)
			}

			//Apply decimal places
			balance := balanceFloat / math.Pow10(token.TokenDecimal)
			logger.Info("USDT balance retrieved", "address", address, "balance", balance)
			return balance, nil
		}
	}

	logger.Info("USDT balance not found", "address", address)
	return 0, nil
}
