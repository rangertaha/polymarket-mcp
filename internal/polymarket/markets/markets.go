// SPDX-License-Identifier: MIT

// Package markets exposes Polymarket Gamma market data: listing markets and
// fetching a single market by ID.
package markets

import (
	"context"
	"fmt"
	"net/url"

	"github.com/rangertaha/polymarket-mcp/internal/polymarket"
)

// Name is the toolset name used for enable/disable filtering.
const Name = "markets"

// service wraps the Polymarket clients for market operations.
type service struct {
	c *polymarket.Clients
}

// Market is a Polymarket market, trimmed to the fields useful to an LLM. Note
// that Gamma returns several numeric fields (prices, volume) as JSON strings.
type Market struct {
	ID            string `json:"id"`
	Question      string `json:"question,omitempty"`
	Slug          string `json:"slug,omitempty"`
	ConditionID   string `json:"conditionId,omitempty"`
	Active        bool   `json:"active"`
	Closed        bool   `json:"closed"`
	Outcomes      string `json:"outcomes,omitempty"`      // JSON-encoded array, e.g. ["Yes","No"]
	OutcomePrices string `json:"outcomePrices,omitempty"` // JSON-encoded array of price strings
	Volume        string `json:"volume,omitempty"`
	Liquidity     string `json:"liquidity,omitempty"`
	EndDate       string `json:"endDate,omitempty"`
}

// ListMarkets returns markets, optionally filtered by active/closed and paged.
func (s *service) ListMarkets(ctx context.Context, active, closed *bool, limit, offset int) ([]Market, error) {
	q := url.Values{}
	if active != nil {
		q.Set("active", fmt.Sprintf("%t", *active))
	}
	if closed != nil {
		q.Set("closed", fmt.Sprintf("%t", *closed))
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", offset))
	}
	var out []Market
	if err := s.c.Gamma.GetJSON(ctx, "/markets", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetMarket returns a single market by its numeric ID.
func (s *service) GetMarket(ctx context.Context, id string) (*Market, error) {
	var m Market
	path := fmt.Sprintf("/markets/%s", url.PathEscape(id))
	if err := s.c.Gamma.GetJSON(ctx, path, nil, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
