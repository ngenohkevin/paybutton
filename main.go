package main

import (
	"fmt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/ngenohkevin/paybutton/payments"
	"github.com/ngenohkevin/paybutton/utils"
	"net/http"
	"os"
)

func main() {
	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Payment Service API",
		})
	})

	r.POST("/payment", func(c *gin.Context) {
		email := c.PostForm("email")
		priceStr := c.PostForm("price")

		if email == "" || priceStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid input: email and price are required",
			})
			return
		}

		priceUSD, err := utils.ParseFloat(priceStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid input: price must be a valid number",
			})
			return
		}
		priceBTC, err := utils.ConvertToBitcoinUSD(priceUSD)
		address, err := payments.GenerateBitcoinAddress(email, priceBTC)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": fmt.Sprintf("Error generating Bitcoin address: %s", err.Error()),
			})
			return
		}
		//comment GenerateQRCode when pushing to production
		ur, err := utils.GenerateBitcoinURI(address, priceBTC)
		if err != nil {
			_ = fmt.Errorf("%v", err)
		}

		qrCodeFileName := fmt.Sprintf("%s.png", address)
		err = payments.GenerateQRCode(ur, qrCodeFileName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": fmt.Sprintf("Error generating QR code: %v", err.Error()),
			})
			// Add this line to log the actual error message:
			fmt.Println(err)
			return
		}

		payment, err := payments.CreatePayment(address, priceUSD, priceBTC, email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": fmt.Sprintf("Error creating payment: %s", err.Error()),
			})
			return
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": fmt.Sprintf("Error getting Bitcoin price: %s", err.Error()),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"address": payment.Address,
			//"qrCodeUrl":   fmt.Sprintf("%s/%s", os.Getenv("QR_CODE_BASE_URL"), qrCodeFileName),
			"priceInUSD":  payment.PriceUSD,
			"priceInBTC":  payment.PriceBTC,
			"email":       payment.Email,
			"created_at":  utils.GetCurrentTime(),
			"expired_at":  utils.GetExpiryTime(),
			"status":      "pending",
			"description": fmt.Sprintf("%s %s", os.Getenv("PRODUCT_NAME"), os.Getenv("PRODUCT_DESC")),
		})
	})

	r.POST("/callback", func(c *gin.Context) {
		address := c.PostForm("address")
		paidAmountStr := c.PostForm("paidAmount")
		email := c.PostForm("email")

		if address == "" || paidAmountStr == "" || email == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid input: address, paidAmount, and email are required",
			})
			return
		}

		paidAmount, err := utils.ParseFloat(paidAmountStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid input: paidAmount must be a valid number",
			})
			return
		}

		err = payments.MarkPaymentAsPaid(address, paidAmount, email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": fmt.Sprintf("Error marking payment as paid: %s", err.Error()),
			})
			return
		}

		// Open the file for appending
		file, err := os.OpenFile("paid_emails.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": fmt.Sprintf("Error writing paid email to file: %s", err.Error()),
			})
			return
		}
		defer func(file *os.File) {
			if err := file.Close(); err != nil {
				fmt.Printf("Error closing file: %v", err)
			}
		}(file)

		// Write the email address to the file
		if _, err := file.WriteString(fmt.Sprintf("%s ---> %s\n", email, address)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": fmt.Sprintf("Error writing paid email to file: %s", err.Error()),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("Payment with address %s has been marked as paid. Email %s has been saved.", address, email),
		})
	})

	err := r.Run()
	if err != nil {
		return
	} // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
