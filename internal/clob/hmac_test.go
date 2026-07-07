// SPDX-License-Identifier: MIT

package clob

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestHMACSignature is a known-answer test: the expected signature was
// computed independently in Python (hmac+hashlib+base64, matching
// Polymarket's documented L2 scheme) to catch any divergence in secret
// decoding, message construction, or output encoding.
func TestHMACSignature(t *testing.T) {
	const (
		secret   = "dGVzdC1zZWNyZXQtYnl0ZXMtMTIzNDU2" // base64url("test-secret-bytes-123456")
		expected = "uV9RLGVfmKPhxHXdrKHP4_mA-lnFagWNLgkZMTQE8t4="
	)
	got := hmacSignature(secret, "1700000000", "POST", "/order", `{"foo":"bar"}`)
	if got != expected {
		t.Fatalf("hmacSignature() = %q, want %q", got, expected)
	}
}

func TestHMACSignatureEmptyBody(t *testing.T) {
	const secret = "dGVzdC1zZWNyZXQtYnl0ZXMtMTIzNDU2"
	// Just asserting it doesn't panic/error and is deterministic.
	a := hmacSignature(secret, "1700000000", "GET", "/data/orders", "")
	b := hmacSignature(secret, "1700000000", "GET", "/data/orders", "")
	if a != b {
		t.Fatalf("hmacSignature() not deterministic: %q vs %q", a, b)
	}
}

// TestHMACSignatureFallsBackOnInvalidBase64Secret guards the fallback in
// hmacSignature: some observed API secrets aren't valid base64url (padding
// inconsistencies), and the signer must use the raw bytes rather than
// erroring out.
func TestHMACSignatureFallsBackOnInvalidBase64Secret(t *testing.T) {
	const notBase64 = "not-valid-base64url!!"
	got := hmacSignature(notBase64, "1700000000", "GET", "/data/orders", "")
	if got == "" {
		t.Fatal("hmacSignature() returned empty signature for invalid base64 secret")
	}
	// Deterministic: the same invalid secret must fall back the same way.
	again := hmacSignature(notBase64, "1700000000", "GET", "/data/orders", "")
	if got != again {
		t.Fatalf("hmacSignature() not deterministic on fallback path: %q vs %q", got, again)
	}
}

// newDeriveOnlyServer returns a mock CLOB server that serves exactly one
// endpoint, /auth/derive-api-key, counting how many times it's hit — used to
// verify Ensure derives credentials at most once even under concurrent calls.
func newDeriveOnlyServer(t *testing.T) (*httptest.Server, *int, *sync.Mutex) {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiKey":"test-key","secret":"c2VjcmV0","passphrase":"test-pass"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &mu
}

func TestNewAuthorizerDefaultsHTTPClient(t *testing.T) {
	w := mustTestWallet(t)
	a := NewAuthorizer(w, "https://clob.polymarket.com", 137, nil)
	if a.hc == nil {
		t.Fatal("NewAuthorizer() with nil *http.Client did not install a default")
	}
}

func TestAuthorizerAPIKeyEmptyBeforeEnsure(t *testing.T) {
	w := mustTestWallet(t)
	a := NewAuthorizer(w, "https://clob.polymarket.com", 137, nil)
	if got := a.APIKey(); got != "" {
		t.Errorf("APIKey() before Ensure = %q, want empty", got)
	}
}

// TestAuthorizerEnsurePropagatesDerivationError verifies that when neither
// CLOB auth endpoint yields credentials, Ensure surfaces the failure rather
// than caching a zero-value Creds.
func TestAuthorizerEnsurePropagatesDerivationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	w := mustTestWallet(t)
	a := NewAuthorizer(w, srv.URL, 137, srv.Client())

	if err := a.Ensure(t.Context()); err == nil {
		t.Fatal("Ensure() expected error when both auth endpoints fail, got nil")
	}
	if got := a.APIKey(); got != "" {
		t.Errorf("APIKey() after failed Ensure = %q, want empty", got)
	}

	// A retry after the transient failure must still be able to succeed
	// (Ensure must not have permanently cached the failure).
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiKey":"test-key","secret":"c2VjcmV0","passphrase":"test-pass"}`))
	})
	if err := a.Ensure(t.Context()); err != nil {
		t.Fatalf("Ensure() retry error = %v", err)
	}
	if got := a.APIKey(); got != "test-key" {
		t.Errorf("APIKey() after retry = %q, want test-key", got)
	}
}

func TestAuthorizerEnsureDerivesOnceAndCaches(t *testing.T) {
	srv, calls, mu := newDeriveOnlyServer(t)
	w := mustTestWallet(t)
	a := NewAuthorizer(w, srv.URL, 137, srv.Client())

	if err := a.Ensure(t.Context()); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if got := a.APIKey(); got != "test-key" {
		t.Errorf("APIKey() = %q, want test-key", got)
	}

	if err := a.Ensure(t.Context()); err != nil {
		t.Fatalf("second Ensure() error = %v", err)
	}
	mu.Lock()
	got := *calls
	mu.Unlock()
	if got != 1 {
		t.Errorf("derive-api-key called %d times, want 1 (credentials should be cached)", got)
	}
}

// TestAuthorizerEnsureConcurrentCallsDeriveOnce guards the double-checked
// locking in Ensure: many goroutines racing to authenticate the first
// request must still only trigger one credential-derivation round trip.
func TestAuthorizerEnsureConcurrentCallsDeriveOnce(t *testing.T) {
	srv, calls, mu := newDeriveOnlyServer(t)
	w := mustTestWallet(t)
	a := NewAuthorizer(w, srv.URL, 137, srv.Client())

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.Ensure(t.Context()); err != nil {
				t.Errorf("Ensure() error = %v", err)
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	got := *calls
	mu.Unlock()
	if got != 1 {
		t.Errorf("derive-api-key called %d times, want 1", got)
	}
}

func TestAuthorizerAuthorizeWithoutCredsIsNoop(t *testing.T) {
	w := mustTestWallet(t)
	a := NewAuthorizer(w, "https://clob.polymarket.com", 137, nil)

	req, err := http.NewRequest(http.MethodGet, "https://clob.polymarket.com/data/orders", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	a.Authorize(req)
	if req.Header.Get("POLY_SIGNATURE") != "" {
		t.Error("Authorize() set POLY_SIGNATURE before Ensure derived credentials")
	}
}

func TestAuthorizerAuthorizeSignsRequest(t *testing.T) {
	srv, _, _ := newDeriveOnlyServer(t)
	w := mustTestWallet(t)
	a := NewAuthorizer(w, srv.URL, 137, srv.Client())
	if err := a.Ensure(t.Context()); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	body := `{"orderID":"0xabc"}`
	req, err := http.NewRequest(http.MethodDelete, "https://clob.polymarket.com/order", strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(body)), nil
	}

	a.Authorize(req)

	if got := req.Header.Get("POLY_ADDRESS"); got != w.Address.Hex() {
		t.Errorf("POLY_ADDRESS = %q, want %q", got, w.Address.Hex())
	}
	if req.Header.Get("POLY_API_KEY") != "test-key" {
		t.Errorf("POLY_API_KEY = %q, want test-key", req.Header.Get("POLY_API_KEY"))
	}
	if req.Header.Get("POLY_PASSPHRASE") != "test-pass" {
		t.Errorf("POLY_PASSPHRASE = %q, want test-pass", req.Header.Get("POLY_PASSPHRASE"))
	}
	ts := req.Header.Get("POLY_TIMESTAMP")
	if ts == "" {
		t.Fatal("POLY_TIMESTAMP is empty")
	}
	wantSig := hmacSignature("c2VjcmV0", ts, http.MethodDelete, "/order", body)
	if got := req.Header.Get("POLY_SIGNATURE"); got != wantSig {
		t.Errorf("POLY_SIGNATURE = %q, want %q", got, wantSig)
	}
}
