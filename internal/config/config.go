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
	"strings"
)

// Environment variable names recognised by the server.
const (
	EnvBaseURL  = "POLYMARKET_BASE_URL" // override the Gamma API base URL
	EnvToolsets = "POLYMARKET_TOOLSETS" // comma-separated toolset names, or "all"
	EnvReadOnly = "POLYMARKET_READONLY" // "true" disables all write tools
)

// DefaultBaseURL is the Polymarket Gamma API base used when POLYMARKET_BASE_URL
// is unset.
const DefaultBaseURL = "https://gamma-api.polymarket.com"

// Config holds validated server configuration.
type Config struct {
	// BaseURL is the Gamma REST base URL (never has a trailing slash).
	BaseURL string
	// Toolsets is the set of enabled toolset names. A nil/empty set means "all".
	Toolsets []string
	// ReadOnly, when true, suppresses mutating tools at registration time.
	ReadOnly bool
}

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
		BaseURL:  strings.TrimRight(strings.TrimSpace(os.Getenv(EnvBaseURL)), "/"),
		Toolsets: splitList(os.Getenv(EnvToolsets)),
		ReadOnly: isTruthy(os.Getenv(EnvReadOnly)),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}

	var errs []error
	if u, err := url.Parse(cfg.BaseURL); err != nil || u.Scheme == "" || u.Host == "" {
		errs = append(errs, fmt.Errorf("%s is not a valid URL: %q", EnvBaseURL, cfg.BaseURL))
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
