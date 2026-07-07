// SPDX-License-Identifier: MIT

// Package config loads and validates runtime configuration for the
// polymarket-mcp server from environment variables.
//
// All configuration is supplied via the environment so the server can run as a
// stdio subprocess launched by an MCP client (Claude Desktop/Code, Cursor, …),
// where command-line flags are awkward to pass.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Environment variable names recognised by the server.
const (
	EnvBaseURL       = "POLYMARKET_BASE_URL"       // override the Gamma API base URL
	EnvToolsets      = "POLYMARKET_TOOLSETS"       // comma-separated toolset names, or "all"
	EnvReadOnly      = "POLYMARKET_READONLY"       // "true" disables all write tools
	EnvPrivateKey    = "POLYMARKET_PRIVATE_KEY"    // wallet private key; enables the trading toolset
	EnvCLOBBaseURL   = "POLYMARKET_CLOB_BASE_URL"  // override the CLOB trading API base URL
	EnvChainID       = "POLYMARKET_CHAIN_ID"       // EVM chain ID for order signing (default: Polygon)
	EnvFunderAddress = "POLYMARKET_FUNDER_ADDRESS" // maker/funder address, if different from the signing wallet
	EnvSignatureType = "POLYMARKET_SIGNATURE_TYPE" // order signature type: 0=EOA, 1=proxy, 2=Gnosis Safe
)

// DefaultBaseURL is the Polymarket Gamma API base used when POLYMARKET_BASE_URL
// is unset.
const DefaultBaseURL = "https://gamma-api.polymarket.com"

// DefaultCLOBBaseURL is the Polymarket CLOB trading API base used when
// POLYMARKET_CLOB_BASE_URL is unset.
const DefaultCLOBBaseURL = "https://clob.polymarket.com"

// DefaultChainID is the Polygon mainnet chain ID used when POLYMARKET_CHAIN_ID
// is unset.
const DefaultChainID = 137

// Config holds validated server configuration.
type Config struct {
	// BaseURL is the Gamma REST base URL (never has a trailing slash).
	BaseURL string
	// Toolsets is the set of enabled toolset names. A nil/empty set means "all".
	Toolsets []string
	// ReadOnly, when true, suppresses mutating tools at registration time.
	ReadOnly bool

	// PrivateKey is a hex-encoded secp256k1 wallet private key used to sign
	// CLOB orders and derive trading API credentials. Empty disables trading:
	// the server falls back to the public, read-only Gamma data API only.
	PrivateKey string
	// CLOBBaseURL is the CLOB trading REST base URL (never has a trailing slash).
	CLOBBaseURL string
	// ChainID is the EVM chain ID used for EIP-712 order signing.
	ChainID int64
	// FunderAddress is the maker/funder address holding trading funds. Empty
	// means the wallet derived from PrivateKey funds its own orders directly.
	FunderAddress string
	// SignatureType selects the order signature scheme (0=EOA, 1=proxy wallet,
	// 2=Gnosis Safe), matching Polymarket's CTF Exchange signature types.
	SignatureType int
}

// TradingEnabled reports whether trading credentials were supplied. When
// false, the trading toolset registers no tools and the server serves only
// the public Gamma data API.
func (c *Config) TradingEnabled() bool { return c.PrivateKey != "" }

// AllToolsets reports whether every toolset should be enabled.
func (c *Config) AllToolsets() bool {
	if len(c.Toolsets) == 0 {
		return true
	}
	for _, t := range c.Toolsets {
		if t == "all" {
			return true
		}
	}
	return false
}

// ToolsetEnabled reports whether the named toolset should be registered.
func (c *Config) ToolsetEnabled(name string) bool {
	if c.AllToolsets() {
		return true
	}
	for _, t := range c.Toolsets {
		if strings.EqualFold(t, name) {
			return true
		}
	}
	return false
}

// Load reads configuration from the process environment and validates it.
func Load() (*Config, error) {
	cfg := &Config{
		BaseURL:       strings.TrimRight(strings.TrimSpace(os.Getenv(EnvBaseURL)), "/"),
		Toolsets:      splitList(os.Getenv(EnvToolsets)),
		ReadOnly:      isTruthy(os.Getenv(EnvReadOnly)),
		PrivateKey:    stripHexPrefix(strings.TrimSpace(os.Getenv(EnvPrivateKey))),
		CLOBBaseURL:   strings.TrimRight(strings.TrimSpace(os.Getenv(EnvCLOBBaseURL)), "/"),
		FunderAddress: strings.TrimSpace(os.Getenv(EnvFunderAddress)),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.CLOBBaseURL == "" {
		cfg.CLOBBaseURL = DefaultCLOBBaseURL
	}

	var errs []error
	if u, err := url.Parse(cfg.BaseURL); err != nil || u.Scheme == "" || u.Host == "" {
		errs = append(errs, fmt.Errorf("%s is not a valid URL: %q", EnvBaseURL, cfg.BaseURL))
	}

	cfg.ChainID = DefaultChainID

	// Trading-only fields are validated only when a private key is actually
	// supplied. Commands that never touch the CLOB (e.g. `polymarket test`,
	// or the MCP server with no wallet configured) must not be blocked by a
	// malformed POLYMARKET_CHAIN_ID/POLYMARKET_CLOB_BASE_URL/etc. that will
	// never be used.
	if cfg.PrivateKey != "" {
		if u, err := url.Parse(cfg.CLOBBaseURL); err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s is not a valid URL: %q", EnvCLOBBaseURL, cfg.CLOBBaseURL))
		}

		if v := strings.TrimSpace(os.Getenv(EnvChainID)); v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s is not a valid integer: %q", EnvChainID, v))
			} else {
				cfg.ChainID = id
			}
		}

		if v := strings.TrimSpace(os.Getenv(EnvSignatureType)); v != "" {
			st, err := strconv.Atoi(v)
			if err != nil || st < 0 || st > 2 {
				errs = append(errs, fmt.Errorf("%s must be 0, 1, or 2: %q", EnvSignatureType, v))
			} else {
				cfg.SignatureType = st
			}
		}

		if _, err := crypto.HexToECDSA(cfg.PrivateKey); err != nil {
			errs = append(errs, fmt.Errorf("%s is not a valid private key: %w", EnvPrivateKey, err))
		}

		if cfg.FunderAddress != "" && !common.IsHexAddress(cfg.FunderAddress) {
			errs = append(errs, fmt.Errorf("%s is not a valid address: %q", EnvFunderAddress, cfg.FunderAddress))
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return cfg, nil
}

// splitList parses a comma-separated environment value into a trimmed,
// lower-cased slice, dropping empty entries.
func splitList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// isTruthy reports whether an environment value represents boolean true.
func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// stripHexPrefix removes a leading "0x"/"0X" from a hex-encoded value.
// crypto.HexToECDSA rejects any such prefix (case-sensitively matching would
// miss "0X", causing a valid key to be misreported as invalid).
func stripHexPrefix(s string) string {
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		return s[2:]
	}
	return s
}
