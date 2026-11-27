"use client";

import { FormEvent, useEffect, useState } from "react";
import { RepositoryOverview } from "./repository-overview";
import { RepositoryCode } from "./repository-code";
import { LegacyRepositoryLink } from "./legacy-repository-link";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import { fetchSession, readApiEnvelope } from "@/lib/http";
import { ConversationLaunchButton } from "@/components/collaboration/conversation-launch-button";

type Project = {
  id: string;
  title: string;
  summary: string;
  visibility: string;
  lifecycle: string;
  project_type: string;
  recruiting: boolean;
  skills: string[];
  version: number;
};
type Member = { user_id: string; display_name: string; role: string };
type Role = {
  id: string;
  title: string;
  description: string;
  openings: number;
  status: string;
  version: number;
  difficulty: string;
  good_first_task: boolean;
};
type Application = {
  id: string;
  role_id: string;
  applicant_name: string;
  message: string;
  status: string;
  version: number;
};
type Details = {
  project: Project;
  description: string;
  viewer_role: string;
  members: Member[];
};
type RepositoryAccess = {
  full_name: string;
  html_url?: string;
  default_branch: string;
  source_access?: boolean;
  access_state: string;
  sync_health: string;
};

const cardClass = "min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col";
const fieldClass = "grid gap-1.5 text-[13px] font-medium text-on-surface";
const inputClass =
  "w-full min-h-11 px-3.5 py-2.5 border border-control rounded-lg bg-surface text-on-surface text-sm leading-5";
const primaryBtn =
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-[18px] py-2 text-sm font-medium border border-primary bg-primary text-on-primary cursor-pointer hover:opacity-90";
const compactPrimaryBtn =
  "inline-flex min-h-[38px] items-center justify-center gap-2 rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-primary bg-primary text-on-primary cursor-pointer hover:opacity-90";
const compactSecondaryBtn =
  "inline-flex min-h-[38px] items-center justify-center gap-2 rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised";
const tabLinkClass =
  "relative text-muted no-underline whitespace-nowrap px-3.5 py-2.5 text-[13px] font-medium border-b-2 border-transparent hover:text-on-surface aria-[current=page]:text-on-surface aria-[current=page]:font-semibold aria-[current=page]:border-b-link";

export function ProjectDetail({ projectID }: { projectID: string }) {
  const [details, setDetails] = useState<Details | null>(null);
  const [roles, setRoles] = useState<Role[]>([]);
  const [applications, setApplications] = useState<Application[]>([]);
  const [csrf, setCSRF] = useState("");
  const [campusAccess, setCampusAccess] = useState(false);
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");
  const [repository, setRepository] = useState<RepositoryAccess | null>(null);
  const [repositoryChecked, setRepositoryChecked] = useState(false);
  const hasRepository = repository !== null;
  const [summary, setSummary] = useState("");
  const [description, setDescription] = useState("");
  const [roleTitle, setRoleTitle] = useState("");
  const [roleDescription, setRoleDescription] = useState("");
  const [roleOpenings, setRoleOpenings] = useState("1");
  const [applicationMessage, setApplicationMessage] = useState<Record<string, string>>({});
  const [tab, setTab] = useState("code");
  const [accessCheck, setAccessCheck] = useState(0);

  useEffect(() => {
    function revalidate() {
      setDetails(null);
      setRepository(null);
      setRepositoryChecked(false);
      setRoles([]);
      setApplications([]);
      setCSRF("");
      setCampusAccess(false);
      setProblem("");
      setMessage("");
      setAccessCheck((value) => value + 1);
    }
    function onReturn(event: PageTransitionEvent) {
      if (event.persisted) revalidate();
    }
    function onVisibility() {
      if (document.visibilityState === "visible") revalidate();
    }
    window.addEventListener("pageshow", onReturn);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.removeEventListener("pageshow", onReturn);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, []);

  useEffect(() => {
    setTab(new URLSearchParams(window.location.search).get("tab") || "code");
  }, []);

  async function loadRoles() {
    const response = await fetch(`/api/v1/projects/${projectID}/roles`, {
      cache: "no-store",
    });
    if (!response.ok) return;
    const result = await readApiEnvelope<{ items: Role[] }>(response, "");
    setRoles(result.data?.items ?? []);
  }

  async function loadApplications() {
    const response = await fetch(`/api/v1/projects/${projectID}/applications`, {
      cache: "no-store",
    });
    if (!response.ok) return;
    const result = await readApiEnvelope<{ items: Application[] }>(response, "");
    setApplications(result.data?.items ?? []);
  }

  useEffect(() => {
    let active = true;
    void Promise.all([
      fetch(`/api/v1/projects/${projectID}`, { cache: "no-store" }).then(async (response) => {
        const result = await readApiEnvelope<Details>(response, "Project could not be loaded.");
        if (!response.ok || !result.data) {
          throw new Error(result.error?.message ?? "Project could not be loaded.");
        }
        return result.data;
      }),
      fetchSession().then(async (response) => {
        const result = await readApiEnvelope<{
          csrf_token?: string;
          user?: {
            college_id?: string;
            terms_accepted?: boolean;
            privacy_accepted?: boolean;
          };
        }>(response, "");
        return result.data;
      }),
    ])
      .then(async ([project, session]) => {
        if (!active) return;
        setDetails(project);
        setSummary(project.project.summary);
        setDescription(project.description);
        setCSRF(session?.csrf_token ?? "");
        const ready = Boolean(
          session?.user?.college_id &&
            session.user.terms_accepted &&
            session.user.privacy_accepted,
        );
        setCampusAccess(ready);
        if (project.viewer_role) {
          const response = await fetch(`/api/v1/projects/${projectID}/repository`, {
            cache: "no-store",
          });
          if (response.ok) {
            const result = await readApiEnvelope<{
              repository: RepositoryAccess | null;
            }>(response, "");
            if (active && result.data) {
              setRepository(result.data.repository);
              setRepositoryChecked(true);
            }
          } else if (active) {
            setProblem("Repository access could not be confirmed.");
          }
        }
        if (ready) await loadRoles();
        if (["owner", "maintainer"].includes(project.viewer_role)) {
          await loadApplications();
        }
      })
      .catch(
        (error) =>
          active &&
          setProblem(error instanceof Error ? error.message : "Project could not be loaded."),
      );
    return () => {
      active = false;
    };
  }, [projectID, accessCheck]);

  async function mutate(
    path: string,
    body: unknown,
    success: string,
    method = "POST",
  ) {
    setProblem("");
    setMessage("");
    const response = await fetch(path, {
      method,
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
      body: JSON.stringify(body),
    });
    const result = await readApiEnvelope<{ version?: number }>(
      response,
      "The change could not be saved.",
    );
    if (!response.ok) {
      throw new Error(result.error?.message ?? "The change could not be saved.");
    }
    setMessage(success);
    return result.data;
  }

  async function saveDetails(event: FormEvent) {
    event.preventDefault();
    if (!details) return;
    try {
      const result = await mutate(
        `/api/v1/projects/${projectID}`,
        {
          summary,
          description,
          visibility: details.project.visibility,
          lifecycle: details.project.lifecycle,
          version: details.project.version,
        },
        "Project details saved.",
        "PATCH",
      );
      setDetails({
        ...details,
        description,
        project: {
          ...details.project,
          summary,
          version: result?.version ?? details.project.version,
        },
      });
    } catch (error) {
      setProblem(
        error instanceof Error ? error.message : "Project details could not be saved.",
      );
    }
  }

  async function publish(visibility: "private" | "campus" | "public") {
    if (!details) return;
    try {
      const success =
        visibility === "private"
          ? "Project made private."
          : `Project published to ${
              visibility === "campus" ? "your campus" : "the public discovery page"
            }. GitHub source files and private snapshots keep their existing access rules.`;
      const result = await mutate(
        `/api/v1/projects/${projectID}`,
        {
          summary,
          description,
          visibility,
          lifecycle: "active",
          version: details.project.version,
        },
        success,
        "PATCH",
      );
      setDetails({
        ...details,
        description,
        project: {
          ...details.project,
          summary,
          visibility,
          lifecycle: "active",
          version: result?.version ?? details.project.version,
        },
      });
    } catch (error) {
      setProblem(
        error instanceof Error ? error.message : "Project could not be published.",
      );
    }
  }

  async function createRole(event: FormEvent) {
    event.preventDefault();
    try {
      await mutate(
        `/api/v1/projects/${projectID}/roles`,
        {
          title: roleTitle,
          description: roleDescription,
          openings: Number(roleOpenings),
          difficulty: "beginner",
          good_first_task: false,
        },
        "Team opening created.",
      );
      setRoleTitle("");
      setRoleDescription("");
      setRoleOpenings("1");
      await loadRoles();
    } catch (error) {
      setProblem(
        error instanceof Error ? error.message : "Opening could not be created.",
      );
    }
  }

  async function apply(role: Role) {
    try {
      await mutate(
        `/api/v1/projects/${projectID}/applications`,
        { role_id: role.id, message: applicationMessage[role.id] ?? "" },
        "Application sent.",
      );
      setApplicationMessage((value) => ({ ...value, [role.id]: "" }));
    } catch (error) {
      setProblem(
        error instanceof Error ? error.message : "Application could not be sent.",
      );
    }
  }

  async function decide(
    application: Application,
    decision: "accepted" | "rejected",
  ) {
    try {
      await mutate(
        `/api/v1/projects/${projectID}/applications/${application.id}/decision`,
        { decision, version: application.version },
        `Application ${decision}.`,
      );
      await loadApplications();
    } catch (error) {
      setProblem(
        error instanceof Error
          ? error.message
          : "Application decision could not be saved.",
      );
    }
  }

  async function archiveProject() {
    if (!details) return;
    try {
      const result = await mutate(
        `/api/v1/projects/${projectID}`,
        {
          summary,
          description,
          visibility: "private",
          lifecycle: "archived",
          version: details.project.version,
        },
        "Project archived.",
        "PATCH",
      );
      setDetails({
        ...details,
        description,
        project: {
          ...details.project,
          summary,
          visibility: "private",
          lifecycle: "archived",
          version: result?.version ?? details.project.version,
        },
      });
    } catch (error) {
      setProblem(
        error instanceof Error ? error.message : "Project could not be archived.",
      );
    }
  }

  const manager = Boolean(
    details && ["owner", "maintainer"].includes(details.viewer_role),
  );

  const providerURL =
    repository?.html_url &&
    /^https:\/\/github\.com\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+\/?$/.test(
      repository.html_url,
    )
      ? repository.html_url.replace(/\/$/, "")
      : undefined;

  return (
    <>
      <a
        className="fixed z-100 left-4 -top-20 px-4 py-2.5 bg-primary text-on-primary rounded-lg font-semibold no-underline focus:top-3"
        href="#project-content"
      >
        Skip to content
      </a>
      <SiteHeader workspace />

      {details && (
        <section className="bg-surface border-b border-hairline pt-6">
          <div className="w-[min(1440px,calc(100%-32px))] md:w-[min(1440px,calc(100%-64px))] mx-auto flex flex-col gap-4">
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div className="flex flex-col gap-1.5">
                <p className="text-xs font-medium leading-4 text-muted m-0">
                  Projects / {details.project.title} ·{" "}
                  {details.project.project_type.replaceAll("_", " ")}
                </p>
                <h1 className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0">
                  {details.project.title}
                </h1>
                <p className="text-sm leading-[21px] text-muted m-0 max-w-[60ch]">
                  {details.project.summary}
                </p>
                <div className="flex flex-wrap gap-2 pt-1.5">
                  {repository && (
                    <span className="font-mono text-[13px] leading-[19px] self-center">
                      GitHub · {repository.full_name}
                    </span>
                  )}
                  <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-raised border border-hairline text-xs font-medium leading-4">
                    Project visibility: {details.project.visibility}
                  </span>
                  {repository && (
                    <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-raised border border-hairline text-xs font-medium leading-4">
                      Source access:{" "}
                      {repository.source_access === false
                        ? repository.access_state.replaceAll("_", " ")
                        : "authorized project members"}
                    </span>
                  )}
                </div>
              </div>

              <div className="flex flex-wrap gap-2.5 items-center">
                <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-success text-success text-xs font-medium leading-4">
                  {details.project.lifecycle}
                  {repository ? ` · ${repository.sync_health.replaceAll("_", " ")}` : ""}
                </span>
                {providerURL && (
                  <a
                    className={compactPrimaryBtn}
                    href={providerURL}
                    target="_blank"
                    rel="noreferrer"
                  >
                    Open on GitHub ↗
                  </a>
                )}
                {details.viewer_role && (
                  <ConversationLaunchButton contextType="project" resourceID={projectID} label="Message team" />
                )}
              </div>
            </div>

            <nav
              className="flex items-center justify-between gap-4 overflow-x-auto [-webkit-overflow-scrolling:touch]"
              aria-label="Project sections"
            >
              <div className="flex items-center gap-1">
                {[
                  ["code", details.viewer_role ? "Code" : "Overview"],
                  ...(details.viewer_role && hasRepository
                    ? [
                        ["activity", "Activity"],
                        ["issues", "Issues"],
                        ["pull-requests", "Pull requests"],
                        ["security", "Security"],
                      ]
                    : []),
                  ...(manager ? [["settings", "Settings"]] : []),
                ].map(([value, label]) => (
                  <a
                    key={value}
                    className={tabLinkClass}
                    href={`/projects/${projectID}?tab=${value}`}
                    aria-current={tab === value ? "page" : undefined}
                  >
                    {label}
                  </a>
                ))}
              </div>
              <div className="flex items-center gap-1 border-l border-hairline pl-3">
                <a
                  className={tabLinkClass}
                  href={`/projects/${projectID}?tab=open-roles`}
                  aria-current={tab === "open-roles" ? "page" : undefined}
                >
                  Open roles
                </a>
                {details.viewer_role && hasRepository && (
                  <a
                    className={tabLinkClass}
                    href={`/projects/${projectID}?tab=snapshots`}
                    aria-current={tab === "snapshots" ? "page" : undefined}
                  >
                    Snapshots
                  </a>
                )}
                <a className={tabLinkClass} href={`/projects/${projectID}/work`}>
                  Project work
                </a>
              </div>
            </nav>
          </div>
        </section>
      )}

      <main
        id="project-content"
        className="flex-1 w-[min(1440px,calc(100%-32px))] md:w-[min(1440px,calc(100%-64px))] mx-auto pt-6 pb-16 flex flex-col gap-6"
      >
        {problem && (
          <p
            className="rounded-lg px-4 py-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger m-0"
            role="alert"
          >
            {problem}
          </p>
        )}
        {message && (
          <p
            className="rounded-lg px-4 py-3 text-sm font-medium bg-success/12 border border-success/35 text-success m-0"
            role="status"
          >
            {message}
          </p>
        )}

        {!details ? (
          problem ? null : (
            <p role="status">Loading project…</p>
          )
        ) : (
          <>
            {tab === "code" && !details.viewer_role && (
              <section className={`${cardClass} gap-3`}>
                <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
                  Project overview
                </h2>
                <p className="text-sm leading-[21px] m-0">{details.project.summary}</p>
                <p className="text-[13px] leading-[19px] text-muted m-0">
                  Repository source is available to project members with access.
                </p>
              </section>
            )}

            {[
              "activity",
              "issues",
              "pull-requests",
              "security",
              "snapshots",
              "settings",
            ].includes(tab) &&
              (!details.viewer_role || (tab === "settings" && !manager)) && (
                <p
                  role="status"
                  className="flex items-start gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px] m-0"
                >
                  This section is unavailable to your account.
                </p>
              )}

            {tab === "code" && details.viewer_role && hasRepository && (
              <RepositoryCode
                projectID={projectID}
                branch={repository?.default_branch ?? ""}
              />
            )}

            {tab === "code" && details.viewer_role && hasRepository && (
              <RepositoryOverview
                projectID={projectID}
                csrf={csrf}
                tab="code"
                viewerRole={details.viewer_role}
              />
            )}

            {["activity", "issues", "pull-requests", "security", "snapshots"].includes(tab) &&
              details.viewer_role &&
              hasRepository && (
                <RepositoryOverview
                  projectID={projectID}
                  csrf={csrf}
                  tab={tab}
                  viewerRole={details.viewer_role}
                />
              )}

            {tab === "settings" && manager && (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 items-start">
                {details.viewer_role === "owner" && (
                  <section className={`${cardClass} gap-3 sm:col-span-2`}>
                    <div>
                      <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
                        Project succession
                      </h2>
                      <p className="text-sm leading-[21px] text-muted mt-1 mb-0">
                        Hand ownership to a current member before you leave. The GitHub repository stays
                        under its current GitHub owner.
                      </p>
                    </div>
                    <div>
                      <a
                        className={compactSecondaryBtn}
                        href={`/account/succession?resource=project:${encodeURIComponent(projectID)}`}
                      >
                        Review eligible successors
                      </a>
                    </div>
                  </section>
                )}
                <section className={`${cardClass} gap-4`}>
                  <div>
                    <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
                      Project details
                    </h2>
                    <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">
                      PeerGit visibility: <strong>{details.project.visibility}</strong> ·
                      lifecycle: {details.project.lifecycle}
                    </p>
                  </div>
                  <form className="grid gap-3.5" onSubmit={(event) => void saveDetails(event)}>
                    <label className={fieldClass}>
                      Summary
                      <textarea
                        required
                        maxLength={500}
                        value={summary}
                        onChange={(event) => setSummary(event.target.value)}
                        className={`${inputClass} min-h-24 resize-y leading-normal`}
                      />
                    </label>
                    <label className={fieldClass}>
                      Description
                      <textarea
                        maxLength={12000}
                        rows={6}
                        value={description}
                        onChange={(event) => setDescription(event.target.value)}
                        className={`${inputClass} min-h-24 resize-y leading-normal`}
                      />
                    </label>
                    <div>
                      <button className={primaryBtn} type="submit">
                        Save details
                      </button>
                    </div>
                  </form>
                </section>

                <div className="flex flex-col gap-5">
                  <section className={`${cardClass} gap-3.5`}>
                    <h3 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
                      Project publication &amp; visibility
                    </h3>
                    <p className="text-[13px] leading-[19px] text-muted m-0">
                      Project publication shares its PeerGit presentation. GitHub visibility does
                      not change, and private source, evidence, and snapshots remain restricted to
                      project members.
                    </p>
                    <div className="flex flex-wrap gap-2.5 items-center">
                      {hasRepository && details.project.lifecycle !== "archived" && (
                        <>
                          <button
                            className={compactSecondaryBtn}
                            type="button"
                            onClick={() => void publish("campus")}
                          >
                            Publish to campus
                          </button>
                          <button
                            className={compactSecondaryBtn}
                            type="button"
                            onClick={() => void publish("public")}
                          >
                            Publish publicly
                          </button>
                        </>
                      )}
                      {details.project.visibility !== "private" && (
                        <button
                          className={compactSecondaryBtn}
                          type="button"
                          onClick={() => void publish("private")}
                        >
                          Make private
                        </button>
                      )}
                    </div>
                  </section>

                  <section className={`${cardClass} gap-2.5`}>
                    <h3 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                      GitHub connection
                    </h3>
                    <p className="text-[13px] leading-[19px] text-muted m-0">
                      Repository access follows the connected GitHub App installation. Manage or
                      revoke your account connection from{" "}
                      <a href="/account/github">GitHub connections</a>.
                    </p>
                  </section>

                  {details.project.lifecycle !== "archived" && (
                    <section className="min-w-0 max-w-full bg-surface border border-danger rounded-2xl p-6 flex flex-wrap items-center justify-between gap-3">
                      <div>
                        <h3 className="text-base font-semibold leading-[22px] text-danger m-0">
                          Archive project
                        </h3>
                        <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">
                          Makes this project read-only and removes it from discovery.
                        </p>
                      </div>
                      <button
                        className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium whitespace-nowrap border border-danger bg-danger text-on-primary cursor-pointer hover:opacity-90"
                        type="button"
                        onClick={() => void archiveProject()}
                      >
                        Archive project
                      </button>
                    </section>
                  )}
                </div>
              </div>
            )}

            {tab === "code" && repositoryChecked && !hasRepository && manager && (
              <LegacyRepositoryLink projectID={projectID} csrf={csrf} />
            )}

            {tab === "open-roles" && (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 items-start">
                <section className={`${cardClass} gap-[18px]`}>
                  <div>
                    <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
                      Open roles
                    </h2>
                    {!campusAccess && (
                      <p className="text-[13px] leading-[19px] text-muted mt-1.5 mb-0">
                        <a href="/onboarding">Verify campus access</a> to view openings and apply.
                      </p>
                    )}
                  </div>

                  {roles.filter((role) => role.status === "open").length ? (
                    <div className="grid gap-3.5">
                      {roles
                        .filter((role) => role.status === "open")
                        .map((role) => (
                          <article
                            className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl p-5 flex flex-col gap-2.5"
                            key={role.id}
                          >
                            <div className="flex items-center justify-between gap-2">
                              <h3 className="text-base font-semibold leading-[22px] m-0">
                                {role.title}
                              </h3>
                              <span className="text-xs font-medium leading-4 text-success">
                                {role.openings} opening(s)
                              </span>
                            </div>
                            <p className="text-[13px] leading-[19px] text-muted m-0">
                              {role.description}
                            </p>
                            <p className="text-xs font-medium leading-4 text-muted m-0">
                              Difficulty: {role.difficulty}
                              {role.good_first_task ? " · beginner-friendly" : ""}
                            </p>
                            {campusAccess &&
                              details.project.lifecycle === "active" &&
                              !manager && (
                                <form
                                  className="grid gap-3.5 mt-1.5 pt-3 border-t border-hairline"
                                  onSubmit={(event) => {
                                    event.preventDefault();
                                    void apply(role);
                                  }}
                                >
                                  <label className={fieldClass}>
                                    Why are you interested?
                                    <textarea
                                      required
                                      maxLength={2000}
                                      value={applicationMessage[role.id] ?? ""}
                                      onChange={(event) =>
                                        setApplicationMessage((value) => ({
                                          ...value,
                                          [role.id]: event.target.value,
                                        }))
                                      }
                                      className={`${inputClass} min-h-24 resize-y leading-normal`}
                                    />
                                  </label>
                                  <div>
                                    <button className={compactPrimaryBtn} type="submit">
                                      Apply
                                    </button>
                                  </div>
                                </form>
                              )}
                          </article>
                        ))}
                    </div>
                  ) : (
                    <p className="text-sm leading-[21px] text-muted m-0">
                      This project has no open roles.
                    </p>
                  )}

                  {manager &&
                    hasRepository &&
                    details.project.lifecycle === "active" && (
                      <form
                        className="grid gap-3.5 mt-5 pt-5 border-t border-hairline"
                        onSubmit={(event) => void createRole(event)}
                      >
                        <h3 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                          Open a team role
                        </h3>
                        <label className={fieldClass}>
                          Role title
                          <input
                            required
                            minLength={2}
                            maxLength={100}
                            value={roleTitle}
                            onChange={(event) => setRoleTitle(event.target.value)}
                            className={inputClass}
                          />
                        </label>
                        <label className={fieldClass}>
                          What will the member work on?
                          <textarea
                            required
                            maxLength={3000}
                            value={roleDescription}
                            onChange={(event) => setRoleDescription(event.target.value)}
                            className={`${inputClass} min-h-24 resize-y leading-normal`}
                          />
                        </label>
                        <label className={fieldClass}>
                          Number of openings
                          <input
                            required
                            type="number"
                            min={1}
                            max={20}
                            value={roleOpenings}
                            onChange={(event) => setRoleOpenings(event.target.value)}
                            className={inputClass}
                          />
                        </label>
                        <div>
                          <button className={primaryBtn} type="submit">
                            Create opening
                          </button>
                        </div>
                      </form>
                    )}
                </section>

                <div className="flex flex-col gap-5">
                  <section className={`${cardClass} gap-3`}>
                    <h3 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                      Team
                    </h3>
                    {details.members.length ? (
                      <ul className="list-none p-0 m-0 grid gap-2">
                        {details.members.map((member) => (
                          <li
                            key={member.user_id}
                            className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl px-3.5 py-2.5"
                          >
                            {member.display_name} · {member.role}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <p className="text-sm leading-[21px] text-muted m-0">
                        No team members are listed.
                      </p>
                    )}
                  </section>

                  {manager && applications.length > 0 && (
                    <section className={`${cardClass} gap-3`}>
                      <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                        Applications ({applications.length})
                      </h2>
                      {applications.map((application) => (
                        <article
                          className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl p-5 flex flex-col gap-2.5"
                          key={application.id}
                        >
                          <h3 className="text-base font-semibold leading-[22px] m-0">
                            {application.applicant_name} · {application.status}
                          </h3>
                          <p className="text-[13px] leading-[19px] text-muted m-0">
                            {application.message}
                          </p>
                          {application.status === "pending" && (
                            <div className="flex flex-wrap gap-2.5 items-center">
                              <button
                                className={compactPrimaryBtn}
                                type="button"
                                onClick={() => void decide(application, "accepted")}
                              >
                                Accept
                              </button>
                              <button
                                className={compactSecondaryBtn}
                                type="button"
                                onClick={() => void decide(application, "rejected")}
                              >
                                Decline
                              </button>
                            </div>
                          )}
                          {application.status === "accepted" && (
                            <ConversationLaunchButton contextType="application" resourceID={application.id} />
                          )}
                        </article>
                      ))}
                    </section>
                  )}
                </div>
              </div>
            )}
          </>
        )}
      </main>
      <SiteFooter />
    </>
  );
}
