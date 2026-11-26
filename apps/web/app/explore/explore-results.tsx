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
        if (!response.ok) throw new Error(result.error?.message ?? apiLoadError);
        if (active) {
          setItems((result.data?.items ?? []).filter((item) => item.visibility === request.scope));
        }
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
    <div className="flex flex-col gap-7">
      {/* Page Heading & Audience Scope Switch */}
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="font-display text-[clamp(1.75rem,3.2vw,2.75rem)] font-bold leading-[1.14] tracking-[-0.02em] m-0">
            Explore projects
          </h1>
          <p className="text-sm leading-[21px] text-muted mt-1.5 mb-0">
            Showing opted-in public presentations. Verify your campus to switch to campus-only
            projects.
          </p>
        </div>

        <div
          role="group"
          aria-label="Discovery scope"
          className="inline-flex p-1 rounded-[10px] bg-raised border border-hairline gap-1"
        >
          <button
            type="button"
            aria-pressed={scope === "public"}
            onClick={() => {
              setScope("public");
              runSearch("public", query.trim());
            }}
            className="px-3.5 py-1.5 rounded-[7px] border-0 cursor-pointer text-[13px] font-medium bg-transparent text-muted aria-pressed:bg-surface aria-pressed:text-on-surface"
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
            className="px-3.5 py-1.5 rounded-[7px] border-0 cursor-pointer text-[13px] font-medium bg-transparent text-muted aria-pressed:bg-surface aria-pressed:text-on-surface"
          >
            My campus
          </button>
        </div>
      </div>

      {/* Search & Filters */}
      <section
        aria-label="Filter projects"
        className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 grid gap-4"
      >
        <form
          className="w-full grid gap-2.5"
          role="search"
          onSubmit={(event: FormEvent<HTMLFormElement>) => {
            event.preventDefault();
            runSearch();
          }}
        >
          <div className="grid grid-cols-1 md:grid-cols-[minmax(280px,3fr)_minmax(170px,1.2fr)_auto_auto] gap-3.5 items-end">
            <label className="grid gap-1.5 text-[13px] font-medium text-on-surface" htmlFor="explore-search">
              Search projects
              <input
                id="explore-search"
                type="search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search title, skill, or topic"
                className="w-full min-h-11 px-3.5 py-2.5 border border-control rounded-lg bg-surface text-on-surface text-sm leading-5"
              />
            </label>

            <label className="grid gap-1.5 text-[13px] font-medium text-on-surface" htmlFor="explore-category">
              Category
              <select
                id="explore-category"
                value={category}
                onChange={(event) => setCategory(event.target.value)}
                className="w-full min-h-11 px-3.5 py-2.5 border border-control rounded-lg bg-surface text-on-surface text-sm leading-5"
              >
                <option value="all">All categories</option>
                {categories.map((cat) => (
                  <option key={cat} value={cat}>
                    {cat.replaceAll("_", " ")}
                  </option>
                ))}
              </select>
            </label>

            <label
              className="flex items-center gap-2.5 min-h-11 text-sm leading-5 font-normal text-on-surface cursor-pointer"
              htmlFor="recruiting-only"
            >
              <input
                id="recruiting-only"
                type="checkbox"
                checked={recruitingOnly}
                onChange={(event) => setRecruitingOnly(event.target.checked)}
                className="w-4 h-4 accent-primary shrink-0"
              />
              Open roles only
            </label>

            <button
              type="submit"
              className="inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-[18px] py-2 text-sm font-medium leading-5 whitespace-nowrap border border-primary bg-primary text-on-primary cursor-pointer hover:opacity-90 transition-opacity duration-100"
            >
              Search
            </button>
          </div>
        </form>

        <div className="flex flex-wrap items-center justify-between gap-3 pt-3 border-t border-hairline">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-[13px] font-medium leading-[18px] tabular-nums">
              {filtered.length} {filtered.length === 1 ? "result" : "results"}
            </span>
            {category !== "all" && (
              <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-raised border border-hairline text-xs font-medium leading-4">
                {category.replaceAll("_", " ")}
              </span>
            )}
            {recruitingOnly && (
              <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-raised border border-hairline text-xs font-medium leading-4">
                Open roles only
              </span>
            )}
            {scope === "campus" && (
              <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-raised border border-hairline text-xs font-medium leading-4">
                My campus
              </span>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-4">
            <a href="/onboarding" className="text-[13px] font-medium leading-[18px] text-link no-underline hover:underline">
              Verify campus for campus-only discovery
            </a>
            <button
              type="button"
              className="border-0 py-1 px-0 bg-transparent text-link cursor-pointer text-[13px] font-medium leading-[18px] hover:underline"
              onClick={resetFilters}
            >
              Reset filters
            </button>
          </div>
        </div>
      </section>

      {error && (
        <div
          className="rounded-lg px-4 py-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger flex items-center justify-between gap-3 flex-wrap"
          role="alert"
        >
          <span>{error}</span>
          <button
            type="button"
            className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised"
            onClick={() => runSearch()}
          >
            Retry
          </button>
        </div>
      )}

      {!error && loading ? (
        <div
          className="grid justify-items-center content-center gap-2.5 min-h-[220px] p-8 border border-dashed border-control rounded-2xl text-center bg-surface"
          role="status"
        >
          Loading projects…
        </div>
      ) : !error && filtered.length > 0 ? (
        <section aria-label="Project results" className="grid grid-cols-1 min-[900px]:grid-cols-3 gap-6">
          {filtered.map((project) => (
            <a
              key={project.id}
              className="bg-surface border border-hairline rounded-2xl p-6 no-underline text-on-surface flex flex-col justify-between gap-4 hover:border-link transition-colors duration-100"
              href={`/${request.scope === "public" ? "showcase" : "projects"}/${project.id}`}
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
                <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                  {project.title}
                </h2>
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
        </section>
      ) : !error ? (
        <div className="grid justify-items-center content-center gap-2.5 min-h-[220px] p-8 border border-dashed border-control rounded-2xl text-center bg-surface">
          <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
            No public projects yet
          </h2>
          <p className="text-sm leading-[21px] text-muted max-w-[500px] m-0">
            Public discovery only includes project presentations their owners have chosen to share.
            You can still create an account and verify your campus to find campus projects.
          </p>
          <a
            className="inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-[18px] py-2 text-sm font-medium leading-5 no-underline whitespace-nowrap border border-primary bg-primary text-on-primary hover:opacity-90"
            href="/signup"
          >
            Join PeerGit
          </a>
        </div>
      ) : null}
    </div>
  );
}
