import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { PreviewBadge } from "@/components/layout/preview-banner";

const previews = [
  ["Student workspace", "/home", "Phase 5A–5C"],
  ["Inbox and messages", "/inbox", "Phases 5B–5C"],
  ["Tasks and milestones", "/projects/sample/work", "Phase 5A"],
  ["Student portfolio", "/u/example-student", "Phase 5D"],
  ["Campus overview", "/campus/overview", "Phases 6–8"],
  ["Cohorts and reports", "/campus/reports", "Phases 6–8"],
  ["PeerGit hosted repositories", "/preview/native", "Stage B1"],
];

export default function PreviewPage() {
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <SiteHeader />
      <main
        id="main-content"
        className="page-container"
        style={{ flex: 1, display: "flex", flexDirection: "column", gap: "24px" }}
      >
        <div className="section-heading" style={{ marginBottom: 0 }}>
          <div>
            <p className="eyebrow">Roadmap previews</p>
            <h1 className="t-headline-md">Design previews</h1>
          </div>
          <PreviewBadge />
        </div>
        <p className="t-body-md muted" style={{ margin: 0 }}>
          Will get implemented in a later phase. These screens use fictional examples to demonstrate
          planned collaboration, institutional reporting, and qualified-pilot native hosting.
        </p>

        <div className="pg-grid-3">
          {previews.map(([name, href, phase]) => (
            <a key={href} className="pg-card-interactive" href={href}>
              <div>
                <span className="t-label-sm" style={{ color: "var(--link)" }}>
                  {phase}
                </span>
                <h2 className="t-title-md" style={{ marginTop: "4px" }}>
                  {name}
                </h2>
              </div>
              <span className="t-label-md" style={{ color: "var(--link)" }}>
                Open preview →
              </span>
            </a>
          ))}
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
