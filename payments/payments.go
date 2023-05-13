package payments

import (
	"errors"
	"fmt"
	"github.com/ngenohkevin/paybutton/utils"
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

func CreatePayment(address string, priceUSD float64, priceBTC float64, email string) (*Payment, error) {
	if !utils.IsValidEmail(email) {
		return nil, errors.New("invalid email address")
	}

	if priceUSD <= 0 {
		return nil, errors.New("price in USD must be greater than zero")
	}

	if priceBTC <= 0 {
		return nil, errors.New("price in BTC must be greater than zero")
	}

	payment := &Payment{
		Email:      email,
		PriceUSD:   priceUSD,
		PriceBTC:   priceBTC,
		Address:    address,
		Paid:       false,
		PaidAmount: 0,
		Date:       time.Now(),
	}

	payments = append(payments, payment)

	return payment, nil
}

func getPaymentByAddress(address string) (*Payment, error) {
	for _, payment := range payments {
		if payment.Address == address {
			return payment, nil
		}
	}
	return nil, errors.New("payment not found")
}

func MarkPaymentAsPaid(address string, paidAmount float64, email string) error {
	if address == "" || paidAmount <= 0 || email == "" {
		return errors.New("invalid input: address, paidAmount, and email are required")
	}

	payment, err := getPaymentByAddress(address)
	if err != nil {
		return fmt.Errorf("error marking payment as paid: %v", err)
	}

	if payment.Email != email {
		return errors.New("invalid input: email does not match the payment record")
	}

	payment.Paid = true
	payment.PaidAmount = paidAmount

	return nil
}

//func generateInvoice(address string, amount float64) (string, error) {
//	payment, err := getPaymentByAddress(address)
//	if err != nil {
//		return "", err
//	}
//
//	invoice := fmt.Sprintf("Payment Invoice\n\nEmail: %s\nAmount: %.2f BTC\nAddress: %s\nPaid: %t\nPaid Amount: %.2f BTC\nDate: %s",
//		payment.Email, payment.Amount, payment.Address, payment.Paid, payment.PaidAmount, payment.Date.Format("2006-01-02 15:04:05"))
//
//	return invoice, nil
//}
