package main

import (
	"api-gateway/logger"
	"api-gateway/middlewares"
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
	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	apperrors "github.com/yashrajoria/common/errors"
	"go.uber.org/zap"
)

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
		config.AllowOrigins = []string{"http://localhost:3000", "http://localhost:3001", "https://shopswift-storefront.vercel.app", "https://shopswift-admin.vercel.app"}
	} else if allowed != "" {
		var origins []string
		for _, o := range strings.Split(allowed, ",") {
			origins = append(origins, strings.TrimSpace(o))
		}
		config.AllowOrigins = origins
	} else {
		config.AllowOrigins = []string{"http://localhost:3000", "http://localhost:3001", "https://shopswift-storefront.vercel.app", "https://shopswift-admin.vercel.app"}
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

	if err := middlewares.InitJWTConfig(); err != nil {
		logger.Log.Fatal("JWT middleware init failed", zap.Error(err))
	}

	// --- CloudWatch (Logs + Metrics) ---
	cwLogsClient, err := awspkg.NewCloudWatchLogsClient(context.Background(), "api-gateway")
	if err != nil {
		logger.Log.Warn("CloudWatch logs client init failed (non-fatal)", zap.Error(err))
	}
	_ = cwLogsClient

	metricsClient, err := awspkg.NewMetricsClient(context.Background())
	if err != nil {
		logger.Log.Warn("CloudWatch metrics client init failed (non-fatal)", zap.Error(err))
	}

	r := gin.New()

	// Configure Gin to handle trailing slashes
	r.RedirectTrailingSlash = true

	// SECURITY: Gin's default trusted-proxy setting trusts everything and derives
	// ClientIP() from X-Forwarded-For, which lets a caller spoof a fresh IP on every
	// request and bypass the rate limiters below. There is no reverse proxy/load
	// balancer in front of this gateway by default, so trust nothing unless the
	// deployment explicitly configures one via TRUSTED_PROXIES (comma-separated CIDRs).
	var trustedProxies []string
	if tp := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")); tp != "" {
		for _, p := range strings.Split(tp, ",") {
			if p = strings.TrimSpace(p); p != "" {
				trustedProxies = append(trustedProxies, p)
			}
		}
	}
	if err := r.SetTrustedProxies(trustedProxies); err != nil {
		logger.Log.Fatal("Failed to set trusted proxies", zap.Error(err))
	}

	r.Use(middlewares.RequestIDMiddleware())
	r.Use(CustomRecovery(logger.Log))
	r.Use(CORSMiddleware())
	r.Use(apperrors.ErrorMiddleware())
	r.Use(middlewares.StructuredRequestLogger())

	// CloudWatch HTTP metrics middleware
	if metricsClient != nil && metricsClient.IsEnabled() {
		r.Use(func(c *gin.Context) {
			start := time.Now()
			c.Next()
			go func(path, method string, status int, dur time.Duration) {
				mctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				dims := map[string]string{"Service": "api-gateway", "Method": method, "Path": path}
				_ = metricsClient.RecordCount(mctx, awspkg.MetricHTTPRequests, dims)
				_ = metricsClient.RecordLatency(mctx, awspkg.MetricHTTPLatency, dur, dims)
				if status >= 400 {
					_ = metricsClient.RecordCount(mctx, awspkg.MetricHTTPErrors, dims)
				}
			}(c.Request.URL.Path, c.Request.Method, c.Writer.Status(), time.Since(start))
		})
	}

	r.GET("/test-cors", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "CORS is working!"})
	})

	// Initialize Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis:6379"
	}
	redisClient := redis.NewClient(&redis.Options{Addr: redisURL})
	defer redisClient.Close()

	routes.RegisterAllRoutes(r, redisClient)

	// Server setup
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
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
