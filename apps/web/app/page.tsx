import { ServiceStatus } from "./service-status";
import styles from "./home.module.css";
import { ProjectsPanel } from "@/components/projects/projects-panel";
import { SiteHeader } from "@/components/layout/site-header";

export default function Home() {
  return (
    <main>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <SiteHeader />

      <section
        className="hero"
        id="main-content"
        aria-labelledby="welcome-title"
      >
        <div className={styles.heroGrid}>
          <div>
            <p className="eyebrow">Campus projects, connected</p>
            <h1 id="welcome-title">
              Build something
              <br />
              worth sharing.
            </h1>
            <p className="intro">
              Find campus teammates, work on real projects, and keep a clear
              record of what you built—from the first commit to a milestone you
              choose to share.
            </p>
            <div className="hero-actions">
              <a className="primary-link" href="/explore">
                Explore projects <span aria-hidden="true">→</span>
              </a>
              <a className="secondary-button" href="/signup">
                Join your campus
              </a>
            </div>
            <ServiceStatus />
          </div>
          <aside
            className={`panel ${styles.projectPreview}`}
            aria-label="Example repository"
          >
            <p className="eyebrow">Example project · Northgate campus</p>
            <h2>Campus Shuttle Tracker</h2>
            <p className="muted">GitHub · main</p>
            <p>
              <strong>Add live route status</strong> · <code>7c1a9de</code>
            </p>
            <ul>
              <li>
                <code>src/routes/</code>
              </li>
              <li>
                <code>docs/</code>
              </li>
              <li>
                <code>README.md</code>
              </li>
            </ul>
            <p className="chip">Open role · Field tester</p>
          </aside>
        </div>
      </section>

      <section className="content" aria-label="How PeerGit works">
        <div className="section-heading">
          <div>
            <p className="eyebrow">A home for campus building</p>
            <h2>One project, from idea to evidence.</h2>
          </div>
          <a href="/institutes">For institutes →</a>
        </div>
        <div className="feature-grid">
          <article className="feature-card">
            <p className="eyebrow">Find your people</p>
            <h3>Meet students ready to build</h3>
            <p>
              Discover opted-in public projects or find campus-only
              opportunities after verifying your affiliation.
            </p>
          </article>
          <article className="feature-card">
            <p className="eyebrow">Build together</p>
            <h3>Keep the project and code connected</h3>
            <p>
              Bring a GitHub repository into a shared project workspace with
              team roles, code browsing, commits and snapshots.
            </p>
          </article>
          <article className="feature-card">
            <p className="eyebrow">Share on your terms</p>
            <h3>Choose what others can see</h3>
            <p>
              Project presentation, repository access and academic evidence each
              have their own audience and permission boundaries.
            </p>
          </article>
        </div>
      </section>
      <ProjectsPanel />

      <section className="how" id="how-it-works" aria-labelledby="how-title">
        <p className="eyebrow">From idea to record</p>
        <h2 id="how-title">
          A project grows when the right work meets the right people.
        </h2>
        <div className="steps">
          <article>
            <span>01</span>
            <h3>Verify your campus</h3>
            <p>
              Confirm your affiliation separately from the GitHub account you
              use to sign in.
            </p>
          </article>
          <article>
            <span>02</span>
            <h3>Start or join a project</h3>
            <p>
              Import a GitHub repository and create a private project draft with
              your team.
            </p>
          </article>
          <article>
            <span>03</span>
            <h3>Build in the open with your team</h3>
            <p>
              Browse code and history, recruit collaborators, and keep useful
              project context together.
            </p>
          </article>
          <article>
            <span>04</span>
            <h3>Share outcomes by choice</h3>
            <p>
              Publish an approved project presentation or capture a verifiable
              milestone snapshot.
            </p>
          </article>
        </div>
      </section>
      <section className="how" aria-labelledby="institute-title">
        <p className="eyebrow">For colleges</p>
        <h2 id="institute-title">Support student work with better context.</h2>
        <p className="intro">
          A separate institute workspace can verify affiliation, organize
          academic programs and review evidence students authorize. Missing data
          stays visible as a gap—never a productivity score.
        </p>
        <a className="primary-link" href="/institutes">
          See PeerGit for institutes →
        </a>
      </section>
      <footer>
        <span>PeerGit · Campus projects, connected</span>
        <span>
          <a href="/privacy">Privacy</a> · <a href="/terms">Terms</a> ·{" "}
          <a href="/institutes">For institutes</a> ·{" "}
          <a href="/preview">Design previews</a>
        </span>
      </footer>
    </main>
  );
}
