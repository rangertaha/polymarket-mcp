// SPDX-License-Identifier: MIT

package main

import "testing"

// These are structural checks on the CLI command tree, not behavioral tests:
// mcpCommand's and testCommand's Action bodies serve stdio and hit the real
// Gamma/CLOB APIs respectively, which would need dependency injection to
// exercise safely in a unit test. This at least catches wiring regressions
// (a renamed subcommand, a dropped Action) at build/test time.

func TestMCPCommand(t *testing.T) {
	cmd := mcpCommand()
	if cmd.Name != "mcp" {
		t.Errorf("Name = %q, want mcp", cmd.Name)
	}
	if cmd.Action == nil {
		t.Error("Action is nil, want runMCP")
	}
	if cmd.Usage == "" {
		t.Error("Usage is empty")
	}
}

func TestTestCommand(t *testing.T) {
	cmd := testCommand()
	if cmd.Name != "test" {
		t.Errorf("Name = %q, want test", cmd.Name)
	}
	if cmd.Action == nil {
		t.Error("Action is nil")
	}
	if cmd.Usage == "" {
		t.Error("Usage is empty")
	}
}
