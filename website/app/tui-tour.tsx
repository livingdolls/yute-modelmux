"use client";

import Link from "next/link";
import * as m from "motion/react-m";
import { useReducedMotion } from "motion/react";

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
  const reduceMotion = useReducedMotion();

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
          <m.figure
            className="tui-screenshot-frame"
            whileHover={reduceMotion ? undefined : { y: -3, scale: 1.002 }}
            transition={{ type: "spring", stiffness: 150, damping: 20 }}
          >
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
              {["01", "02", "03", "04"].map((hotspot, index) => (
                <m.span
                  className={`tui-hotspot tui-hotspot-${index + 1}`}
                  aria-hidden="true"
                  key={hotspot}
                  animate={reduceMotion ? undefined : { opacity: [0.78, 1, 0.78] }}
                  transition={{ duration: 2.4, repeat: Infinity, delay: index * 0.28 }}
                >
                  {hotspot}
                </m.span>
              ))}
            </div>

            <figcaption className="tui-keyboard-strip">
              <span><b>q</b> quit</span>
              <span><b>r</b> refresh</span>
              <span><b>/</b> filter</span>
              <span><b>enter</b> inspect</span>
              <span><b>?</b> help</span>
            </figcaption>
          </m.figure>

          <aside className="tui-tour-notes" aria-label="ModelMux TUI interface tour">
            {tourItems.map((item) => (
              <m.article
                key={item.index}
                whileHover={reduceMotion ? undefined : { x: 4 }}
                transition={{ type: "spring", stiffness: 260, damping: 22 }}
              >
                <span>{item.index}</span>
                <div>
                  <h3>{item.title}</h3>
                  <p>{item.text}</p>
                </div>
              </m.article>
            ))}

            <m.div
              className="tui-tour-command"
              whileHover={reduceMotion ? undefined : { scale: 1.01 }}
            >
              <span>$</span>
              <code>modelmux tui</code>
              <i aria-hidden="true">↵</i>
            </m.div>

            <Link className="tui-tour-link" href="/docs/tui">
              explore the TUI reference <span aria-hidden="true">→</span>
            </Link>
          </aside>
        </div>
      </div>
    </section>
  );
}
