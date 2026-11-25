import { SiteHeader } from "@/components/layout/site-header";

export default async function PublicProfilePage({
  params,
}: {
  params: Promise<{ handle: string }>;
}) {
  const { handle } = await params;

  return (
    <>
      <SiteHeader />
      <main id="main-content" className="page-container">
        <p className="prototype-note">
          Will get implemented in a later phase · Phase 5D
        </p>
        <section className="profile-hero panel">
          <div>
            <p className="eyebrow">Fictional student portfolio</p>
            <h1>Example Student</h1>
            <p className="muted">@{handle} · Northgate campus</p>
            <p>Building accessible campus tools with project teams.</p>
            <ul className="skill-list" aria-label="Example skills">
              <li>Accessibility</li>
              <li>TypeScript</li>
              <li>Testing</li>
            </ul>
          </div>
          <span className="avatar" aria-hidden="true">
            ES
          </span>
        </section>
        <div className="repo-layout content">
          <section className="panel">
            <h2>Projects</h2>
            <h3>Campus Shuttle Tracker</h3>
            <p>Example role · Field tester</p>
            <p>
              Tested arrival announcements with screen readers and documented
              the results.
            </p>
          </section>
          <section className="panel">
            <h2>Contributions</h2>
            <p>
              Example GitHub and campus evidence will appear only for the
              audience allowed to view it.
            </p>
            <button type="button" className="secondary-button" disabled>
              Share portfolio
            </button>
          </section>
        </div>
        <a href="/account/profile">Edit your PeerGit profile</a>
      </main>
    </>
  );
}
