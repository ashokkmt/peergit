import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { PreviewBadge } from "@/components/layout/preview-banner";

const cardClass = "min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col";

export default async function PublicProfilePage({
  params,
}: {
  params: Promise<{ handle: string }>;
}) {
  const { handle } = await params;

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
        className="flex-1 w-[min(1240px,calc(100%-32px))] md:w-[min(1240px,calc(100%-64px))] mx-auto pt-8 pb-16 flex flex-col gap-6"
      >
        <div className="flex flex-wrap items-center justify-between gap-3">
          <PreviewBadge phase="Phase 5D" />
          <a className="text-[13px] font-medium leading-[18px]" href="/preview">
            ← Design previews
          </a>
        </div>

        {/* Top Profile Band */}
        <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-[20px] p-6 flex flex-wrap items-start justify-between gap-5">
          <div className="flex flex-wrap items-start gap-5">
            <div
              className="w-[72px] h-[72px] rounded-full border border-hairline bg-raised text-on-surface grid place-items-center font-display text-[28px] font-semibold shrink-0"
              aria-hidden="true"
            >
              ES
            </div>
            <div className="flex flex-col gap-1.5 max-w-[38rem]">
              <div className="flex flex-wrap items-center gap-2.5">
                <h1 className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0">
                  Example Student
                </h1>
                <span className="font-mono text-[13px] leading-[19px] text-muted">@{handle}</span>
                <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-success text-success text-xs font-medium leading-4">
                  Northgate State University
                </span>
              </div>
              <p className="text-sm leading-[21px] text-muted m-0">
                B.S. Computer Science ’27. Building accessible campus tools, transit telemetry, and
                reliable distributed systems.
              </p>
            </div>
          </div>

          <div className="flex flex-wrap gap-2.5 items-center">
            <button
              type="button"
              className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-control bg-surface text-on-surface disabled:opacity-55 disabled:cursor-not-allowed"
              disabled
            >
              Share portfolio
            </button>
            <a
              className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-primary bg-primary text-on-primary hover:opacity-90"
              href="/account/profile"
            >
              Edit your PeerGit profile
            </a>
          </div>
        </section>

        <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_320px] gap-6">
          <aside className="min-w-0 self-start flex flex-col gap-5 lg:order-2">
            <section className={`${cardClass} gap-3`}>
              <h2 className="text-base font-semibold leading-[22px] m-0">Skills</h2>
              <ul className="flex flex-wrap gap-1.5 list-none m-0 p-0" aria-label="Example skills">
                {["Accessibility", "TypeScript", "Go", "PostgreSQL", "Testing"].map((skill) => (
                  <li
                    key={skill}
                    className="inline-flex items-center px-2.5 py-1 rounded bg-raised text-on-surface text-xs font-medium leading-4"
                  >
                    {skill}
                  </li>
                ))}
              </ul>
            </section>

            <section className={`${cardClass} gap-2`}>
              <h2 className="text-base font-semibold leading-[22px] m-0">Links</h2>
              <p className="text-[13px] leading-[19px] text-muted m-0">
                Verified OAuth identity and opted-in portfolio links appear here for authorized
                audiences.
              </p>
            </section>
          </aside>

          <div className="min-w-0 flex flex-col gap-5 lg:order-1">
            <div role="tablist" aria-label="Profile sections" className="flex gap-2 border-b border-hairline">
              <span
                role="tab"
                aria-selected="true"
                className="px-3.5 py-2.5 border-b-2 border-link font-semibold text-sm"
              >
                Projects
              </span>
              <span role="tab" aria-selected="false" className="px-3.5 py-2.5 text-sm text-muted">
                Contributions
              </span>
              <span role="tab" aria-selected="false" className="px-3.5 py-2.5 text-sm text-muted">
                Portfolio
              </span>
            </div>

            <section className={`${cardClass} gap-2.5`}>
              <div className="flex items-center justify-between gap-2">
                <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                  Campus Shuttle Tracker
                </h2>
                <span className="text-xs font-medium leading-4 text-success">
                  Public showcase · Sample
                </span>
              </div>
              <p className="text-sm leading-[21px] text-muted m-0">
                Example role · Field tester — Tested arrival announcements with screen readers and
                documented edge-case delays.
              </p>
              <p className="text-[13px] leading-[19px] text-muted m-0">
                Snapshot <code className="font-mono text-[13px] leading-[19px]">rec_84f19a</code> ·
                18 observed commits
              </p>
            </section>

            <section className={`${cardClass} gap-4`}>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                  Authorized contribution record
                </h2>
                <span className="text-xs font-medium leading-4 text-muted">
                  Source: GitHub + PeerGit · Sample range
                </span>
              </div>
              <div className="grid grid-cols-1 min-[900px]:grid-cols-3 gap-6">
                {[
                  ["Observed commits", "24"],
                  ["PR reviews", "9"],
                  ["Campus snapshots", "3"],
                ].map(([label, val]) => (
                  <div
                    key={label}
                    className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl p-5"
                  >
                    <span className="text-xs font-medium leading-4 text-muted">{label}</span>
                    <strong className="block font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] tabular-nums mt-1">
                      {val}
                    </strong>
                  </div>
                ))}
              </div>
            </section>
          </div>
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
