import { AccountNavigation } from "@/components/account/account-navigation";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";

export default function AccountLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <a
        className="fixed z-100 left-4 -top-20 px-4 py-2.5 bg-primary text-on-primary rounded-lg font-semibold no-underline focus:top-3"
        href="#account-main"
      >
        Skip to content
      </a>
      <SiteHeader workspace />
      <div className="flex-1 w-[min(1240px,calc(100%-32px))] md:w-[min(1240px,calc(100%-64px))] mx-auto pt-8 pb-16 grid grid-cols-1 md:grid-cols-[240px_minmax(0,1fr)] gap-6 md:gap-8">
        <AccountNavigation />
        <main id="account-main" className="min-w-0 flex flex-col gap-6">
          {children}
        </main>
      </div>
      <SiteFooter />
    </>
  );
}
