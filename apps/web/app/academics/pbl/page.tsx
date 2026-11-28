import { PBLWorkspace } from "@/components/academic/pbl-workspace";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";

export default function PBLPage() {
  return <>
    <a href="#main-content" className="fixed -top-20 left-4 z-50 rounded-lg bg-primary px-4 py-2.5 font-semibold text-on-primary focus:top-3">Skip to content</a>
    <SiteHeader workspace />
    <PBLWorkspace />
    <SiteFooter />
  </>;
}
