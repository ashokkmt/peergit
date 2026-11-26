import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { Inbox } from "@/components/notifications/inbox";

export default function InboxPage() {
  return (
    <>
      <a
        className="fixed z-100 left-4 -top-20 px-4 py-2.5 bg-primary text-on-primary rounded-lg font-semibold no-underline focus:top-3"
        href="#main-content"
      >
        Skip to content
      </a>
      <SiteHeader workspace />
      <main
        id="main-content"
        className="flex-1 w-[min(1440px,calc(100%-32px))] md:w-[min(1440px,calc(100%-64px))] mx-auto pt-8 pb-16 flex flex-col gap-6"
      >
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <p className="mt-0 mb-1.5 text-link text-xs font-semibold tracking-[0.02em]">
              PeerGit notifications
            </p>
            <h1
              id="inbox-title"
              className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0"
            >
              Inbox
            </h1>
            <p className="text-sm leading-[21px] text-muted mt-1.5 mb-0 max-w-[60ch]">
              A durable record of project and application activity, with live updates when
              available.
            </p>
          </div>
          <a
            className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-control bg-surface text-on-surface hover:bg-raised"
            href="/account/settings#notifications"
          >
            Notification preferences
          </a>
        </div>

        <Inbox />
      </main>
      <SiteFooter />
    </>
  );
}
