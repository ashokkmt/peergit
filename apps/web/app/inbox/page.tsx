import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { PreviewBadge } from "@/components/layout/preview-banner";

export default function InboxPage() {
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <SiteHeader workspace />
      <main
        id="main-content"
        className="pg-workspace-container"
        style={{
          flex: 1,
          padding: "32px 0 64px",
          display: "flex",
          flexDirection: "column",
          gap: "24px",
        }}
      >
        <div className="section-heading" style={{ marginBottom: 0 }}>
          <div>
            <p className="eyebrow">PeerGit messages</p>
            <h1 className="t-headline-md">Inbox</h1>
          </div>
          <PreviewBadge phase="Phase 5B–5C preview" />
        </div>

        <p className="t-body-md muted" style={{ margin: 0 }}>
          Notifications and contextual messaging are planned for Phases 5B–5C. This preview shows
          how project context and participant identity stay together.
        </p>

        <div className="split-layout">
          <section className="split-list-panel" aria-label="Message threads">
            <div
              style={{
                padding: "12px 16px",
                borderBottom: "1px solid var(--hairline)",
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                gap: "8px",
              }}
            >
              <div style={{ display: "flex", gap: "6px" }}>
                <span className="pg-pill pg-pill-raised">All</span>
                <span className="pg-pill">Unread (2)</span>
              </div>
              <span className="t-label-sm muted">Sample threads</span>
            </div>

            <div className="list-rail">
              <a className="list-item" aria-current="true" href="#thread">
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: "8px" }}>
                  <span className="t-label-sm" style={{ color: "var(--link)" }}>
                    Application · Campus Shuttle Tracker
                  </span>
                  <span className="brand-dot" aria-label="Unread" />
                </div>
                <strong className="t-title-sm" style={{ display: "block", marginTop: "4px" }}>
                  Dev Patel sent a message about QA &amp; Field Tester
                </strong>
                <small>3 hours ago · Sample</small>
              </a>

              <a className="list-item" href="#thread">
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: "8px" }}>
                  <span className="t-label-sm muted">Project update · CS 340</span>
                  <span className="brand-dot" aria-label="Unread" />
                </div>
                <strong className="t-title-sm" style={{ display: "block", marginTop: "4px" }}>
                  Milestone 2 snapshot window opens Nov 26
                </strong>
                <small>Yesterday · Sample</small>
              </a>
            </div>
          </section>

          <section
            className="pg-card"
            id="thread"
            style={{ padding: 0, overflow: "hidden", display: "flex", flexDirection: "column" }}
          >
            <div
              style={{
                padding: "18px 22px",
                borderBottom: "1px solid var(--hairline)",
                display: "flex",
                flexWrap: "wrap",
                alignItems: "center",
                justifyContent: "space-between",
                gap: "12px",
              }}
            >
              <div>
                <p className="t-label-sm muted" style={{ margin: 0 }}>
                  Context: Application for QA &amp; Field Tester · Campus Shuttle Tracker
                </p>
                <h2 className="t-title-md" style={{ marginTop: "4px" }}>
                  Participants: Asha Nair (Maintainer), Dev Patel (Applicant)
                </h2>
              </div>
              <span className="t-label-sm" style={{ color: "var(--danger)" }}>
                Report or block
              </span>
            </div>

            <div style={{ padding: "22px", display: "flex", flexDirection: "column", gap: "14px" }}>
              <div className="pg-card-canvas" style={{ maxWidth: "36rem" }}>
                <p className="t-label-sm muted" style={{ margin: 0 }}>
                  Dev Patel · Sample message
                </p>
                <p className="t-body-md" style={{ margin: "6px 0 0" }}>
                  Hi Asha, I ride the North Loop at 8:15 AM daily and have tested VoiceOver on iOS
                  for our HCI lab. Happy to verify PR #14 tomorrow morning.
                </p>
              </div>
              <div className="notice">
                Messages will be available when Phase 5B notification and Phase 5C messaging
                contracts ship.
              </div>
            </div>

            <form
              style={{
                padding: "16px 22px",
                borderTop: "1px solid var(--hairline)",
                display: "flex",
                flexWrap: "wrap",
                gap: "10px",
              }}
            >
              <label className="sr-only" htmlFor="reply-box">
                Reply
              </label>
              <input
                id="reply-box"
                className="pg-input"
                style={{ flex: "1 1 200px" }}
                disabled
                placeholder="Messaging is not available in this preview"
              />
              <button className="primary-button" disabled type="button">
                Send
              </button>
            </form>
          </section>
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
