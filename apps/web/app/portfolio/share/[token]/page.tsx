import { PublicPortfolio } from "@/components/portfolio/portfolio-pages";
import { SiteFooter } from "@/components/layout/site-footer";
import { SiteHeader } from "@/components/layout/site-header";

export default async function SharedPortfolioPage({
  params,
}: {
  params: Promise<{ token: string }>;
}) {
  const { token } = await params;

  return (
    <>
      <div className="print:hidden">
        <SiteHeader />
      </div>
      <PublicPortfolio token={token} />
      <div className="print:hidden">
        <SiteFooter />
      </div>
    </>
  );
}
