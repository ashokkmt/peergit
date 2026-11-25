import { SiteHeader } from "@/components/layout/site-header";

export default function NativePreviewPage() {
  return (
    <>
      <SiteHeader />
      <main id="main-content" className="page-container">
        <a href="/preview">Design previews</a>
        <h1>PeerGit hosted repositories</h1>
        <p className="prototype-note">
          Will get implemented in a later phase · Stage B1
        </p>
        <p>Fictional workspace · Northgate campus · Shuttle Tracker</p>
        <div className="repo-layout">
          <div className="list-rail">
            <section className="panel">
              <h2>Create a private repository</h2>
              <fieldset disabled className="account-form">
                <legend>Example project</legend>
                <label>
                  Repository name
                  <input defaultValue="shuttle-tracker" />
                </label>
                <label>
                  Initialize repository
                  <select defaultValue="readme">
                    <option value="readme">With a README</option>
                    <option value="empty">Empty</option>
                  </select>
                </label>
                <button type="button" className="primary-button">
                  Create repository
                </button>
              </fieldset>
            </section>
            <section className="panel">
              <h2>Scoped Git credential</h2>
              <p>
                Example scope · shuttle-tracker · Read and write · Expires in 30
                days
              </p>
              <button type="button" disabled className="secondary-button">
                Create credential
              </button>
            </section>
          </div>
          <div className="list-rail">
            <section className="panel">
              <h2>Issue #19 · Fix arrival banner</h2>
              <p>Open · Accessibility · Unassigned</p>
              <button type="button" disabled className="secondary-button">
                Comment
              </button>
            </section>
            <section className="panel">
              <h2>Pull request #14 · Add detour alerts</h2>
              <p>
                <code>detour-alerts → main</code>
              </p>
              <p>
                Waiting for one approval. A new commit requires a new review.
              </p>
              <button type="button" disabled className="secondary-button">
                Approve
              </button>{" "}
              <button type="button" disabled className="secondary-button">
                Merge
              </button>
            </section>
          </div>
        </div>
      </main>
    </>
  );
}
