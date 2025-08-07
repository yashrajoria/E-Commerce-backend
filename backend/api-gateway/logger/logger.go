package logger

import (
	"os"

	"go.uber.org/zap"
)

var Log *zap.Logger

func InitLogger() {
	if Log != nil {
		return
	}

	env := os.Getenv("APP_ENV")
	var err error
	if env == "production" {
		Log, err = zap.NewProduction()
	} else {
		Log, err = zap.NewDevelopment()
	}
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
}

func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}
