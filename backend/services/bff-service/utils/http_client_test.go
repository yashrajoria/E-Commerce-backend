package utils

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForwardGet_ForwardsAllowedHeaders(t *testing.T) {
	t.Parallel()

	var gotHeader http.Header
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer downstream.Close()

	headers := http.Header{}
	headers.Set("X-User-ID", "u-1")
	headers.Set("X-User-Role", "admin")
	headers.Set("Authorization", "Bearer abc")
	headers.Set("X-Request-ID", "req-1")
	headers.Set("Accept", "application/json")
	headers.Set("Cookie", "a=b")
	headers.Set("X-Not-Forwarded", "nope")

	body, statusCode, err := ForwardGet(context.Background(), downstream.Client(), downstream.URL, headers)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.JSONEq(t, `{"ok":true}`, string(body))
	assert.Equal(t, "u-1", gotHeader.Get("X-User-ID"))
	assert.Equal(t, "admin", gotHeader.Get("X-User-Role"))
	assert.Equal(t, "Bearer abc", gotHeader.Get("Authorization"))
	assert.Equal(t, "req-1", gotHeader.Get("X-Request-ID"))
	assert.Equal(t, "application/json", gotHeader.Get("Accept"))
	assert.Equal(t, "a=b", gotHeader.Get("Cookie"))
	assert.Equal(t, "", gotHeader.Get("X-Not-Forwarded"))
}

func TestForwardPost_SetsJSONContentTypeWhenMissing(t *testing.T) {
	t.Parallel()

	var gotContentType string
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer downstream.Close()

	_, statusCode, err := ForwardPost(context.Background(), downstream.Client(), downstream.URL, []byte(`{"name":"x"}`), nil)

	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, statusCode)
	assert.Equal(t, "application/json", gotContentType)
}

func TestForwardDelete_ReturnsDownstreamStatusAndBody(t *testing.T) {
	t.Parallel()

	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer downstream.Close()

	body, statusCode, err := ForwardDelete(context.Background(), downstream.Client(), downstream.URL, nil)

	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, statusCode)
	assert.Empty(t, body)
}

func TestForwardGet_ContextTimeout(t *testing.T) {
	t.Parallel()

	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer downstream.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, _, err := ForwardGet(ctx, downstream.Client(), downstream.URL, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "execute request")
}

func TestForwardGet_InvalidURL(t *testing.T) {
	t.Parallel()

	_, _, err := ForwardGet(context.Background(), NewHTTPClient(), "http://[::1", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create request")
}

func TestMapDownstreamError_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		expected   string
	}{
		{name: "bad_request", statusCode: http.StatusBadRequest, expected: "invalid request"},
		{name: "not_found", statusCode: http.StatusNotFound, expected: "not found"},
		{name: "internal_server_error", statusCode: http.StatusInternalServerError, expected: "service unavailable"},
		{name: "gateway_timeout", statusCode: http.StatusGatewayTimeout, expected: "service unavailable"},
		{name: "unknown_4xx", statusCode: http.StatusConflict, expected: "unexpected error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, MapDownstreamError(tt.statusCode))
		})
	}
}

func TestCopyForwardHeaders_NilSourceDoesNothing(t *testing.T) {
	t.Parallel()

	dst := http.Header{}
	copyForwardHeaders(dst, nil)
	assert.Len(t, dst, 0)
}

func BenchmarkForwardGet(b *testing.B) {
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer downstream.Close()

	client := downstream.Client()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := ForwardGet(ctx, client, downstream.URL, nil)
		if err != nil {
			b.Fatalf("forward get failed: %v", err)
		}
	}
}

func FuzzMapDownstreamError(f *testing.F) {
	f.Add(200)
	f.Add(400)
	f.Add(404)
	f.Add(500)
	f.Add(503)

	f.Fuzz(func(t *testing.T, statusCode int) {
		_ = MapDownstreamError(statusCode)
	})
}

func TestForwardGet_TransportErrorPropagation(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripErrorFunc(func(*http.Request) error {
			return errors.New("connection refused")
		}),
	}

	_, _, err := ForwardGet(context.Background(), client, "http://example.com", nil)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "execute request"))
}

type roundTripErrorFunc func(*http.Request) error

func (r roundTripErrorFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, r(req)
}
