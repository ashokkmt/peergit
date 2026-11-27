"use client";

import { FormEvent, KeyboardEvent, useCallback, useEffect, useMemo, useState } from "react";
import { fetchSession, readApiEnvelope } from "@/lib/http";

type ItemList<T> = { items: T[] };
type Project = {
  id: string;
  title: string;
  summary: string;
  visibility: string;
  lifecycle: string;
};
type Update = {
  id: string;
  author_id: string;
  author_name: string;
  body: string;
  audience: string;
  version: number;
  reaction_count: number;
  comment_count: number;
  viewer_reaction?: string;
  created_at: string;
};
type Discussion = {
  id: string;
  author_id: string;
  author_name: string;
  title: string;
  body: string;
  status: string;
  version: number;
  reply_count: number;
  created_at: string;
};
type Task = {
  id: string;
  title: string;
  description: string;
  status: Status;
  priority: string;
  milestone_id: string | null;
  milestone_title: string | null;
  assignee_ids: string[];
  assignee_names: string[];
  version: number;
  due_at: string | null;
  can_edit: boolean;
};
type Milestone = {
  id: string;
  title: string;
  description: string;
  status: string;
  version: number;
  task_count: number;
  completed_count: number;
  due_at: string | null;
};
type Comment = {
  id: string;
  parent_id: string | null;
  author_name: string;
  body: string;
  version: number;
  created_at: string;
};
type Status = "todo" | "in_progress" | "blocked" | "done";
type Tab = "updates" | "tasks" | "milestones" | "discussions";

const statuses: Status[] = ["todo", "in_progress", "blocked", "done"];
const statusNames: Record<Status, string> = {
  todo: "To do",
  in_progress: "In progress",
  blocked: "Blocked",
  done: "Done",
};

const cardClass =
  "min-w-0 p-[22px] border border-hairline rounded-2xl bg-surface shadow-[var(--shadow)]";
const editorClass =
  "grid gap-4 mb-5 p-[22px] border border-hairline rounded-2xl bg-surface shadow-[var(--shadow)]";
const fieldClass = "grid gap-[7px] text-on-surface font-semibold";
const fieldControlClass =
  "min-h-[42px] px-[11px] py-[9px] border border-control rounded-lg bg-canvas text-on-surface";
const primaryBtn =
  "inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium border border-primary bg-primary text-on-primary cursor-pointer hover:opacity-90";
const secondaryBtn =
  "inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised";
const quietBtn =
  "border-0 p-2 bg-transparent text-muted cursor-pointer underline underline-offset-[3px]";

function formatDate(value?: string | null) {
  if (!value) return "No due date";
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf())
    ? "No due date"
    : parsed.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
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
    const response = url === "/api/v1/session"
      ? await fetchSession()
      : await fetch(url, { cache: "no-store" });
    const envelope = await readApiEnvelope<T>(response, "Could not load project collaboration.");
    if (!response.ok || !envelope.data) {
      throw new Error(envelope.error?.message ?? "Could not load project collaboration.");
    }
    return envelope.data;
  }, []);

  const mutate = useCallback(
    async <T,>(url: string, method: string, body?: unknown): Promise<T | undefined> => {
      const response = await fetch(url, {
        method,
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
      if (response.status === 204) return undefined;
      const envelope = await readApiEnvelope<T>(
        response,
        response.status === 409
          ? "This item changed. Reload it before trying again."
          : "The change could not be saved.",
      );
      if (!response.ok) {
        throw new Error(
          envelope.error?.message ??
            (response.status === 409
              ? "This item changed. Reload it before trying again."
              : "The change could not be saved."),
        );
      }
      return envelope.data;
    },
    [csrf],
  );

  const refresh = useCallback(async () => {
    setLoading(true);
    setProblem("");
    try {
      const [
        projectResult,
        session,
        updateResult,
        taskResult,
        milestoneResult,
        discussionResult,
        saved,
      ] = await Promise.all([
        get<{ project: Project; viewer_role: string }>(`/api/v1/projects/${projectID}`),
        get<{ csrf_token?: string; user?: { id?: string } }>("/api/v1/session"),
        get<ItemList<Update>>(`/api/v1/projects/${projectID}/updates`),
        get<ItemList<Task>>(`/api/v1/projects/${projectID}/tasks`),
        get<ItemList<Milestone>>(`/api/v1/projects/${projectID}/milestones`),
        get<ItemList<Discussion>>(`/api/v1/projects/${projectID}/discussions`),
        get<ItemList<{ id: string; following: boolean; bookmarked: boolean }>>(
          "/api/v1/me/saved-projects",
        ),
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

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    const revalidate = () => {
      setProject(null);
      setUpdates([]);
      setTasks([]);
      setMilestones([]);
      setDiscussions([]);
      setComments({});
      void refresh();
    };
    const onPageShow = (event: PageTransitionEvent) => {
      if (event.persisted) revalidate();
    };
    const onVisibility = () => {
      if (document.visibilityState === "visible") revalidate();
    };
    window.addEventListener("pageshow", onPageShow);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.removeEventListener("pageshow", onPageShow);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [refresh]);

  const manager = role === "owner" || role === "maintainer";
  const base = `/api/v1/projects/${projectID}`;
  const taskGroups = useMemo(
    () =>
      Object.fromEntries(
        statuses.map((status) => [status, tasks.filter((task) => task.status === status)]),
      ) as Record<Status, Task[]>,
    [tasks],
  );

  async function submit(
    event: FormEvent<HTMLFormElement>,
    kind: "update" | "task" | "milestone" | "discussion",
  ) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const body =
      kind === "update"
        ? { body: String(data.get("body") ?? ""), audience: String(data.get("audience") ?? "members") }
        : kind === "discussion"
          ? { title: String(data.get("title") ?? ""), body: String(data.get("body") ?? "") }
          : kind === "milestone"
            ? {
                title: String(data.get("title") ?? ""),
                description: String(data.get("description") ?? ""),
                due_at: data.get("due_at") ? new Date(String(data.get("due_at"))).toISOString() : null,
              }
            : {
                title: String(data.get("title") ?? ""),
                description: String(data.get("description") ?? ""),
                status: "todo",
                priority: String(data.get("priority") ?? "normal"),
                milestone_id: String(data.get("milestone_id") ?? ""),
                assignee_ids: data.get("assignee_id") ? [String(data.get("assignee_id"))] : [],
                due_at: data.get("due_at") ? new Date(String(data.get("due_at"))).toISOString() : null,
              };
    try {
      await mutate(`${base}/${kind === "update" ? "updates" : `${kind}s`}`, "POST", body);
      form.reset();
      setShowNewTask(false);
      setShowNewUpdate(false);
      setShowNewDiscussion(false);
      setShowNewMilestone(false);
      setNotice(`${kind[0].toUpperCase()}${kind.slice(1)} created.`);
      await refresh();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "The item could not be created.");
    }
  }

  async function moveTask(task: Task, status: Status) {
    if (task.status === status) return;
    try {
      await mutate(`${base}/tasks/${task.id}`, "PATCH", {
        title: task.title,
        description: task.description,
        status,
        priority: task.priority,
        milestone_id: task.milestone_id ?? "",
        due_at: task.due_at,
        version: task.version,
      });
      setNotice(`Moved “${task.title}” to ${statusNames[status]}.`);
      await refresh();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Task status could not be changed.");
    }
  }

  async function editTask(task: Task) {
    const title = window.prompt("Task title", task.title);
    if (title === null) return;
    const description = window.prompt("Task description", task.description);
    if (description === null) return;
    try {
      await mutate(`${base}/tasks/${task.id}`, "PATCH", {
        title,
        description,
        status: task.status,
        priority: task.priority,
        milestone_id: task.milestone_id ?? "",
        due_at: task.due_at,
        version: task.version,
      });
      setNotice("Task updated.");
      await refresh();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Task could not be updated.");
    }
  }

  async function editUpdate(item: Update) {
    const body = window.prompt("Edit progress update", item.body);
    if (body === null) return;
    try {
      await mutate(`${base}/updates/${item.id}`, "PATCH", {
        body,
        audience: item.audience,
        version: item.version,
      });
      setNotice("Update saved.");
      await refresh();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Update could not be saved.");
    }
  }

  async function changeMilestone(milestone: Milestone, status: string) {
    try {
      await mutate(`${base}/milestones/${milestone.id}`, "PATCH", {
        title: milestone.title,
        description: milestone.description,
        status,
        due_at: milestone.due_at,
        version: milestone.version,
      });
      setNotice("Milestone updated.");
      await refresh();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Milestone could not be updated.");
    }
  }

  async function keyboardMove(event: KeyboardEvent<HTMLElement>, task: Task) {
    if (!task.can_edit) return;
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    if (
      event.target instanceof HTMLElement &&
      ["BUTTON", "SELECT", "INPUT", "TEXTAREA"].includes(event.target.tagName)
    ) {
      return;
    }
    event.preventDefault();
    const current = statuses.indexOf(task.status);
    const next = Math.max(
      0,
      Math.min(statuses.length - 1, current + (event.key === "ArrowRight" ? 1 : -1)),
    );
    await moveTask(task, statuses[next]);
  }

  async function toggleSignal(kind: "follow" | "bookmark") {
    try {
      await mutate(
        `${base}/${kind}`,
        (kind === "follow" ? following : bookmarked) ? "DELETE" : "PUT",
        {},
      );
      if (kind === "follow") setFollowing(!following);
      else setBookmarked(!bookmarked);
      setNotice(
        kind === "follow"
          ? following
            ? "Project unfollowed."
            : "Following project updates."
          : bookmarked
            ? "Removed from saved projects."
            : "Project saved.",
      );
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Preference could not be saved.");
    }
  }

  async function loadComments(type: "update" | "discussion" | "task", id: string) {
    const key = `${type}:${id}`;
    if (comments[key]) return;
    try {
      const result = await get<ItemList<Comment>>(
        `${base}/comments?target_type=${type}&target_id=${id}`,
      );
      setComments((current) => ({ ...current, [key]: result.items ?? [] }));
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Comments could not be loaded.");
    }
  }

  async function addComment(
    event: FormEvent<HTMLFormElement>,
    type: "update" | "discussion" | "task",
    id: string,
  ) {
    event.preventDefault();
    const key = `${type}:${id}`;
    const body = commentDrafts[key]?.trim();
    if (!body) return;
    try {
      await mutate(`${base}/comments`, "POST", { target_type: type, target_id: id, body });
      setCommentDrafts((current) => ({ ...current, [key]: "" }));
      const result = await get<ItemList<Comment>>(
        `${base}/comments?target_type=${type}&target_id=${id}`,
      );
      setComments((current) => ({ ...current, [key]: result.items ?? [] }));
      await refresh();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Comment could not be posted.");
    }
  }

  async function react(id: string, value?: string) {
    try {
      if (value) {
        await mutate(`${base}/reactions`, "PUT", {
          target_type: "update",
          target_id: id,
          reaction: value,
        });
      } else {
        await mutate(`${base}/reactions`, "DELETE", {
          target_type: "update",
          target_id: id,
        });
      }
      await refresh();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Reaction could not be saved.");
    }
  }

  async function report(type: "update" | "discussion" | "task", id: string) {
    const reason = window.prompt(
      "Why are you reporting this item? Enter spam, harassment, unsafe, or other.",
    );
    if (!reason || !["spam", "harassment", "unsafe", "other"].includes(reason)) return;
    try {
      await mutate(`${base}/reports`, "POST", {
        target_type: type,
        target_id: id,
        reason,
        details: "",
      });
      setNotice("Report sent to the project moderation queue.");
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Report could not be sent.");
    }
  }

  if (loading) {
    return (
      <section className="my-10 p-[22px] border border-hairline rounded-2xl bg-surface shadow-[var(--shadow)]" aria-live="polite">
        Loading project work…
      </section>
    );
  }

  if (!project) {
    return (
      <section
        className="my-10 p-[22px] border border-hairline rounded-2xl bg-surface shadow-[var(--shadow)] flex flex-col items-start gap-3"
        role="alert"
      >
        <p className="m-0">{problem || "Project work is unavailable."}</p>
        <button className={secondaryBtn} onClick={() => void refresh()}>
          Retry
        </button>
      </section>
    );
  }

  const commentThread = (type: "update" | "discussion" | "task", id: string, locked = false) => {
    const key = `${type}:${id}`;
    return (
      <details
        className="w-full border-t border-hairline pt-3"
        onToggle={(event) => {
          if (event.currentTarget.open) void loadComments(type, id);
        }}
      >
        <summary className="cursor-pointer text-link font-semibold">Comments</summary>
        <div className="grid gap-2.5 pt-3">
          {(comments[key] ?? []).map((comment) => (
            <div className="py-2.5 px-3 border-l-2 border-brand bg-canvas" key={comment.id}>
              <strong>{comment.author_name}</strong>
              <p className="mt-1 mb-0 whitespace-pre-wrap [overflow-wrap:anywhere]">{comment.body}</p>
            </div>
          ))}
          {(comments[key] ?? []).length === 0 && (
            <p className="text-muted m-0">No comments yet. Start the conversation.</p>
          )}
          {role && !locked && (
            <form
              className="flex flex-col sm:flex-row gap-2"
              onSubmit={(event) => void addComment(event, type, id)}
            >
              <label className="sr-only" htmlFor={`comment-${id}`}>
                Write a comment
              </label>
              <input
                id={`comment-${id}`}
                value={commentDrafts[key] ?? ""}
                onChange={(event) =>
                  setCommentDrafts((current) => ({ ...current, [key]: event.target.value }))
                }
                maxLength={2000}
                placeholder="Add a comment…"
                className={`${fieldControlClass} flex-1 min-w-0`}
              />
              <button
                className={secondaryBtn}
                type="submit"
                disabled={!commentDrafts[key]?.trim()}
              >
                Comment
              </button>
            </form>
          )}
        </div>
      </details>
    );
  };

  return (
    <div className="pt-5 sm:pt-7 pb-16">
      <nav
        className="flex flex-wrap gap-[9px] mb-[26px] text-muted text-[13px]"
        aria-label="Breadcrumb"
      >
        <a className="text-link" href="/projects">
          Projects
        </a>
        <span>/</span>
        <a className="text-link" href={`/projects/${projectID}`}>
          {project.title}
        </a>
        <span>/</span>
        <span>Team work</span>
      </nav>

      <header className="flex flex-col min-[840px]:flex-row items-start min-[840px]:items-end justify-between gap-3.5 py-[26px] border-b border-hairline">
        <div>
          <p className="mt-0 mb-1.5 text-link text-xs font-semibold tracking-[0.02em]">
            Project workspace
          </p>
          <h1 className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] mb-1">
            Team work
          </h1>
          <p className="text-muted m-0">
            Updates, planning, and conversations for <strong>{project.title}</strong>.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3.5 w-full sm:w-auto">
          <button
            className={secondaryBtn}
            onClick={() => void toggleSignal("follow")}
            aria-pressed={following}
          >
            {following ? "Following" : "Follow updates"}
          </button>
          <button
            className={secondaryBtn}
            onClick={() => void toggleSignal("bookmark")}
            aria-pressed={bookmarked}
          >
            {bookmarked ? "Saved" : "Save project"}
          </button>
        </div>
      </header>

      {(problem || notice) && (
        <div
          className={`flex flex-wrap items-center gap-2.5 my-3.5 px-3.5 py-3 border rounded-lg bg-surface ${
            problem ? "border-danger text-danger" : "border-hairline text-success"
          }`}
          role={problem ? "alert" : "status"}
        >
          <span>{problem || notice}</span>
          <button
            type="button"
            onClick={() => {
              setProblem("");
              setNotice("");
            }}
            aria-label="Dismiss message"
            className="ml-auto border-0 bg-transparent text-inherit text-xl cursor-pointer"
          >
            ×
          </button>
          {problem && (
            <button className={secondaryBtn} onClick={() => void refresh()}>
              Reload
            </button>
          )}
        </div>
      )}

      <div
        className="flex gap-1 border-b border-hairline mb-7 overflow-x-auto"
        role="tablist"
        aria-label="Project work sections"
      >
        {(["updates", "tasks", "milestones", "discussions"] as Tab[]).map((item) => (
          <button
            key={item}
            role="tab"
            aria-selected={tab === item}
            onClick={() => setTab(item)}
            className={`shrink-0 px-4 py-3.5 border-0 border-b-2 bg-transparent cursor-pointer font-semibold ${
              tab === item
                ? "text-on-surface border-b-primary"
                : "text-muted border-b-transparent hover:text-on-surface"
            }`}
          >
            {item[0].toUpperCase() + item.slice(1)}
            {item === "tasks" ? ` · ${tasks.length}` : ""}
          </button>
        ))}
      </div>

      {tab === "updates" && (
        <section className="min-h-[300px]" aria-labelledby="updates-title">
          <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-3.5 mb-5">
            <div>
              <h2 id="updates-title" className="font-display text-2xl font-bold leading-tight mb-1">
                Project updates
              </h2>
              <p className="text-muted m-0">
                Dated progress notes from the people building this project.
              </p>
            </div>
            {role && (
              <button className={primaryBtn} onClick={() => setShowNewUpdate(!showNewUpdate)}>
                {showNewUpdate ? "Cancel" : "Post update"}
              </button>
            )}
          </div>

          {showNewUpdate && (
            <form className={editorClass} onSubmit={(event) => void submit(event, "update")}>
              <label className={fieldClass}>
                Update
                <textarea
                  name="body"
                  required
                  maxLength={5000}
                  rows={4}
                  placeholder="Share progress, blockers, or what comes next…"
                  className={`${fieldControlClass} resize-y`}
                />
              </label>
              <label className={fieldClass}>
                Audience
                <select name="audience" className={fieldControlClass}>
                  <option value="members">Project members</option>
                  {project.visibility !== "private" && <option value="campus">Campus</option>}
                </select>
              </label>
              <button className={primaryBtn}>Publish update</button>
            </form>
          )}

          {updates.length === 0 ? (
            <div className="grid justify-items-start max-w-[880px] p-8 border border-hairline rounded-2xl bg-surface shadow-[var(--shadow)]">
              <span className="grid place-items-center w-[46px] h-[46px] rounded-2xl bg-brand/14 text-primary text-[21px] font-bold">
                ↗
              </span>
              <h3 className="font-display text-xl font-bold leading-tight mt-4 mb-1">
                No updates yet
              </h3>
              <p className="m-0 text-muted">
                Share the first milestone of progress with your teammates.
              </p>
            </div>
          ) : (
            <div className="grid gap-4 max-w-[880px]">
              {updates.map((update) => (
                <article className={cardClass} key={update.id}>
                  <div className="flex items-center justify-start gap-3.5">
                    <div className="grid place-items-center w-[38px] h-[38px] shrink-0 rounded-xl bg-brand/14 text-primary font-bold">
                      {update.author_name.slice(0, 1).toUpperCase()}
                    </div>
                    <div className="grid gap-0.5">
                      <strong>{update.author_name}</strong>
                      <time className="text-muted text-xs">{formatDate(update.created_at)}</time>
                    </div>
                    <span className="ml-auto px-2.5 py-1 rounded-full bg-raised text-muted text-xs capitalize">
                      {update.audience === "campus" ? "Campus" : "Members"}
                    </span>
                  </div>
                  <p className="my-[18px] whitespace-pre-wrap [overflow-wrap:anywhere]">
                    {update.body}
                  </p>
                  <div className="flex flex-wrap items-center justify-start gap-3.5 border-t border-hairline pt-3">
                    {role && (
                      <button
                        className={secondaryBtn}
                        aria-pressed={Boolean(update.viewer_reaction)}
                        onClick={() =>
                          void react(update.id, update.viewer_reaction ? undefined : "support")
                        }
                      >
                        {update.viewer_reaction ? "♥ Supported" : "♡ Support"} ·{" "}
                        {update.reaction_count}
                      </button>
                    )}
                    <button
                      className={secondaryBtn}
                      onClick={(event) => {
                        const details = event.currentTarget.closest("article")?.querySelector("details");
                        if (details) details.open = !details.open;
                      }}
                    >
                      Comments · {update.comment_count}
                    </button>
                    {(update.author_id === viewerID || manager) && (
                      <button className={quietBtn} onClick={() => void editUpdate(update)}>
                        Edit
                      </button>
                    )}
                    <button className={quietBtn} onClick={() => void report("update", update.id)}>
                      Report
                    </button>
                  </div>
                  {commentThread("update", update.id)}
                </article>
              ))}
            </div>
          )}
        </section>
      )}

      {tab === "tasks" && (
        <section className="min-h-[300px]" aria-labelledby="tasks-title">
          <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-3.5 mb-5">
            <div>
              <h2 id="tasks-title" className="font-display text-2xl font-bold leading-tight mb-1">
                Tasks
              </h2>
              <p className="text-muted m-0">
                Keep work moving with a board or a keyboard-friendly list.
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-3.5 w-full sm:w-auto">
              <div
                className="inline-flex p-[3px] border border-hairline rounded-lg bg-canvas"
                aria-label="Task view"
              >
                <button
                  aria-pressed={view === "board"}
                  onClick={() => setView("board")}
                  className="border-0 rounded-[6px] py-2 px-3 text-muted bg-transparent cursor-pointer aria-pressed:bg-surface aria-pressed:text-on-surface aria-pressed:shadow-[var(--shadow)]"
                >
                  Board
                </button>
                <button
                  aria-pressed={view === "list"}
                  onClick={() => setView("list")}
                  className="border-0 rounded-[6px] py-2 px-3 text-muted bg-transparent cursor-pointer aria-pressed:bg-surface aria-pressed:text-on-surface aria-pressed:shadow-[var(--shadow)]"
                >
                  List
                </button>
              </div>
              {role && (
                <button className={primaryBtn} onClick={() => setShowNewTask(!showNewTask)}>
                  {showNewTask ? "Cancel" : "New task"}
                </button>
              )}
            </div>
          </div>

          {showNewTask && (
            <form className={editorClass} onSubmit={(event) => void submit(event, "task")}>
              <label className={fieldClass}>
                Task title
                <input
                  name="title"
                  required
                  minLength={3}
                  maxLength={180}
                  className={fieldControlClass}
                />
              </label>
              <label className={fieldClass}>
                Description
                <textarea
                  name="description"
                  maxLength={5000}
                  rows={3}
                  className={`${fieldControlClass} resize-y`}
                />
              </label>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3.5">
                <label className={fieldClass}>
                  Priority
                  <select name="priority" className={fieldControlClass}>
                    <option value="normal">Normal</option>
                    <option value="low">Low</option>
                    <option value="high">High</option>
                  </select>
                </label>
                <label className={fieldClass}>
                  Milestone
                  <select name="milestone_id" className={fieldControlClass}>
                    <option value="">No milestone</option>
                    {milestones
                      .filter((m) => m.status === "open")
                      .map((m) => (
                        <option value={m.id} key={m.id}>
                          {m.title}
                        </option>
                      ))}
                  </select>
                </label>
                <label className={fieldClass}>
                  Due date
                  <input type="date" name="due_at" className={fieldControlClass} />
                </label>
                <label className={fieldClass}>
                  Assign to (your account)
                  <input
                    name="assignee_id"
                    value={viewerID}
                    readOnly
                    placeholder="Sign in to assign"
                    className={fieldControlClass}
                  />
                </label>
              </div>
              <button className={primaryBtn}>Create task</button>
            </form>
          )}

          {tasks.length === 0 ? (
            <div className="grid justify-items-start max-w-[880px] p-8 border border-hairline rounded-2xl bg-surface shadow-[var(--shadow)]">
              <span className="grid place-items-center w-[46px] h-[46px] rounded-2xl bg-brand/14 text-primary text-[21px] font-bold">
                ✓
              </span>
              <h3 className="font-display text-xl font-bold leading-tight mt-4 mb-1">
                Room for the first task
              </h3>
              <p className="m-0 text-muted">
                Break your next project goal into a small, assignable step.
              </p>
            </div>
          ) : view === "board" ? (
            <div
              className="grid grid-cols-[repeat(4,minmax(240px,1fr))] min-[840px]:grid-cols-[repeat(4,minmax(210px,1fr))] gap-3 items-start overflow-x-auto pb-2"
              aria-label="Task board"
            >
              {statuses.map((status) => (
                <div
                  className="min-h-[210px] p-3 border border-hairline rounded-2xl bg-raised/50"
                  key={status}
                >
                  <h3 className="flex justify-between mt-0 mb-2.5 pt-0.5 px-1 pb-2 border-b border-hairline text-[13px]">
                    {statusNames[status]}{" "}
                    <span className="text-muted">{taskGroups[status].length}</span>
                  </h3>
                  {taskGroups[status].map((task) => (
                    <article
                      className="grid gap-2.5 my-2 p-[13px] border border-hairline rounded-lg bg-surface shadow-[var(--shadow)] focus-visible:outline-2 focus-visible:outline-link focus-visible:outline-offset-2"
                      key={task.id}
                      tabIndex={0}
                      onKeyDown={(event) => void keyboardMove(event, task)}
                      aria-label={`${task.title}, ${statusNames[task.status]}. Use left and right arrow keys to change status.`}
                    >
                      <strong>{task.title}</strong>
                      {task.description && (
                        <p className="m-0 text-muted [overflow-wrap:anywhere]">{task.description}</p>
                      )}
                      <small className="text-muted">
                        {task.milestone_title ?? "No milestone"} · {formatDate(task.due_at)}
                      </small>
                      <small className="text-muted">
                        {task.assignee_names.join(", ") || "Unassigned"}
                      </small>
                      <label className="flex items-center justify-between gap-2 text-muted text-xs">
                        Status
                        <select
                          aria-label={`Status for ${task.title}`}
                          value={task.status}
                          disabled={!task.can_edit}
                          onChange={(event) => void moveTask(task, event.target.value as Status)}
                          className="min-h-9 px-2 py-1 border border-control rounded-lg bg-canvas text-on-surface"
                        >
                          <option value="todo">To do</option>
                          <option value="in_progress">In progress</option>
                          <option value="blocked">Blocked</option>
                          <option value="done">Done</option>
                        </select>
                      </label>
                      <div className="flex flex-wrap items-center justify-between gap-1.5">
                        {task.can_edit && (
                          <button
                            type="button"
                            className={quietBtn}
                            onClick={() => void editTask(task)}
                          >
                            Edit task
                          </button>
                        )}
                        <div className="flex-1 basis-full">{commentThread("task", task.id)}</div>
                        <button className={quietBtn} onClick={() => void report("task", task.id)}>
                          Report
                        </button>
                      </div>
                    </article>
                  ))}
                </div>
              ))}
            </div>
          ) : (
            <div className="overflow-x-auto border border-hairline rounded-2xl bg-surface">
              <table className="w-full min-w-[720px] border-collapse">
                <thead>
                  <tr className="border-b border-hairline">
                    <th className="p-3 text-left text-muted text-xs uppercase tracking-[0.04em]">
                      Task
                    </th>
                    <th className="p-3 text-left text-muted text-xs uppercase tracking-[0.04em]">
                      Status
                    </th>
                    <th className="p-3 text-left text-muted text-xs uppercase tracking-[0.04em]">
                      Assignee
                    </th>
                    <th className="p-3 text-left text-muted text-xs uppercase tracking-[0.04em]">
                      Milestone
                    </th>
                    <th className="p-3 text-left text-muted text-xs uppercase tracking-[0.04em]">
                      Due
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {tasks.map((task) => (
                    <tr key={task.id} className="border-b border-hairline last:border-b-0">
                      <td className="p-3 align-top">
                        <strong>{task.title}</strong>
                        <small className="block max-w-[42ch] mt-1 text-muted">
                          {task.description}
                        </small>
                      </td>
                      <td className="p-3 align-top">
                        <select
                          aria-label={`Status for ${task.title}`}
                          value={task.status}
                          disabled={!task.can_edit}
                          onChange={(event) => void moveTask(task, event.target.value as Status)}
                          className={fieldControlClass}
                        >
                          {statuses.map((status) => (
                            <option key={status} value={status}>
                              {statusNames[status]}
                            </option>
                          ))}
                        </select>
                      </td>
                      <td className="p-3 align-top">
                        {task.assignee_names.join(", ") || "Unassigned"}
                      </td>
                      <td className="p-3 align-top">{task.milestone_title ?? "—"}</td>
                      <td className="p-3 align-top">{formatDate(task.due_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <p className="mt-3.5 text-muted text-[13px]">
            Keyboard: focus a task card and use ← / → to move it between adjacent columns. The list
            view and status selector are alternatives.
          </p>
        </section>
      )}

      {tab === "milestones" && (
        <section className="min-h-[300px]" aria-labelledby="milestones-title">
          <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-3.5 mb-5">
            <div>
              <h2
                id="milestones-title"
                className="font-display text-2xl font-bold leading-tight mb-1"
              >
                Milestones
              </h2>
              <p className="text-muted m-0">
                A shared view of the outcomes the team is working toward.
              </p>
            </div>
            {manager && (
              <button
                className={primaryBtn}
                onClick={() => setShowNewMilestone(!showNewMilestone)}
              >
                {showNewMilestone ? "Cancel" : "New milestone"}
              </button>
            )}
          </div>

          {showNewMilestone && (
            <form className={editorClass} onSubmit={(event) => void submit(event, "milestone")}>
              <label className={fieldClass}>
                Milestone title
                <input
                  name="title"
                  required
                  minLength={3}
                  maxLength={160}
                  className={fieldControlClass}
                />
              </label>
              <label className={fieldClass}>
                Description
                <textarea
                  name="description"
                  rows={3}
                  maxLength={3000}
                  className={`${fieldControlClass} resize-y`}
                />
              </label>
              <label className={fieldClass}>
                Target date
                <input type="date" name="due_at" className={fieldControlClass} />
              </label>
              <button className={primaryBtn}>Create milestone</button>
            </form>
          )}

          {!manager && (
            <p className="mt-3.5 p-[22px] max-w-[880px] border border-hairline rounded-2xl bg-surface text-muted text-[13px]">
              Only project owners and maintainers can create or update milestones.
            </p>
          )}

          {milestones.length === 0 ? (
            <div className="grid justify-items-start max-w-[880px] p-8 border border-hairline rounded-2xl bg-surface shadow-[var(--shadow)]">
              <span className="grid place-items-center w-[46px] h-[46px] rounded-2xl bg-brand/14 text-primary text-[21px] font-bold">
                ◎
              </span>
              <h3 className="font-display text-xl font-bold leading-tight mt-4 mb-1">
                No milestones yet
              </h3>
              <p className="m-0 text-muted">
                Milestones make larger goals visible and give tasks a shared destination.
              </p>
            </div>
          ) : (
            <div className="grid gap-4 max-w-[880px]">
              {milestones.map((milestone) => {
                const progress = milestone.task_count
                  ? Math.round((100 * milestone.completed_count) / milestone.task_count)
                  : 0;
                return (
                  <article className={cardClass} key={milestone.id}>
                    <div className="flex items-center justify-between gap-3.5">
                      <div>
                        <span className="inline-block px-2.5 py-1 rounded-full bg-raised text-muted text-xs capitalize">
                          {milestone.status}
                        </span>
                        <h3 className="font-display text-[19px] font-bold leading-tight mt-2 mb-0">
                          {milestone.title}
                        </h3>
                      </div>
                      <span className="shrink-0 text-muted text-[13px]">
                        {formatDate(milestone.due_at)}
                      </span>
                    </div>
                    {milestone.description && (
                      <p className="my-[18px] whitespace-pre-wrap [overflow-wrap:anywhere]">
                        {milestone.description}
                      </p>
                    )}
                    <div className="flex items-center justify-between gap-3.5 mt-[18px] mb-[7px] text-muted text-[13px]">
                      <span>
                        {milestone.completed_count} of {milestone.task_count} tasks complete
                      </span>
                      <strong>{progress}%</strong>
                    </div>
                    <div className="h-2 overflow-hidden rounded-[10px] bg-raised">
                      <span
                        className="block h-full rounded-[inherit] bg-brand"
                        style={{ width: `${progress}%` }}
                      />
                    </div>
                    {manager && (
                      <label className="flex items-center gap-2.5 mt-3 text-muted text-[13px]">
                        Milestone status
                        <select
                          value={milestone.status}
                          onChange={(event) => void changeMilestone(milestone, event.target.value)}
                          className="min-h-9 px-2 py-1 border border-control rounded-lg bg-canvas text-on-surface"
                        >
                          <option value="open">Open</option>
                          <option value="completed">Completed</option>
                          <option value="cancelled">Cancelled</option>
                        </select>
                      </label>
                    )}
                    <a
                      href="#tasks-title"
                      onClick={() => setTab("tasks")}
                      className="inline-block mt-[15px] font-semibold"
                    >
                      View milestone tasks →
                    </a>
                  </article>
                );
              })}
            </div>
          )}
        </section>
      )}

      {tab === "discussions" && (
        <section className="min-h-[300px]" aria-labelledby="discussions-title">
          <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-3.5 mb-5">
            <div>
              <h2
                id="discussions-title"
                className="font-display text-2xl font-bold leading-tight mb-1"
              >
                Discussions
              </h2>
              <p className="text-muted m-0">
                Open questions and decisions stay with the project.
              </p>
            </div>
            {role && (
              <button
                className={primaryBtn}
                onClick={() => setShowNewDiscussion(!showNewDiscussion)}
              >
                {showNewDiscussion ? "Cancel" : "Start discussion"}
              </button>
            )}
          </div>

          {showNewDiscussion && (
            <form className={editorClass} onSubmit={(event) => void submit(event, "discussion")}>
              <label className={fieldClass}>
                Topic
                <input
                  name="title"
                  required
                  minLength={3}
                  maxLength={160}
                  placeholder="What should the team discuss?"
                  className={fieldControlClass}
                />
              </label>
              <label className={fieldClass}>
                First post
                <textarea
                  name="body"
                  required
                  rows={4}
                  maxLength={5000}
                  className={`${fieldControlClass} resize-y`}
                />
              </label>
              <button className={primaryBtn}>Post topic</button>
            </form>
          )}

          {discussions.length === 0 ? (
            <div className="grid justify-items-start max-w-[880px] p-8 border border-hairline rounded-2xl bg-surface shadow-[var(--shadow)]">
              <span className="grid place-items-center w-[46px] h-[46px] rounded-2xl bg-brand/14 text-primary text-[21px] font-bold">
                ◌
              </span>
              <h3 className="font-display text-xl font-bold leading-tight mt-4 mb-1">
                No discussions yet
              </h3>
              <p className="m-0 text-muted">
                Start a focused thread for a design question, decision, or idea.
              </p>
            </div>
          ) : (
            <div className="grid gap-4 max-w-[880px]">
              {discussions.map((discussion) => (
                <article className={cardClass} key={discussion.id}>
                  <div className="flex items-center justify-start gap-3.5">
                    <div className="grid place-items-center w-[38px] h-[38px] shrink-0 rounded-xl bg-brand/14 text-primary font-bold">
                      {discussion.author_name.slice(0, 1).toUpperCase()}
                    </div>
                    <div className="grid gap-0.5">
                      <strong>{discussion.author_name}</strong>
                      <time className="text-muted text-xs">
                        {formatDate(discussion.created_at)}
                      </time>
                    </div>
                    <span className="ml-auto px-2.5 py-1 rounded-full bg-raised text-muted text-xs capitalize">
                      {discussion.status}
                    </span>
                  </div>
                  <h3 className="font-display text-[19px] font-bold leading-snug mt-2 mb-0">
                    {discussion.title}
                  </h3>
                  <p className="my-[18px] whitespace-pre-wrap [overflow-wrap:anywhere]">
                    {discussion.body}
                  </p>
                  <div className="flex flex-wrap items-center justify-start gap-3.5 border-t border-hairline pt-3">
                    <span className="text-muted">{discussion.reply_count} replies</span>
                    <button
                      className={quietBtn}
                      onClick={() => void report("discussion", discussion.id)}
                    >
                      Report
                    </button>
                  </div>
                  {commentThread("discussion", discussion.id, discussion.status === "locked")}
                </article>
              ))}
            </div>
          )}
        </section>
      )}
    </div>
  );
}
