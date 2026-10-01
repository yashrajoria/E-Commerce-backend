package logger

import (
	"os"

	"go.uber.org/zap"
)

var Log *zap.Logger

func InitLogger() error {
	if Log != nil {
		return nil
	}

	env := os.Getenv("APP_ENV")
	var err error
	if env == "production" {
		Log, err = zap.NewProduction()
	} else {
		// Development console output, but only attach stack traces on
		// Error+ (default NewDevelopment stacks on Warn+, which turns
		// every expected 401/404 into multi-line stack noise).
		cfg := zap.NewDevelopmentConfig()
		cfg.DisableStacktrace = false
		cfg.Level.SetLevel(zap.DebugLevel)
		Log, err = cfg.Build(zap.AddStacktrace(zap.ErrorLevel))
	}
	if err != nil {
		return err
	}

	return nil
}

func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}
