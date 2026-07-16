import type { ReactNode } from "react";

export type DocTocItem = {
  id: string;
  label: string;
};

export type DocPage = {
  slug: string;
  href: string;
  section: string;
  title: string;
  description: string;
  toc: DocTocItem[];
  body: ReactNode;
  previous?: { href: string; title: string };
  next?: { href: string; title: string };
};

export const docNavigation = [
  {
    title: "Getting started",
    links: [
      { href: "/docs", label: "Overview" },
      { href: "/docs/installation", label: "Installation" },
      { href: "/docs/quick-start", label: "Quick start" },
      { href: "/docs/core-concepts", label: "Core concepts" },
    ],
  },
  {
    title: "Configuration",
    links: [
      { href: "/docs/configuration", label: "Configuration reference" },
      { href: "/docs/providers", label: "Providers" },
      { href: "/docs/routing", label: "Routing and failover" },
      { href: "/docs/rate-limiting", label: "Rate limiting" },
    ],
  },
  {
    title: "Operations",
    links: [
      { href: "/docs/storage", label: "Storage" },
      { href: "/docs/observability", label: "Observability" },
      { href: "/docs/security", label: "Security" },
      { href: "/docs/tui", label: "TUI dashboard" },
      { href: "/docs/ai-routing", label: "AI routing" },
    ],
  },
  {
    title: "Reference",
    links: [
      { href: "/docs/cli-reference", label: "CLI reference" },
      { href: "/docs/admin-api", label: "Admin API" },
      { href: "/docs/troubleshooting", label: "Troubleshooting" },
    ],
  },
];

function DocSection({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return (
    <section className="docs-section" id={id}>
      <h2><a href={`#${id}`}>{title}</a></h2>
      {children}
    </section>
  );
}

function CodeBlock({ label, children }: { label: string; children: string }) {
  return (
    <div className="code-block">
      <div className="code-header"><span>{label}</span></div>
      <pre><code>{children}</code></pre>
    </div>
  );
}

function Callout({ title, tone = "info", children }: { title: string; tone?: "info" | "warning"; children: ReactNode }) {
  return (
    <div className={`docs-callout docs-callout-${tone}`}>
      <strong>{title}</strong>
      <div>{children}</div>
    </div>
  );
}

function DocTable({ headers, rows }: { headers: string[]; rows: ReactNode[][] }) {
  return (
    <div className="docs-table-wrap">
      <table className="docs-table">
        <thead><tr>{headers.map((header) => <th key={header}>{header}</th>)}</tr></thead>
        <tbody>{rows.map((row, rowIndex) => <tr key={rowIndex}>{row.map((cell, cellIndex) => <td key={cellIndex}>{cell}</td>)}</tr>)}</tbody>
      </table>
    </div>
  );
}

function DocCards({ items }: { items: Array<{ href: string; title: string; description: string }> }) {
  return (
    <div className="docs-card-grid">
      {items.map((item) => (
        <a className="docs-card" href={item.href} key={item.href}>
          <strong>{item.title}</strong>
          <span>{item.description}</span>
        </a>
      ))}
    </div>
  );
}

const overview: DocPage = {
  slug: "overview",
  href: "/docs",
  section: "Getting started",
  title: "ModelMux documentation",
  description: "Run one OpenAI-compatible endpoint in front of multiple LLM providers and API keys, with routing, failover, limits, logs, and terminal-first operations.",
  toc: [
    { id: "what-is-modelmux", label: "What is ModelMux?" },
    { id: "request-flow", label: "Request flow" },
    { id: "use-cases", label: "When to use it" },
    { id: "documentation-map", label: "Documentation map" },
    { id: "production-checklist", label: "Production checklist" },
  ],
  body: (
    <>
      <Callout title="ModelMux in one sentence">
        <p>ModelMux is a lightweight reverse proxy that accepts OpenAI-compatible requests and routes them across configured providers, models, groups, and API keys.</p>
      </Callout>

      <DocSection id="what-is-modelmux" title="What is ModelMux?">
        <p>ModelMux sits between your application and upstream LLM APIs. Your application sends requests to one local or self-hosted endpoint. ModelMux then selects a model or group, chooses an eligible key, enforces limits, retries recoverable failures, and forwards the provider response back to the client.</p>
        <p>It runs as a single Go binary. Redis, message queues, Postgres, and an external control plane are not required. Runtime state can remain in memory for local use or be persisted with embedded SQLite.</p>
        <div className="docs-feature-list">
          <div><strong>Compatible endpoint</strong><span>Keep OpenAI-compatible clients and SDKs.</span></div>
          <div><strong>Key resilience</strong><span>Rotate keys and cool down unhealthy credentials.</span></div>
          <div><strong>Provider abstraction</strong><span>Route to OpenAI-compatible, Anthropic, Gemini, and custom upstreams.</span></div>
          <div><strong>Operational visibility</strong><span>Inspect logs, metrics, key state, route traces, and the TUI.</span></div>
        </div>
      </DocSection>

      <DocSection id="request-flow" title="Request flow">
        <div className="docs-flow">
          <span>Client</span><i>→</i><span>Model or group</span><i>→</i><span>Eligible key</span><i>→</i><span>Provider</span><i>→</i><span>Response</span>
        </div>
        <ol className="docs-list docs-list-numbered">
          <li>The client sends a request to <code>/v1/chat/completions</code> or another supported OpenAI-compatible route.</li>
          <li>ModelMux resolves the requested model ID. A model group may select one member before key selection begins.</li>
          <li>Disabled, invalid, cooling-down, quota-exhausted, or concurrency-limited keys are excluded.</li>
          <li>The configured routing strategy selects an eligible key.</li>
          <li>ModelMux applies provider conversion when Anthropic or Gemini is used.</li>
          <li>Recoverable failures may trigger retry and failover according to the retry and cooldown configuration.</li>
          <li>The response is streamed or returned normally, while usage, latency, status, and request metadata are recorded.</li>
        </ol>
      </DocSection>

      <DocSection id="use-cases" title="When to use ModelMux">
        <DocTable
          headers={["Situation", "How ModelMux helps"]}
          rows={[
            ["Multiple keys for one model", "Distributes or fails over traffic across credentials."],
            ["Provider rate limits", "Enforces local limits and temporarily cools down keys after upstream 429 responses."],
            ["Multiple LLM vendors", "Presents one OpenAI-compatible endpoint while adapting supported provider formats."],
            ["Self-hosted applications", "Keeps routing and credentials under your control without a hosted gateway dependency."],
            ["Terminal operations", "Provides CLI commands and a TUI for configuration, logs, metrics, chat, and key health."],
          ]}
        />
      </DocSection>

      <DocSection id="documentation-map" title="Documentation map">
        <DocCards items={[
          { href: "/docs/quick-start", title: "Quick start", description: "Install ModelMux and send the first proxied request." },
          { href: "/docs/configuration", title: "Configuration", description: "Understand every major YAML section and environment variable." },
          { href: "/docs/routing", title: "Routing and failover", description: "Configure key strategies, groups, retries, and cooldowns." },
          { href: "/docs/security", title: "Security", description: "Protect proxy traffic, admin endpoints, and stored secrets." },
          { href: "/docs/observability", title: "Observability", description: "Use metrics, logs, request IDs, and route traces." },
          { href: "/docs/troubleshooting", title: "Troubleshooting", description: "Diagnose exhausted keys, auth errors, storage locks, and bad config." },
        ]} />
      </DocSection>

      <DocSection id="production-checklist" title="Production checklist">
        <ul className="docs-checklist">
          <li>Bind to localhost unless remote access is required.</li>
          <li>Enable proxy authentication before exposing the service over a network.</li>
          <li>Keep <code>server.admin.require_auth</code> enabled.</li>
          <li>Use environment variables or the encrypted secret store for provider credentials.</li>
          <li>Enable SQLite when quotas, cooldown state, and request history must survive restarts.</li>
          <li>Set realistic model and key-level RPM, token, and concurrency limits.</li>
          <li>Monitor <code>/metrics</code>, request errors, cooldown counts, and active-key capacity.</li>
          <li>Back up both the SQLite database and <code>secrets.enc</code> when encrypted secrets are used.</li>
        </ul>
      </DocSection>
    </>
  ),
  next: { href: "/docs/installation", title: "Installation" },
};

const installation: DocPage = {
  slug: "installation",
  href: "/docs/installation",
  section: "Getting started",
  title: "Installation",
  description: "Install a prebuilt binary, install with Go, or build ModelMux from source.",
  toc: [
    { id: "prebuilt", label: "Prebuilt binaries" },
    { id: "go-install", label: "Install with Go" },
    { id: "source", label: "Build from source" },
    { id: "verify", label: "Verify installation" },
    { id: "paths", label: "Default paths" },
    { id: "upgrade", label: "Upgrade and remove" },
  ],
  body: (
    <>
      <DocSection id="prebuilt" title="Prebuilt binaries">
        <p>GitHub Releases provides archives for common Linux, macOS, and Windows targets. Download the archive matching your operating system and CPU architecture.</p>
        <DocTable headers={["Platform", "Release asset"]} rows={[
          ["Linux x86_64", <code key="1">modelmux_linux_amd64.tar.gz</code>],
          ["Linux arm64", <code key="2">modelmux_linux_arm64.tar.gz</code>],
          ["macOS Intel", <code key="3">modelmux_darwin_amd64.tar.gz</code>],
          ["macOS Apple Silicon", <code key="4">modelmux_darwin_arm64.tar.gz</code>],
          ["Windows x86_64", <code key="5">modelmux_windows_amd64.zip</code>],
          ["Windows arm64", <code key="6">modelmux_windows_arm64.zip</code>],
        ]} />
        <CodeBlock label="Linux x86_64">{`curl -L -o modelmux.tar.gz \\
  https://github.com/livingdolls/yute-modelmux/releases/latest/download/modelmux_linux_amd64.tar.gz

tar -xzf modelmux.tar.gz
sudo install modelmux_linux_amd64/modelmux /usr/local/bin/modelmux`}</CodeBlock>
        <p>Release archives include platform-specific binaries. Checksums are published with each release in <code>checksums.txt</code>.</p>
      </DocSection>

      <DocSection id="go-install" title="Install with Go">
        <p>Go 1.25 or newer is required when installing directly from source with the Go toolchain.</p>
        <CodeBlock label="Terminal">{`go install github.com/livingdolls/yute-modelmux/cmd/modelmux@latest`}</CodeBlock>
        <p>The binary is normally written to <code>$(go env GOPATH)/bin</code>. Make sure that directory is included in your <code>PATH</code>.</p>
        <CodeBlock label="Shell profile">{`export PATH="$(go env GOPATH)/bin:$PATH"`}</CodeBlock>
      </DocSection>

      <DocSection id="source" title="Build from source">
        <CodeBlock label="Terminal">{`git clone https://github.com/livingdolls/yute-modelmux.git
cd yute-modelmux
make build
./bin/modelmux --help`}</CodeBlock>
        <p>Building from source is useful when testing unreleased changes or contributing to the project. Release binaries are built with <code>CGO_ENABLED=0</code> for portability.</p>
      </DocSection>

      <DocSection id="verify" title="Verify the installation">
        <CodeBlock label="Terminal">{`modelmux version
modelmux --help`}</CodeBlock>
        <p>The version command prints the release version, commit, and build timestamp. A source installation may report development values when build-time metadata was not injected.</p>
      </DocSection>

      <DocSection id="paths" title="Default paths">
        <DocTable headers={["Purpose", "Default path"]} rows={[
          ["Configuration", <code key="1">~/.config/modelmux/config.yaml</code>],
          ["SQLite database", <code key="2">~/.local/share/modelmux/modelmux.db</code>],
          ["Encrypted secret store", <code key="3">~/.local/share/modelmux/secrets.enc</code>],
        ]} />
        <p>The database and secret store directories are created when the related features are used.</p>
      </DocSection>

      <DocSection id="upgrade" title="Upgrade and remove">
        <p>To upgrade a Go installation, run the same <code>go install ...@latest</code> command again. For a prebuilt installation, replace the existing binary with the binary from the newer release.</p>
        <p>Before upgrading production systems, keep a copy of the current binary and back up persistent state. To remove ModelMux, delete the binary. Remove the config and data directories only when their contents are no longer needed.</p>
      </DocSection>
    </>
  ),
  previous: { href: "/docs", title: "Overview" },
  next: { href: "/docs/quick-start", title: "Quick start" },
};

const quickStart: DocPage = {
  slug: "quick-start",
  href: "/docs/quick-start",
  section: "Getting started",
  title: "Quick start",
  description: "Create a minimal provider, model, and key configuration and send a request through ModelMux.",
  toc: [
    { id: "initialize", label: "Initialize config" },
    { id: "configure", label: "Configure a route" },
    { id: "validate", label: "Validate config" },
    { id: "start", label: "Start the proxy" },
    { id: "first-request", label: "Send a request" },
    { id: "sdk", label: "Connect an SDK" },
    { id: "inspect", label: "Inspect the router" },
  ],
  body: (
    <>
      <Callout title="Result">
        <p>You will run ModelMux on <code>127.0.0.1:8787</code> with one OpenAI-compatible upstream and one API key loaded from an environment variable.</p>
      </Callout>

      <DocSection id="initialize" title="1. Initialize the configuration">
        <CodeBlock label="Terminal">{`modelmux config init
${"${EDITOR:-vi}"} ~/.config/modelmux/config.yaml`}</CodeBlock>
        <p><code>config init</code> creates an example file at the default configuration path. Existing files should be reviewed before replacement.</p>
      </DocSection>

      <DocSection id="configure" title="2. Configure a provider, model, and key">
        <CodeBlock label="~/.config/modelmux/config.yaml">{`app:
  name: modelmux
  log_level: info

server:
  host: "127.0.0.1"
  port: 8787
  require_auth: false
  admin:
    require_auth: true

providers:
  - id: example
    name: Example Provider
    type: openai-compatible
    base_url: https://api.example.com/v1
    auth_type: bearer
    timeout_seconds: 120
    enabled: true

models:
  - id: example-chat
    provider_id: example
    model_name: upstream-model-name
    strategy: failover
    enabled: true

keys:
  - id: example-primary
    provider_id: example
    model_id: example-chat
    value_env: EXAMPLE_API_KEY
    status: active
    priority: 1

retry:
  max_retry_per_key: 1
  max_total_attempts: 3
  backoff_milliseconds: [300, 700, 1500]

cooldown:
  rate_limit_seconds: 300
  server_error_seconds: 60
  timeout_seconds: 60`}</CodeBlock>
        <Callout title="Do not commit plaintext credentials" tone="warning">
          <p>Use <code>value_env</code> or <code>secret_ref</code>. Plaintext <code>value</code> is intended only for controlled development environments.</p>
        </Callout>
      </DocSection>

      <DocSection id="validate" title="3. Validate the configuration">
        <CodeBlock label="Terminal">{`export EXAMPLE_API_KEY="your-provider-key"
modelmux config validate

# Optional: also contact configured providers
modelmux config validate --check-provider`}</CodeBlock>
        <p>Validation catches malformed YAML and invalid configuration relationships before the proxy starts. Use <code>--json</code> when validation output is consumed by scripts.</p>
      </DocSection>

      <DocSection id="start" title="4. Start the proxy">
        <CodeBlock label="Terminal">{`modelmux start`}</CodeBlock>
        <p>The service listens at <code>http://127.0.0.1:8787</code>. The OpenAI-compatible API base URL is <code>http://127.0.0.1:8787/v1</code>.</p>
      </DocSection>

      <DocSection id="first-request" title="5. Send the first request">
        <CodeBlock label="curl">{`curl http://127.0.0.1:8787/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "example-chat",
    "messages": [
      {"role": "user", "content": "Explain reliable LLM routing."}
    ]
  }'`}</CodeBlock>
        <p>The request uses the ModelMux model ID <code>example-chat</code>. ModelMux translates that ID to the configured upstream model name.</p>
      </DocSection>

      <DocSection id="sdk" title="6. Connect an OpenAI-compatible SDK">
        <CodeBlock label="TypeScript">{`import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "http://127.0.0.1:8787/v1",
  apiKey: "local-development-token",
});

const response = await client.chat.completions.create({
  model: "example-chat",
  messages: [{ role: "user", content: "Hello from ModelMux" }],
});`}</CodeBlock>
        <CodeBlock label="Python">{`from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:8787/v1",
    api_key="local-development-token",
)

response = client.chat.completions.create(
    model="example-chat",
    messages=[{"role": "user", "content": "Hello from ModelMux"}],
)`}</CodeBlock>
        <p>When <code>server.require_auth</code> is false, the client API key is not used by ModelMux. Once authentication is enabled, pass the configured ModelMux bearer token instead.</p>
      </DocSection>

      <DocSection id="inspect" title="7. Inspect the router">
        <CodeBlock label="Terminal">{`modelmux tui

# In another terminal
modelmux logs --limit 20
curl http://127.0.0.1:8787/metrics`}</CodeBlock>
        <p>The TUI exposes providers, models, groups, key health, logs, configuration, metrics, chat sessions, and AI diagnostics.</p>
      </DocSection>
    </>
  ),
  previous: { href: "/docs/installation", title: "Installation" },
  next: { href: "/docs/core-concepts", title: "Core concepts" },
};

const coreConcepts: DocPage = {
  slug: "core-concepts",
  href: "/docs/core-concepts",
  section: "Getting started",
  title: "Core concepts",
  description: "Understand providers, models, keys, groups, runtime state, and how they work together.",
  toc: [
    { id: "providers", label: "Providers" },
    { id: "models", label: "Models" },
    { id: "keys", label: "API keys" },
    { id: "groups", label: "Model groups" },
    { id: "state", label: "Runtime key state" },
    { id: "selection", label: "Selection hierarchy" },
  ],
  body: (
    <>
      <DocSection id="providers" title="Providers">
        <p>A provider describes an upstream API: its base URL, protocol type, authentication behavior, timeout, and enabled state. Multiple models and keys can reference one provider.</p>
        <p>Provider IDs are internal configuration identifiers. They should be stable and unique, such as <code>openai-prod</code>, <code>anthropic-team</code>, or <code>local-vllm</code>.</p>
      </DocSection>

      <DocSection id="models" title="Models">
        <p>A model is the public routing target used by clients. Its <code>id</code> is sent in the request, while <code>model_name</code> is forwarded to the upstream provider.</p>
        <CodeBlock label="config.yaml">{`models:
  - id: coding-fast
    provider_id: deepseek
    model_name: deepseek-coder
    strategy: least_error
    requests_per_minute: 120
    max_concurrent_requests: 10
    enabled: true`}</CodeBlock>
        <p>This separation lets applications depend on stable ModelMux IDs while upstream model names or providers change behind the gateway.</p>
      </DocSection>

      <DocSection id="keys" title="API keys">
        <p>A key belongs to one provider and model. It contains a credential reference plus routing and limit settings.</p>
        <DocTable headers={["Field", "Purpose"]} rows={[
          [<code key="1">priority</code>, "Ordering for failover selection. Lower values are considered first."],
          [<code key="2">status</code>, "Configured state, normally active or disabled."],
          [<code key="3">value_env</code>, "Environment variable containing the provider credential."],
          [<code key="4">secret_ref</code>, "Reference in the encrypted secret store."],
          [<code key="5">requests_per_minute</code>, "Maximum requests in the rolling minute window."],
          [<code key="6">tokens_per_minute</code>, "Maximum combined input and output tokens per minute."],
          [<code key="7">max_concurrent_requests</code>, "Maximum simultaneous requests using the key."],
          [<code key="8">daily_request_limit</code>, "Maximum requests per day."],
          [<code key="9">daily_token_limit</code>, "Maximum total tokens per day."],
        ]} />
      </DocSection>

      <DocSection id="groups" title="Model groups">
        <p>A model group is a client-visible alias that can route across several models or exact keys. Groups are useful for cross-provider failover, cost tiers, quality tiers, or workload aliases.</p>
        <CodeBlock label="config.yaml">{`model_groups:
  - id: production-chat
    name: Production Chat
    strategy: weighted
    enabled: true
    members:
      - model_id: primary-chat
        priority: 1
        weight: 3
        enabled: true
      - model_id: backup-chat
        priority: 2
        weight: 1
        enabled: true
      - key_id: emergency-key
        priority: 3
        weight: 1
        enabled: true`}</CodeBlock>
        <p>A <code>model_id</code> member keeps the selected model&apos;s normal key-pool behavior. A <code>key_id</code> member pins routing to one exact key.</p>
      </DocSection>

      <DocSection id="state" title="Runtime key state">
        <DocTable headers={["State", "Meaning"]} rows={[
          ["Active", "Eligible when limits and routing conditions allow."],
          ["Disabled", "Explicitly excluded by configuration or an admin action."],
          ["Cooldown", "Temporarily excluded after a rate limit, timeout, server error, or transient failure."],
          ["Invalid", "Excluded after authentication failures such as 401 or 403."],
          ["Limited", "Currently unavailable because a local quota or concurrency limit is exhausted."],
        ]} />
        <p>Background health checks can recover keys after transient failures. Invalid credentials require correcting the credential before they can become healthy.</p>
      </DocSection>

      <DocSection id="selection" title="Selection hierarchy">
        <ol className="docs-list docs-list-numbered">
          <li>Resolve the requested model ID or model-group ID.</li>
          <li>If a group was requested, select an enabled member using the group strategy.</li>
          <li>Build the candidate key pool for the selected model, or use the exact pinned key.</li>
          <li>Remove keys that are disabled, invalid, cooling down, or locally limited.</li>
          <li>Choose a candidate using the model&apos;s key-rotation strategy.</li>
          <li>Attempt the upstream request and apply retry/failover rules when necessary.</li>
        </ol>
      </DocSection>
    </>
  ),
  previous: { href: "/docs/quick-start", title: "Quick start" },
  next: { href: "/docs/configuration", title: "Configuration reference" },
};

const configuration: DocPage = {
  slug: "configuration",
  href: "/docs/configuration",
  section: "Configuration",
  title: "Configuration reference",
  description: "Configure the server, storage, providers, models, groups, keys, retries, cooldowns, health checks, and AI features.",
  toc: [
    { id: "location", label: "File location" },
    { id: "complete-example", label: "Complete example" },
    { id: "server", label: "Server" },
    { id: "storage", label: "Storage" },
    { id: "routing-sections", label: "Routing sections" },
    { id: "reliability", label: "Reliability settings" },
    { id: "secrets", label: "Credential sources" },
    { id: "validation", label: "Validation and reload" },
  ],
  body: (
    <>
      <DocSection id="location" title="File location">
        <p>The default configuration file is <code>~/.config/modelmux/config.yaml</code>. Create an example with <code>modelmux config init</code>. Use the global <code>--config</code> flag to point commands at another file.</p>
        <p>YAML is the source of truth for declarative routing. Runtime admin actions that enable or disable keys update both runtime state and the configuration file.</p>
      </DocSection>

      <DocSection id="complete-example" title="Complete example">
        <CodeBlock label="config.yaml">{`app:
  name: modelmux
  log_level: info

server:
  host: "127.0.0.1"
  port: 8787
  require_auth: true
  auth_token_env: MODELMUX_AUTH_TOKEN
  max_request_body_mb: 10
  admin:
    require_auth: true

storage:
  type: sqlite
  path: ~/.local/share/modelmux/modelmux.db

providers:
  - id: provider-a
    name: Provider A
    type: openai-compatible
    base_url: https://api.example.com/v1
    auth_type: bearer
    timeout_seconds: 120
    enabled: true

models:
  - id: chat-primary
    provider_id: provider-a
    model_name: upstream-chat-model
    strategy: failover
    enabled: true
    requests_per_minute: 120
    max_concurrent_requests: 10
    capabilities:
      tools: true
      json_mode: true

model_groups:
  - id: production-chat
    name: Production Chat
    strategy: weighted
    enabled: true
    members:
      - model_id: chat-primary
        priority: 1
        weight: 3
        enabled: true

keys:
  - id: provider-a-primary
    provider_id: provider-a
    model_id: chat-primary
    secret_ref: provider-a-primary
    status: active
    priority: 1
    requests_per_minute: 60
    tokens_per_minute: 30000
    max_concurrent_requests: 3
    daily_request_limit: 1000
    daily_token_limit: 500000

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

ai:
  enabled: false`}</CodeBlock>
      </DocSection>

      <DocSection id="server" title="Server settings">
        <DocTable headers={["Field", "Description"]} rows={[
          [<code key="1">server.host</code>, "Listening interface. Keep 127.0.0.1 for local-only access."],
          [<code key="2">server.port</code>, "HTTP listening port. The documented default is 8787."],
          [<code key="3">server.require_auth</code>, "Requires bearer authentication for proxy requests."],
          [<code key="4">server.auth_token_env</code>, "Environment variable holding the ModelMux bearer token."],
          [<code key="5">server.max_request_body_mb</code>, "Rejects request bodies above the configured size."],
          [<code key="6">server.admin.require_auth</code>, "Requires authentication for /admin/* endpoints. Keep enabled."],
        ]} />
      </DocSection>

      <DocSection id="storage" title="Storage settings">
        <DocTable headers={["Field", "Description"]} rows={[
          [<code key="1">storage.type</code>, "Leave empty for in-memory state or set to sqlite for persistence."],
          [<code key="2">storage.path</code>, "SQLite file path. Parent directories are created as needed."],
        ]} />
        <p>SQLite persists request logs, key runtime state, daily counters, quotas, cooldown timers, and metrics source data. Minute rolling windows remain in memory.</p>
      </DocSection>

      <DocSection id="routing-sections" title="Providers, models, groups, and keys">
        <p>These four sections form the routing graph:</p>
        <ul className="docs-list">
          <li><code>providers[]</code> defines upstream protocols and base URLs.</li>
          <li><code>models[]</code> defines client-visible model IDs and their provider mapping.</li>
          <li><code>model_groups[]</code> defines aliases that can select across models or exact keys.</li>
          <li><code>keys[]</code> defines credentials, priorities, state, and per-key limits.</li>
        </ul>
        <p>IDs must be unique within their respective section. References such as <code>provider_id</code>, <code>model_id</code>, and <code>key_id</code> must resolve to existing enabled configuration entries.</p>
      </DocSection>

      <DocSection id="reliability" title="Health checks, cooldowns, and retry">
        <DocTable headers={["Section", "Important fields"]} rows={[
          [<code key="1">health_check</code>, <><code>enabled</code>, <code>interval_seconds</code>, <code>timeout_seconds</code></>],
          [<code key="2">cooldown</code>, <><code>rate_limit_seconds</code>, <code>server_error_seconds</code>, <code>timeout_seconds</code></>],
          [<code key="3">retry</code>, <><code>max_retry_per_key</code>, <code>max_total_attempts</code>, <code>backoff_milliseconds</code></>],
        ]} />
        <p>Total attempts should be sized to the available key pool. Excessive retries increase latency and can multiply upstream load during provider incidents.</p>
      </DocSection>

      <DocSection id="secrets" title="Credential sources">
        <DocTable headers={["Key field", "Use"]} rows={[
          [<code key="1">value_env</code>, "Recommended for environment-managed credentials."],
          [<code key="2">secret_ref</code>, "Recommended when using the encrypted ModelMux secret store."],
          [<code key="3">value</code>, "Plaintext development fallback. Avoid in committed configuration."],
        ]} />
        <p>Encrypted secrets require <code>MODELMUX_MASTER_KEY</code>. The master key is not stored in the encrypted file and must be supplied whenever the store is opened.</p>
      </DocSection>

      <DocSection id="validation" title="Validation and reload">
        <CodeBlock label="Terminal">{`modelmux config validate
modelmux config validate --json
modelmux config validate --check-provider

# Reload a running instance through the protected admin API
curl -X POST http://127.0.0.1:8787/admin/reload \\
  -H "Authorization: Bearer $MODELMUX_AUTH_TOKEN"`}</CodeBlock>
        <p>Validate configuration before deployment. Provider checks perform network calls and therefore require credentials and upstream availability.</p>
      </DocSection>
    </>
  ),
  previous: { href: "/docs/core-concepts", title: "Core concepts" },
  next: { href: "/docs/providers", title: "Providers" },
};

const providers: DocPage = {
  slug: "providers",
  href: "/docs/providers",
  section: "Configuration",
  title: "Providers",
  description: "Configure OpenAI-compatible, Anthropic, Gemini, and custom upstream APIs.",
  toc: [
    { id: "types", label: "Provider types" },
    { id: "openai-compatible", label: "OpenAI-compatible" },
    { id: "anthropic", label: "Anthropic" },
    { id: "gemini", label: "Gemini" },
    { id: "custom", label: "Custom providers" },
    { id: "testing", label: "Testing providers" },
  ],
  body: (
    <>
      <DocSection id="types" title="Provider types">
        <DocTable headers={["Type", "Supported behavior", "Conversion"]} rows={[
          [<code key="1">openai-compatible</code>, "Chat, completions, streaming, and tools supported by the upstream", "Direct pass-through"],
          [<code key="2">anthropic</code>, "Chat and streaming", "OpenAI request/response conversion"],
          [<code key="3">gemini</code>, "Chat and streaming", "OpenAI request/response conversion"],
          [<code key="4">custom</code>, "Same routing behavior as OpenAI-compatible", "Direct pass-through"],
        ]} />
        <p>Conversion provides a common client-facing protocol, but provider-specific features still depend on what the upstream API supports.</p>
      </DocSection>

      <DocSection id="openai-compatible" title="OpenAI-compatible providers">
        <CodeBlock label="config.yaml">{`providers:
  - id: local-vllm
    name: Local vLLM
    type: openai-compatible
    base_url: http://127.0.0.1:8000/v1
    auth_type: bearer
    timeout_seconds: 120
    enabled: true`}</CodeBlock>
        <p>This type is suitable for OpenAI-style hosted APIs, local inference servers, and gateways that expose compatible routes. ModelMux forwards supported request structures without provider conversion.</p>
      </DocSection>

      <DocSection id="anthropic" title="Anthropic providers">
        <CodeBlock label="config.yaml">{`providers:
  - id: anthropic
    name: Anthropic
    type: anthropic
    base_url: https://api.anthropic.com
    auth_type: bearer
    timeout_seconds: 120
    enabled: true`}</CodeBlock>
        <p>ModelMux accepts the OpenAI-compatible client request and converts supported chat and streaming data for Anthropic. Use a model entry whose <code>model_name</code> matches the upstream Anthropic model identifier.</p>
      </DocSection>

      <DocSection id="gemini" title="Gemini providers">
        <CodeBlock label="config.yaml">{`providers:
  - id: gemini
    name: Google Gemini
    type: gemini
    base_url: https://generativelanguage.googleapis.com
    auth_type: bearer
    timeout_seconds: 120
    enabled: true`}</CodeBlock>
        <p>Gemini chat and streaming responses are converted to the compatible response shape. Test the exact endpoint and authentication behavior required by the configured Gemini API environment.</p>
      </DocSection>

      <DocSection id="custom" title="Custom providers">
        <p>Use <code>custom</code> when the upstream follows the same request and response conventions as an OpenAI-compatible service but should be identified separately in configuration, logs, or operations.</p>
        <p>ModelMux does not make an incompatible API compatible merely by selecting <code>custom</code>. The upstream still needs to understand the forwarded request format.</p>
      </DocSection>

      <DocSection id="testing" title="Testing providers and keys">
        <CodeBlock label="Terminal">{`modelmux config validate --check-provider
modelmux key test --id provider-a-primary`}</CodeBlock>
        <p>A key test verifies connectivity with the key&apos;s configured provider. Authentication failures normally move a key toward an invalid state, while transient provider failures trigger cooldown behavior.</p>
      </DocSection>
    </>
  ),
  previous: { href: "/docs/configuration", title: "Configuration reference" },
  next: { href: "/docs/routing", title: "Routing and failover" },
};

const routing: DocPage = {
  slug: "routing",
  href: "/docs/routing",
  section: "Configuration",
  title: "Routing and failover",
  description: "Choose keys and group members with failover, round-robin, least-error, least-used, and weighted strategies.",
  toc: [
    { id: "key-strategies", label: "Key strategies" },
    { id: "groups", label: "Group strategies" },
    { id: "eligibility", label: "Key eligibility" },
    { id: "retry", label: "Retry behavior" },
    { id: "cooldown", label: "Cooldown behavior" },
    { id: "patterns", label: "Routing patterns" },
  ],
  body: (
    <>
      <DocSection id="key-strategies" title="Key-rotation strategies">
        <DocTable headers={["Strategy", "Selection behavior", "Good fit"]} rows={[
          [<code key="1">failover</code>, "Uses the lowest-priority eligible key first.", "Primary/backup credentials."],
          [<code key="2">round_robin</code>, "Distributes requests across eligible keys in sequence.", "Even traffic distribution."],
          [<code key="3">least_error</code>, "Chooses the eligible key with the fewest recorded errors.", "Pools with uneven reliability."],
          [<code key="4">least_used</code>, "Chooses the key with the lowest daily usage.", "Balancing daily quotas."],
        ]} />
        <p>The strategy is configured on the model. It operates after unavailable keys have been filtered out.</p>
      </DocSection>

      <DocSection id="groups" title="Model-group strategies">
        <DocTable headers={["Strategy", "Behavior"]} rows={[
          [<code key="1">failover</code>, "Selects members by priority, using later members when earlier members are unavailable."],
          [<code key="2">round_robin</code>, "Cycles across enabled group members."],
          [<code key="3">weighted</code>, "Distributes selection according to each member's weight."],
        ]} />
        <p>Group selection happens before normal per-model key selection. A member referencing <code>key_id</code> bypasses the model pool and selects that exact key.</p>
      </DocSection>

      <DocSection id="eligibility" title="Key eligibility">
        <p>A configured key is eligible only when all relevant conditions pass:</p>
        <ul className="docs-checklist">
          <li>The key, provider, and model are enabled.</li>
          <li>The key is not explicitly disabled or marked invalid.</li>
          <li>No active cooldown is blocking it.</li>
          <li>Per-minute request and token limits have capacity.</li>
          <li>Daily request and token quotas have capacity.</li>
          <li>The concurrency limit has an available slot.</li>
        </ul>
        <p>When all keys are ineligible, ModelMux returns <code>429</code> rather than forwarding a request that cannot be served safely.</p>
      </DocSection>

      <DocSection id="retry" title="Retry behavior">
        <CodeBlock label="config.yaml">{`retry:
  max_retry_per_key: 1
  max_total_attempts: 5
  backoff_milliseconds: [300, 700, 1500]`}</CodeBlock>
        <p><code>max_retry_per_key</code> limits repeated attempts against the same credential. <code>max_total_attempts</code> caps attempts across the complete route. Backoff values reduce immediate retry pressure during transient incidents.</p>
        <Callout title="Avoid retry storms" tone="warning">
          <p>Application-level retries and ModelMux retries can multiply each other. Keep both layers bounded and use jitter or backoff in the client.</p>
        </Callout>
      </DocSection>

      <DocSection id="cooldown" title="Cooldown behavior">
        <CodeBlock label="config.yaml">{`cooldown:
  rate_limit_seconds: 300
  server_error_seconds: 60
  timeout_seconds: 60`}</CodeBlock>
        <p>Rate-limit responses typically need longer cooldowns than isolated network or server failures. During cooldown, the key remains configured but is removed from candidate selection.</p>
        <p>When health checks are enabled, recovered keys can return to active routing automatically.</p>
      </DocSection>

      <DocSection id="patterns" title="Common routing patterns">
        <DocCards items={[
          { href: "#patterns", title: "Primary and backup", description: "Use failover with priorities 1, 2, and 3." },
          { href: "#patterns", title: "Shared team keys", description: "Use round-robin plus per-key RPM and concurrency caps." },
          { href: "#patterns", title: "Cross-provider alias", description: "Use a group whose members reference several models." },
          { href: "#patterns", title: "Quota balancing", description: "Use least-used with daily request or token limits." },
        ]} />
      </DocSection>
    </>
  ),
  previous: { href: "/docs/providers", title: "Providers" },
  next: { href: "/docs/rate-limiting", title: "Rate limiting" },
};

const rateLimiting: DocPage = {
  slug: "rate-limiting",
  href: "/docs/rate-limiting",
  section: "Configuration",
  title: "Rate limiting",
  description: "Protect models and credentials with RPM, token, concurrency, and daily quota limits.",
  toc: [
    { id: "levels", label: "Limit levels" },
    { id: "order", label: "Evaluation order" },
    { id: "configuration", label: "Configuration example" },
    { id: "exhaustion", label: "Exhaustion behavior" },
    { id: "restart", label: "Restart semantics" },
    { id: "capacity", label: "Capacity planning" },
  ],
  body: (
    <>
      <DocSection id="levels" title="Limit levels">
        <DocTable headers={["Level", "Configuration", "Scope"]} rows={[
          ["Model RPM", <code key="1">models[].requests_per_minute</code>, "All traffic targeting one model."],
          ["Model concurrency", <code key="2">models[].max_concurrent_requests</code>, "Simultaneous requests for one model."],
          ["Key RPM", <code key="3">keys[].requests_per_minute</code>, "Requests using one credential."],
          ["Key tokens/minute", <code key="4">keys[].tokens_per_minute</code>, "Combined input and output tokens for one key."],
          ["Key concurrency", <code key="5">keys[].max_concurrent_requests</code>, "Simultaneous requests using one key."],
          ["Daily request quota", <code key="6">keys[].daily_request_limit</code>, "Requests per key per day."],
          ["Daily token quota", <code key="7">keys[].daily_token_limit</code>, "Tokens per key per day."],
        ]} />
      </DocSection>

      <DocSection id="order" title="Evaluation order">
        <ol className="docs-list docs-list-numbered">
          <li>Model-wide RPM capacity is checked.</li>
          <li>Model-wide concurrency capacity is checked.</li>
          <li>The router filters key candidates using per-key minute, daily, and concurrency limits.</li>
          <li>The routing strategy selects one of the remaining candidates.</li>
          <li>Usage is updated from the request and provider response.</li>
        </ol>
        <p>Model-level limits protect the complete route. Key-level limits protect individual provider credentials.</p>
      </DocSection>

      <DocSection id="configuration" title="Configuration example">
        <CodeBlock label="config.yaml">{`models:
  - id: production-chat
    provider_id: provider-a
    model_name: upstream-chat
    strategy: round_robin
    requests_per_minute: 200
    max_concurrent_requests: 20
    enabled: true

keys:
  - id: key-a
    provider_id: provider-a
    model_id: production-chat
    value_env: PROVIDER_KEY_A
    requests_per_minute: 60
    tokens_per_minute: 30000
    max_concurrent_requests: 4
    daily_request_limit: 5000
    daily_token_limit: 2000000`}</CodeBlock>
      </DocSection>

      <DocSection id="exhaustion" title="When capacity is exhausted">
        <p>An exhausted key is skipped while another eligible key exists. When the complete model or group has no usable capacity, ModelMux returns HTTP <code>429</code>.</p>
        <p>A local <code>429</code> can therefore mean either the model-wide limit is full or every candidate key is unavailable, limited, disabled, invalid, or cooling down. Inspect key state and recent logs to identify the exact cause.</p>
      </DocSection>

      <DocSection id="restart" title="Restart semantics">
        <p>Minute windows are maintained in memory and reset when the process restarts. Daily counters, key runtime state, quotas, and cooldowns can survive restarts when SQLite storage is enabled.</p>
        <p>Do not use process restarts as a quota-reset mechanism. Provider-side limits remain in effect regardless of local state.</p>
      </DocSection>

      <DocSection id="capacity" title="Capacity planning">
        <ul className="docs-list">
          <li>Set local limits below provider-advertised limits when upstream enforcement is strict or bursty.</li>
          <li>Reserve concurrency for health checks and operational tests.</li>
          <li>Use token limits for workloads with highly variable prompt and response sizes.</li>
          <li>Use model-wide limits to prevent a large key pool from exceeding application or infrastructure capacity.</li>
          <li>Monitor cooldown frequency. Repeated cooldowns usually indicate local limits are too high or upstream capacity is insufficient.</li>
        </ul>
      </DocSection>
    </>
  ),
  previous: { href: "/docs/routing", title: "Routing and failover" },
  next: { href: "/docs/storage", title: "Storage" },
};

const storage: DocPage = {
  slug: "storage",
  href: "/docs/storage",
  section: "Operations",
  title: "Storage",
  description: "Choose in-memory operation or persist logs, counters, quotas, cooldowns, and key state with SQLite.",
  toc: [
    { id: "modes", label: "Storage modes" },
    { id: "sqlite", label: "Enable SQLite" },
    { id: "persisted", label: "Persisted data" },
    { id: "files", label: "Database files" },
    { id: "backup", label: "Backup and restore" },
    { id: "deployment", label: "Deployment constraints" },
  ],
  body: (
    <>
      <DocSection id="modes" title="Storage modes">
        <DocTable headers={["Mode", "Behavior", "Recommended for"]} rows={[
          ["In memory", "State resets on restart; the recent-log buffer keeps the latest 200 entries.", "Local development and temporary tests."],
          ["SQLite", "Persists operational state and request history in one embedded database.", "Long-running single-instance deployments."],
        ]} />
      </DocSection>

      <DocSection id="sqlite" title="Enable SQLite">
        <CodeBlock label="config.yaml">{`storage:
  type: sqlite
  path: ~/.local/share/modelmux/modelmux.db`}</CodeBlock>
        <p>The schema is migrated automatically at startup. SQLite is opened in WAL mode for safer concurrent reads and writes inside the process.</p>
      </DocSection>

      <DocSection id="persisted" title="Persisted data">
        <ul className="docs-checklist">
          <li>Key status, usage, cooldown timers, and runtime state.</li>
          <li>Daily request and token counters used by configured quotas.</li>
          <li>Request logs with group, model, provider, key, status, latency, tokens, and timestamp.</li>
          <li>Data queried by <code>modelmux logs</code> and the <code>/logs</code> endpoint.</li>
          <li>Metrics source data derived from stored request history.</li>
        </ul>
      </DocSection>

      <DocSection id="files" title="Database and secret files">
        <CodeBlock label="Default data directory">{`~/.local/share/modelmux/modelmux.db
~/.local/share/modelmux/modelmux.db-wal
~/.local/share/modelmux/modelmux.db-shm
~/.local/share/modelmux/secrets.enc`}</CodeBlock>
        <p>The <code>-wal</code> and <code>-shm</code> files are normal SQLite WAL files. Do not delete them while ModelMux is running.</p>
      </DocSection>

      <DocSection id="backup" title="Backup and restore">
        <p>Stop ModelMux or use a consistent SQLite backup method before copying a live database. A simple stopped-service backup is:</p>
        <CodeBlock label="Terminal">{`cp ~/.local/share/modelmux/modelmux.db ~/modelmux.db.backup
cp ~/.local/share/modelmux/secrets.enc ~/modelmux-secrets.enc.backup`}</CodeBlock>
        <Callout title="Keep the master key" tone="warning">
          <p>A copied <code>secrets.enc</code> file cannot be decrypted without the matching <code>MODELMUX_MASTER_KEY</code>.</p>
        </Callout>
      </DocSection>

      <DocSection id="deployment" title="Deployment constraints">
        <p>SQLite storage is designed for one ModelMux process using one database file. Do not share the same SQLite path across several running ModelMux instances or network-mounted replicas.</p>
        <p>For container deployment, mount the data directory on persistent storage. Ensure the process user can create the database, WAL files, and encrypted secret file.</p>
      </DocSection>
    </>
  ),
  previous: { href: "/docs/rate-limiting", title: "Rate limiting" },
  next: { href: "/docs/observability", title: "Observability" },
};

const observability: DocPage = {
  slug: "observability",
  href: "/docs/observability",
  section: "Operations",
  title: "Observability",
  description: "Inspect metrics, structured logs, request IDs, key state, and stored request history.",
  toc: [
    { id: "request-ids", label: "Request IDs" },
    { id: "metrics", label: "Metrics" },
    { id: "logs", label: "Request logs" },
    { id: "queries", label: "Log queries" },
    { id: "monitoring", label: "What to monitor" },
  ],
  body: (
    <>
      <DocSection id="request-ids" title="Request IDs and structured logging">
        <p>Every proxied response includes an <code>X-ModelMux-Request-ID</code> header. Use it to correlate client failures with ModelMux logs and, when available, AI route traces.</p>
        <p>Application logs are emitted with Go&apos;s structured <code>log/slog</code> package and include request method, path, remote address, and latency.</p>
      </DocSection>

      <DocSection id="metrics" title="Metrics endpoints">
        <CodeBlock label="HTTP">{`GET /metrics
GET /metrics?format=prometheus`}</CodeBlock>
        <p>The default response is JSON. Add <code>format=prometheus</code> for Prometheus exposition format.</p>
        <DocTable headers={["Metric", "Meaning"]} rows={[
          [<code key="1">modelmux_requests_total</code>, "Requests by model, key, and group."],
          [<code key="2">modelmux_errors_total</code>, "Observed error counts."],
          [<code key="3">modelmux_rate_limits_total</code>, "Local or upstream rate-limit events."],
          [<code key="4">modelmux_latency_ms</code>, "Latency histogram with predefined millisecond buckets."],
          [<code key="5">modelmux_status_total</code>, "Response status classes such as 2xx, 4xx, and 5xx."],
          [<code key="6">modelmux_active_keys</code>, "Currently active-key gauge."],
          [<code key="7">cooldown_keys</code>, "Keys currently cooling down."],
          [<code key="8">invalid_keys</code>, "Keys marked invalid."],
          [<code key="9">limited_keys</code>, "Keys unavailable due to local limits."],
        ]} />
      </DocSection>

      <DocSection id="logs" title="Request logs">
        <p>Request history can include the selected group, model, provider, key, status code, latency, input/output token counts, estimated cost when available, and timestamp.</p>
        <p>In-memory mode retains a bounded recent buffer. SQLite mode keeps persistent history and allows later queries through the CLI and HTTP endpoint.</p>
      </DocSection>

      <DocSection id="queries" title="Query request history">
        <CodeBlock label="CLI">{`modelmux logs --limit 50
modelmux logs --model-id example-chat
modelmux logs --status-code 429
modelmux logs --json --limit 100`}</CodeBlock>
        <CodeBlock label="HTTP">{`GET /logs?limit=50
GET /logs?model_id=example-chat
GET /logs?status_code=429`}</CodeBlock>
        <p>JSON CLI output is useful for scripts, incident reports, and ingestion into other tooling.</p>
      </DocSection>

      <DocSection id="monitoring" title="What to monitor">
        <ul className="docs-list">
          <li><strong>Error and status rate:</strong> watch 429 and 5xx growth by model and provider.</li>
          <li><strong>Latency:</strong> alert on sustained p95 or p99 changes, not isolated slow requests.</li>
          <li><strong>Key capacity:</strong> monitor active, cooldown, invalid, and limited key counts.</li>
          <li><strong>Quota headroom:</strong> inspect daily request and token consumption before exhaustion.</li>
          <li><strong>Failover frequency:</strong> repeated failover can indicate provider degradation or incorrect local limits.</li>
          <li><strong>Storage growth:</strong> manage request-log retention and database size in long-running deployments.</li>
        </ul>
      </DocSection>
    </>
  ),
  previous: { href: "/docs/storage", title: "Storage" },
  next: { href: "/docs/security", title: "Security" },
};

const security: DocPage = {
  slug: "security",
  href: "/docs/security",
  section: "Operations",
  title: "Security",
  description: "Secure proxy access, admin endpoints, provider credentials, the secret store, and reverse-proxy deployments.",
  toc: [
    { id: "network", label: "Network exposure" },
    { id: "proxy-auth", label: "Proxy authentication" },
    { id: "admin-auth", label: "Admin authentication" },
    { id: "secrets", label: "Secret storage" },
    { id: "reverse-proxy", label: "Reverse proxies" },
    { id: "checklist", label: "Security checklist" },
  ],
  body: (
    <>
      <DocSection id="network" title="Network exposure">
        <p>The safest default is binding ModelMux to <code>127.0.0.1</code>. Bind to a broader interface only when another machine or container must connect.</p>
        <Callout title="Authentication is required for network exposure" tone="warning">
          <p>Do not expose an unauthenticated ModelMux endpoint to an untrusted network. Anyone with access could spend provider quota and inspect operational endpoints.</p>
        </Callout>
      </DocSection>

      <DocSection id="proxy-auth" title="Proxy authentication">
        <CodeBlock label="config.yaml">{`server:
  host: "0.0.0.0"
  port: 8787
  require_auth: true
  auth_token_env: MODELMUX_AUTH_TOKEN`}</CodeBlock>
        <CodeBlock label="Terminal">{`export MODELMUX_AUTH_TOKEN="replace-with-a-long-random-token"
modelmux start`}</CodeBlock>
        <p>Clients authenticate with <code>Authorization: Bearer &lt;token&gt;</code>. Rotate the token through your secret-management process and restart or reload the service as required by your deployment.</p>
      </DocSection>

      <DocSection id="admin-auth" title="Admin endpoint authentication">
        <p><code>/admin/*</code> is protected separately by <code>server.admin.require_auth</code>. Keep this setting enabled even when the proxy is local.</p>
        <p>Admin endpoints can reload configuration, change key state, and test provider credentials. They should be treated as privileged operational controls.</p>
      </DocSection>

      <DocSection id="secrets" title="Provider credentials and encrypted secrets">
        <p>Provider keys are never written to request logs or metrics. Prefer environment variables or the encrypted store:</p>
        <CodeBlock label="Terminal">{`export MODELMUX_MASTER_KEY="a-strong-master-password"
modelmux secret set --ref provider-a-primary
# Enter value through the hidden prompt`}</CodeBlock>
        <CodeBlock label="config.yaml">{`keys:
  - id: provider-a-primary
    provider_id: provider-a
    model_id: production-chat
    secret_ref: provider-a-primary`}</CodeBlock>
        <p>The encrypted store uses AES-256-GCM. Its encryption key is derived with Argon2id using a per-file salt. The documented derivation uses 64 MiB memory and three iterations.</p>
      </DocSection>

      <DocSection id="reverse-proxy" title="Reverse-proxy deployments">
        <p>Do not assume localhost checks remain safe behind a reverse proxy. Proxied traffic can appear to ModelMux as local traffic. Keep ModelMux authentication enabled and enforce TLS and access controls at the edge.</p>
        <ul className="docs-list">
          <li>Do not publish <code>/admin/*</code> without authentication.</li>
          <li>Restrict <code>/metrics</code> and <code>/logs</code> when they contain sensitive operational data.</li>
          <li>Preserve or generate request correlation headers intentionally.</li>
          <li>Set request body limits at both the edge and ModelMux.</li>
          <li>Use HTTPS between clients and the reverse proxy.</li>
        </ul>
      </DocSection>

      <DocSection id="checklist" title="Security checklist">
        <ul className="docs-checklist">
          <li>Keep ModelMux and its dependencies updated.</li>
          <li>Use a dedicated operating-system user with minimal file permissions.</li>
          <li>Restrict read access to config, SQLite, and <code>secrets.enc</code>.</li>
          <li>Never commit provider keys or <code>MODELMUX_MASTER_KEY</code>.</li>
          <li>Use different ModelMux bearer tokens for different environments.</li>
          <li>Back up encrypted secrets and store the matching master key separately.</li>
          <li>Review logs and metrics before sharing them outside the operations team.</li>
        </ul>
      </DocSection>
    </>
  ),
  previous: { href: "/docs/observability", title: "Observability" },
  next: { href: "/docs/tui", title: "TUI dashboard" },
};

const tui: DocPage = {
  slug: "tui",
  href: "/docs/tui",
  section: "Operations",
  title: "TUI dashboard",
  description: "Operate providers, models, groups, chat, keys, logs, configuration, metrics, and AI diagnostics from the terminal.",
  toc: [
    { id: "start", label: "Start the TUI" },
    { id: "pages", label: "Dashboard pages" },
    { id: "navigation", label: "Global navigation" },
    { id: "chat", label: "Chat shortcuts" },
    { id: "config", label: "Config shortcuts" },
    { id: "operations", label: "Operational shortcuts" },
  ],
  body: (
    <>
      <DocSection id="start" title="Start the TUI">
        <CodeBlock label="Terminal">{`modelmux tui`}</CodeBlock>
        <p>The TUI reads the same configuration as the proxy and provides a terminal-native operational interface. Some actions update configuration and reload the router.</p>
      </DocSection>

      <DocSection id="pages" title="Dashboard pages">
        <DocTable headers={["Page", "Purpose"]} rows={[
          ["Providers", "Inspect upstream routes and provider state."],
          ["Models", "Inspect routable model IDs and strategy settings."],
          ["Groups", "Inspect aliases and their model/key members."],
          ["Chat", "Run in-memory prompt sessions against configured models or groups."],
          ["Keys", "Inspect live key state, cooldowns, usage, and errors."],
          ["Logs", "View and filter recent request history."],
          ["Config", "Edit the YAML-backed configuration through forms and selectors."],
          ["AI", "Inspect classification, guardrails, route diagnostics, and related tools."],
        ]} />
      </DocSection>

      <DocSection id="navigation" title="Global navigation">
        <DocTable headers={["Shortcut", "Action"]} rows={[
          [<kbd key="1">tab / shift+tab</kbd>, "Move between main pages."],
          [<kbd key="2">enter</kbd>, "Open the selected page or item."],
          [<kbd key="3">esc</kbd>, "Cancel input, close a dialog, or return to the previous level."],
          [<kbd key="4">?</kbd>, "Toggle help when not typing."],
          [<kbd key="5">q</kbd>, "Quit when not typing."],
          [<kbd key="6">ctrl+c</kbd>, "Force quit."],
          [<kbd key="7">t</kbd>, "Cycle the active color theme."],
          [<kbd key="8">T</kbd>, "Open the theme picker."],
        ]} />
      </DocSection>

      <DocSection id="chat" title="Chat shortcuts">
        <DocTable headers={["Shortcut", "Action"]} rows={[
          [<kbd key="1">enter</kbd>, "Open a selected session or send the current message."],
          [<kbd key="2">up / down</kbd>, "Select a chat session."],
          [<kbd key="3">pgup / pgdown</kbd>, "Scroll conversation history."],
          [<kbd key="4">ctrl+n</kbd>, "Create a new chat session."],
          [<kbd key="5">ctrl+t</kbd>, "Switch the target model or group."],
          [<kbd key="6">ctrl+f</kbd>, "Filter sessions."],
          [<kbd key="7">esc</kbd>, "Clear input or return to the session list/menu."],
        ]} />
      </DocSection>

      <DocSection id="config" title="Configuration shortcuts">
        <DocTable headers={["Shortcut", "Action"]} rows={[
          [<kbd key="1">up / down</kbd>, "Select a configuration row."],
          [<kbd key="2">left / right</kbd>, "Switch sections or open a selectable field."],
          [<kbd key="3">enter</kbd>, "Edit the selected item."],
          [<kbd key="4">space</kbd>, "Toggle an item in a multi-select popup."],
          [<kbd key="5">a</kbd>, "Add an item."],
          [<kbd key="6">delete</kbd>, "Delete the selected item."],
          [<kbd key="7">ctrl+s</kbd>, "Save configuration and reload the router."],
          [<kbd key="8">ctrl+r</kbd>, "Reload the draft configuration."],
          [<kbd key="9">/ or ctrl+f</kbd>, "Filter configuration rows."],
        ]} />
      </DocSection>

      <DocSection id="operations" title="Operational shortcuts">
        <DocTable headers={["Area", "Shortcut", "Action"]} rows={[
          ["Keys", <kbd key="1">1 / 2 / 3</kbd>, "Sort by status, cooldown, or errors."],
          ["Keys", <kbd key="2">x</kbd>, "Test a key by ID."],
          ["Logs", <kbd key="3">1–6</kbd>, "Change log filter or sort order."],
        ]} />
        <p>Single-letter navigation is disabled while typing into text fields so normal text entry is not intercepted.</p>
      </DocSection>
    </>
  ),
  previous: { href: "/docs/security", title: "Security" },
  next: { href: "/docs/ai-routing", title: "AI routing" },
};

const aiRouting: DocPage = {
  slug: "ai-routing",
  href: "/docs/ai-routing",
  section: "Operations",
  title: "AI routing and diagnostics",
  description: "Classify requests, apply guardrails, trace routing decisions, and select models with policy rules.",
  toc: [
    { id: "enable", label: "Enable AI features" },
    { id: "classifier", label: "Classifier" },
    { id: "guardrails", label: "Guardrails" },
    { id: "trace", label: "Route traces" },
    { id: "policy", label: "Policy routing" },
    { id: "diagnostics", label: "CLI diagnostics" },
  ],
  body: (
    <>
      <Callout title="Disabled by default">
        <p>AI features default to off for backward compatibility. Enable only the components needed by your deployment.</p>
      </Callout>

      <DocSection id="enable" title="Enable AI features">
        <CodeBlock label="config.yaml">{`ai:
  enabled: true
  classifier:
    enabled: true
    mode: heuristic
  guardrails:
    enabled: true
  route_trace:
    enabled: true
    include_response_header: true
  routing_rules:
    - when:
        task: coding
      use_model: deepseek-coder`}</CodeBlock>
      </DocSection>

      <DocSection id="classifier" title="Request classifier">
        <p>The heuristic classifier can identify task categories including coding, reasoning, summarization, translation, JSON extraction, tool use, vision, and long-context requests.</p>
        <p>Classification is designed for routing diagnostics and policy decisions. Review behavior against your own prompts before relying on it for production-critical selection.</p>
      </DocSection>

      <DocSection id="guardrails" title="Guardrails">
        <p>Built-in guardrails can reject prompts containing common API-key patterns such as <code>sk-</code>, <code>sk-ant-</code>, <code>AIza</code>, and <code>Bearer</code>. They can also enforce a configured maximum prompt size.</p>
        <p>These checks reduce accidental credential leakage but do not replace a complete data-loss-prevention or content-safety system.</p>
      </DocSection>

      <DocSection id="trace" title="Route traces">
        <p>Route tracing records classification and routing decisions. With SQLite enabled, traces are persisted and can be inspected through the admin API.</p>
        <CodeBlock label="HTTP">{`X-ModelMux-Route-Trace-ID: <trace-id>
GET /admin/traces/<trace-id>`}</CodeBlock>
        <p>Use route trace IDs with the normal <code>X-ModelMux-Request-ID</code> when debugging unexpected model selection.</p>
      </DocSection>

      <DocSection id="policy" title="Policy routing">
        <p>Routing rules can redirect requests based on detected task type and request capabilities such as tools, vision, or streaming.</p>
        <CodeBlock label="config.yaml">{`ai:
  enabled: true
  routing_rules:
    - when:
        task: coding
      use_model: coding-model
    - when:
        vision: true
      use_model: vision-model
    - when:
        tools: true
      use_group: tool-capable-models`}</CodeBlock>
        <p>Ensure every policy target exists, is enabled, and has eligible keys. Use diagnostics to understand which rule matched.</p>
      </DocSection>

      <DocSection id="diagnostics" title="CLI diagnostics">
        <CodeBlock label="Terminal">{`modelmux ai classify --file request.json
modelmux ai route --file request.json --json
modelmux ai explain --request-id <request-id>
modelmux ai doctor-config`}</CodeBlock>
        <p><code>doctor-config</code> checks AI-related configuration relationships. <code>route</code> previews routing behavior without requiring the application to issue a normal production request.</p>
      </DocSection>
    </>
  ),
  previous: { href: "/docs/tui", title: "TUI dashboard" },
  next: { href: "/docs/cli-reference", title: "CLI reference" },
};

const cliReference: DocPage = {
  slug: "cli-reference",
  href: "/docs/cli-reference",
  section: "Reference",
  title: "CLI reference",
  description: "Command groups for running the proxy, editing configuration, managing keys and secrets, reading logs, and diagnosing AI routing.",
  toc: [
    { id: "global", label: "Global usage" },
    { id: "server", label: "Server and TUI" },
    { id: "config", label: "Configuration" },
    { id: "resources", label: "Providers, models, groups" },
    { id: "keys", label: "Keys" },
    { id: "secrets", label: "Secrets" },
    { id: "logs", label: "Logs and lists" },
    { id: "ai", label: "AI diagnostics" },
  ],
  body: (
    <>
      <DocSection id="global" title="Global usage">
        <CodeBlock label="Terminal">{`modelmux --help
modelmux --config /path/to/config.yaml <command>
modelmux version`}</CodeBlock>
        <p>Use <code>--help</code> on any command or subcommand for the authoritative flags available in the installed version.</p>
      </DocSection>

      <DocSection id="server" title="Server and TUI">
        <DocTable headers={["Command", "Purpose"]} rows={[
          [<code key="1">modelmux start</code>, "Start the HTTP proxy server."],
          [<code key="2">modelmux tui</code>, "Open the terminal dashboard."],
          [<code key="3">modelmux version</code>, "Print version, commit, and build timestamp."],
        ]} />
      </DocSection>

      <DocSection id="config" title="Configuration commands">
        <DocTable headers={["Command", "Purpose"]} rows={[
          [<code key="1">modelmux config init</code>, "Create an example configuration file."],
          [<code key="2">modelmux config validate</code>, "Validate YAML and configuration relationships."],
          [<code key="3">modelmux config validate --json</code>, "Return machine-readable validation output."],
          [<code key="4">modelmux config validate --check-provider</code>, "Also test provider connectivity."],
        ]} />
      </DocSection>

      <DocSection id="resources" title="Providers, models, and groups">
        <DocTable headers={["Command", "Purpose"]} rows={[
          [<code key="1">modelmux provider add ...</code>, "Add a provider to configuration."],
          [<code key="2">modelmux model add ...</code>, "Add a model to configuration."],
          [<code key="3">modelmux group add ...</code>, "Add a model group to configuration."],
          [<code key="4">modelmux providers</code>, "List providers; supports JSON output."],
          [<code key="5">modelmux models</code>, "List models; supports JSON output."],
          [<code key="6">modelmux groups</code>, "List groups; supports JSON output."],
        ]} />
        <p>The exact add-command flags can evolve. Use <code>modelmux provider add --help</code>, <code>modelmux model add --help</code>, and <code>modelmux group add --help</code>.</p>
      </DocSection>

      <DocSection id="keys" title="Key commands">
        <DocTable headers={["Command", "Purpose"]} rows={[
          [<code key="1">modelmux key add ...</code>, "Add a key to configuration."],
          [<code key="2">modelmux key test --id &lt;key&gt;</code>, "Test a key against its provider."],
          [<code key="3">modelmux key enable --id &lt;key&gt;</code>, "Enable a key in config and runtime."],
          [<code key="4">modelmux key disable --id &lt;key&gt;</code>, "Disable a key in config and runtime."],
          [<code key="5">modelmux keys</code>, "List configured keys; supports JSON output."],
        ]} />
      </DocSection>

      <DocSection id="secrets" title="Secret-store commands">
        <DocTable headers={["Command", "Purpose"]} rows={[
          [<code key="1">modelmux secret set --ref name</code>, "Store a value using a hidden prompt."],
          [<code key="2">modelmux secret set --ref name --value "$TOKEN"</code>, "Store a value non-interactively."],
          [<code key="3">modelmux secret list</code>, "List stored secret references, not plaintext values."],
          [<code key="4">modelmux secret delete --ref name</code>, "Delete one stored reference."],
          [<code key="5">modelmux secret export --output file</code>, "Export an encrypted backup."],
          [<code key="6">modelmux secret import --input file</code>, "Import an encrypted backup."],
          [<code key="7">modelmux secret verify</code>, "Check encrypted-store integrity."],
          [<code key="8">modelmux secret rotate-master-key</code>, "Re-encrypt with a new master key."],
        ]} />
      </DocSection>

      <DocSection id="logs" title="Logs and resource lists">
        <CodeBlock label="Terminal">{`modelmux logs --limit 50
modelmux logs --model-id example-chat
modelmux logs --status-code 429
modelmux logs --json --limit 100

modelmux providers --json
modelmux models --json
modelmux keys --json
modelmux groups --json`}</CodeBlock>
      </DocSection>

      <DocSection id="ai" title="AI diagnostics">
        <DocTable headers={["Command", "Purpose"]} rows={[
          [<code key="1">modelmux ai classify --file request.json</code>, "Classify a saved request."],
          [<code key="2">modelmux ai route --file request.json --json</code>, "Preview policy routing."],
          [<code key="3">modelmux ai explain --request-id &lt;id&gt;</code>, "Explain routing for a recorded request."],
          [<code key="4">modelmux ai doctor-config</code>, "Validate AI routing configuration."],
        ]} />
      </DocSection>
    </>
  ),
  previous: { href: "/docs/ai-routing", title: "AI routing" },
  next: { href: "/docs/admin-api", title: "Admin API" },
};

const adminApi: DocPage = {
  slug: "admin-api",
  href: "/docs/admin-api",
  section: "Reference",
  title: "Admin API",
  description: "Reload configuration, inspect status, manage key state, test credentials, and read route traces.",
  toc: [
    { id: "authentication", label: "Authentication" },
    { id: "endpoints", label: "Endpoints" },
    { id: "reload", label: "Reload configuration" },
    { id: "keys", label: "Manage keys" },
    { id: "status", label: "Read status" },
    { id: "traces", label: "Read route traces" },
    { id: "exposure", label: "Exposure guidance" },
  ],
  body: (
    <>
      <DocSection id="authentication" title="Authentication">
        <p>Admin routes use bearer authentication when <code>server.admin.require_auth</code> is true. The token is loaded from the environment variable configured by <code>server.auth_token_env</code>.</p>
        <CodeBlock label="curl">{`curl http://127.0.0.1:8787/admin/status \\
  -H "Authorization: Bearer $MODELMUX_AUTH_TOKEN"`}</CodeBlock>
      </DocSection>

      <DocSection id="endpoints" title="Endpoint reference">
        <DocTable headers={["Method", "Path", "Purpose"]} rows={[
          [<code key="1">POST</code>, <code key="2">/admin/reload</code>, "Reload configuration without restarting the process."],
          [<code key="3">POST</code>, <code key="4">/admin/keys/{id}/enable</code>, "Enable a key and update config/runtime state."],
          [<code key="5">POST</code>, <code key="6">/admin/keys/{id}/disable</code>, "Disable a key and update config/runtime state."],
          [<code key="7">POST</code>, <code key="8">/admin/keys/{id}/test</code>, "Test provider connectivity using one key."],
          [<code key="9">GET</code>, <code key="10">/admin/status</code>, "Read a summary of providers, models, keys, and server state."],
          [<code key="11">GET</code>, <code key="12">/admin/traces/{id}</code>, "Read a stored AI route trace when tracing is enabled."],
        ]} />
      </DocSection>

      <DocSection id="reload" title="Reload configuration">
        <CodeBlock label="curl">{`curl -X POST http://127.0.0.1:8787/admin/reload \\
  -H "Authorization: Bearer $MODELMUX_AUTH_TOKEN"`}</CodeBlock>
        <p>Validate the file before reload. A reload replaces the active routing configuration without requiring a full process restart.</p>
      </DocSection>

      <DocSection id="keys" title="Enable, disable, and test keys">
        <CodeBlock label="curl">{`curl -X POST http://127.0.0.1:8787/admin/keys/provider-a-primary/disable \\
  -H "Authorization: Bearer $MODELMUX_AUTH_TOKEN"

curl -X POST http://127.0.0.1:8787/admin/keys/provider-a-primary/enable \\
  -H "Authorization: Bearer $MODELMUX_AUTH_TOKEN"

curl -X POST http://127.0.0.1:8787/admin/keys/provider-a-primary/test \\
  -H "Authorization: Bearer $MODELMUX_AUTH_TOKEN"`}</CodeBlock>
        <p>Disable a credential before maintenance, revocation, or provider troubleshooting. Testing a key makes an upstream call and may consume a small amount of provider capacity.</p>
      </DocSection>

      <DocSection id="status" title="Read server status">
        <CodeBlock label="curl">{`curl http://127.0.0.1:8787/admin/status \\
  -H "Authorization: Bearer $MODELMUX_AUTH_TOKEN"`}</CodeBlock>
        <p>The status endpoint provides an operational summary. Treat its response as sensitive because it can reveal configured resource names and key state.</p>
      </DocSection>

      <DocSection id="traces" title="Read route traces">
        <CodeBlock label="curl">{`curl http://127.0.0.1:8787/admin/traces/<trace-id> \\
  -H "Authorization: Bearer $MODELMUX_AUTH_TOKEN"`}</CodeBlock>
        <p>Trace records are available when AI route tracing is enabled and the trace has been stored. The response header <code>X-ModelMux-Route-Trace-ID</code> identifies the relevant trace.</p>
      </DocSection>

      <DocSection id="exposure" title="Exposure guidance">
        <Callout title="Do not rely on localhost detection behind a proxy" tone="warning">
          <p>Reverse-proxied requests can appear to originate locally. Keep admin authentication enabled and restrict these routes at the reverse proxy or network layer.</p>
        </Callout>
      </DocSection>
    </>
  ),
  previous: { href: "/docs/cli-reference", title: "CLI reference" },
  next: { href: "/docs/troubleshooting", title: "Troubleshooting" },
};

const troubleshooting: DocPage = {
  slug: "troubleshooting",
  href: "/docs/troubleshooting",
  section: "Reference",
  title: "Troubleshooting",
  description: "Diagnose configuration errors, exhausted routes, provider authentication failures, cooldowns, SQLite locks, and secret-store problems.",
  toc: [
    { id: "start", label: "Start with diagnostics" },
    { id: "config", label: "Configuration failures" },
    { id: "429", label: "All keys exhausted" },
    { id: "auth", label: "Provider 401 or 403" },
    { id: "timeouts", label: "Timeouts and 5xx" },
    { id: "sqlite", label: "SQLite problems" },
    { id: "secrets", label: "Secret-store problems" },
    { id: "proxy", label: "Reverse-proxy auth" },
  ],
  body: (
    <>
      <DocSection id="start" title="Start with these diagnostics">
        <CodeBlock label="Terminal">{`modelmux version
modelmux config validate
modelmux config validate --check-provider
modelmux keys --json
modelmux logs --json --limit 100
curl http://127.0.0.1:8787/metrics`}</CodeBlock>
        <p>Record the request ID, requested model/group, HTTP status, selected provider/key when available, and the exact time of the failure.</p>
      </DocSection>

      <DocSection id="config" title="The configuration does not load">
        <ul className="docs-list">
          <li>Run <code>modelmux config validate</code> and fix the first reported error before later errors.</li>
          <li>Confirm indentation uses spaces and list entries start with <code>-</code>.</li>
          <li>Verify every <code>provider_id</code>, <code>model_id</code>, and <code>key_id</code> reference exists.</li>
          <li>Check that IDs are unique and enabled entries do not reference disabled or missing dependencies.</li>
          <li>Confirm environment variables referenced by <code>value_env</code> and <code>auth_token_env</code> are exported in the process environment.</li>
        </ul>
      </DocSection>

      <DocSection id="429" title="ModelMux returns 429 or all keys are exhausted">
        <ol className="docs-list docs-list-numbered">
          <li>Inspect active, cooldown, invalid, limited, and disabled key counts.</li>
          <li>Check model-level RPM and concurrency capacity.</li>
          <li>Check each key&apos;s minute, daily, token, and concurrency limits.</li>
          <li>Search recent logs for upstream 429 responses and cooldown events.</li>
          <li>Confirm the requested group has at least one enabled member with an eligible key.</li>
        </ol>
        <p>Do not immediately raise limits above provider capacity. Upstream 429 responses indicate that the provider or credential is already enforcing a lower effective limit.</p>
      </DocSection>

      <DocSection id="auth" title="A provider returns 401 or 403">
        <ul className="docs-list">
          <li>Verify the environment variable or secret reference resolves to the intended key.</li>
          <li>Check provider base URL, type, and authentication mode.</li>
          <li>Confirm the credential is authorized for the configured upstream model.</li>
          <li>Run <code>modelmux key test --id &lt;key&gt;</code>.</li>
          <li>Replace revoked credentials, then re-enable or retest the key.</li>
        </ul>
        <p>Authentication failures can mark a key invalid. A health check cannot repair a credential that remains unauthorized.</p>
      </DocSection>

      <DocSection id="timeouts" title="Timeouts, network errors, and provider 5xx responses">
        <p>Check DNS, firewall, TLS, and the provider base URL from the same host or container running ModelMux. Compare <code>providers[].timeout_seconds</code> with normal model latency, especially for long streaming generations.</p>
        <p>Repeated transient errors place keys into cooldown. If every key uses the same unavailable provider, adding more retries will not restore service and may increase latency.</p>
      </DocSection>

      <DocSection id="sqlite" title="SQLite is locked or cannot be opened">
        <ul className="docs-list">
          <li>Confirm only one ModelMux instance uses the database file.</li>
          <li>Check directory ownership and write permissions for the database, WAL, and SHM files.</li>
          <li>Do not place the database on an unsuitable shared network filesystem.</li>
          <li>Do not remove <code>-wal</code> or <code>-shm</code> files while the service is running.</li>
          <li>Verify the disk has available space and the filesystem is writable.</li>
        </ul>
      </DocSection>

      <DocSection id="secrets" title="The encrypted secret store cannot be opened">
        <ul className="docs-list">
          <li>Export the same <code>MODELMUX_MASTER_KEY</code> used when the file was created.</li>
          <li>Confirm the process can read <code>secrets.enc</code>.</li>
          <li>Run <code>modelmux secret verify</code>.</li>
          <li>Restore the encrypted backup and matching master key together.</li>
          <li>Do not edit the encrypted file manually.</li>
        </ul>
        <Callout title="Lost master keys cannot be recovered" tone="warning">
          <p>AES-GCM encryption is designed to prevent decryption without the correct master key. Keep an independent secure copy.</p>
        </Callout>
      </DocSection>

      <DocSection id="proxy" title="Authentication behaves unexpectedly behind a reverse proxy">
        <p>Ensure the client sends <code>Authorization: Bearer ...</code> and the reverse proxy forwards that header. Keep both <code>server.require_auth</code> and <code>server.admin.require_auth</code> enabled where appropriate.</p>
        <p>Do not bypass admin authentication because the reverse proxy connects from localhost or a trusted container network. Enforce access at both layers.</p>
      </DocSection>
    </>
  ),
  previous: { href: "/docs/admin-api", title: "Admin API" },
};

export const docPages: Record<string, DocPage> = {
  overview,
  installation,
  "quick-start": quickStart,
  "core-concepts": coreConcepts,
  configuration,
  providers,
  routing,
  "rate-limiting": rateLimiting,
  storage,
  observability,
  security,
  tui,
  "ai-routing": aiRouting,
  "cli-reference": cliReference,
  "admin-api": adminApi,
  troubleshooting,
};
