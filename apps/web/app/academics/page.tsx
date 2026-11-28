import { AcademicPortal } from "@/components/academic/academic-portal";

export default function AcademicsPage() {
  return (
    <>
      <a className="text-sm font-medium text-link hover:underline" href="/academics/pbl">
        PBL teams and repositories
      </a>
      <AcademicPortal section="academics" />
    </>
  );
}
