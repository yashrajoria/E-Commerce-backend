package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// Log is the global logger instance
	Log *zap.Logger
)

// RequestIDKey is the key used to store request ID in context
const RequestIDKey = "request_id"

// Initialize sets up the logger with the specified environment
func Initialize(env string) {
	InitializeWithWriter(env, nil)
}

// InitializeWithWriter sets up the logger with the specified environment and optional CloudWatch writer
func InitializeWithWriter(env string, cloudWatchWriter io.Writer) {
	var config zap.Config

	if env == "production" {
		config = zap.NewProductionConfig()
		config.EncoderConfig.TimeKey = "timestamp"
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	var err error

	// If CloudWatch writer is provided, add it as a sink
	if cloudWatchWriter != nil {
		// Create encoder
		encoder := zapcore.NewJSONEncoder(config.EncoderConfig)

		// Create console syncer (stdout/stderr)
		consoleEncoder := zapcore.NewConsoleEncoder(config.EncoderConfig)
		consoleLevel := zap.NewAtomicLevelAt(config.Level.Level())
		consoleSyncer := zapcore.AddSync(os.Stdout)
		consoleCore := zapcore.NewCore(consoleEncoder, consoleSyncer, consoleLevel)

		// Create CloudWatch syncer
		cwLevel := zap.NewAtomicLevelAt(config.Level.Level())
		cwSyncer := zapcore.AddSync(cloudWatchWriter)
		cwCore := zapcore.NewCore(encoder, cwSyncer, cwLevel)

		// Combine both cores
		core := zapcore.NewTee(consoleCore, cwCore)
		Log = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	} else {
		// Standard initialization without CloudWatch
		Log, err = config.Build()
		if err != nil {
			fmt.Printf("Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
	}
}

// RequestLogger returns a gin middleware that logs request details
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Generate request ID
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("%d", time.Now().UnixNano())
		}
		c.Set(RequestIDKey, requestID)

		// Process request
		c.Next()

		// Log request details
		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		Log.Info("Request completed",
			zap.String("request_id", requestID),
			zap.String("method", method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", statusCode),
			zap.String("ip", clientIP),
			zap.Duration("latency", latency),
			zap.String("user_agent", c.Request.UserAgent()),
		)
	}
}

// Error logs an error with request ID and additional context
func Error(ctx context.Context, msg string, err error, fields ...zap.Field) {
	requestID := getRequestID(ctx)
	fields = append(fields, zap.String("request_id", requestID))
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	Log.Error(msg, fields...)
}

// Info logs an info message with request ID and additional context
func Info(ctx context.Context, msg string, fields ...zap.Field) {
	requestID := getRequestID(ctx)
	fields = append(fields, zap.String("request_id", requestID))
	Log.Info(msg, fields...)
}

// Debug logs a debug message with request ID and additional context
func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	requestID := getRequestID(ctx)
	fields = append(fields, zap.String("request_id", requestID))
	Log.Debug(msg, fields...)
}

// Warn logs a warning message with request ID and additional context
func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	requestID := getRequestID(ctx)
	fields = append(fields, zap.String("request_id", requestID))
	Log.Warn(msg, fields...)
}

// getRequestID extracts request ID from context
func getRequestID(ctx context.Context) string {
	if ginCtx, ok := ctx.(*gin.Context); ok {
		if requestID, exists := ginCtx.Get(RequestIDKey); exists {
			return requestID.(string)
		}
	}
	return "unknown"
}

// WithContext creates a new context with the given request ID
func WithContext(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}
