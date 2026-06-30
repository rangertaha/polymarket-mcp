// SPDX-License-Identifier: MIT

package markets

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rangertaha/polymarket-mcp/internal/polymarket"
	"github.com/rangertaha/polymarket-mcp/internal/server"
)

// Register adds the markets toolset to the server.
func Register(s *server.Server, c *polymarket.Clients) {
	s.NoteToolset(Name)
	svc := &service{c: c}

	server.Register(s, server.ToolDef{
		Name:        "markets_list",
		Title:       "List markets",
		Description: "List Polymarket markets, optionally filtered to active and/or closed markets, with paging.",
	}, svc.list)

	server.Register(s, server.ToolDef{
		Name:        "markets_get",
		Title:       "Get market",
		Description: "Get a single Polymarket market by its numeric ID.",
	}, svc.get)
}

// --- Tool input types (schemas are inferred from these structs) ---

// ListInput filters and pages the markets list.
type ListInput struct {
	Active *bool `json:"active,omitempty" jsonschema:"only markets currently accepting orders (optional)"`
	Closed *bool `json:"closed,omitempty" jsonschema:"only resolved/closed markets (optional)"`
	Limit  int   `json:"limit,omitempty" jsonschema:"maximum number of markets to return (optional)"`
	Offset int   `json:"offset,omitempty" jsonschema:"number of markets to skip for paging (optional)"`
}

// GetInput identifies a single market.
type GetInput struct {
	ID string `json:"id" jsonschema:"market ID (numeric, as a string)"`
}

// --- Tool handlers ---

func (s *service) list(ctx context.Context, _ *mcp.CallToolRequest, in ListInput) (*mcp.CallToolResult, server.ListResult[Market], error) {
	out, err := s.ListMarkets(ctx, in.Active, in.Closed, in.Limit, in.Offset)
	return nil, server.List(out), err
}

func (s *service) get(ctx context.Context, _ *mcp.CallToolRequest, in GetInput) (*mcp.CallToolResult, *Market, error) {
	out, err := s.GetMarket(ctx, in.ID)
	return nil, out, err
}
