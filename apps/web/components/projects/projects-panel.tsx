"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";

type Project = { id: string; title: string; summary: string; project_type: string; visibility: string; recruiting: boolean; skills: string[] };
type Envelope<T> = { data?: T; error?: { message?: string } };

export function ProjectsPanel() {
  const [items, setItems] = useState<Project[]>([]);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(true);
  const [problem, setProblem] = useState("");
  const [signedIn, setSignedIn] = useState(false);
  const [campusAccess, setCampusAccess] = useState(false);
  const load = useCallback(async (term = query) => {
    setBusy(true); setProblem("");
    try {
      const [sessionResponse, projectResponse] = await Promise.all([
        fetch("/api/v1/session", { cache: "no-store" }),
        fetch(`/api/v1/projects${term.trim() ? `?q=${encodeURIComponent(term.trim())}` : ""}`, { cache: "no-store" }),
      ]);
      const session = await sessionResponse.json() as Envelope<{ authenticated: boolean; user?: { college_id?: string } }>;
      const projects = await projectResponse.json() as Envelope<{ items: Project[] }>;
      if (!projectResponse.ok) throw new Error(projects.error?.message ?? "Projects could not be loaded.");
      setSignedIn(Boolean(session.data?.authenticated)); setCampusAccess(Boolean(session.data?.user?.college_id)); setItems(projects.data?.items ?? []);
    } catch (error) { setProblem(error instanceof Error ? error.message : "Projects could not be loaded."); }
    finally { setBusy(false); }
  }, []);
  useEffect(() => { void load(""); }, [load]);
  function search(event: FormEvent<HTMLFormElement>) { event.preventDefault(); void load(); }
  return <section className="content" id="projects" aria-labelledby="projects-title">
    <div className="section-heading"><div><p className="eyebrow">Find your next step</p><h2 id="projects-title">Explore projects</h2></div><span className="result-count">{items.length} projects</span></div>
    <form className="search" role="search" onSubmit={search}><label htmlFor="project-search">Search projects</label><div className="search-row"><input id="project-search" type="search" value={query} onChange={event => setQuery(event.target.value)} placeholder="Try design, robotics, or research" /><button type="submit">Search</button></div></form>
    {problem && <p className="form-error" role="alert">{problem}</p>}
    {busy ? <div className="empty-state" role="status"><h3>Loading projects…</h3></div> : items.length ? <div className="project-list">{items.map(project => <article className="project-card" key={project.id}><div><p className="eyebrow">{project.project_type.replaceAll("_", " ")} · {project.visibility} · {project.recruiting ? "recruiting" : "not recruiting"}</p><h3>{project.title}</h3><p>{project.summary}</p>{project.skills.length > 0 && <ul className="skill-list">{project.skills.map(skill => <li key={skill}>{skill}</li>)}</ul>}</div><a className="secondary-button" href={`/projects/${project.id}`}>View project</a></article>)}</div> : <div className="empty-state" aria-live="polite"><span className="empty-icon" aria-hidden="true">✳</span><h3>{query.trim() ? `No public projects found for “${query.trim()}”` : campusAccess ? "No campus projects have been published yet" : "No public projects yet"}</h3><p>{campusAccess ? "Imported projects appear here after their owners choose to publish them." : "Sign in and verify a campus email to explore campus projects and collaborate."}</p>{!campusAccess && <a href={signedIn ? "/onboarding" : "/login"}>{signedIn ? "Verify campus access" : "Sign in to continue"}</a>}</div>}
  </section>;
}
