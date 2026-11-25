"use client";

import { FormEvent, useEffect, useState } from "react";
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

export function ExploreResults() {
  const [items, setItems] = useState<Project[]>([]);
  const [query, setQuery] = useState("");
  const [request, setRequest] = useState({ term: "", scope: "public" });
  const [scope, setScope] = useState("public");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    const url = new URL("/api/v1/projects", window.location.origin);
    url.searchParams.set("scope", request.scope);
    if (request.term) url.searchParams.set("q", request.term);
    fetch(url, { cache: "no-store" })
      .then(async (response) => {
        const result = (await response.json()) as Envelope<{
          items: Project[];
        }>;
        if (!response.ok)
          throw new Error(
            result.error?.message ?? "Projects could not be loaded.",
          );
        if (active)
          setItems(
            (result.data?.items ?? []).filter(
              (item) => item.visibility === request.scope,
            ),
          );
      })
      .catch(
        (reason) =>
          active &&
          setError(
            reason instanceof Error
              ? reason.message
              : "Projects could not be loaded.",
          ),
      )
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, [request]);
  function search() {
    setLoading(true);
    setError("");
    setItems([]);
    setRequest({ term: query.trim(), scope });
  }
  return (
    <>
      <div className="section-heading">
        <div>
          <p className="eyebrow">Find a place to contribute</p>
          <h1>Explore projects</h1>
          <p className="muted">
            Discover opted-in project presentations. Campus-only work appears
            after campus verification.
          </p>
        </div>
        <span className="chip">Public showcase</span>
      </div>
      <form
        className="search"
        role="search"
        onSubmit={(event: FormEvent<HTMLFormElement>) => {
          event.preventDefault();
          search();
        }}
      >
        <label htmlFor="explore-search">
          Search by project, skill or topic
        </label>
        <div className="search-row">
          <input
            id="explore-search"
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Try robotics, design or accessibility"
          />
          <button type="submit">Search</button>
        </div>
        <label>
          Audience
          <select
            value={scope}
            onChange={(event) => setScope(event.target.value)}
          >
            <option value="public">Public showcase</option>
            <option value="campus">My campus</option>
          </select>
        </label>
      </form>
      <div className="inline-meta">
        <span>Public showcase</span>
        <a href="/onboarding">Verify campus for campus-only discovery</a>
        <span>Projects shown only when owners choose to publish</span>
      </div>
      {error && (
        <div className="form-error" role="alert">
          {error}{" "}
          <button className="secondary-button" onClick={search}>
            Retry
          </button>
        </div>
      )}
      {error ? null : loading ? (
        <div className="empty-state" role="status">
          Loading public projects…
        </div>
      ) : items.length ? (
        <div className="project-grid">
          {items.map((project) => (
            <article className="project-card" key={project.id}>
              <div>
                <p className="eyebrow">
                  {project.project_type.replaceAll("_", " ")} ·{" "}
                  {project.recruiting ? "Recruiting" : "Project"}
                </p>
                <h2>{project.title}</h2>
                <p>{project.summary}</p>
                <ul className="skill-list">
                  {project.skills.slice(0, 4).map((skill) => (
                    <li key={skill}>{skill}</li>
                  ))}
                </ul>
              </div>
              <a
                className="secondary-button"
                href={`/${request.scope === "public" ? "showcase" : "projects"}/${project.id}`}
              >
                View project <span aria-hidden="true">→</span>
              </a>
            </article>
          ))}
        </div>
      ) : (
        <div className="empty-state">
          <span className="empty-icon" aria-hidden="true">
            ↗
          </span>
          <h2>No public projects yet</h2>
          <p>
            Public discovery only includes project presentations their owners
            have chosen to share. You can still create an account and verify
            your campus to find campus projects.
          </p>
          <a className="primary-link" href="/signup">
            Join PeerGit
          </a>
        </div>
      )}
    </>
  );
}
