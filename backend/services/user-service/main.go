package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "user-service/database"
    "user-service/middleware"
    "user-service/models"
    "user-service/routes"

    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    log.Println("Starting User Service...")

    cfg, err := LoadConfig()
    if err != nil {
        logger.Fatal("Failed to load config", zap.Error(err))
    }

    if err := database.Connect(); err != nil {
        logger.Fatal("Database connection failed", zap.Error(err))
    }

    if os.Getenv("ENV") != "production" {
        if err := models.Migrate(database.DB); err != nil {
            logger.Fatal("Migration failed", zap.Error(err))
        }
    }

    r := gin.New()
    r.Use(gin.Recovery())

    // CORS setup (could move allowedOrigins to config)
    allowedOrigins := map[string]bool{
        "http://localhost:3000":  true,
        "https://yourdomain.com": true,
    }
    r.Use(func(c *gin.Context) {
        origin := c.Request.Header.Get("Origin")
        if !allowedOrigins[origin] {
            origin = "http://localhost:3000"
        }
        c.Header("Access-Control-Allow-Origin", origin)
        c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
        c.Header("Access-Control-Allow-Credentials", "true")
        if c.Request.Method == "OPTIONS" {
            c.AbortWithStatus(http.StatusOK)
            return
        }
        c.Next()
    })

    r.GET("/health", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"status": "OK"})
    })

    // User routes with authentication middleware
    userRoutes := r.Group("/users")
    userRoutes.Use(middleware.AuthMiddleware())
    routes.RegisterUserRoutes(userRoutes)

    port := cfg.Port
    if port == "" {
        port = "8085"
    }

    srv := &http.Server{
        Addr:    ":" + port,
        Handler: r,
    }

    go func() {
        logger.Info("User Service started", zap.String("port", port))
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logger.Fatal("Server error", zap.Error(err))
        }
    }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    logger.Info("Shutting down server...")
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := srv.Shutdown(ctx); err != nil {
        logger.Fatal("Server forced to shutdown", zap.Error(err))
    }
    logger.Info("Server exited cleanly")
}
