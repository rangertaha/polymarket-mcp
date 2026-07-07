// SPDX-License-Identifier: MIT

package server

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectClient wires s to a fresh in-process MCP client over an in-memory
// transport, so tests can exercise Register/AddPrompt through the same
// tools/list, tools/call, and prompts/list paths a real MCP client uses.
func connectClient(t *testing.T, s *Server) *mcp.ClientSession {
	t.Helper()
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
	t.Cleanup(func() { _ = session.Close() })
	return session
}

type echoInput struct {
	Msg      string `json:"msg,omitempty"`
	Anything any    `json:"anything,omitempty"`
}

type echoOutput struct {
	Msg string `json:"msg"`
}

func echoHandler(_ context.Context, _ *mcp.CallToolRequest, in echoInput) (*mcp.CallToolResult, echoOutput, error) {
	return nil, echoOutput{Msg: in.Msg}, nil
}

func TestRegisterSkipsWriteToolsInReadOnlyMode(t *testing.T) {
	s := New("test", "0.0.0", true)

	Register(s, ToolDef{Name: "read_thing", Description: "reads"}, echoHandler)
	Register(s, ToolDef{Name: "write_thing", Description: "writes", Write: true, Destructive: true}, echoHandler)

	if s.ToolCount() != 1 {
		t.Fatalf("ToolCount() = %d, want 1 (write tool must be skipped in read-only mode)", s.ToolCount())
	}

	session := connectClient(t, s)
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(list.Tools) != 1 || list.Tools[0].Name != "read_thing" {
		t.Fatalf("ListTools() = %v, want only read_thing", list.Tools)
	}
}

func TestRegisterAnnotations(t *testing.T) {
	s := New("test", "0.0.0", false)
	Register(s, ToolDef{Name: "read_thing", Description: "reads"}, echoHandler)
	Register(s, ToolDef{Name: "write_thing", Description: "writes", Write: true, Destructive: true}, echoHandler)

	if s.ToolCount() != 2 {
		t.Fatalf("ToolCount() = %d, want 2 (not read-only)", s.ToolCount())
	}

	session := connectClient(t, s)
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tool := range list.Tools {
		byName[tool.Name] = tool
	}

	read := byName["read_thing"]
	if read == nil || read.Annotations == nil || !read.Annotations.ReadOnlyHint {
		t.Errorf("read_thing annotations = %+v, want ReadOnlyHint=true", read.Annotations)
	}
	if read.Annotations.DestructiveHint != nil {
		t.Errorf("read_thing DestructiveHint = %v, want nil (only meaningful for write tools)", *read.Annotations.DestructiveHint)
	}

	write := byName["write_thing"]
	if write == nil || write.Annotations == nil || write.Annotations.ReadOnlyHint {
		t.Errorf("write_thing annotations = %+v, want ReadOnlyHint=false", write.Annotations)
	}
	if write.Annotations.DestructiveHint == nil || !*write.Annotations.DestructiveHint {
		t.Errorf("write_thing DestructiveHint = %v, want true", write.Annotations.DestructiveHint)
	}
}

// TestRegisterNormalizesInterfaceSchema guards the fix noted in Register's
// doc comment: Go's schema inference emits a bare JSON boolean (`true`) for
// interface{} fields, which some MCP clients reject during tool-list
// validation. The schema must not contain any raw JSON boolean subschemas.
func TestRegisterNormalizesInterfaceSchema(t *testing.T) {
	s := New("test", "0.0.0", false)
	Register(s, ToolDef{Name: "echo", Description: "echoes"}, echoHandler)

	session := connectClient(t, s)
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(list.Tools) != 1 {
		t.Fatalf("len(Tools) = %d, want 1", len(list.Tools))
	}

	raw, err := json.Marshal(list.Tools[0].InputSchema)
	if err != nil {
		t.Fatalf("marshaling input schema: %v", err)
	}
	if containsRawBoolean(t, raw) {
		t.Errorf("input schema contains a raw JSON boolean subschema: %s", raw)
	}
}

// containsRawBoolean reports whether any object value in the schema tree is a
// bare JSON boolean rather than an object ({} / {"not": {}}).
func containsRawBoolean(t *testing.T, raw json.RawMessage) bool {
	t.Helper()
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Fatalf("unmarshaling schema: %v", err)
	}
	var walk func(v any) bool
	walk = func(v any) bool {
		switch n := v.(type) {
		case bool:
			return true
		case map[string]any:
			for _, child := range n {
				if walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range n {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(node)
}

// unsupportedSchemaType has a field kind jsonschema.ForType cannot represent,
// so normalizedSchema must fall back to nil (letting the SDK's own
// generation run) instead of panicking or emitting a broken schema.
type unsupportedSchemaType struct {
	C chan int `json:"c"`
}

func TestNormalizedSchemaFallsBackOnUnsupportedType(t *testing.T) {
	if got := normalizedSchema(reflect.TypeFor[unsupportedSchemaType]()); got != nil {
		t.Errorf("normalizedSchema() = %s, want nil for an unsupported field type", got)
	}
}

func TestListResult(t *testing.T) {
	got := List([]int{1, 2, 3})
	if got.Count != 3 {
		t.Errorf("Count = %d, want 3", got.Count)
	}
	if len(got.Items) != 3 || got.Items[2] != 3 {
		t.Errorf("Items = %v, want [1 2 3]", got.Items)
	}

	empty := List([]int{})
	if empty.Count != 0 || empty.Items == nil {
		t.Errorf("List([]int{}) = %+v, want Count=0 and non-nil empty Items", empty)
	}
}

func TestAddPromptRendersArgs(t *testing.T) {
	s := New("test", "0.0.0", false)
	s.AddPrompt("greet", "Greets someone", []PromptArg{
		{Name: "name", Description: "who to greet", Required: true},
	}, func(args map[string]string) string {
		return "Hello, " + args["name"] + "!"
	})

	if s.PromptCount() != 1 {
		t.Fatalf("PromptCount() = %d, want 1", s.PromptCount())
	}

	session := connectClient(t, s)
	list, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListPrompts() error = %v", err)
	}
	if len(list.Prompts) != 1 || list.Prompts[0].Name != "greet" {
		t.Fatalf("ListPrompts() = %v, want [greet]", list.Prompts)
	}

	got, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "greet",
		Arguments: map[string]string{"name": "Ada"},
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
	if text.Text != "Hello, Ada!" {
		t.Errorf("rendered text = %q, want %q", text.Text, "Hello, Ada!")
	}
}

// TestAddPromptRejectsMissingRequiredArg guards against a required prompt
// argument being silently omitted: the MCP SDK declares
// PromptArgument.Required for clients' benefit but never enforces it, so
// AddPrompt's own handler must reject the call rather than rendering with an
// empty/missing value.
func TestAddPromptRejectsMissingRequiredArg(t *testing.T) {
	s := New("test", "0.0.0", false)
	s.AddPrompt("greet", "Greets someone", []PromptArg{
		{Name: "name", Description: "who to greet", Required: true},
	}, func(args map[string]string) string {
		return "Hello, " + args["name"] + "!"
	})

	session := connectClient(t, s)
	if _, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "greet"}); err == nil {
		t.Fatal("GetPrompt() expected error for missing required argument, got nil")
	}
}

// TestRunReturnsOnContextCancel drives Run with an already-canceled context,
// so it returns promptly instead of blocking for the life of the connection.
func TestRunReturnsOnContextCancel(t *testing.T) {
	s := New("test", "0.0.0", false)
	serverTransport, _ := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := s.Run(ctx, serverTransport); !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestNewServerDefaults(t *testing.T) {
	s := New("test", "0.0.0", false)
	if s.ReadOnly() {
		t.Error("ReadOnly() = true, want false")
	}
	if s.ToolCount() != 0 || s.PromptCount() != 0 {
		t.Errorf("fresh server should have zero tools/prompts, got %d/%d", s.ToolCount(), s.PromptCount())
	}
	if len(s.Toolsets()) != 0 {
		t.Errorf("Toolsets() = %v, want empty before NoteToolset", s.Toolsets())
	}
	s.NoteToolset("markets")
	s.NoteToolset("trading")
	if got := s.Toolsets(); len(got) != 2 || got[0] != "markets" || got[1] != "trading" {
		t.Errorf("Toolsets() = %v, want [markets trading] in registration order", got)
	}
}
