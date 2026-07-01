# polymarket-mcp

[![CI](https://github.com/rangertaha/polymarket-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/rangertaha/polymarket-mcp/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/rangertaha/polymarket-mcp.svg)](https://pkg.go.dev/github.com/rangertaha/polymarket-mcp)
[![Go Report Card](https://goreportcard.com/badge/github.com/rangertaha/polymarket-mcp)](https://goreportcard.com/report/github.com/rangertaha/polymarket-mcp)
[![Go Version](https://img.shields.io/github/go-mod/go-version/rangertaha/polymarket-mcp)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server, written in Go, that exposes **Polymarket** — both the public Gamma market data API and the authenticated CLOB trading API — as tools an LLM client (Claude Desktop/Code, Cursor, and others) can call.

> [!WARNING]
> **This server can place real trades with real money on Polygon mainnet.**
>
> The `trading` toolset signs and submits live orders using a wallet private key you provide. It is **experimental**: it has not been battle-tested against production trading, endpoint behavior was reconstructed from Polymarket's public API docs rather than a first-party SDK, and an LLM client acting on your behalf can make mistakes. Before relying on it:
>
> - Start with `POLYMARKET_READONLY=true` and small test orders on a wallet you can afford to lose.
> - Never share your private key with anyone, commit it to a repo, or paste it into a prompt — it grants full control over the wallet's funds.
> - Trading is entirely opt-in: leave `POLYMARKET_PRIVATE_KEY` unset and the server only ever reads public market data.
> - This project comes with **no warranty of any kind** (see [LICENSE](LICENSE)). You are solely responsible for any funds you trade with it.

## Install

Prebuilt binaries are published on the [latest release](https://github.com/rangertaha/polymarket-mcp/releases/latest). Download the archive for your platform, extract the `polymarket` binary, and put it on your `PATH`:

| Platform | Architecture          | Download (latest)                                                                                                                          |
| -------- | --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| macOS    | Apple Silicon (arm64) | [`polymarket-mcp_darwin_arm64.tar.gz`](https://github.com/rangertaha/polymarket-mcp/releases/latest/download/polymarket-mcp_darwin_arm64.tar.gz) |
| macOS    | Intel (amd64)         | [`polymarket-mcp_darwin_amd64.tar.gz`](https://github.com/rangertaha/polymarket-mcp/releases/latest/download/polymarket-mcp_darwin_amd64.tar.gz) |
| Linux    | amd64                 | [`polymarket-mcp_linux_amd64.tar.gz`](https://github.com/rangertaha/polymarket-mcp/releases/latest/download/polymarket-mcp_linux_amd64.tar.gz)   |
| Linux    | arm64                 | [`polymarket-mcp_linux_arm64.tar.gz`](https://github.com/rangertaha/polymarket-mcp/releases/latest/download/polymarket-mcp_linux_arm64.tar.gz)   |
| Windows  | amd64                 | [`polymarket-mcp_windows_amd64.zip`](https://github.com/rangertaha/polymarket-mcp/releases/latest/download/polymarket-mcp_windows_amd64.zip)     |
| Windows  | arm64                 | [`polymarket-mcp_windows_arm64.zip`](https://github.com/rangertaha/polymarket-mcp/releases/latest/download/polymarket-mcp_windows_arm64.zip)     |

Each link always resolves to the newest release. A [`checksums.txt`](https://github.com/rangertaha/polymarket-mcp/releases/latest/download/checksums.txt) is published alongside the archives.

<details>
<summary><strong>macOS / Linux</strong></summary>

Pick your `OS`/`ARCH`:

```sh
OS=darwin ARCH=arm64   # OS: darwin|linux   ARCH: amd64|arm64
curl -sSL "https://github.com/rangertaha/polymarket-mcp/releases/latest/download/polymarket-mcp_${OS}_${ARCH}.tar.gz" | tar -xz polymarket
sudo mv polymarket /usr/local/bin/
polymarket --version
```

</details>

<details>
<summary><strong>Windows (PowerShell)</strong></summary>

Pick your `$Arch`:

```powershell
$Arch = "amd64"   # ARCH: amd64|arm64
Invoke-WebRequest "https://github.com/rangertaha/polymarket-mcp/releases/latest/download/polymarket-mcp_windows_${Arch}.zip" -OutFile polymarket.zip
Expand-Archive polymarket.zip -DestinationPath .
.\polymarket.exe --version
```

</details>

<details>
<summary><strong>Install with Go</strong></summary>

```sh
go install github.com/rangertaha/polymarket-mcp/cmd/polymarket@latest
```

</details>

<details>
<summary><strong>Build from source</strong></summary>

```sh
git clone https://github.com/rangertaha/polymarket-mcp
cd polymarket-mcp
make build        # produces ./bin/polymarket
```

</details>

## Features

- **Public by default**: the Gamma market data API needs no credentials — list and inspect markets out of the box.
- **Trading is opt-in**: set `POLYMARKET_PRIVATE_KEY` to additionally enable the `trading` toolset (place/cancel orders, balances, open orders, trade history, order book/price data) against Polymarket's CLOB API. Without it, the trading toolset registers no tools at all.
- **Typed tools with schemas**: every tool has an auto-generated JSON Schema for its input and output, inferred from Go structs, with per-field descriptions. Inputs are validated before a handler runs.
- **Read-only switch**: `POLYMARKET_READONLY=true` hides every mutating tool (placing/canceling orders), so the server can be safely pointed at a funded wallet for read access only.
- **Toolset filtering**: enable only the areas you need with `POLYMARKET_TOOLSETS`.
- **Lazy credential derivation**: CLOB API credentials are derived from your private key on first authenticated call, not at startup, so a misconfigured or unreachable trading endpoint never breaks the public data tools.
- **Built on the official SDK**: uses [`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) (v1).

## Configuration

All configuration is read from the environment.

| Variable                    | Required | Description                                                                                       |
| ---------------------------- | :------: | --------------------------------------------------------------------------------------------------- |
| `POLYMARKET_BASE_URL`        |    no    | Gamma API base URL (default `https://gamma-api.polymarket.com`).                                    |
| `POLYMARKET_TOOLSETS`        |    no    | Comma-separated toolset names to enable, or `all` (default).                                        |
| `POLYMARKET_READONLY`        |    no    | `true` to expose only read-only tools.                                                               |
| `POLYMARKET_PRIVATE_KEY`     |    no    | Wallet private key (hex). **Enables the `trading` toolset** — see the disclaimer above.              |
| `POLYMARKET_CLOB_BASE_URL`   |    no    | CLOB trading API base URL (default `https://clob.polymarket.com`).                                  |
| `POLYMARKET_CHAIN_ID`        |    no    | EVM chain ID for order signing (default `137`, Polygon mainnet).                                    |
| `POLYMARKET_FUNDER_ADDRESS`  |    no    | Maker/funder address, if different from the address derived from `POLYMARKET_PRIVATE_KEY`.          |
| `POLYMARKET_SIGNATURE_TYPE`  |    no    | Order signature type: `0` (EOA, default), `1` (proxy wallet), `2` (Gnosis Safe).                     |

See [docs/configuration.md](docs/configuration.md) for more detail on the trading variables.

### Use with Claude Desktop / Claude Code

Read-only market data, no credentials needed:

```json
{
  "mcpServers": {
    "polymarket": {
      "command": "polymarket",
      "args": ["mcp"]
    }
  }
}
```

With trading enabled (see the disclaimer above first):

```json
{
  "mcpServers": {
    "polymarket": {
      "command": "polymarket",
      "args": ["mcp"],
      "env": {
        "POLYMARKET_PRIVATE_KEY": "your-wallet-private-key"
      }
    }
  }
}
```

For Claude Code: `claude mcp add polymarket -- polymarket mcp`.

### Local development

The repo ships a committed [`.mcp.json`](.mcp.json) that runs the server straight from source (`go run ./cmd/polymarket mcp`), so changes take effect on the next session without a build step. It reads credentials from your environment (no secrets in the repo). Run `cp .env.example .env` and fill it in before launching Claude Code in this directory. A local `.claude/settings.local.json` auto-trusts the server via `enabledMcpjsonServers`.

## CLI

```sh
polymarket mcp      # run the MCP server over stdio (default when no subcommand)
polymarket test     # verify connectivity against the Gamma API
```

## Toolsets

| Toolset   | Covers                                                                                                                     |
| --------- | ---------------------------------------------------------------------------------------------------------------------- |
| `markets` | List markets (`markets_list`) and get one (`markets_get`) — public Gamma data, no credentials needed.                    |
| `trading` | Place/cancel orders, balances, open orders, trade history, order book/price data — CLOB API. Requires `POLYMARKET_PRIVATE_KEY`; registers no tools without it. |

### TODO toolsets

- `events` — list/get events and their nested markets.

## Tools

Tools follow the naming convention `<toolset>_<verb>_<noun>`. Tools marked **[write]** mutate data (place real orders) and are hidden when `POLYMARKET_READONLY=true`.

<details>
<summary><strong>All 11 tools</strong></summary>

### markets
- `markets_list`: List Polymarket markets, optionally filtered to active and/or closed markets, with paging.
- `markets_get`: Get a single Polymarket market by its numeric ID.

### trading
- `trading_get_order_book`: Get the CLOB order book (bids, asks, tick size, neg-risk flag) for an outcome token.
- `trading_get_price`: Get the best bid (side=BUY) or best ask (side=SELL) price for an outcome token.
- `trading_get_midpoint`: Get the midpoint between the best bid and best ask for an outcome token.
- `trading_get_balance`: Get the authenticated wallet's collateral balance (default) or a specific outcome token balance, plus exchange allowances.
- `trading_list_open_orders`: List the authenticated wallet's open CLOB orders, optionally filtered by market or outcome token.
- `trading_list_trades`: List the authenticated wallet's trade (fill) history, optionally filtered by market or outcome token.
- `trading_place_order` **[write]**: Sign and submit a limit order (BUY or SELL) for an outcome token. Moves real funds.
- `trading_cancel_order` **[write]**: Cancel a single open order by its ID (order hash).
- `trading_cancel_all_orders` **[write]**: Cancel every open order for the authenticated wallet.

</details>

## Prompts (workflows)

MCP clients surface prompts as **slash commands** automatically (e.g. in Claude Code and Claude Desktop):

| Prompt        | Arguments | What it does                                                          |
| ------------- | --------- | ----------------------------------------------------------------------- |
| `market_odds` | id        | Read the implied odds for a Polymarket market and explain what the prices mean. |

## Architecture

<details>
<summary>Project layout</summary>

```
cmd/polymarket                 entrypoint: a urfave/cli command tree (mcp, test)
internal/config                environment configuration + validation
internal/client                generic JSON REST client (auth, paging, error mapping)
internal/clob                  CLOB trading protocol: wallet, EIP-712 signing, L1/L2 auth
internal/server                MCP server wrapper, typed tool registration, read-only policy
internal/polymarket            shared Gamma + CLOB REST clients
internal/polymarket/markets    public Gamma market data: list/get
internal/polymarket/trading    CLOB trading: orders, balances, order book/price data
internal/prompts               built-in workflow prompts
```

The `mcp` subcommand loads config, registers the enabled toolsets, and serves over stdio; `test` checks Gamma connectivity.

Each service area follows the same shape: a `service` wrapping the shared REST clients exposes typed operations, and a `Register` function registers thin MCP tool handlers for them. `trading.Register` is the one exception that can register zero tools — it no-ops when no wallet is configured. Adding a new area is a matter of dropping in a package and listing it in `internal/app/app.go`.

</details>

## Development

<details>
<summary>Build, test, and smoke-test</summary>

```sh
make test        # go test -race ./...
make vet         # go vet ./...
make fmt-check   # gofmt verification
make all         # fmt-check + vet + lint + test + build
```

### Smoke-testing the protocol

List the tools over stdio without an MCP client:

```sh
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"s","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
| ./bin/polymarket mcp
```

Or browse interactively with the [MCP Inspector](https://github.com/modelcontextprotocol/inspector):

```sh
npx @modelcontextprotocol/inspector ./bin/polymarket mcp
```

</details>

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

## License

MIT — see [LICENSE](LICENSE).
