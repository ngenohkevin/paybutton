package payments

import (
	"errors"
	"time"
)

type Payment struct {
	Email      string
	PriceUSD   float64
	PriceBTC   float64
	Address    string
	Paid       bool
	PaidAmount float64
	Date       time.Time
}

var payments []*Payment

//func CreatePayment(address string, priceUSD float64, priceBTC float64, email string) (*Payment, error) {
//	if !utils.IsValidEmail(email) {
//		return nil, errors.New("invalid email address")
//	}
//
//	if priceUSD <= 0 {
//		return nil, errors.New("price in USD must be greater than zero")
//	}
//
//	if priceBTC <= 0 {
//		return nil, errors.New("price in BTC must be greater than zero")
//	}
//
//	payment := &Payment{
//		Email:      email,
//		PriceUSD:   priceUSD,
//		PriceBTC:   priceBTC,
//		Address:    address,
//		Paid:       false,
//		PaidAmount: 0,
//		Date:       time.Now(),
//	}
//
//	payments = append(payments, payment)
//
//	return payment, nil
//}

func getPaymentByAddress(address string) (*Payment, error) {
	for _, payment := range payments {
		if payment.Address == address {
			return payment, nil
		}
	}
	return nil, errors.New("payment not found")
}
