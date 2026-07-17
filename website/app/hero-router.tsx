"use client";

import Link from "next/link";
import * as m from "motion/react-m";
import { useReducedMotion } from "motion/react";
import type { CSSProperties } from "react";

const bootLines = [
  ["config", "~/.config/modelmux/config.yaml", "loaded"],
  ["providers", "4 adapters", "ready"],
  ["keys", "10 configured", "healthy"],
  ["server", "http://127.0.0.1:8787", "listening"],
] as const;

const copyVariants = {
  hidden: { opacity: 0 },
  visible: {
    opacity: 1,
    transition: { delayChildren: 0.08, staggerChildren: 0.085 },
  },
};

const copyItemVariants = {
  hidden: { opacity: 0, y: 20 },
  visible: {
    opacity: 1,
    y: 0,
    transition: { duration: 0.7, ease: [0.22, 1, 0.36, 1] as const },
  },
};

export function HeroRouter() {
  const reduceMotion = useReducedMotion();

  return (
    <section className="hero-router section-grid">
      <m.div
        className="hero-router-glow"
        aria-hidden="true"
        animate={
          reduceMotion
            ? undefined
            : { opacity: [0.72, 1, 0.72], scale: [0.98, 1.045, 0.98] }
        }
        transition={{ duration: 8, repeat: Infinity, ease: "easeInOut" }}
      />
      <div className="container hero-router-layout">
        <m.div
          className="hero-router-copy"
          variants={copyVariants}
          initial={reduceMotion ? false : "hidden"}
          animate={reduceMotion ? undefined : "visible"}
        >
          <m.div className="eyebrow" variants={copyItemVariants}>
            <span className="pulse-dot" /> Lightweight LLM gateway
          </m.div>
          <m.h1 variants={copyItemVariants}>
            One endpoint.
            <br />
            <span>Every model.</span>
            <br />
            No single key can take you down.
          </m.h1>
          <m.p variants={copyItemVariants}>
            Route OpenAI-compatible traffic across providers and API keys with automatic failover,
            layered limits, usage tracking, and policy-based routing—all from one Go binary.
          </m.p>

          <m.div className="hero-router-actions" variants={copyItemVariants}>
            <Link className="button button-primary" href="/docs/quick-start">
              quick_start →
            </Link>
            <a className="button button-secondary" href="https://github.com/livingdolls/yute-modelmux">
              view_source
            </a>
          </m.div>

          <m.div
            className="hero-router-proof"
            aria-label="ModelMux deployment characteristics"
            variants={copyItemVariants}
          >
            <m.span whileHover={reduceMotion ? undefined : { x: 3 }}><b>01</b> single binary</m.span>
            <m.span whileHover={reduceMotion ? undefined : { x: 3 }}><b>02</b> self-hosted</m.span>
            <m.span whileHover={reduceMotion ? undefined : { x: 3 }}><b>03</b> no required services</m.span>
          </m.div>
        </m.div>

        <m.article
          className="hero-boot-console"
          aria-label="ModelMux server startup demo"
          initial={reduceMotion ? false : { opacity: 0, y: 28, scale: 0.985 }}
          animate={reduceMotion ? undefined : { opacity: 1, y: 0, scale: 1 }}
          whileHover={reduceMotion ? undefined : { y: -4, scale: 1.003 }}
          transition={{ type: "spring", stiffness: 120, damping: 20, delay: 0.16 }}
        >
          <header className="hero-boot-header">
            <div className="hero-boot-dots" aria-hidden="true"><i /><i /><i /></div>
            <code>modelmux start</code>
            <span><i /> demo boot</span>
          </header>

          <div className="hero-boot-body">
            <div className="hero-boot-command"><span>$</span><code>modelmux start</code><i className="terminal-cursor" /></div>

            <div className="hero-boot-lines">
              {bootLines.map(([label, value, state], index) => (
                <div className="hero-boot-line" style={{ "--boot-delay": `${index * 110}ms` } as CSSProperties} key={label}>
                  <span className="hero-boot-check">✓</span>
                  <b>{label}</b>
                  <code>{value}</code>
                  <span>{state}</span>
                </div>
              ))}
            </div>

            <div className="hero-route-topology" aria-label="Application requests route through ModelMux to multiple providers">
              <m.div className="hero-topology-node" whileHover={reduceMotion ? undefined : { y: -2 }}>
                <span>CLIENT</span>
                <strong>your application</strong>
                <small>OpenAI SDK</small>
              </m.div>
              <div className="hero-topology-link"><i /><span>POST /v1/chat</span></div>
              <m.div
                className="hero-topology-core"
                whileHover={reduceMotion ? undefined : { scale: 1.035 }}
                transition={{ type: "spring", stiffness: 260, damping: 18 }}
              >
                <span>&gt;_</span>
                <strong>ModelMux</strong>
                <small>:8787</small>
              </m.div>
              <div className="hero-topology-link hero-topology-link-out"><i /><span>route</span></div>
              <div className="hero-topology-providers">
                {["OpenAI-compatible", "Anthropic", "Gemini", "Custom"].map((provider) => (
                  <m.span
                    key={provider}
                    whileHover={reduceMotion ? undefined : { x: 3, borderColor: "#d787ff" }}
                  >
                    {provider}
                  </m.span>
                ))}
              </div>
            </div>
          </div>

          <footer className="hero-boot-status">
            <span><b>MODE</b> proxy</span>
            <span><b>ROUTES</b> healthy</span>
            <span><b>PORT</b> :8787</span>
          </footer>
        </m.article>
      </div>
    </section>
  );
}
