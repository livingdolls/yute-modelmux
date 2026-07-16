import Link from "next/link";

const bootLines = [
  ["config", "~/.config/modelmux/config.yaml", "loaded"],
  ["providers", "4 adapters", "ready"],
  ["keys", "10 configured", "healthy"],
  ["server", "http://127.0.0.1:8787", "listening"],
] as const;

export function HeroRouter() {
  return (
    <section className="hero-router section-grid">
      <div className="hero-router-glow" aria-hidden="true" />
      <div className="container hero-router-layout">
        <div className="hero-router-copy">
          <div className="eyebrow"><span className="pulse-dot" /> Lightweight LLM gateway</div>
          <h1>
            One endpoint.
            <br />
            <span>Every model.</span>
            <br />
            No single key can take you down.
          </h1>
          <p>
            Route OpenAI-compatible traffic across providers and API keys with automatic failover,
            layered limits, usage tracking, and policy-based routing—all from one Go binary.
          </p>

          <div className="hero-router-actions">
            <Link className="button button-primary" href="/docs/quick-start">
              quick_start →
            </Link>
            <a className="button button-secondary" href="https://github.com/livingdolls/yute-modelmux">
              view_source
            </a>
          </div>

          <div className="hero-router-proof" aria-label="ModelMux deployment characteristics">
            <span><b>01</b> single binary</span>
            <span><b>02</b> self-hosted</span>
            <span><b>03</b> no required services</span>
          </div>
        </div>

        <article className="hero-boot-console" aria-label="ModelMux server startup preview">
          <header className="hero-boot-header">
            <div className="hero-boot-dots" aria-hidden="true"><i /><i /><i /></div>
            <code>modelmux start</code>
            <span><i /> running</span>
          </header>

          <div className="hero-boot-body">
            <div className="hero-boot-command"><span>$</span><code>modelmux start</code><i className="terminal-cursor" /></div>

            <div className="hero-boot-lines">
              {bootLines.map(([label, value, state], index) => (
                <div className="hero-boot-line" style={{ "--boot-delay": `${index * 110}ms` } as React.CSSProperties} key={label}>
                  <span className="hero-boot-check">✓</span>
                  <b>{label}</b>
                  <code>{value}</code>
                  <span>{state}</span>
                </div>
              ))}
            </div>

            <div className="hero-route-topology" aria-label="Application requests route through ModelMux to multiple providers">
              <div className="hero-topology-node">
                <span>CLIENT</span>
                <strong>your application</strong>
                <small>OpenAI SDK</small>
              </div>
              <div className="hero-topology-link"><i /><span>POST /v1/chat</span></div>
              <div className="hero-topology-core">
                <span>&gt;_</span>
                <strong>ModelMux</strong>
                <small>:8787</small>
              </div>
              <div className="hero-topology-link hero-topology-link-out"><i /><span>route</span></div>
              <div className="hero-topology-providers">
                <span>OpenAI-compatible</span>
                <span>Anthropic</span>
                <span>Gemini</span>
                <span>Custom</span>
              </div>
            </div>
          </div>

          <footer className="hero-boot-status">
            <span><b>MODE</b> proxy</span>
            <span><b>ROUTES</b> healthy</span>
            <span><b>PORT</b> :8787</span>
          </footer>
        </article>
      </div>
    </section>
  );
}
