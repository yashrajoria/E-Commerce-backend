package middleware

import (
	"api-gateway/logger"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestDoRefresh_ClassifiesOutageVsRejection(t *testing.T) {
	if logger.Log == nil {
		logger.Log = zap.NewNop()
	}
	status := http.StatusInternalServerError
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
	defer srv.Close()
	old := authBaseURL
	authBaseURL = srv.URL
	defer func() { authBaseURL = old }()

	if _, err := doRefresh("tok", "", "/x"); !errors.Is(err, errRefreshUnavailable) {
		t.Fatalf("5xx should be an outage, got %v", err)
	}
	status = http.StatusUnauthorized
	if _, err := doRefresh("tok", "", "/x"); err == nil || errors.Is(err, errRefreshUnavailable) {
		t.Fatalf("401 should be a plain rejection, got %v", err)
	}
}
