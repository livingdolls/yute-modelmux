import Link from "next/link";
import type { CSSProperties } from "react";

const lifecycleSteps = [
  {
    index: "01",
    title: "authentication",
    detail: "Bearer token verified before proxy work begins.",
    value: "passed",
    latency: "0.4ms",
  },
  {
    index: "02",
    title: "policy + guardrails",
    detail: "Target group and request policy resolved.",
    value: "production",
    latency: "0.7ms",
  },
  {
    index: "03",
    title: "limit check",
    detail: "Model RPM, key quota, and concurrency checked locally.",
    value: "42/60 RPM",
    latency: "0.2ms",
  },
  {
    index: "04",
    title: "key router",
    detail: "Health, priority, cooldown, and strategy evaluated.",
    value: "mimo-primary",
    latency: "0.3ms",
  },
  {
    index: "05",
    title: "provider adapter",
    detail: "Compatible payload forwarded and response streamed.",
    value: "streaming",
    latency: "840ms",
  },
] as const;

const facts = [
  ["runtime", "Go"],
  ["deployment", "single binary"],
  ["database", "optional SQLite"],
  ["services", "none required"],
  ["default port", ":8787"],
] as const;

export function RequestLifecycle() {
  return (
    <section className="request-xray-section section-grid" id="architecture">
      <div className="container request-xray-layout">
        <div className="request-xray-copy">
          <span className="section-kicker">REQUEST LIFECYCLE X-RAY</span>
          <h2>
            Simple to deploy.
            <br />
            Visible at every stage.
          </h2>
          <p>
            Follow one request from authentication to the upstream stream. ModelMux keeps the routing
            path explicit, measurable, and independent from your application code.
          </p>

          <dl className="request-xray-facts">
            {facts.map(([label, value]) => (
              <div key={label}>
                <dt>{label}</dt>
                <dd>{value}</dd>
              </div>
            ))}
          </dl>

          <Link className="request-xray-link" href="/docs/core-concepts">
            inspect core concepts <span aria-hidden="true">→</span>
          </Link>
        </div>

        <article className="request-trace-terminal" aria-label="Illustrated ModelMux request lifecycle derived from request logs">
          <header className="request-trace-header">
            <div className="request-trace-dots" aria-hidden="true"><i /><i /><i /></div>
            <code>modelmux logs --json --limit 1</code>
            <span>illustrated trace</span>
          </header>

          <div className="request-trace-meta">
            <div><span>REQUEST</span><strong>request_8a91</strong></div>
            <div><span>METHOD</span><strong>POST /v1/chat</strong></div>
            <div><span>TARGET</span><strong>high-price</strong></div>
            <div><span>STREAM</span><strong>true</strong></div>
          </div>

          <ol className="request-trace-list">
            {lifecycleSteps.map((step, index) => (
              <li className="request-trace-step" style={{ "--trace-delay": `${index * 100}ms` } as CSSProperties} key={step.index}>
                <div className="request-trace-index">{step.index}</div>
                <div className="request-trace-rail" aria-hidden="true"><i /></div>
                <div className="request-trace-content">
                  <strong>{step.title}</strong>
                  <p>{step.detail}</p>
                </div>
                <code>{step.value}</code>
                <time>{step.latency}</time>
              </li>
            ))}
          </ol>

          <div className="request-trace-result">
            <div>
              <span>RESPONSE</span>
              <strong>stream completed</strong>
            </div>
            <p>Selected route stayed observable from local checks through upstream delivery.</p>
            <code>200 · 842ms</code>
          </div>

          <footer className="request-trace-status">
            <span><b>AUTH</b> passed</span>
            <span><b>LIMITS</b> healthy</span>
            <span><b>RETRY</b> 0</span>
            <span><b>BUFFERING</b> minimal</span>
          </footer>
        </article>
      </div>
    </section>
  );
}
