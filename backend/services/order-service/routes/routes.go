package routes

import (
    "order-service/controllers"
    "order-service/middleware"

    "github.com/gin-gonic/gin"
)

func RegisterOrderRoutes(r *gin.Engine) {
    orderRoutes := r.Group("/orders")
    orderRoutes.Use(middleware.AuthMiddleware())

    orderRoutes.POST("/", controllers.CreateOrder)
    orderRoutes.GET("/", controllers.GetOrders)
    orderRoutes.GET("/:id", controllers.GetOrderByID)
}
