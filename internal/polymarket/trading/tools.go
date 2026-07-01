// SPDX-License-Identifier: MIT

package trading

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rangertaha/polymarket-mcp/internal/clob"
	"github.com/rangertaha/polymarket-mcp/internal/polymarket"
	"github.com/rangertaha/polymarket-mcp/internal/server"
)

// Register adds the trading toolset to the server. It registers no tools
// (and does not count as an enabled toolset) unless POLYMARKET_PRIVATE_KEY
// was configured, so the server keeps serving the public Gamma data API
// unaffected when no wallet is set up.
func Register(s *server.Server, c *polymarket.Clients) {
	if c.CLOB == nil {
		return
	}
	s.NoteToolset(Name)
	svc := &service{c: c}

	server.Register(s, server.ToolDef{
		Name:        "trading_get_order_book",
		Title:       "Get order book",
		Description: "Get the CLOB order book (bids, asks, tick size, neg-risk flag) for an outcome token.",
	}, svc.getOrderBook)

	server.Register(s, server.ToolDef{
		Name:        "trading_get_price",
		Title:       "Get best price",
		Description: "Get the best bid (side=BUY) or best ask (side=SELL) price for an outcome token.",
	}, svc.getPrice)

	server.Register(s, server.ToolDef{
		Name:        "trading_get_midpoint",
		Title:       "Get midpoint price",
		Description: "Get the midpoint between the best bid and best ask for an outcome token.",
	}, svc.getMidpoint)

	server.Register(s, server.ToolDef{
		Name:        "trading_get_balance",
		Title:       "Get balance",
		Description: "Get the authenticated wallet's collateral balance (default) or a specific outcome token balance, plus exchange allowances.",
	}, svc.getBalance)

	server.Register(s, server.ToolDef{
		Name:        "trading_list_open_orders",
		Title:       "List open orders",
		Description: "List the authenticated wallet's open CLOB orders, optionally filtered by market or outcome token.",
	}, svc.listOpenOrders)

	server.Register(s, server.ToolDef{
		Name:        "trading_list_trades",
		Title:       "List trades",
		Description: "List the authenticated wallet's trade (fill) history, optionally filtered by market or outcome token.",
	}, svc.listTrades)

	server.Register(s, server.ToolDef{
		Name:        "trading_place_order",
		Title:       "Place order",
		Description: "Sign and submit a limit order (BUY or SELL) for an outcome token. Moves real funds. Price is a probability in (0,1); the market's tick size and neg-risk exchange are looked up automatically.",
		Write:       true,
		Destructive: true,
	}, svc.placeOrder)

	server.Register(s, server.ToolDef{
		Name:        "trading_cancel_order",
		Title:       "Cancel order",
		Description: "Cancel a single open order by its ID (order hash).",
		Write:       true,
		Destructive: true,
	}, svc.cancelOrder)

	server.Register(s, server.ToolDef{
		Name:        "trading_cancel_all_orders",
		Title:       "Cancel all orders",
		Description: "Cancel every open order for the authenticated wallet.",
		Write:       true,
		Destructive: true,
	}, svc.cancelAllOrders)
}

// --- Tool input types (schemas are inferred from these structs) ---

// GetOrderBookInput identifies an outcome token's order book.
type GetOrderBookInput struct {
	TokenID string `json:"tokenId" jsonschema:"outcome token ID (asset ID)"`
}

// GetPriceInput requests the best bid or ask for an outcome token.
type GetPriceInput struct {
	TokenID string `json:"tokenId" jsonschema:"outcome token ID (asset ID)"`
	Side    string `json:"side" jsonschema:"BUY (best bid) or SELL (best ask)"`
}

// GetMidpointInput requests the midpoint price for an outcome token.
type GetMidpointInput struct {
	TokenID string `json:"tokenId" jsonschema:"outcome token ID (asset ID)"`
}

// GetBalanceInput selects which balance to check.
type GetBalanceInput struct {
	AssetType string `json:"assetType,omitempty" jsonschema:"COLLATERAL (default, USDC-pegged collateral) or CONDITIONAL (an outcome token)"`
	TokenID   string `json:"tokenId,omitempty" jsonschema:"outcome token ID; required when assetType is CONDITIONAL (optional)"`
}

// ListOpenOrdersInput filters the open-orders listing.
type ListOpenOrdersInput struct {
	Market  string `json:"market,omitempty" jsonschema:"filter by market condition ID (optional)"`
	AssetID string `json:"assetId,omitempty" jsonschema:"filter by outcome token ID (optional)"`
}

// ListTradesInput filters the trade-history listing.
type ListTradesInput struct {
	Market  string `json:"market,omitempty" jsonschema:"filter by market condition ID (optional)"`
	AssetID string `json:"assetId,omitempty" jsonschema:"filter by outcome token ID (optional)"`
}

// PlaceOrderInput describes a limit order to sign and submit.
type PlaceOrderInput struct {
	TokenID   string  `json:"tokenId" jsonschema:"ERC-1155 outcome token ID (decimal string) to trade"`
	Side      string  `json:"side" jsonschema:"BUY or SELL"`
	Price     float64 `json:"price" jsonschema:"limit price as a probability in (0,1), e.g. 0.45"`
	Size      float64 `json:"size" jsonschema:"outcome token quantity to buy or sell"`
	OrderType string  `json:"orderType,omitempty" jsonschema:"GTC (default, good-til-cancelled), FOK, GTD, or FAK (optional)"`
}

// CancelOrderInput identifies a single order to cancel.
type CancelOrderInput struct {
	OrderID string `json:"orderId" jsonschema:"order ID (hash) to cancel"`
}

// CancelAllOrdersInput takes no parameters.
type CancelAllOrdersInput struct{}

// PriceResult wraps a scalar price so the tool's structured output is a JSON
// object, as MCP requires.
type PriceResult struct {
	Price float64 `json:"price" jsonschema:"best bid (BUY) or best ask (SELL) price"`
}

// MidpointResult wraps the scalar midpoint price.
type MidpointResult struct {
	MidPrice string `json:"midPrice" jsonschema:"midpoint between the best bid and best ask"`
}

// --- Tool handlers ---

func (s *service) getOrderBook(ctx context.Context, _ *mcp.CallToolRequest, in GetOrderBookInput) (*mcp.CallToolResult, *OrderBook, error) {
	out, err := s.GetOrderBook(ctx, in.TokenID)
	return nil, out, err
}

func (s *service) getPrice(ctx context.Context, _ *mcp.CallToolRequest, in GetPriceInput) (*mcp.CallToolResult, PriceResult, error) {
	price, err := s.GetPrice(ctx, in.TokenID, in.Side)
	return nil, PriceResult{Price: price}, err
}

func (s *service) getMidpoint(ctx context.Context, _ *mcp.CallToolRequest, in GetMidpointInput) (*mcp.CallToolResult, MidpointResult, error) {
	mid, err := s.GetMidpoint(ctx, in.TokenID)
	return nil, MidpointResult{MidPrice: mid}, err
}

func (s *service) getBalance(ctx context.Context, _ *mcp.CallToolRequest, in GetBalanceInput) (*mcp.CallToolResult, *Balance, error) {
	out, err := s.GetBalance(ctx, in.AssetType, in.TokenID)
	return nil, out, err
}

func (s *service) listOpenOrders(ctx context.Context, _ *mcp.CallToolRequest, in ListOpenOrdersInput) (*mcp.CallToolResult, server.ListResult[Order], error) {
	out, err := s.ListOpenOrders(ctx, in.Market, in.AssetID)
	return nil, server.List(out), err
}

func (s *service) listTrades(ctx context.Context, _ *mcp.CallToolRequest, in ListTradesInput) (*mcp.CallToolResult, server.ListResult[Trade], error) {
	out, err := s.ListTrades(ctx, in.Market, in.AssetID)
	return nil, server.List(out), err
}

func (s *service) placeOrder(ctx context.Context, _ *mcp.CallToolRequest, in PlaceOrderInput) (*mcp.CallToolResult, *PlaceOrderResult, error) {
	out, err := s.PlaceOrder(ctx, in.TokenID, clob.Side(in.Side), in.Price, in.Size, in.OrderType)
	return nil, out, err
}

func (s *service) cancelOrder(ctx context.Context, _ *mcp.CallToolRequest, in CancelOrderInput) (*mcp.CallToolResult, *CancelResult, error) {
	out, err := s.CancelOrder(ctx, in.OrderID)
	return nil, out, err
}

func (s *service) cancelAllOrders(ctx context.Context, _ *mcp.CallToolRequest, _ CancelAllOrdersInput) (*mcp.CallToolResult, *CancelResult, error) {
	out, err := s.CancelAllOrders(ctx)
	return nil, out, err
}
