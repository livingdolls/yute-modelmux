import Link from "next/link";
import { ProviderMatrix } from "./provider-matrix";

const ArrowIcon = () => (
  <svg viewBox="0 0 20 20" aria-hidden="true">
    <path d="M4 10h11M11 6l4 4-4 4" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

const CheckIcon = () => (
  <svg viewBox="0 0 20 20" aria-hidden="true">
    <path d="m4 10 3.2 3.2L16 5.8" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

const features = [
  {
    index: "01",
    title: "Resilient key routing",
    text: "Rotate across keys using failover, round-robin, least-error, or least-used strategies. Cool down unhealthy keys automatically.",
    meta: ["Priority routing", "Automatic retry", "Health checks"],
  },
  {
    index: "02",
    title: "One compatible endpoint",
    text: "Keep your OpenAI SDK and route requests to OpenAI-compatible, Anthropic, Gemini, or custom providers behind one local API.",
    meta: ["Streaming", "Tool calls", "Provider conversion"],
  },
  {
    index: "03",
    title: "Limits at every layer",
    text: "Enforce model and key-level RPM, token, daily quota, and concurrency limits before upstream providers reject production traffic.",
    meta: ["RPM and TPM", "Daily quotas", "Concurrency"],
  },
  {
    index: "04",
    title: "Operational visibility",
    text: "Inspect request IDs, structured logs, latency, token usage, key health, and Prometheus metrics from the terminal or API.",
    meta: ["Prometheus", "SQLite persistence", "Live TUI"],
  },
];

const routingRows = [
  ["mimo-primary", "MIMO", "ACTIVE", "42 / 60 RPM"],
  ["mimo-backup", "MIMO", "STANDBY", "3 / 60 RPM"],
  ["deepseek-01", "DEEPSEEK", "COOLDOWN", "02:41"],
  ["claude-team", "ANTHROPIC", "ACTIVE", "18 / 50 RPM"],
];

export default function HomePage() {
  return (
    <>
      <section className="hero section-grid">
        <div className="hero-glow" aria-hidden="true" />
        <div className="container hero-layout">
          <div className="hero-copy">
            <div className="eyebrow"><span className="pulse-dot" /> Lightweight LLM gateway</div>
            <h1>One endpoint.<br /><span>Every model.</span><br />No single key can take you down.</h1>
            <p>
              Route requests across LLM providers and API keys with automatic failover,
              rate limiting, usage tracking, and policy-based routing—all from one Go binary.
            </p>
            <div className="hero-actions">
              <Link className="button button-primary" href="/docs">
                Read the quick start <ArrowIcon />
              </Link>
              <a className="button button-secondary" href="https://github.com/livingdolls/yute-modelmux">
                View on GitHub
              </a>
            </div>
            <div className="hero-proof">
              <span><CheckIcon /> Single binary</span>
              <span><CheckIcon /> OpenAI-compatible</span>
              <span><CheckIcon /> Self-hosted</span>
            </div>
          </div>

          <div className="hero-visual" aria-label="ModelMux routing status preview">
            <div className="terminal-window">
              <div className="terminal-bar">
                <div className="terminal-dots"><i /><i /><i /></div>
                <span>modelmux — live router</span>
                <span className="terminal-online"><i /> online</span>
              </div>
              <div className="terminal-body">
                <div className="terminal-heading">
                  <div>
                    <span className="muted-label">ROUTING TARGET</span>
                    <strong>high-price</strong>
                  </div>
                  <span className="status-badge">weighted</span>
                </div>

                <div className="route-map" aria-hidden="true">
                  <div className="route-client">POST /v1/chat</div>
                  <div className="route-line"><i /><i /><i /></div>
                  <div className="route-core">
                    <span>MM</span>
                    <small>router</small>
                  </div>
                  <div className="route-branches">
                    <span className="branch-active">MIMO</span>
                    <span>CLAUDE</span>
                    <span>GEMINI</span>
                  </div>
                </div>

                <div className="terminal-table">
                  <div className="terminal-row terminal-row-head">
                    <span>KEY</span><span>PROVIDER</span><span>STATE</span><span>USAGE</span>
                  </div>
                  {routingRows.map(([key, provider, state, usage]) => (
                    <div className="terminal-row" key={key}>
                      <span>{key}</span>
                      <span>{provider}</span>
                      <span className={`state state-${state.toLowerCase()}`}>{state}</span>
                      <span>{usage}</span>
                    </div>
                  ))}
                </div>

                <div className="terminal-log">
                  <span>15:42:08</span>
                  <strong>request completed</strong>
                  <span>mimo-primary</span>
                  <span>200</span>
                  <span>842ms</span>
                </div>
              </div>
            </div>
            <div className="floating-card floating-card-top">
              <span className="floating-icon">↻</span>
              <div><strong>Automatic failover</strong><small>429 → next healthy key</small></div>
            </div>
            <div className="floating-card floating-card-bottom">
              <span className="floating-icon">✓</span>
              <div><strong>99.74% success</strong><small>last 24 hours</small></div>
            </div>
          </div>
        </div>
      </section>

      <ProviderMatrix />

      <section className="section section-grid" id="features">
        <div className="container">
          <div className="section-heading split-heading">
            <div>
              <span className="section-kicker">BUILT FOR FAILURE</span>
              <h2>Reliability belongs<br />in your infrastructure.</h2>
            </div>
            <p>
              ModelMux handles the unstable parts of LLM APIs before they reach your application,
              without adding Redis, queues, or another control plane.
            </p>
          </div>

          <div className="feature-grid">
            {features.map((feature) => (
              <article className="feature-card" key={feature.index}>
                <span className="feature-index">{feature.index}</span>
                <h3>{feature.title}</h3>
                <p>{feature.text}</p>
                <div className="feature-meta">
                  {feature.meta.map((item) => <span key={item}>{item}</span>)}
                </div>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section className="section workflow-section">
        <div className="container">
          <div className="section-heading centered-heading">
            <span className="section-kicker">THREE STEPS</span>
            <h2>From API keys to a reliable endpoint.</h2>
            <p>Configure once, then keep your application code focused on the product.</p>
          </div>

          <div className="workflow-grid">
            <article>
              <span>01</span>
              <h3>Configure providers</h3>
              <pre><code>{`providers:\n  - id: mimo\n    type: openai-compatible\n    base_url: https://api.example.com/v1`}</code></pre>
            </article>
            <article>
              <span>02</span>
              <h3>Add models and keys</h3>
              <pre><code>{`models:\n  - id: mimo-v2.5-pro\n    strategy: failover\n\nkeys:\n  - id: mimo-primary\n    secret_ref: mimo-key`}</code></pre>
            </article>
            <article>
              <span>03</span>
              <h3>Point your client</h3>
              <pre><code>{`const client = new OpenAI({\n  baseURL: "http://127.0.0.1:8787/v1",\n  apiKey: process.env.MODELMUX_TOKEN,\n});`}</code></pre>
            </article>
          </div>
        </div>
      </section>

      <section className="section section-grid" id="architecture">
        <div className="container architecture-layout">
          <div className="architecture-copy">
            <span className="section-kicker">ONE SMALL BINARY</span>
            <h2>Simple to deploy.<br />Serious under load.</h2>
            <p>
              ModelMux keeps domain logic isolated from HTTP, CLI, and TUI adapters. Shared state is
              concurrency-safe, streaming is passed through with minimal buffering, and persistence is optional.
            </p>
            <ul className="check-list">
              <li><CheckIcon /><span><strong>No required external services</strong>Run in memory or enable embedded SQLite.</span></li>
              <li><CheckIcon /><span><strong>Encrypted secret storage</strong>AES-256-GCM with Argon2id key derivation.</span></li>
              <li><CheckIcon /><span><strong>Hot configuration reload</strong>Update routes without restarting the proxy.</span></li>
              <li><CheckIcon /><span><strong>Production observability</strong>Metrics, logs, request IDs, and route traces.</span></li>
            </ul>
            <Link className="text-link" href="/docs">Explore the architecture <ArrowIcon /></Link>
          </div>

          <div className="architecture-diagram" aria-label="ModelMux architecture diagram">
            <div className="diagram-label">APPLICATIONS</div>
            <div className="diagram-apps">
              <span>Web</span><span>API</span><span>Agents</span><span>CLI</span>
            </div>
            <div className="diagram-arrow">↓ OpenAI-compatible API</div>
            <div className="diagram-core">
              <div className="diagram-core-header"><strong>ModelMux</strong><span>:8787</span></div>
              <div className="diagram-core-grid">
                <span>Authentication</span><span>Guardrails</span>
                <span>Model selection</span><span>Rate limits</span>
                <span>Key routing</span><span>Retry + cooldown</span>
              </div>
            </div>
            <div className="diagram-arrow">↓ Provider adapters</div>
            <div className="diagram-providers">
              <span>OpenAI</span><span>Anthropic</span><span>Gemini</span>
            </div>
          </div>
        </div>
      </section>

      <section className="section terminal-showcase">
        <div className="container terminal-showcase-grid">
          <div className="showcase-window">
            <div className="showcase-sidebar">
              <strong>MODELMUX</strong>
              {["Providers", "Models", "Groups", "Chat", "Keys", "Logs", "Config", "AI"].map((item) => (
                <span className={item === "Keys" ? "selected" : ""} key={item}>{item}</span>
              ))}
            </div>
            <div className="showcase-content">
              <div className="showcase-title"><div><strong>Key health</strong><small>Live state across configured providers</small></div><span>8 active</span></div>
              <div className="showcase-stats">
                <div><span>ACTIVE</span><strong>8</strong></div>
                <div><span>COOLDOWN</span><strong>1</strong></div>
                <div><span>INVALID</span><strong>0</strong></div>
                <div><span>REQUESTS</span><strong>14.3k</strong></div>
              </div>
              <div className="showcase-rows">
                {routingRows.map(([key, provider, state, usage]) => (
                  <div key={key}><span>{key}</span><span>{provider}</span><span className={`state state-${state.toLowerCase()}`}>{state}</span><span>{usage}</span></div>
                ))}
              </div>
            </div>
          </div>
          <div className="showcase-copy">
            <span className="section-kicker">OPERATED FROM THE TERMINAL</span>
            <h2>See the router<br />while it works.</h2>
            <p>
              Inspect provider configuration, model groups, live key state, recent logs, metrics,
              AI diagnostics, and prompt sessions from the built-in TUI.
            </p>
            <div className="command-line"><span>$</span><code>modelmux tui</code><i>↵</i></div>
          </div>
        </div>
      </section>

      <section className="cta-section section-grid">
        <div className="container cta-card">
          <div>
            <span className="section-kicker">START ROUTING</span>
            <h2>Make your LLM integrations resilient.</h2>
            <p>Install ModelMux, configure your first provider, and expose one reliable endpoint in minutes.</p>
          </div>
          <div className="cta-actions">
            <Link className="button button-primary" href="/docs">Read the quick start <ArrowIcon /></Link>
            <a className="button button-secondary" href="https://github.com/livingdolls/yute-modelmux/releases">Download binary</a>
          </div>
        </div>
      </section>
    </>
  );
}
