module identity-service

go 1.25

require (
	github.com/gin-gonic/gin v1.10.1
	github.com/golang-jwt/jwt/v4 v4.5.1
	github.com/google/uuid v1.6.0
	github.com/joho/godotenv v1.5.1
	golang.org/x/crypto v0.46.0
	golang.org/x/time v0.11.0
	gorm.io/gorm v1.31.1
)

require github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws v0.0.0

require github.com/yashrajoria/common v0.0.0

replace github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws => ../../pkg/aws

replace github.com/yashrajoria/common => ../common

require (
	go.uber.org/zap v1.27.1
	gorm.io/driver/postgres v1.6.0
)
