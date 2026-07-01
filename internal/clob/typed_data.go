// SPDX-License-Identifier: MIT

package clob

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// domainType returns the EIP-712 domain type declaration for exactly the
// fields present in domain, in the field order apitypes.TypedDataDomain.Map()
// uses. The L1 auth domain (name/version/chainId, no verifyingContract) and
// order domains (all four) declare different field sets; the type array must
// match the domain value exactly; EIP712Domain.Map() only exports the fields
// with values, and encoding a declared field that Map() didn't populate fails.
func domainType(domain apitypes.TypedDataDomain) []apitypes.Type {
	var t []apitypes.Type
	if len(domain.Name) > 0 {
		t = append(t, apitypes.Type{Name: "name", Type: "string"})
	}
	if len(domain.Version) > 0 {
		t = append(t, apitypes.Type{Name: "version", Type: "string"})
	}
	if domain.ChainId != nil {
		t = append(t, apitypes.Type{Name: "chainId", Type: "uint256"})
	}
	if len(domain.VerifyingContract) > 0 {
		t = append(t, apitypes.Type{Name: "verifyingContract", Type: "address"})
	}
	if len(domain.Salt) > 0 {
		t = append(t, apitypes.Type{Name: "salt", Type: "string"})
	}
	return t
}

// signTypedData signs an EIP-712 typed data payload and returns a 0x-prefixed,
// 65-byte (r || s || v) signature with v normalized to {27, 28}, as
// Polymarket's on-chain verifiers expect.
func signTypedData(w *Wallet, domain apitypes.TypedDataDomain, primaryType string, types apitypes.Types, message apitypes.TypedDataMessage) (string, error) {
	allTypes := apitypes.Types{"EIP712Domain": domainType(domain)}
	for k, v := range types {
		allTypes[k] = v
	}

	td := apitypes.TypedData{
		Types:       allTypes,
		PrimaryType: primaryType,
		Domain:      domain,
		Message:     message,
	}

	digest, _, err := apitypes.TypedDataAndHash(td)
	if err != nil {
		return "", fmt.Errorf("hashing typed data: %w", err)
	}

	sig, err := crypto.Sign(digest, w.Key)
	if err != nil {
		return "", fmt.Errorf("signing typed data: %w", err)
	}
	sig[64] += 27
	return hexutil.Encode(sig), nil
}

// chainIDDomain builds the *math.HexOrDecimal256 chainId value shared by every
// domain in this package.
func chainIDDomain(chainID int64) *math.HexOrDecimal256 {
	return math.NewHexOrDecimal256(chainID)
}
