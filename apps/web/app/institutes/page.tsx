import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";

const workflowCards = [
  [
    "1 · Verify affiliation",
    "Mailbox or admin review",
    "Confirm campus membership separately from personal GitHub accounts.",
  ],
  [
    "2 · Organize programs",
    "Courses & cohorts",
    "Group capstones, labs, and innovation programs by term.",
  ],
  [
    "3 · Review evidence",
    "Exact-SHA receipts",
    "Inspect authorized snapshots with coverage and freshness labels.",
  ],
  [
    "4 · Share outcomes",
    "Consented showcases",
    "Publish approved outcomes with small-group privacy suppression.",
  ],
] as const;

const roleMatrix = [
  [
    "Campus Administrator",
    "Affiliation queue, cohort aggregates, consented showcase editor",
    "Cannot read private source code or direct messages without grant",
  ],
  [
    "Faculty / Evaluator",
    "Assigned courses, submitted SHA receipts, qualitative feedback",
    "Scoped strictly to assigned cohorts and submission windows",
  ],
  [
    "Program Staff",
    "Program milestones, conflict-of-interest review queue, exports",
    "Small groups suppressed in both screen and CSV exports",
  ],
] as const;

export default function InstitutesPage() {
  return (
    <>
      <a
        className="fixed z-100 left-4 -top-20 px-4 py-2.5 bg-primary text-on-primary rounded-lg font-semibold no-underline focus:top-3"
        href="#main-content"
      >
        Skip to content
      </a>
      <SiteHeader />
      <main
        id="main-content"
        className="flex-1 w-[min(1240px,calc(100%-32px))] md:w-[min(1240px,calc(100%-64px))] mx-auto pt-8 pb-16 flex flex-col gap-12"
      >
        <section className="max-w-3xl flex flex-col gap-3.5">
          <span className="text-[13px] font-medium leading-[18px] text-link">
            Institutional overview
          </span>
          <h1 className="font-display text-[clamp(1.75rem,3.2vw,2.75rem)] font-bold leading-[1.14] tracking-[-0.02em] m-0">
            Verify affiliation, supervise academic work, and review authorized evidence.
          </h1>
          <p className="text-base leading-6 text-muted m-0">
            Colleges operate a separate workspace scoped strictly to their tenant. PeerGit surfaces
            verifiable snapshots and coverage gaps without productivity scores or automatic grades.
          </p>
          <div className="pt-1.5">
            <a
              className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium no-underline border border-primary bg-primary text-on-primary hover:opacity-90"
              href="/campus/overview"
            >
              See the example campus workspace
            </a>
          </div>
        </section>

        <section
          aria-label="Institutional workflow"
          className="grid grid-cols-1 sm:grid-cols-2 min-[900px]:grid-cols-4 gap-5"
        >
          {workflowCards.map(([step, title, body]) => (
            <article
              key={step}
              className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-2"
            >
              <span className="text-xs font-medium leading-4 text-link">{step}</span>
              <h2 className="text-base font-semibold leading-[22px] m-0">{title}</h2>
              <p className="text-[13px] leading-[19px] text-muted m-0">{body}</p>
            </article>
          ))}
        </section>

        <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-4">
          <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
            What each institutional role can see
          </h2>
          <div className="overflow-x-auto max-w-full">
            <table className="w-full border-collapse text-left min-w-[580px]">
              <thead>
                <tr className="border-b border-hairline">
                  <th className="py-3 px-3.5 text-[13px] font-medium text-on-surface">Role</th>
                  <th className="py-3 px-3.5 text-[13px] font-medium text-on-surface">
                    Permitted scope
                  </th>
                  <th className="py-3 px-3.5 text-[13px] font-medium text-on-surface">
                    Explicit boundary
                  </th>
                </tr>
              </thead>
              <tbody>
                {roleMatrix.map(([role, scope, boundary]) => (
                  <tr key={role} className="border-b border-hairline last:border-b-0">
                    <td className="py-3 px-3.5 align-top text-sm font-medium">{role}</td>
                    <td className="py-3 px-3.5 align-top text-sm text-muted">{scope}</td>
                    <td className="py-3 px-3.5 align-top text-sm text-muted">{boundary}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="text-[13px] leading-[19px] text-muted m-0">
            Signals support a conversation; they do not grade a student. Authorized reviewers
            consider off-platform work and context alongside recorded commits, reviews, and
            snapshots.
          </p>
        </section>
      </main>
      <SiteFooter />
    </>
  );
}
