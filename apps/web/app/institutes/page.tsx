import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";

export default function InstitutesPage() {
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <SiteHeader />
      <main id="main-content" className="page-container" style={{ flex: 1, display: "flex", flexDirection: "column", gap: "48px" }}>
        <section style={{ maxWidth: "48rem", display: "flex", flexDirection: "column", gap: "14px" }}>
          <span className="t-label-md" style={{ color: "var(--link)" }}>
            Institutional overview
          </span>
          <h1 className="t-headline-lg">
            Verify affiliation, supervise academic work, and review authorized evidence.
          </h1>
          <p className="t-body-lg muted" style={{ margin: 0 }}>
            Colleges operate a separate workspace scoped strictly to their tenant. PeerGit surfaces
            verifiable snapshots and coverage gaps without productivity scores or automatic grades.
          </p>
          <div style={{ paddingTop: "6px" }}>
            <a className="primary-link" href="/campus/overview">
              See the example campus workspace
            </a>
          </div>
        </section>

        <section aria-label="Institutional workflow" className="pg-grid-4">
          <article className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
            <span className="t-label-sm" style={{ color: "var(--link)" }}>
              1 · Verify affiliation
            </span>
            <h2 className="t-title-sm">Mailbox or admin review</h2>
            <p className="t-body-sm muted" style={{ margin: 0 }}>
              Confirm campus membership separately from personal GitHub accounts.
            </p>
          </article>
          <article className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
            <span className="t-label-sm" style={{ color: "var(--link)" }}>
              2 · Organize programs
            </span>
            <h2 className="t-title-sm">Courses &amp; cohorts</h2>
            <p className="t-body-sm muted" style={{ margin: 0 }}>
              Group capstones, labs, and innovation programs by term.
            </p>
          </article>
          <article className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
            <span className="t-label-sm" style={{ color: "var(--link)" }}>
              3 · Review evidence
            </span>
            <h2 className="t-title-sm">Exact-SHA receipts</h2>
            <p className="t-body-sm muted" style={{ margin: 0 }}>
              Inspect authorized snapshots with coverage and freshness labels.
            </p>
          </article>
          <article className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
            <span className="t-label-sm" style={{ color: "var(--link)" }}>
              4 · Share outcomes
            </span>
            <h2 className="t-title-sm">Consented showcases</h2>
            <p className="t-body-sm muted" style={{ margin: 0 }}>
              Publish approved outcomes with small-group privacy suppression.
            </p>
          </article>
        </section>

        <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
          <h2 className="t-title-lg">What each institutional role can see</h2>
          <div className="table-scroll">
            <table className="data-table">
              <thead>
                <tr>
                  <th>Role</th>
                  <th>Permitted scope</th>
                  <th>Explicit boundary</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td style={{ fontWeight: 500 }}>Campus Administrator</td>
                  <td className="muted">
                    Affiliation queue, cohort aggregates, consented showcase editor
                  </td>
                  <td className="muted">
                    Cannot read private source code or direct messages without grant
                  </td>
                </tr>
                <tr>
                  <td style={{ fontWeight: 500 }}>Faculty / Evaluator</td>
                  <td className="muted">
                    Assigned courses, submitted SHA receipts, qualitative feedback
                  </td>
                  <td className="muted">
                    Scoped strictly to assigned cohorts and submission windows
                  </td>
                </tr>
                <tr>
                  <td style={{ fontWeight: 500 }}>Program Staff</td>
                  <td className="muted">
                    Program milestones, conflict-of-interest review queue, exports
                  </td>
                  <td className="muted">
                    Small groups suppressed in both screen and CSV exports
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <p className="t-body-sm muted" style={{ margin: 0 }}>
            Signals support a conversation; they do not grade a student. Authorized reviewers
            consider off-platform work and context alongside recorded commits, reviews, and
            snapshots.
          </p>
        </section>
      </main>
      <SiteFooter />
    </>
  );
}
