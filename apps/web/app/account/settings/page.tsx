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
        className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3"
      >
        <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">Appearance</h2>
        <p className="text-sm leading-[21px] text-muted m-0">
          Choose the theme for this device. System default follows your operating system
          preference.
        </p>
        <div>
          <AppearanceControl variant="select" />
        </div>
      </section>

      <NotificationPreferences />

      <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
            Scoped HTTPS Git credentials
          </h2>
          <PreviewBadge phase="Stage B1" />
        </div>
        <p className="text-sm leading-[21px] text-muted m-0">
          Personal Git credentials for PeerGit-hosted repositories will include one-time reveal,
          expiry, last use, and immediate revocation.
        </p>
      </section>

      <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
            Data export
          </h2>
          <PreviewBadge phase="Phase 8" />
        </div>
        <p className="text-sm leading-[21px] text-muted m-0">
          Self-service data export and deletion workflows will get implemented in a later phase.
        </p>
      </section>
    </>
  );
}
