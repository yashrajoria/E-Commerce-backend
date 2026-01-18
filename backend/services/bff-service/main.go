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

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	timeout, err := time.ParseDuration(cfg.RequestTimeout)
	if err != nil {
		timeout = 10 * time.Second
	}

	gateway := clients.NewGatewayClient(cfg.APIGatewayURL, timeout)
	controller := controllers.NewBFFController(gateway)

	r := gin.New()
	r.Use(gin.Recovery())

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
