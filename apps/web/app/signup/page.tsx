import { StackedBrandLogo } from "@/components/layout/brand-logo";
import { AppearanceControl } from "@/components/layout/appearance-control";

export default function SignupPage() {
  return (
    <div className="auth-split">
      <a className="skip-link" href="#signup-content">
        Skip to content
      </a>

      <main id="signup-content" className="auth-main">
        <div className="auth-card-wrap">
          <div
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              gap: "12px",
            }}
          >
            <a className="t-label-md" href="/" style={{ textDecoration: "none" }}>
              ← Back to PeerGit
            </a>
            <div style={{ display: "flex", alignItems: "center", gap: "12px" }}>
              <nav aria-label="Account">
                <a className="t-label-md" href="/login">
                  Sign in
                </a>
              </nav>
              <AppearanceControl />
            </div>
          </div>

          <section className="pg-card" aria-labelledby="signup-title" style={{ display: "flex", flexDirection: "column", gap: "20px", borderRadius: "20px", padding: "32px" }}>
            <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
              <p className="eyebrow">Create your account</p>
              <h1 id="signup-title" className="t-headline-md">
                Start with GitHub
              </h1>
              <p className="t-body-md muted" style={{ margin: 0 }}>
                GitHub confirms your account and contact email. You can explore public pages before
                verifying a campus email.
              </p>
            </div>

            <a
              className="primary-link"
              href="/api/v1/auth/github"
              style={{ width: "100%", minHeight: "48px" }}
            >
              Continue with GitHub <span aria-hidden="true">→</span>
            </a>

            <div className="pg-callout" style={{ flexDirection: "column", gap: "6px" }}>
              <p className="t-label-md" style={{ margin: 0, color: "var(--text)" }}>
                How identity and campus access stay separate
              </p>
              <p className="t-body-sm muted" style={{ margin: 0 }}>
                Campus features unlock after you verify an approved campus email. GitHub repository
                access is requested separately, when you connect a project.
              </p>
            </div>

            <p className="t-body-sm muted" style={{ margin: 0 }}>
              Already have an account? <a href="/login">Log in</a>. By continuing, you agree to the{" "}
              <a href="/terms">Terms of Service</a> and <a href="/privacy">Privacy Policy</a>.
            </p>
          </section>

          <div
            className="pg-card"
            style={{
              padding: "14px 18px",
              display: "flex",
              flexWrap: "wrap",
              alignItems: "center",
              justifyContent: "space-between",
              gap: "12px",
            }}
          >
            <span className="t-body-sm muted">
              Missing verified GitHub email or identity conflict?
            </span>
            <a className="t-label-md" href="/privacy">
              Review account notice
            </a>
          </div>
        </div>
      </main>

      <aside className="auth-companion" aria-label="PeerGit overview">
        <StackedBrandLogo />
        <div style={{ maxWidth: "24rem", display: "flex", flexDirection: "column", gap: "8px" }}>
          <h2 className="t-title-lg">Your campus projects, connected.</h2>
          <p className="t-body-md muted" style={{ margin: 0 }}>
            Keep your GitHub remote, collaborate with verified campus teammates, and capture
            permanent submission receipts.
          </p>
        </div>
      </aside>
    </div>
  );
}
