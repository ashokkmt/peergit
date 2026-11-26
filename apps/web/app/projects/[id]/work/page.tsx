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
      <a
        className="fixed z-100 left-4 -top-20 px-4 py-2.5 bg-primary text-on-primary rounded-lg font-semibold no-underline focus:top-3"
        href="#main-content"
      >
        Skip to content
      </a>
      <SiteHeader workspace />
      <main
        id="main-content"
        className="flex-1 w-[min(1240px,calc(100%-32px))] md:w-[min(1240px,calc(100%-64px))] mx-auto pt-8 pb-16"
      >
        <ProjectWork projectID={id} />
      </main>
      <SiteFooter />
    </>
  );
}
