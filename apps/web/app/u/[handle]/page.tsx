import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { PreviewBadge } from "@/components/layout/preview-banner";

export default async function PublicProfilePage({
  params,
}: {
  params: Promise<{ handle: string }>;
}) {
  const { handle } = await params;

  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <SiteHeader workspace />
      <main
        id="main-content"
        className="page-container"
        style={{ flex: 1, display: "flex", flexDirection: "column", gap: "24px" }}
      >
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: "12px" }}>
          <PreviewBadge phase="Phase 5D" />
          <a className="t-label-md" href="/preview">
            ← Design previews
          </a>
        </div>

        {/* Top Profile Band */}
        <section
          className="pg-card"
          style={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "flex-start",
            justifyContent: "space-between",
            gap: "20px",
            borderRadius: "20px",
          }}
        >
          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "flex-start", gap: "20px" }}>
            <div className="avatar-lg" aria-hidden="true">
              ES
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: "6px", maxWidth: "38rem" }}>
              <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: "10px" }}>
                <h1 className="t-headline-md">Example Student</h1>
                <span className="t-mono-sm muted">@{handle}</span>
                <span className="pg-pill pg-pill-success">Northgate State University</span>
              </div>
              <p className="t-body-md muted" style={{ margin: 0 }}>
                B.S. Computer Science ’27. Building accessible campus tools, transit telemetry, and
                reliable distributed systems.
              </p>
            </div>
          </div>

          <div className="button-row">
            <button type="button" className="secondary-button pg-btn-compact" disabled>
              Share portfolio
            </button>
            <a className="primary-link pg-btn-compact" href="/account/profile">
              Edit your PeerGit profile
            </a>
          </div>
        </section>

        <div className="repo-layout">
          <aside className="repo-aside" style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
            <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
              <h2 className="t-title-sm">Skills</h2>
              <ul className="skill-list" aria-label="Example skills">
                <li>Accessibility</li>
                <li>TypeScript</li>
                <li>Go</li>
                <li>PostgreSQL</li>
                <li>Testing</li>
              </ul>
            </section>

            <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
              <h2 className="t-title-sm">Links</h2>
              <p className="t-body-sm muted" style={{ margin: 0 }}>
                Verified OAuth identity and opted-in portfolio links appear here for authorized
                audiences.
              </p>
            </section>
          </aside>

          <div className="repo-main" style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
            <div
              role="tablist"
              aria-label="Profile sections"
              style={{
                display: "flex",
                gap: "8px",
                borderBottom: "1px solid var(--hairline)",
              }}
            >
              <span
                role="tab"
                aria-selected="true"
                style={{
                  padding: "10px 14px",
                  borderBottom: "2px solid var(--link)",
                  fontWeight: 600,
                  fontSize: "14px",
                }}
              >
                Projects
              </span>
              <span role="tab" aria-selected="false" className="muted" style={{ padding: "10px 14px", fontSize: "14px" }}>
                Contributions
              </span>
              <span role="tab" aria-selected="false" className="muted" style={{ padding: "10px 14px", fontSize: "14px" }}>
                Portfolio
              </span>
            </div>

            <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "10px" }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "8px" }}>
                <h2 className="t-title-md">Campus Shuttle Tracker</h2>
                <span className="t-label-sm" style={{ color: "var(--success)" }}>
                  Public showcase · Sample
                </span>
              </div>
              <p className="t-body-md muted" style={{ margin: 0 }}>
                Example role · Field tester — Tested arrival announcements with screen readers and
                documented edge-case delays.
              </p>
              <p className="t-body-sm muted" style={{ margin: 0 }}>
                Snapshot <code className="t-mono-sm">rec_84f19a</code> · 18 observed commits
              </p>
            </section>

            <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
              <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: "8px" }}>
                <h2 className="t-title-md">Authorized contribution record</h2>
                <span className="t-label-sm muted">
                  Source: GitHub + PeerGit · Sample range
                </span>
              </div>
              <div className="pg-grid-3">
                <div className="pg-card-canvas">
                  <span className="t-label-sm muted">Observed commits</span>
                  <strong className="t-headline-md tabular-nums" style={{ display: "block", marginTop: "4px" }}>
                    24
                  </strong>
                </div>
                <div className="pg-card-canvas">
                  <span className="t-label-sm muted">PR reviews</span>
                  <strong className="t-headline-md tabular-nums" style={{ display: "block", marginTop: "4px" }}>
                    9
                  </strong>
                </div>
                <div className="pg-card-canvas">
                  <span className="t-label-sm muted">Campus snapshots</span>
                  <strong className="t-headline-md tabular-nums" style={{ display: "block", marginTop: "4px" }}>
                    3
                  </strong>
                </div>
              </div>
            </section>
          </div>
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
