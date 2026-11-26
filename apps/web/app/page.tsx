import { ServiceStatus } from "./service-status";
import { ProjectsPanel } from "@/components/projects/projects-panel";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";

const pillars = [
  {
    title: "Find teammates with the skills you need.",
    body: "Post clear openings with required skills and capacity so verified students on your campus can apply directly.",
    example: "Example: 1 QA & Field Tester needed for morning shuttle routes.",
  },
  {
    title: "Keep tasks, code and reviews with the project.",
    body: "Connect your GitHub repository as a private draft while tracking branches, commits, issues, and pull requests in one workspace.",
    example: "Example: PR #14 on feature/route-alerts linked to Milestone 2.",
  },
  {
    title: "Share milestones and submitted work when you choose.",
    body: "Capture immutable repository snapshots pinned to an exact 40-character SHA with separate project publication controls.",
    example: "Example: Receipt rec_84f19a verified at exact commit 7c1a9de.",
  },
] as const;

const workflowSteps = [
  [
    "Step 1",
    "Verify your campus",
    "Confirm your campus mailbox or request administrator review independently from GitHub.",
  ],
  [
    "Step 2",
    "Start or join a project",
    "Import a selected GitHub repository as a private draft or apply to an open campus role.",
  ],
  [
    "Step 3",
    "Build together",
    "Push commits, review PRs, and coordinate milestones without losing context.",
  ],
  [
    "Step 4",
    "Share the outcome",
    "Capture a verifiable snapshot receipt and publish your project card when ready.",
  ],
] as const;

const containerClass = "w-[min(1240px,calc(100%-32px))] md:w-[min(1240px,calc(100%-64px))] mx-auto";
const primaryBtnClass =
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-[18px] py-2 text-sm font-medium leading-5 no-underline whitespace-nowrap border border-primary bg-primary text-on-primary hover:opacity-90 transition-opacity duration-100";
const secondaryBtnClass =
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-[18px] py-2 text-sm font-medium leading-5 no-underline whitespace-nowrap border border-control bg-surface text-on-surface hover:bg-raised transition-colors duration-100";

export default function Home() {
  return (
    <>
      <a
        className="fixed z-100 left-4 -top-20 px-4 py-2.5 bg-primary text-on-primary rounded-lg font-semibold no-underline focus:top-3"
        href="#main-content"
      >
        Skip to content
      </a>
      <SiteHeader />

      <main id="main-content" className="flex-1">
        {/* Hero Section */}
        <section
          className="bg-surface border-b border-hairline py-12 pb-16 md:py-[72px]"
          aria-labelledby="welcome-title"
        >
          <div className={`${containerClass} grid grid-cols-1 lg:grid-cols-2 gap-10 lg:gap-12 items-center`}>
            <div className="flex flex-col gap-5">
              <h1
                id="welcome-title"
                className="font-display text-[clamp(2.25rem,5vw,4.5rem)] font-bold leading-[1.06] tracking-[-0.025em] m-0 text-balance"
              >
                Build something worth sharing.
              </h1>
              <p className="text-base leading-6 text-muted max-w-[52ch] m-0">
                Find campus teammates, work on real projects, and keep a record of what you built.
              </p>
              <div className="flex flex-wrap items-center gap-2.5 pt-1">
                <a className={primaryBtnClass} href="/explore">
                  Explore projects
                </a>
                <a className={secondaryBtnClass} href="/signup">
                  Join your campus
                </a>
              </div>
              <ServiceStatus />
            </div>

            <aside
              className="bg-canvas border border-hairline rounded-[20px] p-5 sm:p-6 flex flex-col gap-4"
              aria-label="Sample project preview"
            >
              <div className="flex flex-wrap items-center justify-between gap-3 pb-3.5 border-b border-hairline">
                <div className="flex items-center gap-2.5">
                  <span className="inline-block w-2 h-2 rounded-full bg-brand shrink-0" aria-hidden="true" />
                  <div>
                    <p className="text-xs font-medium leading-4 text-muted m-0">
                      Sample project · Northgate State University
                    </p>
                    <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                      Campus Shuttle Tracker
                    </h2>
                  </div>
                </div>
                <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-surface border border-success text-success text-xs font-medium leading-4 max-w-full [overflow-wrap:anywhere]">
                  Campus verified
                </span>
              </div>

              <div className="bg-surface border border-hairline rounded-xl overflow-hidden">
                <div className="bg-raised border-b border-hairline px-4 py-2.5 flex items-center justify-between gap-3">
                  <span className="text-[13px] leading-[19px] font-medium">
                    Add live route status
                  </span>
                  <code className="font-mono text-[13px] leading-[19px] text-muted tabular-nums">
                    7c1a9de
                  </code>
                </div>
                <div className="px-4 py-2.5 border-b border-hairline flex items-center justify-between gap-3 text-[13px]">
                  <code className="text-link">src/routes/</code>
                  <span className="text-[13px] leading-[19px] text-muted">Route alert handlers</span>
                </div>
                <div className="px-4 py-2.5 border-b border-hairline flex items-center justify-between gap-3 text-[13px]">
                  <code className="text-link">docs/</code>
                  <span className="text-[13px] leading-[19px] text-muted">Transit feed notes</span>
                </div>
                <div className="px-4 py-2.5 flex items-center justify-between gap-3 text-[13px]">
                  <code>README.md</code>
                  <span className="text-[13px] leading-[19px] text-muted">Setup &amp; endpoints</span>
                </div>
              </div>

              <div className="bg-surface border border-hairline rounded-xl px-4 py-3.5 flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-2.5">
                  <span className="inline-block w-2 h-2 rounded-full bg-brand shrink-0" aria-hidden="true" />
                  <div>
                    <p className="text-[13px] font-medium leading-[18px] m-0">
                      Open role: QA &amp; Field Tester
                    </p>
                    <p className="text-[13px] leading-[19px] text-muted m-0">
                      1 opening · Asha Nair + 3 teammates
                    </p>
                  </div>
                </div>
                <a className="text-xs font-medium leading-4 text-link no-underline hover:underline" href="/explore">
                  View opening
                </a>
              </div>
            </aside>
          </div>
        </section>

        {/* Why PeerGit */}
        <section className="py-14 md:py-[72px]" aria-labelledby="why-title">
          <div className={containerClass}>
            <div className="max-w-2xl mb-10">
              <h2
                id="why-title"
                className="font-display text-[clamp(1.75rem,3.2vw,2.75rem)] font-bold leading-[1.14] tracking-[-0.02em] m-0"
              >
                Give campus projects a place to grow.
              </h2>
            </div>

            <div className="grid grid-cols-1 min-[900px]:grid-cols-3 gap-6">
              {pillars.map((pillar) => (
                <div key={pillar.title} className="border-t border-hairline pt-4 flex flex-col gap-2">
                  <h3 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                    {pillar.title}
                  </h3>
                  <p className="text-sm leading-[21px] text-muted m-0">{pillar.body}</p>
                  <p className="text-[13px] leading-[19px] text-link m-0">{pillar.example}</p>
                </div>
              ))}
            </div>
          </div>
        </section>

        {/* How it works */}
        <section
          id="how-it-works"
          className="bg-surface border-y border-hairline py-14 md:py-[72px]"
          aria-labelledby="how-title"
        >
          <div className={containerClass}>
            <div className="max-w-[44rem] mb-10">
              <h2
                id="how-title"
                className="font-display text-[clamp(1.75rem,3.2vw,2.75rem)] font-bold leading-[1.14] tracking-[-0.02em] m-0"
              >
                How Campus Shuttle Tracker moves from idea to record
              </h2>
              <p className="text-base leading-6 text-muted mt-2.5 mb-0">
                GitHub import is the live creation route. Students keep their Git remote while
                PeerGit records authorized evidence.
              </p>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 min-[900px]:grid-cols-4 gap-5 mb-7">
              {workflowSteps.map(([step, title, desc]) => (
                <article
                  key={step}
                  className="bg-canvas border border-hairline rounded-xl p-5 flex flex-col gap-2"
                >
                  <span className="text-xs font-medium leading-4 text-link">{step}</span>
                  <h3 className="text-base font-semibold leading-[22px] m-0">{title}</h3>
                  <p className="text-[13px] leading-[19px] text-muted m-0">{desc}</p>
                </article>
              ))}
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
              <div className="bg-canvas border border-hairline rounded-xl p-5 flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p className="text-xs font-medium leading-4 text-muted m-0">
                    Observed Git commit
                  </p>
                  <p className="text-base font-semibold leading-[22px] mt-1 mb-0">
                    Add live route status
                  </p>
                  <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">
                    Asha Nair · branch main · 2 hours ago
                  </p>
                </div>
                <code className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-hairline text-on-surface font-mono text-[13px] leading-[19px] tabular-nums">
                  7c1a9de
                </code>
              </div>

              <div className="bg-canvas border border-hairline rounded-xl p-5 flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p className="text-xs font-medium leading-4 text-muted m-0">
                    Immutable snapshot receipt
                  </p>
                  <p className="text-base font-semibold leading-[22px] mt-1 mb-0">
                    Demo build rec_84f19a
                  </p>
                  <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">
                    Received on time · Archive verified · 4.2 MiB
                  </p>
                </div>
                <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-success text-success text-xs font-medium leading-4">
                  Verified
                </span>
              </div>
            </div>
          </div>
        </section>

        {/* Explore public projects */}
        <section className="py-14 md:py-[72px]" aria-labelledby="projects-title">
          <div className={containerClass}>
            <ProjectsPanel />
          </div>
        </section>

        {/* For Institutes */}
        <section
          id="for-institutes"
          className="bg-surface border-y border-hairline py-14 md:py-[72px]"
          aria-labelledby="institute-title"
        >
          <div className={`${containerClass} grid grid-cols-1 lg:grid-cols-[5fr_7fr] gap-8 lg:gap-12 items-center`}>
            <div className="flex flex-col gap-4">
              <h2
                id="institute-title"
                className="font-display text-[clamp(1.75rem,3.2vw,2.75rem)] font-bold leading-[1.14] tracking-[-0.02em] m-0"
              >
                See how campus projects are progressing.
              </h2>
              <p className="text-sm leading-[21px] text-muted m-0">
                Colleges get a dedicated console for verified affiliation, faculty supervision,
                consented showcases, and cohort reports that link directly back to authorized
                evidence.
              </p>
              <div>
                <a className={secondaryBtnClass} href="/institutes">
                  See how institutes use PeerGit
                </a>
              </div>
            </div>

            <div className="bg-canvas border border-hairline rounded-[20px] p-5 sm:p-6 flex flex-col gap-4">
              <div className="flex flex-wrap items-center justify-between gap-3 pb-3.5 border-b border-hairline">
                <div>
                  <p className="text-xs font-medium leading-4 text-muted m-0">
                    Example campus dashboard · Fall Cohort
                  </p>
                  <h3 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                    CS 340 Software Engineering
                  </h3>
                </div>
                <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-hairline text-on-surface text-xs font-medium leading-4 max-w-full [overflow-wrap:anywhere]">
                  Coverage: 18/20 linked
                </span>
              </div>

              <div className="grid grid-cols-1 min-[900px]:grid-cols-3 gap-6">
                <div className="min-w-0 max-w-full bg-surface border border-hairline rounded-xl p-4">
                  <span className="block text-muted text-xs font-medium">Active projects</span>
                  <strong className="block font-display text-[32px] leading-[38px] font-semibold mt-1 tabular-nums">
                    18
                  </strong>
                </div>
                <div className="min-w-0 max-w-full bg-surface border border-hairline rounded-xl p-4">
                  <span className="block text-muted text-xs font-medium">Milestones on track</span>
                  <strong className="block font-display text-[32px] leading-[38px] font-semibold mt-1 tabular-nums text-success">
                    15
                  </strong>
                </div>
                <div className="min-w-0 max-w-full bg-surface border border-hairline rounded-xl p-4">
                  <span className="block text-muted text-xs font-medium">Snapshots verified</span>
                  <strong className="block font-display text-[32px] leading-[38px] font-semibold mt-1 tabular-nums">
                    14
                  </strong>
                </div>
              </div>
            </div>
          </div>
        </section>

        {/* Project outcomes & Final CTA */}
        <section className="py-14 md:py-[72px]" aria-labelledby="outcome-title">
          <div className={containerClass}>
            <div className="bg-surface border border-hairline rounded-[20px] p-6 sm:p-10 lg:p-12 grid grid-cols-1 lg:grid-cols-[7fr_5fr] gap-6 items-center">
              <div className="flex flex-col gap-3">
                <span className="text-xs font-medium leading-4 text-link">
                  Illustrative showcase
                </span>
                <h2
                  id="outcome-title"
                  className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0"
                >
                  Campus Shuttle Tracker cut morning bus wait uncertainty by 40%
                </h2>
                <p className="text-sm leading-[21px] text-muted m-0">
                  Goal: Publish live campus shuttle arrival boards across 4 stops. Outcome: Deployed
                  to 1,200 daily riders with verified snapshot{" "}
                  <code className="font-mono text-[13px] leading-[19px]">rec_84f19a</code> and full
                  commit attribution.
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-2.5 lg:justify-end">
                <a className={primaryBtnClass} href="/explore">
                  Find a project to work on
                </a>
                <a className={secondaryBtnClass} href="/signup">
                  Join your campus
                </a>
              </div>
            </div>
          </div>
        </section>
      </main>

      <SiteFooter />
    </>
  );
}
