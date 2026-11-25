import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { ExploreResults } from "./explore-results";

export default function ExplorePage() {
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <SiteHeader />
      <main id="main-content" className="page-container" style={{ flex: 1 }}>
        <ExploreResults />
      </main>
      <SiteFooter />
    </>
  );
}
