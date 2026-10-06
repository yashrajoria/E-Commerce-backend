package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// Register mounts /health, /health/live, and /health/ready.
// /health aliases live for backward compatibility.
// readyChecks are optional; if any fail, ready returns 503.
func Register(r gin.IRoutes, service string, readyChecks ...func(ctx context.Context) error) {
	live := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": service})
	}
	r.GET("/health", live)
	r.GET("/health/live", live)
	r.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		for _, check := range readyChecks {
			if check == nil {
				continue
			}
			if err := check(ctx); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"status":  "not_ready",
					"service": service,
					"error":   err.Error(),
				})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready", "service": service})
	})
}

// PostgresCheck pings the database.
func PostgresCheck(db *gorm.DB) func(context.Context) error {
	return func(ctx context.Context) error {
		if db == nil {
			return errNotConfigured("postgres")
		}
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		return sqlDB.PingContext(ctx)
	}
}

// RedisCheck pings Redis.
func RedisCheck(client *redis.Client) func(context.Context) error {
	return func(ctx context.Context) error {
		if client == nil {
			return errNotConfigured("redis")
		}
		return client.Ping(ctx).Err()
	}
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func errNotConfigured(name string) error {
	return simpleError(name + " not configured")
}
