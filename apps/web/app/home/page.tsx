import { SiteHeader } from "@/components/layout/site-header";
import { MyProjects } from "@/components/projects/my-projects";

export default function StudentHomePage() {
  return (
    <>
      <SiteHeader />
      <main id="main-content" className="page-container">
        <div className="section-heading">
          <div>
            <p className="eyebrow">Student workspace</p>
            <h1>Your campus work</h1>
          </div>
          <a className="secondary-button" href="/account/profile">
            View your profile
          </a>
        </div>
        <MyProjects />
        <section className="panel">
          <h2>Campus activity</h2>
          <p>Will get implemented in a later phase · Phase 5C</p>
        </section>
      </main>
    </>
  );
}
