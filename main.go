package main

import (
	"fmt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/ngenohkevin/paybutton/payments"
	"github.com/ngenohkevin/paybutton/utils"
	"log"
	"net/http"
	"os"
)

var botApiKey string

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	botApiKey = os.Getenv("BOT_API_KEY")

	//Initialize Telegram Bot
	bot, err := tgbotapi.NewBotAPI(botApiKey)
	if err != nil {
		log.Fatal(err)
	}
	//Set the chatID of the user where you want to send the message
	chatID := int64(6074038462)

	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Payment Service API",
		})
	})

	r.POST("/payment", func(c *gin.Context) {
		//Get IP of the client
		clientIP := c.ClientIP()

		//fetch data from ipAPI
		ipAPIData, err := utils.GetIpLocation(clientIP)
		if err != nil {
			log.Printf("Error getting ip location: %s", err.Error())
		}

		email := c.PostForm("email")
		priceStr := c.PostForm("price")
		description := c.PostForm("description")
		name := c.PostForm("name")

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
		localTime, err := ipAPIData.ParseLocalTime()
		if err != nil {
			log.Printf("Error parsing local time: %s", err)
			// Handle the error as needed
		}

		log.Printf("Formatted Local Time: %s", localTime)

		logMessage := fmt.Sprintf("Email: %s, Address: %s, Amount: %.2f, Name: %s, Product: %s", email, address, priceUSD, name, description)
		log.Printf(logMessage)

		botLogMessage := fmt.Sprintf("*Email:* `%s`\n*Address:* `%s`\n*Amount:* `%0.2f`\n*Name:* "+
			"`%s`\n*Product:* `%s`\n*IP Address:* `%s`\n*Country:* `%s`\n*State:* `%s`\n*City:* `%s`\n*Local Time:* `%s`",
			email, address, priceUSD, name, description, clientIP, ipAPIData.Location.Country, ipAPIData.Location.State, ipAPIData.Location.City, localTime)

		msg := tgbotapi.NewMessage(chatID, botLogMessage)
		msg.ParseMode = tgbotapi.ModeMarkdown
		_, err = bot.Send(msg)
		if err != nil {
			log.Printf("Error sending message to user: %s", err.Error())
		}
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
			"description": description,
			"name":        name,
		})

	})

	err = r.Run()
	if err != nil {
		return
	} // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
