package payments

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/joho/godotenv"
	"io"
	"net/http"
	"os"
	"time"
)

type AddressResponse struct {
	Address string `json:"address"`
}

func GenerateBitcoinAddress(email string, price float64) (string, error) {
	err := godotenv.Load(".env")
	if err != nil {
		return "", err
	}

	apiKey := os.Getenv("BLOCKONOMICS_API_KEY")
	url := "https://www.blockonomics.co/api/new_address"

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

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error generating bitcoin address, status code: %d", resp.StatusCode)
	}

	var addressResponse AddressResponse
	if err := json.NewDecoder(resp.Body).Decode(&addressResponse); err != nil {
		return "", err
	}

	if addressResponse.Address == "" {
		return "", errors.New("empty address returned")
	}

	return addressResponse.Address, nil
}
