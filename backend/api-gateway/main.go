package main

import (
	"api-gateway/logger"
	"api-gateway/middleware"
	"api-gateway/routes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	apperrors "github.com/yashrajoria/common/errors"
	"github.com/yashrajoria/common/internalauth"
	commonmw "github.com/yashrajoria/common/middleware"
	"github.com/yashrajoria/common/telemetry"
	"go.uber.org/zap"
)

// defaultAllowedOrigins is the fallback CORS origin list used when
// ALLOWED_ORIGINS is unset or "*" (wildcard can't be combined with
// AllowCredentials). Kept as a named default so the fallback is
// documented and easy to find, not just inline magic strings.
var defaultAllowedOrigins = []string{
	"http://localhost:3000",
	"http://localhost:3001",
	"https://shopswift-storefront.vercel.app",
	"https://shopswift-admin.vercel.app",
}

// CORS Middleware - Apply this globally
func CORSMiddleware() gin.HandlerFunc {
	// Use gin-contrib/cors with configuration from ALLOWED_ORIGINS
	allowed := os.Getenv("ALLOWED_ORIGINS")
	config := cors.Config{
		AllowCredentials: true,
		AllowMethods:     []string{"POST", "HEAD", "PATCH", "OPTIONS", "GET", "PUT", "DELETE"},
		AllowHeaders:     []string{"Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization", "Accept", "Origin", "Cache-Control", "X-Requested-With", "X-Request-ID", "X-Correlation-ID", "Idempotency-Key"},
	}

	if allowed == "*" {
		// Do not combine wildcard CORS with credentialed cookies. Fall back to
		// explicit trusted origins so browser-enforced auth cookies remain safe.
		logger.Log.Warn("ALLOWED_ORIGINS=* cannot be combined with credentialed cookies; falling back to default origin allowlist", zap.Strings("origins", defaultAllowedOrigins))
		config.AllowOrigins = defaultAllowedOrigins
	} else if allowed != "" {
		var origins []string
		for _, o := range strings.Split(allowed, ",") {
			origins = append(origins, strings.TrimSpace(o))
		}
		config.AllowOrigins = origins
	} else {
		logger.Log.Warn("ALLOWED_ORIGINS not set; falling back to default origin allowlist", zap.Strings("origins", defaultAllowedOrigins))
		config.AllowOrigins = defaultAllowedOrigins
	}

	return cors.New(config)
}

// CustomRecovery recovers from panics and logs them
func CustomRecovery(zlogger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				stack := debug.Stack()
				zlogger.Error("Panic recovered", zap.Any("error", err), zap.ByteString("stack", stack))
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			}
		}()
		c.Next()
	}
}

func main() {
	_ = godotenv.Load()

	if mode := os.Getenv("GIN_MODE"); mode != "" {
		gin.SetMode(mode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize logger
	if err := logger.InitLogger(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()
	logger.Log.Info("Starting API Gateway...")

	// --- OpenTelemetry (optional, no-op when OTEL_EXPORTER_OTLP_ENDPOINT unset) ---
	shutdownTelemetry := telemetry.Init(context.Background(), "api-gateway")
	defer shutdownTelemetry()

	// INTERNAL_SERVICE_TOKEN unset means internalauth.Apply() silently no-ops on
	// every forwarded request — downstream services will then reject them at
	// internalauth.Require(). Surface that misconfiguration at boot, not first request.
	if internalauth.Token() == "" {
		logger.Log.Warn("INTERNAL_SERVICE_TOKEN is not set — requests to internal-auth-gated downstream routes will be rejected")
	}

	if err := middleware.InitJWTConfig(); err != nil {
		logger.Log.Fatal("JWT middleware init failed", zap.Error(err))
	}

	r := gin.New()

	// OpenTelemetry server spans (no-op when telemetry disabled) — before all
	// other middleware so traces span the full request lifecycle.
	r.Use(telemetry.GinMiddleware("api-gateway"))

	// Configure Gin to handle trailing slashes
	r.RedirectTrailingSlash = true

	// The gateway is the edge — trust X-Forwarded-For only from a configured
	// reverse proxy (or nobody, by default). Without this, gin.ClientIP()
	// trusts client-supplied X-Forwarded-For, letting anyone spoof the IP
	// used for rate limiting and audit logging.
	if proxies := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")); proxies != "" {
		trusted := strings.Split(proxies, ",")
		for i := range trusted {
			trusted[i] = strings.TrimSpace(trusted[i])
		}
		if err := r.SetTrustedProxies(trusted); err != nil {
			logger.Log.Fatal("invalid TRUSTED_PROXIES", zap.Error(err))
		}
	} else if err := r.SetTrustedProxies(nil); err != nil {
		logger.Log.Fatal("failed to disable trusted proxies", zap.Error(err))
	}

	r.Use(middleware.RequestIDMiddleware())
	r.Use(CustomRecovery(logger.Log))
	r.Use(CORSMiddleware())
	r.Use(commonmw.SecurityHeaders())
	r.Use(apperrors.ErrorMiddleware())
	r.Use(middleware.StructuredRequestLogger())

	if gin.Mode() != gin.ReleaseMode {
		r.GET("/test-cors", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "CORS is working!"})
		})
	}

	// Initialize Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis:6379"
	}
	var redisClient *redis.Client
	if opts, err := redis.ParseURL(redisURL); err == nil {
		redisClient = redis.NewClient(opts)
	} else {
		// Not a redis:// URI (e.g. plain "host:port") — use it as Addr directly.
		redisClient = redis.NewClient(&redis.Options{Addr: redisURL})
	}
	defer redisClient.Close()

	routes.RegisterAllRoutes(r, redisClient)

	// Server setup
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Start server
	go func() {
		logger.Log.Info("API Gateway listening on port", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Log.Info("Shutting down API Gateway...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Log.Fatal("API Gateway forced to shutdown:", zap.Error(err))
	}

	logger.Log.Info("API Gateway exited gracefully")
}
