package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Client defines an object storage interface for product images and assets.
type Client interface {
	Upload(ctx context.Context, key string, body []byte, contentType string) (string, error)
	GeneratePresignedUpload(ctx context.Context, key, contentType string, expiresSeconds int64) (uploadURL, publicURL string, err error)
	PublicURL(key string) string
}

// Config holds storage configuration.
type Config struct {
	BaseURL string // e.g. https://<project-ref>.supabase.co
	APIKey  string // Supabase anon or service_role key
	Bucket  string // e.g. shopswift
}

// SupabaseStorage implements Client using Supabase Storage REST API.
type SupabaseStorage struct {
	baseURL    string
	apiKey     string
	bucket     string
	httpClient *http.Client
}

// NewStorageClient creates a storage client based on environment.
func NewStorageClient(cfg Config) Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = os.Getenv("SUPABASE_URL")
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("SUPABASE_KEY")
		if cfg.APIKey == "" {
			cfg.APIKey = os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
		}
		if cfg.APIKey == "" {
			cfg.APIKey = os.Getenv("SUPABASE_ANON_KEY")
		}
	}
	if cfg.Bucket == "" {
		cfg.Bucket = os.Getenv("STORAGE_BUCKET")
		if cfg.Bucket == "" {
			cfg.Bucket = "shopswift"
		}
	}

	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	if cfg.BaseURL != "" && cfg.APIKey != "" {
		return &SupabaseStorage{
			baseURL:    cfg.BaseURL,
			apiKey:     cfg.APIKey,
			bucket:     cfg.Bucket,
			httpClient: &http.Client{Timeout: 30 * time.Second},
		}
	}

	// Fallback to local storage
	uploadDir := os.Getenv("LOCAL_UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	return NewLocalStorage(uploadDir, "http://localhost:8082/uploads")
}

// Upload uploads raw bytes directly to Supabase Storage.
func (s *SupabaseStorage) Upload(ctx context.Context, key string, body []byte, contentType string) (string, error) {
	cleanKey := strings.TrimLeft(key, "/")
	reqURL := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.baseURL, s.bucket, cleanKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("apikey", s.apiKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("x-upsert", "true")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("supabase storage upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("supabase storage upload failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return s.PublicURL(cleanKey), nil
}

// GeneratePresignedUpload asks Supabase for a signed upload URL.
func (s *SupabaseStorage) GeneratePresignedUpload(ctx context.Context, key, contentType string, expiresSeconds int64) (string, string, error) {
	cleanKey := strings.TrimLeft(key, "/")
	reqURL := fmt.Sprintf("%s/storage/v1/object/upload/sign/%s/%s", s.baseURL, s.bucket, cleanKey)

	payload := map[string]interface{}{
		"expiresIn": expiresSeconds,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", "", fmt.Errorf("failed to create sign request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("apikey", s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("supabase storage sign request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("supabase storage sign failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var res struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", "", fmt.Errorf("failed to decode sign response: %w", err)
	}

	fullUploadURL := fmt.Sprintf("%s/storage/v1%s", s.baseURL, res.URL)
	return fullUploadURL, s.PublicURL(cleanKey), nil
}

// PublicURL returns the public CDN URL for an object key.
func (s *SupabaseStorage) PublicURL(key string) string {
	cleanKey := strings.TrimLeft(key, "/")
	return fmt.Sprintf("%s/storage/v1/object/public/%s/%s", s.baseURL, s.bucket, cleanKey)
}

// LocalStorage implements Client storing files on local filesystem.
type LocalStorage struct {
	baseDir string
	baseURL string
}

func NewLocalStorage(baseDir, baseURL string) *LocalStorage {
	_ = os.MkdirAll(baseDir, 0755)
	return &LocalStorage{baseDir: baseDir, baseURL: strings.TrimRight(baseURL, "/")}
}

func (l *LocalStorage) Upload(ctx context.Context, key string, body []byte, contentType string) (string, error) {
	cleanKey := strings.TrimLeft(key, "/")
	targetPath := filepath.Join(l.baseDir, cleanKey)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(targetPath, body, 0644); err != nil {
		return "", err
	}
	return l.PublicURL(cleanKey), nil
}

func (l *LocalStorage) GeneratePresignedUpload(ctx context.Context, key, contentType string, expiresSeconds int64) (string, string, error) {
	cleanKey := strings.TrimLeft(key, "/")
	publicURL := l.PublicURL(cleanKey)
	return publicURL, publicURL, nil
}

func (l *LocalStorage) PublicURL(key string) string {
	cleanKey := strings.TrimLeft(key, "/")
	return fmt.Sprintf("%s/%s", l.baseURL, cleanKey)
}
