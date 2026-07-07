// SPDX-License-Identifier: MIT

package polymarket

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/rangertaha/polymarket-mcp/internal/client"
)

func TestCheckReturnsMarketCount(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/markets" {
			t.Errorf("path = %q, want /markets", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"1"}]`))
	}))
	defer srv.Close()

	gamma, err := client.New(srv.URL, nil, client.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}

	n, err := Check(t.Context(), &Clients{Gamma: gamma})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if n != 1 {
		t.Errorf("Check() = %d, want 1", n)
	}
	if gotQuery.Get("limit") != "1" {
		t.Errorf("limit query = %q, want 1", gotQuery.Get("limit"))
	}
}

func TestCheckPropagatesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"unavailable"}`))
	}))
	defer srv.Close()

	gamma, err := client.New(srv.URL, nil, client.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}

	if _, err := Check(t.Context(), &Clients{Gamma: gamma}); err == nil {
		t.Fatal("Check() expected error, got nil")
	}
}
