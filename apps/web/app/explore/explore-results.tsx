"use client";

import { FormEvent, useEffect, useState } from "react";
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
  "Couldn't load projects right now. Check that the backend API is running and try again.";

export function ExploreResults() {
  const [items, setItems] = useState<Project[]>([]);
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("all");
  const [recruitingOnly, setRecruitingOnly] = useState(false);
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
        const result = await readApiEnvelope<{ items: Project[] }>(response, apiLoadError);
        if (!response.ok)
          throw new Error(result.error?.message ?? apiLoadError);
        if (active)
          setItems(
            (result.data?.items ?? []).filter((item) => item.visibility === request.scope),
          );
      })
      .catch(
        (reason) =>
          active &&
          setError(
            reason instanceof Error &&
              !reason.message.includes("<!DOCTYPE") &&
              !reason.message.includes("not valid JSON")
              ? reason.message
              : apiLoadError,
          ),
      )
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, [request]);

  function runSearch(nextScope = scope, nextTerm = query.trim()) {
    setLoading(true);
    setError("");
    setItems([]);
    setRequest({ term: nextTerm, scope: nextScope });
  }

  function resetFilters() {
    setQuery("");
    setCategory("all");
    setRecruitingOnly(false);
    setScope("public");
    runSearch("public", "");
  }

  const filtered = items.filter((item) => {
    if (category !== "all" && item.project_type !== category) return false;
    if (recruitingOnly && !item.recruiting) return false;
    return true;
  });

  const categories = Array.from(new Set(items.map((item) => item.project_type).filter(Boolean)));

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "28px" }}>
      {/* Page Heading & Audience Scope Switch */}
      <div className="section-heading" style={{ marginBottom: 0 }}>
        <div>
          <h1 className="t-headline-lg">Explore projects</h1>
          <p className="t-body-md muted" style={{ margin: "6px 0 0" }}>
            Showing opted-in public presentations. Verify your campus to switch to campus-only
            projects.
          </p>
        </div>

        <div
          role="group"
          aria-label="Discovery scope"
          style={{
            display: "inline-flex",
            padding: "4px",
            borderRadius: "10px",
            background: "var(--raised)",
            border: "1px solid var(--hairline)",
            gap: "4px",
          }}
        >
          <button
            type="button"
            aria-pressed={scope === "public"}
            onClick={() => {
              setScope("public");
              runSearch("public", query.trim());
            }}
            style={{
              padding: "6px 14px",
              borderRadius: "7px",
              border: 0,
              cursor: "pointer",
              fontSize: "13px",
              fontWeight: 500,
              background: scope === "public" ? "var(--surface)" : "transparent",
              color: scope === "public" ? "var(--text)" : "var(--muted)",
            }}
          >
            Public showcase
          </button>
          <button
            type="button"
            aria-pressed={scope === "campus"}
            onClick={() => {
              setScope("campus");
              runSearch("campus", query.trim());
            }}
            style={{
              padding: "6px 14px",
              borderRadius: "7px",
              border: 0,
              cursor: "pointer",
              fontSize: "13px",
              fontWeight: 500,
              background: scope === "campus" ? "var(--surface)" : "transparent",
              color: scope === "campus" ? "var(--text)" : "var(--muted)",
            }}
          >
            My campus
          </button>
        </div>
      </div>

      {/* Search & Filters */}
      <section aria-label="Filter projects" className="pg-card" style={{ display: "grid", gap: "16px" }}>
        <form
          className="search"
          role="search"
          onSubmit={(event: FormEvent<HTMLFormElement>) => {
            event.preventDefault();
            runSearch();
          }}
        >
          <div className="explore-filter-grid">
            <label className="pg-field" htmlFor="explore-search">
              Search projects
              <input
                id="explore-search"
                type="search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search title, skill, or topic"
              />
            </label>

            <label className="pg-field" htmlFor="explore-category">
              Category
              <select
                id="explore-category"
                value={category}
                onChange={(event) => setCategory(event.target.value)}
              >
                <option value="all">All categories</option>
                {categories.map((cat) => (
                  <option key={cat} value={cat}>
                    {cat.replaceAll("_", " ")}
                  </option>
                ))}
              </select>
            </label>

            <label className="check-row" htmlFor="recruiting-only" style={{ minHeight: "44px", alignItems: "center" }}>
              <input
                id="recruiting-only"
                type="checkbox"
                checked={recruitingOnly}
                onChange={(event) => setRecruitingOnly(event.target.checked)}
              />
              Open roles only
            </label>

            <button type="submit" className="primary-button">
              Search
            </button>
          </div>
        </form>

        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "space-between",
            gap: "12px",
            paddingTop: "12px",
            borderTop: "1px solid var(--hairline)",
          }}
        >
          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: "8px" }}>
            <span className="t-label-md tabular-nums">
              {filtered.length} {filtered.length === 1 ? "result" : "results"}
            </span>
            {category !== "all" && (
              <span className="pg-pill pg-pill-raised">{category.replaceAll("_", " ")}</span>
            )}
            {recruitingOnly && <span className="pg-pill pg-pill-raised">Open roles only</span>}
            {scope === "campus" && <span className="pg-pill pg-pill-raised">My campus</span>}
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: "16px" }}>
            <a href="/onboarding" className="t-label-md" style={{ textDecoration: "none" }}>
              Verify campus for campus-only discovery
            </a>
            <button type="button" className="text-button t-label-md" onClick={resetFilters}>
              Reset filters
            </button>
          </div>
        </div>
      </section>

      {error && (
        <div className="form-error" role="alert" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "12px", flexWrap: "wrap" }}>
          <span>{error}</span>
          <button type="button" className="secondary-button pg-btn-compact" onClick={() => runSearch()}>
            Retry
          </button>
        </div>
      )}

      {!error && loading ? (
        <div className="empty-state" role="status">
          Loading projects…
        </div>
      ) : !error && filtered.length > 0 ? (
        <section aria-label="Project results" className="pg-grid-3">
          {filtered.map((project) => (
            <a
              key={project.id}
              className="pg-card-interactive"
              href={`/${request.scope === "public" ? "showcase" : "projects"}/${project.id}`}
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
                <h2 className="t-title-md">{project.title}</h2>
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
        </section>
      ) : !error ? (
        <div className="empty-state">
          <h2 className="t-title-md">No public projects yet</h2>
          <p className="t-body-md muted">
            Public discovery only includes project presentations their owners have chosen to share.
            You can still create an account and verify your campus to find campus projects.
          </p>
          <a className="primary-link" href="/signup">
            Join PeerGit
          </a>
        </div>
      ) : null}
    </div>
  );
}
