import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { PreviewBadge } from "@/components/layout/preview-banner";

const previews = [
  ["Campus reporting preview", "/campus/overview", "Phases 7–8"],
  ["Cohorts and reports", "/campus/reports", "Phases 7–8"],
  ["PeerGit hosted repositories", "/preview/native", "Stage B1"],
];

export default function PreviewPage() {
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
        className="flex-1 w-[min(1240px,calc(100%-32px))] md:w-[min(1240px,calc(100%-64px))] mx-auto pt-8 pb-16 flex flex-col gap-6"
      >
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <p className="mt-0 mb-1.5 text-link text-xs font-semibold tracking-[0.02em]">
              Roadmap previews
            </p>
            <h1 className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0">
              Design previews
            </h1>
          </div>
          <PreviewBadge />
        </div>
        <p className="text-sm leading-[21px] text-muted m-0">
          These screens use fictional examples to show planned institutional reporting and PeerGit
          hosted repositories.
        </p>

        <div className="grid grid-cols-1 min-[900px]:grid-cols-3 gap-6">
          {previews.map(([name, href, phase]) => (
            <a
              key={href}
              className="bg-surface border border-hairline rounded-2xl p-6 no-underline text-on-surface flex flex-col justify-between gap-4 hover:border-link transition-colors duration-100"
              href={href}
            >
              <div>
                <span className="text-xs font-medium leading-4 text-link">{phase}</span>
                <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] mt-1 mb-0">
                  {name}
                </h2>
              </div>
              <span className="text-[13px] font-medium leading-[18px] text-link">
                Open preview →
              </span>
            </a>
          ))}
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
