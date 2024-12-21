package utils

import (
	"encoding/json"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
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

var (
	cache              RateCache
	blockonomicsClient *http.Client
)

func init() {
	proxyURL := os.Getenv("PROXY_URL")

	transport := &http.Transport{
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     time.Second * 30,
	}

	//Set up proxy if it is provided
	if proxyURL != "" {
		parsedProxyURL, err := url.Parse(proxyURL)
		if err != nil {
			log.Fatalf("Invalid PROXY_URL: %v", err)
		}
		transport.Proxy = http.ProxyURL(parsedProxyURL)
	}

	blockonomicsClient = &http.Client{
		Transport: transport,
		Timeout:   time.Second * 10,
	}
}

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

	resp, err := blockonomicsClient.Get(BlockonomicsRateApi)
	if err != nil {
		log.Printf("Error getting blockonomics rate: %s", err.Error())
		return 0, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("Error closing blockonomics rate response body: %s", err)
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

func RandomUSDTAddress() string {
	// List of USDT addresses
	addresses := []string{
		"TJecnsMey1oj1wfSuV7FAaduuje4T3W3AE",
		"TQMFu4XpK2paEPxKhBWtYpHva11Awrn48F",
		"TLR3JMH6u1chcdjdnLqkEW1jaMWKRBHZu3",
		"THzcboCRkkdBrbUMjAgKUGXuqu3QLYNwPP",
		"TCtWyMdkSvdLJqjH7dc1XyF2hFyub5Av4r",
	}

	// Seed the random number generator
	rand.New(rand.NewSource(time.Now().UnixNano()))

	// Select a random address
	randomIndex := rand.Intn(len(addresses))

	return addresses[randomIndex]
}
