import { SiteHeader } from "@/components/layout/site-header";

export default function InstitutesPage() {
  return (
    <>
      <SiteHeader />
      <main id="main-content" className="page-container">
        <section className="institute-banner">
          <p className="eyebrow">PeerGit for institutes</p>
          <h1>Give campus projects a trusted place to grow.</h1>
          <p className="intro">
            Verify affiliation, organize programs, and review authorized project
            evidence inside a workspace scoped to your institution.
          </p>
          <a className="primary-link" href="/campus/overview">
            See the example campus workspace
          </a>
        </section>
        <section className="content">
          <div className="section-heading">
            <div>
              <p className="eyebrow">Designed for campus teams</p>
              <h2>Support student work with clear context</h2>
            </div>
          </div>
          <div className="steps">
            <article>
              <span>01 · AFFILIATION</span>
              <h3>Verify campus membership</h3>
              <p>
                Students verify through a campus mailbox or an authorized
                administrator review. GitHub identity remains a separate account
                connection.
              </p>
            </article>
            <article>
              <span>02 · PROGRAMS</span>
              <h3>Organize cohorts and projects</h3>
              <p>
                Course and cohort spaces group project work by an authorized
                academic context when those phases are enabled.
              </p>
            </article>
            <article>
              <span>03 · EVIDENCE</span>
              <h3>Review what is actually recorded</h3>
              <p>
                See source, receipt time, integrity and coverage. Missing linked
                evidence stays visible as a gap, not a verdict.
              </p>
            </article>
            <article>
              <span>04 · PRIVACY</span>
              <h3>Share only by permission</h3>
              <p>
                Students choose what appears in a showcase. Campus detail views
                and exports require their own access grants.
              </p>
            </article>
          </div>
        </section>
        <section className="panel">
          <p className="eyebrow">Privacy by design</p>
          <h2>Signals support a conversation; they do not grade a student.</h2>
          <p className="muted">
            PeerGit shows recorded commits, reviews, tasks and snapshots with
            source and freshness. It does not generate productivity scores,
            individual leaderboards or automatic grades. Authorized reviewers
            consider off-platform work and context.
          </p>
        </section>
        <section className="panel">
          <h2>What each institutional role can see</h2>
          <dl>
            <dt>
              <strong>Campus administrator</strong>
            </dt>
            <dd>
              Affiliation requests and campus administration within their
              institution.
            </dd>
            <dt>
              <strong>Faculty evaluator</strong>
            </dt>
            <dd>
              Assigned cohorts and evidence they are authorized to review.
            </dd>
            <dt>
              <strong>Public visitor</strong>
            </dt>
            <dd>Project presentations shared in the public showcase.</dd>
          </dl>
        </section>
      </main>
    </>
  );
}
