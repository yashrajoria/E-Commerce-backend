package routes

import (
	"auth-service/controllers"
	"log"

	"github.com/gin-gonic/gin"
)

func RegisterAddressRoutes(r *gin.Engine) {
	log.Println("Registering address routes")
	addressRoutes := r.Group("/address")

	{
		addressRoutes.POST("/", controllers.CreateAddress)
	}
}
