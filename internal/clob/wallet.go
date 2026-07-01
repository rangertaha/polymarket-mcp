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
// leading "0x") into a Wallet.
func NewWallet(hexKey string) (*Wallet, error) {
	hexKey = strings.TrimPrefix(strings.TrimSpace(hexKey), "0x")
	key, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}
	return &Wallet{Key: key, Address: crypto.PubkeyToAddress(key.PublicKey)}, nil
}
