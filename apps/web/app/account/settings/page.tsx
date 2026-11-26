import { AppearanceControl } from "@/components/layout/appearance-control";
import { AccountPanel } from "@/components/account/account-panel";
import { PreviewBadge } from "@/components/layout/preview-banner";
import { NotificationPreferences } from "@/components/notifications/notification-preferences";

export default function SettingsPage() {
  return (
    <>
      <AccountPanel section="settings" />

      <section
        id="appearance"
        className="pg-card"
        style={{ display: "flex", flexDirection: "column", gap: "12px" }}
      >
        <h2 className="t-title-lg">Appearance</h2>
        <p className="t-body-md muted" style={{ margin: 0 }}>
          Choose the theme for this device. System default follows your operating system
          preference.
        </p>
        <div>
          <AppearanceControl variant="select" />
        </div>
      </section>

      <NotificationPreferences />

      <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "space-between",
            gap: "8px",
          }}
        >
          <h2 className="t-title-lg">Scoped HTTPS Git credentials</h2>
          <PreviewBadge phase="Stage B1" />
        </div>
        <p className="t-body-md muted" style={{ margin: 0 }}>
          Personal Git credentials for PeerGit-hosted repositories will include one-time reveal,
          expiry, last use, and immediate revocation.
        </p>
      </section>

      <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "space-between",
            gap: "8px",
          }}
        >
          <h2 className="t-title-lg">Data export</h2>
          <PreviewBadge phase="Phase 8" />
        </div>
        <p className="t-body-md muted" style={{ margin: 0 }}>
          Self-service data export and deletion workflows will get implemented in a later phase.
        </p>
      </section>
    </>
  );
}
