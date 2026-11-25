import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { PreviewBadge } from "@/components/layout/preview-banner";

export default function NativePreviewPage() {
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <SiteHeader workspace />
      <main
        id="main-content"
        className="pg-workspace-container"
        style={{
          flex: 1,
          padding: "32px 0 64px",
          display: "flex",
          flexDirection: "column",
          gap: "24px",
        }}
      >
        <div className="breadcrumbs">
          <a href="/preview">← Design previews</a>
          <span>/</span>
          <span>Stage B1 qualified pilot</span>
        </div>

        <div className="section-heading" style={{ marginBottom: 0 }}>
          <div>
            <h1 className="t-headline-md">PeerGit hosted repositories</h1>
            <p className="t-body-md muted" style={{ margin: "4px 0 0" }}>
              Fictional workspace · Northgate State University · Campus Shuttle Tracker
            </p>
          </div>
          <PreviewBadge phase="Stage B1" />
        </div>

        <div className="pg-grid-2" style={{ alignItems: "start" }}>
          <div style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
            <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "14px" }}>
              <h2 className="t-title-lg">Create a private repository</h2>
              <p className="t-body-sm muted" style={{ margin: 0 }}>
                Limits: 250 MiB per repository, 25 MiB single-file ceiling.
              </p>
              <fieldset
                disabled
                style={{
                  border: "1px solid var(--hairline)",
                  borderRadius: "12px",
                  padding: "16px",
                  margin: 0,
                  display: "grid",
                  gap: "12px",
                }}
              >
                <legend className="t-label-sm muted">Example project</legend>
                <label className="pg-field">
                  Repository name
                  <input className="pg-input t-mono-sm" defaultValue="shuttle-tracker" readOnly />
                </label>
                <label className="pg-field">
                  Initialize repository
                  <select className="pg-select" defaultValue="readme">
                    <option value="readme">With a README</option>
                    <option value="empty">Empty</option>
                  </select>
                </label>
                <button type="button" className="primary-button" disabled>
                  Create repository
                </button>
              </fieldset>
            </section>

            <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
              <h2 className="t-title-md">Scoped Git credential</h2>
              <p className="t-body-sm muted" style={{ margin: 0 }}>
                Example scope · shuttle-tracker · Read and write · Expires in 30 days
              </p>
              <div>
                <button type="button" disabled className="secondary-button pg-btn-compact">
                  Create credential
                </button>
              </div>
            </section>
          </div>

          <div style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
            <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "8px" }}>
                <h2 className="t-title-md">Issue #19 · Fix arrival banner</h2>
                <span className="pg-pill pg-pill-success">Open</span>
              </div>
              <p className="t-body-sm muted" style={{ margin: 0 }}>
                Accessibility · Unassigned · Repository issue (distinct from Project tasks)
              </p>
              <div>
                <button type="button" disabled className="secondary-button pg-btn-compact">
                  Comment
                </button>
              </div>
            </section>

            <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
              <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: "8px" }}>
                <h2 className="t-title-md">Pull request #14 · Add detour alerts</h2>
                <span className="pg-pill pg-pill-warning">Review required</span>
              </div>
              <p className="t-mono-sm muted" style={{ margin: 0 }}>
                feature/route-alerts → main · Head: 9b2e10f
              </p>
              <div className="pg-callout">
                <span>
                  Protected branch rule on <code>main</code>: 1 reviewer approval required after
                  head update.
                </span>
              </div>
              <div className="button-row">
                <button type="button" disabled className="secondary-button pg-btn-compact">
                  Approve
                </button>
                <button type="button" disabled className="secondary-button pg-btn-compact">
                  Merge blocked (1 approval needed)
                </button>
              </div>
            </section>
          </div>
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
