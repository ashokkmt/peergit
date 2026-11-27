"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { fetchSession, readApiEnvelope } from "@/lib/http";

type Project = {
  id: string;
  title: string;
  summary: string;
  project_type: string;
  visibility: string;
  recruiting: boolean;
  skills: string[];
};

const apiLoadError =
  "Couldn't fetch public projects right now. Check that the backend API is running and try again.";

export function ProjectsPanel() {
  const [items, setItems] = useState<Project[]>([]);
  const [query, setQuery] = useState("");
  const [activeQuery, setActiveQuery] = useState("");
  const [busy, setBusy] = useState(true);
  const [problem, setProblem] = useState("");
  const [signedIn, setSignedIn] = useState(false);
  const [campusAccess, setCampusAccess] = useState(false);

  const load = useCallback(async (term = "") => {
    const trimmed = term.trim();
    setBusy(true);
    setProblem("");
    try {
      const [sessionResponse, projectResponse] = await Promise.all([
        fetchSession().catch(() => null),
        fetch(
          `/api/v1/projects?scope=public${trimmed ? `&q=${encodeURIComponent(trimmed)}` : ""}`,
          { cache: "no-store" },
        ),
      ]);
      const projects = await readApiEnvelope<{ items: Project[] }>(
        projectResponse,
        apiLoadError,
      );
      if (!projectResponse.ok) {
        throw new Error(projects.error?.message ?? apiLoadError);
      }
      if (sessionResponse) {
        try {
          const session = await readApiEnvelope<{
            authenticated: boolean;
            user?: { college_id?: string };
          }>(sessionResponse, "");
          setSignedIn(Boolean(session.data?.authenticated));
          setCampusAccess(Boolean(session.data?.user?.college_id));
        } catch {
          setSignedIn(false);
          setCampusAccess(false);
        }
      }
      setActiveQuery(trimmed);
      setItems(projects.data?.items ?? []);
    } catch (error) {
      const message =
        error instanceof Error &&
        !error.message.includes("<!DOCTYPE") &&
        !error.message.includes("not valid JSON")
          ? error.message
          : apiLoadError;
      setProblem(message);
    } finally {
      setBusy(false);
    }
  }, []);

  useEffect(() => {
    void load("");
  }, [load]);

  function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void load(query);
  }

  const showSearch = !problem && (items.length > 0 || activeQuery.length > 0);
  const shareHref = campusAccess
    ? "/projects/import"
    : signedIn
      ? "/onboarding"
      : "/signup";
  const shareLabel = campusAccess
    ? "Add your project"
    : signedIn
      ? "Verify campus to add a project"
      : "Join PeerGit to add your project";

  return (
    <div id="projects">
      <div className="flex flex-wrap items-end justify-between gap-4 mb-6">
        <div>
          <h2
            id="projects-title"
            className="font-display text-[clamp(1.75rem,3.2vw,2.75rem)] font-bold leading-[1.14] tracking-[-0.02em] m-0"
          >
            Explore public projects
          </h2>
          <p className="text-sm leading-[21px] text-muted mt-1.5 mb-0">
            Opted-in campus presentations shared with the public.
          </p>
        </div>
        <a href="/explore" className="text-sm font-medium leading-5 text-link no-underline hover:underline">
          Browse all projects →
        </a>
      </div>

      {showSearch && (
        <form className="w-full grid gap-2.5 mb-6" role="search" onSubmit={search}>
          <label htmlFor="project-search" className="text-[13px] font-medium leading-[18px]">
            Search projects
          </label>
          <div className="flex flex-wrap items-stretch gap-2.5 w-full">
            <input
              id="project-search"
              type="search"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Try design, robotics, or research"
              className="flex-[1_1_240px] w-auto min-w-0 min-h-11 px-3.5 py-2.5 border border-control rounded-lg bg-surface text-on-surface text-sm leading-5"
            />
            <button
              type="submit"
              className="inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-[18px] py-2 text-sm font-medium leading-5 whitespace-nowrap border border-primary bg-primary text-on-primary cursor-pointer hover:opacity-90 transition-opacity duration-100"
            >
              Search
            </button>
          </div>
        </form>
      )}

      {problem ? (
        <div
          className="rounded-lg px-4 py-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger flex flex-wrap items-center justify-between gap-3 m-0"
          role="alert"
        >
          <span>{problem}</span>
          <button
            type="button"
            className="inline-flex min-h-[38px] items-center justify-center gap-2 rounded-lg px-3.5 py-1.5 text-[13px] font-medium leading-5 whitespace-nowrap border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised"
            onClick={() => void load(activeQuery)}
          >
            Retry
          </button>
        </div>
      ) : busy ? (
        <p className="text-sm leading-[21px] text-muted m-0" role="status">
          Loading public projects…
        </p>
      ) : items.length > 0 ? (
        <div className="grid grid-cols-1 min-[900px]:grid-cols-3 gap-6">
          {items.slice(0, 3).map((project) => (
            <a
              className="bg-surface border border-hairline rounded-2xl p-6 no-underline text-on-surface flex flex-col justify-between gap-4 hover:border-link transition-colors duration-100"
              key={project.id}
              href={`/showcase/${project.id}`}
            >
              <div className="flex flex-col gap-2">
                <div className="flex items-center justify-between gap-2">
                  <span
                    className={`text-xs font-medium leading-4 ${
                      project.recruiting ? "text-success" : "text-link"
                    }`}
                  >
                    {project.recruiting ? "Active · Recruiting" : "Active"}
                  </span>
                  <span className="text-xs font-medium leading-4 text-muted">
                    {project.project_type.replaceAll("_", " ")}
                  </span>
                </div>
                <h3 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                  {project.title}
                </h3>
                <p className="text-sm leading-[21px] text-muted m-0">{project.summary}</p>
              </div>
              {project.skills.length > 0 && (
                <ul className="flex flex-wrap gap-1.5 list-none m-0 pt-3 px-0 pb-0 border-t border-hairline">
                  {project.skills.slice(0, 4).map((skill) => (
                    <li
                      key={skill}
                      className="inline-flex items-center px-2.5 py-1 rounded bg-raised text-on-surface text-xs font-medium leading-4"
                    >
                      {skill}
                    </li>
                  ))}
                </ul>
              )}
            </a>
          ))}
        </div>
      ) : activeQuery ? (
        <div className="flex flex-col items-start gap-2.5" aria-live="polite">
          <h3 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
            No public projects found for “{activeQuery}”
          </h3>
          <p className="text-sm leading-[21px] text-muted m-0">
            Try another keyword or clear the search filter to browse available public showcases.
          </p>
          <button
            type="button"
            className="inline-flex min-h-[38px] items-center justify-center gap-2 rounded-lg px-3.5 py-1.5 text-[13px] font-medium leading-5 whitespace-nowrap border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised"
            onClick={() => {
              setQuery("");
              void load("");
            }}
          >
            Clear search
          </button>
        </div>
      ) : (
        <div className="flex flex-col items-start gap-3" aria-live="polite">
          <h3 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
            Be the first to share a public project
          </h3>
          <p className="text-sm leading-[21px] text-muted m-0 max-w-[58ch]">
            Public project showcases are opt-in. Connect your repository and publish a project
            card so students and visitors can explore what your team is building.
          </p>
          <a
            className="inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-[18px] py-2 text-sm font-medium leading-5 no-underline whitespace-nowrap border border-primary bg-primary text-on-primary hover:opacity-90 transition-opacity duration-100"
            href={shareHref}
          >
            {shareLabel}
          </a>
        </div>
      )}
    </div>
  );
}
