package utils

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	BlockonomicsRateApi = "https://blockonomics.co/api/price?currency=USD"
)

type BlockonomicsPrice struct {
	Price float64 `json:"price"`
}

type RateCache struct {
	rate       float64
	expiration time.Time
}

var cache RateCache

func ParseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func GetCurrentTime() time.Time {
	return time.Now()
}

func GetExpiryTime() time.Time {
	return time.Now().Add(15 * time.Minute)
}

func GetBlockonomicsRate() (float64, error) {
	if cache.expiration.After(time.Now()) {
		// Rate is still valid, return it from cache
		return cache.rate, nil
	}

	resp, err := http.Get(BlockonomicsRateApi)
	if err != nil {
		log.Printf("Error getting blockonomics rate: %s", err.Error())
		return 0, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading blockonomics rate response: %s", err.Error())
		return 0, err
	}

	var blockonomicsPrice BlockonomicsPrice
	err = json.Unmarshal(body, &blockonomicsPrice)
	if err != nil {
		log.Printf("Error unmarshaling blockonomics rate JSON: %s", err.Error())
		return 0, err
	}

	bitcoinUSDPrice := blockonomicsPrice.Price

	// Cache the rate and its expiration time
	cache.rate = bitcoinUSDPrice
	cache.expiration = time.Now().Add(5 * time.Minute) // cache for 5 minutes

	return bitcoinUSDPrice, nil
}

func ConvertToBitcoinUSD(priceInUSD float64) (float64, error) {
	bitcoinUSDPrice, err := GetBlockonomicsRate()
	if err != nil {
		return 0, err
	}

	bitcoinAmount := priceInUSD / bitcoinUSDPrice

	return bitcoinAmount, nil
}
