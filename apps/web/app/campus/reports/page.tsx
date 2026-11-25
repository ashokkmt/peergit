import { PreviewBadge } from "@/components/layout/preview-banner";

export default function CampusReportsPage() {
  return (
    <>
      <div className="breadcrumbs" style={{ marginBottom: 0 }}>
        <a href="/campus/overview">Campus overview</a>
        <span>/</span>
        <span>Cohorts, verification &amp; reports</span>
      </div>

      <div className="section-heading" style={{ marginBottom: 0 }}>
        <div>
          <p className="eyebrow">Institution console</p>
          <h1 className="t-headline-md">Cohorts, verification &amp; reports</h1>
        </div>
        <PreviewBadge phase="Phases 6–8 preview" />
      </div>

      <div className="pg-callout">
        <span>
          No report exports or named student analytics are connected yet. Any future reports must
          declare their scope and coverage and suppress groups smaller than campus policy allows.
        </span>
      </div>

      <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "14px" }}>
        <div>
          <h2 className="t-title-lg">CS 340 Cohort Roster (Sample)</h2>
          <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
            No productivity scores or automatic grades. Assessors inspect linked records alongside
            off-platform work.
          </p>
        </div>
        <div className="table-scroll">
          <table className="data-table">
            <thead>
              <tr>
                <th>Student</th>
                <th>Project</th>
                <th>Linked contribution types</th>
                <th>Last observed</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td style={{ fontWeight: 500 }}>Example student</td>
                <td>Campus Shuttle Tracker</td>
                <td>Commits, PR reviews, snapshots</td>
                <td className="muted">2 hours ago (sample)</td>
              </tr>
              <tr>
                <td style={{ fontWeight: 500 }}>Example student</td>
                <td>Not linked</td>
                <td className="muted">No linked evidence in this period</td>
                <td className="muted">Coverage gap</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <div className="pg-grid-2">
        <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "8px" }}>
            <h2 className="t-title-md">Affiliation verification queue</h2>
            <span className="t-label-sm" style={{ color: "var(--warning)" }}>
              Fresh MFA required
            </span>
          </div>
          <p className="t-body-sm muted" style={{ margin: 0 }}>
            Authorized campus administrators review manual affiliation requests with a required
            reason and audited decision.
          </p>
          <div>
            <a className="secondary-button pg-btn-compact" href="/admin/campus-reviews">
              Open live verification queue →
            </a>
          </div>
        </section>

        <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
          <h2 className="t-title-md">Aggregate report export</h2>
          <p className="t-body-sm muted" style={{ margin: 0 }}>
            Small groups (&lt; 5 students) are suppressed in both screen previews and CSV exports
            under campus privacy policy.
          </p>
          <div
            className="pg-card-canvas"
            style={{
              padding: "12px 14px",
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              gap: "12px",
            }}
          >
            <span className="t-label-sm">Fall 2026 Cohort Summary · CSV</span>
            <button type="button" className="secondary-button pg-btn-compact" disabled>
              Generate export
            </button>
          </div>
        </section>
      </div>
    </>
  );
}
