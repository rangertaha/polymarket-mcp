// SPDX-License-Identifier: MIT

package clob

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// testWallet is a throwaway key; it holds no funds and appears nowhere else.
const testPrivateKeyHex = "4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"

func mustTestWallet(t *testing.T) *Wallet {
	t.Helper()
	w, err := NewWallet(testPrivateKeyHex)
	if err != nil {
		t.Fatalf("NewWallet() error = %v", err)
	}
	return w
}

func TestNewWalletInvalidKey(t *testing.T) {
	if _, err := NewWallet("not-hex"); err == nil {
		t.Fatal("NewWallet() expected error for invalid hex key, got nil")
	}
}

// TestNewWalletAcceptsEitherHexPrefixCase guards against a case-sensitive
// prefix strip: crypto.HexToECDSA rejects both "0x" and "0X", so both must
// be stripped, not just the lowercase form.
func TestNewWalletAcceptsEitherHexPrefixCase(t *testing.T) {
	want := mustTestWallet(t).Address

	for _, prefixed := range []string{
		"0x" + testPrivateKeyHex,
		"0X" + testPrivateKeyHex,
		testPrivateKeyHex,
	} {
		w, err := NewWallet(prefixed)
		if err != nil {
			t.Fatalf("NewWallet(%q) error = %v", prefixed, err)
		}
		if w.Address != want {
			t.Errorf("NewWallet(%q).Address = %s, want %s", prefixed, w.Address.Hex(), want.Hex())
		}
	}
}

// TestBuildAndSignRecoversSignerAddress independently recomputes the EIP-712
// digest for the order BuildAndSign produced (duplicating the domain/message
// construction rather than calling internal helpers) and verifies the
// signature ecrecovers to the wallet's address. This guards the two details
// most likely to silently break trading: an off-by-one in the recovery id
// normalization (sig[64] += 27) and any drift between the signed field set
// and the V2 Order type declared in orderType.
func TestBuildAndSignRecoversSignerAddress(t *testing.T) {
	w := mustTestWallet(t)

	for _, side := range []Side{Buy, Sell} {
		order, err := BuildAndSign(w, 137, "", 0, OrderArgs{
			TokenID: "123456789",
			Price:   0.45,
			Size:    10,
			Side:    side,
		}, 1700000000000)
		if err != nil {
			t.Fatalf("BuildAndSign(%s) error = %v", side, err)
		}

		domain := apitypes.TypedDataDomain{
			Name:              "Polymarket CTF Exchange",
			Version:           "2",
			ChainId:           chainIDDomain(137),
			VerifyingContract: StandardExchangeAddress,
		}
		message := apitypes.TypedDataMessage{
			"salt":          order.Salt,
			"maker":         order.Maker,
			"signer":        order.Signer,
			"tokenId":       order.TokenID,
			"makerAmount":   order.MakerAmount,
			"takerAmount":   order.TakerAmount,
			"side":          map[Side]string{Buy: "0", Sell: "1"}[side],
			"signatureType": "0",
			"timestamp":     order.Timestamp,
			"metadata":      order.Metadata,
			"builder":       order.Builder,
		}
		td := apitypes.TypedData{
			Types:       apitypes.Types{"EIP712Domain": domainType(domain), "Order": orderType["Order"]},
			PrimaryType: "Order",
			Domain:      domain,
			Message:     message,
		}
		digest, _, err := apitypes.TypedDataAndHash(td)
		if err != nil {
			t.Fatalf("TypedDataAndHash() error = %v", err)
		}

		sig, err := hexutil.Decode(order.Signature)
		if err != nil {
			t.Fatalf("decoding signature: %v", err)
		}
		if len(sig) != 65 {
			t.Fatalf("signature length = %d, want 65", len(sig))
		}
		if sig[64] != 27 && sig[64] != 28 {
			t.Fatalf("signature recovery byte = %d, want 27 or 28", sig[64])
		}
		recoverSig := append([]byte{}, sig...)
		recoverSig[64] -= 27

		pub, err := crypto.SigToPub(digest, recoverSig)
		if err != nil {
			t.Fatalf("SigToPub() error = %v", err)
		}
		got := crypto.PubkeyToAddress(*pub)
		if got != w.Address {
			t.Fatalf("recovered address = %s, want %s", got.Hex(), w.Address.Hex())
		}
	}
}

func TestBuildAndSignValidatesInput(t *testing.T) {
	w := mustTestWallet(t)

	cases := []struct {
		name string
		args OrderArgs
	}{
		{"bad side", OrderArgs{TokenID: "1", Price: 0.5, Size: 1, Side: "HOLD"}},
		{"price too low", OrderArgs{TokenID: "1", Price: 0, Size: 1, Side: Buy}},
		{"price too high", OrderArgs{TokenID: "1", Price: 1, Size: 1, Side: Buy}},
		{"zero size", OrderArgs{TokenID: "1", Price: 0.5, Size: 0, Side: Buy}},
		{"non-numeric tokenId", OrderArgs{TokenID: "not-a-number", Price: 0.5, Size: 1, Side: Buy}},
		{"negative tokenId", OrderArgs{TokenID: "-5", Price: 0.5, Size: 1, Side: Buy}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := BuildAndSign(w, 137, "", 0, c.args, 1700000000000); err == nil {
				t.Fatalf("BuildAndSign(%+v) expected error, got nil", c.args)
			}
		})
	}
}

func TestBuildAndSignRoundsToTickSize(t *testing.T) {
	w := mustTestWallet(t)
	order, err := BuildAndSign(w, 137, "", 0, OrderArgs{
		TokenID:  "1",
		Price:    0.4523,
		Size:     10,
		Side:     Buy,
		TickSize: 0.01,
	}, 1700000000000)
	if err != nil {
		t.Fatalf("BuildAndSign() error = %v", err)
	}
	// price 0.45 * 10 * 1e6 = 4_500_000
	if order.MakerAmount != "4500000" {
		t.Fatalf("makerAmount = %s, want 4500000 (price should round to tick size 0.01)", order.MakerAmount)
	}
}

// TestBuildAndSignAmountRounding guards against float64 truncation shaving an
// atomic unit off a signed order amount: price=0.7, size=33.33 computes a
// takerAmount whose exact value (23331000) lands a hair below its integer in
// float64 (23330999.999999996), which big.Float.Int truncates to 23330999
// unless rounded to nearest first.
func TestBuildAndSignAmountRounding(t *testing.T) {
	w := mustTestWallet(t)
	order, err := BuildAndSign(w, 137, "", 0, OrderArgs{
		TokenID: "1",
		Price:   0.7,
		Size:    33.33,
		Side:    Sell,
	}, 1700000000000)
	if err != nil {
		t.Fatalf("BuildAndSign() error = %v", err)
	}
	if order.TakerAmount != "23331000" {
		t.Fatalf("takerAmount = %s, want 23331000 (0.7 * 33.33 * 1e6, rounded)", order.TakerAmount)
	}
}

func TestZeroBytes32Length(t *testing.T) {
	hexPart := strings.TrimPrefix(zeroBytes32, "0x")
	b, err := hex.DecodeString(hexPart)
	if err != nil {
		t.Fatalf("decoding zeroBytes32: %v", err)
	}
	if len(b) != 32 {
		t.Fatalf("zeroBytes32 decodes to %d bytes, want 32", len(b))
	}
}
