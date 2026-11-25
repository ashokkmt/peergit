import { PreviewBadge } from "@/components/layout/preview-banner";

export default function CampusOverviewPage() {
  return (
    <>
      <div className="section-heading" style={{ marginBottom: 0 }}>
        <div>
          <p className="eyebrow">Institution workspace</p>
          <h1 className="t-headline-md">Campus overview</h1>
          <p className="t-body-md muted" style={{ margin: "4px 0 0" }}>
            Institution-scoped visibility, verification and authorized evidence.
          </p>
        </div>
        <PreviewBadge phase="Phases 6–8 preview" />
      </div>

      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "12px",
        }}
      >
        <div className="button-row">
          <span className="pg-pill">Cohort: CS 340 Software Engineering (Sample)</span>
          <span className="pg-pill">Period: Last 14 days</span>
        </div>
        <div className="pg-pill pg-pill-raised">
          Coverage: 18/20 projects linked · 2 unlinked (off-platform work possible)
        </div>
      </div>

      <div className="pg-callout">
        <span>
          Sample figures only. Institutional roles and aggregate reporting will be wired to
          authorized campus data in Phases 6–8. No individual productivity scores or automatic
          grades.
        </span>
      </div>

      <section aria-label="Cohort metrics" className="pg-grid-4">
        {[
          [
            "Verified participation",
            "72 / 76",
            "Enrolled students in sample cohort",
            undefined,
          ],
          [
            "Recent observed activity",
            "16 / 18",
            "Linked projects in 14d window",
            undefined,
          ],
          [
            "Snapshots verified",
            "14 / 18",
            "Milestone 2 due Nov 28",
            "var(--success)",
          ],
          [
            "Verification backlog",
            "6",
            "Manual affiliation requests",
            "var(--warning)",
          ],
        ].map(([title, value, meta, color]) => (
          <article className="metric-card" key={title}>
            <span>{title}</span>
            <strong style={color ? { color } : undefined}>{value}</strong>
            <span style={{ marginTop: "4px" }}>{meta}</span>
          </article>
        ))}
      </section>

      <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: "8px" }}>
          <h2 className="t-title-lg">Recent authorized receipts</h2>
          <a className="t-label-md" href="/campus/reports">
            Open cohort roster →
          </a>
        </div>
        <div className="table-scroll">
          <table className="data-table">
            <thead>
              <tr>
                <th>Project</th>
                <th>Receipt &amp; SHA</th>
                <th>Timing</th>
                <th>Verification</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td style={{ fontWeight: 500 }}>Campus Shuttle Tracker</td>
                <td className="t-mono-sm tabular-nums">rec_84f19a · 7c1a9de</td>
                <td style={{ color: "var(--success)" }}>Received on time</td>
                <td style={{ color: "var(--success)" }}>Archive verified</td>
              </tr>
              <tr>
                <td style={{ fontWeight: 500 }}>Lab Equipment Scheduler</td>
                <td className="t-mono-sm tabular-nums">rec_71c02b · 3e8a104</td>
                <td style={{ color: "var(--success)" }}>Received on time</td>
                <td style={{ color: "var(--success)" }}>Archive verified</td>
              </tr>
              <tr>
                <td style={{ fontWeight: 500 }}>Transit Kiosk Firmware</td>
                <td className="t-mono-sm muted">—</td>
                <td className="muted">No linked evidence in this period</td>
                <td className="muted">Awaiting submission</td>
              </tr>
            </tbody>
          </table>
        </div>
        <nav className="button-row" aria-label="Campus sections">
          <a className="secondary-button pg-btn-compact" href="/admin/campus-reviews">
            Affiliation verification
          </a>
          <a className="secondary-button pg-btn-compact" href="/campus/reports">
            Cohorts &amp; reports (preview)
          </a>
        </nav>
      </section>
    </>
  );
}
