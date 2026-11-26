import { PreviewBadge } from "@/components/layout/preview-banner";

const cohortMetrics = [
  ["Verified participation", "72 / 76", "Enrolled students in sample cohort", ""],
  ["Recent observed activity", "16 / 18", "Linked projects in 14d window", ""],
  ["Snapshots verified", "14 / 18", "Milestone 2 due Nov 28", "text-success"],
  ["Verification backlog", "6", "Manual affiliation requests", "text-warning"],
] as const;

export default function CampusOverviewPage() {
  return (
    <>
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="mt-0 mb-1.5 text-link text-xs font-semibold tracking-[0.02em]">
            Institution workspace
          </p>
          <h1 className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0">
            Campus overview
          </h1>
          <p className="text-sm leading-[21px] text-muted mt-1 mb-0">
            Institution-scoped visibility, verification and authorized evidence.
          </p>
        </div>
        <PreviewBadge phase="Phases 6–8 preview" />
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap gap-2.5 items-center">
          <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-hairline text-xs font-medium leading-4">
            Cohort: CS 340 Software Engineering (Sample)
          </span>
          <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-hairline text-xs font-medium leading-4">
            Period: Last 14 days
          </span>
        </div>
        <div className="inline-flex items-center px-2.5 py-1 rounded-full bg-raised border border-hairline text-xs font-medium leading-4 max-w-full [overflow-wrap:anywhere]">
          Coverage: 18/20 projects linked · 2 unlinked (off-platform work possible)
        </div>
      </div>

      <div className="flex items-start gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px]">
        <span>
          Sample figures only. Institutional roles and aggregate reporting will be wired to
          authorized campus data in Phases 6–8. No individual productivity scores or automatic
          grades.
        </span>
      </div>

      <section
        aria-label="Cohort metrics"
        className="grid grid-cols-1 sm:grid-cols-2 min-[900px]:grid-cols-4 gap-5"
      >
        {cohortMetrics.map(([title, value, meta, toneClass]) => (
          <article
            className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6"
            key={title}
          >
            <span className="block text-muted text-xs font-medium">{title}</span>
            <strong
              className={`block font-display text-[32px] leading-[38px] font-semibold mt-1 tabular-nums ${toneClass}`}
            >
              {value}
            </strong>
            <span className="block text-muted text-xs font-medium mt-1">{meta}</span>
          </article>
        ))}
      </section>

      <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
            Recent authorized receipts
          </h2>
          <a className="text-[13px] font-medium leading-[18px]" href="/campus/reports">
            Open cohort roster →
          </a>
        </div>
        <div className="overflow-x-auto max-w-full">
          <table className="w-full border-collapse text-left min-w-[580px]">
            <thead>
              <tr className="border-b border-hairline">
                <th className="py-3 px-3.5 text-[13px] font-medium text-on-surface">Project</th>
                <th className="py-3 px-3.5 text-[13px] font-medium text-on-surface">
                  Receipt &amp; SHA
                </th>
                <th className="py-3 px-3.5 text-[13px] font-medium text-on-surface">Timing</th>
                <th className="py-3 px-3.5 text-[13px] font-medium text-on-surface">
                  Verification
                </th>
              </tr>
            </thead>
            <tbody>
              <tr className="border-b border-hairline">
                <td className="py-3 px-3.5 align-top text-sm font-medium">
                  Campus Shuttle Tracker
                </td>
                <td className="py-3 px-3.5 align-top font-mono text-[13px] leading-[19px] tabular-nums">
                  rec_84f19a · 7c1a9de
                </td>
                <td className="py-3 px-3.5 align-top text-sm text-success">Received on time</td>
                <td className="py-3 px-3.5 align-top text-sm text-success">Archive verified</td>
              </tr>
              <tr className="border-b border-hairline">
                <td className="py-3 px-3.5 align-top text-sm font-medium">
                  Lab Equipment Scheduler
                </td>
                <td className="py-3 px-3.5 align-top font-mono text-[13px] leading-[19px] tabular-nums">
                  rec_71c02b · 3e8a104
                </td>
                <td className="py-3 px-3.5 align-top text-sm text-success">Received on time</td>
                <td className="py-3 px-3.5 align-top text-sm text-success">Archive verified</td>
              </tr>
              <tr>
                <td className="py-3 px-3.5 align-top text-sm font-medium">
                  Transit Kiosk Firmware
                </td>
                <td className="py-3 px-3.5 align-top font-mono text-[13px] leading-[19px] text-muted">
                  —
                </td>
                <td className="py-3 px-3.5 align-top text-sm text-muted">
                  No linked evidence in this period
                </td>
                <td className="py-3 px-3.5 align-top text-sm text-muted">Awaiting submission</td>
              </tr>
            </tbody>
          </table>
        </div>
        <nav className="flex flex-wrap gap-2.5 items-center" aria-label="Campus sections">
          <a
            className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-control bg-surface text-on-surface hover:bg-raised"
            href="/admin/campus-reviews"
          >
            Affiliation verification
          </a>
          <a
            className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-control bg-surface text-on-surface hover:bg-raised"
            href="/campus/reports"
          >
            Cohorts &amp; reports (preview)
          </a>
        </nav>
      </section>
    </>
  );
}
