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

## TUI Chat Sessions

The TUI includes in-memory chat sessions. Open the `Chat` page, type a message, and press enter. Use `ctrl+n` for a new session and `ctrl+t` to switch the active session target between configured groups/models.
