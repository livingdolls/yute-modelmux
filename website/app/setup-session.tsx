import Link from "next/link";

const setupSteps = [
  {
    index: "01",
    label: "provider",
    title: "Configure upstream",
    description: "Register the provider adapter and upstream endpoint.",
    fields: [
      ["provider.id", "mimo"],
      ["provider.type", "openai-compatible"],
      ["provider.url", "https://api.example.com/v1"],
    ],
    result: "configured",
  },
  {
    index: "02",
    label: "routing",
    title: "Attach models and keys",
    description: "Connect routable models, credentials, and recovery policy.",
    fields: [
      ["model.id", "mimo-v2.5-pro"],
      ["strategy", "failover"],
      ["keys", "mimo-primary, mimo-backup"],
    ],
    result: "routing ready",
  },
  {
    index: "03",
    label: "connect",
    title: "Point your application",
    description: "Keep the OpenAI client and replace only its base URL.",
    fields: [
      ["base_url", "http://127.0.0.1:8787/v1"],
      ["auth", "MODELMUX_TOKEN"],
      ["compatibility", "OpenAI SDK"],
    ],
    result: "connected",
  },
] as const;

export function SetupSession() {
  return (
    <section className="setup-session-section section-grid" aria-labelledby="setup-session-title">
      <div className="container setup-session-shell">
        <div className="setup-session-layout">
          <div className="setup-session-copy">
            <span className="section-kicker">THREE STEPS</span>
            <h2 id="setup-session-title">
              From API keys
              <br />
              to a reliable endpoint.
            </h2>
            <p>
              Configure ModelMux once, validate the route, then keep your application focused on
              product logic instead of provider-specific recovery code.
            </p>

            <ol className="setup-copy-steps" aria-label="ModelMux setup steps">
              {setupSteps.map((step) => (
                <li key={step.index}>
                  <span>{step.index}</span>
                  <div>
                    <strong>{step.label}</strong>
                    <small>{step.title}</small>
                  </div>
                  <i aria-hidden="true">✓</i>
                </li>
              ))}
            </ol>

            <Link className="setup-session-link" href="/docs/quick-start">
              open the full quick start <span aria-hidden="true">→</span>
            </Link>
          </div>

          <article className="setup-terminal" aria-label="Example guided ModelMux setup session">
            <header className="setup-terminal-header">
              <div className="setup-terminal-dots" aria-hidden="true">
                <i />
                <i />
                <i />
              </div>
              <code>modelmux init --guided</code>
              <span>setup session</span>
            </header>

            <nav className="setup-terminal-tabs" aria-label="Setup progress">
              {setupSteps.map((step) => (
                <span key={step.index}>
                  <b>{step.index}</b> {step.label}
                  <i aria-hidden="true">done</i>
                </span>
              ))}
            </nav>

            <div className="setup-terminal-body">
              <div className="setup-session-intro">
                <span>$</span>
                <div>
                  <strong>Creating a reliable local route</strong>
                  <p>Answer three configuration stages. ModelMux validates each stage before continuing.</p>
                </div>
              </div>

              <ol className="setup-stage-list">
                {setupSteps.map((step) => (
                  <li className="setup-stage" key={step.index}>
                    <div className="setup-stage-rail" aria-hidden="true">
                      <span>{step.index}</span>
                      <i />
                    </div>

                    <div className="setup-stage-content">
                      <div className="setup-stage-heading">
                        <div>
                          <span>{step.label.toUpperCase()}</span>
                          <h3>{step.title}</h3>
                          <p>{step.description}</p>
                        </div>
                        <strong>✓ {step.result}</strong>
                      </div>

                      <dl className="setup-field-list">
                        {step.fields.map(([key, value]) => (
                          <div key={key}>
                            <dt>{key}</dt>
                            <dd>{value}</dd>
                          </div>
                        ))}
                      </dl>
                    </div>
                  </li>
                ))}
              </ol>

              <div className="setup-ready-panel">
                <div className="setup-ready-heading">
                  <div>
                    <span>READY</span>
                    <strong>Endpoint accepting requests</strong>
                  </div>
                  <i aria-hidden="true">● healthy</i>
                </div>

                <dl>
                  <div>
                    <dt>endpoint</dt>
                    <dd>http://127.0.0.1:8787/v1</dd>
                  </div>
                  <div>
                    <dt>provider</dt>
                    <dd>mimo</dd>
                  </div>
                  <div>
                    <dt>strategy</dt>
                    <dd>failover</dd>
                  </div>
                  <div>
                    <dt>healthy keys</dt>
                    <dd>2</dd>
                  </div>
                </dl>

                <div className="setup-ready-command">
                  <span>$</span>
                  <code>curl localhost:8787/v1/models</code>
                  <strong>200 OK</strong>
                </div>
              </div>
            </div>

            <footer className="setup-status-bar">
              <span><b>PROVIDER</b> 1</span>
              <span><b>MODEL</b> 1</span>
              <span><b>KEYS</b> 2</span>
              <span><b>ROUTE</b> HEALTHY</span>
              <span><b>PORT</b> :8787</span>
            </footer>
          </article>
        </div>
      </div>
    </section>
  );
}
