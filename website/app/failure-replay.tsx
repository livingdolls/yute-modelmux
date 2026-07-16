import Link from "next/link";

const replaySteps = [
  {
    index: "01",
    time: "15:42:08.021",
    title: "request received",
    detail: "POST /v1/chat/completions",
    result: "accepted",
    tone: "neutral",
  },
  {
    index: "02",
    time: "15:42:08.024",
    title: "primary route selected",
    detail: "mimo-primary · priority 1",
    result: "routing",
    tone: "active",
  },
  {
    index: "03",
    time: "15:42:08.311",
    title: "upstream rejected request",
    detail: "rate limit exceeded",
    result: "429",
    tone: "failure",
  },
  {
    index: "04",
    time: "15:42:08.312",
    title: "key placed in cooldown",
    detail: "mimo-primary · retry after 300s",
    result: "cooldown",
    tone: "warning",
  },
  {
    index: "05",
    time: "15:42:08.319",
    title: "next healthy route selected",
    detail: "claude-team · priority 2",
    result: "failover",
    tone: "active",
  },
  {
    index: "06",
    time: "15:42:09.163",
    title: "request completed",
    detail: "response streamed to client",
    result: "200 · 842ms",
    tone: "success",
  },
] as const;

const outcomes = [
  {
    index: "01",
    title: "Automatic failover",
    text: "Move to the next healthy key or provider without changing application code or asking the client to retry.",
    meta: "429 · 5xx · timeout",
  },
  {
    index: "02",
    title: "Layered limits",
    text: "Apply model, key, token, daily quota, and concurrency limits before an exhausted upstream rejects traffic.",
    meta: "RPM · TPM · quota",
  },
  {
    index: "03",
    title: "Observable routing",
    text: "Trace every selection, retry, cooldown, latency measurement, request ID, and final route from one terminal.",
    meta: "logs · metrics · trace",
  },
] as const;

export function FailureReplay() {
  return (
    <section className="failure-replay-section section-grid" id="features">
      <div className="container failure-replay-shell">
        <div className="failure-replay-layout">
          <div className="failure-replay-copy">
            <span className="section-kicker">BUILT FOR FAILURE</span>
            <h2>
              Reliability belongs
              <br />
              in your infrastructure.
            </h2>
            <p>
              Your application should not need to understand provider outages, exhausted keys,
              cooldown timers, or retry policy. ModelMux absorbs those failures before they become
              client errors.
            </p>

            <div className="failure-command" aria-label="Example route explanation command">
              <span>$</span>
              <code>modelmux ai explain --request-id request_7f2a</code>
              <i aria-hidden="true">↵</i>
            </div>

            <Link className="failure-replay-link" href="/docs/routing">
              inspect routing and failover <span aria-hidden="true">→</span>
            </Link>
          </div>

          <article className="failure-terminal" aria-label="Example ModelMux automatic failure recovery trace">
            <header className="failure-terminal-header">
              <div className="failure-terminal-dots" aria-hidden="true">
                <i />
                <i />
                <i />
              </div>
              <code>modelmux replay request_7f2a</code>
              <span>demo trace</span>
            </header>

            <div className="failure-terminal-body">
              <div className="failure-request-meta">
                <div>
                  <span>REQUEST</span>
                  <strong>request_7f2a</strong>
                </div>
                <div>
                  <span>TARGET</span>
                  <strong>high-price</strong>
                </div>
                <div>
                  <span>STRATEGY</span>
                  <strong>failover</strong>
                </div>
              </div>

              <ol className="failure-timeline">
                {replaySteps.map((step) => (
                  <li className={`failure-step failure-step-${step.tone}`} key={step.index}>
                    <span className="failure-step-index">{step.index}</span>
                    <span className="failure-step-track" aria-hidden="true">
                      <i />
                    </span>
                    <div className="failure-step-content">
                      <div>
                        <time>{step.time}</time>
                        <strong>{step.title}</strong>
                      </div>
                      <p>{step.detail}</p>
                    </div>
                    <span className="failure-step-result">{step.result}</span>
                  </li>
                ))}
              </ol>

              <div className="failure-result">
                <div>
                  <span>RESULT</span>
                  <strong>recovered automatically</strong>
                </div>
                <p>No application retry. No leaked provider error. One compatible response.</p>
                <span className="failure-result-code">exit 0</span>
              </div>
            </div>

            <footer className="failure-status-bar">
              <span><b>RETRY</b> 1/5</span>
              <span><b>COOLDOWN</b> 1</span>
              <span><b>HEALTHY KEYS</b> 8</span>
              <span><b>CLIENT ERRORS</b> 0</span>
            </footer>
          </article>
        </div>

        <div className="failure-outcomes">
          {outcomes.map((outcome) => (
            <article key={outcome.index}>
              <span>{outcome.index}</span>
              <div>
                <h3>{outcome.title}</h3>
                <p>{outcome.text}</p>
                <code>{outcome.meta}</code>
              </div>
            </article>
          ))}
        </div>
      </div>
    </section>
  );
}
