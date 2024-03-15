package utils

//
//import (
//	"encoding/json"
//	"fmt"
//	"github.com/joho/godotenv"
//	"io"
//	"log"
//	"net/http"
//	"os"
//	"time"
//)
//
//type IPAPILocation struct {
//	Continent   string `json:"continent"`
//	Country     string `json:"country"`
//	CountryCode string `json:"country_code"`
//	State       string `json:"state"`
//	City        string `json:"city"`
//	Zip         string `json:"zip"`
//	Timezone    string `json:"timezone"`
//	LocalTime   string `json:"local_time"`
//	IsDST       bool   `json:"is_dst"`
//}
//
//type IPAPIData struct {
//	IP            string        `json:"ip"`
//	Location      IPAPILocation `json:"location"`
//	ElapsedMS     float64       `json:"elapsed_ms"`
//	LocalTimeUnix int64         `json:"location.local_time_unix"`
//}
//
//func (i *IPAPIData) ParseLocalTime() (string, error) {
//	// Parse the local time
//	parsedTime, err := time.Parse(time.RFC3339, i.Location.LocalTime)
//	if err != nil {
//		return "", err
//	}
//
//	// Format the time in the desired format (24-hour)
//	formattedTime := parsedTime.Format("15:04")
//
//	return formattedTime, nil
//}
//
//var ipLocationAPI string
//
//func GetIpLocation(ipAddr string) (*IPAPIData, error) {
//	err := godotenv.Load(".env")
//	if err != nil {
//		return nil, err
//	}
//	ipLocationAPI = os.Getenv("IP_LOCATION_API_KEY")
//
//	apiUrl := fmt.Sprintf("https://api.ipapi.is?q=%s&key=%s", ipAddr, ipLocationAPI)
//
//	resp, err := http.Get(apiUrl)
//	if err != nil {
//		return nil, err
//	}
//	defer func(Body io.ReadCloser) {
//		err := Body.Close()
//		if err != nil {
//			log.Printf("Error closing response body: %s", err)
//		}
//	}(resp.Body)
//
//	if resp.StatusCode != http.StatusOK {
//		log.Printf("IPAPI request failed. Status: %d", resp.StatusCode)
//		return nil, fmt.Errorf("IPAPI request failed with status %d", resp.StatusCode)
//	}
//
//	var ipData IPAPIData
//	err = json.NewDecoder(resp.Body).Decode(&ipData)
//	if err != nil {
//		return nil, err
//	}
//
//	return &ipData, nil
//}
