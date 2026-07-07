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

### Fixed
- `trading_place_order`'s `GTD` (good-til-date) order type was unusable: the
  tool had no way to supply an expiration, so every signed order carried
  `expiration=0` regardless of the requested type. Added an `expiration`
  (unix seconds) input parameter, threaded through to the signed order, with
  validation that rejects `GTD` orders unless `expiration` is a future
  timestamp. The `GTD` check is case-insensitive (`orderType` is normalized
  to uppercase before validation and before being sent to the CLOB), so a
  lowercase `gtd` can't bypass the expiration requirement.
- The shared REST client double-encoded any pre-escaped dynamic path segment
  (e.g. `markets_get`'s market ID going through `url.PathEscape`): a literal
  `%2F` became `%252F` on the wire because only the decoded `url.URL.Path`
  was set, so the stdlib re-escaped it. `buildRequest` now sets `RawPath`
  alongside `Path` so an already-escaped segment is sent as-is.
- `POLYMARKET_PRIVATE_KEY` given with an uppercase `0X` prefix (instead of
  `0x`) failed configuration validation with a misleading "not a valid
  private key" error: the prefix strip only matched the lowercase form, so
  `crypto.HexToECDSA` then rejected the leftover `X`. Both the config loader
  and `clob.NewWallet` now strip either case.
- The `market_odds` prompt's `id` argument was declared `Required: true` but
  never actually enforced: the MCP SDK only uses `Required` as a client-side
  hint and does no server-side validation, so calling the prompt without `id`
  silently rendered a broken prompt (an empty market ID interpolated into the
  instructions) instead of erroring. `server.AddPrompt` now validates
  required arguments before rendering.
- A typo'd `POLYMARKET_TOOLSETS` value (e.g. `makets` instead of `markets`)
  silently started the server with **zero tools and no error**: an unknown
  name just never matches `ToolsetEnabled`, so every toolset registers
  nothing. `app.Assemble` now validates toolset names against the real
  registry and fails with a clear error listing the valid names.
- `POLYMARKET_FUNDER_ADDRESS`, unlike every other trading-only config field
  (chain ID, CLOB URL, signature type), had no validation at all: a malformed
  value passed `Load()` silently and only surfaced as a signing error much
  later, on the first `trading_place_order` call. It's now validated eagerly
  alongside the other trading fields, consistent with the "fail fast at
  startup" behavior the rest of that block already documents.
