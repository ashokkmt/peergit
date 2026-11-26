import { SiteFooter } from "@/components/layout/site-footer";
import { SiteHeader } from "@/components/layout/site-header";
import { ProjectWork } from "@/components/projects/project-work";

export default async function ProjectWorkPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <>
      <a className="skip-link" href="#main-content">Skip to content</a>
      <SiteHeader workspace />
      <main id="main-content" className="page-container" style={{ flex: 1 }}>
        <ProjectWork projectID={id} />
      </main>
      <SiteFooter />
    </>
  );
}
