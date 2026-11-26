import { PreviewBadge } from "@/components/layout/preview-banner";

export default function CampusReportsPage() {
  return (
    <>
      <div className="flex flex-wrap items-center gap-2 text-muted text-[13px] font-medium">
        <a className="text-link no-underline hover:underline" href="/campus/overview">
          Campus overview
        </a>
        <span>/</span>
        <span>Cohorts, verification &amp; reports</span>
      </div>

      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="mt-0 mb-1.5 text-link text-xs font-semibold tracking-[0.02em]">
            Institution console
          </p>
          <h1 className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0">
            Cohorts, verification &amp; reports
          </h1>
        </div>
        <PreviewBadge phase="Phases 6–8 preview" />
      </div>

      <div className="flex items-start gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px]">
        <span>
          No report exports or named student analytics are connected yet. Any future reports must
          declare their scope and coverage and suppress groups smaller than campus policy allows.
        </span>
      </div>

      <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3.5">
        <div>
          <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
            CS 340 Cohort Roster (Sample)
          </h2>
          <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">
            No productivity scores or automatic grades. Assessors inspect linked records alongside
            off-platform work.
          </p>
        </div>
        <div className="overflow-x-auto max-w-full">
          <table className="w-full border-collapse text-left min-w-[580px]">
            <thead>
              <tr className="border-b border-hairline">
                <th className="py-3 px-3.5 text-[13px] font-medium text-on-surface">Student</th>
                <th className="py-3 px-3.5 text-[13px] font-medium text-on-surface">Project</th>
                <th className="py-3 px-3.5 text-[13px] font-medium text-on-surface">
                  Linked contribution types
                </th>
                <th className="py-3 px-3.5 text-[13px] font-medium text-on-surface">
                  Last observed
                </th>
              </tr>
            </thead>
            <tbody>
              <tr className="border-b border-hairline">
                <td className="py-3 px-3.5 align-top text-sm font-medium">Example student</td>
                <td className="py-3 px-3.5 align-top text-sm">Campus Shuttle Tracker</td>
                <td className="py-3 px-3.5 align-top text-sm">Commits, PR reviews, snapshots</td>
                <td className="py-3 px-3.5 align-top text-sm text-muted">2 hours ago (sample)</td>
              </tr>
              <tr>
                <td className="py-3 px-3.5 align-top text-sm font-medium">Example student</td>
                <td className="py-3 px-3.5 align-top text-sm">Not linked</td>
                <td className="py-3 px-3.5 align-top text-sm text-muted">
                  No linked evidence in this period
                </td>
                <td className="py-3 px-3.5 align-top text-sm text-muted">Coverage gap</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
        <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3">
          <div className="flex items-center justify-between gap-2">
            <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
              Affiliation verification queue
            </h2>
            <span className="text-xs font-medium leading-4 text-warning">Fresh MFA required</span>
          </div>
          <p className="text-[13px] leading-[19px] text-muted m-0">
            Authorized campus administrators review manual affiliation requests with a required
            reason and audited decision.
          </p>
          <div>
            <a
              className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-control bg-surface text-on-surface hover:bg-raised"
              href="/admin/campus-reviews"
            >
              Open live verification queue →
            </a>
          </div>
        </section>

        <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3">
          <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
            Aggregate report export
          </h2>
          <p className="text-[13px] leading-[19px] text-muted m-0">
            Small groups (&lt; 5 students) are suppressed in both screen previews and CSV exports
            under campus privacy policy.
          </p>
          <div className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl px-3.5 py-3 flex items-center justify-between gap-3">
            <span className="text-xs font-medium leading-4">Fall 2026 Cohort Summary · CSV</span>
            <button
              type="button"
              className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-control bg-surface text-on-surface disabled:opacity-55 disabled:cursor-not-allowed"
              disabled
            >
              Generate export
            </button>
          </div>
        </section>
      </div>
    </>
  );
}
