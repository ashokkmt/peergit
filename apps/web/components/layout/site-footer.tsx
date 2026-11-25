import { BrandLogo } from "./brand-logo";

export function SiteFooter() {
  return (
    <footer className="pg-footer">
      <div className="pg-footer-grid">
        <div style={{ display: "flex", flexDirection: "column", gap: "10px" }}>
          <BrandLogo />
          <p className="t-body-sm muted" style={{ maxWidth: "36ch", margin: 0 }}>
            Campus project discovery, team collaboration, and verifiable engineering records.
          </p>
        </div>
        <nav aria-label="Product and institutes">
          <span className="t-label-md" style={{ color: "var(--text)" }}>
            Platform
          </span>
          <a href="/explore">Explore projects</a>
          <a href="/institutes">Institute overview</a>
          <a href="/preview">Design previews</a>
        </nav>
        <nav aria-label="Support and legal">
          <span className="t-label-md" style={{ color: "var(--text)" }}>
            Support &amp; Policy
          </span>
          <a href="/privacy">Privacy policy</a>
          <a href="/terms">Terms of service</a>
          <a href="/onboarding">Campus verification</a>
        </nav>
      </div>
    </footer>
  );
}
