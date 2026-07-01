// SPDX-License-Identifier: MIT

package clob

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Authorizer signs CLOB requests with Polymarket's L2 HMAC scheme. Credentials
// are derived lazily (on first use) from the wallet's L1 signature rather than
// at server startup, so a misconfigured or unreachable CLOB endpoint only
// fails the trading calls that need it — the rest of the server (the public
// Gamma data API) keeps working regardless.
//
// Authorizer implements client.Authorizer.
type Authorizer struct {
	wallet  *Wallet
	baseURL string
	chainID int64
	hc      *http.Client

	mu    sync.RWMutex
	creds *Creds
}

// NewAuthorizer builds an Authorizer for the given wallet and CLOB base URL.
// Credentials are not derived until Ensure (or the first authorized request)
// is called.
func NewAuthorizer(w *Wallet, baseURL string, chainID int64, hc *http.Client) *Authorizer {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Authorizer{wallet: w, baseURL: baseURL, chainID: chainID, hc: hc}
}

// Ensure derives L2 API credentials if they have not been derived yet. It is
// safe to call before every authenticated request; subsequent calls are
// no-ops once credentials are cached.
func (a *Authorizer) Ensure(ctx context.Context) error {
	a.mu.RLock()
	ready := a.creds != nil
	a.mu.RUnlock()
	if ready {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.creds != nil { // lost the race to another caller
		return nil
	}
	creds, err := DeriveAPICreds(ctx, a.hc, a.baseURL, a.wallet, a.chainID)
	if err != nil {
		return err
	}
	a.creds = creds
	return nil
}

// APIKey returns the derived API key (the "owner" UUID the CLOB API expects
// in order payloads), or "" if Ensure has not yet succeeded.
func (a *Authorizer) APIKey() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.creds == nil {
		return ""
	}
	return a.creds.APIKey
}

// Authorize signs the request with the cached L2 credentials. Callers must
// have already succeeded at Ensure(ctx); Authorize does not perform network
// calls and silently leaves the request unsigned if credentials aren't ready
// (the CLOB API will then reject it with 401, surfaced as an APIError).
func (a *Authorizer) Authorize(r *http.Request) {
	a.mu.RLock()
	creds := a.creds
	a.mu.RUnlock()
	if creds == nil {
		return
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	var body string
	if r.GetBody != nil {
		if rc, err := r.GetBody(); err == nil {
			if b, err := io.ReadAll(rc); err == nil {
				body = string(b)
			}
			_ = rc.Close()
		}
	}

	sig := hmacSignature(creds.Secret, ts, r.Method, r.URL.Path, body)
	r.Header.Set("POLY_ADDRESS", a.wallet.Address.Hex())
	r.Header.Set("POLY_SIGNATURE", sig)
	r.Header.Set("POLY_TIMESTAMP", ts)
	r.Header.Set("POLY_API_KEY", creds.APIKey)
	r.Header.Set("POLY_PASSPHRASE", creds.Passphrase)
}

// hmacSignature computes Polymarket's L2 request signature: base64url-decode
// the API secret, HMAC-SHA256 the timestamp+method+path(+body), and
// base64url-encode the result.
func hmacSignature(secret, timestamp, method, requestPath, body string) string {
	key, err := base64.URLEncoding.DecodeString(secret)
	if err != nil {
		// Secrets are also seen padded/unpadded inconsistently; fall back to
		// raw bytes rather than failing the request outright.
		key = []byte(secret)
	}
	message := timestamp + method + requestPath + body
	mac := hmac.New(sha256.New, key)
	_, _ = fmt.Fprint(mac, message)
	return base64.URLEncoding.EncodeToString(mac.Sum(nil))
}
