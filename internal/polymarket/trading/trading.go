// SPDX-License-Identifier: MIT

// Package trading exposes Polymarket's CLOB (Central Limit Order Book)
// trading API: placing and canceling orders, open orders, trade history,
// balances/allowances, and order book/price data.
//
// Unlike the public Gamma data API, every operation here needs a funded,
// signing wallet (POLYMARKET_PRIVATE_KEY) and moves real money on Polygon
// mainnet. This code has not been exercised against live trading — verify
// carefully (small test orders first) before relying on it with real funds.
// Without a configured wallet, Register registers no tools and the server
// keeps serving the public Gamma data API unaffected.
package trading

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/rangertaha/polymarket-mcp/internal/clob"
	"github.com/rangertaha/polymarket-mcp/internal/polymarket"
)

// Name is the toolset name used for enable/disable filtering.
const Name = "trading"

// errNotConfigured is returned by every method when no wallet was configured.
// Register avoids this by not registering any tools in that case, but the
// service methods stay defensive since they may be reused elsewhere.
var errNotConfigured = errors.New("trading is not configured: set POLYMARKET_PRIVATE_KEY to enable it")

// service wraps the Polymarket clients for trading operations.
type service struct {
	c *polymarket.Clients
}

// requireCLOB reports whether the trading wallet/client was configured.
func (s *service) requireCLOB() error {
	if s.c.CLOB == nil {
		return errNotConfigured
	}
	return nil
}

// requireAuth ensures L2 API credentials are derived before an authenticated
// call. Derivation happens lazily (see clob.Authorizer.Ensure) so a wallet
// that's misconfigured or unreachable only fails trading calls, not startup.
func (s *service) requireAuth(ctx context.Context) error {
	if err := s.requireCLOB(); err != nil {
		return err
	}
	return s.c.Auth.Ensure(ctx)
}

// maker returns the funder address that owns trading funds: the configured
// funder override, or the wallet's own address.
func (s *service) maker() string {
	if s.c.Funder != "" {
		return s.c.Funder
	}
	return s.c.Wallet.Address.Hex()
}

// --- Domain types returned to the model ---

// PriceLevel is a single price/size row in an order book.
type PriceLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

// OrderBook is the CLOB order book summary for one outcome token.
type OrderBook struct {
	Market         string       `json:"market"`
	AssetID        string       `json:"asset_id"`
	Bids           []PriceLevel `json:"bids"`
	Asks           []PriceLevel `json:"asks"`
	MinOrderSize   string       `json:"min_order_size"`
	TickSize       string       `json:"tick_size"`
	NegRisk        bool         `json:"neg_risk"`
	LastTradePrice string       `json:"last_trade_price,omitempty"`
}

// Order is an open (or recently resting) CLOB order for the authenticated user.
type Order struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	MakerAddress string `json:"maker_address"`
	Market       string `json:"market"`
	AssetID      string `json:"asset_id"`
	Side         string `json:"side"`
	OriginalSize string `json:"original_size"`
	SizeMatched  string `json:"size_matched"`
	Price        string `json:"price"`
	Outcome      string `json:"outcome"`
	Expiration   string `json:"expiration"`
	OrderType    string `json:"order_type"`
	CreatedAt    int64  `json:"created_at"`
}

// Trade is a fill for the authenticated user.
type Trade struct {
	ID              string `json:"id"`
	Market          string `json:"market"`
	AssetID         string `json:"asset_id"`
	Side            string `json:"side"`
	Size            string `json:"size"`
	FeeRateBps      string `json:"fee_rate_bps"`
	Price           string `json:"price"`
	Status          string `json:"status"`
	Outcome         string `json:"outcome"`
	MakerAddress    string `json:"maker_address"`
	TransactionHash string `json:"transaction_hash,omitempty"`
	MatchTime       string `json:"match_time"`
}

// Balance is the collateral or conditional-token balance/allowances for the
// authenticated user's address.
type Balance struct {
	Balance    string            `json:"balance"`
	Allowances map[string]string `json:"allowances"`
}

// PlaceOrderResult is the CLOB's response to a submitted order.
type PlaceOrderResult struct {
	Success      bool   `json:"success"`
	OrderID      string `json:"orderID"`
	Status       string `json:"status"`
	MakingAmount string `json:"makingAmount,omitempty"`
	TakingAmount string `json:"takingAmount,omitempty"`
	ErrorMsg     string `json:"errorMsg,omitempty"`
}

// CancelResult reports which orders a cancel request actually canceled.
type CancelResult struct {
	Canceled    []string          `json:"canceled"`
	NotCanceled map[string]string `json:"not_canceled"`
}

// --- Business methods ---

// GetOrderBook returns the public order book summary for an outcome token,
// including its tick size and whether it settles through the neg-risk
// exchange — both needed to build a valid order for it.
func (s *service) GetOrderBook(ctx context.Context, tokenID string) (*OrderBook, error) {
	if err := s.requireCLOB(); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("token_id", tokenID)
	var out OrderBook
	if err := s.c.CLOB.GetJSON(ctx, "/book", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPrice returns the best bid (side=BUY) or best ask (side=SELL) for an
// outcome token.
func (s *service) GetPrice(ctx context.Context, tokenID string, side string) (float64, error) {
	if err := s.requireCLOB(); err != nil {
		return 0, err
	}
	q := url.Values{}
	q.Set("token_id", tokenID)
	q.Set("side", side)
	var out struct {
		Price float64 `json:"price"`
	}
	if err := s.c.CLOB.GetJSON(ctx, "/price", q, &out); err != nil {
		return 0, err
	}
	return out.Price, nil
}

// GetMidpoint returns the midpoint between the best bid and best ask for an
// outcome token.
func (s *service) GetMidpoint(ctx context.Context, tokenID string) (string, error) {
	if err := s.requireCLOB(); err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("token_id", tokenID)
	var out struct {
		MidPrice string `json:"mid_price"`
	}
	if err := s.c.CLOB.GetJSON(ctx, "/midpoint", q, &out); err != nil {
		return "", err
	}
	return out.MidPrice, nil
}

// PlaceOrder builds, signs, and submits a limit order. Price is looked up
// against the market's tick size and neg-risk exchange automatically via
// GetOrderBook before signing.
func (s *service) PlaceOrder(ctx context.Context, tokenID string, side clob.Side, price, size float64, orderType string) (*PlaceOrderResult, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}

	book, err := s.GetOrderBook(ctx, tokenID)
	if err != nil {
		return nil, fmt.Errorf("looking up market (tick size / neg-risk) for token %s: %w", tokenID, err)
	}
	tickSize, _ := strconv.ParseFloat(book.TickSize, 64)

	exchange := clob.StandardExchangeAddress
	if book.NegRisk {
		exchange = clob.NegRiskExchangeAddress
	}

	signed, err := clob.BuildAndSign(s.c.Wallet, s.c.ChainID, s.maker(), s.c.SignatureType, clob.OrderArgs{
		TokenID:         tokenID,
		Price:           price,
		Size:            size,
		Side:            side,
		TickSize:        tickSize,
		ExchangeAddress: exchange,
	}, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}

	if orderType == "" {
		orderType = "GTC"
	}
	body := map[string]any{
		"order":     signed,
		"owner":     s.c.Auth.APIKey(),
		"orderType": orderType,
	}

	var out PlaceOrderResult
	if err := s.c.CLOB.PostJSON(ctx, "/order", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelOrder cancels a single order by its ID (order hash).
func (s *service) CancelOrder(ctx context.Context, orderID string) (*CancelResult, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	var out CancelResult
	if err := s.c.CLOB.DeleteJSON(ctx, "/order", nil, map[string]string{"orderID": orderID}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelAllOrders cancels every open order for the authenticated user.
func (s *service) CancelAllOrders(ctx context.Context) (*CancelResult, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	var out CancelResult
	if err := s.c.CLOB.Delete(ctx, "/cancel-all", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListOpenOrders returns the authenticated user's open orders, optionally
// filtered by market (condition ID) or asset (outcome token ID).
func (s *service) ListOpenOrders(ctx context.Context, market, assetID string) ([]Order, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	q := url.Values{}
	if market != "" {
		q.Set("market", market)
	}
	if assetID != "" {
		q.Set("asset_id", assetID)
	}
	var out struct {
		Data []Order `json:"data"`
	}
	if err := s.c.CLOB.GetJSON(ctx, "/data/orders", q, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// ListTrades returns the authenticated user's trade history, optionally
// filtered by market (condition ID) or asset (outcome token ID).
func (s *service) ListTrades(ctx context.Context, market, assetID string) ([]Trade, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("maker_address", s.maker())
	if market != "" {
		q.Set("market", market)
	}
	if assetID != "" {
		q.Set("asset_id", assetID)
	}
	var out struct {
		Data []Trade `json:"data"`
	}
	if err := s.c.CLOB.GetJSON(ctx, "/data/trades", q, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// GetBalance returns the collateral (assetType="COLLATERAL", the default) or
// conditional-token (assetType="CONDITIONAL", requires tokenID) balance and
// exchange allowances for the authenticated user's address.
func (s *service) GetBalance(ctx context.Context, assetType, tokenID string) (*Balance, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	if assetType == "" {
		assetType = "COLLATERAL"
	}
	q := url.Values{}
	q.Set("asset_type", assetType)
	if tokenID != "" {
		q.Set("token_id", tokenID)
	}
	q.Set("signature_type", strconv.Itoa(s.c.SignatureType))

	var out Balance
	if err := s.c.CLOB.GetJSON(ctx, "/balance-allowance", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
