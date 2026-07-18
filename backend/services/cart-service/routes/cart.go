package routes

import (
	"cart-service/config"
	"cart-service/controllers"
	"cart-service/database"
	"cart-service/middleware"

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

	// Protected cart routes — gateway JWT + service-side X-User-ID gate
	api := r.Group("/cart")
	api.Use(middleware.RequireUser())
	{
		api.GET("/", controller.GetCart)
		api.POST("/add", controller.AddItems)
		api.DELETE("/remove/:product_id", controller.RemoveItem)
		api.DELETE("/clear", controller.ClearCart)
		api.POST("/coupon", controller.ApplyCoupon)
		api.POST("/checkout", controller.Checkout)
	}
}
