# ModelMux

ModelMux is a terminal-based LLM API key router built with Go.

## Quick Start

```bash
modelmux config init
vim ~/.config/modelmux/config.yaml
modelmux start
```

Put API keys directly in `keys[].value`:

```yaml
keys:
  - id: "mimo-key-1"
    model_id: "mimo-v2.5-pro"
    value: "your-api-key"
```

Open the dashboard with:

```bash
modelmux tui
```

## Model Groups

Model groups let one request alias fan out across multiple provider models. Configure `model_groups`, then call the group ID directly:

```json
{
  "model": "high-price",
  "messages": []
}
```

ModelMux will try group members by priority, then rotate keys inside the selected member model.

## Chat Sessions

Chat sessions map a stable session ID to a model or model group.

```yaml
chat_sessions:
  - id: "chat-session-1"
    target: "high-price"
    enabled: true
  - id: "chat-session-2"
    target: "fast-lane"
    enabled: true
```

Call the session ID directly as the OpenAI `model`:

```json
{
  "model": "chat-session-1",
  "messages": []
}
```

ModelMux resolves the session to its target, then applies normal group/model routing and key rotation.
