"use client";

import { FormEvent, useEffect, useState } from "react";
import { RepositoryOverview } from "./repository-overview";
import { RepositoryCode } from "./repository-code";
import { LegacyRepositoryLink } from "./legacy-repository-link";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";

type Envelope<T> = { data?: T; error?: { message?: string } };
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
    const result = (await response.json()) as Envelope<{ items: Role[] }>;
    setRoles(result.data?.items ?? []);
  }

  async function loadApplications() {
    const response = await fetch(`/api/v1/projects/${projectID}/applications`, {
      cache: "no-store",
    });
    if (!response.ok) return;
    const result = (await response.json()) as Envelope<{ items: Application[] }>;
    setApplications(result.data?.items ?? []);
  }

  useEffect(() => {
    let active = true;
    void Promise.all([
      fetch(`/api/v1/projects/${projectID}`, { cache: "no-store" }).then(
        async (response) => {
          const result = (await response.json()) as Envelope<Details>;
          if (!response.ok || !result.data)
            throw new Error(result.error?.message ?? "Project could not be loaded.");
          return result.data;
        },
      ),
      fetch("/api/v1/session", { cache: "no-store" })
        .then((response) => response.json())
        .then(
          (result: {
            data?: {
              csrf_token?: string;
              user?: {
                college_id?: string;
                terms_accepted?: boolean;
                privacy_accepted?: boolean;
              };
            };
          }) => result.data,
        ),
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
          const response = await fetch(
            `/api/v1/projects/${projectID}/repository`,
            { cache: "no-store" },
          );
          if (response.ok) {
            const result = (await response.json()) as Envelope<{
              repository: RepositoryAccess | null;
            }>;
            if (active && result.data) {
              setRepository(result.data.repository);
              setRepositoryChecked(true);
            }
          } else if (active) {
            setProblem("Repository access could not be confirmed.");
          }
        }
        if (ready) await loadRoles();
        if (["owner", "maintainer"].includes(project.viewer_role))
          await loadApplications();
      })
      .catch(
        (error) =>
          active &&
          setProblem(
            error instanceof Error ? error.message : "Project could not be loaded.",
          ),
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
    const result = (await response.json()) as Envelope<{ version?: number }>;
    if (!response.ok)
      throw new Error(result.error?.message ?? "The change could not be saved.");
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

  const manager = Boolean(
    details && ["owner", "maintainer"].includes(details.viewer_role),
  );

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

  const providerURL =
    repository?.html_url &&
    /^https:\/\/github\.com\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+\/?$/.test(
      repository.html_url,
    )
      ? repository.html_url.replace(/\/$/, "")
      : undefined;

  return (
    <>
      <a className="skip-link" href="#project-content">
        Skip to content
      </a>
      <SiteHeader workspace />

      {details && (
        <section className="workspace-header-band">
          <div className="workspace-header-inner">
            <div className="repo-title-row">
              <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
                <p className="t-label-sm muted" style={{ margin: 0 }}>
                  Projects / {details.project.title} · {details.project.project_type.replaceAll("_", " ")}
                </p>
                <h1 className="t-headline-md">{details.project.title}</h1>
                <p className="t-body-md muted" style={{ margin: 0, maxWidth: "60ch" }}>
                  {details.project.summary}
                </p>
                <div style={{ display: "flex", flexWrap: "wrap", gap: "8px", paddingTop: "6px" }}>
                  {repository && (
                    <span className="t-mono-sm" style={{ alignSelf: "center" }}>
                      GitHub · {repository.full_name}
                    </span>
                  )}
                  <span className="pg-pill pg-pill-raised">
                    Project visibility: {details.project.visibility}
                  </span>
                  {repository && (
                    <span className="pg-pill pg-pill-raised">
                      Source access:{" "}
                      {repository.source_access === false
                        ? repository.access_state.replaceAll("_", " ")
                        : "authorized project members"}
                    </span>
                  )}
                </div>
              </div>

              <div className="button-row">
                <span className="pg-pill pg-pill-success">
                  {details.project.lifecycle}
                  {repository ? ` · ${repository.sync_health.replaceAll("_", " ")}` : ""}
                </span>
                {providerURL && (
                  <a
                    className="primary-link pg-btn-compact"
                    href={providerURL}
                    target="_blank"
                    rel="noreferrer"
                  >
                    Open on GitHub ↗
                  </a>
                )}
              </div>
            </div>

            <nav className="project-tabs" aria-label="Project sections">
              <div className="project-tabs-group">
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
                    href={`/projects/${projectID}?tab=${value}`}
                    aria-current={tab === value ? "page" : undefined}
                  >
                    {label}
                  </a>
                ))}
              </div>
              <div className="project-tabs-secondary">
                <a
                  href={`/projects/${projectID}?tab=open-roles`}
                  aria-current={tab === "open-roles" ? "page" : undefined}
                >
                  Open roles
                </a>
                {details.viewer_role && hasRepository && (
                  <a
                    href={`/projects/${projectID}?tab=snapshots`}
                    aria-current={tab === "snapshots" ? "page" : undefined}
                  >
                    Snapshots
                  </a>
                )}
                <a href={`/projects/${projectID}/work`}>Project work</a>
              </div>
            </nav>
          </div>
        </section>
      )}

      <main id="project-content" className="project-detail-page" style={{ flex: 1 }}>
        {problem && (
          <p className="form-error" role="alert" style={{ margin: 0 }}>
            {problem}
          </p>
        )}
        {message && (
          <p className="form-success" role="status" style={{ margin: 0 }}>
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
              <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
                <h2 className="t-title-lg">Project overview</h2>
                <p className="t-body-md" style={{ margin: 0 }}>
                  {details.project.summary}
                </p>
                <p className="t-body-sm muted" style={{ margin: 0 }}>
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
                <p role="status" className="pg-callout">
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

            {tab === "activity" && details.viewer_role && hasRepository && (
              <RepositoryOverview
                projectID={projectID}
                csrf={csrf}
                tab="activity"
                viewerRole={details.viewer_role}
              />
            )}

            {tab === "issues" && details.viewer_role && hasRepository && (
              <RepositoryOverview
                projectID={projectID}
                csrf={csrf}
                tab="issues"
                viewerRole={details.viewer_role}
              />
            )}

            {tab === "pull-requests" && details.viewer_role && hasRepository && (
              <RepositoryOverview
                projectID={projectID}
                csrf={csrf}
                tab="pull-requests"
                viewerRole={details.viewer_role}
              />
            )}

            {tab === "security" && details.viewer_role && hasRepository && (
              <RepositoryOverview
                projectID={projectID}
                csrf={csrf}
                tab="security"
                viewerRole={details.viewer_role}
              />
            )}

            {tab === "snapshots" && details.viewer_role && hasRepository && (
              <RepositoryOverview
                projectID={projectID}
                csrf={csrf}
                tab="snapshots"
                viewerRole={details.viewer_role}
              />
            )}

            {tab === "settings" && manager && (
              <div className="pg-grid-2" style={{ alignItems: "start" }}>
                <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
                  <div>
                    <h2 className="t-title-lg">Project details</h2>
                    <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                      PeerGit visibility: <strong>{details.project.visibility}</strong> ·
                      lifecycle: {details.project.lifecycle}
                    </p>
                  </div>
                  <form
                    className="account-form"
                    style={{ marginTop: 0, paddingTop: 0, borderTop: 0 }}
                    onSubmit={(event) => void saveDetails(event)}
                  >
                    <label>
                      Summary
                      <textarea
                        required
                        maxLength={500}
                        value={summary}
                        onChange={(event) => setSummary(event.target.value)}
                      />
                    </label>
                    <label>
                      Description
                      <textarea
                        maxLength={12000}
                        rows={6}
                        value={description}
                        onChange={(event) => setDescription(event.target.value)}
                      />
                    </label>
                    <div>
                      <button className="primary-button" type="submit">
                        Save details
                      </button>
                    </div>
                  </form>
                </section>

                <div style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
                  <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "14px" }}>
                    <h3 className="t-title-lg">Project publication &amp; visibility</h3>
                    <p className="t-body-sm muted" style={{ margin: 0 }}>
                      Project publication shares its PeerGit presentation. GitHub visibility does
                      not change, and private source, evidence, and snapshots remain restricted to
                      project members.
                    </p>
                    <div className="button-row">
                      {hasRepository && details.project.lifecycle !== "archived" && (
                        <>
                          <button
                            className="secondary-button pg-btn-compact"
                            type="button"
                            onClick={() => void publish("campus")}
                          >
                            Publish to campus
                          </button>
                          <button
                            className="secondary-button pg-btn-compact"
                            type="button"
                            onClick={() => void publish("public")}
                          >
                            Publish publicly
                          </button>
                        </>
                      )}
                      {details.project.visibility !== "private" && (
                        <button
                          className="secondary-button pg-btn-compact"
                          type="button"
                          onClick={() => void publish("private")}
                        >
                          Make private
                        </button>
                      )}
                    </div>
                  </section>

                  <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "10px" }}>
                    <h3 className="t-title-md">GitHub connection</h3>
                    <p className="t-body-sm muted" style={{ margin: 0 }}>
                      Repository access follows the connected GitHub App installation. Manage or
                      revoke your account connection from{" "}
                      <a href="/account/github">GitHub connections</a>.
                    </p>
                  </section>

                  {details.project.lifecycle !== "archived" && (
                    <section
                      className="pg-card"
                      style={{
                        borderColor: "var(--danger)",
                        display: "flex",
                        flexWrap: "wrap",
                        alignItems: "center",
                        justifyContent: "space-between",
                        gap: "12px",
                      }}
                    >
                      <div>
                        <h3 className="t-title-sm" style={{ color: "var(--danger)" }}>
                          Archive project
                        </h3>
                        <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                          Makes this project read-only and removes it from discovery.
                        </p>
                      </div>
                      <button
                        className="danger-button pg-btn-compact"
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
              <div className="pg-grid-2" style={{ alignItems: "start" }}>
                <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "18px" }}>
                  <div>
                    <h2 className="t-title-lg">Open roles</h2>
                    {!campusAccess && (
                      <p className="t-body-sm muted" style={{ margin: "6px 0 0" }}>
                        <a href="/onboarding">Verify campus access</a> to view openings and apply.
                      </p>
                    )}
                  </div>

                  {roles.filter((role) => role.status === "open").length ? (
                    <div style={{ display: "grid", gap: "14px" }}>
                      {roles
                        .filter((role) => role.status === "open")
                        .map((role) => (
                          <article className="pg-card-canvas" key={role.id} style={{ display: "flex", flexDirection: "column", gap: "10px" }}>
                            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "8px" }}>
                              <h3 className="t-title-sm">{role.title}</h3>
                              <span className="t-label-sm" style={{ color: "var(--success)" }}>
                                {role.openings} opening(s)
                              </span>
                            </div>
                            <p className="t-body-sm muted" style={{ margin: 0 }}>
                              {role.description}
                            </p>
                            <p className="t-label-sm muted" style={{ margin: 0 }}>
                              Difficulty: {role.difficulty}
                              {role.good_first_task ? " · beginner-friendly" : ""}
                            </p>
                            {campusAccess &&
                              details.project.lifecycle === "active" &&
                              !manager && (
                                <form
                                  className="account-form"
                                  style={{ marginTop: "6px", paddingTop: "12px" }}
                                  onSubmit={(event) => {
                                    event.preventDefault();
                                    void apply(role);
                                  }}
                                >
                                  <label>
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
                                    />
                                  </label>
                                  <div>
                                    <button className="primary-button pg-btn-compact" type="submit">
                                      Apply
                                    </button>
                                  </div>
                                </form>
                              )}
                          </article>
                        ))}
                    </div>
                  ) : (
                    <p className="t-body-md muted" style={{ margin: 0 }}>
                      This project has no open roles.
                    </p>
                  )}

                  {manager &&
                    hasRepository &&
                    details.project.lifecycle === "active" && (
                      <form
                        className="account-form project-role-form"
                        onSubmit={(event) => void createRole(event)}
                      >
                        <h3 className="t-title-md">Open a team role</h3>
                        <label>
                          Role title
                          <input
                            required
                            minLength={2}
                            maxLength={100}
                            value={roleTitle}
                            onChange={(event) => setRoleTitle(event.target.value)}
                          />
                        </label>
                        <label>
                          What will the member work on?
                          <textarea
                            required
                            maxLength={3000}
                            value={roleDescription}
                            onChange={(event) => setRoleDescription(event.target.value)}
                          />
                        </label>
                        <label>
                          Number of openings
                          <input
                            required
                            type="number"
                            min={1}
                            max={20}
                            value={roleOpenings}
                            onChange={(event) => setRoleOpenings(event.target.value)}
                          />
                        </label>
                        <div>
                          <button className="primary-button" type="submit">
                            Create opening
                          </button>
                        </div>
                      </form>
                    )}
                </section>

                <div style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
                  <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
                    <h3 className="t-title-md">Team</h3>
                    {details.members.length ? (
                      <ul style={{ listStyle: "none", padding: 0, margin: 0, display: "grid", gap: "8px" }}>
                        {details.members.map((member) => (
                          <li
                            key={member.user_id}
                            className="pg-card-canvas"
                            style={{ padding: "10px 14px" }}
                          >
                            {member.display_name} · {member.role}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <p className="t-body-md muted" style={{ margin: 0 }}>
                        No team members are listed.
                      </p>
                    )}
                  </section>

                  {manager && applications.length > 0 && (
                    <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
                      <h2 className="t-title-md">Applications ({applications.length})</h2>
                      {applications.map((application) => (
                        <article
                          className="pg-card-canvas"
                          key={application.id}
                          style={{ display: "flex", flexDirection: "column", gap: "10px" }}
                        >
                          <h3 className="t-title-sm">
                            {application.applicant_name} · {application.status}
                          </h3>
                          <p className="t-body-sm muted" style={{ margin: 0 }}>
                            {application.message}
                          </p>
                          {application.status === "pending" && (
                            <div className="button-row">
                              <button
                                className="primary-button pg-btn-compact"
                                type="button"
                                onClick={() => void decide(application, "accepted")}
                              >
                                Accept
                              </button>
                              <button
                                className="secondary-button pg-btn-compact"
                                type="button"
                                onClick={() => void decide(application, "rejected")}
                              >
                                Decline
                              </button>
                            </div>
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
