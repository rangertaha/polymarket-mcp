// SPDX-License-Identifier: MIT

package trading

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rangertaha/polymarket-mcp/internal/client"
	"github.com/rangertaha/polymarket-mcp/internal/clob"
	"github.com/rangertaha/polymarket-mcp/internal/polymarket"
)

// testPrivateKey is a throwaway key; it holds no funds and appears nowhere else.
const testPrivateKey = "4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"

// newTestService wires a service against a mock CLOB server, so trading's
// request-shaping and response-parsing can be exercised without hitting the
// real network or a funded wallet.
func newTestService(t *testing.T, handler http.Handler) (*service, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
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
	return &service{c: c}, srv
}

// mockCLOBMux builds the standard mock CLOB API: L1 credential derivation
// plus a book fixture, so most tests only need to add the one endpoint they
// care about on top.
func mockCLOBMux(t *testing.T, extra func(mux *http.ServeMux)) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/derive-api-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiKey":"test-owner-uuid","secret":"dGVzdC1zZWNyZXQtYnl0ZXM=","passphrase":"test-pass"}`))
	})
	mux.HandleFunc("/book", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"market": "0xabc",
			"asset_id": "1",
			"bids": [{"price":"0.44","size":"100"}],
			"asks": [{"price":"0.46","size":"150"}],
			"min_order_size": "1",
			"tick_size": "0.01",
			"neg_risk": false
		}`))
	})
	if extra != nil {
		extra(mux)
	}
	return mux
}

func TestRequireCLOBWithoutWallet(t *testing.T) {
	svc := &service{c: &polymarket.Clients{}}
	ctx := t.Context()

	if _, err := svc.GetOrderBook(ctx, "1"); err != errNotConfigured {
		t.Errorf("GetOrderBook() error = %v, want errNotConfigured", err)
	}
	if _, err := svc.GetBalance(ctx, "COLLATERAL", ""); err != errNotConfigured {
		t.Errorf("GetBalance() error = %v, want errNotConfigured", err)
	}
	if _, err := svc.PlaceOrder(ctx, "1", clob.Buy, 0.5, 10, ""); err != errNotConfigured {
		t.Errorf("PlaceOrder() error = %v, want errNotConfigured", err)
	}
}

func TestGetOrderBook(t *testing.T) {
	svc, _ := newTestService(t, mockCLOBMux(t, nil))
	book, err := svc.GetOrderBook(t.Context(), "1")
	if err != nil {
		t.Fatalf("GetOrderBook() error = %v", err)
	}
	if book.TickSize != "0.01" || book.NegRisk {
		t.Errorf("book = %+v, unexpected", book)
	}
	if len(book.Bids) != 1 || book.Bids[0].Price != "0.44" {
		t.Errorf("book.Bids = %+v, unexpected", book.Bids)
	}
}

func TestGetPriceAndMidpoint(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/price", func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("side"); got != "BUY" {
				t.Errorf("side query = %q, want BUY", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"price":0.45}`))
		})
		mux.HandleFunc("/midpoint", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"mid_price":"0.455"}`))
		})
	})
	svc, _ := newTestService(t, mux)

	price, err := svc.GetPrice(t.Context(), "1", "BUY")
	if err != nil {
		t.Fatalf("GetPrice() error = %v", err)
	}
	if price != 0.45 {
		t.Errorf("price = %v, want 0.45", price)
	}

	mid, err := svc.GetMidpoint(t.Context(), "1")
	if err != nil {
		t.Fatalf("GetMidpoint() error = %v", err)
	}
	if mid != "0.455" {
		t.Errorf("mid = %q, want 0.455", mid)
	}
}

func TestPlaceOrderUsesOwnerAndDefaultMaker(t *testing.T) {
	var gotOwner, gotSide, gotOrderType string
	var gotMaker string
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Order struct {
					Maker string `json:"maker"`
					Side  string `json:"side"`
				} `json:"order"`
				Owner     string `json:"owner"`
				OrderType string `json:"orderType"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding order body: %v", err)
			}
			gotOwner, gotSide, gotOrderType, gotMaker = body.Owner, body.Order.Side, body.OrderType, body.Order.Maker

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"orderID":"0xdeadbeef","status":"live"}`))
		})
	})
	svc, _ := newTestService(t, mux)

	out, err := svc.PlaceOrder(t.Context(), "1", clob.Buy, 0.45, 10, "")
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if !out.Success || out.OrderID != "0xdeadbeef" || out.Status != "live" {
		t.Errorf("out = %+v, unexpected", out)
	}
	if gotOwner != "test-owner-uuid" {
		t.Errorf("owner = %q, want the derived API key", gotOwner)
	}
	if gotSide != "BUY" {
		t.Errorf("side = %q, want BUY", gotSide)
	}
	if gotOrderType != "GTC" {
		t.Errorf("orderType = %q, want GTC (default)", gotOrderType)
	}
	if gotMaker != svc.c.Wallet.Address.Hex() {
		t.Errorf("maker = %q, want wallet address %q (no funder configured)", gotMaker, svc.c.Wallet.Address.Hex())
	}
}

func TestCancelOrderAndCancelAll(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("method = %s, want DELETE", r.Method)
			}
			var body struct {
				OrderID string `json:"orderID"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.OrderID != "0xabc" {
				t.Errorf("orderID = %q, want 0xabc", body.OrderID)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"canceled":["0xabc"],"not_canceled":{}}`))
		})
		mux.HandleFunc("/cancel-all", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("method = %s, want DELETE", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"canceled":["0xabc","0xdef"],"not_canceled":{}}`))
		})
	})
	svc, _ := newTestService(t, mux)

	out, err := svc.CancelOrder(t.Context(), "0xabc")
	if err != nil {
		t.Fatalf("CancelOrder() error = %v", err)
	}
	if len(out.Canceled) != 1 || out.Canceled[0] != "0xabc" {
		t.Errorf("CancelOrder() out = %+v, unexpected", out)
	}

	all, err := svc.CancelAllOrders(t.Context())
	if err != nil {
		t.Fatalf("CancelAllOrders() error = %v", err)
	}
	if len(all.Canceled) != 2 {
		t.Errorf("CancelAllOrders() out = %+v, want 2 canceled", all)
	}
}

func TestListOpenOrdersAndTrades(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/data/orders", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"0x1","status":"ORDER_STATUS_LIVE","side":"BUY"}]}`))
		})
		mux.HandleFunc("/data/trades", func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("maker_address"); got == "" {
				t.Error("maker_address query param missing (required by the CLOB API)")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"trade-1","side":"SELL"}]}`))
		})
	})
	svc, _ := newTestService(t, mux)

	orders, err := svc.ListOpenOrders(t.Context(), "", "")
	if err != nil {
		t.Fatalf("ListOpenOrders() error = %v", err)
	}
	if len(orders) != 1 || orders[0].ID != "0x1" {
		t.Errorf("orders = %+v, unexpected", orders)
	}

	trades, err := svc.ListTrades(t.Context(), "", "")
	if err != nil {
		t.Fatalf("ListTrades() error = %v", err)
	}
	if len(trades) != 1 || trades[0].ID != "trade-1" {
		t.Errorf("trades = %+v, unexpected", trades)
	}
}

func TestGetBalanceDefaultsToCollateral(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/balance-allowance", func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("asset_type"); got != "COLLATERAL" {
				t.Errorf("asset_type = %q, want COLLATERAL (default)", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"balance":"1000000","allowances":{"0xexchange":"1000000"}}`))
		})
	})
	svc, _ := newTestService(t, mux)

	bal, err := svc.GetBalance(t.Context(), "", "")
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if bal.Balance != "1000000" {
		t.Errorf("bal.Balance = %q, want 1000000", bal.Balance)
	}
}
