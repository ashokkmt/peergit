import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { PublicPortfolio } from "@/components/portfolio/portfolio-pages";

export default async function PublicProfilePage({
  params,
}: {
  params: Promise<{ handle: string }>;
}) {
  const { handle } = await params;

  return (
    <>
      <a
        className="fixed left-4 -top-20 z-100 rounded-lg bg-primary px-4 py-2.5 font-semibold text-on-primary focus:top-3 print:hidden"
        href="#main-content"
      >
        Skip to content
      </a>
      <div className="print:hidden">
        <SiteHeader workspace />
      </div>
      <main id="main-content" className="flex-1">
        <PublicPortfolio handle={handle} />
      </main>
      <div className="print:hidden">
        <SiteFooter />
      </div>
    </>
  );
}
