"use client";

import { useEffect, useState } from "react";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { readApiEnvelope } from "@/lib/http";

type Project = {
  id: string;
  title: string;
  summary: string;
  project_type: string;
  visibility: string;
  recruiting: boolean;
  skills: string[];
};

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
        const result = await readApiEnvelope<{
          project: Project & { lifecycle: string };
          description: string;
        }>(response, "This project is unavailable.");
        if (!response.ok || result.data?.project.visibility !== "public") {
          throw new Error(result.error?.message ?? "This project is unavailable.");
        }
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
      <a
        className="fixed z-100 left-4 -top-20 px-4 py-2.5 bg-primary text-on-primary rounded-lg font-semibold no-underline focus:top-3"
        href="#main-content"
      >
        Skip to content
      </a>
      <SiteHeader />
      <main
        id="main-content"
        className="flex-1 w-[min(1240px,calc(100%-32px))] md:w-[min(1240px,calc(100%-64px))] mx-auto pt-8 pb-16"
      >
        {error && (
          <div className="grid justify-items-center content-center gap-2.5 min-h-[220px] p-8 border border-dashed border-control rounded-2xl text-center bg-surface">
            <h1 className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0">
              Project unavailable
            </h1>
            <p className="text-sm text-muted max-w-[500px] m-0">{error}</p>
            <a
              className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium no-underline border border-primary bg-primary text-on-primary hover:opacity-90"
              href="/explore"
            >
              Return to Explore
            </a>
          </div>
        )}
        {!project && !error && <p role="status">Loading project…</p>}
        {project && (
          <div className="flex flex-col gap-7">
            <div className="flex flex-wrap items-start justify-between gap-6 pb-6 border-b border-hairline">
              <div className="flex flex-col gap-2.5 max-w-[42rem]">
                <div className="flex flex-wrap items-center gap-2 text-muted text-[13px] font-medium">
                  <a className="text-link no-underline hover:underline" href="/explore">
                    ← Explore projects
                  </a>
                  <span>/</span>
                  <span>{project.project.project_type.replaceAll("_", " ")}</span>
                </div>
                <h1 className="font-display text-[clamp(1.75rem,3.2vw,2.75rem)] font-bold leading-[1.14] tracking-[-0.02em] m-0">
                  {project.project.title}
                </h1>
                <p className="text-base leading-6 text-muted m-0">
                  {project.project.summary}
                </p>
                <div className="flex flex-wrap gap-2 pt-1">
                  <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-hairline text-xs font-medium leading-4">
                    Project visibility: Public showcase
                  </span>
                  <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-hairline text-xs font-medium leading-4">
                    Source access: Private to project members
                  </span>
                  <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-hairline text-xs font-medium leading-4">
                    {project.project.lifecycle.replaceAll("_", " ")}
                  </span>
                </div>
              </div>

              <div className="flex flex-col gap-2 items-start">
                <a
                  className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium no-underline border border-primary bg-primary text-on-primary hover:opacity-90"
                  href={`/projects/${encodeURIComponent(projectID)}?tab=open-roles`}
                >
                  Sign in or verify to apply
                </a>
                <span className="text-[13px] leading-[19px] text-muted">
                  {project.project.recruiting
                    ? "Recruiting open team roles"
                    : "Check team participation details"}
                </span>
              </div>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_320px] gap-6">
              <div className="min-w-0 flex flex-col gap-5">
                <article className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3.5">
                  <div className="flex items-center justify-between pb-2.5 border-b border-hairline">
                    <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                      Approved overview
                    </h2>
                    <span className="text-xs font-medium leading-4 text-muted">
                      Opted-in presentation
                    </span>
                  </div>
                  <p className="text-sm leading-[21px] m-0 max-w-[65ch]">
                    {project.description || project.project.summary}
                  </p>
                  {project.project.skills.length > 0 && (
                    <ul className="flex flex-wrap gap-1.5 list-none m-0 p-0">
                      {project.project.skills.map((skill) => (
                        <li
                          key={skill}
                          className="inline-flex items-center px-2.5 py-1 rounded bg-raised text-on-surface text-xs font-medium leading-4"
                        >
                          {skill}
                        </li>
                      ))}
                    </ul>
                  )}
                </article>

                <div className="flex items-start gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px]">
                  <span>
                    Code files, issue discussions, and contributor emails are restricted to
                    authorized project members. Public visitors see only the team’s opted-in
                    presentation.
                  </span>
                </div>
              </div>

              <aside className="min-w-0 self-start">
                <article className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3.5">
                  <div>
                    <span className="text-xs font-medium leading-4 text-link">
                      {project.project.recruiting ? "Published opening" : "Join the team"}
                    </span>
                    <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] mt-1 mb-0">
                      {project.project.recruiting ? "Open to collaborators" : "Team details"}
                    </h2>
                  </div>
                  <p className="text-sm leading-[21px] text-muted m-0">
                    {project.project.recruiting
                      ? "This project is actively recruiting. Campus verification may be required to apply."
                      : "View team membership and ways to participate after signing in."}
                  </p>
                  <a
                    className="inline-flex w-full min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium no-underline border border-primary bg-primary text-on-primary hover:opacity-90"
                    href={`/projects/${encodeURIComponent(projectID)}?tab=open-roles`}
                  >
                    View team openings
                  </a>
                  <p className="text-[13px] leading-[19px] text-muted m-0">
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
