package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSupabaseStorageURLs(t *testing.T) {
	client := NewStorageClient(Config{
		BaseURL: "https://vtfryikkounwjuyceztn.supabase.co",
		APIKey:  "test-key",
		Bucket:  "shopswift",
	})

	url := client.PublicURL("products/demo/item.png")
	assert.Equal(t, "https://vtfryikkounwjuyceztn.supabase.co/storage/v1/object/public/shopswift/products/demo/item.png", url)
}

func TestLocalStorage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage_test_*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	client := NewLocalStorage(tmpDir, "http://localhost:8082/uploads")
	ctx := context.Background()

	data := []byte("hello storage")
	url, err := client.Upload(ctx, "test/hello.txt", data, "text/plain")
	assert.NoError(t, err)
	assert.Equal(t, "http://localhost:8082/uploads/test/hello.txt", url)

	readData, err := os.ReadFile(filepath.Join(tmpDir, "test/hello.txt"))
	assert.NoError(t, err)
	assert.Equal(t, data, readData)
}
