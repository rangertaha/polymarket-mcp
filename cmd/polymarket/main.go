// SPDX-License-Identifier: MIT

// Command polymarket runs the Polymarket Model Context Protocol server
// (`polymarket mcp`) and checks connectivity (`polymarket test`).
//
// Configuration is read from the environment (see package config). The `mcp`
// command communicates over stdio, the transport expected by MCP clients such
// as Claude Desktop/Code and Cursor.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urfave/cli/v3"

	"github.com/rangertaha/polymarket-mcp/internal"
	"github.com/rangertaha/polymarket-mcp/internal/app"
	"github.com/rangertaha/polymarket-mcp/internal/config"
	"github.com/rangertaha/polymarket-mcp/internal/polymarket"
)

func main() {
	cmd := &cli.Command{
		Name:    "polymarket",
		Usage:   "Polymarket prediction markets as an MCP server",
		Version: internal.Version(),
		// A bare `polymarket` (no subcommand) runs the MCP server.
		Action: runMCP,
		Commands: []*cli.Command{
			mcpCommand(),
			testCommand(),
		},
		// Print errors ourselves so the MCP stdio stream is never touched.
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "polymarket: %v\n", err)
		os.Exit(1)
	}
}

// mcpCommand runs the MCP server over stdio.
func mcpCommand() *cli.Command {
	return &cli.Command{
		Name:   "mcp",
		Usage:  "Run the MCP server over stdio (for Claude Desktop/Code, Cursor, ...)",
		Action: runMCP,
	}
}

// runMCP assembles and serves the MCP server over stdio.
func runMCP(ctx context.Context, _ *cli.Command) error {
	if err := config.LoadEnvFile(config.EnvFile); err != nil {
		log.Printf("polymarket: reading %s: %v", config.EnvFile, err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration error:\n%w", err)
	}

	ver := internal.Version()
	srv, cleanup, err := app.Assemble(cfg, ver)
	if err != nil {
		return err
	}
	defer cleanup()

	log.Printf("polymarket-mcp %s starting: %d tools, %d prompts across toolsets %v (read-only=%v)",
		ver, srv.ToolCount(), srv.PromptCount(), srv.Toolsets(), cfg.ReadOnly)

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return srv.Run(ctx, &mcp.StdioTransport{})
}

// testCommand verifies connectivity against the Polymarket Gamma API.
func testCommand() *cli.Command {
	return &cli.Command{
		Name:  "test",
		Usage: "Test connectivity against the Polymarket Gamma API",
		Action: func(ctx context.Context, _ *cli.Command) error {
			if err := config.LoadEnvFile(config.EnvFile); err != nil {
				log.Printf("polymarket: reading %s: %v", config.EnvFile, err)
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("configuration error:\n%w", err)
			}

			clients, err := polymarket.NewClients(cfg.BaseURL)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			n, err := polymarket.Check(ctx, clients)
			if err != nil {
				return fmt.Errorf("connecting to %s: %w", cfg.BaseURL, err)
			}

			fmt.Printf("OK  connected to %s (%d market(s) returned)\n", cfg.BaseURL, n)
			fmt.Printf("    read-only=%v\n", cfg.ReadOnly)
			return nil
		},
	}
}
