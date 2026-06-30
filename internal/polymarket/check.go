// SPDX-License-Identifier: MIT

package polymarket

import (
	"context"
	"net/url"
)

// Check verifies connectivity by requesting a single market. It returns the
// number of markets returned.
func Check(ctx context.Context, c *Clients) (int, error) {
	q := url.Values{}
	q.Set("limit", "1")
	var out []struct {
		ID string `json:"id"`
	}
	if err := c.Gamma.GetJSON(ctx, "/markets", q, &out); err != nil {
		return 0, err
	}
	return len(out), nil
}
