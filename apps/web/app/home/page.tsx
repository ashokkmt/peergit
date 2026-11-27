import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { MyProjects } from "@/components/projects/my-projects";

const actionItems = [
  {
    title: "Inbox",
    description: "Review project updates, application decisions, and unread notifications.",
    href: "/inbox",
    cta: "Open inbox →",
  },
  {
    title: "Project work",
    description: "Choose a project and open Work to see tasks, milestones, and discussions.",
    href: "/projects",
    cta: "Open my projects →",
  },
  {
    title: "Messages",
    description: "Continue conversations from your projects and accepted applications.",
    href: "/messages",
    cta: "Open messages →",
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
              Your campus projects, conversations, and workspace activity.
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

        <section aria-labelledby="collaboration-title" className="flex flex-col gap-3.5">
          <h2 id="collaboration-title" className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
            Collaboration
          </h2>
          <div className="grid grid-cols-1 min-[900px]:grid-cols-3 gap-6">
            {actionItems.map((item) => (
              <article
                key={item.title}
                className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col justify-between gap-3"
              >
                <div>
                  <h3 className="text-base font-semibold leading-[22px] mt-1 mb-0">{item.title}</h3>
                  <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">
                    {item.description}
                  </p>
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
            <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
              Campus activity
            </h2>
            <p className="text-sm leading-[21px] m-0 text-muted">
              See activity from projects you can access and switch to projects you follow.
            </p>
            <a className="text-[13px] font-medium leading-[18px]" href="/feed">
              Open campus feed →
            </a>
          </section>
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
