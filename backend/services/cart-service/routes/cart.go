package routes

import (
	"cart-service/config"
	"cart-service/controllers"
	"cart-service/database"
	"cart-service/kafka"
	"cart-service/middleware"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func RegisterCartRoutes(
	r *gin.Engine,
	redisClient *redis.Client,
	producer *kafka.Producer,
	cfg config.Config,
) {
	repo := database.NewCartRepository(redisClient, cfg.CartTTL)
	controller := controllers.NewCartController(repo, producer, cfg)

	// Protected cart routes (require authentication)
	api := r.Group("/cart")
	api.Use(middleware.AuthMiddleware())
	{
		api.GET("/", controller.GetCart)
		api.POST("/add", controller.AddItems)
		api.DELETE("/remove/:product_id", controller.RemoveItem)
		api.DELETE("/clear", controller.ClearCart)
		api.POST("/checkout", controller.Checkout)
	}
}
