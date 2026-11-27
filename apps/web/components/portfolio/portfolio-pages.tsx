"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { fetchSession, readApiEnvelope } from "@/lib/http";

type Item = {
  project_id: string;
  caption: string;
  title: string;
  summary: string;
  visibility: string;
  lifecycle: string;
  role: string;
  source_active: boolean;
};

type Project = {
  id: string;
  title: string;
  summary: string;
  visibility: string;
  lifecycle: string;
};

type Share = {
  id: string;
  audience: "public" | "campus";
  expires_at: string;
  revoked_at: string | null;
};

type Handover = {
  id: string;
  resource_type: "project" | "organization";
  resource_id: string;
  title: string;
  from_user_id: string;
  to_user_id: string;
  state: string;
  provider_state: string;
  message: string;
  expires_at: string;
  recipient: boolean;
};

type Resource = {
  resource_type: "project" | "organization";
  resource_id: string;
  title: string;
};

type Person = { user_id: string; display_name: string; handle: string };
type CampusUser = { id: string; display_name: string; email: string; status: string };

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = path === "/api/v1/session"
    ? await fetchSession()
    : await fetch(path, {
        ...init,
        cache: "no-store",
        headers: {
          ...(init?.body ? { "Content-Type": "application/json" } : {}),
          ...(init?.headers ?? {}),
        },
      });
  const result = await readApiEnvelope<T>(response, `Request failed (${response.status}).`);
  if (!response.ok || result.error) {
    throw new Error(result.error?.message ?? `Request failed (${response.status}).`);
  }
  return result.data as T;
}

function Card({ children }: { children: React.ReactNode }) {
  return (
    <section className="min-w-0 max-w-full rounded-2xl border border-hairline bg-surface p-5 md:p-6">
      {children}
    </section>
  );
}

const fieldClass =
  "min-h-11 w-full rounded-lg border border-control bg-surface px-3.5 py-2.5 text-sm text-on-surface";
const buttonClass =
  "inline-flex min-h-11 cursor-pointer items-center justify-center rounded-lg border border-primary bg-primary px-4 py-2 text-sm font-semibold text-on-primary no-underline whitespace-nowrap hover:opacity-90 transition-opacity duration-100 disabled:cursor-not-allowed disabled:opacity-60";
const secondaryClass =
  "inline-flex min-h-10 cursor-pointer items-center justify-center rounded-lg border border-control bg-surface px-4 py-2 text-sm font-semibold text-on-surface no-underline whitespace-nowrap hover:bg-raised transition-colors duration-100 disabled:cursor-not-allowed disabled:opacity-60";

function handoverStateTone(state: string): string {
  if (state === "accepted") return "border-success/35 bg-success/10 text-success";
  if (state === "pending") return "border-warning/35 bg-warning/10 text-warning";
  if (state === "conflict" || state === "expired") return "border-danger/35 bg-danger/10 text-danger";
  return "border-hairline bg-raised text-muted";
}

function formatHandoverState(state: string): string {
  if (state === "accepted") return "Completed";
  if (!state) return "Pending";
  return state.charAt(0).toUpperCase() + state.slice(1);
}

export function PortfolioEditor() {
  const [csrf, setCSRF] = useState("");
  const [handle, setHandle] = useState("");
  const [consented, setConsented] = useState(false);
  const [items, setItems] = useState<Item[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [shares, setShares] = useState<Share[]>([]);
  const [audience, setAudience] = useState<"public" | "campus">("public");
  const [days, setDays] = useState(30);
  const [message, setMessage] = useState("");
  const [problem, setProblem] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [newURL, setNewURL] = useState("");
  const [copiedURL, setCopiedURL] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setProblem("");
    try {
      const [session, portfolio, projectList] = await Promise.all([
        api<{ csrf_token?: string; authenticated: boolean }>("/api/v1/session"),
        api<{ handle: string; consented: boolean; items: Item[]; shares: Share[] }>(
          "/api/v1/me/portfolio",
        ),
        api<{ items: Project[] }>("/api/v1/projects"),
      ]);
      if (!session.authenticated) throw new Error("Sign in to manage your portfolio.");
      setCSRF(session.csrf_token ?? "");
      setHandle(portfolio.handle);
      setConsented(portfolio.consented);
      setItems(portfolio.items);
      setShares(portfolio.shares);
      setProjects(projectList.items);
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Portfolio could not be loaded.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  function setCaption(projectID: string, caption: string) {
    setItems((current) =>
      current.map((item) => (item.project_id === projectID ? { ...item, caption } : item)),
    );
  }

  function toggleProject(project: Project, checked: boolean) {
    setItems((current) =>
      checked
        ? [
            ...current,
            {
              project_id: project.id,
              caption: "",
              title: project.title,
              summary: project.summary,
              visibility: project.visibility,
              lifecycle: project.lifecycle,
              role: "member",
              source_active: true,
            },
          ]
        : current.filter((item) => item.project_id !== project.id),
    );
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setProblem("");
    setMessage("");
    try {
      await api("/api/v1/me/portfolio/items", {
        method: "PUT",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({
          items: items.map(({ project_id, caption }) => ({ project_id, caption })),
        }),
      });
      setMessage("Portfolio selections saved.");
      await load();
    } catch (error) {
      setProblem(
        error instanceof Error ? error.message : "Portfolio selections could not be saved.",
      );
    } finally {
      setSaving(false);
    }
  }

  async function createShare() {
    setProblem("");
    setMessage("");
    setNewURL("");
    setCopiedURL(false);
    try {
      const share = await api<{ id: string; url: string }>("/api/v1/me/portfolio/shares", {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({ audience, days }),
      });
      setNewURL(`${window.location.origin}${share.url}`);
      setMessage("Share link created. Copy it now; it is only shown once.");
      await load();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Share link could not be created.");
    }
  }

  async function revoke(id: string) {
    setProblem("");
    try {
      await api(`/api/v1/me/portfolio/shares/${id}`, {
        method: "DELETE",
        headers: { "X-CSRF-Token": csrf },
      });
      setMessage("Share link revoked. Its page is unavailable now.");
      await load();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Share link could not be revoked.");
    }
  }

  async function copyNewLink() {
    if (!newURL || !navigator.clipboard) return;
    try {
      await navigator.clipboard.writeText(newURL);
      setCopiedURL(true);
    } catch {
      setCopiedURL(false);
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="mb-1.5 mt-0 text-xs font-semibold tracking-[0.02em] text-link">
            Your work, selected by you
          </p>
          <h1 className="m-0 font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em]">
            Portfolio
          </h1>
          <p className="mb-0 mt-1.5 max-w-[65ch] text-sm leading-[21px] text-muted">
            Select projects you currently belong to and write a short contribution note. Consent and
            project visibility are checked on every visit; private code and files are never exposed.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <a className={secondaryClass} href="/account/profile">
            Profile settings
          </a>
          {handle && (
            <a className={secondaryClass} href={`/u/${encodeURIComponent(handle)}`}>
              Open portfolio preview
            </a>
          )}
        </div>
      </header>

      {problem && (
        <div
          className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-danger/35 bg-danger/10 p-4 text-sm font-medium text-danger"
          role="alert"
        >
          <span>{problem}</span>
          <button className={secondaryClass} onClick={() => void load()} type="button">
            Retry
          </button>
        </div>
      )}

      {message && (
        <p
          className="m-0 rounded-xl border border-success/35 bg-success/10 p-4 text-sm font-medium text-success"
          role="status"
        >
          {message}
        </p>
      )}

      {loading && (
        <p className="m-0 text-sm text-muted" role="status">
          Loading portfolio…
        </p>
      )}

      {!loading && (
        <>
          <Card>
            <div className="mb-5 flex flex-wrap items-start justify-between gap-3">
              <div>
                <h2 className="mb-1 mt-0 text-lg font-semibold">Selected projects</h2>
                <p className="m-0 max-w-[62ch] text-sm leading-5 text-muted">
                  Only projects you still belong to can be selected. Public links show active public
                  showcases; campus links require campus visibility and active membership.
                </p>
              </div>
              <span
                className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${
                  consented
                    ? "border-success/35 bg-success/10 text-success"
                    : "border-warning/35 bg-warning/10 text-warning"
                }`}
              >
                {consented ? "Portfolio sharing enabled" : "Consent required"}
              </span>
            </div>

            {projects.length === 0 ? (
              <div className="grid justify-items-start gap-2 rounded-xl border border-dashed border-control bg-canvas p-5">
                <p className="m-0 text-sm text-muted">You do not have any available projects yet.</p>
                <a className={secondaryClass} href="/projects/import">
                  Import a project
                </a>
              </div>
            ) : (
              <form className="grid gap-4" onSubmit={save}>
                <ul className="m-0 grid list-none gap-3 p-0">
                  {projects.map((project) => {
                    const selected = items.find((item) => item.project_id === project.id);
                    return (
                      <li
                        key={project.id}
                        className={`rounded-xl border p-4 transition-colors duration-100 ${
                          selected
                            ? "border-link/40 bg-raised/45"
                            : "border-hairline bg-canvas"
                        }`}
                      >
                        <label className="flex cursor-pointer items-start gap-3">
                          <input
                            type="checkbox"
                            checked={Boolean(selected)}
                            onChange={(event) => toggleProject(project, event.target.checked)}
                            className="mt-1 h-4 w-4 shrink-0 accent-primary"
                          />
                          <span className="min-w-0 flex-1">
                            <span className="flex flex-wrap items-center justify-between gap-2">
                              <span className="font-semibold [overflow-wrap:anywhere]">
                                {project.title}
                              </span>
                              <span className="flex flex-wrap items-center gap-1.5 text-xs">
                                <span className="inline-flex items-center rounded-full border border-hairline bg-surface px-2.5 py-0.5 font-medium text-muted">
                                  {project.visibility}
                                </span>
                                <span className="inline-flex items-center rounded-full border border-hairline bg-surface px-2.5 py-0.5 font-medium text-muted">
                                  {project.lifecycle}
                                </span>
                                {selected && !selected.source_active && (
                                  <span className="inline-flex items-center rounded-full border border-warning/35 bg-warning/10 px-2.5 py-0.5 font-medium text-warning">
                                    Source inactive
                                  </span>
                                )}
                              </span>
                            </span>
                            <span className="mt-1 block text-sm leading-5 text-muted">
                              {project.summary}
                            </span>
                          </span>
                        </label>
                        {selected && (
                          <label className="mt-4 grid gap-1.5 text-sm font-medium">
                            Your contribution note
                            <textarea
                              className={`${fieldClass} min-h-24 resize-y`}
                              maxLength={500}
                              value={selected.caption}
                              onChange={(event) => setCaption(project.id, event.target.value)}
                              placeholder="Describe what you worked on, in your own words."
                            />
                          </label>
                        )}
                      </li>
                    );
                  })}
                </ul>
                <div className="flex flex-wrap items-center gap-3 pt-1">
                  <button className={buttonClass} disabled={saving} type="submit">
                    {saving ? "Saving…" : "Save selections"}
                  </button>
                  <span className="text-sm text-muted tabular-nums">
                    {items.length} of 50 selected
                  </span>
                </div>
              </form>
            )}
          </Card>

          <Card>
            <div className="flex flex-wrap items-center justify-between gap-4">
              <div>
                <h2 className="mb-1 mt-0 text-lg font-semibold">Preview and print</h2>
                <p className="m-0 max-w-[60ch] text-sm leading-5 text-muted">
                  Print directly from your browser using the live semantic view. No static PDF copy
                  is stored, so revoking consent takes effect immediately.
                </p>
              </div>
              <a className={secondaryClass} href={`/u/${encodeURIComponent(handle)}`}>
                Open portfolio preview
              </a>
            </div>
          </Card>

          <Card>
            <h2 className="mb-1 mt-0 text-lg font-semibold">Share links</h2>
            <p className="mb-4 mt-0 max-w-[62ch] text-sm leading-5 text-muted">
              Links expire within 90 days. Public links include public project presentations only;
              campus links require a verified session at your campus.
            </p>
            {!consented && (
              <p className="mb-4 mt-0 rounded-xl border border-warning/40 bg-warning/10 p-3.5 text-sm text-on-surface">
                Enable portfolio consent in{" "}
                <a className="font-semibold text-link" href="/account/profile">
                  Profile settings
                </a>{" "}
                before creating links. Revoking consent immediately disables every existing link.
              </p>
            )}
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_140px_auto] sm:items-end">
              <label className="grid gap-1.5 text-sm font-medium">
                Audience
                <select
                  className={fieldClass}
                  value={audience}
                  onChange={(e) => setAudience(e.target.value as "public" | "campus")}
                >
                  <option value="public">Anyone with the link</option>
                  <option value="campus">Verified campus members</option>
                </select>
              </label>
              <label className="grid gap-1.5 text-sm font-medium">
                Expires in
                <select
                  className={fieldClass}
                  value={days}
                  onChange={(e) => setDays(Number(e.target.value))}
                >
                  {[7, 30, 90].map((value) => (
                    <option key={value} value={value}>
                      {value} days
                    </option>
                  ))}
                </select>
              </label>
              <button
                className={buttonClass}
                type="button"
                disabled={!consented || items.length === 0}
                onClick={() => void createShare()}
              >
                Create link
              </button>
            </div>

            {newURL && (
              <div className="mt-4 grid gap-2 rounded-xl border border-link/35 bg-raised p-4">
                <label className="grid gap-1.5 text-sm font-medium" htmlFor="portfolio-new-link">
                  Copy your new link
                </label>
                <div className="flex flex-wrap gap-2">
                  <input
                    id="portfolio-new-link"
                    className={`${fieldClass} flex-1 font-mono text-xs`}
                    readOnly
                    value={newURL}
                    onFocus={(e) => e.currentTarget.select()}
                  />
                  <button
                    className={secondaryClass}
                    type="button"
                    onClick={() => void copyNewLink()}
                  >
                    {copiedURL ? "Copied" : "Copy link"}
                  </button>
                </div>
              </div>
            )}

            <ul className="mt-5 grid list-none gap-2.5 border-t border-hairline p-0 pt-4">
              {shares.length === 0 ? (
                <li className="text-sm text-muted">No share links yet.</li>
              ) : (
                shares.map((share) => (
                  <li
                    key={share.id}
                    className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-hairline bg-canvas p-3.5"
                  >
                    <div className="grid min-w-0 gap-1 text-sm">
                      <div className="flex flex-wrap items-center gap-2">
                        <strong>
                          {share.audience === "public" ? "Public" : "Campus"} link
                        </strong>
                        {share.revoked_at ? (
                          <span className="inline-flex items-center rounded-full border border-danger/35 bg-danger/10 px-2 py-0.5 text-xs font-medium text-danger">
                            Revoked
                          </span>
                        ) : (
                          <span className="inline-flex items-center rounded-full border border-success/35 bg-success/10 px-2 py-0.5 text-xs font-medium text-success">
                            Active
                          </span>
                        )}
                      </div>
                      <span className="text-xs text-muted">
                        Expires {new Date(share.expires_at).toLocaleString()}
                      </span>
                    </div>
                    {!share.revoked_at && (
                      <button
                        className={secondaryClass}
                        type="button"
                        onClick={() => void revoke(share.id)}
                      >
                        Revoke
                      </button>
                    )}
                  </li>
                ))
              )}
            </ul>
          </Card>
        </>
      )}
    </div>
  );
}

type PublicData = {
  handle: string;
  display_name: string;
  headline: string;
  bio: string;
  audience: string;
  items: Array<{
    project_id: string;
    title: string;
    summary: string;
    caption: string;
    role: string;
  }>;
};

export function PublicPortfolio({ handle, token }: { handle?: string; token?: string }) {
  const [data, setData] = useState<PublicData | null>(null);
  const [problem, setProblem] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    const path = token
      ? `/api/v1/portfolio/share/${encodeURIComponent(token)}`
      : `/api/v1/portfolio/${encodeURIComponent(handle ?? "")}`;
    void api<PublicData>(path)
      .then((value) => {
        if (active) setData(value);
      })
      .catch((error: unknown) => {
        if (active) setProblem(error instanceof Error ? error.message : "Portfolio is unavailable.");
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [handle, token]);

  return (
    <div className="mx-auto flex w-[min(1240px,calc(100%-32px))] flex-1 flex-col gap-6 py-8 md:w-[min(1240px,calc(100%-64px))] md:py-10 print:w-full print:py-0">
      <div className="print:hidden flex flex-wrap items-center justify-between gap-3">
        <a className="text-sm font-medium text-link no-underline hover:underline" href="/">
          ← Back to PeerGit
        </a>
        <div className="flex flex-wrap items-center gap-2">
          <button
            className={secondaryClass}
            type="button"
            disabled={!data}
            onClick={() => window.print()}
          >
            Print or save as PDF
          </button>
        </div>
      </div>

      {loading && (
        <p className="m-0 text-sm text-muted" role="status">
          Loading portfolio…
        </p>
      )}

      {problem && !loading && (
        <Card>
          <h1 className="mb-2 mt-0 font-display text-2xl font-semibold">Portfolio unavailable</h1>
          <p className="m-0 max-w-[60ch] text-sm leading-6 text-muted">
            The owner may have withdrawn portfolio consent, hidden the selected work, or revoked
            this link. {problem}
          </p>
        </Card>
      )}

      {data && (
        <article className="flex flex-col gap-6 bg-canvas text-on-surface print:bg-white print:text-black">
          <header className="rounded-2xl border border-hairline bg-surface p-6 md:p-8 print:rounded-none print:border-x-0 print:border-t-0 print:border-b print:bg-white print:p-0 print:pb-4">
            <div className="flex flex-wrap items-start gap-5">
              <div
                className="grid h-[72px] w-[72px] shrink-0 place-items-center rounded-full border border-hairline bg-raised font-display text-2xl font-semibold text-on-surface print:hidden"
                aria-hidden="true"
              >
                {(data.display_name || data.handle).slice(0, 2).toUpperCase()}
              </div>
              <div className="grid min-w-0 flex-1 gap-2">
                <div className="flex flex-wrap items-center gap-2.5">
                  <h1 className="m-0 font-display text-[clamp(1.75rem,3vw,2.25rem)] font-semibold leading-tight tracking-[-0.015em]">
                    {data.display_name}
                  </h1>
                  <span className="font-mono text-sm text-muted print:text-black">
                    @{data.handle}
                  </span>
                  <span
                    className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium print:border-black print:text-black ${
                      data.audience === "campus"
                        ? "border-link/40 bg-link/10 text-link"
                        : "border-success/40 bg-success/10 text-success"
                    }`}
                  >
                    {data.audience === "campus"
                      ? "Verified campus portfolio"
                      : "Publicly shared"}
                  </span>
                </div>
                {data.headline && (
                  <p className="m-0 text-base font-medium leading-6 text-on-surface print:text-black">
                    {data.headline}
                  </p>
                )}
                {data.bio && (
                  <p className="m-0 max-w-3xl text-sm leading-6 text-muted print:text-black">
                    {data.bio}
                  </p>
                )}
              </div>
            </div>
          </header>

          <section aria-labelledby="selected-work-heading" className="grid gap-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h2 id="selected-work-heading" className="m-0 font-display text-2xl font-semibold">
                Selected work
              </h2>
              <span className="text-xs font-medium text-muted tabular-nums print:text-black">
                {data.items.length} {data.items.length === 1 ? "project" : "projects"}
              </span>
            </div>

            {data.items.length === 0 ? (
              <p className="m-0 rounded-2xl border border-dashed border-control bg-surface p-6 text-sm text-muted">
                No selected work is currently visible to this audience.
              </p>
            ) : (
              <div className="grid gap-4">
                {data.items.map((item) => (
                  <section
                    key={item.project_id}
                    className="break-inside-avoid grid gap-3 rounded-2xl border border-hairline bg-surface p-5 md:p-6 print:rounded-none print:border-hairline print:bg-white"
                  >
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <h3 className="m-0 text-lg font-semibold leading-6">
                        <a
                          className="text-on-surface no-underline hover:text-link hover:underline print:text-black"
                          href={
                            data.audience === "campus"
                              ? `/projects/${encodeURIComponent(item.project_id)}`
                              : `/showcase/${encodeURIComponent(item.project_id)}`
                          }
                        >
                          {item.title}
                        </a>
                      </h3>
                      <span className="inline-flex items-center rounded-full border border-hairline bg-raised px-2.5 py-0.5 text-xs font-medium text-on-surface print:border-black print:bg-white print:text-black">
                        PeerGit role: {item.role}
                      </span>
                    </div>
                    <p className="m-0 text-sm leading-6 text-muted print:text-black">
                      {item.summary}
                    </p>
                    {item.caption && (
                      <p className="m-0 rounded-r-lg border-l-2 border-l-link bg-raised/55 px-3.5 py-2 text-sm leading-6 text-on-surface print:border-l-black print:bg-white print:text-black">
                        {item.caption}
                      </p>
                    )}
                  </section>
                ))}
              </div>
            )}
          </section>

          <p className="m-0 border-t border-hairline pt-4 text-xs leading-5 text-muted print:border-black print:text-black">
            This view contains selected project presentation details only. Private source code,
            repository snapshots, and restricted project records are excluded.
          </p>
        </article>
      )}
    </div>
  );
}

export function SuccessionManager({ initialResource }: { initialResource?: string }) {
  const [csrf, setCSRF] = useState("");
  const [resources, setResources] = useState<Resource[]>([]);
  const [selected, setSelected] = useState("");
  const [people, setPeople] = useState<Person[]>([]);
  const [recipient, setRecipient] = useState("");
  const [handovers, setHandovers] = useState<Handover[]>([]);
  const [campusAdmin, setCampusAdmin] = useState(false);
  const [mfaFresh, setMfaFresh] = useState(false);
  const [campusUsers, setCampusUsers] = useState<CampusUser[]>([]);
  const [memberToAdd, setMemberToAdd] = useState("");
  const [message, setMessage] = useState("");
  const [problem, setProblem] = useState("");
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setProblem("");
    try {
      const [session, owned, requests] = await Promise.all([
        api<{
          csrf_token?: string;
          authenticated: boolean;
          user?: { campus_admin?: boolean; mfa_verified?: boolean };
        }>("/api/v1/session"),
        api<{ items: Resource[] }>("/api/v1/me/handoverable"),
        api<{ items: Handover[] }>("/api/v1/me/handovers"),
      ]);
      if (!session.authenticated) throw new Error("Sign in to manage project handovers.");
      setCSRF(session.csrf_token ?? "");
      setCampusAdmin(Boolean(session.user?.campus_admin));
      setMfaFresh(Boolean(session.user?.mfa_verified));
      setResources(owned.items);
      setHandovers(requests.items);
      const firstOwned = owned.items[0]
        ? `${owned.items[0].resource_type}:${owned.items[0].resource_id}`
        : "";
      setSelected((current) => current || initialResource || firstOwned);
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Handover details could not be loaded.");
    } finally {
      setLoading(false);
    }
  }, [initialResource]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    const [kind, id] = selected.split(":");
    if (!kind || !id || (kind !== "project" && kind !== "organization")) {
      setPeople([]);
      return;
    }
    let active = true;
    void api<{ items: Person[] }>(
      `/api/v1/${kind === "project" ? "projects" : "organizations"}/${encodeURIComponent(id)}/handover/eligible`,
    )
      .then((value) => {
        if (active) setPeople(value.items);
      })
      .catch((error: unknown) => {
        if (active) {
          setProblem(
            error instanceof Error ? error.message : "Eligible successors could not be loaded.",
          );
        }
      });
    return () => {
      active = false;
    };
  }, [selected]);

  useEffect(() => {
    if (!campusAdmin || !mfaFresh || !selected.startsWith("organization:")) {
      setCampusUsers([]);
      return;
    }
    let active = true;
    void api<{ items: CampusUser[] }>("/api/v1/admin/users")
      .then((value) => {
        if (active) setCampusUsers(value.items.filter((user) => user.status === "active"));
      })
      .catch((error: unknown) => {
        if (active) {
          setProblem(
            error instanceof Error ? error.message : "Campus members could not be loaded.",
          );
        }
      });
    return () => {
      active = false;
    };
  }, [campusAdmin, mfaFresh, selected]);

  async function propose(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const [kind, id] = selected.split(":");
    if (!kind || !id || !recipient) return;
    setProblem("");
    setMessage("");
    try {
      await api(`/api/v1/${kind === "project" ? "projects" : "organizations"}/${id}/handover`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({
          to_user_id: recipient,
          message: "Please take over and keep this work moving.",
        }),
      });
      setMessage("Handover proposed. The new owner must accept within seven days.");
      await load();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Handover could not be proposed.");
    }
  }

  async function decide(item: Handover, action: "accept" | "decline" | "cancel") {
    setProblem("");
    setMessage("");
    try {
      await api(`/api/v1/handovers/${item.id}/${action}`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
        body: "{}",
      });
      setMessage(
        action === "accept"
          ? "Ownership transferred in PeerGit."
          : action === "cancel"
            ? "Handover request cancelled."
            : "Handover request declined.",
      );
      await load();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : `Handover could not be ${action}ed.`);
      await load();
    }
  }

  async function addOrganizationMember(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const organizationID = selected.split(":")[1];
    if (!organizationID || !memberToAdd) return;
    setProblem("");
    setMessage("");
    try {
      await api(`/api/v1/admin/organizations/${organizationID}/members`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({ user_id: memberToAdd, role: "member" }),
      });
      const eligible = await api<{ items: Person[] }>(
        `/api/v1/organizations/${organizationID}/handover/eligible`,
      );
      setPeople(eligible.items);
      setMemberToAdd("");
      setMessage(
        "Verified campus member added. They can now accept this organization's handover.",
      );
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Campus member could not be added.");
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="mb-1.5 mt-0 text-xs font-semibold tracking-[0.02em] text-link">
            Keep the project moving
          </p>
          <h1 className="m-0 font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em]">
            Project succession
          </h1>
          <p className="mb-0 mt-1.5 max-w-[65ch] text-sm leading-[21px] text-muted">
            Transfer PeerGit ownership to a current verified project or organization member ahead of
            graduation or departure. GitHub repository ownership remains with its GitHub account.
          </p>
        </div>
        <button className={secondaryClass} onClick={() => void load()} type="button">
          Refresh
        </button>
      </header>

      <section
        className="grid gap-3 rounded-2xl border border-hairline bg-raised p-4 sm:grid-cols-3"
        aria-label="Current responsibilities"
      >
        <div>
          <h2 className="m-0 text-xs font-semibold text-on-surface">Fresh sign-in or MFA</h2>
          <p className="mt-1 mb-0 text-xs leading-5 text-muted">
            Ownership proposals check current session authentication and campus membership.
          </p>
        </div>
        <div>
          <h2 className="m-0 text-xs font-semibold text-on-surface">Maintainer continuity</h2>
          <p className="mt-1 mb-0 text-xs leading-5 text-muted">
            After acceptance, the outgoing owner stays on as a maintainer unless removed later.
          </p>
        </div>
        <div>
          <h2 className="m-0 text-xs font-semibold text-on-surface">Repository boundary</h2>
          <p className="mt-1 mb-0 text-xs leading-5 text-muted">
            PeerGit transfers cleanly in one transaction; upstream GitHub settings stay separate.
          </p>
        </div>
      </section>

      {problem && (
        <div
          className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-danger/35 bg-danger/10 p-4 text-sm font-medium text-danger"
          role="alert"
        >
          <span>{problem}</span>
          <button className={secondaryClass} onClick={() => void load()} type="button">
            Retry
          </button>
        </div>
      )}

      {message && (
        <p
          className="m-0 rounded-xl border border-success/35 bg-success/10 p-4 text-sm font-medium text-success"
          role="status"
        >
          {message}
        </p>
      )}

      {loading && (
        <p className="m-0 text-sm text-muted" role="status">
          Loading handovers…
        </p>
      )}

      {!loading && (
        <>
          <Card>
            <h2 className="mb-1 mt-0 text-lg font-semibold">Propose a handover</h2>
            <p className="mb-4 mt-0 text-sm leading-5 text-muted">
              The invitation expires after seven days and checks eligibility again upon acceptance.
            </p>
            {resources.length === 0 ? (
              <p className="m-0 text-sm text-muted">
                You do not own an eligible project or organization yet.
              </p>
            ) : (
              <form className="grid gap-4" onSubmit={propose}>
                <label className="grid gap-1.5 text-sm font-medium">
                  Project or organization
                  <select
                    className={fieldClass}
                    value={selected}
                    onChange={(e) => {
                      setSelected(e.target.value);
                      setRecipient("");
                    }}
                  >
                    {resources.map((resource) => (
                      <option
                        key={`${resource.resource_type}:${resource.resource_id}`}
                        value={`${resource.resource_type}:${resource.resource_id}`}
                      >
                        {resource.title} ({resource.resource_type})
                      </option>
                    ))}
                  </select>
                </label>
                <label className="grid gap-1.5 text-sm font-medium">
                  Eligible successor
                  <select
                    className={fieldClass}
                    value={recipient}
                    onChange={(e) => setRecipient(e.target.value)}
                    required
                  >
                    <option value="">Choose a current member</option>
                    {people.map((person) => (
                      <option key={person.user_id} value={person.user_id}>
                        {person.display_name} (@{person.handle})
                      </option>
                    ))}
                  </select>
                </label>
                {people.length === 0 && (
                  <p className="m-0 text-sm text-muted">
                    Add a verified campus member to this organization or project before proposing
                    succession.
                  </p>
                )}
                <div>
                  <button className={buttonClass} disabled={!recipient} type="submit">
                    Send proposal
                  </button>
                </div>
              </form>
            )}

            {selected.startsWith("organization:") && campusAdmin && mfaFresh && (
              <form
                className="mt-5 grid gap-3 border-t border-hairline pt-5 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end"
                onSubmit={addOrganizationMember}
              >
                <label className="grid gap-1.5 text-sm font-medium">
                  Add a verified campus member
                  <select
                    className={fieldClass}
                    value={memberToAdd}
                    onChange={(e) => setMemberToAdd(e.target.value)}
                    required
                  >
                    <option value="">Choose a campus account</option>
                    {campusUsers.map((user) => (
                      <option key={user.id} value={user.id}>
                        {user.display_name} · {user.email}
                      </option>
                    ))}
                  </select>
                </label>
                <button className={secondaryClass} type="submit" disabled={!memberToAdd}>
                  Add member
                </button>
              </form>
            )}
          </Card>

          <Card>
            <h2 className="mb-4 mt-0 text-lg font-semibold">Handover requests</h2>
            {handovers.length === 0 ? (
              <p className="m-0 text-sm text-muted">No handover requests yet.</p>
            ) : (
              <ul className="m-0 grid list-none gap-3 p-0">
                {handovers.map((item) => (
                  <li key={item.id} className="rounded-xl border border-hairline bg-canvas p-4">
                    <div className="flex flex-wrap items-start justify-between gap-4">
                      <div className="grid min-w-0 flex-1 gap-2">
                        <div className="flex flex-wrap items-center gap-2">
                          <h3 className="m-0 text-base font-semibold [overflow-wrap:anywhere]">
                            {item.title}
                          </h3>
                          <span
                            className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${handoverStateTone(item.state)}`}
                          >
                            {formatHandoverState(item.state)}
                          </span>
                          <span className="inline-flex items-center rounded-full border border-hairline bg-surface px-2.5 py-0.5 text-xs font-medium text-muted">
                            {item.resource_type}
                          </span>
                        </div>
                        {item.message && (
                          <p className="m-0 text-sm leading-5 text-on-surface">{item.message}</p>
                        )}
                        <p
                          className={`m-0 rounded-lg border px-3 py-2 text-xs leading-5 ${
                            item.provider_state === "source_unavailable"
                              ? "border-warning/35 bg-warning/10 text-on-surface"
                              : "border-hairline bg-surface text-muted"
                          }`}
                        >
                          {item.provider_state === "upstream_unchanged" &&
                            "PeerGit ownership changes; the GitHub repository remains under its existing GitHub owner."}
                          {item.provider_state === "source_unavailable" &&
                            "Repository source reconciliation unavailable. Reconnect it in GitHub settings to restore source access."}
                          {item.provider_state === "not_applicable" &&
                            "This handover does not change a third-party repository."}
                        </p>
                        <span className="text-xs text-muted">
                          Expires {new Date(item.expires_at).toLocaleString()}
                        </span>
                      </div>

                      {item.state === "pending" && (
                        <div className="flex flex-wrap gap-2">
                          {item.recipient ? (
                            <button
                              className={buttonClass}
                              type="button"
                              onClick={() => void decide(item, "accept")}
                            >
                              Accept
                            </button>
                          ) : null}
                          <button
                            className={secondaryClass}
                            type="button"
                            onClick={() => void decide(item, item.recipient ? "decline" : "cancel")}
                          >
                            {item.recipient ? "Decline" : "Cancel request"}
                          </button>
                        </div>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </>
      )}
    </div>
  );
}
