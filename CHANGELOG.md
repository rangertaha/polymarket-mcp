# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial scaffold: MCP server over stdio with the `markets` toolset
  (`markets_list`, `markets_get`), a `market_odds` prompt, and a
  `polymarket test` connectivity check.
- `trading` toolset for Polymarket's CLOB API: order book/price/midpoint
  lookups, balances, open orders, trade history, and signed order
  placement/cancellation. Opt-in via `POLYMARKET_PRIVATE_KEY`; registers no
  tools without it. **Experimental — moves real funds, see the README
  disclaimer.**
