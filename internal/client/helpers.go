// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"net/http"
	"net/url"
)

// GetJSON performs a GET request and decodes the response into out.
func (c *Client) GetJSON(ctx context.Context, path string, query url.Values, out any) error {
	_, err := c.Do(ctx, Request{Method: http.MethodGet, Path: path, Query: query, Out: out})
	return err
}

// PostJSON performs a POST request with a JSON body and decodes the response.
func (c *Client) PostJSON(ctx context.Context, path string, query url.Values, body, out any) error {
	_, err := c.Do(ctx, Request{Method: http.MethodPost, Path: path, Query: query, Body: body, Out: out})
	return err
}

// PutJSON performs a PUT request with a JSON body and decodes the response.
func (c *Client) PutJSON(ctx context.Context, path string, query url.Values, body, out any) error {
	_, err := c.Do(ctx, Request{Method: http.MethodPut, Path: path, Query: query, Body: body, Out: out})
	return err
}

// PatchJSON performs a PATCH request with a JSON body and decodes the response.
func (c *Client) PatchJSON(ctx context.Context, path string, query url.Values, body, out any) error {
	_, err := c.Do(ctx, Request{Method: http.MethodPatch, Path: path, Query: query, Body: body, Out: out})
	return err
}

// Delete performs a DELETE request, optionally decoding a response body.
func (c *Client) Delete(ctx context.Context, path string, query url.Values, out any) error {
	_, err := c.Do(ctx, Request{Method: http.MethodDelete, Path: path, Query: query, Out: out})
	return err
}
