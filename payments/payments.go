package payments

import (
	"errors"
	"fmt"
	"github.com/ngenohkevin/paybutton/utils"
	"time"
)

type Payment struct {
	Email      string
	Amount     float64
	Address    string
	Paid       bool
	PaidAmount float64
	Date       time.Time
}

var payments []*Payment

func createPayment(email string, amount float64) (*Payment, error) {
	if !utils.IsValidEmail(email) {
		return nil, errors.New("invalid email address")
	}

	if amount <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}

	address, err := GenerateBitcoinAddress(email, amount)
	if err != nil {
		return nil, fmt.Errorf("error generating Bitcoin address: %v", err)
	}

	payment := &Payment{
		Email:      email,
		Amount:     amount,
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

func markPaymentAsPaid(address string, paidAmount float64) error {
	payment, err := getPaymentByAddress(address)
	if err != nil {
		return err
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
