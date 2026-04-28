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
		Log, err = zap.NewDevelopment()
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
