import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { MyProjects } from "@/components/projects/my-projects";
import { PreviewBadge } from "@/components/layout/preview-banner";

const actionItems = [
  {
    color: "text-warning",
    label: "Review requested · Sample",
    title: "PR #14 · Add live route status",
    meta: "Campus Shuttle Tracker · feature/route-alerts → main",
    href: "/projects/sample/work",
    cta: "Preview project work →",
  },
  {
    color: "text-link",
    label: "Team application · Sample",
    title: "Dev Patel applied for QA & Field Tester",
    meta: "Campus Shuttle Tracker · 1 seat remaining",
    href: "/inbox",
    cta: "Open inbox →",
  },
  {
    color: "text-success",
    label: "Upcoming submission · Sample",
    title: "CS 340 Milestone 2 Snapshot",
    meta: "Due Nov 28 · Last verified at 7c1a9de",
    href: "/projects",
    cta: "Open my projects →",
  },
] as const;

export default function StudentHomePage() {
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
        className="flex-1 w-[min(1440px,calc(100%-32px))] md:w-[min(1440px,calc(100%-64px))] mx-auto pt-8 pb-16 flex flex-col gap-8"
      >
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <h1 className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0">
              Student workspace
            </h1>
            <p className="text-sm leading-[21px] text-muted mt-1 mb-0">
              Your campus projects, action items, and workspace activity.
            </p>
          </div>
          <div className="flex flex-wrap gap-2.5 items-center">
            <a
              className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-primary bg-primary text-on-primary hover:opacity-90"
              href="/projects/import"
            >
              + Create project
            </a>
            <a
              className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-control bg-surface text-on-surface hover:bg-raised"
              href="/account/profile"
            >
              View your profile
            </a>
          </div>
        </div>

        {/* Actionable Items Preview */}
        <section aria-label="Actionable items" className="flex flex-col gap-3.5">
          <div className="flex flex-wrap items-center justify-between gap-2.5">
            <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
              Needs your attention
            </h2>
            <PreviewBadge phase="Phase 5A–5B sample" />
          </div>
          <div className="grid grid-cols-1 min-[900px]:grid-cols-3 gap-6">
            {actionItems.map((item) => (
              <article
                key={item.title}
                className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col justify-between gap-3"
              >
                <div>
                  <span className={`text-xs font-medium leading-4 ${item.color}`}>{item.label}</span>
                  <h3 className="text-base font-semibold leading-[22px] mt-1 mb-0">{item.title}</h3>
                  <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">{item.meta}</p>
                </div>
                <a className="text-[13px] font-medium leading-[18px]" href={item.href}>
                  {item.cta}
                </a>
              </article>
            ))}
          </div>
        </section>

        <div className="grid grid-cols-1 lg:grid-cols-[repeat(auto-fit,minmax(300px,1fr))] gap-6 items-start">
          <MyProjects />

          <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                Campus activity
              </h2>
              <PreviewBadge phase="Phase 5C" />
            </div>
            <div className="flex flex-col gap-3.5">
              <div className="pb-3 border-b border-hairline">
                <p className="text-sm leading-[21px] m-0">
                  <strong>Campus Shuttle Tracker</strong> captured snapshot{" "}
                  <code className="font-mono text-[13px] leading-[19px]">rec_84f19a</code>
                </p>
                <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">
                  Sample · Archive verified
                </p>
              </div>
              <div className="pb-3 border-b border-hairline">
                <p className="text-sm leading-[21px] m-0">
                  <strong>Bike Share Telemetry</strong> opened 2 roles for Spring mapping
                </p>
                <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">
                  Sample · Northgate Campus
                </p>
              </div>
              <div>
                <p className="text-sm leading-[21px] m-0">
                  <strong>CS 340 Software Engineering</strong> posted Milestone 2 deadline
                </p>
                <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">
                  Sample · Prof. Elena Vance
                </p>
              </div>
            </div>
          </section>
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
