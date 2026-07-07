// SPDX-License-Identifier: MIT

package markets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rangertaha/polymarket-mcp/internal/client"
	"github.com/rangertaha/polymarket-mcp/internal/polymarket"
	"github.com/rangertaha/polymarket-mcp/internal/server"
)

// connectClient wires s to a fresh in-process MCP client over an in-memory
// transport, so tool calls exercise Register's actual tools/list and
// tools/call wiring rather than the service methods directly.
func connectClient(t *testing.T, s *server.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	if _, err := s.Connect(ctx, serverTransport); err != nil {
		t.Fatalf("Server.Connect() error = %v", err)
	}
	c := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := c.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// newTestServer registers the markets toolset against a mock Gamma server and
// connects an in-process MCP client to it.
func newTestServer(t *testing.T, handler http.Handler) (*server.Server, *mcp.ClientSession) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	gamma, err := client.New(srv.URL, nil, client.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}

	s := server.New("test", "0.0.0", false)
	Register(s, &polymarket.Clients{Gamma: gamma})
	return s, connectClient(t, s)
}

// structuredContent unmarshals a tool call's structured output into out.
func structuredContent(t *testing.T, res *mcp.CallToolResult, out any) {
	t.Helper()
	// The in-memory transport used by connectClient hands back the decoded
	// value as-is (e.g. map[string]any) rather than raw JSON bytes, so
	// round-trip through JSON to decode into a concrete type either way.
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshaling structured content: %v", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("unmarshaling structured content: %v", err)
	}
}

func TestRegisterMarketsTools(t *testing.T) {
	s, session := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))

	if s.ToolCount() != 2 {
		t.Fatalf("ToolCount() = %d, want 2", s.ToolCount())
	}
	if got := s.Toolsets(); len(got) != 1 || got[0] != Name {
		t.Errorf("Toolsets() = %v, want [%s]", got, Name)
	}

	list, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"markets_list", "markets_get"} {
		if !names[want] {
			t.Errorf("ListTools() missing tool %q", want)
		}
	}
}

func TestMarketsListToolCall(t *testing.T) {
	var gotQuery url.Values
	_, session := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/markets" {
			t.Errorf("path = %q, want /markets", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"1","question":"Will it rain?","active":true,"closed":false}]`))
	}))

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "markets_list",
		Arguments: map[string]any{"active": true, "limit": 5},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}
	if gotQuery.Get("active") != "true" {
		t.Errorf("active query = %q, want true", gotQuery.Get("active"))
	}
	if gotQuery.Get("limit") != "5" {
		t.Errorf("limit query = %q, want 5", gotQuery.Get("limit"))
	}

	var out server.ListResult[Market]
	structuredContent(t, res, &out)
	if out.Count != 1 || len(out.Items) != 1 || out.Items[0].Question != "Will it rain?" {
		t.Fatalf("out = %+v, unexpected", out)
	}
}

func TestMarketsListToolCallEmpty(t *testing.T) {
	_, session := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "markets_list"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}

	var out server.ListResult[Market]
	structuredContent(t, res, &out)
	if out.Count != 0 || out.Items == nil {
		t.Errorf("out = %+v, want Count=0 and non-nil empty Items", out)
	}
}

func TestMarketsGetToolCall(t *testing.T) {
	_, session := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/markets/42" {
			t.Errorf("path = %q, want /markets/42", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"42","question":"Will it snow?","active":true,"closed":false,"outcomePrices":"[\"0.5\",\"0.5\"]"}`))
	}))

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "markets_get",
		Arguments: map[string]any{"id": "42"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}

	var out Market
	structuredContent(t, res, &out)
	if out.ID != "42" || out.Question != "Will it snow?" {
		t.Fatalf("out = %+v, unexpected", out)
	}
}

func TestMarketsGetToolCallSurfacesAPIError(t *testing.T) {
	_, session := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"market not found"}`))
	}))

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "markets_get",
		Arguments: map[string]any{"id": "does-not-exist"},
	})
	if err != nil {
		t.Fatalf("CallTool() transport error = %v", err)
	}
	if !res.IsError {
		t.Fatalf("CallTool() IsError = false, want true (a Gamma 404 should surface as a tool error, not a protocol error)")
	}
}
