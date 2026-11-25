import { AccountNavigation } from "@/components/account/account-navigation";
import { SiteHeader } from "@/components/layout/site-header";

export default function AccountLayout({ children }: { children: React.ReactNode }) {
  return <><a className="skip-link" href="#account-main">Skip to content</a><SiteHeader />
    <div className="account-layout"><AccountNavigation /><main id="account-main">{children}</main></div>
  </>;
}
