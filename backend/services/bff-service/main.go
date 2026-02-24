package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"bff-service/clients"
	"bff-service/config"
	"bff-service/controllers"
	"bff-service/routes"

	"github.com/redis/go-redis/v9"

	"github.com/gin-gonic/gin"
	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()

	// ── CloudWatch Logs + Metrics ──
	var metricsClient *awspkg.MetricsClient
	if os.Getenv("CLOUDWATCH_ENABLED") == "true" {
		cwCtx := context.Background()
		cwLogs, err := awspkg.NewCloudWatchLogsClient(cwCtx, "bff-service")
		if err != nil {
			log.Printf("[BFF] CloudWatch Logs init failed: %v", err)
		} else {
			log.Println("[BFF] CloudWatch Logs enabled")
			_ = cwLogs // writes happen via logger; keep reference
		}
		mc, err := awspkg.NewMetricsClient(cwCtx)
		if err != nil {
			log.Printf("[BFF] CloudWatch Metrics init failed: %v", err)
		} else {
			metricsClient = mc
			log.Println("[BFF] CloudWatch Metrics enabled")
		}
	}

	timeout, err := time.ParseDuration(cfg.RequestTimeout)
	if err != nil {
		timeout = 10 * time.Second
	}

	gateway := clients.NewGatewayClient(cfg.APIGatewayURL, timeout)

	// Initialize Redis (optional - for idempotency)
	var redisClient *redis.Client
	if addr := os.Getenv("REDIS_URL"); addr != "" {
		opts, err := redis.ParseURL(addr)
		if err != nil {
			log.Fatalf("invalid REDIS_URL: %v", err)
		}
		redisClient = redis.NewClient(opts)
		if err := redisClient.Ping(context.Background()).Err(); err != nil {
			log.Fatalf("failed to connect to Redis: %v", err)
		}
		log.Println("Connected to Redis (BFF)")
	}

	controller := controllers.NewBFFController(gateway, redisClient)

	r := gin.New()
	r.Use(gin.Recovery())

	// Initialize structured logger for request logging
	zapLogger, _ := zap.NewProduction()
	defer zapLogger.Sync()

	// Structured HTTP request logging → CloudWatch
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
			zapLogger.Error("http_request", fields...)
		case status >= 400:
			zapLogger.Warn("http_request", fields...)
		default:
			zapLogger.Info("http_request", fields...)
		}
	})

	// ── HTTP metrics middleware (inline) ──
	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if metricsClient != nil {
			go func(path, method string, status int, dur time.Duration) {
				mctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				dims := map[string]string{"Service": "bff-service", "Method": method, "Path": path}
				_ = metricsClient.RecordCount(mctx, awspkg.MetricHTTPRequests, dims)
				_ = metricsClient.RecordLatency(mctx, awspkg.MetricHTTPLatency, dur, dims)
				if status >= 500 {
					_ = metricsClient.RecordCount(mctx, awspkg.MetricHTTPErrors, dims)
				}
			}(c.Request.URL.Path, c.Request.Method, c.Writer.Status(), time.Since(start))
		}
	})

	r.GET("/docs", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, "<!doctype html><html><head><title>API Docs</title><link rel=\"stylesheet\" href=\"https://unpkg.com/swagger-ui-dist@5/swagger-ui.css\"></head><body><div id=\"swagger-ui\"></div><script src=\"https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js\"></script><script>window.onload=function(){SwaggerUIBundle({url:'/docs/openapi.yaml',dom_id:'#swagger-ui'});};</script></body></html>")
	})
	r.GET("/docs/openapi.yaml", func(c *gin.Context) {
		c.File("/docs/openapi.yaml")
	})

	routes.RegisterRoutes(r, controller)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("[BFF] listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[BFF] server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[BFF] shutdown error: %v", err)
	}
}
