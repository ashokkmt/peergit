"use client";

import { usePathname } from "next/navigation";
import { BrandLogo } from "@/components/layout/brand-logo";
import { AppearanceControl } from "@/components/layout/appearance-control";

const consoleLinks = [
  ["/campus/overview", "Overview"],
  ["/admin/campus-reviews", "Verification"],
  ["/admin/academic-reviews", "Faculty & mentor access"],
  ["/faculty", "Academic workspace"],
  ["/academics/pbl", "PBL teams and audit"],
  ["/campus/reports", "Cohorts & reports"],
  ["/preview", "Design previews"],
] as const;

export function CampusConsoleShell({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children: React.ReactNode;
}) {
  const pathname = usePathname();

  const navLinks = consoleLinks.map(([href, label]) => (
    <a
      key={href}
      href={href}
      aria-current={pathname === href ? "page" : undefined}
      className="px-3 py-2 rounded-lg text-muted no-underline text-[13px] font-medium border-l-2 border-transparent hover:bg-raised hover:text-on-surface aria-[current=page]:bg-raised aria-[current=page]:text-on-surface aria-[current=page]:border-link aria-[current=page]:font-semibold"
    >
      {label}
    </a>
  ));

  return (
    <div className="min-h-screen flex flex-col min-[900px]:flex-row">
      <a
        className="fixed z-100 left-4 -top-20 px-4 py-2.5 bg-primary text-on-primary rounded-lg font-semibold no-underline focus:top-3"
        href="#main-content"
      >
        Skip to content
      </a>

      <aside
        className="hidden min-[900px]:flex w-[248px] shrink-0 bg-surface border-r border-hairline px-4 py-5 flex-col justify-between gap-6"
        aria-label="Institutional console"
      >
        <div className="flex flex-col gap-5">
          <div className="px-1.5">
            <BrandLogo />
            <p className="text-xs font-medium leading-4 text-muted mt-1.5 mb-0">
              Campus Console
            </p>
          </div>
          <nav className="flex flex-col gap-1" aria-label="Campus console navigation">
            {navLinks}
          </nav>
        </div>

        <div className="pt-3 px-1.5 border-t border-hairline">
          <p className="text-xs font-medium leading-4 m-0 text-on-surface">
            Northgate State University
          </p>
          <p className="text-[13px] leading-[19px] text-muted mt-0.5 mb-0">
            Institutional workspace
          </p>
        </div>
      </aside>

      <div className="flex-1 min-w-0 flex flex-col">
        <header className="bg-surface border-b border-hairline px-4 py-3 md:px-8 md:min-h-16 flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-wrap items-center gap-2.5">
            <details className="relative md:hidden">
              <summary className="list-none inline-flex items-center justify-center min-h-10 px-3 border border-hairline rounded-lg bg-surface text-on-surface text-[13px] font-medium cursor-pointer [&::-webkit-details-marker]:hidden">
                Console
              </summary>
              <nav
                className="absolute z-40 left-0 top-[calc(100%+8px)] w-[min(260px,calc(100vw-32px))] grid gap-0.5 p-2 bg-surface border border-hairline rounded-xl shadow-[var(--shadow)]"
                aria-label="Mobile console navigation"
              >
                {navLinks}
              </nav>
            </details>
            <span className="text-base font-semibold leading-[22px]">{title}</span>
            {subtitle && (
              <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-raised border border-hairline text-on-surface text-xs font-medium leading-4 max-w-full [overflow-wrap:anywhere]">
                {subtitle}
              </span>
            )}
          </div>
          <div className="flex items-center gap-2">
            <a className="text-[13px] font-medium leading-[18px] text-link no-underline" href="/">
              PeerGit home
            </a>
            <AppearanceControl />
          </div>
        </header>

        <main id="main-content" className="w-[min(1240px,100%)] px-4 pt-6 pb-16 md:p-8 flex flex-col gap-6">
          {children}
        </main>
      </div>
    </div>
  );
}
