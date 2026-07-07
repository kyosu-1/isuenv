package myip

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "198.51.100.7")
	}))
	defer srv.Close()
	orig := Endpoint
	Endpoint = srv.URL
	defer func() { Endpoint = orig }()

	ip, err := Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "198.51.100.7" {
		t.Errorf("expected trimmed ip, got %q", ip)
	}
}

func TestGet_InvalidResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "<html>not an ip</html>")
	}))
	defer srv.Close()
	orig := Endpoint
	Endpoint = srv.URL
	defer func() { Endpoint = orig }()

	if _, err := Get(context.Background()); err == nil {
		t.Fatal("expected error for non-IP response")
	}
}
