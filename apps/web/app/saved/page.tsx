import { SiteFooter } from "@/components/layout/site-footer";
import { SiteHeader } from "@/components/layout/site-header";
import { SavedProjects } from "@/components/projects/saved-projects";

export default function SavedProjectsPage() {
  return <><a className="skip-link" href="#main-content">Skip to content</a><SiteHeader workspace /><main id="main-content" className="page-container" style={{ flex: 1 }}><SavedProjects /></main><SiteFooter /></>;
}
