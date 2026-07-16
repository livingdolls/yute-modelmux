import Link from "next/link";
import type { CSSProperties } from "react";

const installSteps = [
  {
    label: "install",
    command: "go install github.com/livingdolls/yute-modelmux/cmd/modelmux@latest",
    result: "binary installed",
  },
  {
    label: "configure",
    command: "modelmux config init && modelmux config validate",
    result: "configuration valid",
  },
  {
    label: "run",
    command: "modelmux start",
    result: "proxy listening on :8787",
  },
] as const;

export function InstallConsole() {
  return (
    <section className="install-console-section section-grid">
      <div className="container install-console-layout">
        <div className="install-console-copy">
          <span className="section-kicker">READY TO ROUTE?</span>
          <h2>
            Install one binary.
            <br />
            Expose one reliable endpoint.
          </h2>
          <p>
            Start locally with Go, add a provider and keys, then point your existing OpenAI-compatible
            client at ModelMux. No external control plane is required.
          </p>

          <div className="install-console-actions">
            <Link className="button button-primary" href="/docs/installation">installation guide →</Link>
            <a className="button button-secondary" href="https://github.com/livingdolls/yute-modelmux/releases">
              release binaries
            </a>
          </div>
        </div>

        <article className="install-terminal" aria-label="ModelMux installation commands">
          <header className="install-terminal-header">
            <div className="install-terminal-dots" aria-hidden="true"><i /><i /><i /></div>
            <code>install modelmux</code>
            <span>3 commands</span>
          </header>

          <div className="install-platform-tabs" aria-label="Available installation methods">
            <span className="active">[ Go ]</span>
            <span>[ macOS / Linux binary ]</span>
            <span>[ source ]</span>
          </div>

          <div className="install-terminal-body">
            {installSteps.map((step, index) => (
              <div className="install-step" style={{ "--install-delay": `${index * 130}ms` } as CSSProperties} key={step.label}>
                <div className="install-step-label"><span>{String(index + 1).padStart(2, "0")}</span>{step.label}</div>
                <div className="install-command-line">
                  <span>$</span>
                  <code>{step.command}</code>
                </div>
                <div className="install-command-result">
                  <span>✓</span>
                  <strong>{step.result}</strong>
                </div>
              </div>
            ))}

            <div className="install-ready-panel">
              <div className="install-ready-icon">&gt;_</div>
              <div>
                <span>ENDPOINT READY</span>
                <strong>http://127.0.0.1:8787/v1</strong>
                <p>Use config-defined model IDs from any OpenAI-compatible SDK.</p>
              </div>
              <code>exit 0</code>
            </div>
          </div>

          <footer className="install-status-bar">
            <span><b>BINARY</b> ready</span>
            <span><b>CONFIG</b> valid</span>
            <span><b>ROUTE</b> healthy</span>
            <span><b>PORT</b> :8787</span>
          </footer>
        </article>
      </div>
    </section>
  );
}
