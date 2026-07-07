// SPDX-License-Identifier: MIT

package client

import (
	"net/http"
	"testing"
)

func TestBearerAuthorizerSetsHeader(t *testing.T) {
	a := NewBearerAuthorizer("secret-token")

	req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	a.Authorize(req)

	if got := req.Header.Get("Authorization"); got != "Bearer secret-token" {
		t.Errorf("Authorization header = %q, want %q", got, "Bearer secret-token")
	}
}
