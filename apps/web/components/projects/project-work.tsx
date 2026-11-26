"use client";

import { FormEvent, KeyboardEvent, useCallback, useEffect, useMemo, useState } from "react";
import styles from "./project-work.module.css";

type Envelope<T> = { data?: T; error?: { message?: string } };
type ItemList<T> = { items: T[] };
type Project = { id: string; title: string; summary: string; visibility: string; lifecycle: string };
type Update = { id: string; author_id: string; author_name: string; body: string; audience: string; version: number; reaction_count: number; comment_count: number; viewer_reaction?: string; created_at: string };
type Discussion = { id: string; author_id: string; author_name: string; title: string; body: string; status: string; version: number; reply_count: number; created_at: string };
type Task = { id: string; title: string; description: string; status: Status; priority: string; milestone_id: string | null; milestone_title: string | null; assignee_ids: string[]; assignee_names: string[]; version: number; due_at: string | null; can_edit: boolean };
type Milestone = { id: string; title: string; description: string; status: string; version: number; task_count: number; completed_count: number; due_at: string | null };
type Comment = { id: string; parent_id: string | null; author_name: string; body: string; version: number; created_at: string };
type Status = "todo" | "in_progress" | "blocked" | "done";
type Tab = "updates" | "tasks" | "milestones" | "discussions";
const statuses: Status[] = ["todo", "in_progress", "blocked", "done"];
const statusNames: Record<Status, string> = { todo: "To do", in_progress: "In progress", blocked: "Blocked", done: "Done" };

function date(value?: string | null) {
  if (!value) return "No due date";
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? "No due date" : parsed.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}

export function ProjectWork({ projectID }: { projectID: string }) {
  const [csrf, setCSRF] = useState("");
  const [project, setProject] = useState<Project | null>(null);
  const [role, setRole] = useState("");
  const [viewerID, setViewerID] = useState("");
  const [updates, setUpdates] = useState<Update[]>([]);
  const [discussions, setDiscussions] = useState<Discussion[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [milestones, setMilestones] = useState<Milestone[]>([]);
  const [tab, setTab] = useState<Tab>("updates");
  const [view, setView] = useState<"board" | "list">("board");
  const [following, setFollowing] = useState(false);
  const [bookmarked, setBookmarked] = useState(false);
  const [loading, setLoading] = useState(true);
  const [problem, setProblem] = useState("");
  const [notice, setNotice] = useState("");
  const [comments, setComments] = useState<Record<string, Comment[]>>({});
  const [commentDrafts, setCommentDrafts] = useState<Record<string, string>>({});
  const [showNewTask, setShowNewTask] = useState(false);
  const [showNewUpdate, setShowNewUpdate] = useState(false);
  const [showNewDiscussion, setShowNewDiscussion] = useState(false);
  const [showNewMilestone, setShowNewMilestone] = useState(false);

  const get = useCallback(async <T,>(url: string): Promise<T> => {
    const response = await fetch(url, { cache: "no-store" });
    const envelope = (await response.json()) as Envelope<T>;
    if (!response.ok || !envelope.data) throw new Error(envelope.error?.message ?? "Could not load project collaboration.");
    return envelope.data;
  }, []);

  const mutate = useCallback(async <T,>(url: string, method: string, body?: unknown): Promise<T | undefined> => {
    const response = await fetch(url, {
      method,
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (response.status === 204) return undefined;
    const envelope = (await response.json()) as Envelope<T>;
    if (!response.ok) throw new Error(envelope.error?.message ?? (response.status === 409 ? "This item changed. Reload it before trying again." : "The change could not be saved."));
    return envelope.data;
  }, [csrf]);

  const refresh = useCallback(async () => {
    setLoading(true);
    setProblem("");
    try {
      const [projectResult, session, updateResult, taskResult, milestoneResult, discussionResult, saved] = await Promise.all([
        get<{ project: Project; viewer_role: string }>(`/api/v1/projects/${projectID}`),
        get<{ csrf_token?: string; user?: { id?: string } }>("/api/v1/session"),
        get<ItemList<Update>>(`/api/v1/projects/${projectID}/updates`),
        get<ItemList<Task>>(`/api/v1/projects/${projectID}/tasks`),
        get<ItemList<Milestone>>(`/api/v1/projects/${projectID}/milestones`),
        get<ItemList<Discussion>>(`/api/v1/projects/${projectID}/discussions`),
        get<ItemList<{ id: string; following: boolean; bookmarked: boolean }>>("/api/v1/me/saved-projects"),
      ]);
      setProject(projectResult.project);
      setRole(projectResult.viewer_role);
      setCSRF(session.csrf_token ?? "");
      setViewerID(session.user?.id ?? "");
      setUpdates(updateResult.items ?? []);
      setTasks(taskResult.items ?? []);
      setMilestones(milestoneResult.items ?? []);
      setDiscussions(discussionResult.items ?? []);
      const current = saved.items?.find((item) => item.id === projectID);
      setFollowing(current?.following ?? false);
      setBookmarked(current?.bookmarked ?? false);
    } catch (error) {
      setProject(null);
      setProblem(error instanceof Error ? error.message : "Project work could not be loaded.");
    } finally {
      setLoading(false);
    }
  }, [get, projectID]);

  useEffect(() => { void refresh(); }, [refresh]);

  useEffect(() => {
    const revalidate = () => { setProject(null); setUpdates([]); setTasks([]); setMilestones([]); setDiscussions([]); setComments({}); void refresh(); };
    const onPageShow = (event: PageTransitionEvent) => { if (event.persisted) revalidate(); };
    const onVisibility = () => { if (document.visibilityState === "visible") revalidate(); };
    window.addEventListener("pageshow", onPageShow);
    document.addEventListener("visibilitychange", onVisibility);
    return () => { window.removeEventListener("pageshow", onPageShow); document.removeEventListener("visibilitychange", onVisibility); };
  }, [refresh]);

  const manager = role === "owner" || role === "maintainer";
  const base = `/api/v1/projects/${projectID}`;
  const taskGroups = useMemo(() => Object.fromEntries(statuses.map((status) => [status, tasks.filter((task) => task.status === status)])) as Record<Status, Task[]>, [tasks]);

  async function submit(event: FormEvent<HTMLFormElement>, kind: "update" | "task" | "milestone" | "discussion") {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const body = kind === "update"
      ? { body: String(data.get("body") ?? ""), audience: String(data.get("audience") ?? "members") }
      : kind === "discussion"
        ? { title: String(data.get("title") ?? ""), body: String(data.get("body") ?? "") }
        : kind === "milestone"
          ? { title: String(data.get("title") ?? ""), description: String(data.get("description") ?? ""), due_at: data.get("due_at") ? new Date(String(data.get("due_at"))).toISOString() : null }
          : { title: String(data.get("title") ?? ""), description: String(data.get("description") ?? ""), status: "todo", priority: String(data.get("priority") ?? "normal"), milestone_id: String(data.get("milestone_id") ?? ""), assignee_ids: data.get("assignee_id") ? [String(data.get("assignee_id"))] : [], due_at: data.get("due_at") ? new Date(String(data.get("due_at"))).toISOString() : null };
    try {
      await mutate(`${base}/${kind === "update" ? "updates" : `${kind}s`}`, "POST", body);
      form.reset();
      setShowNewTask(false); setShowNewUpdate(false); setShowNewDiscussion(false); setShowNewMilestone(false);
      setNotice(`${kind[0].toUpperCase()}${kind.slice(1)} created.`);
      await refresh();
    } catch (error) { setProblem(error instanceof Error ? error.message : "The item could not be created."); }
  }

  async function moveTask(task: Task, status: Status) {
    if (task.status === status) return;
    try {
      await mutate(`${base}/tasks/${task.id}`, "PATCH", { title: task.title, description: task.description, status, priority: task.priority, milestone_id: task.milestone_id ?? "", due_at: task.due_at, version: task.version });
      setNotice(`Moved “${task.title}” to ${statusNames[status]}.`);
      await refresh();
    } catch (error) { setProblem(error instanceof Error ? error.message : "Task status could not be changed."); }
  }

  async function editTask(task: Task) {
    const title = window.prompt("Task title", task.title);
    if (title === null) return;
    const description = window.prompt("Task description", task.description);
    if (description === null) return;
    try {
      await mutate(`${base}/tasks/${task.id}`, "PATCH", { title, description, status: task.status, priority: task.priority, milestone_id: task.milestone_id ?? "", due_at: task.due_at, version: task.version });
      setNotice("Task updated."); await refresh();
    } catch (error) { setProblem(error instanceof Error ? error.message : "Task could not be updated."); }
  }

  async function editUpdate(item: Update) {
    const body = window.prompt("Edit progress update", item.body);
    if (body === null) return;
    try {
      await mutate(`${base}/updates/${item.id}`, "PATCH", { body, audience: item.audience, version: item.version });
      setNotice("Update saved."); await refresh();
    } catch (error) { setProblem(error instanceof Error ? error.message : "Update could not be saved."); }
  }

  async function changeMilestone(milestone: Milestone, status: string) {
    try {
      await mutate(`${base}/milestones/${milestone.id}`, "PATCH", { title: milestone.title, description: milestone.description, status, due_at: milestone.due_at, version: milestone.version });
      setNotice("Milestone updated."); await refresh();
    } catch (error) { setProblem(error instanceof Error ? error.message : "Milestone could not be updated."); }
  }

  async function keyboardMove(event: KeyboardEvent<HTMLElement>, task: Task) {
    if (!task.can_edit) return;
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    if (event.target instanceof HTMLElement && ["BUTTON", "SELECT", "INPUT", "TEXTAREA"].includes(event.target.tagName)) return;
    event.preventDefault();
    const current = statuses.indexOf(task.status);
    const next = Math.max(0, Math.min(statuses.length - 1, current + (event.key === "ArrowRight" ? 1 : -1)));
    await moveTask(task, statuses[next]);
  }

  async function toggleSignal(kind: "follow" | "bookmark") {
    try {
      await mutate(`${base}/${kind}`, (kind === "follow" ? following : bookmarked) ? "DELETE" : "PUT", {});
      if (kind === "follow") setFollowing(!following); else setBookmarked(!bookmarked);
      setNotice(kind === "follow" ? (following ? "Project unfollowed." : "Following project updates.") : (bookmarked ? "Removed from saved projects." : "Project saved."));
    } catch (error) { setProblem(error instanceof Error ? error.message : "Preference could not be saved."); }
  }

  async function loadComments(type: "update" | "discussion" | "task", id: string) {
    const key = `${type}:${id}`;
    if (comments[key]) return;
    try {
      const result = await get<ItemList<Comment>>(`${base}/comments?target_type=${type}&target_id=${id}`);
      setComments((current) => ({ ...current, [key]: result.items ?? [] }));
    } catch (error) { setProblem(error instanceof Error ? error.message : "Comments could not be loaded."); }
  }

  async function addComment(event: FormEvent<HTMLFormElement>, type: "update" | "discussion" | "task", id: string) {
    event.preventDefault();
    const key = `${type}:${id}`;
    const body = commentDrafts[key]?.trim();
    if (!body) return;
    try {
      await mutate(`${base}/comments`, "POST", { target_type: type, target_id: id, body });
      setCommentDrafts((current) => ({ ...current, [key]: "" }));
      const result = await get<ItemList<Comment>>(`${base}/comments?target_type=${type}&target_id=${id}`);
      setComments((current) => ({ ...current, [key]: result.items ?? [] }));
      await refresh();
    } catch (error) { setProblem(error instanceof Error ? error.message : "Comment could not be posted."); }
  }

  async function react(id: string, value?: string) {
    try {
      if (value) await mutate(`${base}/reactions`, "PUT", { target_type: "update", target_id: id, reaction: value });
      else await mutate(`${base}/reactions`, "DELETE", { target_type: "update", target_id: id });
      await refresh();
    } catch (error) { setProblem(error instanceof Error ? error.message : "Reaction could not be saved."); }
  }

  async function report(type: "update" | "discussion" | "task", id: string) {
    const reason = window.prompt("Why are you reporting this item? Enter spam, harassment, unsafe, or other.");
    if (!reason || !["spam", "harassment", "unsafe", "other"].includes(reason)) return;
    try { await mutate(`${base}/reports`, "POST", { target_type: type, target_id: id, reason, details: "" }); setNotice("Report sent to the project moderation queue."); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Report could not be sent."); }
  }

  if (loading) return <section className={styles.status} aria-live="polite">Loading project work…</section>;
  if (!project) return <section className={styles.status} role="alert"><p>{problem || "Project work is unavailable."}</p><button className="secondary-button" onClick={() => void refresh()}>Retry</button></section>;

  const commentThread = (type: "update" | "discussion" | "task", id: string, locked = false) => {
    const key = `${type}:${id}`;
    return <details className={styles.comments} onToggle={(event) => { if (event.currentTarget.open) void loadComments(type, id); }}>
      <summary>Comments</summary>
      <div className={styles.commentList}>
        {(comments[key] ?? []).map((comment) => <div className={styles.comment} key={comment.id}><strong>{comment.author_name}</strong><p>{comment.body}</p></div>)}
        {(comments[key] ?? []).length === 0 && <p className="muted">No comments yet. Start the conversation.</p>}
        {role && !locked && <form className={styles.commentForm} onSubmit={(event) => void addComment(event, type, id)}>
          <label className="sr-only" htmlFor={`comment-${id}`}>Write a comment</label>
          <input id={`comment-${id}`} value={commentDrafts[key] ?? ""} onChange={(event) => setCommentDrafts((current) => ({ ...current, [key]: event.target.value }))} maxLength={2000} placeholder="Add a comment…" />
          <button className="secondary-button" type="submit" disabled={!commentDrafts[key]?.trim()}>Comment</button>
        </form>}
      </div>
    </details>;
  };

  return <div className={styles.workspace}>
    <nav className={styles.crumbs} aria-label="Breadcrumb"><a href="/projects">Projects</a><span>/</span><a href={`/projects/${projectID}`}>{project.title}</a><span>/</span><span>Team work</span></nav>
    <header className={styles.header}>
      <div><p className="eyebrow">Project workspace</p><h1 className="t-headline-md">Team work</h1><p className="muted">Updates, planning, and conversations for <strong>{project.title}</strong>.</p></div>
      <div className={styles.signals}><button className="secondary-button" onClick={() => void toggleSignal("follow")} aria-pressed={following}>{following ? "Following" : "Follow updates"}</button><button className="secondary-button" onClick={() => void toggleSignal("bookmark")} aria-pressed={bookmarked}>{bookmarked ? "Saved" : "Save project"}</button></div>
    </header>
    {(problem || notice) && <div className={problem ? styles.error : styles.notice} role={problem ? "alert" : "status"}><span>{problem || notice}</span><button type="button" onClick={() => { setProblem(""); setNotice(""); }} aria-label="Dismiss message">×</button>{problem && <button className="secondary-button" onClick={() => void refresh()}>Reload</button>}</div>}
    <div className={styles.tabs} role="tablist" aria-label="Project work sections">
      {(["updates", "tasks", "milestones", "discussions"] as Tab[]).map((item) => <button key={item} role="tab" aria-selected={tab === item} className={tab === item ? styles.activeTab : ""} onClick={() => setTab(item)}>{item[0].toUpperCase() + item.slice(1)}{item === "tasks" ? ` · ${tasks.length}` : ""}</button>)}
    </div>

    {tab === "updates" && <section className={styles.section} aria-labelledby="updates-title">
      <div className={styles.sectionTitle}><div><h2 id="updates-title">Project updates</h2><p className="muted">Dated progress notes from the people building this project.</p></div>{role && <button className="primary-button" onClick={() => setShowNewUpdate(!showNewUpdate)}>{showNewUpdate ? "Cancel" : "Post update"}</button>}</div>
      {showNewUpdate && <form className={styles.editor} onSubmit={(event) => void submit(event, "update")}><label>Update<textarea name="body" required maxLength={5000} rows={4} placeholder="Share progress, blockers, or what comes next…" /></label><label>Audience<select name="audience"><option value="members">Project members</option>{project.visibility !== "private" && <option value="campus">Campus</option>}</select></label><button className="primary-button">Publish update</button></form>}
      {updates.length === 0 ? <div className={styles.empty}><span className={styles.emptyMark}>↗</span><h3>No updates yet</h3><p>Share the first milestone of progress with your teammates.</p></div> : <div className={styles.feed}>{updates.map((update) => <article className={styles.card} key={update.id}><div className={styles.cardMeta}><div className={styles.avatar}>{update.author_name.slice(0, 1).toUpperCase()}</div><div><strong>{update.author_name}</strong><time>{date(update.created_at)}</time></div><span className={styles.audience}>{update.audience === "campus" ? "Campus" : "Members"}</span></div><p className={styles.prose}>{update.body}</p><div className={styles.actions}>{role && <button className="secondary-button" aria-pressed={Boolean(update.viewer_reaction)} onClick={() => void react(update.id, update.viewer_reaction ? undefined : "support")}>{update.viewer_reaction ? "♥ Supported" : "♡ Support"} · {update.reaction_count}</button>}<button className="secondary-button" onClick={(event) => { const details = event.currentTarget.closest("article")?.querySelector("details"); if (details) details.open = !details.open; }}>Comments · {update.comment_count}</button>{(update.author_id === viewerID || manager) && <button className={styles.quietButton} onClick={() => void editUpdate(update)}>Edit</button>}<button className={styles.quietButton} onClick={() => void report("update", update.id)}>Report</button></div>{commentThread("update", update.id)}</article>)}</div>}
    </section>}

    {tab === "tasks" && <section className={styles.section} aria-labelledby="tasks-title">
      <div className={styles.sectionTitle}><div><h2 id="tasks-title">Tasks</h2><p className="muted">Keep work moving with a board or a keyboard-friendly list.</p></div><div className={styles.controls}><div className={styles.switch} aria-label="Task view"><button aria-pressed={view === "board"} onClick={() => setView("board")}>Board</button><button aria-pressed={view === "list"} onClick={() => setView("list")}>List</button></div>{role && <button className="primary-button" onClick={() => setShowNewTask(!showNewTask)}>{showNewTask ? "Cancel" : "New task"}</button>}</div></div>
      {showNewTask && <form className={styles.editor} onSubmit={(event) => void submit(event, "task")}><label>Task title<input name="title" required minLength={3} maxLength={180} /></label><label>Description<textarea name="description" maxLength={5000} rows={3} /></label><div className={styles.formGrid}><label>Priority<select name="priority"><option value="normal">Normal</option><option value="low">Low</option><option value="high">High</option></select></label><label>Milestone<select name="milestone_id"><option value="">No milestone</option>{milestones.filter((m) => m.status === "open").map((m) => <option value={m.id} key={m.id}>{m.title}</option>)}</select></label><label>Due date<input type="date" name="due_at" /></label><label>Assign to (your account)<input name="assignee_id" value={viewerID} readOnly placeholder="Sign in to assign" /></label></div><button className="primary-button">Create task</button></form>}
      {tasks.length === 0 ? <div className={styles.empty}><span className={styles.emptyMark}>✓</span><h3>Room for the first task</h3><p>Break your next project goal into a small, assignable step.</p></div> : view === "board" ? <div className={styles.board} aria-label="Task board">{statuses.map((status) => <div className={styles.column} key={status}><h3>{statusNames[status]} <span>{taskGroups[status].length}</span></h3>{taskGroups[status].map((task) => <article className={styles.task} key={task.id} tabIndex={0} onKeyDown={(event) => void keyboardMove(event, task)} aria-label={`${task.title}, ${statusNames[task.status]}. Use left and right arrow keys to change status.`}><strong>{task.title}</strong>{task.description && <p>{task.description}</p>}<small>{task.milestone_title ?? "No milestone"} · {date(task.due_at)}</small><small>{task.assignee_names.join(", ") || "Unassigned"}</small><label>Status<select aria-label={`Status for ${task.title}`} value={task.status} disabled={!task.can_edit} onChange={(event) => void moveTask(task, event.target.value as Status)}><option value="todo">To do</option><option value="in_progress">In progress</option><option value="blocked">Blocked</option><option value="done">Done</option></select></label><div className={styles.taskTools}>{task.can_edit && <button type="button" className={styles.quietButton} onClick={() => void editTask(task)}>Edit task</button>}{commentThread("task", task.id)}<button className={styles.quietButton} onClick={() => void report("task", task.id)}>Report</button></div></article>)}</div>)}</div> : <div className={styles.tableWrap}><table className={styles.taskTable}><thead><tr><th>Task</th><th>Status</th><th>Assignee</th><th>Milestone</th><th>Due</th></tr></thead><tbody>{tasks.map((task) => <tr key={task.id}><td><strong>{task.title}</strong><small>{task.description}</small></td><td><select aria-label={`Status for ${task.title}`} value={task.status} disabled={!task.can_edit} onChange={(event) => void moveTask(task, event.target.value as Status)}>{statuses.map((status) => <option key={status} value={status}>{statusNames[status]}</option>)}</select></td><td>{task.assignee_names.join(", ") || "Unassigned"}</td><td>{task.milestone_title ?? "—"}</td><td>{date(task.due_at)}</td></tr>)}</tbody></table></div>}
      <p className={styles.keyboardHint}>Keyboard: focus a task card and use ← / → to move it between adjacent columns. The list view and status selector are alternatives.</p>
    </section>}

    {tab === "milestones" && <section className={styles.section} aria-labelledby="milestones-title"><div className={styles.sectionTitle}><div><h2 id="milestones-title">Milestones</h2><p className="muted">A shared view of the outcomes the team is working toward.</p></div>{manager && <button className="primary-button" onClick={() => setShowNewMilestone(!showNewMilestone)}>{showNewMilestone ? "Cancel" : "New milestone"}</button>}</div>{showNewMilestone && <form className={styles.editor} onSubmit={(event) => void submit(event, "milestone")}><label>Milestone title<input name="title" required minLength={3} maxLength={160} /></label><label>Description<textarea name="description" rows={3} maxLength={3000} /></label><label>Target date<input name="due_at" type="date" /></label><button className="primary-button">Create milestone</button></form>}{!manager && <p className={styles.note}>Only project owners and maintainers can create or update milestones.</p>}{milestones.length === 0 ? <div className={styles.empty}><span className={styles.emptyMark}>◎</span><h3>No milestones yet</h3><p>Milestones make larger goals visible and give tasks a shared destination.</p></div> : <div className={styles.milestones}>{milestones.map((milestone) => { const progress = milestone.task_count ? Math.round(100 * milestone.completed_count / milestone.task_count) : 0; return <article className={styles.card} key={milestone.id}><div className={styles.milestoneHead}><div><span className={styles.audience}>{milestone.status}</span><h3>{milestone.title}</h3></div><span className={styles.due}>{date(milestone.due_at)}</span></div>{milestone.description && <p className={styles.prose}>{milestone.description}</p>}<div className={styles.progressLabel}><span>{milestone.completed_count} of {milestone.task_count} tasks complete</span><strong>{progress}%</strong></div><div className={styles.progress}><span style={{ width: `${progress}%` }} /></div>{manager && <label className={styles.milestoneStatus}>Milestone status<select value={milestone.status} onChange={(event) => void changeMilestone(milestone, event.target.value)}><option value="open">Open</option><option value="completed">Completed</option><option value="cancelled">Cancelled</option></select></label>}<a href="#tasks-title" onClick={() => setTab("tasks")}>View milestone tasks →</a></article>; })}</div>}</section>}

    {tab === "discussions" && <section className={styles.section} aria-labelledby="discussions-title"><div className={styles.sectionTitle}><div><h2 id="discussions-title">Discussions</h2><p className="muted">Open questions and decisions stay with the project.</p></div>{role && <button className="primary-button" onClick={() => setShowNewDiscussion(!showNewDiscussion)}>{showNewDiscussion ? "Cancel" : "Start discussion"}</button>}</div>{showNewDiscussion && <form className={styles.editor} onSubmit={(event) => void submit(event, "discussion")}><label>Topic<input name="title" required minLength={3} maxLength={160} placeholder="What should the team discuss?" /></label><label>First post<textarea name="body" required rows={4} maxLength={5000} /></label><button className="primary-button">Post topic</button></form>}{discussions.length === 0 ? <div className={styles.empty}><span className={styles.emptyMark}>◌</span><h3>No discussions yet</h3><p>Start a focused thread for a design question, decision, or idea.</p></div> : <div className={styles.feed}>{discussions.map((discussion) => <article className={styles.card} key={discussion.id}><div className={styles.cardMeta}><div className={styles.avatar}>{discussion.author_name.slice(0, 1).toUpperCase()}</div><div><strong>{discussion.author_name}</strong><time>{date(discussion.created_at)}</time></div><span className={styles.audience}>{discussion.status}</span></div><h3 className={styles.topic}>{discussion.title}</h3><p className={styles.prose}>{discussion.body}</p><div className={styles.actions}><span className="muted">{discussion.reply_count} replies</span><button className={styles.quietButton} onClick={() => void report("discussion", discussion.id)}>Report</button></div>{commentThread("discussion", discussion.id, discussion.status === "locked")}</article>)}</div>}</section>}
  </div>;
}
