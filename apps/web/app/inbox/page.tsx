import { SiteHeader } from "@/components/layout/site-header";

export default function InboxPage() {
  return (
    <>
      <SiteHeader />
      <main id="main-content" className="page-container">
        <div className="section-heading">
          <div>
            <p className="eyebrow">PeerGit messages</p>
            <h1>Inbox</h1>
          </div>
          <span className="prototype-note">
            Will get implemented in a later phase · Phase 5B preview
          </span>
        </div>
        <p className="muted">
          Messages are planned for Phase 5B. This preview shows how project
          context and participant identity will stay together.
        </p>
        <div className="split-layout">
          <nav className="list-rail" aria-label="Message threads">
            {[
              [
                "Application · QA & Field Tester",
                "Campus Shuttle Tracker · Sample",
              ],
              ["Project update", "CS 340 Milestone 2 · Sample"],
            ].map(([title, meta]) => (
              <a key={title} className="list-item" href="#thread">
                <strong>{title}</strong>
                <small>{meta}</small>
              </a>
            ))}
          </nav>
          <section className="panel" id="thread">
            <p className="eyebrow">Selected conversation · sample</p>
            <h2>QA & Field Tester application</h2>
            <p className="muted">
              Participants: project maintainer and applicant
            </p>
            <div className="notice">
              Messages will be available when Phase 5B notification and
              messaging contracts ship.
            </div>
            <form className="account-form">
              <label>
                Reply
                <textarea
                  rows={4}
                  disabled
                  placeholder="Messaging is not available in this preview"
                />
              </label>
              <button className="primary-button" disabled type="button">
                Send
              </button>
            </form>
          </section>
        </div>
      </main>
    </>
  );
}
