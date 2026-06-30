// SPDX-License-Identifier: MIT

// Package prompts registers MCP prompts: user-invoked, parameterized templates
// that clients surface as slash commands. Each prompt encodes a multi-step
// workflow by guiding the model to call the right tools in order.
package prompts

import (
	"fmt"

	"github.com/rangertaha/polymarket-mcp/internal/server"
)

// Register adds the built-in workflow prompts to the server.
func Register(s *server.Server) {
	s.AddPrompt(
		"market_odds",
		"Read the implied odds for a Polymarket market and explain what the prices mean.",
		[]server.PromptArg{
			{Name: "id", Description: "market ID", Required: true},
		},
		func(a map[string]string) string {
			return fmt.Sprintf(`Explain the odds for Polymarket market %s.

Steps:
1. Call markets_get (id="%s") to load the market.
2. Parse the outcomes and outcomePrices (both JSON-encoded string arrays) and
   report each outcome with its price as an implied probability (price ~ 0-1).
3. Note the market's volume, liquidity, end date, and whether it is active or
   closed.`,
				a["id"], a["id"])
		},
	)
}
