import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { Inbox } from "@/components/notifications/inbox";

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
            <p className="eyebrow">PeerGit notifications</p>
            <h1 id="inbox-title" className="t-headline-md">
              Inbox
            </h1>
            <p className="t-body-md muted" style={{ margin: "6px 0 0", maxWidth: "60ch" }}>
              A durable record of project and application activity, with live updates when
              available.
            </p>
          </div>
          <a className="secondary-button pg-btn-compact" href="/account/settings#notifications">
            Notification preferences
          </a>
        </div>

        <Inbox />
      </main>
      <SiteFooter />
    </>
  );
}
