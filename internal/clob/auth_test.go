// SPDX-License-Identifier: MIT

package clob

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestL1Headers guards against the L1 ClobAuthDomain (name/version/chainId,
// no verifyingContract) breaking EIP-712 encoding: the domain type declared
// for hashing must match exactly which fields the domain value populates.
func TestL1Headers(t *testing.T) {
	w := mustTestWallet(t)

	h, err := l1Headers(w, 137)
	if err != nil {
		t.Fatalf("l1Headers() error = %v", err)
	}
	for _, key := range []string{"POLY_ADDRESS", "POLY_SIGNATURE", "POLY_TIMESTAMP", "POLY_NONCE"} {
		if h.Get(key) == "" {
			t.Errorf("l1Headers() missing header %s", key)
		}
	}
	if got := h.Get("POLY_ADDRESS"); got != w.Address.Hex() {
		t.Errorf("POLY_ADDRESS = %s, want %s", got, w.Address.Hex())
	}
	if got := h.Get("POLY_NONCE"); got != "0" {
		t.Errorf("POLY_NONCE = %s, want 0", got)
	}
}

// TestDeriveAPICredsPrefersDerive verifies the happy path: GET
// /auth/derive-api-key succeeds, so the POST /auth/api-key fallback (which
// would create a new key) must never be called.
func TestDeriveAPICredsPrefersDerive(t *testing.T) {
	w := mustTestWallet(t)
	var postCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/derive-api-key", func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"apiKey":"derived-key","secret":"c2VjcmV0","passphrase":"pass"}`))
	})
	mux.HandleFunc("/auth/api-key", func(rw http.ResponseWriter, r *http.Request) {
		postCalled = true
		rw.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	creds, err := DeriveAPICreds(t.Context(), srv.Client(), srv.URL, w, 137)
	if err != nil {
		t.Fatalf("DeriveAPICreds() error = %v", err)
	}
	if creds.APIKey != "derived-key" {
		t.Errorf("APIKey = %q, want derived-key", creds.APIKey)
	}
	if postCalled {
		t.Error("POST /auth/api-key was called even though GET /auth/derive-api-key succeeded")
	}
}

// TestDeriveAPICredsFallsBackToCreate verifies that when deriving existing
// credentials fails (e.g. none exist yet for this wallet), DeriveAPICreds
// falls back to creating a new API key via POST /auth/api-key.
func TestDeriveAPICredsFallsBackToCreate(t *testing.T) {
	w := mustTestWallet(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/derive-api-key", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/auth/api-key", func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"apiKey":"created-key","secret":"c2VjcmV0","passphrase":"pass"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	creds, err := DeriveAPICreds(t.Context(), srv.Client(), srv.URL, w, 137)
	if err != nil {
		t.Fatalf("DeriveAPICreds() error = %v", err)
	}
	if creds.APIKey != "created-key" {
		t.Errorf("APIKey = %q, want created-key", creds.APIKey)
	}
}

// TestDeriveAPICredsFallsBackOnEmptyCredentials covers a derive response that
// is 2xx but carries no credentials (requestCreds must treat that as a
// failure so DeriveAPICreds still falls back to creating a key).
func TestDeriveAPICredsFallsBackOnEmptyCredentials(t *testing.T) {
	w := mustTestWallet(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/derive-api-key", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{}`))
	})
	mux.HandleFunc("/auth/api-key", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"apiKey":"created-key","secret":"c2VjcmV0","passphrase":"pass"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	creds, err := DeriveAPICreds(t.Context(), srv.Client(), srv.URL, w, 137)
	if err != nil {
		t.Fatalf("DeriveAPICreds() error = %v", err)
	}
	if creds.APIKey != "created-key" {
		t.Errorf("APIKey = %q, want created-key", creds.APIKey)
	}
}

// TestDeriveAPICredsBothFail verifies the error path when neither endpoint
// yields usable credentials.
func TestDeriveAPICredsBothFail(t *testing.T) {
	w := mustTestWallet(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/derive-api-key", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/auth/api-key", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if _, err := DeriveAPICreds(t.Context(), srv.Client(), srv.URL, w, 137); err == nil {
		t.Fatal("DeriveAPICreds() expected error, got nil")
	}
}

// TestDeriveAPICredsUnreachableServer covers the network-error branch of
// requestCreds (as opposed to an HTTP error status): both attempts must fail
// against a server that isn't listening at all.
func TestDeriveAPICredsUnreachableServer(t *testing.T) {
	w := mustTestWallet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {}))
	baseURL := srv.URL
	srv.Close() // nothing is listening at baseURL anymore

	if _, err := DeriveAPICreds(t.Context(), http.DefaultClient, baseURL, w, 137); err == nil {
		t.Fatal("DeriveAPICreds() expected error against an unreachable server, got nil")
	}
}

// TestDeriveAPICredsInvalidJSONResponse covers requestCreds' decode-error
// branch: a 2xx response that isn't valid JSON must fail rather than return
// a zero-value Creds.
func TestDeriveAPICredsInvalidJSONResponse(t *testing.T) {
	w := mustTestWallet(t)
	mux := http.NewServeMux()
	notJSON := func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("not json"))
	}
	mux.HandleFunc("/auth/derive-api-key", notJSON)
	mux.HandleFunc("/auth/api-key", notJSON)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if _, err := DeriveAPICreds(t.Context(), srv.Client(), srv.URL, w, 137); err == nil {
		t.Fatal("DeriveAPICreds() expected error for a non-JSON 2xx response, got nil")
	}
}

// TestRequestCredsInvalidURL covers requestCreds' request-building error
// branch: a URL with an embedded control character fails http.NewRequestWithContext.
func TestRequestCredsInvalidURL(t *testing.T) {
	w := mustTestWallet(t)
	if _, err := requestCreds(t.Context(), http.DefaultClient, http.MethodGet, "http://exa\nmple.com", w, 137); err == nil {
		t.Fatal("requestCreds() expected error for an invalid URL, got nil")
	}
}

// TestRequestCredsPropagatesReadBodyError covers requestCreds' io.ReadAll
// error branch: the server declares more bytes (Content-Length) than it
// actually sends before closing the connection.
func TestRequestCredsPropagatesReadBodyError(t *testing.T) {
	w := mustTestWallet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		hj, ok := rw.(http.Hijacker)
		if !ok {
			t.Fatal("ResponseWriter does not support hijacking")
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Fatalf("Hijack() error = %v", err)
		}
		defer func() { _ = conn.Close() }()
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 100\r\nContent-Type: application/json\r\n\r\n{\"apiKey\":")
		_ = buf.Flush()
	}))
	defer srv.Close()

	if _, err := requestCreds(t.Context(), srv.Client(), http.MethodGet, srv.URL, w, 137); err == nil {
		t.Fatal("requestCreds() expected error for a truncated response body, got nil")
	}
}
