# ModelMux

**Lightweight LLM API Key Router — Built for Reliability, Operated from the Terminal.**

ModelMux is a high-performance reverse proxy that sits between your application and multiple LLM providers. It multiplexes requests across API keys with intelligent routing, rate limiting, and automatic failover — all configurable from a single YAML file and a terminal dashboard.

No dependencies on Redis, message queues, or external databases. Just a single Go binary.

---

## Why ModelMux?

| Problem | ModelMux Solution |
|---------|-------------------|
| API key rate limits kill production traffic | Priority-based rotation, per-minute/concurrency caps, cooldown logic |
| One key goes down, everything breaks | Automatic failover with retry + backoff across multiple keys |
| Token usage tracking gaps | Full accounting for stream AND non-stream requests (OpenAI, Anthropic, Gemini) |
| Config drift between deployments | YAML is the single source of truth; validate before deploy |
| No visibility into routing | Built-in Prometheus metrics, request logs, key health dashboard |
| Secrets in shell history | Encrypted secret store with Argon2id + hidden terminal prompt |

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
server:
  host: "127.0.0.1"
  port: 8787
  require_auth: false           # set true + auth_token_env for network exposure
  max_request_body_mb: 10

storage:
  type: sqlite                  # optional: persist state across restarts
  path: ~/.local/share/modelmux/modelmux.db

providers:
  - id: openai
    name: OpenAI
    type: openai-compatible
    base_url: https://api.openai.com/v1
    auth_type: bearer
    timeout_seconds: 120
    enabled: true

  - id: anthropic
    name: Anthropic
    type: anthropic
    base_url: https://api.anthropic.com
    timeout_seconds: 120
    enabled: true

models:
  - id: gpt-4o
    provider_id: openai
    model_name: gpt-4o
    strategy: failover              # failover | round_robin | least_error | least_used
    requests_per_minute: 120        # optional: limit model-wide RPM
    max_concurrent_requests: 10     # optional: limit simultaneous requests
    capabilities:                   # optional: fine-grained feature gating
      tools: true
      json_mode: true

  - id: claude-sonnet
    provider_id: anthropic
    model_name: claude-3-5-sonnet-20241022
    strategy: round_robin

model_groups:
  - id: premium
    name: Premium Tier
    strategy: weighted             # failover | round_robin | weighted
    members:
      - model_id: gpt-4o
        priority: 1
        weight: 70
      - model_id: claude-sonnet
        priority: 2
        weight: 30

keys:
  - id: openai-key-1
    provider_id: openai
    model_id: gpt-4o
    value_env: OPENAI_KEY_1        # env var (recommended)
    status: active
    priority: 1
    daily_request_limit: 1000
    daily_token_limit: 500000
    requests_per_minute: 60
    tokens_per_minute: 30000
    max_concurrent_requests: 3

  - id: openai-key-2
    provider_id: openai
    model_id: gpt-4o
    secret_ref: openai-backup       # from encrypted secret store
    status: active
    priority: 2

health_check:
  enabled: true
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

| Strategy | Behavior |
|----------|----------|
| `failover` | Lowest priority first (default) |
| `round_robin` | Distribute evenly across keys |
| `least_error` | Pick key with fewest errors |
| `least_used` | Pick key with lowest daily usage |

Group-level strategies (`failover`, `round_robin`, `weighted`) control how members are selected before key rotation kicks in.

### Provider Types

| Type | Supports | Auto-Converted |
|------|----------|----------------|
| `openai-compatible` | Chat, Completions, Streaming, Tools | Direct pass-through |
| `anthropic` | Chat, Streaming | Requests/Responses converted to OpenAI format |
| `gemini` | Chat, Streaming | Requests/Responses converted to OpenAI format |
| `custom` | Same as `openai-compatible` | Direct pass-through |

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

modelmux secret set --ref name          Store secret (omit --value for hidden prompt)
modelmux secret list                    List stored references
modelmux secret delete --ref name       Delete a secret
modelmux secret export --output file    Backup encrypted store
modelmux secret import --input file     Restore from backup
modelmux secret verify                  Check store integrity
modelmux secret rotate-master-key       Re-encrypt with new key

modelmux logs --json --limit 50         Show recent request logs
modelmux logs --model-id gpt-4o --status-code 429

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
modelmux secret set --ref openai-key-1
# Enter value for openai-key-1: ****

# Scripting-safe
modelmux secret set --ref openai-key-2 --value "$TOKEN"
```

Reference secrets in config via `keys[].secret_ref`.

---

## Admin API

When the server is running, admin endpoints are available. Requires auth if `server.require_auth=true`, or localhost-only access otherwise.

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

| Level | Config | Scope |
|-------|--------|-------|
| **Model RPM** | `models[].requests_per_minute` | Global per-model |
| **Model Concurrency** | `models[].max_concurrent_requests` | Simultaneous requests |
| **Key RPM** | `keys[].requests_per_minute` | Per-key |
| **Key Tokens/min** | `keys[].tokens_per_minute` | Per-key total tokens (input + output) |
| **Key Concurrency** | `keys[].max_concurrent_requests` | Per-key simultaneous |
| **Daily Quota** | `keys[].daily_request_limit`, `daily_token_limit` | Per-key daily |

Limits use in-memory rolling windows (minute granularity). Counters reset on restart. When all keys for a model are exhausted, a `429` is returned.

---

## Observability

### Prometheus Metrics

`GET /metrics` returns Prometheus-formatted output:

- `modelmux_requests_total` — Per-model, per-key, per-group request counts
- `modelmux_errors_total` — Error counts
- `modelmux_rate_limits_total` — Rate limit hits
- `modelmux_latency_ms` — Histogram with buckets (50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000 ms)
- `modelmux_status_total` — Status class breakdown (2xx, 4xx, 5xx)
- `modelmux_active_keys`, `cooldown_keys`, `invalid_keys`, `limited_keys` — Key state gauges

### Request Logging

Every proxied request gets a unique `X-ModelMux-Request-ID` response header. Structured JSON logging via `log/slog` records method, path, remote address, and latency.

## Storage

ModelMux uses in-memory state by default. Add SQLite for persistence across restarts:

```yaml
storage:
  type: sqlite
  path: ~/.local/share/modelmux/modelmux.db   # default, optional
```

When enabled, the following data is persisted:

- **Key runtime state** — status, usage counts, cooldown timers, daily quotas
- **Request logs** — full history with model, key, status code, latency, token counts
- **Log queries** — `modelmux logs` CLI and `/logs` HTTP endpoint pull from this store

If `storage.type` is empty or omitted, runtime state resets on restart and logs are kept in-memory (last 200 entries). SQLite uses WAL mode and auto-migrates on startup.

### Health Check

When `health_check.enabled: true`, a background goroutine periodically tests each active key. Auth failures (401/403) mark keys as invalid; transient errors (429, 5xx, network) trigger cooldown. Recovered keys are automatically re-enabled.

---

## TUI Dashboard

```
modelmux tui
```

- **Chat** — In-memory chat sessions with config models. `ctrl+n` new session, `ctrl+t` switch model.
- **Config** — Full YAML editor. Add/edit/delete providers, models, groups, keys. `a` add, `e` edit, `d` delete, `s` save+reload.
- **Logs** — View recent request logs with filtering.
- **Metrics** — Live Prometheus metrics display.
- **Themes** — Multiple terminal color themes.

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

Requirements: Go 1.22+

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
