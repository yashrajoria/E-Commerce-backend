package main

import (
	"api-gateway/logger"
	"api-gateway/routes"
	"context"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"go.uber.org/zap"
)

// CORS Middleware - Apply this globally
func CORSMiddleware() gin.HandlerFunc {
	// Use gin-contrib/cors with configuration from ALLOWED_ORIGINS
	allowed := os.Getenv("ALLOWED_ORIGINS")
	config := cors.Config{
		AllowCredentials: true,
		AllowMethods:     []string{"POST", "HEAD", "PATCH", "OPTIONS", "GET", "PUT", "DELETE"},
		AllowHeaders:     []string{"Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization", "Accept", "Origin", "Cache-Control", "X-Requested-With"},
	}

	if allowed == "*" {
		config.AllowAllOrigins = true
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
	// Initialize logger
	logger.InitLogger()
	defer logger.Sync()
	logger.Log.Info("Starting API Gateway...")

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

	r.Use(gin.Logger())
	r.Use(CustomRecovery(logger.Log))

	r.Use(CORSMiddleware())

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

	// Health check / Test route for CORS
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "API Gateway is running"})
	})

	r.GET("/test-cors", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "CORS is working!"})
	})
	// Structured HTTP request logging → CloudWatch via Zap writer
	r.Use(func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()
		fields := []zap.Field{
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.Int("body_size", c.Writer.Size()),
		}
		switch {
		case status >= 500:
			logger.Log.Error("http_request", fields...)
		case status >= 400:
			logger.Log.Warn("http_request", fields...)
		default:
			logger.Log.Info("http_request", fields...)
		}
	})

	routes.RegisterAllRoutes(r)

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
