package controllers

import (
	"auth-service/services"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mock Service ---
type MockAuthService struct {
	mock.Mock
}

func (m *MockAuthService) Login(ctx context.Context, email, password string) (*services.TokenPair, error) {
	args := m.Called(ctx, email, password)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.TokenPair), args.Error(1)
}
func (m *MockAuthService) Register(ctx context.Context, name, email, password, role string) error {
	args := m.Called(ctx, name, email, password, role)
	return args.Error(0)
}
func (m *MockAuthService) VerifyEmail(ctx context.Context, email, code string) error {
	args := m.Called(ctx, email, code)
	return args.Error(0)
}
func (m *MockAuthService) RefreshTokens(ctx context.Context, refreshToken string) (*services.TokenPair, error) {
	args := m.Called(ctx, refreshToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.TokenPair), args.Error(1)
}

// --- Tests ---

func TestLoginController(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Success - 200 OK", func(t *testing.T) {
		// Arrange
		mockService := new(MockAuthService)
		authController := NewAuthController(mockService)

		expectedTokenPair := &services.TokenPair{AccessToken: "fake-access-token", RefreshToken: "fake-refresh-token"}
		mockService.On("Login", mock.Anything, "test@example.com", "password123").Return(expectedTokenPair, nil).Once()

		router := gin.New()
		router.POST("/login", authController.Login)

		payload := `{"email": "test@example.com", "password": "password123"}`
		req, _ := http.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		// Act
		router.ServeHTTP(recorder, req)

		// Assert
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "Logged in successfully")
		cookies := recorder.Result().Cookies()
		assert.Len(t, cookies, 2)
		mockService.AssertExpectations(t)
	})

	t.Run("Failure - Invalid Credentials - 401 Unauthorized", func(t *testing.T) {
		// Arrange
		mockService := new(MockAuthService)
		authController := NewAuthController(mockService)
		mockService.On("Login", mock.Anything, "test@example.com", "wrongpassword").Return(nil, errors.New("invalid email or password")).Once()

		router := gin.New()
		router.POST("/login", authController.Login)

		payload := `{"email": "test@example.com", "password": "wrongpassword"}`
		req, _ := http.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		// Act
		router.ServeHTTP(recorder, req)

		// Assert
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "invalid email or password")
		mockService.AssertExpectations(t)
	})

	t.Run("Failure - Bad Request Body - 400 Bad Request", func(t *testing.T) {
		// Arrange
		mockService := new(MockAuthService)
		authController := NewAuthController(mockService)
		router := gin.New()
		router.POST("/login", authController.Login)

		payload := `{"email": "test@example.com"}` // Missing password
		req, _ := http.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		// Act
		router.ServeHTTP(recorder, req)

		// Assert
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		mockService.AssertNotCalled(t, "Login")
	})
}
