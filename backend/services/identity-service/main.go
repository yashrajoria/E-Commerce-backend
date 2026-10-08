package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	authcontrollers "identity-service/auth/controllers"
	authservices "identity-service/auth/services"
	"identity-service/database"
	"identity-service/middleware"
	"identity-service/repository"
	usercontrollers "identity-service/users/controllers"
	userroutes "identity-service/users/routes"
	userservices "identity-service/users/services"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/yashrajoria/common/internalauth"
	commonlog "github.com/yashrajoria/common/logger"
	commonmw "github.com/yashrajoria/common/middleware"
	commondb "github.com/yashrajoria/common/db"
	"github.com/yashrajoria/common/telemetry"
	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	// Initialize common logger to avoid nil pointer panics in sub-packages
	commonlog.Log = logger

	_ = godotenv.Load()

	// --- OpenTelemetry (optional, no-op when OTEL_EXPORTER_OTLP_ENDPOINT unset) ---
	shutdownTelemetry := telemetry.Init(context.Background(), "identity-service")
	defer shutdownTelemetry()

	cfg, err := LoadConfig()
	if err != nil {
		zap.L().Fatal("Failed to load config", zap.Error(err))
	}

	// Unset means internalauth.Require() fail-closes with 503 on internal-only
	// routes — surface it at boot.
	if internalauth.Token() == "" {
		zap.L().Warn("INTERNAL_SERVICE_TOKEN is not set — internal-only routes will reject all requests")
	}

	// Single connection, single migration for the whole identity schema
	// (users, refresh_tokens, addresses).
	if err := database.Connect(); err != nil {
		zap.L().Fatal("Database connection failed", zap.Error(err))
	}

	purgeCtx, stopPurge := context.WithCancel(context.Background())
	defer stopPurge()
	commondb.StartPurger(purgeCtx, database.DB, time.Hour,
		commondb.PurgeJob{Table: "refresh_tokens", Where: "expires_at < now() - interval '1 day'"},
	)

	snsPublisher := authservices.NewSNSPublisherWithDB(database.DB)

	// --- Dependency Injection ---

	// One repository instance serves both domains — same tables, same connection.
	userRepo := repository.NewUserRepository(database.DB)

	tokenService := authservices.NewTokenService()
	authService := authservices.NewAuthService(userRepo, tokenService, snsPublisher, database.DB)

	if err := authService.BootstrapAdminFromEnv(context.Background()); err != nil {
		zap.L().Fatal("Admin bootstrap failed", zap.Error(err))
	}

	// Password changes revoke refresh tokens via direct in-process call —
	// the old user-service → auth-service HTTP hop (AUTH_SERVICE_URL +
	// /auth/internal/revoke-tokens) is gone.
	userService := userservices.NewUserService(userRepo, authService)

	authController := authcontrollers.NewAuthController(authService)
	userController := usercontrollers.NewUserController(userService)

	// --- HTTP Server & Middleware ---
	r := gin.New()
	r.Use(gin.Recovery())
	// OpenTelemetry server spans (no-op when telemetry disabled) — before all
	// other middleware so traces span the full request lifecycle.
	r.Use(telemetry.GinMiddleware("identity-service"))

	r.Use(commonmw.MetricsMiddleware(nil, "identity-service"))
	r.Use(commonmw.RequestLogger(logger))

	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	r.Use(commonmw.SecurityHeaders())

	// --- Route Registration ---

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/health/live", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/health/ready", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ready"}) })

	// Auth routes (ex-auth-service, paths unchanged).
	authRoutes := r.Group("/auth")
	{
		// Defense-in-depth: api-gateway already rate-limits these paths, but
		// identity-service enforces its own per-IP limit too, so brute-forcing
		// login/register/refresh directly (bypassing the gateway) is also throttled.
		bruteForceLimit := commonmw.RateLimitMiddleware()
		authRoutes.POST("/register", bruteForceLimit, authController.Register)
		authRoutes.POST("/login", bruteForceLimit, authController.Login)
		authRoutes.POST("/verify-email", authController.VerifyEmail)
		authRoutes.POST("/resend-verification", authController.ResendVerificationEmail)
		authRoutes.POST("/logout", authController.Logout)
		authRoutes.POST("/refresh", bruteForceLimit, authController.Refresh)

		// Reached only through api-gateway (which always attaches the internal
		// mesh token when forwarding); reject direct hits so no one can spoof
		// X-User-Role by talking to identity-service straight off the network.
		authRoutes.GET("/status", internalauth.Require(), authController.GetAuthStatus)

		// Admin routes (defense-in-depth; gateway also requires admin JWT)
		authRoutes.POST("/admin/users", middleware.AdminOnly(), authController.AdminCreateUser)
	}

	// User profile routes (ex-user-service, paths unchanged).
	userRoutes := r.Group("/users")
	userRoutes.Use(middleware.AuthMiddleware())
	userroutes.RegisterUserRoutes(userRoutes, userController)

	adminUserRoutes := r.Group("/users")
	adminUserRoutes.Use(middleware.AuthMiddleware(), middleware.AdminOnly())
	userroutes.RegisterAdminRoutes(adminUserRoutes, userController)

	// --- Graceful Shutdown ---

	port := cfg.Port

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		zap.L().Info("Identity Service started", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("Server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	zap.L().Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		zap.L().Fatal("Server forced to shutdown", zap.Error(err))
	}

	if err := database.Close(); err != nil {
		zap.L().Error("Failed to close database", zap.Error(err))
	}

	zap.L().Info("Server exited gracefully")
}
