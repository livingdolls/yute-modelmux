![ModelMux TUI](https://res.cloudinary.com/dwg1vtwlc/image/upload/v1782697574/repo/Cuplikan_layar_dari_2026-06-29_08-43-28_oxcdu6.png)

# ModelMux

**Lightweight LLM API Key Router — Built for Reliability, Operated from the Terminal.**

ModelMux is a high-performance reverse proxy that sits between your application and multiple LLM providers. It multiplexes requests across API keys with intelligent routing, rate limiting, and automatic failover — all configurable from a single YAML file and a terminal dashboard.

No dependencies on Redis, message queues, or external databases. Just a single Go binary.

---

## Why ModelMux?

| Problem                                     | ModelMux Solution                                                              |
| ------------------------------------------- | ------------------------------------------------------------------------------ |
| API key rate limits kill production traffic | Priority-based rotation, per-minute/concurrency caps, cooldown logic           |
| One key goes down, everything breaks        | Automatic failover with retry + backoff across multiple keys                   |
| Token usage tracking gaps                   | Full accounting for stream AND non-stream requests (OpenAI, Anthropic, Gemini) |
| Config drift between deployments            | YAML is the single source of truth; validate before deploy                     |
| No visibility into routing                  | Built-in Prometheus metrics, request logs, key health dashboard                |
| Secrets in shell history                    | Encrypted secret store with Argon2id + hidden terminal prompt                  |

---

## Quick Start

```bash
# 1. Install
go install github.com/livingdolls/yute-modelmux/cmd/modelmux@latest

# 2. Create config
modelmux config init
vim ~/.config/modelmux/config.yaml

# 3. Start
modelmux start
```

Point your OpenAI-compatible client at `http://127.0.0.1:8787/v1` and use config-defined model IDs.

---

## Configuration

Everything lives in `~/.config/modelmux/config.yaml`:

```yaml
app:
  name: modelmux
  log_level: info

server:
  host: "127.0.0.1"
  port: 8787
  require_auth: false # set true + auth_token_env for network exposure
  max_request_body_mb: 10

storage:
  type: "" # set to sqlite to persist state across restarts
  path: ~/.local/share/modelmux/modelmux.db

providers:
  - id: mimo
    name: Xiaomi MiMo
    type: openai-compatible
    base_url: https://api.example.com/v1
    auth_type: bearer
    timeout_seconds: 120
    enabled: true

models:
  - id: mimo-v2.5-pro
    provider_id: mimo
    model_name: mimo-v2.5-pro
    strategy: failover # failover | round_robin | least_error | least_used
    enabled: true
    requests_per_minute: 120 # optional: limit model-wide RPM
    max_concurrent_requests: 10 # optional: limit simultaneous requests
    capabilities: # optional: fine-grained feature gating
      tools: true
      json_mode: true

model_groups:
  - id: high-price
    name: High Price Models
    strategy: weighted # failover | round_robin | weighted
    enabled: true
    members:
      # model_id members route through any available key for that model
      - model_id: mimo-v2.5-pro
        priority: 1
        weight: 1
        enabled: true
      # key_id members pin this group member to one exact key
      - key_id: mimo-key-1
        priority: 2
        weight: 1
        enabled: true

keys:
  - id: mimo-key-1
    provider_id: mimo
    model_id: mimo-v2.5-pro
    value_env: MIMO_API_KEY # env var (recommended)
    status: active
    priority: 1
    daily_request_limit: 1000
    daily_token_limit: 500000
    requests_per_minute: 60
    tokens_per_minute: 30000
    max_concurrent_requests: 3

health_check:
  enabled: false
  interval_seconds: 300
  timeout_seconds: 15

cooldown:
  rate_limit_seconds: 300
  server_error_seconds: 60
  timeout_seconds: 60

retry:
  max_retry_per_key: 1
  max_total_attempts: 5
  backoff_milliseconds: [300, 700, 1500]
```

### Key Rotation Strategies

| Strategy      | Behavior                         |
| ------------- | -------------------------------- |
| `failover`    | Lowest priority first (default)  |
| `round_robin` | Distribute evenly across keys    |
| `least_error` | Pick key with fewest errors      |
| `least_used`  | Pick key with lowest daily usage |

Group-level strategies (`failover`, `round_robin`, `weighted`) control how members are selected before key rotation kicks in. Group members can reference either `model_id` or `key_id`: `model_id` keeps the normal per-model key pool behavior, while `key_id` pins that member to one exact key. This lets one group contain entries such as `key-1-deepseek`, `key-1-mimo`, `key-2-mimo`, and a model-level fallback.

### Provider Types

| Type                | Supports                            | Auto-Converted                                |
| ------------------- | ----------------------------------- | --------------------------------------------- |
| `openai-compatible` | Chat, Completions, Streaming, Tools | Direct pass-through                           |
| `anthropic`         | Chat, Streaming                     | Requests/Responses converted to OpenAI format |
| `gemini`            | Chat, Streaming                     | Requests/Responses converted to OpenAI format |
| `custom`            | Same as `openai-compatible`         | Direct pass-through                           |

---

## CLI Commands

```
modelmux start                 Start the proxy server
modelmux tui                   Open terminal dashboard
modelmux config init           Create example config file
modelmux config validate       Validate config (--json for machine output)
modelmux config validate --check-provider   Also ping provider health

modelmux key test --id <key>   Test a key against its provider
modelmux key enable --id <key> Enable a key in config
modelmux key disable --id <key> Disable a key in config
modelmux key add ...           Add a new key

modelmux provider add ...      Add a provider
modelmux model add ...         Add a model
modelmux group add ...         Add a model group

modelmux secret set --ref name          Store secret (omit --value for hidden prompt)
modelmux secret list                    List stored references
modelmux secret delete --ref name       Delete a secret
modelmux secret export --output file    Backup encrypted store
modelmux secret import --input file     Restore from backup
modelmux secret verify                  Check store integrity
modelmux secret rotate-master-key       Re-encrypt with new key

modelmux logs --json --limit 50         Show recent request logs
modelmux logs --model-id mimo-v2.5-pro --status-code 429

modelmux providers                     List providers (--json)
modelmux models                        List models (--json)
modelmux keys                          List keys (--json)
modelmux groups                        List groups (--json)
```

### Secret Store

Secrets are encrypted at rest with AES-256-GCM, key derivation via Argon2id (64 MiB memory, 3 iterations). Master key comes from `MODELMUX_MASTER_KEY` env var.

```bash
export MODELMUX_MASTER_KEY="your-strong-master-password"

# Interactive hidden prompt
modelmux secret set --ref mimo-key-1
# Enter value for mimo-key-1: ****

# Scripting-safe
modelmux secret set --ref mimo-key-2 --value "$TOKEN"
```

Reference secrets in config via `keys[].secret_ref`.

---

## Admin API

When the server is running, admin endpoints are available. Requires auth if `server.require_auth=true`; otherwise `/admin/*` is allowed only from localhost.

```
POST /admin/reload                    Reload config without restart
POST /admin/keys/{id}/enable          Enable a key (updates YAML + runtime)
POST /admin/keys/{id}/disable         Disable a key
POST /admin/keys/{id}/test            Test key connectivity
GET  /admin/status                    Server summary (keys, models, providers)
```

---

## Rate Limiting

ModelMux enforces limits at three levels, applied in order:

| Level                 | Config                                            | Scope                                 |
| --------------------- | ------------------------------------------------- | ------------------------------------- |
| **Model RPM**         | `models[].requests_per_minute`                    | Global per-model                      |
| **Model Concurrency** | `models[].max_concurrent_requests`                | Simultaneous requests                 |
| **Key RPM**           | `keys[].requests_per_minute`                      | Per-key                               |
| **Key Tokens/min**    | `keys[].tokens_per_minute`                        | Per-key total tokens (input + output) |
| **Key Concurrency**   | `keys[].max_concurrent_requests`                  | Per-key simultaneous                  |
| **Daily Quota**       | `keys[].daily_request_limit`, `daily_token_limit` | Per-key daily                         |

Limits use in-memory rolling windows (minute granularity). Counters reset on restart. When all keys for a model are exhausted, a `429` is returned.

---

## Observability

### Prometheus Metrics

`GET /metrics` returns JSON metrics by default. Use `GET /metrics?format=prometheus` for Prometheus-formatted output:

- `modelmux_requests_total` — Per-model, per-key, per-group request counts
- `modelmux_errors_total` — Error counts
- `modelmux_rate_limits_total` — Rate limit hits
- `modelmux_latency_ms` — Histogram with buckets (50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000 ms)
- `modelmux_status_total` — Status class breakdown (2xx, 4xx, 5xx)
- `modelmux_active_keys`, `cooldown_keys`, `invalid_keys`, `limited_keys` — Key state gauges

### Request Logging

Every proxied request gets a unique `X-ModelMux-Request-ID` response header. Structured JSON logging via `log/slog` records method, path, remote address, and latency.

## Storage

ModelMux uses in-memory state by default. This is fine for local testing, but all runtime state resets when the process restarts. Enable SQLite when you want request logs, key usage counters, daily quotas, and cooldown state to survive restarts.

```yaml
storage:
  type: sqlite
  path: ~/.local/share/modelmux/modelmux.db # default, optional
```

If `storage.type` is empty or omitted, runtime state resets on restart and logs are kept in-memory only. The in-memory log buffer keeps the last 200 entries.

When SQLite is enabled, the following data is persisted:

- **Key runtime state** — status, usage counts, cooldown timers, daily quotas
- **Daily usage counters** — request and token counts used by `daily_request_limit` and `daily_token_limit`
- **Request logs** — history with group, model, provider, key, status code, latency, token counts, and timestamp
- **Log queries** — `modelmux logs` CLI and `/logs` HTTP endpoint pull from this store
- **Metrics source data** — `/metrics` can use stored request logs instead of only in-memory logs

SQLite is opened in WAL mode and auto-migrates its schema on startup. No Redis, Postgres, or external database service is required.

Common log queries:

```bash
modelmux logs --limit 50
modelmux logs --model-id mimo-v2.5-pro
modelmux logs --status-code 429
modelmux logs --json --limit 100
```

The HTTP log endpoint supports similar filters:

```http
GET /logs?limit=50
GET /logs?model_id=mimo-v2.5-pro
GET /logs?status_code=429
```

SQLite creates the main database file plus WAL files next to it:

```text
~/.local/share/modelmux/modelmux.db
~/.local/share/modelmux/modelmux.db-wal
~/.local/share/modelmux/modelmux.db-shm
```

The `-wal` and `-shm` files are normal SQLite WAL files.

If you use the encrypted secret store, its file is placed next to the storage directory. With the default storage path, the secret store is:

```text
~/.local/share/modelmux/secrets.enc
```

Secret store usage still requires `MODELMUX_MASTER_KEY`:

```bash
export MODELMUX_MASTER_KEY="your-strong-master-password"
```

For backups, copy the SQLite database and, if used, the encrypted secret store:

```bash
cp ~/.local/share/modelmux/modelmux.db ~/modelmux.db.backup
cp ~/.local/share/modelmux/secrets.enc ~/modelmux-secrets.enc.backup
```

Keep the matching `MODELMUX_MASTER_KEY` safe. Without it, `secrets.enc` cannot be decrypted.

SQLite storage is intended for a single ModelMux instance using one database file. Sharing the same SQLite file across multiple running instances is not recommended.

### Health Check

When `health_check.enabled: true`, a background goroutine periodically tests each active key. Auth failures (401/403) mark keys as invalid; transient errors (429, 5xx, network) trigger cooldown. Recovered keys are automatically re-enabled.

---

## TUI Dashboard

```
modelmux tui
```

- **Chat** — In-memory chat sessions with config models/groups. `enter` opens Chat from the menu, choose a session, then `enter` opens the conversation. `pgup/pgdown` scrolls history; `esc` returns from conversation to sessions.
- **Config** — Full YAML editor. `enter` opens Config from the menu, `up/down` select rows, `left/right` switch sections, and `esc` returns to the menu. Selectable fields open popups; group members support multi-select across models and keys.
- **Logs** — View recent request logs with filtering.
- **Metrics** — Live Prometheus metrics display.
- **Themes** — Multiple terminal color themes.

### TUI Keyboard Shortcuts

| Area   | Shortcut            | Action                                            |
| ------ | ------------------- | ------------------------------------------------- |
| Menu   | `tab` / `shift+tab` | Move between pages                                |
| Menu   | `enter`             | Open the selected page                            |
| Global | `esc`               | Cancel input, close dialogs, or return to menu    |
| Global | `?`                 | Toggle help when not typing                       |
| Global | `q`                 | Quit when not typing                              |
| Global | `ctrl+c`            | Force quit                                        |
| Chat   | `enter`             | Open selected session or send message             |
| Chat   | `up/down`           | Choose chat session                               |
| Chat   | `pgup` / `pgdown`   | Scroll conversation history                       |
| Chat   | `esc`               | Clear text, return to sessions, or return to menu |
| Chat   | `ctrl+n`            | New chat session                                  |
| Chat   | `ctrl+t`            | Switch target model                               |
| Chat   | `ctrl+f`            | Filter chat sessions                              |
| Config | `up/down`           | Select row                                        |
| Config | `left/right`        | Switch section                                    |
| Config | `enter`             | Edit selected item                                |
| Config | `esc`               | Cancel form/filter or return to menu              |
| Config | `right`             | Open select popup for selectable form fields      |
| Config | `space`             | Toggle item in multi-select popup                 |
| Config | `a`                 | Add item                                          |
| Config | `delete`            | Delete selected item                              |
| Config | `ctrl+s`            | Save config and reload router                     |
| Config | `ctrl+r`            | Reload draft config                               |
| Config | `/` or `ctrl+f`     | Filter rows                                       |
| Keys   | `1`, `2`, `3`       | Sort by status, cooldown, or errors               |
| Keys   | `x`                 | Test a key by ID                                  |
| Logs   | `1`-`6`             | Change log filter or sort order                   |
| Theme  | `t`                 | Cycle theme when not typing                       |
| Theme  | `T`                 | Open theme picker when not typing                 |

Text input modes do not use single-letter navigation shortcuts, so normal typing works as expected.

---

## Architecture

```
Client (OpenAI SDK)
       │
       ▼
┌─────────────┐    ┌──────────────┐
│  HTTP Proxy │───▶│ RouterService │── Key selection, retry, rate limiting
│  :8787      │    │               │── Provider conversion (Anthropic ↔ OpenAI)
└─────────────┘    └──────┬────────┘
                          │
          ┌───────────────┼───────────────┐
          ▼               ▼               ▼
   ┌──────────┐   ┌──────────┐   ┌──────────┐
   │ OpenAI   │   │Anthropic │   │ Gemini   │
   │ upstream │   │ upstream │   │ upstream │
   └──────────┘   └──────────┘   └──────────┘
```

- **Hexagonal architecture** — Domain logic isolated from adapters (CLI, HTTP, TUI)
- **Concurrency-safe** — All shared state protected by mutexes
- **Streaming passthrough** — SSE events forwarded with minimal buffering; token usage tracked for all providers
- **Storage** — Optional SQLite for persistence (request logs, key runtime state)

---

## Security

- API keys never logged or exposed in metrics
- Config YAML preserves `value_env`/`secret_ref` — plaintext `value` optional for dev
- Secret store: AES-256-GCM + Argon2id key derivation with per-file salt
- Admin endpoints protected by auth or localhost-only
- Prometheus label values escaped to prevent injection

---

## Install

```bash
go install github.com/livingdolls/yute-modelmux/cmd/modelmux@latest
```

Requirements: Go 1.25+

Or build from source:

```bash
git clone https://github.com/livingdolls/yute-modelmux.git
cd yute-modelmux
make build
./bin/modelmux --help
```

---

## License

MIT
