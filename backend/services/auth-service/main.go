package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"auth-service/controllers"
	"auth-service/database"
	"auth-service/models"
	"auth-service/repository"
	"auth-service/services"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

func main() {
	// --- 1. Initialization ---

	// Initialize structured logger
	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	// Load .env file
	_ = godotenv.Load()

	// Connect to the database
	if err := database.Connect(); err != nil { // Assuming you have a Connect function
		zap.L().Fatal("Database connection failed", zap.Error(err))
	}
	// Run migrations (auto-migrate models including refresh tokens)
	if err := models.Migrate(database.DB); err != nil {
		zap.L().Fatal("DB migration failed", zap.Error(err))
	}

	// --- 2. Dependency Injection (Wiring the layers) ---

	// Initialize Repositories
	userRepo := repository.NewUserRepository(database.DB)

	// Initialize Services
	tokenService := services.NewTokenService()
	// emailService := services.NewEmailService()
	authService := services.NewAuthService(userRepo, tokenService, database.DB)

	// Initialize Controllers
	authController := controllers.NewAuthController(authService)

	// --- 3. HTTP Server & Middleware ---

	r := gin.New()
	r.Use(gin.Recovery()) // Panic protection

	// Add request timeout middleware
	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	// r.Use(middlewares.SecurityHeaders()) // Good to have
	// r.Use(middlewares.RateLimitMiddleware()) // Good to have

	// --- 4. Route Registration ---

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "OK"})
	})

	// Auth routes, now using the controller methods
	authRoutes := r.Group("/auth")
	{
		authRoutes.POST("/register", authController.Register)
		authRoutes.POST("/login", authController.Login)
		authRoutes.POST("/verify-email", authController.VerifyEmail)
		authRoutes.POST("/logout", authController.Logout)
		authRoutes.POST("/refresh", authController.Refresh)     // Added the refresh route
		authRoutes.GET("/status", authController.GetAuthStatus) // Added the status route
	}

	// --- 5. Graceful Shutdown ---

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081" // Default port for auth-service
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		zap.L().Info("Auth Service started", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("Server error", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	zap.L().Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		zap.L().Fatal("Server forced to shutdown", zap.Error(err))
	}

	// Close database connection
	if err := database.Close(); err != nil {
		zap.L().Error("Failed to close database", zap.Error(err))
	}

	zap.L().Info("Server exited gracefully")
}
