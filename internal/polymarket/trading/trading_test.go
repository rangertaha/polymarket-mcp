// SPDX-License-Identifier: MIT

package trading

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
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

	if _, err := svc.GetOrderBook(ctx, "1"); !errors.Is(err, errNotConfigured) {
		t.Errorf("GetOrderBook() error = %v, want errNotConfigured", err)
	}
	if _, err := svc.GetBalance(ctx, "COLLATERAL", ""); !errors.Is(err, errNotConfigured) {
		t.Errorf("GetBalance() error = %v, want errNotConfigured", err)
	}
	if _, err := svc.PlaceOrder(ctx, "1", clob.Buy, 0.5, 10, "", 0); !errors.Is(err, errNotConfigured) {
		t.Errorf("PlaceOrder() error = %v, want errNotConfigured", err)
	}
	if _, err := svc.GetPrice(ctx, "1", "BUY"); !errors.Is(err, errNotConfigured) {
		t.Errorf("GetPrice() error = %v, want errNotConfigured", err)
	}
	if _, err := svc.GetMidpoint(ctx, "1"); !errors.Is(err, errNotConfigured) {
		t.Errorf("GetMidpoint() error = %v, want errNotConfigured", err)
	}
	if _, err := svc.CancelOrder(ctx, "0xabc"); !errors.Is(err, errNotConfigured) {
		t.Errorf("CancelOrder() error = %v, want errNotConfigured", err)
	}
	if _, err := svc.CancelAllOrders(ctx); !errors.Is(err, errNotConfigured) {
		t.Errorf("CancelAllOrders() error = %v, want errNotConfigured", err)
	}
	if _, err := svc.ListOpenOrders(ctx, "", ""); !errors.Is(err, errNotConfigured) {
		t.Errorf("ListOpenOrders() error = %v, want errNotConfigured", err)
	}
	if _, err := svc.ListTrades(ctx, "", ""); !errors.Is(err, errNotConfigured) {
		t.Errorf("ListTrades() error = %v, want errNotConfigured", err)
	}
}

// TestRequireAuthPropagatesDerivationError verifies that when a wallet is
// configured but the CLOB never yields L2 credentials, authenticated methods
// fail (rather than sending an unsigned request) instead of the earlier,
// narrower errNotConfigured check.
func TestRequireAuthPropagatesDerivationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	w, err := clob.NewWallet(testPrivateKey)
	if err != nil {
		t.Fatalf("NewWallet() error = %v", err)
	}
	auth := clob.NewAuthorizer(w, srv.URL, 137, srv.Client())
	clobClient, err := client.New(srv.URL, auth, client.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}
	svc := &service{c: &polymarket.Clients{CLOB: clobClient, Auth: auth, Wallet: w, ChainID: 137}}

	if _, err := svc.CancelOrder(t.Context(), "0xabc"); err == nil {
		t.Fatal("CancelOrder() expected error when CLOB auth can't be derived, got nil")
	}
}

// TestMakerUsesFunderWhenConfigured verifies that a configured Funder (a
// proxy wallet or Gnosis Safe holding trading funds) takes precedence over
// the signing wallet's own address when building/submitting an order.
func TestMakerUsesFunderWhenConfigured(t *testing.T) {
	var gotMaker string
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Order struct {
					Maker string `json:"maker"`
				} `json:"order"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding order body: %v", err)
			}
			gotMaker = body.Order.Maker
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"orderID":"0xdeadbeef","status":"live"}`))
		})
	})
	svc, _ := newTestService(t, mux)
	svc.c.Funder = "0xFEEDFACECAFEBEEFFEEDFACECAFEBEEFFEEDFACE"

	if _, err := svc.PlaceOrder(t.Context(), "1", clob.Buy, 0.45, 10, "", 0); err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if gotMaker != svc.c.Funder {
		t.Errorf("maker = %q, want configured funder %q", gotMaker, svc.c.Funder)
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

// TestGetOrderBookPropagatesAPIError does not use mockCLOBMux (which already
// registers /book) since GetOrderBook needs no auth and the test wants a
// failing /book handler instead.
func TestGetOrderBookPropagatesAPIError(t *testing.T) {
	svc, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid token_id"}`))
	}))

	if _, err := svc.GetOrderBook(t.Context(), "not-a-token"); err == nil {
		t.Fatal("GetOrderBook() expected error, got nil")
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

func TestGetMidpointPropagatesAPIError(t *testing.T) {
	svc, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid token_id"}`))
	}))

	if _, err := svc.GetMidpoint(t.Context(), "not-a-token"); err == nil {
		t.Fatal("GetMidpoint() expected error, got nil")
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

	out, err := svc.PlaceOrder(t.Context(), "1", clob.Buy, 0.45, 10, "", 0)
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

// clobOrderEIP712Types mirrors the CLOB V2 "Order" struct clob.BuildAndSign
// signs (internal/clob/order.go's unexported orderType), duplicated here so
// this test can independently recompute the EIP-712 digest and verify which
// exchange address an order was actually signed for.
var clobOrderEIP712Types = apitypes.Types{
	"Order": []apitypes.Type{
		{Name: "salt", Type: "uint256"},
		{Name: "maker", Type: "address"},
		{Name: "signer", Type: "address"},
		{Name: "tokenId", Type: "uint256"},
		{Name: "makerAmount", Type: "uint256"},
		{Name: "takerAmount", Type: "uint256"},
		{Name: "side", Type: "uint8"},
		{Name: "signatureType", Type: "uint8"},
		{Name: "timestamp", Type: "uint256"},
		{Name: "metadata", Type: "bytes32"},
		{Name: "builder", Type: "bytes32"},
	},
}

// signedOrderRecoversFor reports whether signed's signature recovers to the
// wallet address when hashed against the given verifying contract (exchange
// address) — the only way to check which exchange an order was signed for,
// since ExchangeAddress isn't itself part of the signed order's wire fields.
func signedOrderRecoversFor(t *testing.T, signed *clob.SignedOrder, chainID int64, exchange string) bool {
	t.Helper()
	domain := apitypes.TypedDataDomain{
		Name:              "Polymarket CTF Exchange",
		Version:           "2",
		ChainId:           math.NewHexOrDecimal256(chainID),
		VerifyingContract: exchange,
	}
	message := apitypes.TypedDataMessage{
		"salt":          signed.Salt,
		"maker":         signed.Maker,
		"signer":        signed.Signer,
		"tokenId":       signed.TokenID,
		"makerAmount":   signed.MakerAmount,
		"takerAmount":   signed.TakerAmount,
		"side":          map[clob.Side]string{clob.Buy: "0", clob.Sell: "1"}[signed.Side],
		"signatureType": strconv.Itoa(signed.SignatureType),
		"timestamp":     signed.Timestamp,
		"metadata":      signed.Metadata,
		"builder":       signed.Builder,
	}
	td := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": []apitypes.Type{
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Order": clobOrderEIP712Types["Order"],
		},
		PrimaryType: "Order",
		Domain:      domain,
		Message:     message,
	}
	digest, _, err := apitypes.TypedDataAndHash(td)
	if err != nil {
		t.Fatalf("TypedDataAndHash() error = %v", err)
	}
	sig, err := hexutil.Decode(signed.Signature)
	if err != nil {
		t.Fatalf("decoding signature: %v", err)
	}
	recoverSig := append([]byte{}, sig...)
	recoverSig[64] -= 27
	pub, err := crypto.SigToPub(digest, recoverSig)
	if err != nil {
		return false
	}
	w, err := clob.NewWallet(testPrivateKey)
	if err != nil {
		t.Fatalf("NewWallet() error = %v", err)
	}
	return crypto.PubkeyToAddress(*pub) == w.Address
}

// TestPlaceOrderUsesNegRiskExchange verifies a neg-risk market (per its order
// book) is signed against NegRiskExchangeAddress rather than the standard
// CTF Exchange, since submitting to the wrong exchange address produces a
// signature the CLOB's on-chain verifier for that market would reject.
func TestPlaceOrderUsesNegRiskExchange(t *testing.T) {
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
			"neg_risk": true
		}`))
	})
	var gotOrder clob.SignedOrder
	mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Order clob.SignedOrder `json:"order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding order body: %v", err)
		}
		gotOrder = body.Order
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"orderID":"0xdeadbeef","status":"live"}`))
	})
	svc, _ := newTestService(t, mux)

	if _, err := svc.PlaceOrder(t.Context(), "1", clob.Buy, 0.45, 10, "", 0); err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}

	if !signedOrderRecoversFor(t, &gotOrder, 137, clob.NegRiskExchangeAddress) {
		t.Error("order does not recover against NegRiskExchangeAddress, want it signed for the neg-risk exchange")
	}
	if signedOrderRecoversFor(t, &gotOrder, 137, clob.StandardExchangeAddress) {
		t.Error("order recovers against StandardExchangeAddress, want it signed only for the neg-risk exchange (book reported neg_risk=true)")
	}
}

func TestPlaceOrderPropagatesOrderBookLookupError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/derive-api-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiKey":"test-owner-uuid","secret":"dGVzdC1zZWNyZXQtYnl0ZXM=","passphrase":"test-pass"}`))
	})
	mux.HandleFunc("/book", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid token_id"}`))
	})
	svc, _ := newTestService(t, mux)

	if _, err := svc.PlaceOrder(t.Context(), "not-a-token", clob.Buy, 0.45, 10, "", 0); err == nil {
		t.Fatal("PlaceOrder() expected error when the order book lookup fails, got nil")
	}
}

func TestPlaceOrderRejectsInvalidPrice(t *testing.T) {
	svc, _ := newTestService(t, mockCLOBMux(t, nil))

	if _, err := svc.PlaceOrder(t.Context(), "1", clob.Buy, 2.0, 10, "", 0); err == nil {
		t.Fatal("PlaceOrder() expected error for an out-of-range price, got nil")
	}
}

func TestPlaceOrderPropagatesSubmitError(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"insufficient balance"}`))
		})
	})
	svc, _ := newTestService(t, mux)

	if _, err := svc.PlaceOrder(t.Context(), "1", clob.Buy, 0.45, 10, "", 0); err == nil {
		t.Fatal("PlaceOrder() expected error when order submission fails, got nil")
	}
}

// TestPlaceOrderRejectsGTDWithoutFutureExpiration guards against silently
// submitting a good-til-date order that can never actually expire: the CLOB
// signs whatever expiration it's given, so the client must refuse to send
// GTD orders whose expiration is zero or already in the past.
func TestPlaceOrderRejectsGTDWithoutFutureExpiration(t *testing.T) {
	svc, _ := newTestService(t, mockCLOBMux(t, nil))

	cases := []struct {
		name       string
		expiration int64
	}{
		{"zero expiration", 0},
		{"past expiration", time.Now().Add(-time.Hour).Unix()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := svc.PlaceOrder(t.Context(), "1", clob.Buy, 0.45, 10, "GTD", c.expiration); err == nil {
				t.Fatal("PlaceOrder() expected error for GTD without a future expiration, got nil")
			}
		})
	}
}

// TestPlaceOrderGTDValidationIsCaseInsensitive guards against a caller
// spelling the order type "gtd"/"Gtd" bypassing the future-expiration check
// (which would silently reintroduce the original expiration=0 bug for that
// casing) and verifies the order type sent to the CLOB is normalized to the
// canonical uppercase form.
func TestPlaceOrderGTDValidationIsCaseInsensitive(t *testing.T) {
	svc, _ := newTestService(t, mockCLOBMux(t, nil))
	if _, err := svc.PlaceOrder(t.Context(), "1", clob.Buy, 0.45, 10, "gtd", 0); err == nil {
		t.Fatal("PlaceOrder() expected error for lowercase gtd without a future expiration, got nil")
	}

	var gotOrderType string
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				OrderType string `json:"orderType"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding order body: %v", err)
			}
			gotOrderType = body.OrderType
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"orderID":"0xdeadbeef","status":"live"}`))
		})
	})
	svc2, _ := newTestService(t, mux)
	future := time.Now().Add(time.Hour).Unix()
	if _, err := svc2.PlaceOrder(t.Context(), "1", clob.Buy, 0.45, 10, "gtd", future); err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if gotOrderType != "GTD" {
		t.Errorf("orderType sent to CLOB = %q, want normalized GTD", gotOrderType)
	}
}

// TestPlaceOrderGTDSignsExpiration verifies a GTD order with a valid future
// expiration is actually signed with that expiration, rather than the
// hardcoded zero every other order type gets.
func TestPlaceOrderGTDSignsExpiration(t *testing.T) {
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
	svc, _ := newTestService(t, mux)

	if _, err := svc.PlaceOrder(t.Context(), "1", clob.Buy, 0.45, 10, "GTD", wantExpiration); err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if gotOrderType != "GTD" {
		t.Errorf("orderType = %q, want GTD", gotOrderType)
	}
	if gotExpiration != strconv.FormatInt(wantExpiration, 10) {
		t.Errorf("expiration = %q, want %d", gotExpiration, wantExpiration)
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

func TestCancelOrderPropagatesAPIError(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"unknown order"}`))
		})
	})
	svc, _ := newTestService(t, mux)

	if _, err := svc.CancelOrder(t.Context(), "0xabc"); err == nil {
		t.Fatal("CancelOrder() expected error, got nil")
	}
}

func TestCancelAllOrdersPropagatesAPIError(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/cancel-all", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
	})
	svc, _ := newTestService(t, mux)

	if _, err := svc.CancelAllOrders(t.Context()); err == nil {
		t.Fatal("CancelAllOrders() expected error, got nil")
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

func TestListOpenOrdersFiltersByAssetID(t *testing.T) {
	var gotAssetID string
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/data/orders", func(w http.ResponseWriter, r *http.Request) {
			gotAssetID = r.URL.Query().Get("asset_id")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[]}`))
		})
	})
	svc, _ := newTestService(t, mux)

	if _, err := svc.ListOpenOrders(t.Context(), "", "42"); err != nil {
		t.Fatalf("ListOpenOrders() error = %v", err)
	}
	if gotAssetID != "42" {
		t.Errorf("asset_id query = %q, want 42", gotAssetID)
	}
}

func TestListOpenOrdersPropagatesAPIError(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/data/orders", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
	})
	svc, _ := newTestService(t, mux)

	if _, err := svc.ListOpenOrders(t.Context(), "", ""); err == nil {
		t.Fatal("ListOpenOrders() expected error, got nil")
	}
}

func TestListTradesFiltersByMarketAndAssetID(t *testing.T) {
	var gotMarket, gotAssetID string
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/data/trades", func(w http.ResponseWriter, r *http.Request) {
			gotMarket = r.URL.Query().Get("market")
			gotAssetID = r.URL.Query().Get("asset_id")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[]}`))
		})
	})
	svc, _ := newTestService(t, mux)

	if _, err := svc.ListTrades(t.Context(), "0xcond", "42"); err != nil {
		t.Fatalf("ListTrades() error = %v", err)
	}
	if gotMarket != "0xcond" {
		t.Errorf("market query = %q, want 0xcond", gotMarket)
	}
	if gotAssetID != "42" {
		t.Errorf("asset_id query = %q, want 42", gotAssetID)
	}
}

func TestListTradesPropagatesAPIError(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/data/trades", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
	})
	svc, _ := newTestService(t, mux)

	if _, err := svc.ListTrades(t.Context(), "", ""); err == nil {
		t.Fatal("ListTrades() expected error, got nil")
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

func TestGetBalanceConditionalWithTokenID(t *testing.T) {
	var gotAssetType, gotTokenID string
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/balance-allowance", func(w http.ResponseWriter, r *http.Request) {
			gotAssetType = r.URL.Query().Get("asset_type")
			gotTokenID = r.URL.Query().Get("token_id")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"balance":"500","allowances":{}}`))
		})
	})
	svc, _ := newTestService(t, mux)

	bal, err := svc.GetBalance(t.Context(), "CONDITIONAL", "42")
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if gotAssetType != "CONDITIONAL" {
		t.Errorf("asset_type query = %q, want CONDITIONAL", gotAssetType)
	}
	if gotTokenID != "42" {
		t.Errorf("token_id query = %q, want 42", gotTokenID)
	}
	if bal.Balance != "500" {
		t.Errorf("bal.Balance = %q, want 500", bal.Balance)
	}
}

func TestGetBalancePropagatesAPIError(t *testing.T) {
	mux := mockCLOBMux(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/balance-allowance", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
	})
	svc, _ := newTestService(t, mux)

	if _, err := svc.GetBalance(t.Context(), "", ""); err == nil {
		t.Fatal("GetBalance() expected error, got nil")
	}
}
