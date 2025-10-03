package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/api-gateway/logger"
	"github.com/yashrajoria/api-gateway/routes"
	"go.uber.org/zap"
)

func main() {
	logger.InitLogger()
	defer logger.Sync()

	logger.Log.Info("Starting API Gateway...")

	r := gin.New()

	// Global middleware
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// CORS middleware
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Register routes from modular route packages
	routes.RegisterAllRoutes(r)

	// Read port from env or default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// --- Graceful server shutdown setup ---
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	// Start the server in a goroutine so it doesn't block.
	go func() {
		logger.Log.Info("API Gateway listening on port", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for an interrupt signal to gracefully shut down the server.
	quit := make(chan os.Signal, 1)
	// signal.Notify listens for the specified signals and sends them to the channel.
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit // This blocks until a signal is received.
	logger.Log.Info("Shutting down API Gateway...")

	// Create a context with a 5-second timeout to allow existing connections to finish.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Log.Fatal("API Gateway forced to shutdown:", zap.Error(err))
	}

	logger.Log.Info("API Gateway exiting gracefully")
}
