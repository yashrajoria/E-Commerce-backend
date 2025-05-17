package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/order-service/controllers"
)

func RegisterOrderRoutes(r *gin.Engine) {
	orderRoutes := r.Group("/orders")
	{
		orderRoutes.POST("/", controllers.CreateOrder)
	}
}
