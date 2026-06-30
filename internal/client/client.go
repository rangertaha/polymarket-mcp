// SPDX-License-Identifier: MIT

// Package client provides a small, dependency-free HTTP client for talking to
// JSON REST APIs. It is the single transport layer shared by every service
// wrapper in this server.
//
// The client is intentionally generic: callers describe a request (method,
// path, query, body) and supply a destination for the decoded JSON response.
// Authentication is pluggable via the Authorizer interface.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultTimeout bounds a single HTTP request.
const defaultTimeout = 60 * time.Second

// Authorizer applies authentication credentials to an outgoing request.
type Authorizer interface {
	// Authorize mutates the request to carry authentication.
	Authorize(*http.Request)
}

// Client is a reusable JSON REST client bound to a base URL and an Authorizer.
// A Client is safe for concurrent use by multiple goroutines.
type Client struct {
	base      *url.URL
	http      *http.Client
	auth      Authorizer
	userAgent string
	header    http.Header // extra headers applied to every request
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets a custom *http.Client (e.g. for testing or proxies).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithUserAgent sets the User-Agent header sent on every request.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// WithHeader adds a header applied to every request (e.g. an API version pin).
func WithHeader(key, value string) Option {
	return func(c *Client) {
		if c.header == nil {
			c.header = http.Header{}
		}
		c.header.Set(key, value)
	}
}

// New creates a Client for the given base URL and Authorizer.
func New(baseURL string, auth Authorizer, opts ...Option) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL %q: %w", baseURL, err)
	}
	c := &Client{
		base:      u,
		http:      &http.Client{Timeout: defaultTimeout},
		auth:      auth,
		userAgent: "mcp-client",
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// Request describes a single REST call.
type Request struct {
	// Method is the HTTP method (GET, POST, PATCH, PUT, DELETE).
	Method string
	// Path is appended to the client's base URL. A leading slash is optional.
	Path string
	// Query holds URL query parameters.
	Query url.Values
	// Body, when non-nil, is JSON-encoded and sent as the request body.
	// If Body already implements io.Reader it is sent verbatim.
	Body any
	// ContentType overrides the request Content-Type. Defaults to
	// "application/json" when a body is present.
	ContentType string
	// Header holds extra request headers (applied after defaults, so they win).
	Header http.Header
	// Out, when non-nil, receives the JSON-decoded response body.
	Out any
}

// Response carries metadata about a completed request.
type Response struct {
	StatusCode int
	Header     http.Header
}

// Do executes a request, decoding a successful JSON response into req.Out (when
// set) and returning a typed *APIError for non-2xx responses.
func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	httpReq, err := c.buildRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", req.Method, httpReq.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	out := &Response{StatusCode: resp.StatusCode, Header: resp.Header}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, parseAPIError(req.Method, httpReq.URL, resp.StatusCode, body)
	}

	// Raw (non-JSON) responses are captured verbatim.
	if rb, ok := req.Out.(*RawBody); ok {
		rb.Bytes = body
		rb.ContentType = resp.Header.Get("Content-Type")
		return out, nil
	}
	if req.Out != nil && len(body) > 0 {
		if err := json.Unmarshal(body, req.Out); err != nil {
			return out, fmt.Errorf("decoding %s %s response: %w", req.Method, httpReq.URL.Path, err)
		}
	}
	return out, nil
}

// RawBody captures an undecoded response body. Pass a *RawBody as Request.Out to
// receive the raw bytes (and content type) instead of JSON decoding.
type RawBody struct {
	Bytes       []byte
	ContentType string
}

// String returns the raw body as a string.
func (r *RawBody) String() string { return string(r.Bytes) }

// buildRequest assembles an *http.Request from a Request.
func (c *Client) buildRequest(ctx context.Context, req Request) (*http.Request, error) {
	u := *c.base
	u.Path = joinPath(c.base.Path, req.Path)

	if len(req.Query) > 0 {
		q := url.Values{}
		for k, v := range req.Query {
			q[k] = v
		}
		u.RawQuery = q.Encode()
	}

	body, contentType, err := encodeBody(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	if c.userAgent != "" {
		httpReq.Header.Set("User-Agent", c.userAgent)
	}
	for k, vs := range c.header {
		for _, v := range vs {
			httpReq.Header.Set(k, v)
		}
	}
	if c.auth != nil {
		c.auth.Authorize(httpReq)
	}
	// Caller-supplied headers take precedence over defaults.
	for k, vs := range req.Header {
		httpReq.Header.Del(k)
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}
	return httpReq, nil
}

// encodeBody turns Request.Body into an io.Reader and resolves the Content-Type.
func encodeBody(req Request) (io.Reader, string, error) {
	if req.Body == nil {
		return nil, "", nil
	}
	if r, ok := req.Body.(io.Reader); ok {
		ct := req.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		return r, ct, nil
	}
	data, err := json.Marshal(req.Body)
	if err != nil {
		return nil, "", fmt.Errorf("encoding request body: %w", err)
	}
	ct := req.ContentType
	if ct == "" {
		ct = "application/json"
	}
	return bytes.NewReader(data), ct, nil
}

// joinPath joins a base path and a relative path with exactly one separator.
func joinPath(base, rel string) string {
	base = strings.TrimRight(base, "/")
	rel = strings.TrimLeft(rel, "/")
	switch {
	case base == "" && rel == "":
		return "/"
	case rel == "":
		return base
	case base == "":
		return "/" + rel
	default:
		return base + "/" + rel
	}
}
