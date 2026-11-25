import { MyProjects } from "@/components/projects/my-projects";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";

export default function ProjectsPage() {
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <SiteHeader workspace />
      <main id="main-content" className="page-container" style={{ flex: 1 }}>
        <MyProjects />
      </main>
      <SiteFooter />
    </>
  );
}
