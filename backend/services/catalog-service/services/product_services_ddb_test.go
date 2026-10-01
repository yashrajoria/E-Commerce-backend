package services

import "testing"

func TestPublicObjectURL_UsesBrowserReachableBaseURL(t *testing.T) {
	svc := &ProductServiceDDB{
		bucket:   "shopswift",
		endpoint: "http://localhost:4566",
	}

	got := svc.publicObjectURL("products/item.jpg")

	want := "http://localhost:4566/shopswift/products/item.jpg"
	if got != want {
		t.Fatalf("publicObjectURL() = %q, want %q", got, want)
	}
}

func TestPublicObjectURL_PrefersCDNDomain(t *testing.T) {
	svc := &ProductServiceDDB{
		bucket:    "shopswift",
		endpoint:  "http://localhost:4566",
		cdnDomain: "cdn.example.com",
	}

	got := svc.publicObjectURL("products/item.jpg")

	want := "https://cdn.example.com/products/item.jpg"
	if got != want {
		t.Fatalf("publicObjectURL() = %q, want %q", got, want)
	}
}
