package utils

import (
	"encoding/json"
	"io"
	"log"
	"math/rand"
	"net/http"
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
	//proxyURL := os.Getenv("PROXY_URL")

	transport := &http.Transport{
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     time.Second * 30,
	}

	//Set up proxy if it is provided
	//if proxyURL != "" {
	//	parsedProxyURL, err := url.Parse(proxyURL)
	//	if err != nil {
	//		log.Fatalf("Invalid PROXY_URL: %v", err)
	//	}
	//	transport.Proxy = http.ProxyURL(parsedProxyURL)
	//}

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
		"TKQEwi15GtqWG1qHHESm7eFqt9b5qtyW6Z",
		"TJwLnEYFNRPgFQ3GBTEVEq7aZDcFM15eaH",
		"THxoGNFakcgVpQinNtXhBiT5WRpTviwGmR",
		"TC5PUugKrgPj63vQM2s1YubnDDu7CoEUjv",
		"TX2npc6nmZis5jNsLEfSppiXEQCmyoZGhu",
		"TGdZ4T4BkwGBkaWFRHMjrKqxEBEBZEyuA8",
		"TB36N2GfmDWiuWCs1cTMJq9ytenz8PHe8w",
		"TBpAXWEGD8LPpx58Fjsu1ejSMJhgDUBNZK",
		"TPg34ZwyjTeyguqYMedLe5pfkhrGDvZp1G",
		"TVdgysNmtGHSifXJqfqoCeZovP9Ez5gpqH",
		"TWMtviX1jZqzFZxh3mJHvLGv48d1qYLWwu",
		"TT7wKXWk29kGTi6gC1jN6fXbx59Ve8exYf",
	}

	// Use modern Go random generator (automatically seeded)
	randomIndex := rand.Intn(len(addresses))

	return addresses[randomIndex]
}

// IsProduction checks if the app is running in production (Render)
func IsProduction() bool {
	// Render sets this environment variable in production
	return os.Getenv("RENDER") != ""
}

// GetWebhookURL constructs the webhook URL for the Telegram bot
func GetWebhookURL() string {
	// Get the host from environment variables
	renderURL := os.Getenv("RENDER_EXTERNAL_URL")
	if renderURL == "" {
		// Fallback for testing
		renderURL = "https://paybutton.onrender.com"
	}

	// Get the bot token
	botToken := os.Getenv("BOT_API_KEY")

	// Construct the webhook URL with the token as part of the path for security
	return renderURL + "/bot" + botToken + "/webhook"
}
