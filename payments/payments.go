package payments

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type BalanceResponse struct {
	Address string `json:"address"`
	Balance int64  `json:"balance"`
}

func GetBitcoinAddressBalance(address string) (int64, error) {
	url := fmt.Sprintf("https://www.blockonomics.co/api/balance")

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
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("error fetching balance, status code: %v", resp.StatusCode)
		}

		var balanceResponses []BalanceResponse
		if err := json.NewDecoder(resp.Body).Decode(&balanceResponses); err != nil {
			return 0, err
		}

		if len(balanceResponses) == 0 {
			return 0, fmt.Errorf("no balance data returned")
		}

		return balanceResponses[0].Balance, nil

	case err := <-errChan:
		return 0, err

	case <-time.After(time.Second * 30):
		return 0, fmt.Errorf("timed out waiting for API response")
	}
}
