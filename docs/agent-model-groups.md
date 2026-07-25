# Agent model groups

Model groups are virtual OpenAI-compatible models. A coding agent sends a normal request with a group ID in the `model` field, and ModelMux selects a compatible physical model, provider, and key.

```json
{
  "model": "coding-balanced",
  "messages": [{"role": "user", "content": "Refactor this package"}],
  "stream": true
}
```

The agent never needs to know which physical model or provider served the request.

## Prerequisites

Before configuring Cline or OpenCode:

1. Define and enable the physical models and keys used by the group.
2. Define at least one enabled model group.
3. Start or reload ModelMux.
4. Confirm the group appears in `GET /v1/models`.

When proxy authentication is enabled, set the token configured by `server.auth_token_env` before starting ModelMux:

```bash
export MODELMUX_AUTH_TOKEN="replace-with-a-strong-token"
modelmux start
```

The token given to Cline or OpenCode is the **ModelMux proxy token**. Provider credentials for OpenAI, Anthropic, Gemini, and other upstreams remain configured only in ModelMux.

## Configure model groups

The model IDs under `members` must match IDs declared in the top-level `models` configuration.

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
    strategy: failover
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

`failover` is the safest starting strategy for coding agents. Use `consistent_hash` when the client reliably sends a stable session identifier.

Before selection, ModelMux filters members against the request's chat/completions, streaming, tools, vision, and JSON-mode requirements. `required_capabilities` additionally guarantees that every enabled member provides the listed capabilities when the router starts.

## Reload and verify

After editing the configuration, restart ModelMux or call the admin reload endpoint.

List the physical models and virtual groups visible to clients:

```bash
curl http://127.0.0.1:8787/v1/models \
  -H "Authorization: Bearer $MODELMUX_AUTH_TOKEN"
```

The response should contain the group ID, for example `coding-balanced`.

Test the group directly before configuring an agent:

```bash
curl http://127.0.0.1:8787/v1/chat/completions \
  -H "Authorization: Bearer $MODELMUX_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "coding-balanced",
    "messages": [
      {"role": "user", "content": "Write a Go function that validates an email address"}
    ],
    "stream": false
  }'
```

## OpenCode

OpenCode supports custom OpenAI-compatible providers through `@ai-sdk/openai-compatible`.

Create or update `opencode.json` in the project directory:

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
        "coding-flagship": {
          "name": "Coding — Flagship",
          "limit": {
            "context": 200000,
            "output": 32000
          }
        },
        "coding-balanced": {
          "name": "Coding — Balanced",
          "limit": {
            "context": 128000,
            "output": 32000
          }
        },
        "coding-economy": {
          "name": "Coding — Economy",
          "limit": {
            "context": 64000,
            "output": 16000
          }
        }
      }
    }
  }
}
```

Export the proxy token and start OpenCode:

```bash
export MODELMUX_AUTH_TOKEN="replace-with-your-modelmux-token"
opencode
```

Inside OpenCode, run:

```text
/models
```

Select one of:

```text
modelmux/coding-flagship
modelmux/coding-balanced
modelmux/coding-economy
```

OpenCode uses the full identifier `provider_id/model_id`. In this configuration, `modelmux` is the provider ID and `coding-balanced` is the virtual model ID sent to ModelMux.

The `limit.context` and `limit.output` values should match the group's `context_window` and `max_output_tokens`. These limits help OpenCode manage its context budget; they do not override the upstream model's actual limits.

OpenCode also supports storing the credential interactively:

1. Run `/connect`.
2. Select **Other**.
3. Enter `modelmux` as the provider ID.
4. Enter the ModelMux proxy token.
5. Keep the provider and models declared in `opencode.json`.

The provider ID used by `/connect` must match the key under `provider` in `opencode.json`.

## Cline

Cline can connect to ModelMux through its **OpenAI Compatible** provider.

1. Open the Cline panel in VS Code.
2. Click the settings icon.
3. Set **API Provider** to **OpenAI Compatible**.
4. Enter these values:

```text
Base URL: http://127.0.0.1:8787/v1
API Key: <MODELMUX_AUTH_TOKEN>
Model ID: coding-balanced
```

5. In Cline's model configuration, set values matching the selected group:

```text
Context Window: 128000
Max Output Tokens: 32000
```

Enable image support only when every enabled member of the group supports vision. Coding groups should require `tools` because Cline relies on tool/function calling for agent actions.

Use **Verify** when available, or send a simple test prompt such as `Reply with OK` before starting a large task.

To switch quality or cost tier, change only the Model ID:

```text
coding-flagship
coding-balanced
coding-economy
```

Cline sends that ID in the normal OpenAI-compatible `model` field. ModelMux performs the physical-model, provider, and key selection.

## Local, Docker, and remote URLs

Use `127.0.0.1` only when the agent and ModelMux run on the same host.

When ModelMux is running on another server, use an HTTPS endpoint:

```text
https://modelmux.example.com/v1
```

For OpenCode:

```jsonc
"options": {
  "baseURL": "https://modelmux.example.com/v1",
  "apiKey": "{env:MODELMUX_AUTH_TOKEN}"
}
```

For Cline:

```text
Base URL: https://modelmux.example.com/v1
API Key: <MODELMUX_AUTH_TOKEN>
Model ID: coding-balanced
```

When the agent runs in a container, `127.0.0.1` refers to that container. Use the ModelMux Compose service name on a shared Docker network, or use the host gateway address when ModelMux runs on the host.

For any non-local deployment:

- enable proxy authentication;
- use HTTPS;
- do not expose admin endpoints without authentication;
- keep upstream provider keys only on the ModelMux server.

## Stable sessions

For `consistent_hash`, ModelMux uses the first available value from:

1. `X-ModelMux-Session-ID` header
2. `X-OpenAI-User` or `OpenAI-User` header
3. OpenAI request field `user`
4. `metadata.session_id`, `metadata.conversation_id`, or `metadata.thread_id`

Without a session value, `consistent_hash` falls back to priority order. When it is unclear whether an agent sends a stable identity, prefer `failover`.

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

Example:

```text
X-ModelMux-Requested-Model: coding-balanced
X-ModelMux-Selected-Model: claude-sonnet
X-ModelMux-Selected-Provider: anthropic
X-ModelMux-Group: coding-balanced
X-ModelMux-Attempt: 1
```

## Troubleshooting

### `401` or `403` authentication error

Confirm that the token supplied by Cline or OpenCode matches the environment variable named by `server.auth_token_env`. The agent should receive the ModelMux proxy token, not an upstream provider key.

### Model or group not found

Check that the exact group ID appears in:

```bash
curl http://127.0.0.1:8787/v1/models \
  -H "Authorization: Bearer $MODELMUX_AUTH_TOKEN"
```

OpenCode uses `modelmux/coding-balanced` in its own configuration, but sends `coding-balanced` to ModelMux. Cline should use only `coding-balanced` as its Model ID.

### `group_capability_mismatch`

No currently enabled group member supports all capabilities required by the request. Check the physical models' capability declarations, especially `tools`, `streaming`, `vision`, and `json_mode`.

### `all_group_models_unavailable`

The group exists, but no member can currently serve the request. Check whether models, providers, and keys are enabled, rate-limited, exhausted, invalid, or in cooldown.

### Tool calls do not work

Confirm that every coding-group member supports tool/function calling and declare:

```yaml
required_capabilities: [chat, streaming, tools]
```

### Connection refused or timeout

Confirm that ModelMux is running, the port is reachable, and the Base URL includes `/v1`. For remote deployments, check firewalls, reverse-proxy timeouts, TLS, and DNS.