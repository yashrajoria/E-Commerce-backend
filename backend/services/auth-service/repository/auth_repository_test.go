package repository

import (
	"context"
	"log"
	"testing"

	"auth-service/database"
	"auth-service/models"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

// TestSuite is a suite of tests that require a database connection.
type UserRepositoryTestSuite struct {
	suite.Suite
	db   *gorm.DB
	repo *UserRepository
}

// SetupSuite runs once before all tests in the suite.
func (s *UserRepositoryTestSuite) SetupSuite() {
	// Load test environment variables
	if err := godotenv.Load("../.env.test"); err != nil {
		log.Println("Warning: .env.test file not found. Using system environment variables.")
	}

	// Connect to the test database
	if err := database.Connect(); err != nil {
		s.T().Fatalf("Failed to connect to test database: %v", err)
	}

	s.db = database.DB
	// Auto-migrate the schema for our test database
	s.db.AutoMigrate(&models.User{})
	s.repo = NewUserRepository(s.db)
}

// TearDownSuite runs once after all tests in the suite.
func (s *UserRepositoryTestSuite) TearDownSuite() {
	// Drop all tables to clean up
	s.db.Migrator().DropTable(&models.User{})
}

// BeforeTest runs before each test. We use transactions to keep tests isolated.
func (s *UserRepositoryTestSuite) BeforeTest(suiteName, testName string) {
	s.db = s.db.Begin()
	s.repo = NewUserRepository(s.db) // Use the transaction in the repo
}

// AfterTest runs after each test, rolling back the transaction.
func (s *UserRepositoryTestSuite) AfterTest(suiteName, testName string) {
	s.db.Rollback()
}

// This function is the entry point for running the test suite.
func TestUserRepository(t *testing.T) {
	suite.Run(t, new(UserRepositoryTestSuite))
}

// --- Actual Tests ---

func (s *UserRepositoryTestSuite) TestCreateAndFindByEmail() {
	ctx := context.Background()
	newUser := &models.User{
		ID:       uuid.New(),
		Email:    "test.repo@example.com",
		Name:     "Repo Test User",
		Password: "hashedpassword",
	}

	// Test Create
	err := s.repo.Create(ctx, newUser)
	s.NoError(err, "Creating user should not produce an error")

	// Test FindByEmail (Success)
	foundUser, err := s.repo.FindByEmail(ctx, "test.repo@example.com")
	s.NoError(err, "Finding an existing user should not produce an error")
	s.NotNil(foundUser)
	s.Equal(newUser.ID, foundUser.ID, "Found user ID should match created user ID")
	s.Equal("Repo Test User", foundUser.Name, "Found user name should match")

	// Test FindByEmail (Not Found)
	_, err = s.repo.FindByEmail(ctx, "notfound@example.com")
	s.Error(err, "Finding a non-existent user should produce an error")
	s.ErrorIs(gorm.ErrRecordNotFound, err, "Error should be of type ErrRecordNotFound")
}
