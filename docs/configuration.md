# Configuration

All configuration is read from the environment. The Gamma data API is public.

| Variable               | Required | Description                                              |
| ---------------------- | :------: | -------------------------------------------------------- |
| `POLYMARKET_BASE_URL`  |    no    | Gamma API base URL (default gamma-api.polymarket.com).   |
| `POLYMARKET_TOOLSETS`  |    no    | Comma-separated toolset names to enable, or `all`.       |
| `POLYMARKET_READONLY`  |    no    | `true` to expose only read-only tools.                   |

## Trading (CLOB API)

> **Trading moves real funds on Polygon mainnet.** See the disclaimer in the
> [README](../README.md) before configuring any of this.

The `trading` toolset (place/cancel orders, balances, open orders, trade
history, order book/price data) is disabled entirely unless
`POLYMARKET_PRIVATE_KEY` is set — with no key, the server runs exactly as
above, serving only the public Gamma data API.

| Variable                    | Required | Description                                                          |
| ---------------------------- | :------: | --------------------------------------------------------------------- |
| `POLYMARKET_PRIVATE_KEY`     |    no    | Wallet private key (hex, with or without `0x`). Enables `trading`.    |
| `POLYMARKET_CLOB_BASE_URL`   |    no    | CLOB API base URL (default `https://clob.polymarket.com`).           |
| `POLYMARKET_CHAIN_ID`        |    no    | EVM chain ID for order signing (default `137`, Polygon mainnet).     |
| `POLYMARKET_FUNDER_ADDRESS`  |    no    | Maker/funder address, if different from the key's own address (e.g. a Polymarket proxy wallet). |
| `POLYMARKET_SIGNATURE_TYPE`  |    no    | Order signature type: `0` (EOA, default), `1` (proxy wallet), `2` (Gnosis Safe). Must match how the funder address holds funds. |

L2 API credentials (key/secret/passphrase) are derived automatically from the
private key on first authenticated request, not at startup — a misconfigured
or unreachable CLOB endpoint only fails trading calls, never the rest of the
server.
