// SPDX-License-Identifier: MIT

package clob

import "testing"

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
