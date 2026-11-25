"use client";

import { usePathname } from "next/navigation";
import { BrandLogo } from "@/components/layout/brand-logo";
import { AppearanceControl } from "@/components/layout/appearance-control";

const consoleLinks = [
  ["/campus/overview", "Overview"],
  ["/admin/campus-reviews", "Verification"],
  ["/campus/reports", "Cohorts & reports"],
  ["/preview", "Design previews"],
] as const;

export function CampusConsoleShell({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children: React.ReactNode;
}) {
  const pathname = usePathname();

  const navLinks = consoleLinks.map(([href, label]) => (
    <a key={href} href={href} aria-current={pathname === href ? "page" : undefined}>
      {label}
    </a>
  ));

  return (
    <div className="console-shell">
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>

      <aside className="console-sidebar" aria-label="Institutional console">
        <div style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
          <div style={{ paddingInline: "6px" }}>
            <BrandLogo />
            <p className="t-label-sm muted" style={{ margin: "6px 0 0" }}>
              Campus Console
            </p>
          </div>
          <nav className="console-nav" aria-label="Campus console navigation">
            {navLinks}
          </nav>
        </div>

        <div style={{ padding: "12px 6px 0", borderTop: "1px solid var(--hairline)" }}>
          <p className="t-label-sm" style={{ margin: 0 }}>
            Northgate State University
          </p>
          <p className="t-body-sm muted" style={{ margin: "2px 0 0" }}>
            Institutional workspace
          </p>
        </div>
      </aside>

      <div className="console-main-col">
        <header className="console-topbar">
          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: "10px" }}>
            <details className="mobile-main-menu">
              <summary>Console</summary>
              <nav className="account-menu-panel" aria-label="Mobile console navigation">
                {navLinks}
              </nav>
            </details>
            <span className="t-title-sm">{title}</span>
            {subtitle && <span className="pg-pill pg-pill-raised">{subtitle}</span>}
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
            <a className="t-label-md" href="/" style={{ textDecoration: "none" }}>
              PeerGit home
            </a>
            <AppearanceControl />
          </div>
        </header>

        <main id="main-content" className="console-content">
          {children}
        </main>
      </div>
    </div>
  );
}
