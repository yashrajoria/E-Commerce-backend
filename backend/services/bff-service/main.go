package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "BFF Service is healthy",
		})
	})
	r.Run(":8088") // Start the server on port 8088

}
