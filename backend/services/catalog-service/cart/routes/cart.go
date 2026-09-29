package routes

import (
	"time"

	"catalog-service/cart/controllers"
	"catalog-service/cart/database"
	"catalog-service/cart/middleware"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

func RegisterCartRoutes(
	r *gin.Engine,
	redisClient *redis.Client,
	snsClient *aws_pkg.SNSClient,
	cartTTL time.Duration,
	validator controllers.ProductValidator,
) {
	repo := database.NewCartRepository(redisClient, cartTTL)
	controller := controllers.NewCartController(repo, snsClient, cartTTL, validator)

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
