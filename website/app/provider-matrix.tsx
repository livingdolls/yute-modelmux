const providers = [
  {
    name: "OpenAI",
    id: "openai",
    adapter: "openai-compatible",
    capabilities: "stream · tools · json",
    keys: "3 / 3",
    health: "online",
    latency: "842ms",
  },
  {
    name: "Anthropic",
    id: "anthropic",
    adapter: "native-adapter",
    capabilities: "stream · tools · cache",
    keys: "2 / 2",
    health: "online",
    latency: "1.12s",
  },
  {
    name: "Gemini",
    id: "gemini",
    adapter: "native-adapter",
    capabilities: "stream · vision · tools",
    keys: "1 / 2",
    health: "degraded",
    latency: "690ms",
  },
  {
    name: "Custom API",
    id: "custom",
    adapter: "passthrough",
    capabilities: "openai schema · custom base URL",
    keys: "4 / 4",
    health: "online",
    latency: "410ms",
  },
] as const;

const events = [
  { time: "12:42:08", tone: "neutral", text: "request received", detail: "model=high-price" },
  { time: "12:42:08", tone: "neutral", text: "selecting key", detail: "mimo-primary" },
  { time: "12:42:09", tone: "warning", text: "upstream returned 429", detail: "retryable=true" },
  { time: "12:42:09", tone: "warning", text: "cooldown applied", detail: "expires_in=300s" },
  { time: "12:42:09", tone: "neutral", text: "failover", detail: "claude-team" },
  { time: "12:42:10", tone: "success", text: "request completed", detail: "status=200" },
] as const;

const summaries = [
  { label: "providers", value: "4", suffix: "connected" },
  { label: "healthy keys", value: "10", suffix: "of 11" },
  { label: "success rate", value: "99.7%", suffix: "demo" },
  { label: "average latency", value: "842ms", suffix: "demo" },
] as const;

export function ProviderMatrix() {
  return (
    <section className="provider-matrix-section" aria-labelledby="providers-title">
      <div className="container provider-matrix-shell">
        <div className="provider-matrix-heading">
          <div>
            <span className="section-kicker">connected providers</span>
            <h2 id="providers-title">One interface. Multiple upstreams. Live failover.</h2>
          </div>
          <p>
            ModelMux evaluates provider and key health before every request, applies configured limits,
            and moves traffic to the next eligible target when an upstream becomes unavailable.
          </p>
        </div>

        <div className="provider-summary-bar" aria-label="Provider demonstration summary">
          {summaries.map((item) => (
            <div className="provider-summary-item" key={item.label}>
              <span>{item.label}</span>
              <strong>
                {item.value}
                <em>{item.suffix}</em>
              </strong>
            </div>
          ))}
        </div>

        <div className="provider-runtime-grid">
          <div className="provider-panel">
            <div className="provider-panel-header">
              <code>modelmux providers --status</code>
              <span><i className="provider-live-dot" /> simulated runtime view</span>
            </div>

            <div className="provider-table" role="table" aria-label="Supported provider status example">
              <div className="provider-table-head" role="row">
                <span>PROVIDER</span>
                <span>ADAPTER</span>
                <span>KEYS</span>
                <span>HEALTH</span>
                <span>LATENCY</span>
              </div>

              {providers.map((provider) => (
                <div className="provider-table-row" role="row" key={provider.id}>
                  <div className="provider-name" role="cell">
                    <strong>{provider.name}</strong>
                    <small>id={provider.id}</small>
                  </div>
                  <div className="provider-adapter" role="cell">
                    <strong>{provider.adapter}</strong>
                    <small>{provider.capabilities}</small>
                  </div>
                  <span className="provider-key-count" role="cell"><b>{provider.keys}</b> active</span>
                  <span className={`provider-health provider-health-${provider.health}`} role="cell">
                    {provider.health}
                  </span>
                  <span className="provider-latency" role="cell"><b>{provider.latency}</b></span>
                </div>
              ))}
            </div>
          </div>

          <aside className="routing-activity-panel" aria-label="Example routing activity">
            <div className="routing-activity-header">
              <code>modelmux logs --follow</code>
              <span>route trace</span>
            </div>
            <div className="routing-activity-body">
              <div className="routing-request-id">
                <div>
                  <span>REQUEST_ID</span>
                  <strong>req_01JZ7M2A9F</strong>
                </div>
                <span>stream=true</span>
              </div>

              <ol className="routing-log-list">
                {events.map((event) => (
                  <li className={`routing-log-item routing-log-item-${event.tone}`} key={`${event.time}-${event.text}`}>
                    <time>{event.time}</time>
                    <span className="routing-log-marker">›</span>
                    <span><strong>{event.text}</strong> · {event.detail}</span>
                  </li>
                ))}
              </ol>

              <div className="routing-result">
                <span>final_target</span>
                <strong>claude-team</strong>
                <span>1.12s</span>
              </div>
            </div>
          </aside>
        </div>

        <div className="provider-topology" aria-label="Example routing topology">
          <div className="provider-topology-line" aria-hidden="true"><i /><i /><i /></div>
          <div className="provider-topology-node">
            <span>incoming</span>
            <strong>POST /v1/chat</strong>
          </div>
          <div className="provider-topology-node provider-topology-core">
            <span>policy + health</span>
            <strong>modelmux :8787</strong>
          </div>
          <div className="provider-topology-targets">
            {providers.map((provider) => (
              <div
                className={`provider-topology-target provider-topology-target-${provider.health}`}
                key={provider.id}
              >
                <strong>{provider.name}</strong>
                <span>{provider.health}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
