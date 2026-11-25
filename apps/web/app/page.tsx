import { ServiceStatus } from "./service-status";
import styles from "./home.module.css";
import { ProjectsPanel } from "@/components/projects/projects-panel";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";

export default function Home() {
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <SiteHeader />

      <main id="main-content" style={{ flex: 1 }}>
        {/* Hero Section */}
        <section className={styles.heroBand} aria-labelledby="welcome-title">
          <div className={`pg-container ${styles.heroGrid}`}>
            <div className={styles.heroCopy}>
              <h1 id="welcome-title" className="t-display-lg">
                Build something worth sharing.
              </h1>
              <p className="t-body-lg muted" style={{ maxWidth: "52ch", margin: 0 }}>
                Find campus teammates, work on real projects, and keep a record of what you built.
              </p>
              <div className="button-row" style={{ paddingTop: "4px" }}>
                <a className="primary-link" href="/explore">
                  Explore projects
                </a>
                <a className="secondary-button" href="/signup">
                  Join your campus
                </a>
              </div>
              <ServiceStatus />
            </div>

            <aside className={styles.heroCard} aria-label="Sample project preview">
              <div className={styles.heroCardHeader}>
                <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                  <span className="brand-dot" aria-hidden="true" />
                  <div>
                    <p className="t-label-sm muted" style={{ margin: 0 }}>
                      Sample project · Northgate State University
                    </p>
                    <h2 className="t-title-md">Campus Shuttle Tracker</h2>
                  </div>
                </div>
                <span className="pg-pill pg-pill-success">Campus verified</span>
              </div>

              <div className={styles.heroDirectory}>
                <div className={styles.heroCommitRow}>
                  <span className="t-body-sm" style={{ fontWeight: 500 }}>
                    Add live route status
                  </span>
                  <code className="t-mono-sm muted tabular-nums">7c1a9de</code>
                </div>
                <div className={styles.heroFileRow}>
                  <code style={{ color: "var(--link)" }}>src/routes/</code>
                  <span className="t-body-sm muted">Route alert handlers</span>
                </div>
                <div className={styles.heroFileRow}>
                  <code style={{ color: "var(--link)" }}>docs/</code>
                  <span className="t-body-sm muted">Transit feed notes</span>
                </div>
                <div className={styles.heroFileRow}>
                  <code>README.md</code>
                  <span className="t-body-sm muted">Setup &amp; endpoints</span>
                </div>
              </div>

              <div className={styles.heroRoleRow}>
                <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                  <span className="brand-dot" aria-hidden="true" />
                  <div>
                    <p className="t-label-md" style={{ margin: 0 }}>
                      Open role: QA &amp; Field Tester
                    </p>
                    <p className="t-body-sm muted" style={{ margin: 0 }}>
                      1 opening · Asha Nair + 3 teammates
                    </p>
                  </div>
                </div>
                <a className="t-label-sm" href="/explore" style={{ textDecoration: "none" }}>
                  View opening
                </a>
              </div>
            </aside>
          </div>
        </section>

        {/* Why PeerGit */}
        <section className={styles.sectionPad} aria-labelledby="why-title">
          <div className="pg-container">
            <div style={{ maxWidth: "42rem", marginBottom: "40px" }}>
              <h2 id="why-title" className="t-headline-lg">
                Give campus projects a place to grow.
              </h2>
            </div>

            <div className="pg-grid-3">
              <div className={styles.editorialItem}>
                <h3 className="t-title-md">Find teammates with the skills you need.</h3>
                <p className="t-body-md muted" style={{ margin: 0 }}>
                  Post clear openings with required skills and capacity so verified students on your
                  campus can apply directly.
                </p>
                <p className="t-body-sm" style={{ color: "var(--link)", margin: 0 }}>
                  Example: 1 QA &amp; Field Tester needed for morning shuttle routes.
                </p>
              </div>

              <div className={styles.editorialItem}>
                <h3 className="t-title-md">Keep tasks, code and reviews with the project.</h3>
                <p className="t-body-md muted" style={{ margin: 0 }}>
                  Connect your GitHub repository as a private draft while tracking branches, commits,
                  issues, and pull requests in one workspace.
                </p>
                <p className="t-body-sm" style={{ color: "var(--link)", margin: 0 }}>
                  Example: PR #14 on feature/route-alerts linked to Milestone 2.
                </p>
              </div>

              <div className={styles.editorialItem}>
                <h3 className="t-title-md">Share milestones and submitted work when you choose.</h3>
                <p className="t-body-md muted" style={{ margin: 0 }}>
                  Capture immutable repository snapshots pinned to an exact 40-character SHA with
                  separate project publication controls.
                </p>
                <p className="t-body-sm" style={{ color: "var(--link)", margin: 0 }}>
                  Example: Receipt rec_84f19a verified at exact commit 7c1a9de.
                </p>
              </div>
            </div>
          </div>
        </section>

        {/* How it works */}
        <section id="how-it-works" className={styles.surfaceSection} aria-labelledby="how-title">
          <div className="pg-container">
            <div style={{ maxWidth: "44rem", marginBottom: "40px" }}>
              <h2 id="how-title" className="t-headline-lg">
                How Campus Shuttle Tracker moves from idea to record
              </h2>
              <p className="t-body-lg muted" style={{ margin: "10px 0 0" }}>
                GitHub import is the live creation route. Students keep their Git remote while
                PeerGit records authorized evidence.
              </p>
            </div>

            <div className="pg-grid-4" style={{ marginBottom: "28px" }}>
              <article className={styles.stepCard}>
                <span className="t-label-sm" style={{ color: "var(--link)" }}>
                  Step 1
                </span>
                <h3 className="t-title-sm">Verify your campus</h3>
                <p className="t-body-sm muted" style={{ margin: 0 }}>
                  Confirm your campus mailbox or request administrator review independently from
                  GitHub.
                </p>
              </article>
              <article className={styles.stepCard}>
                <span className="t-label-sm" style={{ color: "var(--link)" }}>
                  Step 2
                </span>
                <h3 className="t-title-sm">Start or join a project</h3>
                <p className="t-body-sm muted" style={{ margin: 0 }}>
                  Import a selected GitHub repository as a private draft or apply to an open campus
                  role.
                </p>
              </article>
              <article className={styles.stepCard}>
                <span className="t-label-sm" style={{ color: "var(--link)" }}>
                  Step 3
                </span>
                <h3 className="t-title-sm">Build together</h3>
                <p className="t-body-sm muted" style={{ margin: 0 }}>
                  Push commits, review PRs, and coordinate milestones without losing context.
                </p>
              </article>
              <article className={styles.stepCard}>
                <span className="t-label-sm" style={{ color: "var(--link)" }}>
                  Step 4
                </span>
                <h3 className="t-title-sm">Share the outcome</h3>
                <p className="t-body-sm muted" style={{ margin: 0 }}>
                  Capture a verifiable snapshot receipt and publish your project card when ready.
                </p>
              </article>
            </div>

            <div className="pg-grid-2">
              <div className={styles.artifactCard}>
                <div>
                  <p className="t-label-sm muted" style={{ margin: 0 }}>
                    Observed Git commit
                  </p>
                  <p className="t-title-sm" style={{ margin: "4px 0 0" }}>
                    Add live route status
                  </p>
                  <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                    Asha Nair · branch main · 2 hours ago
                  </p>
                </div>
                <code className="pg-pill t-mono-sm tabular-nums">7c1a9de</code>
              </div>

              <div className={styles.artifactCard}>
                <div>
                  <p className="t-label-sm muted" style={{ margin: 0 }}>
                    Immutable snapshot receipt
                  </p>
                  <p className="t-title-sm" style={{ margin: "4px 0 0" }}>
                    Demo build rec_84f19a
                  </p>
                  <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                    Received on time · Archive verified · 4.2 MiB
                  </p>
                </div>
                <span className="pg-pill pg-pill-success">Verified</span>
              </div>
            </div>
          </div>
        </section>

        {/* Explore public projects */}
        <section className={styles.sectionPad} aria-labelledby="projects-title">
          <div className="pg-container">
            <ProjectsPanel />
          </div>
        </section>

        {/* For Institutes */}
        <section id="for-institutes" className={styles.surfaceSection} aria-labelledby="institute-title">
          <div className={`pg-container ${styles.instituteGrid}`}>
            <div style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
              <h2 id="institute-title" className="t-headline-lg">
                See how campus projects are progressing.
              </h2>
              <p className="t-body-md muted" style={{ margin: 0 }}>
                Colleges get a dedicated console for verified affiliation, faculty supervision,
                consented showcases, and cohort reports that link directly back to authorized
                evidence.
              </p>
              <div>
                <a className="secondary-button" href="/institutes">
                  See how institutes use PeerGit
                </a>
              </div>
            </div>

            <div className={styles.heroCard}>
              <div className={styles.heroCardHeader}>
                <div>
                  <p className="t-label-sm muted" style={{ margin: 0 }}>
                    Example campus dashboard · Fall Cohort
                  </p>
                  <h3 className="t-title-md">CS 340 Software Engineering</h3>
                </div>
                <span className="pg-pill">Coverage: 18/20 linked</span>
              </div>

              <div className="pg-grid-3">
                <div className="metric-card" style={{ padding: "16px", borderRadius: "12px" }}>
                  <span>Active projects</span>
                  <strong>18</strong>
                </div>
                <div className="metric-card" style={{ padding: "16px", borderRadius: "12px" }}>
                  <span>Milestones on track</span>
                  <strong style={{ color: "var(--success)" }}>15</strong>
                </div>
                <div className="metric-card" style={{ padding: "16px", borderRadius: "12px" }}>
                  <span>Snapshots verified</span>
                  <strong>14</strong>
                </div>
              </div>
            </div>
          </div>
        </section>

        {/* Project outcomes & Final CTA */}
        <section className={styles.sectionPad} aria-labelledby="outcome-title">
          <div className="pg-container">
            <div className={styles.ctaBanner}>
              <div style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
                <span className="t-label-sm" style={{ color: "var(--link)" }}>
                  Illustrative showcase
                </span>
                <h2 id="outcome-title" className="t-headline-md">
                  Campus Shuttle Tracker cut morning bus wait uncertainty by 40%
                </h2>
                <p className="t-body-md muted" style={{ margin: 0 }}>
                  Goal: Publish live campus shuttle arrival boards across 4 stops. Outcome: Deployed
                  to 1,200 daily riders with verified snapshot{" "}
                  <code className="t-mono-sm">rec_84f19a</code> and full commit attribution.
                </p>
              </div>
              <div className="button-row" style={{ justifyContent: "flex-end" }}>
                <a className="primary-link" href="/explore">
                  Find a project to work on
                </a>
                <a className="secondary-button" href="/signup">
                  Join your campus
                </a>
              </div>
            </div>
          </div>
        </section>
      </main>

      <SiteFooter />
    </>
  );
}
