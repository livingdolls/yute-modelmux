import Link from "next/link";

const tourItems = [
  {
    index: "01",
    title: "Workspace navigation",
    text: "Move between providers, models, groups, chat, keys, logs, config, and AI diagnostics.",
  },
  {
    index: "02",
    title: "Provider operations",
    text: "Inspect adapters and endpoints, then create, update, test, enable, or disable configuration in place.",
  },
  {
    index: "03",
    title: "Context-aware commands",
    text: "The footer exposes only the shortcuts that are valid for the current workspace and selected row.",
  },
  {
    index: "04",
    title: "Native Violet theme",
    text: "The website now uses the same dark violet shell, selection color, lavender text, and status treatment.",
  },
] as const;

export function TuiTour() {
  return (
    <section className="tui-tour-section">
      <div className="container tui-tour-shell">
        <div className="tui-tour-heading">
          <div>
            <span className="section-kicker">ACTUAL TUI WORKSPACE</span>
            <h2>
              Operate the router
              <br />
              where developers already work.
            </h2>
          </div>
          <p>
            This is the real ModelMux dashboard—not a decorative terminal mockup. Configuration,
            health, logs, and routing tools stay available without opening a browser control plane.
          </p>
        </div>

        <div className="tui-tour-layout">
          <figure className="tui-screenshot-frame">
            <div className="tui-screenshot-bar">
              <div className="tui-screenshot-dots" aria-hidden="true"><i /><i /><i /></div>
              <code>modelmux tui --theme violet</code>
              <span>actual product UI</span>
            </div>

            <div className="tui-screenshot-stage">
              <img
                src="https://res.cloudinary.com/dwg1vtwlc/image/upload/v1782697574/repo/Cuplikan_layar_dari_2026-06-29_08-43-28_oxcdu6.png"
                alt="ModelMux Violet terminal dashboard showing the providers workspace"
                loading="lazy"
              />
              <span className="tui-hotspot tui-hotspot-1" aria-hidden="true">01</span>
              <span className="tui-hotspot tui-hotspot-2" aria-hidden="true">02</span>
              <span className="tui-hotspot tui-hotspot-3" aria-hidden="true">03</span>
              <span className="tui-hotspot tui-hotspot-4" aria-hidden="true">04</span>
            </div>

            <figcaption className="tui-keyboard-strip">
              <span><b>q</b> quit</span>
              <span><b>r</b> refresh</span>
              <span><b>/</b> filter</span>
              <span><b>enter</b> inspect</span>
              <span><b>?</b> help</span>
            </figcaption>
          </figure>

          <aside className="tui-tour-notes" aria-label="ModelMux TUI interface tour">
            {tourItems.map((item) => (
              <article key={item.index}>
                <span>{item.index}</span>
                <div>
                  <h3>{item.title}</h3>
                  <p>{item.text}</p>
                </div>
              </article>
            ))}

            <div className="tui-tour-command">
              <span>$</span>
              <code>modelmux tui</code>
              <i aria-hidden="true">↵</i>
            </div>

            <Link className="tui-tour-link" href="/docs/tui">
              explore the TUI reference <span aria-hidden="true">→</span>
            </Link>
          </aside>
        </div>
      </div>
    </section>
  );
}
