"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";

type Envelope<T> = { data?: T; error?: { message?: string } };
type Binding = {
  id: string;
  full_name: string;
  html_url: string;
  description: string;
  default_branch: string;
  topics: string[];
  languages: Record<string, number>;
  readme_markdown: string;
  access_state: string;
  sync_health: string;
  last_synced_at?: string | null;
  limitations: string;
  source_access?: boolean;
  provider?: string;
};
type Contribution = {
  id: string;
  kind: string;
  canonical_id: string;
  title: string;
  summary: string;
  author_name: string;
  author_login: string;
  occurred_at?: string | null;
  number?: number;
  state?: string;
  observed_at?: string | null;
  provider_updated_at?: string | null;
};
type Snapshot = {
  id: string;
  ref: string;
  commit_sha: string;
  state: string;
  receipt_at: string;
  archive_sha256?: string | null;
  archive_bytes?: number | null;
  failure_code?: string | null;
  retry_available?: boolean;
};
type Data = {
  repository: Binding | null;
  contributions: Contribution[];
  snapshots: Snapshot[];
};

export function RepositoryOverview({
  projectID,
  csrf,
  tab = "code",
}: {
  projectID: string;
  csrf: string;
  tab?: string;
}) {
  const [data, setData] = useState<Data>({
    repository: null,
    contributions: [],
    snapshots: [],
  });
  const [ref, setRef] = useState("main");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");
  const [diff, setDiff] = useState<{
    sha: string;
    files: { filename: string; patch?: string; status: string }[];
  } | null>(null);
  const api = useCallback(
    async <T,>(path: string, init?: RequestInit): Promise<T> => {
      const headers = new Headers(init?.headers);
      if (init?.body) headers.set("Content-Type", "application/json");
      if (init?.method && init.method !== "GET")
        headers.set("X-CSRF-Token", csrf);
      const r = await fetch(path, {
        ...init,
        headers,
        cache: "no-store",
        credentials: "same-origin",
      });
      const v =
        r.status === 204
          ? { data: undefined }
          : ((await r.json()) as Envelope<T>);
      if (!r.ok)
        throw new Error(v.error?.message ?? "Repository request failed.");
      return v.data as T;
    },
    [csrf],
  );
  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      setData(await api<Data>(`/api/v1/projects/${projectID}/repository`));
      setProblem("");
    } catch (e) {
      setData({ repository: null, contributions: [], snapshots: [] });
      setDiff(null);
      setProblem(
        e instanceof Error
          ? e.message
          : "Repository evidence could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [api, projectID]);
  useEffect(() => {
    void refresh();
  }, [refresh]);
  const [history, setHistory] = useState<Contribution[] | null>(null);
  const [historyProblem, setHistoryProblem] = useState("");
  useEffect(() => {
    if (tab !== "activity") return;
    const controller = new AbortController();
    const ref = new URLSearchParams(location.search).get("ref");
    const query = ref ? "?" + new URLSearchParams({ ref }) : "";
    type Entry = {
      sha: string;
      commit: { message: string; author: { name: string; date: string } };
    };
    void api<{ items: Entry[] }>(
      "/api/v1/projects/" + projectID + "/repository/history" + query,
      { signal: controller.signal },
    )
      .then((result) => {
        if (controller.signal.aborted) return;
        setHistory(
          result.items.map((item) => ({
            id: item.sha,
            kind: "commit",
            canonical_id: item.sha,
            title: item.commit.message.split("\n")[0],
            summary: "",
            author_name: item.commit.author.name,
            author_login: "",
            occurred_at: item.commit.author.date,
          })),
        );
      })
      .catch(() => {
        if (!controller.signal.aborted)
          setHistoryProblem(
            "Live branch history is unavailable. Showing synchronized commit evidence.",
          );
      });
    return () => controller.abort();
  }, [api, projectID, tab]);
  const commits =
    history ?? data.contributions.filter((item) => item.kind === "commit");
  const [selection, setSelection] = useState("");
  useEffect(() => {
    setSelection(
      new URLSearchParams(location.search).get(
        tab === "activity" ? "commit" : "item",
      ) ?? "",
    );
  }, [tab]);
  async function capture(e: FormEvent) {
    e.preventDefault();
    if (!data.repository) return;
    setBusy(true);
    setProblem("");
    try {
      await api(`/api/v1/projects/${projectID}/repository/snapshots`, {
        method: "POST",
        headers: { "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify({ binding_id: data.repository.id, ref }),
      });
      setMessage("Exact commit receipt recorded; archive capture is running.");
      await refresh();
    } catch (x) {
      setProblem(x instanceof Error ? x.message : "Snapshot request failed.");
    } finally {
      setBusy(false);
    }
  }
  async function retry(id: string) {
    setBusy(true);
    try {
      await api(
        `/api/v1/projects/${projectID}/repository/snapshots/${id}/retry`,
        { method: "POST", body: "{}" },
      );
      setMessage("Snapshot retry queued for the same commit.");
      await refresh();
    } catch (e) {
      setProblem(e instanceof Error ? e.message : "Retry failed.");
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    if (tab !== "activity" || !selection || !/^[a-f0-9]{40}$/.test(selection))
      return;
    const controller = new AbortController();
    void api<NonNullable<typeof diff>>(
      `/api/v1/projects/${projectID}/repository/commit?sha=${selection}`,
      { signal: controller.signal },
    )
      .then((value) => {
        if (!controller.signal.aborted) setDiff(value);
      })
      .catch((e) => {
        if (!controller.signal.aborted)
          setProblem(
            e instanceof Error ? e.message : "Commit diff could not be loaded.",
          );
      });
    return () => controller.abort();
  }, [api, projectID, selection, tab]);
  const base = `/projects/${encodeURIComponent(projectID)}`;
  const providerURL =
    /^https:\/\/github\.com\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+\/?$/.test(
      data.repository?.html_url ?? "",
    )
      ? data.repository?.html_url.replace(/\/$/, "")
      : undefined;
  return (
    <section className="repository-panel" aria-label="Repository evidence">
      {tab === "code" && (
        <div className="repository-heading">
          <div>
            <p className="eyebrow">GitHub project</p>
            <h2 id="repository-overview-title">Code</h2>
          </div>
          <button
            className="secondary-button"
            type="button"
            disabled={loading || busy}
            onClick={() => void refresh()}
          >
            Refresh
          </button>
        </div>
      )}
      {problem && (
        <p className="form-error" role="alert">
          {problem}
        </p>
      )}
      {message && (
        <p className="form-success" role="status">
          {message}
        </p>
      )}
      {loading ? (
        <p role="status">Loading repository…</p>
      ) : !data.repository ? (
        <div className="repository-empty">
          <p>This older project has no connected repository yet.</p>
          <a className="primary-link" href="/account/github">
            Connect GitHub and import a repository
          </a>
        </div>
      ) : (
        <>
          {tab === "code" && (
            <>
              <h3>
                {providerURL ? (
                  <a href={providerURL} target="_blank" rel="noreferrer">
                    {data.repository.full_name} ↗
                  </a>
                ) : (
                  data.repository.full_name
                )}
              </h3>
              <p>
                {data.repository.description || "No repository description."}
              </p>
              <p>
                {data.repository.access_state.replaceAll("_", " ")} · sync{" "}
                {data.repository.sync_health.replaceAll("_", " ")}
                {data.repository.last_synced_at
                  ? ` · ${new Date(data.repository.last_synced_at).toLocaleString()}`
                  : ""}
              </p>
              <p>
                Source access:{" "}
                {data.repository.source_access === false
                  ? "unavailable"
                  : "authorized members"}{" "}
                · Default branch: {data.repository.default_branch || "not set"}
              </p>
              {data.repository.topics.length > 0 && (
                <ul className="skill-list">
                  {data.repository.topics.map((t) => (
                    <li key={t}>{t}</li>
                  ))}
                </ul>
              )}
              {Object.keys(data.repository.languages).length > 0 && (
                <p>
                  Languages: {Object.keys(data.repository.languages).join(", ")}
                </p>
              )}
              <p className="repository-limits">{data.repository.limitations}</p>
            </>
          )}
          {tab === "activity" && (
            <>
              <h2>Commit activity</h2>
              <div className="button-row">
                <a href={`${base}?tab=issues`}>Observed issues</a>
                <a href={`${base}?tab=pull-requests`}>Observed pull requests</a>
              </div>
              <p className="muted">
                Commit records are observations from the connected GitHub
                repository. Unmatched authors stay unlinked.
              </p>
              {historyProblem && <p role="status">{historyProblem}</p>}
              {commits.length ? (
                <ul className="repository-activity">
                  {commits.map((c) => (
                    <li key={c.id}>
                      <div>
                        <button
                          className="text-button"
                          type="button"
                          onClick={() =>
                            location.assign(
                              `${base}?${new URLSearchParams({ tab: "activity", commit: c.canonical_id.replace(/^git:sha1:/, ""), ...(new URLSearchParams(location.search).get("ref") && { ref: new URLSearchParams(location.search).get("ref")! }) })}`,
                            )
                          }
                        >
                          {c.title || c.canonical_id}
                        </button>
                        <p>
                          {c.author_name ||
                            c.author_login ||
                            "Unlinked GitHub account"}
                        </p>
                        {c.summary && <p>{c.summary}</p>}
                      </div>
                      <time>
                        {c.occurred_at
                          ? new Date(c.occurred_at).toLocaleString()
                          : "Date unavailable"}
                      </time>
                    </li>
                  ))}
                </ul>
              ) : (
                <p>Commit evidence has not synced yet.</p>
              )}
              {diff && (
                <section className="account-form">
                  <h3>Commit {diff.sha.slice(0, 12)}</h3>
                  <a href={`${base}?tab=code&sha=${diff.sha}`}>
                    Browse this revision
                  </a>
                  {diff.files.map((file) => (
                    <article key={file.filename}>
                      <h4>
                        {file.filename} · {file.status}
                      </h4>
                      <pre className="readme-text">
                        {file.patch ||
                          "Diff omitted by GitHub for a binary or large file."}
                      </pre>
                    </article>
                  ))}
                </section>
              )}
            </>
          )}
          {(tab === "issues" || tab === "pull-requests") &&
            (() => {
              const isIssue = tab === "issues";
              const records = data.contributions.filter(
                (item) => item.kind === (isIssue ? "issue" : "pull_request"),
              );
              const selected = records.find((item) => item.id === selection);
              const label = isIssue ? "Issues" : "Pull requests";
              return (
                <>
                  <div className="repository-heading">
                    <h2>{label}</h2>
                    {providerURL && (
                      <a
                        className="secondary-button"
                        href={providerURL + (isIssue ? "/issues" : "/pulls")}
                      >
                        Open on GitHub ↗
                      </a>
                    )}
                  </div>
                  <p className="muted">
                    PeerGit displays synchronized summaries. Open GitHub for the
                    complete record and to make changes.
                  </p>
                  {selection && !selected && (
                    <p role="status">
                      This record is unavailable in the current observations.
                    </p>
                  )}
                  <div className="split-layout">
                    <nav className="list-rail" aria-label={label}>
                      {records.map((item) => (
                        <a
                          key={item.id}
                          className="list-item"
                          aria-current={
                            selected?.id === item.id ? "true" : undefined
                          }
                          href={
                            base +
                            "?" +
                            new URLSearchParams({ tab, item: item.id })
                          }
                        >
                          {item.number ? "#" + item.number + " " : ""}
                          {item.title || "Title unavailable"}
                          <small>{item.state || "State unavailable"}</small>
                        </a>
                      ))}
                      {!records.length && (
                        <p>No synchronized summaries available.</p>
                      )}
                    </nav>
                    {selected && (
                      <article className="panel">
                        <p className="eyebrow">
                          {selected.state || "State unavailable"}
                        </p>
                        <h3>{selected.title || "Title unavailable"}</h3>
                        <p className="record-summary">
                          {selected.summary || "Summary unavailable."}
                        </p>
                        <p className="muted">
                          Observed{" "}
                          {selected.observed_at
                            ? new Date(selected.observed_at).toLocaleString()
                            : "at an unknown time"}
                          .
                        </p>
                        {providerURL && selected.number && (
                          <a
                            href={
                              providerURL +
                              (isIssue ? "/issues/" : "/pull/") +
                              selected.number
                            }
                          >
                            Open record on GitHub ↗
                          </a>
                        )}
                      </article>
                    )}
                  </div>
                </>
              );
            })()}
          {tab === "security" && (
            <>
              <p className="eyebrow">Access and source protection</p>
              <h2>Security</h2>
              <div className="notice">
                PeerGit does not run hosted Actions, source scans or dependency
                alerts. Source remains governed by GitHub access. Project
                publication never changes repository visibility.
              </div>
              <div className="feature-grid" style={{ marginTop: "1rem" }}>
                <article className="feature-card">
                  <h3>Provider authority</h3>
                  <p>
                    {data.repository?.full_name ?? "Repository"} ·{" "}
                    {data.repository?.sync_health?.replaceAll("_", " ") ??
                      "status unavailable"}
                  </p>
                  <p className="muted">
                    Last observed{" "}
                    {data.repository?.last_synced_at
                      ? new Date(
                          data.repository.last_synced_at,
                        ).toLocaleString()
                      : "time unavailable"}
                    .
                  </p>
                </article>
                <article className="feature-card">
                  <h3>Project audience</h3>
                  <p>
                    PeerGit’s project presentation has its own visibility
                    setting. Private source and snapshots stay limited to
                    authorized project members.
                  </p>
                </article>
                <article className="feature-card">
                  <h3>Repository controls</h3>
                  <p>
                    Default branch protection and repository credentials are
                    managed with GitHub and your account connection.
                  </p>
                  <a href="/account/github">Review GitHub connection →</a>
                </article>
              </div>
            </>
          )}
          {tab === "snapshots" && (
            <>
              <h2>Immutable snapshots</h2>
              <form className="repository-capture" onSubmit={capture}>
                <label htmlFor="snapshot-ref">
                  Record a snapshot of a branch, tag, or commit
                </label>
                <div>
                  <input
                    id="snapshot-ref"
                    value={ref}
                    onChange={(e) => setRef(e.target.value)}
                    maxLength={255}
                    required
                  />
                  <button
                    className="primary-button"
                    disabled={busy || data.repository.access_state !== "active"}
                  >
                    Capture
                  </button>
                </div>
              </form>
              {data.snapshots.length ? (
                <ul className="repository-snapshots">
                  {data.snapshots.map((s) => (
                    <li key={s.id}>
                      <div>
                        <strong>{s.state.replaceAll("_", " ")}</strong>
                        <p>
                          {s.ref} → <code>{s.commit_sha}</code> ·{" "}
                          {new Date(s.receipt_at).toLocaleString()}
                          {s.failure_code ? ` · ${s.failure_code}` : ""}
                        </p>
                        {s.archive_sha256 && (
                          <p className="record-summary">
                            Archive SHA-256 <code>{s.archive_sha256}</code>
                          </p>
                        )}
                      </div>
                      <div className="snapshot-actions">
                        {s.state === "verified" &&
                          data.repository?.source_access !== false && (
                            <a
                              className="secondary-button"
                              href={`/api/v1/projects/${projectID}/repository/snapshots/${s.id}/download`}
                            >
                              Download
                            </a>
                          )}
                        {s.retry_available && (
                          <button
                            className="secondary-button"
                            type="button"
                            onClick={() => void retry(s.id)}
                            disabled={busy}
                          >
                            Retry same SHA
                          </button>
                        )}
                      </div>
                    </li>
                  ))}
                </ul>
              ) : (
                <p>No snapshots yet.</p>
              )}
            </>
          )}
        </>
      )}
    </section>
  );
}
