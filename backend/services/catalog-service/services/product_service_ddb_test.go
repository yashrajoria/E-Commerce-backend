package services

import (
	"testing"

	"github.com/yashrajoria/common/storage"
)

func TestPublicObjectURL_Supabase(t *testing.T) {
	svc := &ProductServiceDDB{
		storage: storage.NewStorageClient(storage.Config{
			BaseURL: "https://vtfryikkounwjuyceztn.supabase.co",
			APIKey:  "test-key",
			Bucket:  "shopswift",
		}),
	}

	got := svc.publicObjectURL("products/item.jpg")
	want := "https://vtfryikkounwjuyceztn.supabase.co/storage/v1/object/public/shopswift/products/item.jpg"
	if got != want {
		t.Fatalf("publicObjectURL() = %q, want %q", got, want)
	}
}

func TestPublicObjectURL_Local(t *testing.T) {
	svc := &ProductServiceDDB{
		storage: storage.NewLocalStorage("/tmp/uploads", "http://localhost:8082/uploads"),
	}

	got := svc.publicObjectURL("products/item.jpg")
	want := "http://localhost:8082/uploads/products/item.jpg"
	if got != want {
		t.Fatalf("publicObjectURL() = %q, want %q", got, want)
	}
}
