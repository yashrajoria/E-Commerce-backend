package services

import (
	"context"
	"testing"

	"auth-service/models"

	"github.com/golang-jwt/jwt/v4" // <-- Make sure this import is present
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// --- Mocks for Dependencies ---

type MockUserRepository struct{ mock.Mock }

func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *MockUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *MockUserRepository) Create(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}
func (m *MockUserRepository) Update(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

type MockTokenService struct{ mock.Mock }

func (m *MockTokenService) GenerateTokenPair(userID, email, role string) (*TokenPair, error) {
	args := m.Called(userID, email, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*TokenPair), args.Error(1)
}

// === THIS IS THE FIX ===
// The return type of the mock method must match the interface exactly.
func (m *MockTokenService) ValidateToken(tokenStr, expectedType string) (jwt.MapClaims, error) {
	args := m.Called(tokenStr, expectedType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	// The type assertion here must also be updated to jwt.MapClaims
	return args.Get(0).(jwt.MapClaims), args.Error(1)
}

type MockEmailService struct{ mock.Mock }

func (m *MockEmailService) SendVerificationEmail(email, code string) error {
	args := m.Called(email, code)
	return args.Error(0)
}

// --- Tests ---

func TestLogin(t *testing.T) {
	mockRepo := new(MockUserRepository)
	mockTokenService := new(MockTokenService)
	authService := NewAuthService(mockRepo, mockTokenService, nil, nil)
	ctx := context.Background()

	password := "strongpassword123"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	testUser := &models.User{
		ID:            uuid.New(),
		Email:         "test@example.com",
		Password:      string(hashedPassword),
		Role:          "user",
		EmailVerified: true,
	}

	t.Run("Success", func(t *testing.T) {
		// Arrange
		mockRepo.On("FindByEmail", ctx, testUser.Email).Return(testUser, nil).Once()
		mockTokenService.On("GenerateTokenPair", testUser.ID.String(), testUser.Email, testUser.Role).Return(&TokenPair{"access", "refresh"}, nil).Once()

		// Act
		tokenPair, err := authService.Login(ctx, testUser.Email, password)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, tokenPair)
		assert.Equal(t, "access", tokenPair.AccessToken)
		mockRepo.AssertExpectations(t)
		mockTokenService.AssertExpectations(t)
	})

	t.Run("User Not Found", func(t *testing.T) {
		// Arrange
		mockRepo.On("FindByEmail", ctx, "notfound@example.com").Return(nil, gorm.ErrRecordNotFound).Once()

		// Act
		_, err := authService.Login(ctx, "notfound@example.com", password)

		// Assert
		assert.Error(t, err)
		assert.Equal(t, "invalid email or password", err.Error())
		mockRepo.AssertExpectations(t)
	})

	t.Run("Incorrect Password", func(t *testing.T) {
		// Arrange
		mockRepo.On("FindByEmail", ctx, testUser.Email).Return(testUser, nil).Once()

		// Act
		_, err := authService.Login(ctx, testUser.Email, "wrongpassword")

		// Assert
		assert.Error(t, err)
		assert.Equal(t, "invalid email or password", err.Error())
		mockRepo.AssertExpectations(t)
	})

	t.Run("Email Not Verified", func(t *testing.T) {
		// Arrange
		unverifiedUser := *testUser // Make a copy
		unverifiedUser.EmailVerified = false
		mockRepo.On("FindByEmail", ctx, unverifiedUser.Email).Return(&unverifiedUser, nil).Once()

		// Act
		_, err := authService.Login(ctx, unverifiedUser.Email, password)

		// Assert
		assert.Error(t, err)
		assert.Equal(t, "email not verified", err.Error())
		mockRepo.AssertExpectations(t)
	})
}
