package main

import (
	"fmt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/ngenohkevin/paybutton/payments"
	"github.com/ngenohkevin/paybutton/utils"
	"github.com/sirupsen/logrus"
	"log"
	"net/http"
	"os"
)

func main() {
	r := gin.Default()
	r.Use(cors.Default())

	// Create a new Logrus logger
	logger := logrus.New()

	// Set the log formatter
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Set the logger as the default logger for Gin
	logger.SetOutput(gin.DefaultWriter)
	r.Use(Logger(logger))

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
		//ur, err := utils.GenerateBitcoinURI(address, priceBTC)
		//if err != nil {
		//	_ = fmt.Errorf("%v", err)
		//}

		log.Printf("Email: %s, Address: %s, Amount: %.2f", email, address, priceUSD)

		//qrCodeFileName := fmt.Sprintf("%s.png", address)
		//err = payments.GenerateQRCode(ur, qrCodeFileName)
		//if err != nil {
		//	c.JSON(http.StatusInternalServerError, gin.H{
		//		"message": fmt.Sprintf("Error generating QR code: %v", err.Error()),
		//	})
		//	// Add this line to log the actual error message:
		//	fmt.Println(err)
		//	return
		//}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": fmt.Sprintf("Error getting Bitcoin price: %s", err.Error()),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"address": address,
			//"qrCodeUrl":   fmt.Sprintf("%s/%s", os.Getenv("QR_CODE_BASE_URL"), qrCodeFileName),
			"priceInUSD":  priceUSD,
			"priceInBTC":  priceBTC,
			"email":       email,
			"created_at":  utils.GetCurrentTime(),
			"expired_at":  utils.GetExpiryTime(),
			"status":      "pending",
			"description": fmt.Sprintf("%s %s", os.Getenv("PRODUCT_NAME"), os.Getenv("PRODUCT_DESC")),
		})

		logger := c.MustGet("logger").(*logrus.Logger)
		logger.Printf("email: %s, address: %s, amount: %.2f", email, address, priceUSD)

	})
	//
	////callback handler not working. Needs to some refactoring
	//
	//r.POST("/callback", func(c *gin.Context) {
	//	address := c.PostForm("address")
	//	paidAmountStr := c.PostForm("paidAmount")
	//	email := c.PostForm("email")
	//
	//	if address == "" || paidAmountStr == "" || email == "" {
	//		c.JSON(http.StatusBadRequest, gin.H{
	//			"message": "Invalid input: address, paidAmount, and email are required",
	//		})
	//		return
	//	}
	//
	//	paidAmount, err := utils.ParseFloat(paidAmountStr)
	//	if err != nil {
	//		c.JSON(http.StatusBadRequest, gin.H{
	//			"message": "Invalid input: paidAmount must be a valid number",
	//		})
	//		return
	//	}
	//
	//	err = payments.MarkPaymentAsPaid(address, paidAmount, email)
	//	if err != nil {
	//		c.JSON(http.StatusInternalServerError, gin.H{
	//			"message": fmt.Sprintf("Error marking payment as paid: %s", err.Error()),
	//		})
	//		return
	//	}
	//
	//	// Open the file for appending
	//	file, err := os.OpenFile("paid_emails.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	//	if err != nil {
	//		c.JSON(http.StatusInternalServerError, gin.H{
	//			"message": fmt.Sprintf("Error writing paid email to file: %s", err.Error()),
	//		})
	//		return
	//	}
	//	defer func(file *os.File) {
	//		if err := file.Close(); err != nil {
	//			fmt.Printf("Error closing file: %v", err)
	//		}
	//	}(file)
	//
	//	// Write the email address to the file
	//	if _, err := file.WriteString(fmt.Sprintf("%s ---> %s\n", email, address)); err != nil {
	//		c.JSON(http.StatusInternalServerError, gin.H{
	//			"message": fmt.Sprintf("Error writing paid email to file: %s", err.Error()),
	//		})
	//		return
	//	}
	//
	//	c.JSON(http.StatusOK, gin.H{
	//		"message": fmt.Sprintf("Payment with address %s has been marked as paid. Email %s has been saved.", address, email),
	//	})
	//})

	err := r.Run()
	if err != nil {
		return
	} // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
func Logger(logger *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Set the logger in the Gin context
		c.Set("logger", logger)

		c.Next()

		entry := logger.WithFields(logrus.Fields{})
		entry.Print()
	}
}
