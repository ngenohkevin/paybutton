package payments

//import (
//	"encoding/json"
//	"errors"
//	"fmt"
//	"github.com/btcsuite/btcd/btcutil"
//	"github.com/btcsuite/btcd/btcutil/hdkeychain"
//	"github.com/btcsuite/btcd/chaincfg"
//	"github.com/joho/godotenv"
//	"log"
//	"os"
//	"sync"
//	"time"
//)
//
//type AddressResponse struct {
//	Address string `json:"address"`
//}
//
//var (
//	bitcoinXpub   string
//	walletService *WalletService
//	initOnce      sync.Once
//)
//
//// WalletService manages Bitcoin address generation
//type WalletService struct {
//	xpub         string
//	currentIndex int
//	mu           sync.Mutex
//	netParams    *chaincfg.Params
//	storage      *PersistentStorage
//}
//
//type PersistentStorage struct {
//	filePath string
//	mu       sync.Mutex
//}
//
//func NewPersistentStorage(filePath string) *PersistentStorage {
//	return &PersistentStorage{filePath: filePath}
//}
//
//func (ps *PersistentStorage) LoadIndex() (int, error) {
//	ps.mu.Lock()
//	defer ps.mu.Unlock()
//
//	file, err := os.Open(ps.filePath)
//	if err != nil {
//		if os.IsNotExist(err) {
//			return 0, nil // Start at index 0 if the file doesn't exist
//		}
//		return 0, err
//	}
//	defer file.Close()
//
//	var index int
//	err = json.NewDecoder(file).Decode(&index)
//	if err != nil {
//		return 0, err
//	}
//
//	return index, nil
//}
//
//func (ps *PersistentStorage) SaveIndex(index int) error {
//	ps.mu.Lock()
//	defer ps.mu.Unlock()
//
//	file, err := os.Create(ps.filePath)
//	if err != nil {
//		return err
//	}
//	defer file.Close()
//
//	return json.NewEncoder(file).Encode(index)
//}
//
//// Initialize loads environment variables and sets up the WalletService
//func Initialize() {
//	initOnce.Do(func() {
//		err := godotenv.Load(".env")
//		if err != nil {
//			log.Fatal("Error loading .env file")
//		}
//
//		bitcoinXpub = os.Getenv("BLOCKCHAIN_XPUB")
//		if bitcoinXpub == "" {
//			log.Fatal("BLOCKCHAIN_XPUB environment variable is required")
//		}
//
//		walletService, err = NewWalletService(bitcoinXpub, &chaincfg.MainNetParams)
//		if err != nil {
//			log.Fatalf("Failed to initialize wallet service: %v", err)
//		}
//	})
//}
//
//// NewWalletService initializes a WalletService with an xpub and network parameters
//func NewWalletService(xpub string, netParams *chaincfg.Params) (*WalletService, error) {
//	storage := NewPersistentStorage("wallet_index.json")
//	index, err := storage.LoadIndex()
//	if err != nil {
//		return nil, fmt.Errorf("failed to load index: %v", err)
//	}
//
//	return &WalletService{
//		xpub:         xpub,
//		currentIndex: index,
//		netParams:    netParams,
//		storage:      storage,
//	}, nil
//}
//
//// validateXpub checks if the provided xpub is valid and matches the network
//func validateXpub(xpub string, netParams *chaincfg.Params) error {
//	extKey, err := hdkeychain.NewKeyFromString(xpub)
//	if err != nil {
//		return fmt.Errorf("failed to parse xpub: %v", err)
//	}
//	if !extKey.IsForNet(netParams) {
//		return errors.New("xpub does not match the specified network")
//	}
//	return nil
//}
//
//// GenerateBitcoinAddress derives a native SegWit Bitcoin address
//func GenerateBitcoinAddress(email string, price float64) (string, error) {
//	Initialize()
//
//	// Create a label (used only for logging or tracking, not affecting address)
//	label := fmt.Sprintf("%s-%d", email, time.Now().Unix())
//	log.Printf("Generating address for label: %s, price: %.8f BTC", label, price)
//
//	address, err := walletService.GenerateAddress()
//	if err != nil {
//		return "", fmt.Errorf("failed to generate Bitcoin address: %v", err)
//	}
//
//	return address, nil
//}
//
//// GenerateAddress derives the next native SegWit (Bech32) address
//func (w *WalletService) GenerateAddress() (string, error) {
//	w.mu.Lock()
//	defer w.mu.Unlock()
//
//	// Parse the extended public key (xpub)
//	extKey, err := hdkeychain.NewKeyFromString(w.xpub)
//	if err != nil {
//		return "", fmt.Errorf("invalid xpub: %v", err)
//	}
//
//	// Derive the next child key
//	childKey, err := extKey.Derive(uint32(w.currentIndex))
//	if err != nil {
//		return "", fmt.Errorf("failed to derive child key: %v", err)
//	}
//
//	// Extract public key and create address
//	pubKey, err := childKey.ECPubKey()
//	if err != nil {
//		return "", fmt.Errorf("failed to get public key: %v", err)
//	}
//	witnessProg := btcutil.Hash160(pubKey.SerializeCompressed())
//	address, err := btcutil.NewAddressWitnessPubKeyHash(witnessProg, w.netParams)
//	if err != nil {
//		return "", fmt.Errorf("failed to create address: %v", err)
//	}
//
//	// Save the updated index
//	w.currentIndex++
//	err = w.storage.SaveIndex(w.currentIndex)
//	if err != nil {
//		return "", fmt.Errorf("failed to save index: %v", err)
//	}
//
//	return address.EncodeAddress(), nil
//}
