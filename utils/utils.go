package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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

//func GetBitcoinPrice() (float64, error) {
//	resp, err := http.Get(BlockonomicsRateApi)
//	if err != nil {
//		log.Printf("Error getting blockonomics rate: %s", err.Error())
//		return 0, err
//	}
//	defer func(Body io.ReadCloser) {
//		err := Body.Close()
//		if err != nil {
//
//		}
//	}(resp.Body)
//
//	body, err := io.ReadAll(resp.Body)
//	if err != nil {
//		log.Printf("Error reading blockonomics rate response: %s", err.Error())
//		return 0, err
//	}
//
//	var blockonomicsPrice map[string]interface{}
//	err = json.Unmarshal(body, &blockonomicsPrice)
//	if err != nil {
//		log.Printf("Error unmarshaling blockonomics rate JSON: %s", err.Error())
//		return 0, err
//	}
//
//	bitcoinUSDPrice, ok := blockonomicsPrice["price"].(float64)
//	if !ok {
//		log.Printf("Error getting blockonomics rate: invalid response")
//		return 0, err
//	}
//
//	return bitcoinUSDPrice, nil
//}

func GetCurrentTime() time.Time {
	return time.Now()
}

func GetExpiryTime() time.Time {
	return time.Now().Add(15 * time.Minute)
}

func getBlockonomicsRate() (float64, error) {
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
	bitcoinUSDPrice, err := getBlockonomicsRate()
	if err != nil {
		return 0, err
	}

	bitcoinAmount := priceInUSD / bitcoinUSDPrice

	return bitcoinAmount, nil
}

func GenerateBitcoinURI(address string, amountBTC float64) (string, error) {
	// Format the amount in BTC as a string with 8 decimal places
	amountStr := fmt.Sprintf("%.8f", amountBTC)

	// Construct the URI with the Bitcoin address and amount
	uri := fmt.Sprintf("bitcoin:%s?amount=%s", address, amountStr)

	// Encode the URI as a URL
	encodedURI, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	return encodedURI.String(), nil
}

func parseFloat(str string) (float64, error) {
	return strconv.ParseFloat(str, 64)
}
