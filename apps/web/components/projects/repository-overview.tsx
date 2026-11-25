"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { PreviewBadge } from "@/components/layout/preview-banner";

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

function renderDiffLines(patch: string) {
  return patch.split("\n").map((line, index) => {
    if (line.startsWith("+") && !line.startsWith("+++")) {
      return (
        <span key={index} className="diff-line-add" aria-label="Added line">
          {line}
        </span>
      );
    }
    if (line.startsWith("-") && !line.startsWith("---")) {
      return (
        <span key={index} className="diff-line-del" aria-label="Removed line">
          {line}
        </span>
      );
    }
    if (line.startsWith("@@")) {
      return (
        <span key={index} className="diff-line-hunk">
          {line}
        </span>
      );
    }
    return <span key={index} style={{ display: "block" }}>{line}</span>;
  });
}

export function RepositoryOverview({
  projectID,
  csrf,
  tab = "code",
  viewerRole = "",
}: {
  projectID: string;
  csrf: string;
  tab?: string;
  viewerRole?: string;
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
      if (init?.method && init.method !== "GET") headers.set("X-CSRF-Token", csrf);
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
      if (!r.ok) throw new Error(v.error?.message ?? "Repository request failed.");
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
        e instanceof Error ? e.message : "Repository evidence could not be loaded.",
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
    const branchRef = new URLSearchParams(location.search).get("ref");
    const query = branchRef ? "?" + new URLSearchParams({ ref: branchRef }) : "";
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
    <section
      className="repository-panel"
      aria-label="Repository evidence"
      style={{ display: "flex", flexDirection: "column", gap: "20px" }}
    >
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
            <div
              className="pg-card"
              style={{
                display: "flex",
                flexWrap: "wrap",
                alignItems: "center",
                justifyContent: "space-between",
                gap: "16px",
              }}
            >
              <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
                <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: "10px" }}>
                  <strong className="t-title-sm">
                    {providerURL ? (
                      <a href={providerURL} target="_blank" rel="noreferrer">
                        {data.repository.full_name} ↗
                      </a>
                    ) : (
                      data.repository.full_name
                    )}
                  </strong>
                  <span className="t-body-sm muted">
                    {data.repository.access_state.replaceAll("_", " ")} · sync{" "}
                    {data.repository.sync_health.replaceAll("_", " ")}
                    {data.repository.last_synced_at
                      ? ` · ${new Date(data.repository.last_synced_at).toLocaleString()}`
                      : ""}
                  </span>
                </div>
                <p className="t-body-sm muted" style={{ margin: 0 }}>
                  {data.repository.description || "No repository description."} · Default branch:{" "}
                  <code className="t-mono-sm">{data.repository.default_branch || "not set"}</code>
                  {Object.keys(data.repository.languages).length > 0
                    ? ` · Languages: ${Object.keys(data.repository.languages).join(", ")}`
                    : ""}
                </p>
                {data.repository.limitations && (
                  <p className="t-body-sm muted" style={{ margin: 0 }}>
                    {data.repository.limitations}
                  </p>
                )}
              </div>
              <button
                className="secondary-button pg-btn-compact"
                type="button"
                disabled={loading || busy}
                onClick={() => void refresh()}
              >
                Refresh
              </button>
            </div>
          )}

          {tab === "activity" && (
            <div style={{ display: "flex", flexDirection: "column", gap: "18px" }}>
              <div
                style={{
                  display: "flex",
                  flexWrap: "wrap",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: "12px",
                }}
              >
                <div>
                  <h2 className="t-title-lg">Commit activity</h2>
                  <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                    Commit records are observations from the connected GitHub repository. Unmatched
                    authors stay unlinked.
                  </p>
                </div>
                <div className="button-row">
                  <a className="secondary-button pg-btn-compact" href={`${base}?tab=issues`}>
                    Observed issues
                  </a>
                  <a
                    className="secondary-button pg-btn-compact"
                    href={`${base}?tab=pull-requests`}
                  >
                    Observed pull requests
                  </a>
                </div>
              </div>

              {historyProblem && (
                <p className="pg-callout" role="status">
                  {historyProblem}
                </p>
              )}

              <div className="split-layout">
                <div className="split-list-panel">
                  <div
                    style={{
                      padding: "12px 16px",
                      borderBottom: "1px solid var(--hairline)",
                      display: "flex",
                      justifyContent: "space-between",
                      alignItems: "center",
                    }}
                  >
                    <span className="t-title-sm">Commits</span>
                    <span className="t-label-sm muted">{commits.length} observed</span>
                  </div>
                  {commits.length ? (
                    <div className="list-rail">
                      {commits.map((c) => {
                        const sha = c.canonical_id.replace(/^git:sha1:/, "");
                        return (
                          <button
                            key={c.id}
                            type="button"
                            className="list-item"
                            aria-current={selection === sha ? "true" : undefined}
                            onClick={() =>
                              location.assign(
                                `${base}?${new URLSearchParams({
                                  tab: "activity",
                                  commit: sha,
                                  ...(new URLSearchParams(location.search).get("ref") && {
                                    ref: new URLSearchParams(location.search).get("ref")!,
                                  }),
                                })}`,
                              )
                            }
                            style={{ cursor: "pointer", borderRight: 0, borderTop: 0 }}
                          >
                            <strong className="t-body-md" style={{ display: "block" }}>
                              {c.title || c.canonical_id}
                            </strong>
                            <small>
                              {c.author_name || c.author_login || "Unlinked GitHub account"} ·{" "}
                              <code className="t-mono-sm">{sha.slice(0, 7)}</code> ·{" "}
                              {c.occurred_at
                                ? new Date(c.occurred_at).toLocaleDateString()
                                : "Date unavailable"}
                            </small>
                          </button>
                        );
                      })}
                    </div>
                  ) : (
                    <p className="t-body-md muted" style={{ padding: "16px", margin: 0 }}>
                      Commit evidence has not synced yet.
                    </p>
                  )}
                </div>

                <div style={{ minWidth: 0, display: "flex", flexDirection: "column", gap: "16px" }}>
                  {diff ? (
                    <>
                      <div
                        className="pg-card"
                        style={{
                          display: "flex",
                          flexWrap: "wrap",
                          alignItems: "center",
                          justifyContent: "space-between",
                          gap: "12px",
                        }}
                      >
                        <div>
                          <h3 className="t-title-md">Commit {diff.sha.slice(0, 12)}</h3>
                          <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                            {diff.files.length} {diff.files.length === 1 ? "file" : "files"} changed
                          </p>
                        </div>
                        <a
                          className="secondary-button pg-btn-compact"
                          href={`${base}?tab=code&sha=${diff.sha}`}
                        >
                          Browse tree at SHA
                        </a>
                      </div>

                      {diff.files.map((file) => (
                        <article key={file.filename} className="code-file-box">
                          <div className="code-file-header">
                            <code className="t-mono-sm" style={{ fontWeight: 600 }}>
                              {file.filename}
                            </code>
                            <span className="t-label-sm muted">{file.status} · Unified diff</span>
                          </div>
                          <pre className="code-pre">
                            {file.patch
                              ? renderDiffLines(file.patch)
                              : "Diff omitted by GitHub for a binary or large file."}
                          </pre>
                        </article>
                      ))}
                    </>
                  ) : (
                    <div className="pg-card">
                      <p className="t-body-md muted" style={{ margin: 0 }}>
                        Select a commit from the history list to inspect changed files and diffs.
                      </p>
                    </div>
                  )}
                </div>
              </div>
            </div>
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
                <div style={{ display: "flex", flexDirection: "column", gap: "18px" }}>
                  <div className="section-heading" style={{ marginBottom: 0 }}>
                    <div>
                      <h2 className="t-title-lg">{label}</h2>
                      <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                        PeerGit displays synchronized summaries. Open GitHub for the complete record
                        and to make changes.
                      </p>
                    </div>
                    {providerURL && (
                      <a
                        className="secondary-button pg-btn-compact"
                        href={providerURL + (isIssue ? "/issues" : "/pulls")}
                      >
                        Open on GitHub ↗
                      </a>
                    )}
                  </div>

                  {selection && !selected && (
                    <p className="pg-callout" role="status">
                      This record is unavailable in the current observations.
                    </p>
                  )}

                  <div className="split-layout">
                    <nav className="split-list-panel list-rail" aria-label={label}>
                      {records.map((item) => (
                        <a
                          key={item.id}
                          className="list-item"
                          aria-current={selected?.id === item.id ? "true" : undefined}
                          href={base + "?" + new URLSearchParams({ tab, item: item.id })}
                        >
                          <div style={{ display: "flex", justifyContent: "space-between", gap: "8px", marginBottom: "4px" }}>
                            <span className="t-mono-sm muted">
                              {item.number ? `#${item.number}` : ""}
                            </span>
                            <span className="pg-pill pg-pill-success">
                              {item.state || "State unavailable"}
                            </span>
                          </div>
                          <strong className="t-title-sm" style={{ display: "block" }}>
                            {item.title || "Title unavailable"}
                          </strong>
                        </a>
                      ))}
                      {!records.length && (
                        <p className="t-body-md muted" style={{ padding: "16px", margin: 0 }}>
                          No synchronized summaries available.
                        </p>
                      )}
                    </nav>

                    <div style={{ minWidth: 0 }}>
                      {selected ? (
                        <article className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
                          <div
                            style={{
                              display: "flex",
                              flexWrap: "wrap",
                              alignItems: "flex-start",
                              justifyContent: "space-between",
                              gap: "12px",
                              paddingBottom: "14px",
                              borderBottom: "1px solid var(--hairline)",
                            }}
                          >
                            <div>
                              <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: "8px" }}>
                                <span className="pg-pill pg-pill-success">
                                  {selected.state || "State unavailable"} · GitHub{" "}
                                  {isIssue ? "Issue" : "PR"}
                                  {selected.number ? ` #${selected.number}` : ""}
                                </span>
                                <span className="t-label-sm muted">
                                  {isIssue
                                    ? "Repository issue (distinct from Project tasks)"
                                    : "Synchronized pull request observation"}
                                </span>
                              </div>
                              <h3 className="t-title-lg" style={{ marginTop: "8px" }}>
                                {selected.title || "Title unavailable"}
                              </h3>
                            </div>
                            {providerURL && selected.number && (
                              <a
                                className="secondary-button pg-btn-compact"
                                href={
                                  providerURL +
                                  (isIssue ? "/issues/" : "/pull/") +
                                  selected.number
                                }
                              >
                                Open record on GitHub ↗
                              </a>
                            )}
                          </div>

                          <p className="t-body-md record-summary" style={{ margin: 0 }}>
                            {selected.summary || "Summary unavailable."}
                          </p>
                          <p className="t-body-sm muted" style={{ margin: 0 }}>
                            Observed{" "}
                            {selected.observed_at
                              ? new Date(selected.observed_at).toLocaleString()
                              : "at an unknown time"}
                            .
                          </p>

                          <div className="pg-callout" style={{ flexDirection: "column", gap: "6px" }}>
                            <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: "8px", width: "100%" }}>
                              <strong className="t-label-md" style={{ color: "var(--link)" }}>
                                {isIssue
                                  ? "Qualified Pilot Preview (Native Issue Composer)"
                                  : "Qualified Pilot Preview (Native PR Review & Merge)"}
                              </strong>
                              <PreviewBadge phase="Stage B1" />
                            </div>
                            <span>
                              {isIssue
                                ? "For PeerGit-hosted repositories, team members can reply, label, and close/reopen issues directly here."
                                : "Native Conversation, Commits, Changes diff, and branch-protected merge rules activate in Stage B1."}
                            </span>
                          </div>
                        </article>
                      ) : (
                        <div className="pg-card">
                          <p className="t-body-md muted" style={{ margin: 0 }}>
                            Select an item from the list to inspect its synchronized summary and
                            upstream GitHub link.
                          </p>
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              );
            })()}

          {tab === "security" && (
            <div style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
              <div>
                <p className="eyebrow">Access and source protection</p>
                <h2 className="t-title-lg">Security</h2>
              </div>

              <div className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
                <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: "8px" }}>
                  <h3 className="t-title-md">Repository access</h3>
                  <span className="t-label-sm muted">
                    Last observed{" "}
                    {data.repository.last_synced_at
                      ? new Date(data.repository.last_synced_at).toLocaleString()
                      : "time unavailable"}
                  </span>
                </div>

                <div className="pg-grid-3">
                  <div className="pg-card-canvas">
                    <span className="t-label-sm muted">Provider authority</span>
                    <strong className="t-title-sm" style={{ display: "block", marginTop: "4px", color: "var(--success)" }}>
                      {data.repository.full_name} · {data.repository.sync_health.replaceAll("_", " ")}
                    </strong>
                  </div>
                  <div className="pg-card-canvas">
                    <span className="t-label-sm muted">Effective role</span>
                    <strong className="t-title-sm" style={{ display: "block", marginTop: "4px" }}>
                      {viewerRole || "Project member"}
                    </strong>
                  </div>
                  <div className="pg-card-canvas">
                    <span className="t-label-sm muted">Source access</span>
                    <strong className="t-title-sm" style={{ display: "block", marginTop: "4px" }}>
                      {data.repository.source_access === false
                        ? "Unavailable"
                        : "Authorized members"}
                    </strong>
                  </div>
                </div>

                <p className="t-body-sm muted" style={{ margin: 0 }}>
                  Default branch protection and repository credentials are managed with GitHub and
                  your account connection. Project publication never changes repository visibility.
                </p>

                <div className="button-row">
                  <a className="t-label-md" href="/account/github">
                    Review GitHub connection →
                  </a>
                  {providerURL && (
                    <a className="t-label-md" href={`${providerURL}/settings`}>
                      Open GitHub repository settings ↗
                    </a>
                  )}
                </div>
              </div>

              <div className="pg-callout">
                <span>
                  PeerGit does not run hosted Actions, source scans or dependency alerts. Source
                  remains governed by GitHub access.
                </span>
              </div>
            </div>
          )}

          {tab === "snapshots" && (
            <div style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
              <div className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
                <div>
                  <h2 className="t-title-lg">Immutable snapshots</h2>
                  <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
                    Snapshots pin an exact 40-character SHA. Timing (receipt) and integrity (archive
                    verified) are tracked separately.
                  </p>
                </div>

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
                        <div style={{ display: "flex", flexDirection: "column", gap: "4px" }}>
                          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: "8px" }}>
                            <strong className="t-title-sm">
                              {s.state.replaceAll("_", " ")}
                            </strong>
                            <span
                              className={`pg-pill ${
                                s.state === "verified" ? "pg-pill-success" : "pg-pill-warning"
                              }`}
                            >
                              {s.state === "verified"
                                ? "Archive verified"
                                : s.state.replaceAll("_", " ")}
                            </span>
                          </div>
                          <p className="t-body-sm muted" style={{ margin: 0 }}>
                            {s.ref} → <code className="t-mono-sm">{s.commit_sha}</code> ·{" "}
                            {new Date(s.receipt_at).toLocaleString()}
                            {s.failure_code ? ` · ${s.failure_code}` : ""}
                          </p>
                          {s.archive_sha256 && (
                            <p className="t-body-sm muted record-summary" style={{ margin: 0 }}>
                              Archive SHA-256{" "}
                              <code className="t-mono-sm">{s.archive_sha256}</code>
                            </p>
                          )}
                        </div>
                        <div className="snapshot-actions">
                          {s.state === "verified" &&
                            data.repository?.source_access !== false && (
                              <a
                                className="secondary-button pg-btn-compact"
                                href={`/api/v1/projects/${projectID}/repository/snapshots/${s.id}/download`}
                              >
                                Download
                              </a>
                            )}
                          {s.retry_available && (
                            <button
                              className="secondary-button pg-btn-compact"
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
                  <p className="t-body-md muted" style={{ margin: 0 }}>
                    No snapshots yet.
                  </p>
                )}
              </div>
            </div>
          )}
        </>
      )}
    </section>
  );
}
