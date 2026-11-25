"use client";

import { useEffect, useState } from "react";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";

type Project = {
  id: string;
  title: string;
  summary: string;
  project_type: string;
  visibility: string;
  recruiting: boolean;
  skills: string[];
};
type Envelope<T> = { data?: T; error?: { message?: string } };

export function PublicProjectPage({ projectID }: { projectID: string }) {
  const [project, setProject] = useState<{
    project: Project & { lifecycle: string };
    description: string;
  } | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    fetch(`/api/v1/projects/${encodeURIComponent(projectID)}?scope=public`, {
      cache: "no-store",
    })
      .then(async (response) => {
        const result = (await response.json()) as Envelope<{
          project: Project & { lifecycle: string };
          description: string;
        }>;
        if (!response.ok || result.data?.project.visibility !== "public")
          throw new Error(result.error?.message ?? "This project is unavailable.");
        if (active) setProject(result.data);
      })
      .catch(
        (reason) =>
          active &&
          setError(
            reason instanceof Error ? reason.message : "This project is unavailable.",
          ),
      );
    return () => {
      active = false;
    };
  }, [projectID]);

  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <SiteHeader />
      <main id="main-content" className="page-container" style={{ flex: 1 }}>
        {error && (
          <div className="empty-state">
            <h1 className="t-headline-md">Project unavailable</h1>
            <p>{error}</p>
            <a className="primary-link" href="/explore">
              Return to Explore
            </a>
          </div>
        )}
        {!project && !error && <p role="status">Loading project…</p>}
        {project && (
          <div style={{ display: "flex", flexDirection: "column", gap: "28px" }}>
            <div
              style={{
                display: "flex",
                flexWrap: "wrap",
                alignItems: "flex-start",
                justifyContent: "space-between",
                gap: "24px",
                paddingBottom: "24px",
                borderBottom: "1px solid var(--hairline)",
              }}
            >
              <div style={{ display: "flex", flexDirection: "column", gap: "10px", maxWidth: "42rem" }}>
                <div className="breadcrumbs" style={{ marginBottom: 0 }}>
                  <a href="/explore">← Explore projects</a>
                  <span>/</span>
                  <span>{project.project.project_type.replaceAll("_", " ")}</span>
                </div>
                <h1 className="t-headline-lg">{project.project.title}</h1>
                <p className="t-body-lg muted" style={{ margin: 0 }}>
                  {project.project.summary}
                </p>
                <div style={{ display: "flex", flexWrap: "wrap", gap: "8px", paddingTop: "4px" }}>
                  <span className="pg-pill">Project visibility: Public showcase</span>
                  <span className="pg-pill">Source access: Private to project members</span>
                  <span className="pg-pill">{project.project.lifecycle.replaceAll("_", " ")}</span>
                </div>
              </div>

              <div style={{ display: "flex", flexDirection: "column", gap: "8px", alignItems: "flex-start" }}>
                <a
                  className="primary-link"
                  href={`/projects/${encodeURIComponent(projectID)}?tab=open-roles`}
                >
                  Sign in or verify to apply
                </a>
                <span className="t-body-sm muted">
                  {project.project.recruiting
                    ? "Recruiting open team roles"
                    : "Check team participation details"}
                </span>
              </div>
            </div>

            <div className="repo-layout">
              <div className="repo-main" style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
                <article className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "14px" }}>
                  <div
                    style={{
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      paddingBottom: "10px",
                      borderBottom: "1px solid var(--hairline)",
                    }}
                  >
                    <h2 className="t-title-md">Approved overview</h2>
                    <span className="t-label-sm muted">Opted-in presentation</span>
                  </div>
                  <p className="t-body-md" style={{ margin: 0, maxWidth: "65ch" }}>
                    {project.description || project.project.summary}
                  </p>
                  {project.project.skills.length > 0 && (
                    <ul className="skill-list">
                      {project.project.skills.map((skill) => (
                        <li key={skill}>{skill}</li>
                      ))}
                    </ul>
                  )}
                </article>

                <div className="pg-callout">
                  <span>
                    Code files, issue discussions, and contributor emails are restricted to
                    authorized project members. Public visitors see only the team’s opted-in
                    presentation.
                  </span>
                </div>
              </div>

              <aside className="repo-aside">
                <article className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "14px" }}>
                  <div>
                    <span className="t-label-sm" style={{ color: "var(--link)" }}>
                      {project.project.recruiting ? "Published opening" : "Join the team"}
                    </span>
                    <h2 className="t-title-md" style={{ marginTop: "4px" }}>
                      {project.project.recruiting ? "Open to collaborators" : "Team details"}
                    </h2>
                  </div>
                  <p className="t-body-md muted" style={{ margin: 0 }}>
                    {project.project.recruiting
                      ? "This project is actively recruiting. Campus verification may be required to apply."
                      : "View team membership and ways to participate after signing in."}
                  </p>
                  <a
                    className="primary-link"
                    style={{ width: "100%" }}
                    href={`/projects/${encodeURIComponent(projectID)}?tab=open-roles`}
                  >
                    View team openings
                  </a>
                  <p className="t-body-sm muted" style={{ margin: 0 }}>
                    <a href="/login">Sign in</a> · <a href="/signup">Join PeerGit</a>
                  </p>
                </article>
              </aside>
            </div>
          </div>
        )}
      </main>
      <SiteFooter />
    </>
  );
}
