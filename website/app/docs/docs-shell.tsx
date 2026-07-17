import Link from "next/link";
import { docNavigation, type DocPage } from "./docs-content";

export function DocsShell({ page }: { page: DocPage }) {
  return (
    <div className="docs-shell">
      <aside className="docs-sidebar">
        <div className="docs-sidebar-inner" data-lenis-prevent>
          <div className="docs-index-label">
            <span>docs://modelmux</span>
            <strong>INDEX</strong>
          </div>

          <nav aria-label="Documentation navigation">
            {docNavigation.map((group) => (
              <div className="docs-nav-group" key={group.title}>
                <strong>{group.title}</strong>
                {group.links.map((link) => {
                  const active = link.href === page.href;
                  return (
                    <Link
                      className={active ? "active" : undefined}
                      href={link.href}
                      aria-current={active ? "page" : undefined}
                      key={link.href}
                    >
                      {active ? "> " : "  "}{link.label}
                    </Link>
                  );
                })}
              </div>
            ))}
          </nav>
        </div>
      </aside>

      <article className="docs-content">
        <details className="docs-mobile-nav" data-lenis-prevent>
          <summary>Open documentation index</summary>
          <div>
            {docNavigation.map((group) => (
              <section key={group.title}>
                <strong>{group.title}</strong>
                {group.links.map((link) => (
                  <Link href={link.href} key={link.href}>{link.label}</Link>
                ))}
              </section>
            ))}
          </div>
        </details>

        <div className="docs-breadcrumb">
          <Link href="/docs">Docs</Link>
          <span>/</span>
          <span>{page.section}</span>
          <span>/</span>
          <strong>{page.title}</strong>
        </div>

        <header className="docs-title">
          <span className="section-kicker">{page.section}</span>
          <h1>{page.title}</h1>
          <p>{page.description}</p>
        </header>

        <div className="docs-prose">{page.body}</div>

        {(page.previous || page.next) && (
          <nav className="docs-pagination" aria-label="Documentation pagination">
            {page.previous ? (
              <Link href={page.previous.href}>
                <small>← Previous</small>
                <strong>{page.previous.title}</strong>
              </Link>
            ) : <span />}
            {page.next ? (
              <Link href={page.next.href}>
                <small>Next →</small>
                <strong>{page.next.title}</strong>
              </Link>
            ) : <span />}
          </nav>
        )}
      </article>

      <aside className="docs-toc" data-lenis-prevent>
        <strong>ON THIS PAGE</strong>
        {page.toc.map((item) => (
          <a href={`#${item.id}`} key={item.id}>{item.label}</a>
        ))}
        <a className="docs-edit-link" href="https://github.com/livingdolls/yute-modelmux">VIEW SOURCE ↗</a>
      </aside>
    </div>
  );
}
