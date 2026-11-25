import { SiteHeader } from "@/components/layout/site-header";

export default function CampusLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <SiteHeader />
      <nav
        className="button-row wide-container"
        aria-label="Campus design previews"
      >
        <a href="/preview">Design previews</a>
        <a href="/campus/overview">Campus overview</a>
        <a href="/campus/reports">Cohorts and reports</a>
      </nav>
      {children}
    </>
  );
}
