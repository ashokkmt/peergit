"use client";

import { FormEvent, useEffect, useState } from "react";
import { RepositoryPanel } from "./repository-panel";

type Envelope<T> = { data?: T; error?: { message: string } };
type Session = { authenticated: boolean; csrf_token?: string; user?: { college_id?: string } };
type Role = { id: string; title: string; description: string; openings: number; status: string; version: number; difficulty: string; good_first_task: boolean; prerequisite_skills: string[] };
type Application = { id: string; role_id: string; applicant_user_id: string; applicant_name: string; message: string; status: string; version: number };
type Project = { id: string; slug: string; title: string; summary: string; project_type: string; visibility: string; lifecycle: string; version: number; recruiting: boolean; skills: string[] };
type ProjectDetails = { project: Project; description: string; viewer_role: string; members: { user_id: string; display_name: string; role: string }[] };
type ProjectDetailState = ProjectDetails & { roles: Role[] };
type Invitation = { id: string; project_id: string; project_title: string; role: string; expires_at: string };
type MyApplication = { id: string; project_id: string; project_title: string; role_title: string; status: string; version: number };

export function ProjectsPanel() {
  const [items, setItems] = useState<Project[]>([]);
  const [csrf, setCsrf] = useState("");
  const [signedIn, setSignedIn] = useState(false);
  const [campusAccess, setCampusAccess] = useState(false);
  const [busy, setBusy] = useState(true);
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");
  const [query, setQuery] = useState("");
  const [active, setActive] = useState<ProjectDetailState | null>(null);
  const [invitations, setInvitations] = useState<Invitation[]>([]);
  const [myApplications, setMyApplications] = useState<MyApplication[]>([]);
  const [applications, setApplications] = useState<Application[]>([]);
  const [people, setPeople] = useState<{ id: string; display_name: string; handle: string }[]>([]);
  const [inviteTokens, setInviteTokens] = useState<Record<string, string>>({});
  const [inviteLink, setInviteLink] = useState("");
  const [roleID, setRoleID] = useState("");
  const [applicationMessage, setApplicationMessage] = useState("");

  useEffect(() => {
    const params = new URLSearchParams(window.location.hash.slice(1)); const id = params.get("project_invitation"), token = params.get("token");
    if (id && token) { setInviteTokens({ [id]: token }); sessionStorage.setItem("pending-project-invitation", JSON.stringify({ id, token })); window.history.replaceState({}, "", window.location.pathname + window.location.search); }
    else { try { const saved = sessionStorage.getItem("pending-project-invitation"); if (saved) { const pending = JSON.parse(saved) as { id: string; token: string }; if (pending.id && pending.token) setInviteTokens({ [pending.id]: pending.token }); } } catch { sessionStorage.removeItem("pending-project-invitation"); } }
    void load();
  }, []);

  async function api<T>(path: string, init?: RequestInit): Promise<T> {
    const res = await fetch(path, { ...init, cache: "no-store", headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) } });
    const envelope = res.status === 204 ? { data: undefined } : await res.json() as Envelope<T>;
    if (!res.ok || envelope.error) throw new Error(envelope.error?.message ?? `Request failed (${res.status}).`);
    return envelope.data as T;
  }

  async function load(term = query) {
    setBusy(true); setProblem("");
    try {
      const session = await api<Session>("/api/v1/session"); setSignedIn(session.authenticated); setCampusAccess(Boolean(session.authenticated && session.user?.college_id)); setCsrf(session.csrf_token ?? "");
      const result = await api<{ items: Project[] }>(`/api/v1/projects${term.trim() ? `?q=${encodeURIComponent(term.trim())}` : ""}`);
      setItems(result.items);
      if (session.authenticated && session.user?.college_id) {
        const pending = await api<{ items: Invitation[] }>("/api/v1/my/project-invitations"); setInvitations(pending.items);
        const ownApplications = await api<{ items: MyApplication[] }>("/api/v1/my/applications"); setMyApplications(ownApplications.items);
      } else { setInvitations([]); setMyApplications([]); }
    } catch (error) { setProblem(error instanceof Error ? error.message : "Projects could not be loaded."); }
    finally { setBusy(false); }
  }

  async function mutate<T>(path: string, method: string, body: unknown) {
    return api<T>(path, { method, body: JSON.stringify(body), headers: { "X-CSRF-Token": csrf } });
  }

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const formElement = event.currentTarget; const form = new FormData(formElement);
    try {
      await mutate("/api/v1/projects", "POST", { slug: form.get("slug"), title: form.get("title"), summary: form.get("summary"), description: form.get("description"), media_ids: String(form.get("media_ids") ?? "").split(",").map((value) => value.trim()).filter(Boolean), project_type: form.get("project_type"), visibility: form.get("visibility"), lifecycle: "draft", skills: String(form.get("skills") ?? "").split(",").map((value) => value.trim()).filter(Boolean) });
      setMessage("Draft saved. Review it and publish from the project lifecycle controls."); formElement.reset(); await load();
    } catch (error) { setProblem(error instanceof Error ? error.message : "Project could not be created."); }
  }

  async function openProject(project: Project) {
    try { const result = await api<ProjectDetails>(`/api/v1/projects/${project.id}`); const roles = campusAccess ? await api<{ items: Role[] }>(`/api/v1/projects/${project.id}/roles`) : { items: [] }; setActive({ ...result, roles: roles.items }); setRoleID(roles.items.find((role) => role.status === "open")?.id ?? ""); setApplicationMessage(""); if (campusAccess && (result.viewer_role === "owner" || result.viewer_role === "maintainer")) { const pending = await api<{ items: Application[] }>(`/api/v1/projects/${project.id}/applications`); setApplications(pending.items); } else setApplications([]); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Project details could not be loaded."); }
  }

  async function createRole(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!active) return; const form = new FormData(event.currentTarget);
    try { await mutate(`/api/v1/projects/${active.project.id}/roles`, "POST", { title: form.get("title"), description: form.get("description"), openings: Number(form.get("openings")), difficulty: form.get("difficulty"), good_first_task: form.get("good_first_task") === "on", prerequisite_skills: String(form.get("prerequisite_skills") ?? "").split(",").map((value) => value.trim()).filter(Boolean) }); setMessage("Role published."); await openProject(active.project); await load(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Role could not be created."); }
  }

  async function decide(application: Application, decision: "accepted" | "rejected") {
    if (!active) return;
    try { await mutate(`/api/v1/projects/${active.project.id}/applications/${application.id}/decision`, "POST", { decision, version: application.version }); setMessage(`Application ${decision}.`); await openProject(active.project); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Application decision could not be saved."); }
  }

  async function searchPeople(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!active) return; const value = new FormData(event.currentTarget).get("person")?.toString().trim() ?? "";
    try { const result = await api<{ items: typeof people }>(`/api/v1/projects/${active.project.id}/people?q=${encodeURIComponent(value)}`); setPeople(result.items); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Campus users could not be found."); }
  }

  async function invite(userID: string, role: string) {
    if (!active) return;
    try { const result = await mutate<{ id: string; token: string }>(`/api/v1/projects/${active.project.id}/invitations`, "POST", { user_id: userID, role }); setInviteLink(`${window.location.origin}/#project_invitation=${encodeURIComponent(result.id)}&token=${encodeURIComponent(result.token)}`); setMessage("Invitation created. Share this one-time link with the invited campus member."); setPeople((items) => items.filter((person) => person.id !== userID)); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Invitation could not be created."); }
  }

  async function acceptInvitation(invitation: Invitation) {
    const token = inviteTokens[invitation.id]; if (!token) { setProblem("Open the one-time invitation link shared with you before accepting."); return; }
    try { await mutate(`/api/v1/project-invitations/${invitation.id}/accept`, "POST", { token }); setInviteTokens((items) => { const next = { ...items }; delete next[invitation.id]; return next; }); sessionStorage.removeItem("pending-project-invitation"); setMessage(`You joined ${invitation.project_title}.`); await load(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Invitation could not be accepted."); }
  }

  async function withdraw(application: MyApplication) {
    try { await mutate(`/api/v1/projects/${application.project_id}/applications/${application.id}/withdraw`, "POST", { version: application.version }); setMessage("Application withdrawn."); await load(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Application could not be withdrawn."); }
  }

  async function setLifecycle(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!active) return; const lifecycle = new FormData(event.currentTarget).get("lifecycle");
    try { await mutate(`/api/v1/projects/${active.project.id}`, "PATCH", { summary: active.project.summary, description: active.description, visibility: active.project.visibility, lifecycle, version: active.project.version }); setMessage("Project lifecycle updated."); await openProject(active.project); await load(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Project status could not be updated."); }
  }

  async function toggleRole(role: Role) {
    if (!active) return;
    try { await mutate(`/api/v1/projects/${active.project.id}/roles/${role.id}`, "PATCH", { openings: role.openings, status: role.status === "open" ? "closed" : "open", version: role.version }); setMessage("Role status updated."); await openProject(active.project); await load(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Role status could not be updated."); }
  }

  async function removeMember(memberID: string) {
    if (!active) return;
    try { await mutate(`/api/v1/projects/${active.project.id}/members/${memberID}`, "DELETE", undefined); setMessage("Team membership removed."); await openProject(active.project); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Member could not be removed."); }
  }

  async function changeMemberRole(memberID: string, role: string) {
    if (!active) return;
    try { await mutate(`/api/v1/projects/${active.project.id}/members/${memberID}/role`, "POST", { role }); setMessage("Team role updated."); await openProject(active.project); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Team role could not be updated."); }
  }

  async function apply(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!active || !roleID) return;
    try { await mutate(`/api/v1/projects/${active.project.id}/applications`, "POST", { role_id: roleID, message: applicationMessage }); setMessage("Your application was sent to the project team."); setApplicationMessage(""); await load(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Application could not be sent."); }
  }

  return <section className="content" id="projects" aria-labelledby="projects-title">
    <div className="section-heading">
      <div><p className="eyebrow">Find your next step</p><h2 id="projects-title">Explore projects</h2></div><span className="result-count">{items.length} projects</span></div>
    {problem && <p className="form-error" role="alert">{problem}</p>}{message && <p className="form-success" role="status">{message}</p>}
    {inviteLink && <p className="invite-link">Share this one-time invitation link with the selected campus member: <a href={inviteLink}>{inviteLink}</a></p>}
    <form className="search" role="search" onSubmit={(event) => { event.preventDefault(); void load(); }}><label htmlFor="project-search">Search projects</label><div className="search-row"><input id="project-search" type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Try design, robotics, or research" /><button type="submit">Search</button></div></form>
    {campusAccess && invitations.length > 0 && <section className="project-create" aria-labelledby="invitations-title"><h3 id="invitations-title">Project invitations</h3>{invitations.map((invitation) => <p key={invitation.id}><strong>{invitation.project_title}</strong> · {invitation.role} · <label>Invitation token<input type="password" value={inviteTokens[invitation.id] ?? ""} onChange={(event) => setInviteTokens((tokens) => ({ ...tokens, [invitation.id]: event.target.value }))} autoComplete="off" /></label> <button className="secondary-button" type="button" disabled={!inviteTokens[invitation.id]} onClick={() => void acceptInvitation(invitation)}>Accept invitation</button></p>)}</section>}
    {campusAccess && myApplications.length > 0 && <section className="project-create" aria-labelledby="my-applications-title"><h3 id="my-applications-title">Your applications</h3>{myApplications.map((application) => <p key={application.id}><strong>{application.project_title}</strong> · {application.role_title} · {application.status}{application.status === "pending" && <> · <button className="secondary-button" type="button" onClick={() => void withdraw(application)}>Withdraw</button></>}</p>)}</section>}
    {campusAccess && <form className="project-create" onSubmit={create}><h3>Start a project</h3><div className="project-fields"><label>Project name<input name="title" required minLength={3} maxLength={120} /></label><label>Project URL slug<input name="slug" required pattern="[a-z0-9]+(-[a-z0-9]+)*" placeholder="campus-robotics" /></label><label>Type<select name="project_type"><option value="side_project">Side project</option><option value="coursework">Coursework</option><option value="research">Research</option><option value="startup">Startup</option><option value="open_source">Open source</option><option value="hackathon">Hackathon</option></select></label><label>Visibility<select name="visibility"><option value="campus">Campus</option><option value="private">Private</option><option value="public">Public</option></select></label></div><label>Short summary<textarea name="summary" required maxLength={500} rows={2} /></label><label>Project description<textarea name="description" maxLength={12000} rows={4} /></label><label>Skills needed, separated by commas<input name="skills" placeholder="Go, product design, robotics" /></label><label>Clean image IDs from your uploads, comma separated<input name="media_ids" placeholder="Optional" /></label><button className="primary-button" type="submit">Create draft</button></form>}
    {busy ? <div className="empty-state" role="status"><h3>Loading projects…</h3></div> : items.length ? <div className="project-list">{items.map((project) => <article className="project-card" key={project.id}><div><p className="eyebrow">{project.project_type.replaceAll("_", " ")} · {project.visibility} · {project.recruiting ? "recruiting" : "not recruiting"}</p><h3>{project.title}</h3><p>{project.summary}</p><ul className="skill-list">{project.skills.map((skill) => <li key={skill}>{skill}</li>)}</ul></div><button className="secondary-button" type="button" onClick={() => void openProject(project)}>{campusAccess ? "View team and roles" : "Preview public project"}</button></article>)}</div> : <div className="empty-state" aria-live="polite"><span className="empty-icon" aria-hidden="true">✳</span><h3>{query.trim() ? `No public projects found for “${query.trim()}”` : campusAccess ? "Your next idea starts here" : "No public projects yet"}</h3><p>{query.trim() ? "Try another title or summary." : campusAccess ? "Campus projects will appear here once they have been published." : "Sign in and verify a campus email to explore campus projects and collaborate."}</p>{!campusAccess && <a href={signedIn ? "/onboarding" : "/login"}>{signedIn ? "Verify campus access" : "Sign in to continue"}</a>}</div>}
    {active && <div className="project-modal" role="dialog" aria-modal="true" aria-labelledby="project-detail-title"><article><button className="secondary-button close-project" type="button" onClick={() => setActive(null)}>Close</button><p className="eyebrow">{active.project.lifecycle.replaceAll("_", " ")} · {active.project.visibility}</p><h3 id="project-detail-title">{active.project.title}</h3><p>{active.project.summary}</p>{campusAccess && <RepositoryPanel projectID={active.project.id} csrf={csrf} />}{!campusAccess ? <><p>This public preview shows the project overview. Verify a campus email to see teams, roles, and collaboration details.</p><a className="primary-button" href={signedIn ? "/onboarding" : "/login"}>{signedIn ? "Verify campus access" : "Sign in to continue"}</a></> : <>{(active.viewer_role === "owner" || active.viewer_role === "maintainer") && <form className="account-form" onSubmit={setLifecycle}><h4>Project lifecycle</h4><label>Status<select name="lifecycle" defaultValue={active.project.lifecycle}><option value="draft">Draft</option><option value="active">Active</option><option value="on_hold">On hold</option><option value="completed">Completed</option><option value="archived">Archived</option></select></label><button className="secondary-button" type="submit">Save project status</button></form>}<h4>Team</h4><ul>{active.members.map((member) => <li key={member.user_id}><span>{member.display_name} · {member.role}</span>{(active.viewer_role === "owner" || active.viewer_role === "maintainer") && <span className="member-actions"><select aria-label={`Role for ${member.display_name}`} defaultValue={member.role} onChange={(event) => void changeMemberRole(member.user_id, event.target.value)}><option value="owner">Owner</option><option value="maintainer">Maintainer</option><option value="member">Member</option><option value="mentor">Mentor</option></select><button className="secondary-button" type="button" onClick={() => void removeMember(member.user_id)}>Remove</button></span>}</li>)}</ul><h4>Roles</h4>{active.roles.length ? active.roles.map((role) => <div className="role-card" key={role.id}><strong>{role.title}</strong><p>{role.description}</p><p>{role.openings} opening(s) · {role.difficulty}{role.good_first_task ? " · good first task" : ""}{role.prerequisite_skills.length ? ` · Needs ${role.prerequisite_skills.join(", ")}` : ""}</p>{(active.viewer_role === "owner" || active.viewer_role === "maintainer") && <button className="secondary-button" type="button" onClick={() => void toggleRole(role)}>{role.status === "open" ? "Close role" : "Reopen role"}</button>}</div>) : <p>This project is not recruiting right now.</p>}{signedIn && active.roles.some((role) => role.status === "open") && <form className="account-form" onSubmit={apply}><label>Apply for<select value={roleID} onChange={(event) => setRoleID(event.target.value)}>{active.roles.filter((role) => role.status === "open").map((role) => <option key={role.id} value={role.id}>{role.title}</option>)}</select></label><label>Message to the team<textarea required maxLength={2000} value={applicationMessage} onChange={(event) => setApplicationMessage(event.target.value)} /></label><button className="primary-button" type="submit">Send application</button></form>}{(active.viewer_role === "owner" || active.viewer_role === "maintainer") && <><form className="account-form" onSubmit={createRole}><h4>Create a recruiting role</h4><label>Role title<input name="title" required maxLength={100} /></label><label>What the member will do<textarea name="description" required maxLength={3000} /></label><label>Openings<input name="openings" type="number" min={1} max={20} defaultValue={1} /></label><label>Difficulty<select name="difficulty" defaultValue="beginner"><option value="novice">Novice</option><option value="beginner">Beginner</option><option value="intermediate">Intermediate</option><option value="advanced">Advanced</option></select></label><label className="check-row"><input type="checkbox" name="good_first_task" />Beginner-friendly first task</label><label>Prerequisite skills, comma separated<input name="prerequisite_skills" /></label><button className="primary-button" type="submit">Publish role</button></form><form className="account-form" onSubmit={searchPeople}><h4>Invite a campus member</h4><label>Name or handle<input name="person" required minLength={2} /></label><button className="secondary-button" type="submit">Find members</button></form>{people.map((person) => <p key={person.id}>{person.display_name} (@{person.handle}) <button className="secondary-button" type="button" onClick={() => void invite(person.id, "member")}>Invite as member</button></p>)}{applications.filter((application) => application.status === "pending").length > 0 && <><h4>Applications</h4>{applications.filter((application) => application.status === "pending").map((application) => <div className="role-card" key={application.id}><strong>{application.applicant_name}</strong><p>{application.message}</p><button className="primary-button" type="button" onClick={() => void decide(application, "accepted")}>Accept</button> <button className="secondary-button" type="button" onClick={() => void decide(application, "rejected")}>Reject</button></div>)}</>}</>}</>}</article>
    </div>}
  </section>;
}
