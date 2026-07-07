// SPDX-License-Identifier: MIT

package app

import (
	"testing"

	"github.com/rangertaha/polymarket-mcp/internal/config"
)

// testPrivateKey is a throwaway key; it holds no funds and appears nowhere else.
const testPrivateKey = "4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"

func TestAssembleWithoutTradingRegistersOnlyMarkets(t *testing.T) {
	cfg := &config.Config{BaseURL: config.DefaultBaseURL}

	srv, cleanup, err := Assemble(cfg, "test")
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	defer cleanup()

	if srv.ToolCount() != 2 {
		t.Errorf("ToolCount() = %d, want 2 (markets_list, markets_get only)", srv.ToolCount())
	}
	if got := srv.Toolsets(); len(got) != 1 || got[0] != "markets" {
		t.Errorf("Toolsets() = %v, want [markets] (trading must not register without a key)", got)
	}
	if srv.PromptCount() != 1 {
		t.Errorf("PromptCount() = %d, want 1", srv.PromptCount())
	}
}

func TestAssembleWithTradingRegistersBothToolsets(t *testing.T) {
	cfg := &config.Config{
		BaseURL:     config.DefaultBaseURL,
		CLOBBaseURL: config.DefaultCLOBBaseURL,
		ChainID:     config.DefaultChainID,
		PrivateKey:  testPrivateKey,
	}

	srv, cleanup, err := Assemble(cfg, "test")
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	defer cleanup()

	if srv.ToolCount() != 11 {
		t.Errorf("ToolCount() = %d, want 11 (2 markets + 9 trading)", srv.ToolCount())
	}
	if got := srv.Toolsets(); len(got) != 2 || got[0] != "markets" || got[1] != "trading" {
		t.Errorf("Toolsets() = %v, want [markets trading]", got)
	}
}

func TestAssembleRespectsReadOnly(t *testing.T) {
	cfg := &config.Config{
		BaseURL:     config.DefaultBaseURL,
		CLOBBaseURL: config.DefaultCLOBBaseURL,
		ChainID:     config.DefaultChainID,
		PrivateKey:  testPrivateKey,
		ReadOnly:    true,
	}

	srv, cleanup, err := Assemble(cfg, "test")
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	defer cleanup()

	if !srv.ReadOnly() {
		t.Error("ReadOnly() = false, want true")
	}
	// 2 read-only markets tools + 6 read-only trading tools; the 3 write
	// trading tools (place/cancel/cancel-all) must be hidden.
	if srv.ToolCount() != 8 {
		t.Errorf("ToolCount() = %d, want 8 in read-only mode", srv.ToolCount())
	}
}

func TestAssembleRespectsToolsetFilterEvenWithTradingConfigured(t *testing.T) {
	cfg := &config.Config{
		BaseURL:     config.DefaultBaseURL,
		CLOBBaseURL: config.DefaultCLOBBaseURL,
		ChainID:     config.DefaultChainID,
		PrivateKey:  testPrivateKey,
		Toolsets:    []string{"markets"}, // trading excluded, even though a key is configured
	}

	srv, cleanup, err := Assemble(cfg, "test")
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	defer cleanup()

	if got := srv.Toolsets(); len(got) != 1 || got[0] != "markets" {
		t.Errorf("Toolsets() = %v, want [markets] (POLYMARKET_TOOLSETS should exclude trading)", got)
	}
}

// TestAssembleRejectsUnknownToolsetName guards against a POLYMARKET_TOOLSETS
// typo silently producing a server with zero tools and no diagnostic:
// cfg.ToolsetEnabled simply never matches an unrecognized name, so every
// toolset would otherwise register nothing without any error at all.
func TestAssembleRejectsUnknownToolsetName(t *testing.T) {
	cfg := &config.Config{
		BaseURL:  config.DefaultBaseURL,
		Toolsets: []string{"makets"}, // typo of "markets"
	}

	_, _, err := Assemble(cfg, "test")
	if err == nil {
		t.Fatal("Assemble() expected error for an unknown toolset name, got nil")
	}
}

func TestAssembleInvalidBaseURL(t *testing.T) {
	cfg := &config.Config{BaseURL: "http://exa\nmple.com"}

	if _, _, err := Assemble(cfg, "test"); err == nil {
		t.Fatal("Assemble() expected error for invalid BaseURL, got nil")
	}
}

func TestAssembleInvalidCLOBBaseURL(t *testing.T) {
	cfg := &config.Config{
		BaseURL:     config.DefaultBaseURL,
		CLOBBaseURL: "http://exa\nmple.com",
		ChainID:     config.DefaultChainID,
		PrivateKey:  testPrivateKey,
	}

	if _, _, err := Assemble(cfg, "test"); err == nil {
		t.Fatal("Assemble() expected error for invalid CLOBBaseURL, got nil")
	}
}

func TestAssembleInvalidPrivateKey(t *testing.T) {
	cfg := &config.Config{
		BaseURL:    config.DefaultBaseURL,
		PrivateKey: "not-a-valid-hex-key",
	}

	if _, _, err := Assemble(cfg, "test"); err == nil {
		t.Fatal("Assemble() expected error for invalid private key, got nil")
	}
}
