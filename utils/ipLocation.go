package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type IPAPILocation struct {
	Continent   string `json:"continent"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	State       string `json:"state"`
	City        string `json:"city"`
	Zip         string `json:"zip"`
	Timezone    string `json:"timezone"`
	LocalTime   string `json:"local_time"`
	IsDST       bool   `json:"is_dst"`
}

type IPAPIData struct {
	IP            string        `json:"ip"`
	Location      IPAPILocation `json:"location"`
	ElapsedMS     float64       `json:"elapsed_ms"`
	LocalTimeUnix int64         `json:"location.local_time_unix"`
}

func (i *IPAPIData) ParseLocalTime() (string, error) {
	// Check if Location data is valid
	if i.Location.LocalTime == "" {
		return "00:00", fmt.Errorf("LocalTime is empty")
	}

	// Parse the local time
	parsedTime, err := time.Parse(time.RFC3339, i.Location.LocalTime)
	if err != nil {
		log.Printf("Error parsing LocalTime: %s", err)
		// Return a default fallback time
		return "00:00", nil
	}

	// Format the time in the desired format (24-hour)
	formattedTime := parsedTime.Format("15:04")

	return formattedTime, nil
}

var ipLocationAPI string

func GetIpLocation(ipAddr string) (*IPAPIData, error) {
	location, err := LoadConfig()
	if err != nil {
		log.Printf("Error loading config: %v", err)
		log.Fatal("could not load config")
	}

	ipLocationAPI = location.IpLocation
	apiUrl := fmt.Sprintf("https://api.ipapi.is?q=%s&key=%s", ipAddr, ipLocationAPI)

	resp, err := http.Get(apiUrl)
	if err != nil {
		log.Printf("Error making request to IPAPI: %s", err)
		// Return default data in case of failure
		return &IPAPIData{
			Location: IPAPILocation{
				Continent: "Unknown",
				Country:   "Unknown",
				City:      "Unknown",
				Timezone:  "UTC",
			},
		}, nil
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			return
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("IPAPI request failed. Status: %d", resp.StatusCode)
		// Return default data if API responds with an error
		return &IPAPIData{
			Location: IPAPILocation{
				Continent: "Unknown",
				Country:   "Unknown",
				City:      "Unknown",
				Timezone:  "UTC",
			},
		}, nil
	}

	var ipData IPAPIData
	err = json.NewDecoder(resp.Body).Decode(&ipData)
	if err != nil {
		log.Printf("Error decoding IPAPI response: %s", err)
		// Return default data in case of a decoding error
		return &IPAPIData{
			Location: IPAPILocation{
				Continent: "Unknown",
				Country:   "Unknown",
				City:      "Unknown",
				Timezone:  "UTC",
			},
		}, nil
	}

	return &ipData, nil
}
