package payments

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ngenohkevin/paybutton/utils"
	"io"
	"log"
	"net/http"
	"time"
)

type AddressResponse struct {
	Address string `json:"address"`
}
type httpClient struct {
	client *http.Client
}

var (
	blockonomicsAPIKey string
	httpClientInstance *httpClient
)

func init() {

	config, err := utils.LoadConfig()
	if err != nil {
		log.Printf("Error loading config: %v", err)
		log.Fatal("could not load config")
	}

	blockonomicsAPIKey = config.BlockonomicsAPI
	//proxyURL := os.Getenv("PROXY_URL")

	// Configure the transport with or without proxy
	transport := &http.Transport{
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     time.Second * 90,
	}

	//if proxyURL != "" {
	//	parsedProxyURL, err := url.Parse(proxyURL)
	//	if err != nil {
	//		log.Fatalf("Invalid PROXY_URL: %v", err)
	//	}
	//	transport.Proxy = http.ProxyURL(parsedProxyURL)
	//}

	httpClientInstance = &httpClient{
		client: &http.Client{
			Transport: transport,
			Timeout:   time.Second * 10,
		},
	}
}

func GenerateBitcoinAddress(email string, price float64) (string, error) {
	addrUrl := "https://www.blockonomics.co/api/new_address"

	// Create a unique label using the user's email address and a timestamp
	label := fmt.Sprintf("%s-%d", email, time.Now().Unix())

	data := map[string]interface{}{
		"label":  label,
		"amount": price,
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", addrUrl, bytes.NewBuffer(payload))
	if err != nil {
		return "", err
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

			}
		}(resp.Body)

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return "", fmt.Errorf("error generating bitcoin address, status code: %v, body: %s", resp.StatusCode, string(body))
		}

		var addressResponse AddressResponse
		if err := json.NewDecoder(resp.Body).Decode(&addressResponse); err != nil {
			return "", err
		}

		if addressResponse.Address == "" {
			return "", errors.New("empty address returned")
		}

		return addressResponse.Address, nil

	case err := <-errChan:
		return "", err

	case <-time.After(time.Second * 30):
		return "", errors.New("timed out waiting for API response")
	}
}
