import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { PreviewBadge } from "@/components/layout/preview-banner";

export default async function ProjectWorkPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id: projectID } = await params;
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
          <a href={`/projects/${projectID}`}>← Back to project</a>
          <span>/</span>
          <span>Project work</span>
        </div>

        <div className="section-heading" style={{ marginBottom: 0 }}>
          <div>
            <p className="eyebrow">Team collaboration</p>
            <h1 className="t-headline-md">Tasks &amp; milestones</h1>
            <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
              Milestone 2: Live Route Alerts · Due Nov 28 · 3/5 complete (Sample)
            </p>
          </div>
          <PreviewBadge phase="Phase 5A preview" />
        </div>

        <div className="pg-callout">
          <span>
            Task, milestone, and update records are not wired yet. This preview uses sample content
            and cannot create or change project work.
          </span>
        </div>

        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fit, minmax(300px, 1fr))",
            gap: "24px",
            alignItems: "start",
          }}
        >
          {/* Tasks & Milestones */}
          <section
            className="pg-card"
            style={{ display: "flex", flexDirection: "column", gap: "16px", padding: 0, overflow: "hidden" }}
          >
            <div
              style={{
                padding: "16px 20px",
                borderBottom: "1px solid var(--hairline)",
                display: "flex",
                flexWrap: "wrap",
                alignItems: "center",
                justifyContent: "space-between",
                gap: "12px",
              }}
            >
              <div>
                <h2 className="t-title-md">Tasks</h2>
                <p className="t-body-sm muted" style={{ margin: "2px 0 0" }}>
                  Keyboard-friendly status controls with list view
                </p>
              </div>
              <button type="button" className="secondary-button pg-btn-compact" disabled>
                New task
              </button>
            </div>

            <div style={{ display: "flex", flexDirection: "column" }}>
              {[
                {
                  title: "Verify morning peak route alerts on North Loop",
                  meta: "Linked to PR #14 · Assignee: Asha Nair · Due Nov 26",
                  state: "In review",
                  pillClass: "pg-pill-warning",
                },
                {
                  title: "Document GTFS feed endpoints for route detours",
                  meta: "Linked to docs/alerts.md · Assignee: Samir Patel",
                  state: "To do",
                  pillClass: "",
                },
                {
                  title: "Capture Milestone 2 exact-SHA snapshot",
                  meta: "Receipt rec_84f19a · Assignee: Asha Nair",
                  state: "Done",
                  pillClass: "pg-pill-success",
                },
              ].map((task, idx, arr) => (
                <div
                  key={task.title}
                  style={{
                    padding: "16px 20px",
                    borderBottom: idx < arr.length - 1 ? "1px solid var(--hairline)" : 0,
                    display: "flex",
                    flexWrap: "wrap",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: "12px",
                  }}
                >
                  <div>
                    <p className="t-title-sm" style={{ margin: 0 }}>
                      {task.title}
                    </p>
                    <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                      {task.meta}
                    </p>
                  </div>
                  <span className={`pg-pill ${task.pillClass}`}>{task.state}</span>
                </div>
              ))}
            </div>
          </section>

          {/* Project Updates & Discussions */}
          <section style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
            <div
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                gap: "12px",
              }}
            >
              <h2 className="t-title-lg">Project updates</h2>
              <button type="button" className="secondary-button pg-btn-compact" disabled>
                Post update
              </button>
            </div>

            <article
              className="pg-card"
              style={{ display: "flex", flexDirection: "column", gap: "12px" }}
            >
              <div
                style={{
                  display: "flex",
                  flexWrap: "wrap",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: "8px",
                }}
              >
                <span className="t-label-md">Asha Nair · Sample update</span>
                <span className="pg-pill pg-pill-raised">Audience: Campus</span>
              </div>
              <p className="t-body-md" style={{ margin: 0 }}>
                Live route status endpoints are merged on <code className="t-mono-sm">main</code>{" "}
                (<code className="t-mono-sm">7c1a9de</code>). Looking for one field tester for
                Thursday morning loops.
              </p>
              <div
                style={{
                  paddingTop: "10px",
                  borderTop: "1px solid var(--hairline)",
                  display: "flex",
                  justifyContent: "space-between",
                  fontSize: "13px",
                }}
              >
                <span className="muted">2 sample replies</span>
                <span className="muted">Discussions ship in Phase 5A</span>
              </div>
            </article>
          </section>
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
