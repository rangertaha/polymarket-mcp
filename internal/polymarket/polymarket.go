// SPDX-License-Identifier: MIT

// Package polymarket holds the connection to the Polymarket Gamma API that the
// per-area tool packages (markets, …) share.
package polymarket

import (
	"fmt"

	"github.com/rangertaha/polymarket-mcp/internal/client"
)

// Clients bundles the REST clients needed to reach Polymarket.
type Clients struct {
	// Gamma reaches the public Gamma data API host.
	Gamma *client.Client
}

// NewClients builds the Polymarket Gamma client for the given base URL.
//
// The Gamma data API is public and needs no authentication. Authenticated CLOB
// trading endpoints (added by later toolsets) will need their own signed
// client; wire it in here.
func NewClients(baseURL string, opts ...client.Option) (*Clients, error) {
	base := append([]client.Option{client.WithUserAgent("polymarket-mcp")}, opts...)

	gamma, err := client.New(baseURL, nil, base...)
	if err != nil {
		return nil, fmt.Errorf("creating polymarket client: %w", err)
	}
	return &Clients{Gamma: gamma}, nil
}
