"use client";

import { useEffect, useState } from "react";
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

import { SiteHeader } from "@/components/layout/site-header";
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
          throw new Error(
            result.error?.message ?? "This project is unavailable.",
          );
        if (active) setProject(result.data);
      })
      .catch(
        (reason) =>
          active &&
          setError(
            reason instanceof Error
              ? reason.message
              : "This project is unavailable.",
          ),
      );
    return () => {
      active = false;
    };
  }, [projectID]);
  return (
    <>
      <SiteHeader />
      <main id="main-content" className="page-container">
        <div className="breadcrumbs">
          <a href="/explore">Explore projects</a>
          <span>/</span>
          <span>Project presentation</span>
        </div>
        {error && (
          <div className="empty-state">
            <h1>Project unavailable</h1>
            <p>{error}</p>
            <a href="/explore">Return to Explore</a>
          </div>
        )}
        {!project && !error && <p role="status">Loading project…</p>}
        {project && (
          <>
            <section className="public-project-hero">
              <div className="repo-title-row">
                <div>
                  <p className="eyebrow">
                    Campus project · Public presentation
                  </p>
                  <h1>{project.project.title}</h1>
                  <p className="project-summary">{project.project.summary}</p>
                </div>
                <span className="chip">
                  {project.project.lifecycle.replaceAll("_", " ")}
                </span>
              </div>
              <div className="inline-meta">
                <span>Presentation shared by the project owner</span>
                <span>Source visibility is controlled separately</span>
              </div>
            </section>
            <div className="repo-layout" style={{ marginTop: "1.25rem" }}>
              <section className="repo-main panel">
                <p className="eyebrow">Approved overview</p>
                <h2>About this project</h2>
                <p>{project.project.summary}</p>
                <div className="notice">
                  Code, snapshots and academic evidence are not exposed by the
                  public project presentation. Sign in and receive the
                  appropriate project access to view them.
                </div>
              </section>
              <aside className="repo-aside panel">
                <p className="eyebrow">Join the team</p>
                <h2>
                  {project.project.recruiting
                    ? "Open to collaborators"
                    : "Team details"}
                </h2>
                <p className="muted">
                  {project.project.recruiting
                    ? "This project is recruiting. Campus verification may be required to apply."
                    : "Check with the project team about ways to participate."}
                </p>
                <a className="primary-link" href={`/projects/${encodeURIComponent(projectID)}?tab=open-roles`}>
                  View team openings
                </a>
                <p>
                  <a href="/login">Sign in</a> · <a href="/signup">Join PeerGit</a>
                </p>
              </aside>
            </div>
          </>
        )}
      </main>
    </>
  );
}
