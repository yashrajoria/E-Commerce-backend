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
	"auth-service/middleware"
	"auth-service/repository"
	"auth-service/services"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"github.com/yashrajoria/common/internalauth"
	commonmw "github.com/yashrajoria/common/middleware"
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

	// Unset means internalauth.Require() fail-closes with 503 on every call to
	// /auth/internal/revoke-tokens and /auth/status — surface it at boot.
	if internalauth.Token() == "" {
		zap.L().Warn("INTERNAL_SERVICE_TOKEN is not set — internal-only routes will reject all requests")
	}

	// Connect to the database (AutoMigrate for User/RefreshToken when ALLOW_AUTO_MIGRATE=true)
	if err := database.Connect(); err != nil {
		zap.L().Fatal("Database connection failed", zap.Error(err))
	}
	snsPublisher, err := services.NewSNSPublisher(context.Background())
	if err != nil {
		logger.Warn("SNS publisher unavailable, email notifications disabled", zap.Error(err))
		snsPublisher = nil
	}
	// --- 2. Dependency Injection (Wiring the layers) ---

	// Initialize Repositories
	userRepo := repository.NewUserRepository(database.DB)

	// Initialize Services
	tokenService := services.NewTokenService()
	// emailService := services.NewEmailService()
	authService := services.NewAuthService(userRepo, tokenService, snsPublisher, database.DB)

	if err := authService.BootstrapAdminFromEnv(context.Background()); err != nil {
		zap.L().Fatal("Admin bootstrap failed", zap.Error(err))
	}

	// Initialize Controllers
	authController := controllers.NewAuthController(authService)

	// --- 3. HTTP Server & Middleware ---

	// --- CloudWatch (Logs + Metrics) ---
	cwLogsClient, err := awspkg.NewCloudWatchLogsClient(context.Background(), "auth-service")
	if err != nil {
		zap.L().Warn("CloudWatch logs client init failed (non-fatal)", zap.Error(err))
	}
	_ = cwLogsClient

	metricsClient, err := awspkg.NewMetricsClient(context.Background())
	if err != nil {
		zap.L().Warn("CloudWatch metrics client init failed (non-fatal)", zap.Error(err))
	}

	r := gin.New()
	r.Use(gin.Recovery()) // Panic protection

	// CloudWatch HTTP metrics middleware
	if metricsClient != nil {
		r.Use(commonmw.MetricsMiddleware(metricsClient, "auth-service"))
	}

	// Structured HTTP request logging → CloudWatch via Zap writer
	r.Use(commonmw.RequestLogger(logger))

	// Add request timeout middleware
	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	r.Use(commonmw.SecurityHeaders())

	// --- 4. Route Registration ---

	// Health check
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/health/live", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/health/ready", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ready"}) })

	// Auth routes, now using the controller methods
	authRoutes := r.Group("/auth")
	{
		// Defense-in-depth: api-gateway already rate-limits these paths, but
		// auth-service enforces its own per-IP limit too, so brute-forcing
		// login/register/refresh directly (bypassing the gateway) is also throttled.
		bruteForceLimit := commonmw.RateLimitMiddleware()
		authRoutes.POST("/register", bruteForceLimit, authController.Register)
		authRoutes.POST("/login", bruteForceLimit, authController.Login)
		authRoutes.POST("/verify-email", authController.VerifyEmail)
		authRoutes.POST("/resend-verification", authController.ResendVerificationEmail)
		authRoutes.POST("/logout", authController.Logout)
		authRoutes.POST("/refresh", bruteForceLimit, authController.Refresh) // Added the refresh route

		// Reached only through api-gateway (which always attaches the internal
		// mesh token when forwarding); reject direct hits so no one can spoof
		// X-User-Role by talking to auth-service straight off the network.
		authRoutes.GET("/status", internalauth.Require(), authController.GetAuthStatus)

		// Admin routes (defense-in-depth; gateway also requires admin JWT)
		authRoutes.POST("/admin/users", middlewares.AdminOnly(), authController.AdminCreateUser)

		// Internal-only: called by user-service after a password change to
		// revoke every outstanding refresh token for the account, so a
		// stolen session can't outlive the password that issued it.
		authRoutes.POST("/internal/revoke-tokens", internalauth.Require(), authController.RevokeUserTokens)
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
