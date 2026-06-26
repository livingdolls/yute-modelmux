# ModelMux

ModelMux is a terminal-based LLM API key router built with Go.

## Quick Start

```bash
modelmux config init
vim ~/.config/modelmux/config.yaml
modelmux start
```

Use environment variables for API keys instead of storing them in the config file:

```bash
export MIMO_KEY_1="your-api-key"
```

```yaml
keys:
  - id: "mimo-key-1"
    model_id: "mimo-v2.5-pro"
    value_env: "MIMO_KEY_1"
```

The `value` field is also supported for dev/local use, but `value_env` is recommended for security. When using `value_env`, the actual secret is never written to the YAML file when saving from TUI or CLI.

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

## TUI Config Editor

Open the `Config` page to manage providers, models, groups, and keys without editing YAML manually.

- `left/right`: switch config section
- `up/down`: select row
- `a`: add item
- `e`: edit item
- `d`: delete item with confirmation
- `space`: enable/disable item
- `s`: save config and reload router
- `r`: discard unsaved changes
