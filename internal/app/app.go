// SPDX-License-Identifier: MIT

// Package app assembles the fully-configured polymarket-mcp server from
// configuration. It is shared by the command entry point (cmd/polymarket) so
// the exact server the binary runs is the one under test.
package app

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/rangertaha/polymarket-mcp/internal/clob"
	"github.com/rangertaha/polymarket-mcp/internal/config"
	"github.com/rangertaha/polymarket-mcp/internal/polymarket"
	"github.com/rangertaha/polymarket-mcp/internal/polymarket/markets"
	"github.com/rangertaha/polymarket-mcp/internal/polymarket/trading"
	"github.com/rangertaha/polymarket-mcp/internal/prompts"
	"github.com/rangertaha/polymarket-mcp/internal/server"
)

// Assemble builds the fully-configured server (all enabled toolsets and
// prompts) and returns it with a cleanup function. version is reported to
// clients.
func Assemble(cfg *config.Config, version string) (*server.Server, func(), error) {
	clients, err := polymarket.NewClients(cfg.BaseURL)
	if err != nil {
		return nil, nil, err
	}

	// Trading is opt-in: without a wallet private key the server keeps
	// serving the public Gamma data API and simply registers no trading
	// tools (see trading.Register).
	if cfg.TradingEnabled() {
		wallet, err := clob.NewWallet(cfg.PrivateKey)
		if err != nil {
			return nil, nil, fmt.Errorf("loading trading wallet: %w", err)
		}
		if err := clients.EnableTrading(wallet, cfg.CLOBBaseURL, cfg.ChainID, cfg.FunderAddress, cfg.SignatureType); err != nil {
			return nil, nil, err
		}
	}

	all := toolsets()
	if err := validateToolsetNames(cfg, all); err != nil {
		return nil, nil, err
	}

	srv := server.New("polymarket-mcp", version, cfg.ReadOnly)

	for _, ts := range all {
		if cfg.ToolsetEnabled(ts.Name) {
			ts.Register(srv, clients)
		}
	}

	// Diagnostics go to stderr; stdout is reserved for the MCP protocol.
	log.SetOutput(os.Stderr)

	prompts.Register(srv)

	return srv, func() {}, nil
}

// toolsets returns every toolset registrar, in registration order. New service
// areas are added here.
func toolsets() []server.Toolset {
	return []server.Toolset{
		{Name: markets.Name, Register: markets.Register},
		{Name: trading.Name, Register: trading.Register},
	}
}

// validateToolsetNames rejects an unrecognized POLYMARKET_TOOLSETS entry
// (e.g. a typo) with a clear error. Without this, cfg.ToolsetEnabled simply
// never matches, and every toolset silently registers zero tools — leaving
// the server running with no diagnostic at all.
func validateToolsetNames(cfg *config.Config, all []server.Toolset) error {
	if cfg.AllToolsets() {
		return nil
	}
	known := make(map[string]bool, len(all))
	names := make([]string, len(all))
	for i, ts := range all {
		known[strings.ToLower(ts.Name)] = true
		names[i] = ts.Name
	}
	var unknown []string
	for _, want := range cfg.Toolsets {
		if !known[want] {
			unknown = append(unknown, want)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%s: unknown toolset(s) %s (valid: %s)",
			config.EnvToolsets, strings.Join(unknown, ", "), strings.Join(names, ", "))
	}
	return nil
}
