package payments

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type BalanceResponse struct {
	Address string `json:"addr"`
	Balance int64  `json:"balance"`
}

func GetBitcoinAddressBalance(address string) (int64, error) {
	url := "https://www.blockonomics.co/api/balance"

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

	respChan := make(chan *http.Response)
	errChan := make(chan error)

	go func() {
		resp, err := httpClientInstance.client.Do(req)
		if err != nil {
			errChan <- err
			return
		}
		respChan <- resp
	}()

	select {
	case resp := <-respChan:
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				// Handle error closing the body
			}
		}(resp.Body)

		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("error fetching balance, status code: %v", resp.StatusCode)
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

		return balanceResponse.Response[0].Balance, nil

	case err := <-errChan:
		return 0, err

	case <-time.After(time.Second * 30):
		return 0, fmt.Errorf("timed out waiting for API response")
	}
}
