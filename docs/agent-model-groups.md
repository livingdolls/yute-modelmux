# Agent model groups

Model groups are virtual OpenAI-compatible models. An agent sends a normal request with a group ID in the `model` field, and ModelMux selects a compatible physical model, provider, and key.

```json
{
  "model": "coding-balanced",
  "messages": [{"role": "user", "content": "Refactor this package"}],
  "stream": true
}
```

## Configuration

```yaml
model_groups:
  - id: coding-flagship
    name: Coding — Flagship
    description: Highest-quality models for difficult coding tasks
    strategy: failover
    enabled: true
    required_capabilities: [chat, streaming, tools]
    context_window: 200000
    max_output_tokens: 32000
    members:
      - model_id: claude-opus
        priority: 1
        weight: 1
        enabled: true
      - model_id: gpt-codex
        priority: 2
        weight: 1
        enabled: true

  - id: coding-balanced
    name: Coding — Balanced
    description: General coding with controlled cost
    strategy: consistent_hash
    enabled: true
    required_capabilities: [chat, streaming, tools]
    context_window: 128000
    max_output_tokens: 32000
    members:
      - model_id: claude-sonnet
        priority: 1
        weight: 1
        enabled: true
      - model_id: gemini-pro
        priority: 2
        weight: 1
        enabled: true

  - id: coding-economy
    name: Coding — Economy
    description: Lower-cost models for routine work
    strategy: weighted
    enabled: true
    required_capabilities: [chat, streaming, tools]
    context_window: 64000
    max_output_tokens: 16000
    members:
      - model_id: qwen-coder
        priority: 1
        weight: 70
        enabled: true
      - model_id: deepseek-coder
        priority: 2
        weight: 30
        enabled: true
```

Supported group strategies:

- `failover`: use priority order and move to the next member when the selected model is unavailable.
- `round_robin`: rotate requests across available compatible members.
- `weighted`: randomly select using each member's weight.
- `consistent_hash`: keep the same session on the same member using rendezvous hashing, while retaining failover.

Before selection, ModelMux filters members against the request's chat/completions, streaming, tools, vision, and JSON-mode requirements. `required_capabilities` additionally guarantees that every enabled member provides the listed capabilities when the router starts.

## Stable sessions

For `consistent_hash`, ModelMux uses the first available value from:

1. `X-ModelMux-Session-ID` header
2. `X-OpenAI-User` or `OpenAI-User` header
3. OpenAI request field `user`
4. `metadata.session_id`, `metadata.conversation_id`, or `metadata.thread_id`

Without a session value, `consistent_hash` falls back to priority order.

## OpenCode

Configure ModelMux as an OpenAI-compatible provider and expose group IDs as models:

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "model": "modelmux/coding-balanced",
  "provider": {
    "modelmux": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "ModelMux",
      "options": {
        "baseURL": "http://127.0.0.1:8787/v1",
        "apiKey": "{env:MODELMUX_AUTH_TOKEN}"
      },
      "models": {
        "coding-flagship": {"name": "Coding — Flagship"},
        "coding-balanced": {"name": "Coding — Balanced"},
        "coding-economy": {"name": "Coding — Economy"}
      }
    }
  }
}
```

## Cline

Select **OpenAI Compatible** and set:

```text
Base URL: http://127.0.0.1:8787/v1
API Key: <MODELMUX_AUTH_TOKEN>
Model ID: coding-balanced
```

The enabled groups are returned by `GET /v1/models`, alongside physical models.

## Routing response headers

ModelMux adds these headers to upstream responses:

```text
X-ModelMux-Requested-Model
X-ModelMux-Selected-Model
X-ModelMux-Selected-Provider
X-ModelMux-Group
X-ModelMux-Attempt
```

They make it possible to diagnose which physical model served an agent request while keeping the agent configuration group-based.
