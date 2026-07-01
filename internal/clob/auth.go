// SPDX-License-Identifier: MIT

package clob

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// clobAuthMessage is the fixed literal Polymarket's CLOB expects in the L1
// authentication signature.
const clobAuthMessage = "This message attests that I control the given wallet"

var clobAuthTypes = apitypes.Types{
	"ClobAuth": []apitypes.Type{
		{Name: "address", Type: "address"},
		{Name: "timestamp", Type: "string"},
		{Name: "nonce", Type: "uint256"},
		{Name: "message", Type: "string"},
	},
}

// Creds are the L2 API credentials derived from a one-time L1 (wallet-signed)
// authentication call. They authenticate subsequent CLOB requests via HMAC
// (see Authorizer).
type Creds struct {
	APIKey     string `json:"apiKey"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

// l1Headers builds the POLY_ADDRESS/POLY_SIGNATURE/POLY_TIMESTAMP/POLY_NONCE
// headers for a single L1-authenticated request (wallet-signed, no API key
// required yet).
func l1Headers(w *Wallet, chainID int64) (http.Header, error) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	domain := apitypes.TypedDataDomain{
		Name:    "ClobAuthDomain",
		Version: "1",
		ChainId: chainIDDomain(chainID),
	}
	message := apitypes.TypedDataMessage{
		"address":   w.Address.Hex(),
		"timestamp": ts,
		"nonce":     "0",
		"message":   clobAuthMessage,
	}
	sig, err := signTypedData(w, domain, "ClobAuth", clobAuthTypes, message)
	if err != nil {
		return nil, fmt.Errorf("signing L1 auth: %w", err)
	}
	h := http.Header{}
	h.Set("POLY_ADDRESS", w.Address.Hex())
	h.Set("POLY_SIGNATURE", sig)
	h.Set("POLY_TIMESTAMP", ts)
	h.Set("POLY_NONCE", "0")
	return h, nil
}

// DeriveAPICreds obtains L2 trading credentials for the given wallet: it first
// tries to derive the credentials already associated with the wallet, and
// falls back to creating a new API key if none exist yet. Both calls use a
// fresh L1 (wallet-signed) authentication header.
func DeriveAPICreds(ctx context.Context, hc *http.Client, baseURL string, w *Wallet, chainID int64) (*Creds, error) {
	if creds, err := requestCreds(ctx, hc, http.MethodGet, baseURL+"/auth/derive-api-key", w, chainID); err == nil {
		return creds, nil
	}
	creds, err := requestCreds(ctx, hc, http.MethodPost, baseURL+"/auth/api-key", w, chainID)
	if err != nil {
		return nil, fmt.Errorf("deriving CLOB API credentials: %w", err)
	}
	return creds, nil
}

// requestCreds issues a single L1-authenticated request expected to return a
// JSON body shaped like Creds.
func requestCreds(ctx context.Context, hc *http.Client, method, url string, w *Wallet, chainID int64) (*Creds, error) {
	headers, err := l1Headers(w, chainID)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header = headers
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s -> HTTP %d: %s", method, url, resp.StatusCode, bytes.TrimSpace(body))
	}
	var creds Creds
	if err := json.Unmarshal(body, &creds); err != nil {
		return nil, fmt.Errorf("decoding %s %s response: %w", method, url, err)
	}
	if creds.APIKey == "" || creds.Secret == "" {
		return nil, fmt.Errorf("%s %s: response missing API credentials", method, url)
	}
	return &creds, nil
}
