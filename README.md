# polymarket-mcp

[![CI](https://github.com/rangertaha/polymarket-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/rangertaha/polymarket-mcp/actions/workflows/ci.yml)
[![Status: under construction](https://img.shields.io/badge/status-under%20construction-orange)](#-under-construction)

<div align="center">

## 🚧 &nbsp; UNDER CONSTRUCTION &nbsp; 🚧

**This server is an early scaffold — a work in progress.**

It runs over stdio with **one read-only toolset** wired end-to-end.<br>
More toolsets are on the way (see the **TODO** list below).<br>
APIs, configuration, and tool names may still change.

</div>

---

A [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server, written
in Go, exposing the **Polymarket** Gamma data API as tools an LLM client
(Claude Desktop/Code, Cursor, and others) can call.

## Features

- **Typed tools with schemas**: every tool has an auto-generated JSON Schema for
  its input and output, inferred from Go structs.
- **Read-only switch**: `POLYMARKET_READONLY=true` hides every mutating tool.
- **Toolset filtering**: enable only the areas you need with `POLYMARKET_TOOLSETS`.
- **Public by default**: the Gamma data API needs no credentials.

## Install

```sh
go install github.com/rangertaha/polymarket-mcp/cmd/polymarket@latest
```

Or build from source:

```sh
git clone https://github.com/rangertaha/polymarket-mcp
cd polymarket-mcp
make build        # produces ./bin/polymarket
```

## CLI

```sh
polymarket mcp      # run the MCP server over stdio (default when no subcommand)
polymarket test     # verify connectivity
```

## Configuration

| Variable               | Required | Description                                                  |
| ---------------------- | :------: | ------------------------------------------------------------ |
| `POLYMARKET_BASE_URL`  |    no    | Gamma API base URL (default `https://gamma-api.polymarket.com`). |
| `POLYMARKET_TOOLSETS`  |    no    | Comma-separated toolset names to enable, or `all`.           |
| `POLYMARKET_READONLY`  |    no    | `true` to expose only read-only tools.                       |

## Toolsets

| Toolset   | Covers                                                          |
| --------- | -------------------------------------------------------------- |
| `markets` | list markets (`markets_list`) and get one (`markets_get`) — public Gamma data |

### TODO toolsets

- `events` — list/get events and their nested markets.
- `prices` — CLOB price history / order book (CLOB API).
- `orders` — place/cancel orders (write; needs L2 auth via the CLOB API).

## License

MIT — see [LICENSE](LICENSE).
