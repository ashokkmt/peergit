export default function CampusOverviewPage() {
  return (
    <main id="main-content" className="page-container">
      <div className="section-heading">
        <div>
          <p className="eyebrow">Institution workspace</p>
          <h1>Campus overview</h1>
          <p className="muted">
            Institution-scoped visibility, verification and authorized evidence.
          </p>
        </div>
        <span className="prototype-note">
          Will get implemented in a later phase · Phases 6–8 preview
        </span>
      </div>
      <div className="notice">
        Sample figures only. Institutional roles and aggregate reporting will be
        wired to authorized campus data in Phases 6–8. No individual
        productivity scores or automatic grades.
      </div>
      <div className="metric-grid" style={{ marginTop: "1rem" }}>
        {[
          [
            "Verified participation",
            "72 / 76",
            "Enrolled students in the sample cohort",
          ],
          [
            "Recent observed activity",
            "16 / 18",
            "Linked projects · sample window",
          ],
          [
            "Snapshots verified",
            "14 / 18",
            "Receipt and integrity are separate",
          ],
          ["Verification backlog", "6", "Manual affiliation requests · sample"],
        ].map(([title, value, meta]) => (
          <article className="metric-card" key={title}>
            <span>{title}</span>
            <strong>{value}</strong>
            <small className="muted">{meta}</small>
          </article>
        ))}
      </div>
      <section className="content">
        <div className="section-heading">
          <div>
            <p className="eyebrow">Evidence coverage</p>
            <h2>Coverage gaps are visible</h2>
          </div>
        </div>
        <div className="panel">
          <p>
            18 of 20 sample projects are linked to this institution. Two are
            unlinked; off-platform work may exist.
          </p>
          <p className="muted">
            A named student record appears only in an authorized review context.
            Reports suppress small groups under campus policy.
          </p>
        </div>
        <nav className="button-row" aria-label="Campus sections">
          <a className="secondary-button" href="/admin/campus-reviews">
            Affiliation verification
          </a>
          <a className="secondary-button" href="/campus/reports">
            Cohorts &amp; reports (preview)
          </a>
        </nav>
      </section>
    </main>
  );
}
