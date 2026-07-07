// SPDX-License-Identifier: MIT

package clob

import (
	"testing"

	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// TestDomainType exercises every optional field domainType may need to
// declare. The L1 auth domain (name/version/chainId) and order domains (all
// four, no salt) exercise a subset today; Salt is unused by any domain this
// package currently builds but domainType must still declare it correctly if
// a future domain sets it, since the declared type array must match the
// domain value's populated fields exactly or EIP-712 encoding fails.
func TestDomainType(t *testing.T) {
	cases := []struct {
		name   string
		domain apitypes.TypedDataDomain
		want   []string // expected field names, in order
	}{
		{"empty", apitypes.TypedDataDomain{}, nil},
		{"name only", apitypes.TypedDataDomain{Name: "X"}, []string{"name"}},
		{"L1 auth domain", apitypes.TypedDataDomain{Name: "ClobAuthDomain", Version: "1", ChainId: chainIDDomain(137)}, []string{"name", "version", "chainId"}},
		{"order domain", apitypes.TypedDataDomain{Name: "Polymarket CTF Exchange", Version: "2", ChainId: chainIDDomain(137), VerifyingContract: "0xabc"}, []string{"name", "version", "chainId", "verifyingContract"}},
		{"with salt", apitypes.TypedDataDomain{Name: "X", Salt: "deadbeef"}, []string{"name", "salt"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := domainType(c.domain)
			if len(got) != len(c.want) {
				t.Fatalf("domainType() = %+v, want fields %v", got, c.want)
			}
			for i, f := range got {
				if f.Name != c.want[i] {
					t.Errorf("field[%d].Name = %q, want %q", i, f.Name, c.want[i])
				}
			}
		})
	}
}

// TestSignTypedDataPropagatesHashingError covers signTypedData's
// TypedDataAndHash error branch: a message value that doesn't parse as the
// field's declared type (uint256 here) must fail hashing rather than sign
// garbage.
func TestSignTypedDataPropagatesHashingError(t *testing.T) {
	w := mustTestWallet(t)
	domain := apitypes.TypedDataDomain{Name: "X"}
	types := apitypes.Types{
		"Test": []apitypes.Type{{Name: "value", Type: "uint256"}},
	}
	message := apitypes.TypedDataMessage{"value": "not-a-number"}

	if _, err := signTypedData(w, domain, "Test", types, message); err == nil {
		t.Fatal("signTypedData() expected error for a message value that doesn't match its declared type, got nil")
	}
}
