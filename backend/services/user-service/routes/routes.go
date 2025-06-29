package routes

import (
	"log"
	"user-service/controllers"

	"github.com/gin-gonic/gin"
)

func RegisterUserRoutes(r *gin.Engine) {
	log.Println("Registering user routes...")
	userRoutes := r.Group("/users")

	userRoutes.GET("/profile", controllers.GetProfile)
	userRoutes.PUT("/profile", controllers.UpdateProfile)

}
