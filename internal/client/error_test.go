// SPDX-License-Identifier: MIT

package client

import (
	"strings"
	"testing"
)

func TestAPIErrorErrorMessage(t *testing.T) {
	cases := []struct {
		name string
		err  *APIError
		want string
	}{
		{
			"message set",
			&APIError{Method: "GET", URL: "/markets", StatusCode: 400, Message: "bad request", Body: `{"message":"bad request"}`},
			"GET /markets -> HTTP 400: bad request",
		},
		{
			"message empty, body falls back",
			&APIError{Method: "GET", URL: "/markets", StatusCode: 500, Body: "internal error"},
			"GET /markets -> HTTP 500: internal error",
		},
		{
			"message and body both empty",
			&APIError{Method: "GET", URL: "/markets", StatusCode: 503},
			"GET /markets -> HTTP 503: (no response body)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.err.Error(); got != c.want {
				t.Errorf("Error() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestTruncateShortStringUnchanged(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate() = %q, want %q", got, "short")
	}
}

func TestTruncateAppendsEllipsisWhenCut(t *testing.T) {
	got := truncate("this is a long string", 4)
	if !strings.HasPrefix(got, "this") {
		t.Errorf("truncate() = %q, want prefix %q", got, "this")
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncate() = %q, want ellipsis suffix", got)
	}
}
