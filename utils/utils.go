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
	// Primary and fallback APIs for BTC price
	CoinGeckoRateApi    = "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd"
	BlockonomicsRateApi = "https://blockonomics.co/api/price?currency=USD"
	CoinbaseRateApi     = "https://api.coinbase.com/v2/exchange-rates?currency=BTC"
)

// Response structures for different APIs
type CoinGeckoPrice struct {
	Bitcoin struct {
		USD float64 `json:"usd"`
	} `json:"bitcoin"`
}

type BlockonomicsPrice struct {
	Price float64 `json:"price"`
}

type CoinbasePrice struct {
	Data struct {
		Rates map[string]string `json:"rates"`
	} `json:"data"`
}

type RateCache struct {
	rate       float64
	expiration time.Time
}

var (
	cache      RateCache
	httpClient *http.Client
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

	httpClient = &http.Client{
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

// GetBTCRate fetches BTC to USD rate with multiple provider fallbacks
func GetBTCRate() (float64, error) {
	if cache.expiration.After(time.Now()) {
		// Rate is still valid, return it from cache
		return cache.rate, nil
	}

	// Try providers in order of reliability
	providers := []func() (float64, error){
		getCoinGeckoRate,
		getBlockonomicsRate,
		getCoinbaseRate,
	}

	var lastErr error
	for i, provider := range providers {
		rate, err := provider()
		if err == nil && rate > 0 {
			// Cache the successful rate
			cache.rate = rate
			cache.expiration = time.Now().Add(5 * time.Minute) // cache for 5 minutes

			if i > 0 {
				log.Printf("BTC rate fetched from fallback provider #%d: $%.2f", i+1, rate)
			}
			return rate, nil
		}
		lastErr = err
		log.Printf("Provider #%d failed: %s", i+1, err)
	}

	log.Printf("All BTC rate providers failed, last error: %s", lastErr)
	return 0, lastErr
}

// getCoinGeckoRate fetches from CoinGecko (most reliable)
func getCoinGeckoRate() (float64, error) {
	resp, err := httpClient.Get(CoinGeckoRateApi)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var price CoinGeckoPrice
	err = json.Unmarshal(body, &price)
	if err != nil {
		return 0, err
	}

	return price.Bitcoin.USD, nil
}

// getBlockonomicsRate fetches from Blockonomics (current implementation)
func getBlockonomicsRate() (float64, error) {
	resp, err := httpClient.Get(BlockonomicsRateApi)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var price BlockonomicsPrice
	err = json.Unmarshal(body, &price)
	if err != nil {
		return 0, err
	}

	return price.Price, nil
}

// getCoinbaseRate fetches from Coinbase (additional fallback)
func getCoinbaseRate() (float64, error) {
	resp, err := httpClient.Get(CoinbaseRateApi)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var price CoinbasePrice
	err = json.Unmarshal(body, &price)
	if err != nil {
		return 0, err
	}

	// Parse USD rate from string
	usdRate, err := strconv.ParseFloat(price.Data.Rates["USD"], 64)
	if err != nil {
		return 0, err
	}

	return usdRate, nil
}

// Legacy function for backward compatibility
func GetBlockonomicsRate() (float64, error) {
	return GetBTCRate()
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
	// Can use RENDER_EXTERNAL_URL or EXTERNAL_URL for flexibility
	externalURL := os.Getenv("RENDER_EXTERNAL_URL")
	if externalURL == "" {
		externalURL = os.Getenv("EXTERNAL_URL")
	}
	if externalURL == "" {
		// Default to Coolify deployment
		externalURL = "https://paybutton.perigrine.cloud"
	}

	// Get the bot token
	botToken := os.Getenv("BOT_API_KEY")

	// Construct the webhook URL with the token as part of the path for security
	return externalURL + "/bot" + botToken + "/webhook"
}
