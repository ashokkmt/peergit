import { BrandLogo } from "./brand-logo";

const footerLinkClass = "text-muted no-underline text-[13px] hover:underline hover:text-on-surface";

export function SiteFooter() {
  return (
    <footer className="bg-surface border-t border-hairline py-8 mt-auto">
      <div className="w-[min(1240px,calc(100%-32px))] md:w-[min(1240px,calc(100%-64px))] mx-auto grid grid-cols-1 md:grid-cols-[2fr_1fr_1fr] gap-8">
        <div className="flex flex-col gap-2.5">
          <BrandLogo />
          <p className="text-[13px] leading-[19px] text-muted max-w-[36ch] m-0">
            Campus project discovery, team collaboration, and verifiable engineering records.
          </p>
        </div>
        <nav className="flex flex-col gap-2" aria-label="Product and institutes">
          <span className="text-[13px] font-medium leading-[18px] text-on-surface">
            Platform
          </span>
          <a className={footerLinkClass} href="/explore">
            Explore projects
          </a>
          <a className={footerLinkClass} href="/institutes">
            Institute overview
          </a>
          <a className={footerLinkClass} href="/preview">
            Design previews
          </a>
        </nav>
        <nav className="flex flex-col gap-2" aria-label="Support and legal">
          <span className="text-[13px] font-medium leading-[18px] text-on-surface">
            Support &amp; Policy
          </span>
          <a className={footerLinkClass} href="/privacy">
            Privacy policy
          </a>
          <a className={footerLinkClass} href="/terms">
            Terms of service
          </a>
          <a className={footerLinkClass} href="/onboarding">
            Campus verification
          </a>
        </nav>
      </div>
    </footer>
  );
}
