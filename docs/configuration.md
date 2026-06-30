# Configuration

All configuration is read from the environment. The Gamma data API is public.

| Variable               | Required | Description                                              |
| ---------------------- | :------: | -------------------------------------------------------- |
| `POLYMARKET_BASE_URL`  |    no    | Gamma API base URL (default gamma-api.polymarket.com).   |
| `POLYMARKET_TOOLSETS`  |    no    | Comma-separated toolset names to enable, or `all`.       |
| `POLYMARKET_READONLY`  |    no    | `true` to expose only read-only tools.                   |
