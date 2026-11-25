"use client";

import { usePathname } from "next/navigation";

const sections = [
  ["/account/profile", "Profile"],
  ["/onboarding", "Campus verification"],
  ["/account/github", "GitHub connection"],
  ["/account/settings", "Security & appearance"],
  ["/projects", "My projects"],
] as const;

export function AccountNavigation() {
  const pathname = usePathname();
  const links = sections.map(([href, label]) => (
    <a href={href} key={href} aria-current={pathname === href ? "page" : undefined}>
      {label}
    </a>
  ));

  return (
    <nav className="account-navigation" aria-label="Account navigation">
      <details className="account-mobile-nav">
        <summary>Account menu</summary>
        {links}
      </details>
      <div className="account-sidebar">{links}</div>
    </nav>
  );
}
