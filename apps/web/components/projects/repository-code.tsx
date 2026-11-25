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
      <section className="panel">
        <h2>Code</h2>
        <p role="alert">{problem}</p>
        <div className="button-row">
          <button
            className="secondary-button"
            onClick={() => setRetry((value) => value + 1)}
          >
            Retry
          </button>
          <a href={base}>Repository root</a>
        </div>
      </section>
    );
  if (!data) return <p role="status">Loading repository…</p>;

  const folder = data.file
    ? data.path.split("/").slice(0, -1).join("/")
    : data.path;
  const prefix = folder ? folder + "/" : "";
  const rows = data.entries
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

  return (
    <section className="repository-code" aria-labelledby="code-title">
      <div className="section-heading">
        <div>
          <h2 id="code-title">Code</h2>
          <p>
            {data.pinned ? "Pinned revision" : "Branch revision"}{" "}
            <code className={styles.sha}>{data.sha}</code>
          </p>
        </div>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            window.location.assign(
              `${base}?${new URLSearchParams({ tab: "code", ref: selectedBranch })}`,
            );
          }}
        >
          <label htmlFor="repository-ref">Branch</label>
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
          <button className="secondary-button" disabled={!selectedBranch}>
            Go
          </button>
        </form>
      </div>
      <div className={`panel ${styles.commit}`}>
        <div>
          <strong>{data.commit?.title || "Branch tip"}</strong>
          {data.tip && (
            <p>
              <code>{data.tip.slice(0, 12)}</code> {data.commit?.author_name}
            </p>
          )}
        </div>
        <a
          href={`${base}?${new URLSearchParams({ tab: "activity", ...(data.branch && { ref: data.branch }) })}`}
        >
          Commit history
        </a>
      </div>
      {data.truncated && (
        <p role="status">Only part of this tree is available here.</p>
      )}
      <div className="code-browser">
        <nav aria-label="Repository files">
          <a href={codeURL("")}>Repository root</a>
          {folder && (
            <p>
              <a href={codeURL(folder.split("/").slice(0, -1).join("/"))}>
                Parent directory
              </a>
            </p>
          )}
          <ul>
            {rows.map((entry) => (
              <li key={entry.path}>
                <button
                  type="button"
                  className="text-button"
                  onClick={() => window.location.assign(codeURL(entry.path))}
                >
                  <span aria-hidden="true">
                    {entry.type === "tree" ? "▸" : "▤"}{" "}
                  </span>
                  {entry.path.split("/").at(-1)}
                </button>
              </li>
            ))}
          </ul>
          {!rows.length && <p>No files in this directory.</p>}
        </nav>
        <article aria-label={data.path || "Repository directory"}>
          {data.file ? (
            <>
              <a href={codeURL(folder)}>Back to files</a>
              <h3 className={styles.sha}>{data.path}</h3>
              <div className="button-row">
                <button
                  className="secondary-button"
                  onClick={() =>
                    void copy(
                      new URL(codeURL(data.path, true), window.location.origin)
                        .href,
                      "Permalink copied.",
                    )
                  }
                >
                  Copy permalink
                </button>
                {data.content !== null && (
                  <>
                    <button
                      className="secondary-button"
                      onClick={() =>
                        void copy(data.content ?? "", "File copied.")
                      }
                    >
                      Copy file
                    </button>
                    <button
                      className="secondary-button"
                      aria-pressed={wrap}
                      onClick={() => setWrap(!wrap)}
                    >
                      Wrap lines
                    </button>
                  </>
                )}
              </div>
              <p role="status">{copied}</p>
              {data.content !== null ? (
                <pre tabIndex={0} className={wrap ? styles.wrapped : undefined}>
                  {data.content}
                </pre>
              ) : (
                <p>
                  {data.fileProblem ||
                    "This file type or size cannot be previewed."}
                </p>
              )}
            </>
          ) : (
            <>
              <h3 className={styles.sha}>{folder || "Repository root"}</h3>
              <ul>
                {rows.map((entry) => (
                  <li key={entry.path}>
                    <a href={codeURL(entry.path)}>
                      {entry.path.split("/").at(-1)}
                    </a>
                    {entry.type === "blob" && ` · ${entry.size} bytes`}
                  </li>
                ))}
              </ul>
              {data.readme !== null && (
                <section>
                  <h4>README at this revision</h4>
                  <pre className={styles.wrapped}>{data.readme}</pre>
                </section>
              )}
            </>
          )}
        </article>
      </div>
    </section>
  );
}
