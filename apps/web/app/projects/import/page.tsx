import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { GitHubConnectionsPage } from "@/components/github/github-connections-page";
import { PreviewBadge } from "@/components/layout/preview-banner";

export default function ImportProjectPage() {
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <SiteHeader workspace />
      <main
        id="main-content"
        className="pg-narrow-container"
        style={{
          flex: 1,
          padding: "32px 0 64px",
          display: "flex",
          flexDirection: "column",
          gap: "28px",
        }}
      >
        <div>
          <div className="breadcrumbs">
            <a href="/projects">← Back to My projects</a>
          </div>
          <h1 className="t-headline-md">Start a project</h1>
          <p className="t-body-md muted" style={{ margin: "6px 0 0" }}>
            Projects always start as private drafts. Publication and source access are configured
            separately after creation.
          </p>
        </div>

        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fit, minmax(300px, 1fr))",
            gap: "24px",
            alignItems: "start",
          }}
        >
          {/* Live Route: Import from GitHub */}
          <section
            className="pg-card"
            style={{
              border: "2px solid var(--link)",
              display: "flex",
              flexDirection: "column",
              gap: "16px",
            }}
          >
            <div>
              <span className="t-label-sm" style={{ color: "var(--success)" }}>
                Live launch route
              </span>
              <h2 className="t-title-lg" style={{ marginTop: "4px" }}>
                Import from GitHub
              </h2>
            </div>
            <p className="t-body-md muted" style={{ margin: 0 }}>
              Keep your Git remote and perform Git writes, issues, and PR reviews on GitHub. PeerGit
              displays authorized code and synchronized evidence.
            </p>
            <GitHubConnectionsPage />
          </section>

          {/* Qualified Pilot: Create on PeerGit */}
          <section
            className="pg-card"
            style={{ display: "flex", flexDirection: "column", gap: "16px" }}
          >
            <div
              style={{
                display: "flex",
                flexWrap: "wrap",
                alignItems: "flex-start",
                justifyContent: "space-between",
                gap: "8px",
              }}
            >
              <div>
                <span className="t-label-sm" style={{ color: "var(--link)" }}>
                  Qualified pilot sample (Stage B1)
                </span>
                <h2 className="t-title-lg" style={{ marginTop: "4px" }}>
                  Create on PeerGit
                </h2>
              </div>
              <PreviewBadge phase="Stage B1" />
            </div>

            <p className="t-body-md muted" style={{ margin: 0 }}>
              Host private source directly in your campus Forgejo cell. Limits: 250 MiB per
              repository, 25 MiB single-file ceiling.
            </p>

            <fieldset
              disabled
              style={{
                border: "1px solid var(--hairline)",
                borderRadius: "12px",
                padding: "16px",
                margin: 0,
                display: "grid",
                gap: "14px",
              }}
            >
              <legend className="t-label-sm muted">Sample configuration</legend>
              <label className="pg-field">
                Repository slug (Northgate State University)
                <input
                  className="pg-input t-mono-sm"
                  defaultValue="shuttle-alert-cli"
                  readOnly
                />
                <span className="t-mono-sm muted">
                  Remote: https://git.northgate.peergit.edu/anair/shuttle-alert-cli.git
                </span>
              </label>

              <div style={{ display: "grid", gap: "8px" }}>
                <span className="t-label-md">Initialization</span>
                <label className="check-row">
                  <input type="radio" name="init" defaultChecked />
                  <span>Add a README starter</span>
                </label>
                <label className="check-row">
                  <input type="radio" name="init" />
                  <span>Start empty (push an existing local repository)</span>
                </label>
              </div>

              <button type="button" className="secondary-button" disabled>
                Create private repository
              </button>
            </fieldset>

            <a className="t-label-md" href="/preview/native">
              Open full PeerGit-hosted repository preview →
            </a>
          </section>
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
