// SPDX-License-Identifier: MIT

package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// APIError is a structured error returned for any non-2xx HTTP response. It
// preserves the HTTP status and, when available, the service-provided message
// so callers (and ultimately the LLM) get an actionable explanation rather than
// an opaque status code.
type APIError struct {
	Method     string // HTTP method of the failed request
	URL        string // request URL (path + query)
	StatusCode int    // HTTP status code
	Message    string // human-readable message extracted from the body
	Body       string // raw response body (truncated)
}

// Error implements the error interface.
func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = e.Body
	}
	if msg == "" {
		msg = "(no response body)"
	}
	return fmt.Sprintf("%s %s -> HTTP %d: %s", e.Method, e.URL, e.StatusCode, msg)
}

// errorBody covers the common JSON error envelopes seen across REST APIs:
//
//	{"message": "..."}
//	{"error": "..."}
//	{"error": {"message": "..."}}
type errorBody struct {
	Message string          `json:"message"`
	Error   json.RawMessage `json:"error"`
}

// parseAPIError builds an *APIError from a failed response, best-effort decoding
// a JSON error envelope.
func parseAPIError(method string, u *url.URL, status int, body []byte) *APIError {
	e := &APIError{
		Method:     method,
		URL:        u.RequestURI(),
		StatusCode: status,
		Body:       truncate(strings.TrimSpace(string(body)), 2000),
	}
	var env errorBody
	if json.Unmarshal(body, &env) == nil {
		switch {
		case env.Message != "":
			e.Message = env.Message
		case len(env.Error) > 0:
			// error may be a string or an object with a message field.
			var s string
			if json.Unmarshal(env.Error, &s) == nil && s != "" {
				e.Message = s
			} else {
				var inner struct {
					Message string `json:"message"`
				}
				if json.Unmarshal(env.Error, &inner) == nil {
					e.Message = inner.Message
				}
			}
		}
	}
	return e
}

// truncate shortens s to at most n bytes, appending an ellipsis when cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
