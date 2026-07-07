// SPDX-License-Identifier: MIT

package trading

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rangertaha/polymarket-mcp/internal/client"
	"github.com/rangertaha/polymarket-mcp/internal/clob"
	"github.com/rangertaha/polymarket-mcp/internal/polymarket"
	"github.com/rangertaha/polymarket-mcp/internal/server"
)

// connectClient wires s to a fresh in-process MCP client over an in-memory
// transport, so tool calls exercise Register's actual tools/list and
// tools/call wiring rather than the service methods directly.
func connectClient(t *testing.T, s *server.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	if _, err := s.Connect(ctx, serverTransport); err != nil {
		t.Fatalf("Server.Connect() error = %v", err)
	}
	c := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := c.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// newTradingTestServer registers the trading toolset (with a funded wallet)
// against a mock CLOB server and connects an in-process MCP client to it.
func newTradingTestServer(t *testing.T, readOnly bool, mux http.Handler) (*server.Server, *mcp.ClientSession) {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	w, err := clob.NewWallet(testPrivateKey)
	if err != nil {
		t.Fatalf("NewWallet() error = %v", err)
	}

	auth := clob.NewAuthorizer(w, srv.URL, 137, srv.Client())
	clobClient, err := client.New(srv.URL, auth, client.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}

	c := &polymarket.Clients{
		CLOB:    clobClient,
		Auth:    auth,
		Wallet:  w,
		ChainID: 137,
	}

	s := server.New("test", "0.0.0", readOnly)
	Register(s, c)
	return s, connectClient(t, s)
}

// structuredContent unmarshals a tool call's structured output into out.
func structuredContent(t *testing.T, res *mcp.CallToolResult, out any) {
	t.Helper()
	// The in-memory transport used by connectClient hands back the decoded
	// value as-is (e.g. map[string]any) rather than raw JSON bytes, so
	// round-trip through JSON to decode into a concrete type either way.
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshaling structured content: %v", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("unmarshaling structured content: %v", err)
	}
}

func TestRegisterTradingToolsSkippedWithoutWallet(t *testing.T) {
	s := server.New("test", "0.0.0", false)
	Register(s, &polymarket.Clients{})

	if s.ToolCount() != 0 {
		t.Fatalf("ToolCount() = %d, want 0 (no wallet configured)", s.ToolCount())
	}
	if len(s.Toolsets()) != 0 {
		t.Fatalf("Toolsets() = %v, want empty (no wallet configured)", s.Toolsets())
	}
}

func TestRegisterTradingTools(t *testing.T) {
	s, session := newTradingTestServer(t, false, mockCLOBMux(t, nil))

	const wantTools = 9
	if s.ToolCount() != wantTools {
		t.Fatalf("ToolCount() = %d, want %d", s.ToolCount(), wantTools)
	}
	if got := s.Toolsets(); len(got) != 1 || got[0] != Name {
		t.Errorf("Toolsets() = %v, want [%s]", got, Name)
	}

	list, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{
		"trading_get_order_book", "trading_get_price", "trading_get_midpoint",
		"trading_get_balance", "trading_list_open_orders", "trading_list_trades",
		"trading_place_order", "trading_cancel_order", "trading_cancel_all_orders",
	} {
		if !names[want] {
			t.Errorf("ListTools() missing tool %q", want)
		}
	}
}

func TestRegisterTradingToolsSkipsWriteToolsInReadOnlyMode(t *testing.T) {
	s, session := newTradingTestServer(t, true, mockCLOBMux(t, nil))

	const wantTools = 6 // the 3 write tools (place/cancel/cancel-all) are skipped
	if s.ToolCount() != wantTools {
		t.Fatalf("ToolCount() = %d, want %d (write tools skipped in read-only mode)", s.ToolCount(), wantTools)
	}

	list, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	for _, tool := range list.Tools {
		for _, write := range []string{"trading_place_order", "trading_cancel_order", "trading_cancel_all_orders"} {
			if tool.Name == write {
				t.Errorf("ListTools() unexpectedly includes write tool %q in read-only mode", write)
			}
		}
	}
}

func TestTradingGetOrderBookToolCall(t *testing.T) {
	_, session := newTradingTestServer(t, false, mockCLOBMux(t, nil))

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "trading_get_order_book",
		Arguments: map[string]any{"tokenId": "1"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}

	var out OrderBook
	structuredContent(t, res, &out)
	if out.TickSize != "0.01" || len(out.Bids) != 1 || out.Bids[0].Price != "0.44" {
		t.Fatalf("out = %+v, unexpected", out)
	}
}

func TestTradingGetPriceToolCall(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/price", func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("side"); got != "BUY" {
				t.Errorf("side query = %q, want BUY", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"price":0.45}`))
		})
	})
	_, session := newTradingTestServer(t, false, mux)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "trading_get_price",
		Arguments: map[string]any{"tokenId": "1", "side": "BUY"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}

	var out PriceResult
	structuredContent(t, res, &out)
	if out.Price != 0.45 {
		t.Fatalf("out.Price = %v, want 0.45", out.Price)
	}
}

func TestTradingGetMidpointToolCall(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/midpoint", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"mid_price":"0.455"}`))
		})
	})
	_, session := newTradingTestServer(t, false, mux)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "trading_get_midpoint",
		Arguments: map[string]any{"tokenId": "1"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}

	var out MidpointResult
	structuredContent(t, res, &out)
	if out.MidPrice != "0.455" {
		t.Fatalf("out.MidPrice = %q, want 0.455", out.MidPrice)
	}
}

func TestTradingListOpenOrdersToolCall(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/data/orders", func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("market"); got != "0xcond" {
				t.Errorf("market query = %q, want 0xcond", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"0x1","status":"ORDER_STATUS_LIVE","side":"BUY"}]}`))
		})
	})
	_, session := newTradingTestServer(t, false, mux)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "trading_list_open_orders",
		Arguments: map[string]any{"market": "0xcond"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}

	var out server.ListResult[Order]
	structuredContent(t, res, &out)
	if out.Count != 1 || out.Items[0].ID != "0x1" {
		t.Fatalf("out = %+v, unexpected", out)
	}
}

func TestTradingListTradesToolCall(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/data/trades", func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("maker_address"); got == "" {
				t.Error("maker_address query param missing (required by the CLOB API)")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"trade-1","side":"SELL"}]}`))
		})
	})
	_, session := newTradingTestServer(t, false, mux)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "trading_list_trades"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}

	var out server.ListResult[Trade]
	structuredContent(t, res, &out)
	if out.Count != 1 || out.Items[0].ID != "trade-1" {
		t.Fatalf("out = %+v, unexpected", out)
	}
}

func TestTradingCancelAllOrdersToolCall(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/cancel-all", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("method = %s, want DELETE", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"canceled":["0xabc","0xdef"],"not_canceled":{}}`))
		})
	})
	_, session := newTradingTestServer(t, false, mux)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "trading_cancel_all_orders"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}

	var out CancelResult
	structuredContent(t, res, &out)
	if len(out.Canceled) != 2 {
		t.Fatalf("out = %+v, want 2 canceled", out)
	}
}

func TestTradingGetBalanceToolCall(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/balance-allowance", func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("asset_type"); got != "COLLATERAL" {
				t.Errorf("asset_type = %q, want COLLATERAL (default)", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"balance":"1000000","allowances":{"0xexchange":"1000000"}}`))
		})
	})
	_, session := newTradingTestServer(t, false, mux)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "trading_get_balance"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}

	var out Balance
	structuredContent(t, res, &out)
	if out.Balance != "1000000" {
		t.Fatalf("out.Balance = %q, want 1000000", out.Balance)
	}
}

func TestTradingPlaceOrderToolCall(t *testing.T) {
	var gotSide string
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Order struct {
					Side string `json:"side"`
				} `json:"order"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding order body: %v", err)
			}
			gotSide = body.Order.Side
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"orderID":"0xdeadbeef","status":"live"}`))
		})
	})
	_, session := newTradingTestServer(t, false, mux)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "trading_place_order",
		Arguments: map[string]any{
			"tokenId": "1", "side": "BUY", "price": 0.45, "size": 10,
		},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}
	if gotSide != "BUY" {
		t.Errorf("order side sent to CLOB = %q, want BUY", gotSide)
	}

	var out PlaceOrderResult
	structuredContent(t, res, &out)
	if !out.Success || out.OrderID != "0xdeadbeef" {
		t.Fatalf("out = %+v, unexpected", out)
	}
}

func TestTradingPlaceOrderToolCallGTDRequiresFutureExpiration(t *testing.T) {
	_, session := newTradingTestServer(t, false, mockCLOBMux(t, nil))

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "trading_place_order",
		Arguments: map[string]any{
			"tokenId": "1", "side": "BUY", "price": 0.45, "size": 10, "orderType": "GTD",
		},
	})
	if err != nil {
		t.Fatalf("CallTool() transport error = %v", err)
	}
	if !res.IsError {
		t.Fatalf("CallTool() IsError = false, want true (GTD without an expiration must be rejected)")
	}
}

func TestTradingPlaceOrderToolCallGTDWithExpiration(t *testing.T) {
	wantExpiration := time.Now().Add(24 * time.Hour).Unix()
	var gotExpiration, gotOrderType string
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Order struct {
					Expiration string `json:"expiration"`
				} `json:"order"`
				OrderType string `json:"orderType"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding order body: %v", err)
			}
			gotExpiration, gotOrderType = body.Order.Expiration, body.OrderType
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"orderID":"0xdeadbeef","status":"live"}`))
		})
	})
	_, session := newTradingTestServer(t, false, mux)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "trading_place_order",
		Arguments: map[string]any{
			"tokenId": "1", "side": "BUY", "price": 0.45, "size": 10,
			"orderType": "GTD", "expiration": wantExpiration,
		},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}
	if gotOrderType != "GTD" {
		t.Errorf("orderType sent to CLOB = %q, want GTD", gotOrderType)
	}
	if gotExpiration != strconv.FormatInt(wantExpiration, 10) {
		t.Errorf("expiration sent to CLOB = %q, want %d", gotExpiration, wantExpiration)
	}
}

func TestTradingCancelOrderToolCall(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("method = %s, want DELETE", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"canceled":["0xabc"],"not_canceled":{}}`))
		})
	})
	_, session := newTradingTestServer(t, false, mux)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "trading_cancel_order",
		Arguments: map[string]any{"orderId": "0xabc"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}

	var out CancelResult
	structuredContent(t, res, &out)
	if len(out.Canceled) != 1 || out.Canceled[0] != "0xabc" {
		t.Fatalf("out = %+v, unexpected", out)
	}
}

func TestTradingToolCallSurfacesCLOBError(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/price", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid token_id"}`))
		})
	})
	_, session := newTradingTestServer(t, false, mux)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "trading_get_price",
		Arguments: map[string]any{"tokenId": "not-a-token", "side": "BUY"},
	})
	if err != nil {
		t.Fatalf("CallTool() transport error = %v", err)
	}
	if !res.IsError {
		t.Fatalf("CallTool() IsError = false, want true (a CLOB 400 should surface as a tool error, not a protocol error)")
	}
}
