import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { PreviewBadge } from "@/components/layout/preview-banner";

const disabledSecondaryBtn =
  "inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-control bg-surface text-on-surface disabled:opacity-55 disabled:cursor-not-allowed";

export default function NativePreviewPage() {
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
        <div className="flex flex-wrap items-center gap-2 text-muted text-[13px] font-medium">
          <a className="text-link no-underline hover:underline" href="/preview">
            ← Design previews
          </a>
          <span>/</span>
          <span>Stage B1 qualified pilot</span>
        </div>

        <div className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <h1 className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0">
              PeerGit hosted repositories
            </h1>
            <p className="text-sm leading-[21px] text-muted mt-1 mb-0">
              Fictional workspace · Northgate State University · Campus Shuttle Tracker
            </p>
          </div>
          <PreviewBadge phase="Stage B1" />
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 items-start">
          <div className="flex flex-col gap-5">
            <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3.5">
              <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
                Create a private repository
              </h2>
              <p className="text-[13px] leading-[19px] text-muted m-0">
                Limits: 250 MiB per repository, 25 MiB single-file ceiling.
              </p>
              <fieldset
                disabled
                className="border border-hairline rounded-xl p-4 m-0 grid gap-3"
              >
                <legend className="text-xs font-medium leading-4 text-muted px-1">
                  Example project
                </legend>
                <label className="grid gap-1.5 text-[13px] font-medium text-on-surface">
                  Repository name
                  <input
                    className="w-full min-h-11 px-3.5 py-2.5 border border-control rounded-lg bg-surface text-on-surface font-mono text-[13px] leading-[19px]"
                    defaultValue="shuttle-tracker"
                    readOnly
                  />
                </label>
                <label className="grid gap-1.5 text-[13px] font-medium text-on-surface">
                  Initialize repository
                  <select
                    className="w-full min-h-11 px-3.5 py-2.5 border border-control rounded-lg bg-surface text-on-surface text-sm leading-5"
                    defaultValue="readme"
                  >
                    <option value="readme">With a README</option>
                    <option value="empty">Empty</option>
                  </select>
                </label>
                <button
                  type="button"
                  className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium border border-primary bg-primary text-on-primary disabled:opacity-55 disabled:cursor-not-allowed"
                  disabled
                >
                  Create repository
                </button>
              </fieldset>
            </section>

            <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3">
              <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                Scoped Git credential
              </h2>
              <p className="text-[13px] leading-[19px] text-muted m-0">
                Example scope · shuttle-tracker · Read and write · Expires in 30 days
              </p>
              <div>
                <button type="button" disabled className={disabledSecondaryBtn}>
                  Create credential
                </button>
              </div>
            </section>
          </div>

          <div className="flex flex-col gap-5">
            <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3">
              <div className="flex items-center justify-between gap-2">
                <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                  Issue #19 · Fix arrival banner
                </h2>
                <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-success text-success text-xs font-medium leading-4">
                  Open
                </span>
              </div>
              <p className="text-[13px] leading-[19px] text-muted m-0">
                Accessibility · Unassigned · Repository issue (distinct from Project tasks)
              </p>
              <div>
                <button type="button" disabled className={disabledSecondaryBtn}>
                  Comment
                </button>
              </div>
            </section>

            <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                  Pull request #14 · Add detour alerts
                </h2>
                <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-warning text-warning text-xs font-medium leading-4">
                  Review required
                </span>
              </div>
              <p className="font-mono text-[13px] leading-[19px] text-muted m-0">
                feature/route-alerts → main · Head: 9b2e10f
              </p>
              <div className="flex items-start gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px]">
                <span>
                  Protected branch rule on <code>main</code>: 1 reviewer approval required after
                  head update.
                </span>
              </div>
              <div className="flex flex-wrap gap-2.5 items-center">
                <button type="button" disabled className={disabledSecondaryBtn}>
                  Approve
                </button>
                <button type="button" disabled className={disabledSecondaryBtn}>
                  Merge blocked (1 approval needed)
                </button>
              </div>
            </section>
          </div>
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
