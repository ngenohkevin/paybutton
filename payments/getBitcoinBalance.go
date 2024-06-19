package payments

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type BlockCypherBalance struct {
	Address            string `json:"address"`
	TotalReceived      int64  `json:"total_received"`
	TotalSent          int64  `json:"total_sent"`
	Balance            int64  `json:"balance"`
	UnconfirmedBalance int64  `json:"unconfirmed_balance"`
	FinalBalance       int64  `json:"final_balance"`
}

func GetBitcoinAddressBalanceWithBlockCypher(address, token string) (int64, error) {
	url := fmt.Sprintf("https://api.blockcypher.com/v1/btc/main/addrs/%s/balance?token=%s", address, token)

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("error fetching balance, status code: %v", resp.StatusCode)
	}

	var balanceResponse BlockCypherBalance
	if err := json.NewDecoder(resp.Body).Decode(&balanceResponse); err != nil {
		return 0, err
	}

	totalBalance := balanceResponse.Balance + balanceResponse.UnconfirmedBalance

	return totalBalance, nil
}
