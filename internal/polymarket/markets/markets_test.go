// SPDX-License-Identifier: MIT

package markets

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/rangertaha/polymarket-mcp/internal/client"
	"github.com/rangertaha/polymarket-mcp/internal/polymarket"
)

func newTestService(t *testing.T, handler http.Handler) *service {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	gamma, err := client.New(srv.URL, nil, client.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}
	return &service{c: &polymarket.Clients{Gamma: gamma}}
}

func TestListMarketsBuildsQuery(t *testing.T) {
	var gotQuery url.Values
	svc := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/markets" {
			t.Errorf("path = %q, want /markets", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"1","question":"Will it rain?","active":true,"closed":false}]`))
	}))

	active := true
	markets, err := svc.ListMarkets(t.Context(), &active, nil, 10, 20)
	if err != nil {
		t.Fatalf("ListMarkets() error = %v", err)
	}
	if len(markets) != 1 || markets[0].ID != "1" || markets[0].Question != "Will it rain?" {
		t.Fatalf("markets = %+v, unexpected", markets)
	}
	if gotQuery.Get("active") != "true" {
		t.Errorf("active query = %q, want true", gotQuery.Get("active"))
	}
	if gotQuery.Get("limit") != "10" {
		t.Errorf("limit query = %q, want 10", gotQuery.Get("limit"))
	}
	if gotQuery.Get("offset") != "20" {
		t.Errorf("offset query = %q, want 20", gotQuery.Get("offset"))
	}
	if gotQuery.Has("closed") {
		t.Errorf("closed query should be absent when nil, got %q", gotQuery.Get("closed"))
	}
}

func TestListMarketsOmitsUnsetFilters(t *testing.T) {
	var gotQuery url.Values
	svc := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))

	if _, err := svc.ListMarkets(t.Context(), nil, nil, 0, 0); err != nil {
		t.Fatalf("ListMarkets() error = %v", err)
	}
	for _, key := range []string{"active", "closed", "limit", "offset"} {
		if gotQuery.Has(key) {
			t.Errorf("query has unexpected key %q = %q", key, gotQuery.Get(key))
		}
	}
}

func TestListMarketsSetsClosedFilter(t *testing.T) {
	var gotQuery url.Values
	svc := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))

	closed := true
	if _, err := svc.ListMarkets(t.Context(), nil, &closed, 0, 0); err != nil {
		t.Fatalf("ListMarkets() error = %v", err)
	}
	if gotQuery.Get("closed") != "true" {
		t.Errorf("closed query = %q, want true", gotQuery.Get("closed"))
	}
}

func TestListMarketsPropagatesAPIError(t *testing.T) {
	svc := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	if _, err := svc.ListMarkets(t.Context(), nil, nil, 0, 0); err == nil {
		t.Fatal("ListMarkets() expected error, got nil")
	}
}

func TestGetMarket(t *testing.T) {
	svc := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/markets/42" {
			t.Errorf("path = %q, want /markets/42", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"42","question":"Will it snow?","active":true,"closed":false,"outcomePrices":"[\"0.5\",\"0.5\"]"}`))
	}))

	m, err := svc.GetMarket(t.Context(), "42")
	if err != nil {
		t.Fatalf("GetMarket() error = %v", err)
	}
	if m.ID != "42" || m.Question != "Will it snow?" {
		t.Fatalf("market = %+v, unexpected", m)
	}
}

func TestGetMarketPropagatesAPIError(t *testing.T) {
	svc := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"market not found"}`))
	}))

	_, err := svc.GetMarket(t.Context(), "does-not-exist")
	if err == nil {
		t.Fatal("GetMarket() expected error, got nil")
	}
}
