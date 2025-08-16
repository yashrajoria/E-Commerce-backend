package routes

import (
	"cart-service/config"
	"cart-service/controllers"
	"cart-service/database"
	"cart-service/kafka"

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

	// You can add a middleware here to extract user ID from JWT if needed
	api := r.Group("/cart")
	{
		api.GET("/", controller.GetCart)
		api.POST("/add", controller.AddItems)
		api.DELETE("/remove/:product_id", controller.RemoveItem)
		api.DELETE("/clear", controller.ClearCart)
		api.POST("/checkout", controller.Checkout)
	}
}
