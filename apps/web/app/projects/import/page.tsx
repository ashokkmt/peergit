import { SiteHeader } from "@/components/layout/site-header";
import { GitHubConnectionsPage } from "@/components/github/github-connections-page";

export default function ImportProjectPage() {
  return <><SiteHeader /><main id="main-content" className="page-container">
    <a href="/projects">Back to my projects</a>
    <h1>Start a project</h1>
    <p className="muted">Projects start as private drafts. Project publication and source access are separate.</p>
    <section className="panel">
      <h2>Import from GitHub</h2>
      <p>Keep your Git remote and use GitHub for writes, issues, and pull request reviews.</p>
      <GitHubConnectionsPage />
    </section>
    <p><a href="/preview/native">Preview PeerGit hosted repositories</a> · Will get implemented in a later phase · Stage B1</p>
  </main></>;
}
