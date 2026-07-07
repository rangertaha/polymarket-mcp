// SPDX-License-Identifier: MIT

// Package clob implements the pieces of Polymarket's CLOB (Central Limit Order
// Book) trading protocol that sit outside the generic REST client: wallet
// signing, L1/L2 authentication, and EIP-712 order signing.
//
// This code has not been exercised against live trading; verify carefully
// (small test orders first) before relying on it with real funds.
package clob

import (
	"crypto/ecdsa"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Wallet holds the private key used to derive CLOB API credentials and sign
// orders, and the address it corresponds to.
type Wallet struct {
	Key     *ecdsa.PrivateKey
	Address common.Address
}

// NewWallet parses a hex-encoded secp256k1 private key (with or without a
// leading "0x"/"0X") into a Wallet.
func NewWallet(hexKey string) (*Wallet, error) {
	hexKey = stripHexPrefix(strings.TrimSpace(hexKey))
	key, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}
	return &Wallet{Key: key, Address: crypto.PubkeyToAddress(key.PublicKey)}, nil
}

// stripHexPrefix removes a leading "0x"/"0X" from a hex-encoded value.
// crypto.HexToECDSA rejects any such prefix, and a case-sensitive match alone
// would miss "0X", misreporting a valid key as invalid.
func stripHexPrefix(s string) string {
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		return s[2:]
	}
	return s
}
