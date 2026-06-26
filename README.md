# ModelMux

ModelMux is a terminal-based LLM API key router built with Go.

## Quick Start

```bash
modelmux config init
export MIMO_KEY_1="your-key"
modelmux start
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
