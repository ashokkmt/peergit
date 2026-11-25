import { StackedBrandLogo } from "@/components/layout/brand-logo";
import { AppearanceControl } from "@/components/layout/appearance-control";

export default function LoginPage() {
  return (
    <div className="auth-split">
      <a className="skip-link" href="#login-content">
        Skip to content
      </a>

      <main id="login-content" className="auth-main">
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
                <a className="t-label-md" href="/signup">
                  Join PeerGit
                </a>
              </nav>
              <AppearanceControl />
            </div>
          </div>

          <section className="pg-card" aria-labelledby="login-title" style={{ display: "flex", flexDirection: "column", gap: "20px", borderRadius: "20px", padding: "32px" }}>
            <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
              <p className="eyebrow">Welcome back</p>
              <h1 id="login-title" className="t-headline-md">
                Log in to PeerGit
              </h1>
              <p className="t-body-md muted" style={{ margin: 0 }}>
                Use the GitHub account linked to your PeerGit profile.
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
                GitHub signs you in and attributes your commits. Campus discovery and repository
                imports depend on your verified campus membership.
              </p>
            </div>

            <p className="t-body-sm muted" style={{ margin: 0 }}>
              New to PeerGit? <a href="/signup">Create an account</a>. By continuing, you agree to
              the <a href="/terms">Terms of Service</a> and <a href="/privacy">Privacy Policy</a>.
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
