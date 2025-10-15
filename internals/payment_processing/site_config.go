package payment_processing

import (
	"log"
	"sync"
)

type SiteType string

const (
	SiteTypeProductDelivery SiteType = "product_delivery"
	SiteTypeBalanceUpdate   SiteType = "balance_update"
)

type SiteConfig struct {
	Name          string
	Type          SiteType
	StartIndex    int    // Start of address index range for this site
	EndIndex      int    // End of address index range for this site
	DatabaseTable string // Empty for sites without database
}

var SiteRegistry = map[string]SiteConfig{
	"dwebstore": {
		Name:          "Dwebstore",
		Type:          SiteTypeProductDelivery,
		StartIndex:    0,
		EndIndex:      9999,
		DatabaseTable: "",
	},
	"cardershaven": {
		Name:          "Cardershaven",
		Type:          SiteTypeBalanceUpdate,
		StartIndex:    10000,
		EndIndex:      19999,
		DatabaseTable: "users",
	},
	"ganymede": {
		Name:          "Ganymede",
		Type:          SiteTypeProductDelivery,
		StartIndex:    20000,
		EndIndex:      29999,
		DatabaseTable: "",
	},
	"kuiper": {
		Name:          "Kuiper",
		Type:          SiteTypeProductDelivery,
		StartIndex:    30000,
		EndIndex:      39999,
		DatabaseTable: "",
	},
}

// Store address-to-site mapping
var addressSiteMap = make(map[string]string) // address -> site
var addressSiteMapMutex sync.RWMutex

func RegisterAddressForSite(address, site string) {
	addressSiteMapMutex.Lock()
	defer addressSiteMapMutex.Unlock()
	addressSiteMap[address] = site
	log.Printf("Registered address %s for site %s", address, site)
}

func GetSiteForAddress(address string) string {
	addressSiteMapMutex.RLock()
	defer addressSiteMapMutex.RUnlock()
	return addressSiteMap[address]
}

// DetermineSiteFromAddressPattern - Fallback method using address index
func DetermineSiteFromAddressPattern(address string) string {
	// This would need to decode the address and determine its index
	// For now, return empty to use the map
	return ""
}
