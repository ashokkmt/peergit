"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
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
        fetch("/api/v1/session", { cache: "no-store" }).catch(() => null),
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
      <div className="section-heading">
        <div>
          <h2 id="projects-title" className="t-headline-lg">
            Explore public projects
          </h2>
          <p className="t-body-md muted" style={{ margin: "6px 0 0" }}>
            Opted-in campus presentations shared with the public.
          </p>
        </div>
        <a href="/explore" className="t-label-lg" style={{ textDecoration: "none" }}>
          Browse all projects →
        </a>
      </div>

      {showSearch && (
        <form
          className="search"
          role="search"
          onSubmit={search}
          style={{ width: "100%", marginBottom: "24px" }}
        >
          <label htmlFor="project-search" className="t-label-md">
            Search projects
          </label>
          <div className="search-row">
            <input
              id="project-search"
              type="search"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Try design, robotics, or research"
            />
            <button type="submit" className="primary-button">
              Search
            </button>
          </div>
        </form>
      )}

      {problem ? (
        <div
          className="form-error"
          role="alert"
          style={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "space-between",
            gap: "12px",
            margin: 0,
          }}
        >
          <span>{problem}</span>
          <button
            type="button"
            className="secondary-button pg-btn-compact"
            onClick={() => void load(activeQuery)}
          >
            Retry
          </button>
        </div>
      ) : busy ? (
        <p className="t-body-md muted" role="status" style={{ margin: 0 }}>
          Loading public projects…
        </p>
      ) : items.length > 0 ? (
        <div className="pg-grid-3">
          {items.slice(0, 3).map((project) => (
            <a
              className="pg-card-interactive"
              key={project.id}
              href={`/showcase/${project.id}`}
            >
              <div style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: "8px",
                  }}
                >
                  <span
                    className="t-label-sm"
                    style={{ color: project.recruiting ? "var(--success)" : "var(--link)" }}
                  >
                    {project.recruiting ? "Active · Recruiting" : "Active"}
                  </span>
                  <span className="t-label-sm muted">
                    {project.project_type.replaceAll("_", " ")}
                  </span>
                </div>
                <h3 className="t-title-md">{project.title}</h3>
                <p className="t-body-md muted" style={{ margin: 0 }}>
                  {project.summary}
                </p>
              </div>
              {project.skills.length > 0 && (
                <ul
                  className="skill-list"
                  style={{ paddingTop: "12px", borderTop: "1px solid var(--hairline)" }}
                >
                  {project.skills.slice(0, 4).map((skill) => (
                    <li key={skill}>{skill}</li>
                  ))}
                </ul>
              )}
            </a>
          ))}
        </div>
      ) : activeQuery ? (
        <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-start", gap: "10px" }} aria-live="polite">
          <h3 className="t-title-md">
            No public projects found for “{activeQuery}”
          </h3>
          <p className="t-body-md muted" style={{ margin: 0 }}>
            Try another keyword or clear the search filter to browse available public showcases.
          </p>
          <button
            type="button"
            className="secondary-button pg-btn-compact"
            onClick={() => {
              setQuery("");
              void load("");
            }}
          >
            Clear search
          </button>
        </div>
      ) : (
        <div
          style={{ display: "flex", flexDirection: "column", alignItems: "flex-start", gap: "12px" }}
          aria-live="polite"
        >
          <h3 className="t-title-md">Be the first to share a public project</h3>
          <p className="t-body-md muted" style={{ margin: 0, maxWidth: "58ch" }}>
            Public project showcases are opt-in. Connect your repository and publish a project
            card so students and visitors can explore what your team is building.
          </p>
          <a className="primary-link" href={shareHref}>
            {shareLabel}
          </a>
        </div>
      )}
    </div>
  );
}
