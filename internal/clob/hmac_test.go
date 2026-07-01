// SPDX-License-Identifier: MIT

package clob

import "testing"

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
