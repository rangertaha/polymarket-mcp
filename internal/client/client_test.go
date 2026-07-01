// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// recordingAuthorizer records whether it was invoked and stamps a header, so
// tests can assert both that auth ran and that it can add credentials.
type recordingAuthorizer struct{ called bool }

func (a *recordingAuthorizer) Authorize(r *http.Request) {
	a.called = true
	r.Header.Set("X-Test-Auth", "1")
}

func TestGetJSONDecodesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/widgets" {
			t.Errorf("path = %q, want /widgets", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != "5" {
			t.Errorf("limit query = %q, want 5", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"42","name":"gizmo"}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var out struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	q := url.Values{"limit": {"5"}}
	if err := c.GetJSON(context.Background(), "/widgets", q, &out); err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}
	if out.ID != "42" || out.Name != "gizmo" {
		t.Errorf("out = %+v, want {42 gizmo}", out)
	}
}

func TestAuthorizerIsInvoked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Auth") != "1" {
			t.Error("request missing header set by Authorizer")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	auth := &recordingAuthorizer{}
	c, err := New(srv.URL, auth)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.GetJSON(context.Background(), "/", nil, nil); err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}
	if !auth.called {
		t.Error("Authorizer.Authorize was never called")
	}
}

func TestCallerHeaderOverridesDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "custom-agent" {
			t.Errorf("User-Agent = %q, want custom-agent", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := New(srv.URL, nil, WithUserAgent("mcp-client"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = c.Do(context.Background(), Request{
		Method: http.MethodGet,
		Path:   "/",
		Header: http.Header{"User-Agent": {"custom-agent"}},
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
}

func TestPostJSONSendsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if body["name"] != "gizmo" {
			t.Errorf("body = %v, want name=gizmo", body)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c, err := New(srv.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.PostJSON(context.Background(), "/widgets", nil, map[string]string{"name": "gizmo"}, nil); err != nil {
		t.Fatalf("PostJSON() error = %v", err)
	}
}

func TestDeleteJSONSendsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if body["orderID"] != "0xabc" {
			t.Errorf("body = %v, want orderID=0xabc", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"canceled":["0xabc"]}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var out struct {
		Canceled []string `json:"canceled"`
	}
	if err := c.DeleteJSON(context.Background(), "/order", nil, map[string]string{"orderID": "0xabc"}, &out); err != nil {
		t.Fatalf("DeleteJSON() error = %v", err)
	}
	if len(out.Canceled) != 1 || out.Canceled[0] != "0xabc" {
		t.Errorf("out.Canceled = %v, want [0xabc]", out.Canceled)
	}
}

func TestDoReturnsAPIErrorOnNon2xx(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"message field", `{"message":"bad request"}`, "bad request"},
		{"error string", `{"error":"boom"}`, "boom"},
		{"error object", `{"error":{"message":"nested boom"}}`, "nested boom"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()

			client, err := New(srv.URL, nil)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			err = client.GetJSON(context.Background(), "/", nil, nil)
			if err == nil {
				t.Fatal("GetJSON() expected error, got nil")
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error type = %T, want *APIError", err)
			}
			if apiErr.StatusCode != http.StatusBadRequest {
				t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
			}
			if apiErr.Message != c.wantMsg {
				t.Errorf("Message = %q, want %q", apiErr.Message, c.wantMsg)
			}
		})
	}
}

func TestRawBodyCapturesNonJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	c, err := New(srv.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var raw RawBody
	if _, err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/", Out: &raw}); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if raw.String() != "OK" {
		t.Errorf("raw.String() = %q, want OK", raw.String())
	}
	if raw.ContentType != "text/plain" {
		t.Errorf("raw.ContentType = %q, want text/plain", raw.ContentType)
	}
}

func TestHTTPClientAccessorReturnsConfiguredClient(t *testing.T) {
	custom := &http.Client{}
	c, err := New("https://example.com", nil, WithHTTPClient(custom))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if c.HTTPClient() != custom {
		t.Error("HTTPClient() did not return the client passed via WithHTTPClient")
	}
}

func TestNewRejectsInvalidBaseURL(t *testing.T) {
	if _, err := New("://not-a-url", nil); err == nil {
		t.Fatal("New() expected error for invalid base URL, got nil")
	}
}

func TestJoinPath(t *testing.T) {
	cases := []struct {
		base, rel, want string
	}{
		{"", "", "/"},
		{"", "widgets", "/widgets"},
		{"/api", "", "/api"},
		{"/api", "widgets", "/api/widgets"},
		{"/api/", "/widgets", "/api/widgets"},
		{"/api", "/widgets/", "/api/widgets/"},
	}
	for _, c := range cases {
		if got := joinPath(c.base, c.rel); got != c.want {
			t.Errorf("joinPath(%q, %q) = %q, want %q", c.base, c.rel, got, c.want)
		}
	}
}
