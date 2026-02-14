package routes

import (
	"cart-service/config"
	"cart-service/controllers"
	"cart-service/database"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

func RegisterCartRoutes(
	r *gin.Engine,
	redisClient *redis.Client,
	snsClient *aws_pkg.SNSClient,
	cfg config.Config,
) {
	repo := database.NewCartRepository(redisClient, cfg.CartTTL)
	controller := controllers.NewCartController(repo, snsClient, cfg)

	// Protected cart routes (require authentication)
	api := r.Group("/cart")
	// TODO: Add authentication middleware when implemented
	{
		api.GET("/", controller.GetCart)
		api.POST("/add", controller.AddItems)
		api.DELETE("/remove/:product_id", controller.RemoveItem)
		api.DELETE("/clear", controller.ClearCart)
		api.POST("/checkout", controller.Checkout)
	}
}
