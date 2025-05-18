package routes

import (
	"auth-service/controllers"
	"log"

	"github.com/gin-gonic/gin"
)

func RegisterUserRoutes(r *gin.Engine) {
	r.POST("/register", controllers.Register)
	r.POST("/login", controllers.Login)
	r.POST("/verify-email", controllers.VerifyEmail)

}

func RegisterAddressRoutes(r *gin.Engine) {
	log.Println("Registering address routes")
	addressRoutes := r.Group("/address")

	{
		addressRoutes.POST("/", controllers.CreateAddress)
	}
}
