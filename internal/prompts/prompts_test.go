// SPDX-License-Identifier: MIT

package prompts

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rangertaha/polymarket-mcp/internal/server"
)

func TestRegisterAddsMarketOddsPrompt(t *testing.T) {
	s := server.New("test", "0.0.0", false)
	Register(s)

	if s.PromptCount() != 1 {
		t.Fatalf("PromptCount() = %d, want 1", s.PromptCount())
	}
}

// TestMarketOddsPromptText drives the registered prompt through an in-memory
// MCP client session (the same path a real client uses) since the render
// closure passed to AddPrompt isn't otherwise exported for direct inspection.
func TestMarketOddsPromptText(t *testing.T) {
	s := server.New("test", "0.0.0", false)
	Register(s)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := s.Connect(ctx, serverTransport); err != nil {
		t.Fatalf("Server.Connect() error = %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer func() { _ = session.Close() }()

	list, err := session.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("ListPrompts() error = %v", err)
	}
	if len(list.Prompts) != 1 || list.Prompts[0].Name != "market_odds" {
		t.Fatalf("ListPrompts() = %v, want [market_odds]", list.Prompts)
	}

	got, err := session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name:      "market_odds",
		Arguments: map[string]string{"id": "540817"},
	})
	if err != nil {
		t.Fatalf("GetPrompt() error = %v", err)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(got.Messages))
	}
	text, ok := got.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content type = %T, want *mcp.TextContent", got.Messages[0].Content)
	}
	if !strings.Contains(text.Text, "540817") {
		t.Errorf("rendered prompt = %q, want it to mention market id 540817", text.Text)
	}
	if !strings.Contains(text.Text, "markets_get") {
		t.Errorf("rendered prompt = %q, want it to reference the markets_get tool", text.Text)
	}
}
