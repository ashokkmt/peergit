import { AppearanceControl } from "@/components/layout/appearance-control";
import { AccountPanel } from "@/components/account/account-panel";
import { PreviewBadge } from "@/components/layout/preview-banner";

export default function SettingsPage() {
  return (
    <>
      <AccountPanel section="settings" />

      <section id="appearance" className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
        <h2 className="t-title-lg">Appearance</h2>
        <p className="t-body-md muted" style={{ margin: 0 }}>
          Choose the theme for this device. System default follows your operating system preference.
        </p>
        <div>
          <AppearanceControl variant="select" />
        </div>
      </section>

      <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: "8px" }}>
          <h2 className="t-title-lg">Scoped HTTPS Git credentials</h2>
          <PreviewBadge phase="Stage B1" />
        </div>
        <p className="t-body-md muted" style={{ margin: 0 }}>
          Personal Git credentials for PeerGit-hosted repositories will include one-time reveal,
          expiry, last use, and immediate revocation.
        </p>
      </section>

      <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: "8px" }}>
          <h2 className="t-title-lg">Notifications &amp; data export</h2>
          <PreviewBadge phase="Phases 5B–8" />
        </div>
        <p className="t-body-md muted" style={{ margin: 0 }}>
          Email notification preferences and self-service data export/deletion workflows are
          scheduled for later phases. Required security and verification messages always deliver.
        </p>
      </section>
    </>
  );
}
