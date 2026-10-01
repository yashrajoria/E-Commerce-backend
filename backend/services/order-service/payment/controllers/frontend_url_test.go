package controllers

import (
	"os"
	"testing"
)

func TestFrontendURL_DefaultsToStorefront(t *testing.T) {
	t.Setenv("FRONTEND_URL", "")
	t.Setenv("STOREFRONT_URL", "")
	pc := &PaymentController{}
	got := pc.frontendURL()
	if got != "http://localhost:3001" {
		t.Fatalf("expected storefront default, got %q", got)
	}
}

func TestFrontendURL_PrefersFRONTEND_URL(t *testing.T) {
	t.Setenv("FRONTEND_URL", "https://shop.example.com/")
	t.Setenv("STOREFRONT_URL", "https://ignored.example.com")
	pc := &PaymentController{}
	got := pc.frontendURL()
	if got != "https://shop.example.com" {
		t.Fatalf("expected trimmed FRONTEND_URL, got %q", got)
	}
}

func TestFrontendURL_FallsBackToSTOREFRONT_URL(t *testing.T) {
	os.Unsetenv("FRONTEND_URL")
	t.Setenv("FRONTEND_URL", "")
	t.Setenv("STOREFRONT_URL", "http://localhost:3001/")
	pc := &PaymentController{}
	got := pc.frontendURL()
	if got != "http://localhost:3001" {
		t.Fatalf("expected STOREFRONT_URL, got %q", got)
	}
}
