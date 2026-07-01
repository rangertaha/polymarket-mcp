// SPDX-License-Identifier: MIT

package polymarket

import (
	"testing"

	"github.com/rangertaha/polymarket-mcp/internal/clob"
)

// testPrivateKey is a throwaway key; it holds no funds and appears nowhere else.
const testPrivateKey = "4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"

func TestNewClientsHasNoCLOBByDefault(t *testing.T) {
	c, err := NewClients("https://gamma-api.polymarket.com")
	if err != nil {
		t.Fatalf("NewClients() error = %v", err)
	}
	if c.Gamma == nil {
		t.Fatal("Gamma client is nil")
	}
	if c.CLOB != nil {
		t.Error("CLOB client should be nil until EnableTrading is called")
	}
}

func TestNewClientsRejectsInvalidBaseURL(t *testing.T) {
	if _, err := NewClients("://not-a-url"); err == nil {
		t.Fatal("NewClients() expected error for invalid base URL, got nil")
	}
}

func TestEnableTradingWiresClients(t *testing.T) {
	c, err := NewClients("https://gamma-api.polymarket.com")
	if err != nil {
		t.Fatalf("NewClients() error = %v", err)
	}

	w, err := clob.NewWallet(testPrivateKey)
	if err != nil {
		t.Fatalf("NewWallet() error = %v", err)
	}

	if err := c.EnableTrading(w, "https://clob.polymarket.com", 137, "0xfunder", 1); err != nil {
		t.Fatalf("EnableTrading() error = %v", err)
	}

	if c.CLOB == nil {
		t.Fatal("CLOB client is nil after EnableTrading")
	}
	if c.Auth == nil {
		t.Fatal("Auth is nil after EnableTrading")
	}
	if c.Wallet != w {
		t.Error("Wallet was not stored")
	}
	if c.Funder != "0xfunder" {
		t.Errorf("Funder = %q, want 0xfunder", c.Funder)
	}
	if c.SignatureType != 1 {
		t.Errorf("SignatureType = %d, want 1", c.SignatureType)
	}
	if c.ChainID != 137 {
		t.Errorf("ChainID = %d, want 137", c.ChainID)
	}
}
