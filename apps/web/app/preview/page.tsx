import { SiteHeader } from "@/components/layout/site-header";

const previews = [
  ["Inbox and messages", "/inbox", "Phases 5B–5C"],
  ["Tasks and milestones", "/projects/sample/work", "Phase 5A"],
  ["Student portfolio", "/u/example-student", "Phase 5D"],
  ["Campus overview", "/campus/overview", "Phase 8"],
  ["Cohorts and reports", "/campus/reports", "Phases 6–8"],
  ["PeerGit hosted repositories", "/preview/native", "Stage B1"],
];

export default function PreviewPage() {
  return (
    <>
      <SiteHeader />
      <main id="main-content" className="page-container">
        <h1>Design previews</h1>
        <p>
          Will get implemented in a later phase. These screens use fictional
          examples.
        </p>
        <ul className="list-rail">
          {previews.map(([name, href, phase]) => (
            <li key={href}>
              <a href={href}>{name}</a> · {phase}
            </li>
          ))}
        </ul>
      </main>
    </>
  );
}
