import { SiteHeader } from "@/components/layout/site-header";
import { ExploreResults } from "./explore-results";

export default function ExplorePage() {
  return (
    <>
      <SiteHeader />
      <main id="main-content" className="page-container">
        <ExploreResults />
      </main>
    </>
  );
}
