import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Documentation",
  description: "Install ModelMux and route your first LLM request through one reliable endpoint.",
};

const nav = [
  { title: "Getting started", links: ["Introduction", "Installation", "Quick start"] },
  { title: "Core concepts", links: ["Providers", "Models", "API keys", "Model groups", "Routing strategies"] },
  { title: "Operations", links: ["Rate limiting", "Storage", "Observability", "Security"] },
  { title: "Reference", links: ["CLI commands", "Admin API", "Configuration"] },
];

export default function DocsPage() {
  return (
    <div className="docs-shell">
      <aside className="docs-sidebar">
        <div className="docs-sidebar-inner">
          <label className="docs-search">
            <svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="8.5" cy="8.5" r="5.5" fill="none" stroke="currentColor" strokeWidth="1.5" /><path d="m13 13 4 4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" /></svg>
            <span>Search documentation</span>
            <kbd>/</kbd>
          </label>
          <nav aria-label="Documentation navigation">
            {nav.map((group) => (
              <div className="docs-nav-group" key={group.title}>
                <strong>{group.title}</strong>
                {group.links.map((link) => (
                  <a className={link === "Quick start" ? "active" : ""} href={link === "Quick start" ? "#quick-start" : "#"} key={link}>{link}</a>
                ))}
              </div>
            ))}
          </nav>
        </div>
      </aside>

      <article className="docs-content" id="quick-start">
        <div className="docs-breadcrumb"><Link href="/docs">Docs</Link><span>/</span><span>Getting started</span><span>/</span><strong>Quick start</strong></div>
        <div className="docs-title">
          <span className="section-kicker">GETTING STARTED</span>
          <h1>Quick start</h1>
          <p>Install ModelMux, create a minimal configuration, and route your first request through the local OpenAI-compatible endpoint.</p>
        </div>

        <div className="docs-callout docs-callout-info">
          <strong>What you will build</strong>
          <p>A local ModelMux proxy on <code>127.0.0.1:8787</code> connected to one upstream provider and one API key.</p>
        </div>

        <section className="docs-section" id="install">
          <h2><a href="#install">1. Install ModelMux</a></h2>
          <p>Install the latest release with Go:</p>
          <div className="code-block">
            <div className="code-header"><span>Terminal</span><button type="button">Copy</button></div>
            <pre><code><span className="code-muted">$</span> go install github.com/livingdolls/yute-modelmux/cmd/modelmux@latest</code></pre>
          </div>
          <p>Confirm that the binary is available:</p>
          <div className="code-block">
            <div className="code-header"><span>Terminal</span><button type="button">Copy</button></div>
            <pre><code><span className="code-muted">$</span> modelmux version</code></pre>
          </div>
          <p className="docs-note">Prebuilt binaries are also available for Linux, macOS, and Windows from GitHub Releases.</p>
        </section>

        <section className="docs-section" id="configure">
          <h2><a href="#configure">2. Create a configuration</a></h2>
          <p>Generate the example configuration:</p>
          <div className="code-block">
            <div className="code-header"><span>Terminal</span><button type="button">Copy</button></div>
            <pre><code><span className="code-muted">$</span> modelmux config init</code></pre>
          </div>
          <p>Then add a provider, model, and key to <code>~/.config/modelmux/config.yaml</code>:</p>
          <div className="code-block">
            <div className="code-header"><span>config.yaml</span><button type="button">Copy</button></div>
            <pre><code>{`server:\n  host: "127.0.0.1"\n  port: 8787\n\nproviders:\n  - id: mimo\n    name: Xiaomi MiMo\n    type: openai-compatible\n    base_url: https://api.example.com/v1\n    auth_type: bearer\n    enabled: true\n\nmodels:\n  - id: mimo-v2.5-pro\n    provider_id: mimo\n    model_name: mimo-v2.5-pro\n    strategy: failover\n    enabled: true\n\nkeys:\n  - id: mimo-primary\n    provider_id: mimo\n    model_id: mimo-v2.5-pro\n    value_env: MIMO_API_KEY\n    status: active\n    priority: 1`}</code></pre>
          </div>
          <div className="docs-callout docs-callout-warning">
            <strong>Keep secrets outside YAML</strong>
            <p>Use <code>value_env</code> or the encrypted secret store instead of committing plaintext API keys.</p>
          </div>
        </section>

        <section className="docs-section" id="start">
          <h2><a href="#start">3. Start the proxy</a></h2>
          <p>Export your provider key and start ModelMux:</p>
          <div className="code-block">
            <div className="code-header"><span>Terminal</span><button type="button">Copy</button></div>
            <pre><code>{`$ export MIMO_API_KEY="your-provider-key"\n$ modelmux start`}</code></pre>
          </div>
          <p>The OpenAI-compatible endpoint is now available at:</p>
          <div className="endpoint-card"><span>POST</span><code>http://127.0.0.1:8787/v1/chat/completions</code></div>
        </section>

        <section className="docs-section" id="request">
          <h2><a href="#request">4. Send your first request</a></h2>
          <div className="code-block">
            <div className="code-header"><span>curl</span><button type="button">Copy</button></div>
            <pre><code>{`curl http://127.0.0.1:8787/v1/chat/completions \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "mimo-v2.5-pro",\n    "messages": [\n      {"role": "user", "content": "Explain reliable LLM routing."}\n    ]\n  }'`}</code></pre>
          </div>
        </section>

        <section className="docs-section" id="next">
          <h2><a href="#next">Next steps</a></h2>
          <div className="next-grid">
            <a href="#"><strong>Add multiple keys</strong><span>Configure failover, priority, and per-key limits.</span></a>
            <a href="#"><strong>Enable SQLite</strong><span>Persist logs, usage counters, quotas, and cooldown state.</span></a>
            <a href="#"><strong>Open the TUI</strong><span>Inspect live keys, logs, metrics, and configuration.</span></a>
            <a href="#"><strong>Secure the proxy</strong><span>Enable authentication before network exposure.</span></a>
          </div>
        </section>

        <div className="docs-pagination">
          <a href="#"><small>Previous</small><strong>Installation</strong></a>
          <a href="#"><small>Next</small><strong>Providers →</strong></a>
        </div>
      </article>

      <aside className="docs-toc">
        <strong>On this page</strong>
        <a href="#install">Install ModelMux</a>
        <a href="#configure">Create a configuration</a>
        <a href="#start">Start the proxy</a>
        <a href="#request">Send a request</a>
        <a href="#next">Next steps</a>
      </aside>
    </div>
  );
}
