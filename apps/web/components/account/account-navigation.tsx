"use client";

import { usePathname } from "next/navigation";

const sections = [
  ["/account/profile", "Profile"],
  ["/account/portfolio", "Portfolio"],
  ["/account/succession", "Project succession"],
  ["/onboarding", "Campus verification"],
  ["/account/github", "GitHub connection"],
  ["/account/settings", "Security & appearance"],
  ["/projects", "My projects"],
] as const;

export function AccountNavigation() {
  const pathname = usePathname();
  const links = sections.map(([href, label]) => (
    <a
      href={href}
      key={href}
      aria-current={pathname === href ? "page" : undefined}
      className="block px-3.5 py-2.5 rounded-lg text-muted no-underline text-[13px] font-medium border-l-2 border-transparent hover:bg-surface hover:text-on-surface aria-[current=page]:bg-surface aria-[current=page]:border aria-[current=page]:border-hairline aria-[current=page]:border-l-2 aria-[current=page]:border-l-link aria-[current=page]:text-on-surface aria-[current=page]:font-semibold"
    >
      {label}
    </a>
  ));

  return (
    <nav aria-label="Account navigation">
      <details className="grid mb-2 md:hidden [&[open]]:bg-surface [&[open]]:border [&[open]]:border-hairline [&[open]]:rounded-lg [&[open]]:p-1.5">
        <summary className="list-none [&::-webkit-details-marker]:hidden px-3.5 py-2.5 bg-surface border border-hairline rounded-lg text-sm font-semibold cursor-pointer">
          Account menu
        </summary>
        {links}
      </details>
      <div className="hidden md:flex sticky top-24 flex-col gap-1">{links}</div>
    </nav>
  );
}
