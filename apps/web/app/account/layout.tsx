import { AccountNavigation } from "@/components/account/account-navigation";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";

export default function AccountLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <a className="skip-link" href="#account-main">
        Skip to content
      </a>
      <SiteHeader workspace />
      <div className="account-layout" style={{ flex: 1 }}>
        <AccountNavigation />
        <main id="account-main">{children}</main>
      </div>
      <SiteFooter />
    </>
  );
}
