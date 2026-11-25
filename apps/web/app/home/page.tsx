import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { MyProjects } from "@/components/projects/my-projects";
import { PreviewBadge } from "@/components/layout/preview-banner";

export default function StudentHomePage() {
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <SiteHeader workspace />
      <main
        id="main-content"
        className="pg-workspace-container"
        style={{ flex: 1, padding: "32px 0 64px", display: "flex", flexDirection: "column", gap: "32px" }}
      >
        <div className="section-heading" style={{ marginBottom: 0 }}>
          <div>
            <h1 className="t-headline-md">Student workspace</h1>
            <p className="t-body-md muted" style={{ margin: "4px 0 0" }}>
              Your campus projects, action items, and workspace activity.
            </p>
          </div>
          <div className="button-row">
            <a className="primary-link pg-btn-compact" href="/projects/import">
              + Create project
            </a>
            <a className="secondary-button pg-btn-compact" href="/account/profile">
              View your profile
            </a>
          </div>
        </div>

        {/* Actionable Items Preview */}
        <section aria-label="Actionable items" style={{ display: "flex", flexDirection: "column", gap: "14px" }}>
          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: "10px" }}>
            <h2 className="t-title-md">Needs your attention</h2>
            <PreviewBadge phase="Phase 5A–5B sample" />
          </div>
          <div className="pg-grid-3">
            <article className="pg-card" style={{ display: "flex", flexDirection: "column", justifyContent: "space-between", gap: "12px" }}>
              <div>
                <span className="t-label-sm" style={{ color: "var(--warning)" }}>
                  Review requested · Sample
                </span>
                <h3 className="t-title-sm" style={{ marginTop: "4px" }}>
                  PR #14 · Add live route status
                </h3>
                <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                  Campus Shuttle Tracker · feature/route-alerts → main
                </p>
              </div>
              <a className="t-label-md" href="/projects/sample/work">
                Preview project work →
              </a>
            </article>

            <article className="pg-card" style={{ display: "flex", flexDirection: "column", justifyContent: "space-between", gap: "12px" }}>
              <div>
                <span className="t-label-sm" style={{ color: "var(--link)" }}>
                  Team application · Sample
                </span>
                <h3 className="t-title-sm" style={{ marginTop: "4px" }}>
                  Dev Patel applied for QA &amp; Field Tester
                </h3>
                <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                  Campus Shuttle Tracker · 1 seat remaining
                </p>
              </div>
              <a className="t-label-md" href="/inbox">
                Preview inbox →
              </a>
            </article>

            <article className="pg-card" style={{ display: "flex", flexDirection: "column", justifyContent: "space-between", gap: "12px" }}>
              <div>
                <span className="t-label-sm" style={{ color: "var(--success)" }}>
                  Upcoming submission · Sample
                </span>
                <h3 className="t-title-sm" style={{ marginTop: "4px" }}>
                  CS 340 Milestone 2 Snapshot
                </h3>
                <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                  Due Nov 28 · Last verified at 7c1a9de
                </p>
              </div>
              <a className="t-label-md" href="/projects">
                Open my projects →
              </a>
            </article>
          </div>
        </section>

        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fit, minmax(300px, 1fr))",
            gap: "24px",
            alignItems: "start",
          }}
        >
          <MyProjects />

          <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
            <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: "8px" }}>
              <h2 className="t-title-md">Campus activity</h2>
              <PreviewBadge phase="Phase 5C" />
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: "14px" }}>
              <div style={{ paddingBottom: "12px", borderBottom: "1px solid var(--hairline)" }}>
                <p className="t-body-md" style={{ margin: 0 }}>
                  <strong>Campus Shuttle Tracker</strong> captured snapshot{" "}
                  <code className="t-mono-sm">rec_84f19a</code>
                </p>
                <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                  Sample · Archive verified
                </p>
              </div>
              <div style={{ paddingBottom: "12px", borderBottom: "1px solid var(--hairline)" }}>
                <p className="t-body-md" style={{ margin: 0 }}>
                  <strong>Bike Share Telemetry</strong> opened 2 roles for Spring mapping
                </p>
                <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                  Sample · Northgate Campus
                </p>
              </div>
              <div>
                <p className="t-body-md" style={{ margin: 0 }}>
                  <strong>CS 340 Software Engineering</strong> posted Milestone 2 deadline
                </p>
                <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                  Sample · Prof. Elena Vance
                </p>
              </div>
            </div>
          </section>
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
