package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func ForwardGet(ctx context.Context, client *http.Client, url string, headers http.Header) ([]byte, int, error) {
	return forwardRequest(ctx, client, http.MethodGet, url, nil, headers)
}

func ForwardPost(ctx context.Context, client *http.Client, url string, body []byte, headers http.Header) ([]byte, int, error) {
	return forwardRequest(ctx, client, http.MethodPost, url, body, headers)
}

func ForwardPut(ctx context.Context, client *http.Client, url string, body []byte, headers http.Header) ([]byte, int, error) {
	return forwardRequest(ctx, client, http.MethodPut, url, body, headers)
}

func ForwardDelete(ctx context.Context, client *http.Client, url string, headers http.Header) ([]byte, int, error) {
	return forwardRequest(ctx, client, http.MethodDelete, url, nil, headers)
}

func MapDownstreamError(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return "invalid request"
	case http.StatusNotFound:
		return "not found"
	case http.StatusInternalServerError:
		return "service unavailable"
	default:
		if statusCode >= http.StatusInternalServerError {
			return "service unavailable"
		}
		return "unexpected error"
	}
}

func forwardRequest(ctx context.Context, client *http.Client, method, url string, body []byte, headers http.Header) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	copyForwardHeaders(req.Header, headers)
	if req.Header.Get("Content-Type") == "" && body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response body: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

func copyForwardHeaders(dst, src http.Header) {
	if src == nil {
		return
	}

	if v := src.Get("X-User-ID"); v != "" {
		dst.Set("X-User-ID", v)
	}
	if v := src.Get("X-User-Role"); v != "" {
		dst.Set("X-User-Role", v)
	}
	if v := src.Get("Authorization"); v != "" {
		dst.Set("Authorization", v)
	}
	if v := src.Get("X-Request-ID"); v != "" {
		dst.Set("X-Request-ID", v)
	}
	if v := src.Get("Accept"); v != "" {
		dst.Set("Accept", v)
	}
	if v := src.Get("Content-Type"); v != "" {
		dst.Set("Content-Type", v)
	}
	if v := src.Get("Cookie"); v != "" {
		dst.Set("Cookie", v)
	}
}
