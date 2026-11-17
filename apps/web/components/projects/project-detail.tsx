"use client";

import { FormEvent, useEffect, useState } from "react";
import { RepositoryOverview } from "./repository-overview";
import { RepositoryCode } from "./repository-code";
import { LegacyRepositoryLink } from "./legacy-repository-link";
import { SiteHeader } from "@/components/layout/site-header";

type Envelope<T> = { data?: T; error?: { message?: string } };
type Project = { id: string; title: string; summary: string; visibility: string; lifecycle: string; project_type: string; recruiting: boolean; skills: string[]; version: number };
type Member = { user_id: string; display_name: string; role: string };
type Role = { id: string; title: string; description: string; openings: number; status: string; version: number; difficulty: string; good_first_task: boolean };
type Application = { id: string; role_id: string; applicant_name: string; message: string; status: string; version: number };
type Details = { project: Project; description: string; viewer_role: string; members: Member[] };

export function ProjectDetail({ projectID }: { projectID: string }) {
  const [details, setDetails] = useState<Details | null>(null);
  const [roles, setRoles] = useState<Role[]>([]);
  const [applications, setApplications] = useState<Application[]>([]);
  const [csrf, setCSRF] = useState("");
  const [campusAccess, setCampusAccess] = useState(false);
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");
  const [hasRepository, setHasRepository] = useState(false);
  const [summary, setSummary] = useState("");
  const [description, setDescription] = useState("");
  const [roleTitle, setRoleTitle] = useState("");
  const [roleDescription, setRoleDescription] = useState("");
  const [roleOpenings, setRoleOpenings] = useState("1");
  const [applicationMessage, setApplicationMessage] = useState<Record<string, string>>({});
  const [tab,setTab]=useState("code");

  useEffect(()=>{setTab(new URLSearchParams(window.location.search).get("tab")||"code")},[]);

  async function loadRoles() {
    const response = await fetch(`/api/v1/projects/${projectID}/roles`, { cache: "no-store" });
    if (!response.ok) return;
    const result = await response.json() as Envelope<{ items: Role[] }>;
    setRoles(result.data?.items ?? []);
  }

  async function loadApplications() {
    const response = await fetch(`/api/v1/projects/${projectID}/applications`, { cache: "no-store" });
    if (!response.ok) return;
    const result = await response.json() as Envelope<{ items: Application[] }>;
    setApplications(result.data?.items ?? []);
  }

  useEffect(() => {
    let active = true;
    void Promise.all([
      fetch(`/api/v1/projects/${projectID}`, { cache: "no-store" }).then(async response => {
        const result = await response.json() as Envelope<Details>;
        if (!response.ok || !result.data) throw new Error(result.error?.message ?? "Project could not be loaded.");
        return result.data;
      }),
      fetch("/api/v1/session", { cache: "no-store" }).then(response => response.json()).then((result: { data?: { csrf_token?: string; user?: { college_id?: string; terms_accepted?: boolean; privacy_accepted?: boolean } } }) => result.data),
    ]).then(async ([project, session]) => {
      if (!active) return;
      setDetails(project);
      setSummary(project.project.summary);
      setDescription(project.description);
      setCSRF(session?.csrf_token ?? "");
      const ready = Boolean(session?.user?.college_id && session.user.terms_accepted && session.user.privacy_accepted);
      setCampusAccess(ready);
      if (project.viewer_role) {
        const response = await fetch(`/api/v1/projects/${projectID}/repository`, { cache: "no-store" });
        if (response.ok) {
          const result = await response.json() as Envelope<{ repository: unknown }>;
          if (active) setHasRepository(Boolean(result.data?.repository));
        }
      }
      if (ready) await loadRoles();
      if (["owner", "maintainer"].includes(project.viewer_role)) await loadApplications();
    }).catch(error => active && setProblem(error instanceof Error ? error.message : "Project could not be loaded."));
    return () => { active = false; };
  }, [projectID]);

  async function mutate(path: string, body: unknown, success: string, method = "POST") {
    setProblem(""); setMessage("");
    const response = await fetch(path, { method, credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(body) });
    const result = await response.json() as Envelope<{ version?: number }>;
    if (!response.ok) throw new Error(result.error?.message ?? "The change could not be saved.");
    setMessage(success);
    return result.data;
  }

  async function saveDetails(event: FormEvent) {
    event.preventDefault();
    if (!details) return;
    try {
      const result = await mutate(`/api/v1/projects/${projectID}`, { summary, description, visibility: details.project.visibility, lifecycle: details.project.lifecycle, version: details.project.version }, "Project details saved.", "PATCH");
      setDetails({ ...details, description, project: { ...details.project, summary, version: result?.version ?? details.project.version } });
    } catch (error) { setProblem(error instanceof Error ? error.message : "Project details could not be saved."); }
  }

  async function publish(visibility: "private" | "campus" | "public") {
    if (!details) return;
    try {
      const success=visibility==="private"?"Project made private.":`Project published to ${visibility === "campus" ? "your campus" : "the public discovery page"}. GitHub source files and private snapshots keep their existing access rules.`;
      const result = await mutate(`/api/v1/projects/${projectID}`, { summary, description, visibility, lifecycle: "active", version: details.project.version }, success, "PATCH");
      setDetails({ ...details, description, project: { ...details.project, summary, visibility, lifecycle: "active", version: result?.version ?? details.project.version } });
    } catch (error) { setProblem(error instanceof Error ? error.message : "Project could not be published."); }
  }

  async function createRole(event: FormEvent) {
    event.preventDefault();
    try {
      await mutate(`/api/v1/projects/${projectID}/roles`, { title: roleTitle, description: roleDescription, openings: Number(roleOpenings), difficulty: "beginner", good_first_task: false }, "Team opening created.");
      setRoleTitle(""); setRoleDescription(""); setRoleOpenings("1"); await loadRoles();
    } catch (error) { setProblem(error instanceof Error ? error.message : "Opening could not be created."); }
  }

  async function apply(role: Role) {
    try {
      await mutate(`/api/v1/projects/${projectID}/applications`, { role_id: role.id, message: applicationMessage[role.id] ?? "" }, "Application sent.");
      setApplicationMessage(value => ({ ...value, [role.id]: "" }));
    } catch (error) { setProblem(error instanceof Error ? error.message : "Application could not be sent."); }
  }

  async function decide(application: Application, decision: "accepted" | "rejected") {
    try {
      await mutate(`/api/v1/projects/${projectID}/applications/${application.id}/decision`, { decision, version: application.version }, `Application ${decision}.`);
      await loadApplications();
    } catch (error) { setProblem(error instanceof Error ? error.message : "Application decision could not be saved."); }
  }

  const manager = Boolean(details && ["owner", "maintainer"].includes(details.viewer_role));
  return <><SiteHeader /><main className="content project-detail-page">
    {problem && <p className="form-error" role="alert">{problem}</p>}{message && <p className="form-success" role="status">{message}</p>}
    {!details ? <p role="status">Loading project…</p> : <>
      <header className="project-workspace-header"><p className="eyebrow">{details.project.project_type.replaceAll("_", " ")} · PeerGit {details.project.visibility} · {details.project.lifecycle}</p><h1>{details.project.title}</h1><p className="project-summary">{details.project.summary}</p>{details.viewer_role&&<p>Source visibility and access are governed by GitHub. PeerGit publication does not expose private repository files.</p>}</header>
      <nav className="project-tabs" aria-label="Project sections">{[["code","Code"],["activity","Activity"],["open-roles","Open roles"],["snapshots","Snapshots"],...(manager?[["settings","Settings"]]:[])].map(([value,label])=><a key={value} href={`/projects/${projectID}?tab=${value}`} aria-current={tab===value?"page":undefined}>{label}</a>)}</nav>
      {tab==="code"&&details.viewer_role&&hasRepository&&<RepositoryCode projectID={projectID} branch="main"/>}
      {tab==="code"&&details.viewer_role&&hasRepository&&<RepositoryOverview projectID={projectID} csrf={csrf} tab="code"/>}
      {tab==="activity"&&details.viewer_role&&hasRepository&&<RepositoryOverview projectID={projectID} csrf={csrf} tab="activity"/>}
      {tab==="snapshots"&&details.viewer_role&&hasRepository&&<RepositoryOverview projectID={projectID} csrf={csrf} tab="snapshots"/>}
      {tab==="settings"&&manager&&hasRepository&&<section className="account-form"><h2>Project settings</h2><p>PeerGit visibility: <strong>{details.project.visibility}</strong> · lifecycle: {details.project.lifecycle}</p><form onSubmit={event => void saveDetails(event)}><label>Summary<textarea required maxLength={500} value={summary} onChange={event => setSummary(event.target.value)} /></label><label>Description<textarea maxLength={12000} rows={6} value={description} onChange={event => setDescription(event.target.value)} /></label><button className="primary-button" type="submit">Save details</button></form><h3>Publication</h3><p>Project publication shares its PeerGit presentation. GitHub visibility does not change, and private source, evidence, and snapshots remain restricted to project members.</p>{details.project.lifecycle!=="archived"&&<><button className="secondary-button" type="button" onClick={() => void publish("campus")}>Publish to campus</button> <button className="secondary-button" type="button" onClick={() => void publish("public")}>Publish publicly</button></>}{details.project.visibility!=="private"&&<button className="secondary-button" type="button" onClick={()=>void publish("private")}>Make private</button>}{details.project.lifecycle!=="archived"&&<button className="secondary-button" type="button" onClick={()=>void mutate(`/api/v1/projects/${projectID}`,{summary,description,visibility:"private",lifecycle:"archived",version:details.project.version},"Project archived.","PATCH")}>Archive project</button>}<h3>GitHub connection</h3><p>Repository access follows the connected GitHub App installation. Manage or revoke your account connection from <a href="/account/github">GitHub connections</a>.</p></section>}
      {tab==="code"&&(!details.viewer_role||!hasRepository)&&manager&&<LegacyRepositoryLink projectID={projectID} csrf={csrf} />}
      {tab==="open-roles"&&<section className="account-form"><h2>Open roles</h2><h3>Team</h3>{details.members.length ? <ul>{details.members.map(member => <li key={member.user_id}>{member.display_name} · {member.role}</li>)}</ul> : <p>No team members are listed.</p>}{roles.filter(role => role.status === "open").length ? roles.filter(role => role.status === "open").map(role => <article className="role-card" key={role.id}><h3>{role.title}</h3><p>{role.description}</p><p>{role.openings} opening(s) · {role.difficulty}{role.good_first_task ? " · beginner-friendly" : ""}</p>{campusAccess && details.project.lifecycle === "active" && !manager && <form onSubmit={event => { event.preventDefault(); void apply(role); }}><label>Why are you interested?<textarea required maxLength={2000} value={applicationMessage[role.id] ?? ""} onChange={event => setApplicationMessage(value => ({ ...value, [role.id]: event.target.value }))} /></label><button className="secondary-button" type="submit">Apply</button></form>}</article>) : <p>This project has no open roles.</p>}
        {manager && hasRepository && details.project.lifecycle === "active" && <form className="project-role-form" onSubmit={event => void createRole(event)}><h3>Open a team role</h3><label>Role title<input required minLength={2} maxLength={100} value={roleTitle} onChange={event => setRoleTitle(event.target.value)} /></label><label>What will the member work on?<textarea required maxLength={3000} value={roleDescription} onChange={event => setRoleDescription(event.target.value)} /></label><label>Number of openings<input required type="number" min={1} max={20} value={roleOpenings} onChange={event => setRoleOpenings(event.target.value)} /></label><button className="primary-button" type="submit">Create opening</button></form>}
      </section>}
      {tab==="open-roles"&&manager&&applications.length > 0 && <section className="account-form"><h2>Applications</h2>{applications.map(application => <article className="role-card" key={application.id}><h3>{application.applicant_name} · {application.status}</h3><p>{application.message}</p>{application.status === "pending" && <><button className="secondary-button" type="button" onClick={() => void decide(application, "accepted")}>Accept</button> <button className="secondary-button" type="button" onClick={() => void decide(application, "rejected")}>Decline</button></>}</article>)}</section>}
    </>}
  </main></>;
}
