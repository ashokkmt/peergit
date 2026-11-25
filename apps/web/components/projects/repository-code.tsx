"use client";

import { useEffect, useState } from "react";
import styles from "./repository-code.module.css";

type Entry = { path: string; type: string; size: number; mode?: string };
type Branch = { name: string; commit: { sha: string } };
type Commit = {
  canonical_id: string;
  kind: string;
  title: string;
  author_name: string;
};
type HistoryCommit = {
  sha: string;
  commit: { message: string; author: { name: string } };
};
class RepositoryReadError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}
type BrowserData = {
  branches: Branch[];
  branch: string;
  sha: string;
  pinned: boolean;
  path: string;
  entries: Entry[];
  file: Entry | undefined;
  content: string | null;
  tip: string | undefined;
  commit: Commit | undefined;
  truncated: boolean;
  readme: string | null;
  fileProblem: string;
};

async function read<T>(url: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(url, { cache: "no-store", signal });
  const result = (await response.json()) as {
    data?: T;
    error?: { message?: string };
  };
  if (!response.ok || !result.data) {
    throw new RepositoryReadError(
      response.status === 429
        ? "GitHub rate limit reached. Try again shortly."
        : (result.error?.message ?? "Repository could not be loaded."),
      response.status,
    );
  }
  return result.data;
}

export function RepositoryCode({
  projectID,
  branch,
}: {
  projectID: string;
  branch: string;
}) {
  const [data, setData] = useState<BrowserData | null>(null);
  const [problem, setProblem] = useState("");
  const [retry, setRetry] = useState(0);
  const [selectedBranch, setSelectedBranch] = useState("");
  const [wrap, setWrap] = useState(false);
  const [copied, setCopied] = useState("");
  const [fileQuery, setFileQuery] = useState("");
  const base = `/projects/${encodeURIComponent(projectID)}`;
  const api = `/api/v1${base}/repository`;

  useEffect(() => {
    const controller = new AbortController();
    const { signal } = controller;
    const params = new URLSearchParams(window.location.search);
    const pinned = params.get("sha");
    const path = params.get("path") ?? "";

    async function load() {
      setData(null);
      setProblem("");
      const [branchData, overview] = await Promise.all([
        read<{ items: Branch[]; default_branch: string }>(
          `${api}/branches`,
          signal,
        ),
        read<{ contributions: Commit[] }>(api, signal),
      ]);
      const current = params.get("ref") || branchData.default_branch || branch;
      if (!current && !pinned)
        throw new Error("This repository has no branches yet.");
      if (pinned && !/^[a-f0-9]{40}$/.test(pinned))
        throw new Error("This link does not contain a valid commit SHA.");
      const tree = await read<{
        sha: string;
        tree: Entry[];
        truncated?: boolean;
      }>(
        `${api}/tree?${new URLSearchParams(pinned ? { sha: pinned } : { ref: current })}`,
        signal,
      );
      const entry = tree.tree.find((item) => item.path === path);
      if (path && !entry)
        throw new Error(
          tree.truncated
            ? "This path was outside the returned tree."
            : "This path is unavailable at the selected revision.",
        );
      const file = entry?.type === "tree" ? undefined : entry;
      let content = null;
      let fileProblem = "";
      if (
        file?.type === "blob" &&
        file.mode !== "120000" &&
        file.size <= 1048576
      ) {
        try {
          content = (
            await read<{ content: string }>(
              `${api}/file?${new URLSearchParams({ path, ref: tree.sha })}`,
              signal,
            )
          ).content;
        } catch (error) {
          if (
            error instanceof RepositoryReadError &&
            [401, 403, 404].includes(error.status)
          )
            throw error;
          fileProblem =
            error instanceof Error
              ? error.message
              : "File could not be loaded.";
        }
      }
      const readmeEntry =
        !path &&
        tree.tree.find(
          (item) =>
            /^readme(?:\.md|\.txt)?$/i.test(item.path) &&
            item.type === "blob" &&
            item.mode !== "120000" &&
            item.size <= 1048576,
        );
      const tip = pinned
        ? branchData.items.find((item) => item.name === current)?.commit.sha
        : tree.sha;
      const [readme, history] = await Promise.all([
        readmeEntry
          ? read<{ content: string }>(
              `${api}/file?${new URLSearchParams({ path: readmeEntry.path, ref: tree.sha })}`,
              signal,
            )
              .then((value) => value.content)
              .catch(() => null)
          : null,
        tip
          ? read<{ items: HistoryCommit[] }>(
              `${api}/history?${new URLSearchParams({ ref: tip })}`,
              signal,
            )
              .then((value) => value.items.find((item) => item.sha === tip))
              .catch(() => undefined)
          : undefined,
      ]);
      if (signal.aborted) return;
      setSelectedBranch(current);
      setData({
        branches: branchData.items,
        branch: current,
        sha: tree.sha,
        pinned: Boolean(pinned),
        path,
        entries: tree.tree,
        file,
        content,
        tip,
        truncated: Boolean(tree.truncated),
        readme,
        fileProblem,
        commit: history
          ? {
              kind: "commit",
              canonical_id: history.sha,
              title: history.commit.message.split("\n")[0],
              author_name: history.commit.author.name,
            }
          : overview.contributions.find(
              (item) =>
                item.kind === "commit" &&
                item.canonical_id.replace(/^git:sha1:/, "") === tip,
            ),
      });
    }

    void load().catch((error) => {
      if (!signal.aborted)
        setProblem(
          error instanceof Error
            ? error.message
            : "Repository could not be loaded.",
        );
    });
    return () => controller.abort();
  }, [api, branch, retry]);

  function codeURL(path: string, pin = data?.pinned) {
    const params = new URLSearchParams({ tab: "code" });
    if (data?.branch) params.set("ref", data.branch);
    if (pin && data) params.set("sha", data.sha);
    if (path) params.set("path", path);
    return `${base}?${params}`;
  }

  async function copy(value: string, label: string) {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(label);
    } catch {
      setCopied("Clipboard is unavailable.");
    }
  }

  if (problem)
    return (
      <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
        <h2 className="t-title-md">Code</h2>
        <p className="form-error" role="alert" style={{ margin: 0 }}>
          {problem}
        </p>
        <div className="button-row">
          <button
            className="secondary-button pg-btn-compact"
            onClick={() => setRetry((value) => value + 1)}
          >
            Retry
          </button>
          <a className="t-label-md" href={base}>
            Repository root
          </a>
        </div>
      </section>
    );
  if (!data) return <p role="status">Loading repository…</p>;

  const folder = data.file
    ? data.path.split("/").slice(0, -1).join("/")
    : data.path;
  const prefix = folder ? folder + "/" : "";
  const folderRows = data.entries
    .filter(
      (entry) =>
        entry.path.startsWith(prefix) &&
        entry.path !== folder &&
        !entry.path.slice(prefix.length).includes("/"),
    )
    .sort(
      (a, b) =>
        Number(b.type === "tree") - Number(a.type === "tree") ||
        a.path.localeCompare(b.path),
    );
  const matchingEntries = fileQuery.trim()
    ? data.entries.filter((entry) =>
        entry.path.toLowerCase().includes(fileQuery.trim().toLowerCase()),
      )
    : folderRows;

  return (
    <section
      className="repository-code"
      aria-labelledby="code-title"
      style={{ display: "flex", flexDirection: "column", gap: "18px", margin: 0 }}
    >
      <h2 id="code-title" className="sr-only">
        Code
      </h2>

      {/* Code Toolbar */}
      <div className="code-toolbar">
        <div className="code-toolbar-left">
          <form
            onSubmit={(event) => {
              event.preventDefault();
              window.location.assign(
                `${base}?${new URLSearchParams({ tab: "code", ref: selectedBranch })}`,
              );
            }}
          >
            <label htmlFor="repository-ref" className="sr-only">
              Branch
            </label>
            <select
              id="repository-ref"
              value={selectedBranch}
              onChange={(event) => setSelectedBranch(event.target.value)}
            >
              {!data.branches.some((item) => item.name === data.branch) && (
                <option value={data.branch}>{data.branch}</option>
              )}
              {data.branches.map((item) => (
                <option key={item.name}>{item.name}</option>
              ))}
            </select>
            <button className="secondary-button pg-btn-compact" disabled={!selectedBranch}>
              Go
            </button>
          </form>

          <span className="pg-pill pg-pill-raised t-mono-sm tabular-nums" title={data.sha}>
            {data.pinned ? "Pinned revision" : "Branch revision"}: {data.sha.slice(0, 8)}
          </span>
          <span className="sr-only">{data.sha}</span>
        </div>

        <div className="code-toolbar-right">
          <label className="sr-only" htmlFor="find-file-input">
            Find a file
          </label>
          <input
            id="find-file-input"
            type="search"
            className="pg-input"
            value={fileQuery}
            onChange={(e) => setFileQuery(e.target.value)}
            placeholder="Find a file…"
            style={{ width: "auto", flex: "1 1 200px", minWidth: "160px", minHeight: "38px", padding: "6px 10px" }}
          />
          <button
            type="button"
            className="secondary-button pg-btn-compact"
            onClick={() =>
              void copy(
                new URL(codeURL(data.path, true), window.location.origin).href,
                "Permalink copied.",
              )
            }
          >
            Copy permalink
          </button>
        </div>
      </div>

      {data.truncated && (
        <p className="pg-callout" role="status">
          Only part of this tree is available here.
        </p>
      )}

      {copied && (
        <p className="t-body-sm" role="status" style={{ color: "var(--success)", margin: 0 }}>
          {copied}
        </p>
      )}

      <div className="code-browser">
        <nav aria-label="Repository files">
          <a href={codeURL("")} style={{ fontWeight: 600 }}>
            Repository root
          </a>
          {folder && (
            <a href={codeURL(folder.split("/").slice(0, -1).join("/"))}>
              ↑ Parent directory
            </a>
          )}
          <ul>
            {matchingEntries.slice(0, 80).map((entry) => (
              <li key={entry.path}>
                <button
                  type="button"
                  className="text-button"
                  onClick={() => window.location.assign(codeURL(entry.path))}
                >
                  <span aria-hidden="true">
                    {entry.type === "tree" ? "▸" : "▤"}
                  </span>
                  <span style={{ overflowWrap: "anywhere" }}>
                    {fileQuery.trim() ? entry.path : entry.path.split("/").at(-1)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
          {!matchingEntries.length && (
            <p className="t-body-sm muted" style={{ margin: "6px 0 0" }}>
              No files match this filter.
            </p>
          )}
        </nav>

        <article className="code-reading-pane" aria-label={data.path || "Repository directory"}>
          {/* Latest Commit Strip */}
          <div className="commit-strip">
            <div>
              <strong className="t-body-md">{data.commit?.title || "Branch tip"}</strong>
              {data.tip && (
                <p className="t-body-sm muted" style={{ margin: "2px 0 0" }}>
                  <code className="t-mono-sm">{data.tip.slice(0, 7)}</code>{" "}
                  {data.commit?.author_name}
                </p>
              )}
            </div>
            <a
              className="t-label-md"
              href={`${base}?${new URLSearchParams({
                tab: "activity",
                ...(data.branch && { ref: data.branch }),
              })}`}
            >
              Commit history
            </a>
          </div>

          {data.file ? (
            <div className="code-file-box">
              <div className="code-file-header">
                <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: "10px" }}>
                  <a className="t-label-md" href={codeURL(folder)}>
                    ← Back to files
                  </a>
                  <h3 className={`t-mono-sm ${styles.sha}`} style={{ fontWeight: 600 }}>
                    {data.path}
                  </h3>
                </div>
                <div className="button-row">
                  <button
                    className="secondary-button pg-btn-compact"
                    type="button"
                    onClick={() =>
                      void copy(
                        new URL(codeURL(data.path, true), window.location.origin).href,
                        "Permalink copied.",
                      )
                    }
                  >
                    Copy permalink
                  </button>
                  {data.content !== null && (
                    <>
                      <button
                        className="secondary-button pg-btn-compact"
                        type="button"
                        onClick={() => void copy(data.content ?? "", "File copied.")}
                      >
                        Copy file
                      </button>
                      <button
                        className="secondary-button pg-btn-compact"
                        type="button"
                        aria-pressed={wrap}
                        onClick={() => setWrap(!wrap)}
                      >
                        Wrap lines
                      </button>
                    </>
                  )}
                </div>
              </div>
              {data.content !== null ? (
                <pre
                  tabIndex={0}
                  className={wrap ? `code-pre ${styles.wrapped}` : "code-pre"}
                >
                  {data.content}
                </pre>
              ) : (
                <p className="t-body-md muted" style={{ padding: "16px", margin: 0 }}>
                  {data.fileProblem || "This file type or size cannot be previewed."}
                </p>
              )}
            </div>
          ) : (
            <>
              <div className="dir-table">
                <div
                  className="dir-row"
                  style={{ background: "var(--raised)", fontWeight: 600 }}
                >
                  <span className={styles.sha}>{folder || "Repository root"}</span>
                  <span className="t-label-sm muted">{folderRows.length} entries</span>
                </div>
                {folderRows.map((entry) => (
                  <div className="dir-row" key={entry.path}>
                    <a href={codeURL(entry.path)}>
                      {entry.type === "tree" ? "📁 " : "📄 "}
                      {entry.path.split("/").at(-1)}
                    </a>
                    <span className="t-body-sm muted tabular-nums">
                      {entry.type === "blob" ? `${entry.size} bytes` : "Directory"}
                    </span>
                  </div>
                ))}
              </div>

              {data.readme !== null && (
                <section className="code-file-box">
                  <div className="code-file-header">
                    <h4 className="t-mono-sm" style={{ margin: 0, fontWeight: 600 }}>
                      README at this revision
                    </h4>
                    <span className="t-label-sm muted">Sanitized preview</span>
                  </div>
                  <pre className={`readme-text ${styles.wrapped}`}>{data.readme}</pre>
                </section>
              )}
            </>
          )}
        </article>
      </div>
    </section>
  );
}
