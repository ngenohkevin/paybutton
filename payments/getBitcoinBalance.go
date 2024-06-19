package payments

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func GetBitcoinAddressBalanceWithBlockCypher(address, token string) (int64, error) {
	url := fmt.Sprintf("https://api.blockcypher.com/v1/btc/main/addrs/%s/balance?token=%s", address, token)

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
		} else if resp.StatusCode == 403 {
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
			time.Sleep(time.Second * 2) // Wait for 2 seconds before retrying
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
