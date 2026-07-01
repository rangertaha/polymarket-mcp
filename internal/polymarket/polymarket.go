// SPDX-License-Identifier: MIT

// Package polymarket holds the connection to the Polymarket Gamma API that the
// per-area tool packages (markets, …) share.
package polymarket

import (
	"fmt"

	"github.com/rangertaha/polymarket-mcp/internal/client"
	"github.com/rangertaha/polymarket-mcp/internal/clob"
)

// Clients bundles the REST clients needed to reach Polymarket.
type Clients struct {
	// Gamma reaches the public Gamma data API host.
	Gamma *client.Client

	// CLOB reaches the authenticated CLOB trading API host. It is nil unless
	// a wallet private key was configured, in which case the trading toolset
	// is the only thing that uses it; every other toolset only ever touches
	// Gamma. Toolsets that need it must check for nil and register no tools
	// rather than fail server startup.
	CLOB *client.Client
	// Auth is the CLOB toolset's HMAC authorizer, exposed so it can gate a
	// request on Ensure(ctx) before an authenticated call.
	Auth *clob.Authorizer
	// Wallet signs CLOB orders. Nil unless CLOB is configured.
	Wallet *clob.Wallet
	// Funder is the maker/funder address that holds trading funds. Empty
	// means Wallet.Address funds its own orders directly.
	Funder string
	// SignatureType selects the CTF Exchange order signature scheme (0=EOA,
	// 1=proxy wallet, 2=Gnosis Safe), matching how Funder holds funds.
	SignatureType int
	// ChainID is the EVM chain ID used for EIP-712 order signing.
	ChainID int64
}

// NewClients builds the Polymarket Gamma client for the given base URL.
//
// The Gamma data API is public and needs no authentication. Call
// EnableTrading afterward to wire in the authenticated CLOB client.
func NewClients(baseURL string, opts ...client.Option) (*Clients, error) {
	base := append([]client.Option{client.WithUserAgent("polymarket-mcp")}, opts...)

	gamma, err := client.New(baseURL, nil, base...)
	if err != nil {
		return nil, fmt.Errorf("creating polymarket client: %w", err)
	}
	return &Clients{Gamma: gamma}, nil
}

// EnableTrading wires an authenticated CLOB trading client into c using the
// given wallet. L2 API credentials are derived lazily on first authenticated
// request, not here, so this never makes a network call.
func (c *Clients) EnableTrading(w *clob.Wallet, baseURL string, chainID int64, funder string, signatureType int, opts ...client.Option) error {
	base := append([]client.Option{client.WithUserAgent("polymarket-mcp")}, opts...)

	// Resolve the effective *http.Client (respecting a caller-supplied
	// WithHTTPClient) once, so the Authorizer's own L1 credential-derivation
	// requests use the same transport/timeout/proxy as ordinary CLOB calls
	// rather than silently falling back to a default client.
	probe, err := client.New(baseURL, nil, base...)
	if err != nil {
		return fmt.Errorf("creating CLOB client: %w", err)
	}

	auth := clob.NewAuthorizer(w, baseURL, chainID, probe.HTTPClient())
	clobClient, err := client.New(baseURL, auth, base...)
	if err != nil {
		return fmt.Errorf("creating CLOB client: %w", err)
	}

	c.CLOB = clobClient
	c.Auth = auth
	c.Wallet = w
	c.Funder = funder
	c.SignatureType = signatureType
	c.ChainID = chainID
	return nil
}
