import type { Metadata } from "next";
import Link from "next/link";
import { GeistMono } from "geist/font/mono";
import "lenis/dist/lenis.css";
import "./globals.css";
import { ScrollProgress } from "./scroll-progress";
import { SiteMotionProvider } from "./site-motion-provider";

export const metadata: Metadata = {
  title: {
    default: "ModelMux — Reliable LLM routing",
    template: "%s | ModelMux",
  },
  description:
    "A lightweight, self-hosted LLM gateway with key rotation, automatic failover, rate limiting, observability, and policy-based routing.",
  metadataBase: new URL("https://modelmux.dev"),
  openGraph: {
    title: "ModelMux — Reliable LLM routing",
    description:
      "One OpenAI-compatible endpoint for reliable routing across LLM providers and API keys.",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "ModelMux — Reliable LLM routing",
    description:
      "One OpenAI-compatible endpoint for reliable routing across LLM providers and API keys.",
  },
};

const GithubIcon = () => (
  <svg viewBox="0 0 24 24" aria-hidden="true">
    <path
      fill="currentColor"
      d="M12 .8a11.2 11.2 0 0 0-3.54 21.83c.56.1.77-.24.77-.54v-2.1c-3.14.68-3.8-1.34-3.8-1.34-.51-1.31-1.25-1.66-1.25-1.66-1.03-.7.08-.69.08-.69 1.13.08 1.73 1.16 1.73 1.16 1.01 1.73 2.65 1.23 3.3.94.1-.73.4-1.23.72-1.52-2.51-.29-5.15-1.26-5.15-5.58 0-1.24.44-2.25 1.16-3.04-.12-.29-.5-1.44.11-3 0 0 .95-.3 3.08 1.16A10.7 10.7 0 0 1 12 6.04c.95 0 1.9.13 2.8.38 2.14-1.45 3.08-1.16 3.08-1.16.62 1.56.23 2.71.12 3 .72.8 1.8 1.15 3.04 0 4.34-2.64 5.29-5.16 5.57.4.35.76 1.04.76 2.1v3.12c0 .3.2.65.78.54A11.2 11.2 0 0 0 12 .8Z"
    />
  </svg>
);

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" className={GeistMono.variable}>
      <body>
        <SiteMotionProvider>
          <ScrollProgress />

          <header className="site-header">
            <div className="container nav-shell">
              <Link className="brand" href="/" aria-label="ModelMux home">
                <span className="brand-mark" aria-hidden="true">
                  <i />
                  <i />
                  <i />
                </span>
                <span>modelmux</span>
              </Link>

              <nav className="desktop-nav" aria-label="Main navigation">
                <Link href="/#features">features</Link>
                <Link href="/#architecture">architecture</Link>
                <Link href="/docs">docs</Link>
                <a href="https://github.com/livingdolls/yute-modelmux/releases">download</a>
              </nav>

              <div className="nav-actions">
                <a
                  className="icon-link"
                  href="https://github.com/livingdolls/yute-modelmux"
                  aria-label="View ModelMux on GitHub"
                >
                  <GithubIcon />
                </a>
                <Link className="button button-small button-primary" href="/docs/quick-start">
                  quick_start →
                </Link>
              </div>
            </div>
          </header>

          <main>{children}</main>

          <footer className="site-footer">
            <div className="container footer-grid">
              <div>
                <Link className="brand" href="/">
                  <span className="brand-mark" aria-hidden="true">
                    <i />
                    <i />
                    <i />
                  </span>
                  <span>modelmux</span>
                </Link>
                <p>A lightweight LLM gateway built for reliable, self-hosted inference routing.</p>
              </div>
              <div className="footer-links">
                <div>
                  <strong>product</strong>
                  <Link href="/#features">features</Link>
                  <Link href="/#architecture">architecture</Link>
                  <a href="https://github.com/livingdolls/yute-modelmux/releases">releases</a>
                </div>
                <div>
                  <strong>resources</strong>
                  <Link href="/docs">documentation</Link>
                  <a href="https://github.com/livingdolls/yute-modelmux">github</a>
                  <a href="https://github.com/livingdolls/yute-modelmux/issues">issues</a>
                </div>
              </div>
            </div>
            <div className="container footer-bottom">
              <span>open_source=true</span>
              <span>runtime=go</span>
            </div>
          </footer>
        </SiteMotionProvider>
      </body>
    </html>
  );
}
