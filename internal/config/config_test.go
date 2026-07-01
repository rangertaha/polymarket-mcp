// SPDX-License-Identifier: MIT

package config

import "testing"

// testPrivateKey is a throwaway key; it holds no funds and appears nowhere else.
const testPrivateKey = "4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"

// clearEnv resets every variable Load reads, so each test starts from a known
// baseline regardless of what the outer environment (or a prior subtest, since
// t.Setenv restores on cleanup rather than immediately) happens to have set.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{EnvBaseURL, EnvToolsets, EnvReadOnly, EnvPrivateKey, EnvCLOBBaseURL, EnvChainID, EnvFunderAddress, EnvSignatureType} {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, DefaultBaseURL)
	}
	if cfg.CLOBBaseURL != DefaultCLOBBaseURL {
		t.Errorf("CLOBBaseURL = %q, want %q", cfg.CLOBBaseURL, DefaultCLOBBaseURL)
	}
	if cfg.ChainID != DefaultChainID {
		t.Errorf("ChainID = %d, want %d", cfg.ChainID, DefaultChainID)
	}
	if cfg.ReadOnly {
		t.Error("ReadOnly = true, want false")
	}
	if cfg.TradingEnabled() {
		t.Error("TradingEnabled() = true, want false with no private key")
	}
	if !cfg.AllToolsets() {
		t.Error("AllToolsets() = false, want true with no POLYMARKET_TOOLSETS set")
	}
}

func TestLoadInvalidBaseURL(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvBaseURL, "not a url")

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid POLYMARKET_BASE_URL, got nil")
	}
}

// TestLoadIgnoresMalformedTradingFieldsWithoutKey guards the fix for a bug
// where a malformed trading-only variable (chain ID, CLOB URL, signature
// type) blocked commands that never touch the CLOB, like `polymarket test`,
// even though POLYMARKET_PRIVATE_KEY was never set.
func TestLoadIgnoresMalformedTradingFieldsWithoutKey(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"bad chain id", map[string]string{EnvChainID: "not-a-number"}},
		{"bad clob base url", map[string]string{EnvCLOBBaseURL: "not a url"}},
		{"bad signature type", map[string]string{EnvSignatureType: "99"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err != nil {
				t.Fatalf("Load() error = %v, want nil (trading fields unused without a private key)", err)
			}
		})
	}
}

// TestLoadValidatesTradingFieldsWithKey is the mirror image: once a private
// key is supplied, trading actually will use these fields, so malformed
// values must be rejected.
func TestLoadValidatesTradingFieldsWithKey(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"bad private key", map[string]string{EnvPrivateKey: "not-hex"}},
		{"bad chain id", map[string]string{EnvPrivateKey: testPrivateKey, EnvChainID: "not-a-number"}},
		{"bad clob base url", map[string]string{EnvPrivateKey: testPrivateKey, EnvCLOBBaseURL: "not a url"}},
		{"signature type too high", map[string]string{EnvPrivateKey: testPrivateKey, EnvSignatureType: "3"}},
		{"signature type negative", map[string]string{EnvPrivateKey: testPrivateKey, EnvSignatureType: "-1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatal("Load() expected error, got nil")
			}
		})
	}
}

func TestLoadTradingEnabled(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvPrivateKey, "0x"+testPrivateKey) // leading "0x" must be accepted
	t.Setenv(EnvChainID, "80002")
	t.Setenv(EnvSignatureType, "1")
	t.Setenv(EnvFunderAddress, "0x1234567890123456789012345678901234567890")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.TradingEnabled() {
		t.Error("TradingEnabled() = false, want true")
	}
	if cfg.PrivateKey != testPrivateKey {
		t.Errorf("PrivateKey = %q, want %q (leading 0x stripped)", cfg.PrivateKey, testPrivateKey)
	}
	if cfg.ChainID != 80002 {
		t.Errorf("ChainID = %d, want 80002", cfg.ChainID)
	}
	if cfg.SignatureType != 1 {
		t.Errorf("SignatureType = %d, want 1", cfg.SignatureType)
	}
	if cfg.FunderAddress != "0x1234567890123456789012345678901234567890" {
		t.Errorf("FunderAddress = %q, unexpected", cfg.FunderAddress)
	}
}

func TestToolsetEnabled(t *testing.T) {
	cases := []struct {
		name     string
		toolsets []string
		query    string
		want     bool
	}{
		{"empty means all", nil, "markets", true},
		{"explicit all", []string{"all"}, "trading", true},
		{"listed", []string{"markets", "trading"}, "trading", true},
		{"not listed", []string{"markets"}, "trading", false},
		{"case insensitive", []string{"Markets"}, "markets", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := &Config{Toolsets: c.toolsets}
			if got := cfg.ToolsetEnabled(c.query); got != c.want {
				t.Errorf("ToolsetEnabled(%q) = %v, want %v (toolsets=%v)", c.query, got, c.want, c.toolsets)
			}
		})
	}
}

func TestSplitList(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"  ", nil},
		{"markets", []string{"markets"}},
		{"Markets, Trading ,,markets", []string{"markets", "trading", "markets"}},
	}
	for _, c := range cases {
		got := splitList(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("splitList(%q) = %v, want %v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("splitList(%q) = %v, want %v", c.in, got, c.want)
			}
		}
	}
}

func TestIsTruthy(t *testing.T) {
	truthy := []string{"1", "true", "TRUE", "yes", "on", " true "}
	falsy := []string{"", "0", "false", "no", "off", "garbage"}
	for _, v := range truthy {
		if !isTruthy(v) {
			t.Errorf("isTruthy(%q) = false, want true", v)
		}
	}
	for _, v := range falsy {
		if isTruthy(v) {
			t.Errorf("isTruthy(%q) = true, want false", v)
		}
	}
}
