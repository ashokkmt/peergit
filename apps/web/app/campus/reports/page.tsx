export default function CampusReportsPage() {
  return (
    <main id="main-content" className="page-container">
      <div className="breadcrumbs">
        <a href="/campus/overview">Campus overview</a>
        <span>/</span>
        <span>Cohorts, verification &amp; reports</span>
      </div>
      <div className="section-heading">
        <div>
          <p className="eyebrow">Institution console</p>
          <h1>Cohorts, verification &amp; reports</h1>
        </div>
        <span className="prototype-note">
          Will get implemented in a later phase · Phases 6–8 preview
        </span>
      </div>
      <div className="notice">
        No report exports or named student analytics are connected yet. Any
        future reports must declare their scope and coverage and suppress groups
        smaller than campus policy allows.
      </div>
      <section className="content">
        <h2>Sample cohort roster</h2>
        <div className="table-scroll">
          <table className="data-table">
            <thead>
              <tr>
                <th>Student</th>
                <th>Project</th>
                <th>Linked contribution types</th>
                <th>Last observed</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Example student</td>
                <td>Sample project</td>
                <td>Commits, PR reviews, snapshots</td>
                <td>Sample timestamp</td>
              </tr>
              <tr>
                <td>Example student</td>
                <td>Not linked</td>
                <td>No linked evidence in this period</td>
                <td>Coverage gap</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </main>
  );
}
