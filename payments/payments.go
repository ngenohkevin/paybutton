package payments

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type BalanceResponse struct {
	Addr        string `json:"addr"`
	Confirmed   int64  `json:"confirmed"`
	Unconfirmed int64  `json:"unconfirmed"`
}

func GetBitcoinAddressBalanceWithBlockonomics(address string) (int64, error) {
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

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			return
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

	// Sum confirmed and unconfirmed balances
	totalBalance := balanceResponse.Response[0].Confirmed + balanceResponse.Response[0].Unconfirmed

	return totalBalance, nil
}
