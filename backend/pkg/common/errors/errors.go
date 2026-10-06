package errors

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Error represents an application error
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

// Error implements the error interface
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the wrapped error
func (e *Error) Unwrap() error {
	return e.Err
}

// JSON returns the error as a JSON string
func (e *Error) JSON() string {
	b, _ := json.Marshal(e)
	return string(b)
}

// New creates a new Error
func New(code int, message string, err error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// Common error types
var (
	ErrBadRequest         = New(http.StatusBadRequest, "Bad request", nil)
	ErrUnauthorized       = New(http.StatusUnauthorized, "Unauthorized", nil)
	ErrForbidden          = New(http.StatusForbidden, "Forbidden", nil)
	ErrNotFound           = New(http.StatusNotFound, "Not found", nil)
	ErrMethodNotAllowed   = New(http.StatusMethodNotAllowed, "Method not allowed", nil)
	ErrInternalServer     = New(http.StatusInternalServerError, "Internal server error", nil)
	ErrServiceUnavailable = New(http.StatusServiceUnavailable, "Service unavailable", nil)
)

// Error handlers
func HandleError(w http.ResponseWriter, err error) {
	var appErr *Error
	if e, ok := err.(*Error); ok {
		appErr = e
	} else {
		appErr = ErrInternalServer
		appErr.Err = err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.Code)
	w.Write([]byte(appErr.JSON()))
}

// Error middleware for Gin
func ErrorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Check if there are any errors
		if len(c.Errors) > 0 {
			err := c.Errors.Last().Err
			var appErr *Error
			if e, ok := err.(*Error); ok {
				appErr = e
			} else {
				appErr = ErrInternalServer
				appErr.Err = err
			}

			c.JSON(appErr.Code, appErr)
			c.Abort()
		}
	}
}

// Database error types
var (
	ErrDatabaseConnection  = New(http.StatusServiceUnavailable, "Database connection error", nil)
	ErrDatabaseQuery       = New(http.StatusInternalServerError, "Database query error", nil)
	ErrDatabaseTransaction = New(http.StatusInternalServerError, "Database transaction error", nil)
)

// Validation error types
var (
	ErrValidation   = New(http.StatusBadRequest, "Validation error", nil)
	ErrInvalidInput = New(http.StatusBadRequest, "Invalid input", nil)
)

// Authentication error types
var (
	ErrInvalidCredentials = New(http.StatusUnauthorized, "Invalid credentials", nil)
	ErrTokenExpired       = New(http.StatusUnauthorized, "Token expired", nil)
	ErrInvalidToken       = New(http.StatusUnauthorized, "Invalid token", nil)
)

// Business logic error types
var (
	ErrInsufficientStock = New(http.StatusBadRequest, "Insufficient stock", nil)
	ErrInvalidOrder      = New(http.StatusBadRequest, "Invalid order", nil)
	ErrPaymentFailed     = New(http.StatusBadRequest, "Payment failed", nil)
)
