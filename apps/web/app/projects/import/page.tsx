import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { GitHubConnectionsPage } from "@/components/github/github-connections-page";
import { PreviewBadge } from "@/components/layout/preview-banner";

export default function ImportProjectPage() {
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
        className="flex-1 w-[min(1120px,calc(100%-32px))] md:w-[min(1120px,calc(100%-64px))] mx-auto pt-8 pb-16 flex flex-col gap-7"
      >
        <div>
          <div className="flex flex-wrap items-center gap-2 mb-3 text-muted text-[13px] font-medium">
            <a className="text-link no-underline hover:underline" href="/projects">
              ← Back to My projects
            </a>
          </div>
          <h1 className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0">
            Start a project
          </h1>
          <p className="text-sm leading-[21px] text-muted mt-1.5 mb-0">
            Projects always start as private drafts. Publication and source access are configured
            separately after creation.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-[repeat(auto-fit,minmax(300px,1fr))] gap-6 items-start">
          {/* Live Route: Import from GitHub */}
          <section className="min-w-0 max-w-full bg-surface border-2 border-link rounded-2xl p-6 flex flex-col gap-4">
            <div>
              <span className="text-xs font-medium leading-4 text-success">Live launch route</span>
              <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] mt-1 mb-0">
                Import from GitHub
              </h2>
            </div>
            <p className="text-sm leading-[21px] text-muted m-0">
              Keep your Git remote and perform Git writes, issues, and PR reviews on GitHub. PeerGit
              displays authorized code and synchronized evidence.
            </p>
            <GitHubConnectionsPage />
          </section>

          {/* Qualified Pilot: Create on PeerGit */}
          <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-4">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div>
                <span className="text-xs font-medium leading-4 text-link">
                  Qualified pilot sample (Stage B1)
                </span>
                <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] mt-1 mb-0">
                  Create on PeerGit
                </h2>
              </div>
              <PreviewBadge phase="Stage B1" />
            </div>

            <p className="text-sm leading-[21px] text-muted m-0">
              Host private source directly in your campus Forgejo cell. Limits: 250 MiB per
              repository, 25 MiB single-file ceiling.
            </p>

            <fieldset disabled className="border border-hairline rounded-xl p-4 m-0 grid gap-3.5">
              <legend className="text-xs font-medium leading-4 text-muted px-1">
                Sample configuration
              </legend>
              <label className="grid gap-1.5 text-[13px] font-medium text-on-surface">
                Repository slug (Northgate State University)
                <input
                  className="w-full min-h-11 px-3.5 py-2.5 border border-control rounded-lg bg-surface text-on-surface font-mono text-[13px] leading-[19px]"
                  defaultValue="shuttle-alert-cli"
                  readOnly
                />
                <span className="font-mono text-[13px] leading-[19px] text-muted [overflow-wrap:anywhere]">
                  Remote: https://git.northgate.peergit.edu/anair/shuttle-alert-cli.git
                </span>
              </label>

              <div className="grid gap-2">
                <span className="text-[13px] font-medium leading-[18px]">Initialization</span>
                <label className="flex items-start gap-2.5 my-1 text-sm leading-5">
                  <input
                    type="radio"
                    name="init"
                    defaultChecked
                    className="w-4 h-4 mt-0.5 accent-primary shrink-0"
                  />
                  <span>Add a README starter</span>
                </label>
                <label className="flex items-start gap-2.5 my-1 text-sm leading-5">
                  <input
                    type="radio"
                    name="init"
                    className="w-4 h-4 mt-0.5 accent-primary shrink-0"
                  />
                  <span>Start empty (push an existing local repository)</span>
                </label>
              </div>

              <button
                type="button"
                className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium border border-control bg-surface text-on-surface disabled:opacity-55 disabled:cursor-not-allowed"
                disabled
              >
                Create private repository
              </button>
            </fieldset>

            <a className="text-[13px] font-medium leading-[18px]" href="/preview/native">
              Open full PeerGit-hosted repository preview →
            </a>
          </section>
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
