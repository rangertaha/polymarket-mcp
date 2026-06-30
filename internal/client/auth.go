// SPDX-License-Identifier: MIT

package client

import "net/http"

// BearerAuthorizer authenticates using an OAuth-style bearer token. The public
// Gamma data API needs no auth; this is here for the authenticated CLOB
// endpoints added by later toolsets.
type BearerAuthorizer struct {
	header string
}

// NewBearerAuthorizer builds a BearerAuthorizer for the given token.
func NewBearerAuthorizer(token string) *BearerAuthorizer {
	return &BearerAuthorizer{header: "Bearer " + token}
}

// Authorize sets the Authorization header for bearer auth.
func (a *BearerAuthorizer) Authorize(r *http.Request) {
	r.Header.Set("Authorization", a.header)
}
