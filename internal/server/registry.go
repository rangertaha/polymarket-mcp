// SPDX-License-Identifier: MIT

package server

import "github.com/rangertaha/polymarket-mcp/internal/polymarket"

// Toolset pairs a toolset name with the function that registers its tools.
// Each service area exposes one of these so the entrypoint can register only
// the toolsets enabled by configuration.
type Toolset struct {
	Name     string
	Register func(s *Server, c *polymarket.Clients)
}
