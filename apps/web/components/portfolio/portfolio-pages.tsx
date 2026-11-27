"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { readApiEnvelope } from "@/lib/http";

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
  const response = await fetch(path, {
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
    <section className="min-w-0 rounded-2xl border border-hairline bg-surface p-5 md:p-6">
      {children}
    </section>
  );
}

const fieldClass =
  "min-h-11 w-full rounded-lg border border-control bg-surface px-3.5 py-2.5 text-sm text-on-surface";
const buttonClass =
  "inline-flex min-h-11 items-center justify-center rounded-lg border border-primary bg-primary px-4 py-2 text-sm font-semibold text-on-primary hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60";
const secondaryClass =
  "inline-flex min-h-11 items-center justify-center rounded-lg border border-control bg-surface px-4 py-2 text-sm font-semibold text-on-surface hover:bg-raised disabled:cursor-not-allowed disabled:opacity-60";

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

  const load = useCallback(async () => {
    setLoading(true);
    setProblem("");
    try {
      const [session, portfolio, projectList] = await Promise.all([
        api<{ csrf_token?: string; authenticated: boolean }>("/api/v1/session"),
        api<{ handle: string; consented: boolean; items: Item[]; shares: Share[] }>("/api/v1/me/portfolio"),
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
      setProblem(error instanceof Error ? error.message : "Portfolio selections could not be saved.");
    } finally {
      setSaving(false);
    }
  }

  async function createShare() {
    setProblem("");
    setMessage("");
    setNewURL("");
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

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="mb-1 text-sm font-semibold text-link">Your work, selected by you</p>
          <h1 className="m-0 font-display text-3xl font-semibold leading-tight">Portfolio</h1>
        </div>
        <a className="text-sm font-medium text-link" href="/account/profile">
          Profile settings
        </a>
      </header>

      <p className="m-0 max-w-3xl text-sm leading-6 text-muted">
        Pick projects you currently belong to and add a short note about your role. PeerGit checks
        current portfolio consent and project visibility every time someone opens a page or share
        link. Private code, snapshots and repository files are never included.
      </p>

      {problem && <p className="m-0 rounded-lg border border-danger/40 bg-danger/10 p-3 text-sm text-danger" role="alert">{problem}</p>}
      {message && <p className="m-0 rounded-lg border border-success/40 bg-success/10 p-3 text-sm text-success" role="status">{message}</p>}
      {loading ? <p role="status">Loading portfolio…</p> : null}

      {!loading && (
        <>
          <Card>
            <h2 className="mb-1 mt-0 text-lg font-semibold">Selected projects</h2>
            <p className="mb-5 mt-0 text-sm leading-5 text-muted">
              Only projects you still belong to can be selected. Public visitors see only active
              public showcases. Campus links also require campus visibility and current membership.
            </p>
            {projects.length === 0 ? (
              <p className="m-0 text-sm text-muted">You do not have any available projects yet.</p>
            ) : (
              <form className="grid gap-4" onSubmit={save}>
                <ul className="m-0 grid list-none gap-3 p-0">
                  {projects.map((project) => {
                    const selected = items.find((item) => item.project_id === project.id);
                    return (
                      <li key={project.id} className="rounded-xl border border-hairline bg-canvas p-4">
                        <label className="flex cursor-pointer items-start gap-3">
                          <input
                            type="checkbox"
                            checked={Boolean(selected)}
                            onChange={(event) => toggleProject(project, event.target.checked)}
                            className="mt-1 h-4 w-4 accent-primary"
                          />
                          <span className="min-w-0 flex-1">
                            <span className="block font-semibold">{project.title}</span>
                            <span className="mt-1 block text-sm leading-5 text-muted">
                              {project.summary}
                            </span>
                            <span className="mt-2 block text-xs text-muted">
                              {project.visibility} visibility · {project.lifecycle}
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
                <div className="flex flex-wrap items-center gap-3">
                  <button className={buttonClass} disabled={saving} type="submit">
                    {saving ? "Saving…" : "Save selections"}
                  </button>
                  <span className="text-sm text-muted">{items.length} of 50 selected</span>
                </div>
              </form>
            )}
          </Card>

          <Card>
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h2 className="mb-1 mt-0 text-lg font-semibold">Preview and print</h2>
                <p className="m-0 text-sm leading-5 text-muted">
                  Print is generated by your browser from the same semantic page. PeerGit does not
                  keep a PDF copy that could outlive consent or visibility changes.
                </p>
              </div>
              <a className={secondaryClass} href={`/u/${encodeURIComponent(handle)}`}>Open portfolio preview</a>
            </div>
          </Card>

          <Card>
            <h2 className="mb-1 mt-0 text-lg font-semibold">Share links</h2>
            <p className="mb-4 mt-0 text-sm leading-5 text-muted">
              Links expire within 90 days. A public link contains only current public project
              presentations. A campus link requires a verified account at your campus.
            </p>
            {!consented && (
              <p className="mb-4 mt-0 rounded-lg border border-warning/40 bg-warning/10 p-3 text-sm text-on-surface">
                Enable portfolio consent in Profile settings before creating links. Revoking consent
                immediately disables every existing link.
              </p>
            )}
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_140px_auto] sm:items-end">
              <label className="grid gap-1.5 text-sm font-medium">
                Audience
                <select className={fieldClass} value={audience} onChange={(e) => setAudience(e.target.value as "public" | "campus")}>
                  <option value="public">Anyone with the link</option>
                  <option value="campus">Verified campus members</option>
                </select>
              </label>
              <label className="grid gap-1.5 text-sm font-medium">
                Expires in
                <select className={fieldClass} value={days} onChange={(e) => setDays(Number(e.target.value))}>
                  {[7, 30, 90].map((value) => <option key={value} value={value}>{value} days</option>)}
                </select>
              </label>
              <button className={buttonClass} type="button" disabled={!consented || items.length === 0} onClick={() => void createShare()}>
                Create link
              </button>
            </div>
            {newURL && (
              <label className="mt-4 grid gap-1.5 text-sm font-medium">
                Copy your new link
                <input className={fieldClass} readOnly value={newURL} onFocus={(e) => e.currentTarget.select()} />
              </label>
            )}
            <ul className="mt-5 grid list-none gap-2 border-t border-hairline p-0 pt-4">
              {shares.length === 0 ? <li className="text-sm text-muted">No share links yet.</li> : shares.map((share) => (
                <li key={share.id} className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-canvas p-3">
                  <span className="text-sm">
                    <strong>{share.audience === "public" ? "Public" : "Campus"} link</strong>
                    <span className="block text-xs text-muted">Expires {new Date(share.expires_at).toLocaleString()}</span>
                    {share.revoked_at && <span className="block text-xs text-danger">Revoked</span>}
                  </span>
                  {!share.revoked_at && <button className={secondaryClass} type="button" onClick={() => void revoke(share.id)}>Revoke</button>}
                </li>
              ))}
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
    <div className="mx-auto flex w-[min(1240px,calc(100%-32px))] flex-1 flex-col gap-6 py-8 md:w-[min(1240px,calc(100%-64px))] md:py-10">
      <div className="print:hidden flex flex-wrap items-center justify-between gap-3">
        <a className="text-sm font-medium text-link" href="/">PeerGit</a>
        <button className={secondaryClass} type="button" disabled={!data} onClick={() => window.print()}>
          Print or save as PDF
        </button>
      </div>
      {loading ? <p role="status">Loading portfolio…</p> : null}
      {problem && !loading ? (
        <Card>
          <h1 className="mb-2 mt-0 font-display text-2xl font-semibold">Portfolio unavailable</h1>
          <p className="m-0 text-sm leading-6 text-muted">
            The owner may have withdrawn portfolio consent, hidden the selected work, or revoked
            this link. {problem}
          </p>
        </Card>
      ) : null}
      {data && (
        <article className="flex flex-col gap-6 bg-canvas text-on-surface print:bg-white print:text-black">
          <header className="rounded-2xl border border-hairline bg-surface p-6 md:p-8 print:border-b print:border-x-0 print:border-t-0 print:rounded-none print:bg-white print:p-0">
            <p className="mb-2 text-sm font-semibold text-link print:text-black">PeerGit portfolio</p>
            <h1 className="m-0 font-display text-3xl font-semibold leading-tight md:text-4xl">
              {data.display_name}
            </h1>
            <p className="mb-0 mt-2 text-sm text-muted print:text-black">@{data.handle}</p>
            {data.headline && <p className="mb-0 mt-4 text-lg leading-7">{data.headline}</p>}
            {data.bio && <p className="mb-0 mt-2 max-w-3xl text-sm leading-6">{data.bio}</p>}
            <p className="mb-0 mt-4 text-xs text-muted print:text-black">
              {data.audience === "campus" ? "Shared with verified campus members" : "Publicly shared by the owner"}
            </p>
          </header>
          <section aria-labelledby="selected-work-heading" className="grid gap-4">
            <h2 id="selected-work-heading" className="m-0 font-display text-2xl font-semibold">Selected work</h2>
            {data.items.length === 0 ? (
              <p className="m-0 text-sm text-muted">No selected work is currently visible to this audience.</p>
            ) : data.items.map((item) => (
              <section key={item.project_id} className="break-inside-avoid rounded-2xl border border-hairline bg-surface p-5 md:p-6 print:border-hairline print:rounded-none print:bg-white">
                <h3 className="mb-2 mt-0 text-lg font-semibold">{item.title}</h3>
                <p className="mb-2 mt-0 text-sm leading-6">{item.summary}</p>
                <p className="mb-0 mt-0 text-sm text-muted print:text-black">PeerGit role: {item.role}</p>
                {item.caption && <p className="mb-0 mt-3 text-sm leading-6">{item.caption}</p>}
              </section>
            ))}
          </section>
          <p className="m-0 text-xs leading-5 text-muted print:text-black">
            This print view contains selected project presentation details only. Private source code,
            repository snapshots and restricted project data are not included.
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
        api<{ csrf_token?: string; authenticated: boolean; user?: { campus_admin?: boolean; mfa_verified?: boolean } }>("/api/v1/session"),
        api<{ items: Resource[] }>("/api/v1/me/handoverable"),
        api<{ items: Handover[] }>("/api/v1/me/handovers"),
      ]);
      if (!session.authenticated) throw new Error("Sign in to manage project handovers.");
      setCSRF(session.csrf_token ?? "");
      setCampusAdmin(Boolean(session.user?.campus_admin));
      setMfaFresh(Boolean(session.user?.mfa_verified));
      setResources(owned.items);
      setHandovers(requests.items);
      setSelected((current) => current || initialResource || owned.items[0]?.resource_type + ":" + owned.items[0]?.resource_id || "");
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
    if (!id) {
      setPeople([]);
      return;
    }
    let active = true;
    void api<{ items: Person[] }>(`/api/v1/${kind === "project" ? "projects" : "organizations"}/${encodeURIComponent(id)}/handover/eligible`)
      .then((value) => {
        if (active) setPeople(value.items);
      })
      .catch((error: unknown) => {
        if (active) setProblem(error instanceof Error ? error.message : "Eligible successors could not be loaded.");
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
        if (active) setProblem(error instanceof Error ? error.message : "Campus members could not be loaded.");
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
        body: JSON.stringify({ to_user_id: recipient, message: "Please take over and keep this work moving." }),
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
      const eligible = await api<{ items: Person[] }>(`/api/v1/organizations/${organizationID}/handover/eligible`);
      setPeople(eligible.items);
      setMemberToAdd("");
      setMessage("Verified campus member added. They can now accept this organization's handover.");
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Campus member could not be added.");
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <header>
        <p className="mb-1 text-sm font-semibold text-link">Keep the project moving</p>
        <h1 className="m-0 font-display text-3xl font-semibold leading-tight">Project succession</h1>
      </header>
      <p className="m-0 max-w-3xl text-sm leading-6 text-muted">
        Choose a current project member or organization member. Proposals require a recent sign-in
        or fresh MFA; acceptance checks membership again and changes ownership in one transaction.
        GitHub repository ownership remains with its GitHub account.
      </p>
      {problem && <p className="m-0 rounded-lg border border-danger/40 bg-danger/10 p-3 text-sm text-danger" role="alert">{problem}</p>}
      {message && <p className="m-0 rounded-lg border border-success/40 bg-success/10 p-3 text-sm text-success" role="status">{message}</p>}
      {loading ? <p role="status">Loading handovers…</p> : null}
      {!loading && (
        <>
          <Card>
            <h2 className="mb-1 mt-0 text-lg font-semibold">Propose a handover</h2>
            <p className="mb-4 mt-0 text-sm leading-5 text-muted">
              The current owner keeps access as a maintainer. The invitation expires after seven days.
            </p>
            {resources.length === 0 ? (
              <p className="m-0 text-sm text-muted">You do not own an eligible project or organization yet.</p>
            ) : (
              <form className="grid gap-4" onSubmit={propose}>
                <label className="grid gap-1.5 text-sm font-medium">
                  Project or organization
                  <select className={fieldClass} value={selected} onChange={(e) => { setSelected(e.target.value); setRecipient(""); }}>
                    {resources.map((resource) => (
                      <option key={`${resource.resource_type}:${resource.resource_id}`} value={`${resource.resource_type}:${resource.resource_id}`}>
                        {resource.title} ({resource.resource_type})
                      </option>
                    ))}
                  </select>
                </label>
                <label className="grid gap-1.5 text-sm font-medium">
                  Eligible successor
                  <select className={fieldClass} value={recipient} onChange={(e) => setRecipient(e.target.value)} required>
                    <option value="">Choose a current member</option>
                    {people.map((person) => <option key={person.user_id} value={person.user_id}>{person.display_name} (@{person.handle})</option>)}
                  </select>
                </label>
                {people.length === 0 && <p className="m-0 text-sm text-muted">Add a verified campus member to this organization or project before proposing succession.</p>}
                <button className={buttonClass} disabled={!recipient} type="submit">Send proposal</button>
              </form>
            )}
            {selected.startsWith("organization:") && campusAdmin && mfaFresh && (
              <form className="mt-5 grid gap-3 border-t border-hairline pt-5 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end" onSubmit={addOrganizationMember}>
                <label className="grid gap-1.5 text-sm font-medium">
                  Add a verified campus member
                  <select className={fieldClass} value={memberToAdd} onChange={(e) => setMemberToAdd(e.target.value)} required>
                    <option value="">Choose a campus account</option>
                    {campusUsers.map((user) => <option key={user.id} value={user.id}>{user.display_name} · {user.email}</option>)}
                  </select>
                </label>
                <button className={secondaryClass} type="submit" disabled={!memberToAdd}>Add member</button>
              </form>
            )}
          </Card>

          <Card>
            <h2 className="mb-4 mt-0 text-lg font-semibold">Handover requests</h2>
            {handovers.length === 0 ? <p className="m-0 text-sm text-muted">No handover requests yet.</p> : (
              <ul className="m-0 grid list-none gap-3 p-0">
                {handovers.map((item) => (
                  <li key={item.id} className="rounded-xl border border-hairline bg-canvas p-4">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <h3 className="m-0 font-semibold">{item.title}</h3>
                        <p className="mb-0 mt-1 text-sm text-muted">{item.resource_type} · {item.state}</p>
                        {item.message && <p className="mb-0 mt-2 text-sm leading-5">{item.message}</p>}
                        <p className="mb-0 mt-2 text-xs leading-5 text-muted">
                          {item.provider_state === "upstream_unchanged" && "PeerGit ownership changes; the GitHub repository remains under its existing GitHub owner."}
                          {item.provider_state === "source_unavailable" && "The repository source is currently unavailable. Reconnect it in GitHub settings to restore access."}
                          {item.provider_state === "not_applicable" && "This handover does not change a third-party repository."}
                        </p>
                      </div>
                      {item.state === "pending" && (
                        <div className="flex flex-wrap gap-2">
                          {item.recipient ? <button className={buttonClass} type="button" onClick={() => void decide(item, "accept")}>Accept</button> : null}
                          <button className={secondaryClass} type="button" onClick={() => void decide(item, item.recipient ? "decline" : "cancel")}>{item.recipient ? "Decline" : "Cancel request"}</button>
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
