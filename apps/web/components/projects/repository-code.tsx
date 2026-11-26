"use client";

import { useEffect, useState } from "react";
import { readApiEnvelope } from "@/lib/http";

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

const compactSecondaryBtn =
  "inline-flex min-h-[38px] items-center justify-center gap-2 rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised disabled:opacity-55 disabled:cursor-not-allowed";

async function read<T>(url: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(url, { cache: "no-store", signal });
  const result = await readApiEnvelope<T>(response, "Repository could not be loaded.");
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
        read<{ items: Branch[]; default_branch: string }>(`${api}/branches`, signal),
        read<{ contributions: Commit[] }>(api, signal),
      ]);
      const current = params.get("ref") || branchData.default_branch || branch;
      if (!current && !pinned) throw new Error("This repository has no branches yet.");
      if (pinned && !/^[a-f0-9]{40}$/.test(pinned)) {
        throw new Error("This link does not contain a valid commit SHA.");
      }
      const tree = await read<{
        sha: string;
        tree: Entry[];
        truncated?: boolean;
      }>(
        `${api}/tree?${new URLSearchParams(pinned ? { sha: pinned } : { ref: current })}`,
        signal,
      );
      const entry = tree.tree.find((item) => item.path === path);
      if (path && !entry) {
        throw new Error(
          tree.truncated
            ? "This path was outside the returned tree."
            : "This path is unavailable at the selected revision.",
        );
      }
      const file = entry?.type === "tree" ? undefined : entry;
      let content = null;
      let fileProblem = "";
      if (file?.type === "blob" && file.mode !== "120000" && file.size <= 1048576) {
        try {
          content = (
            await read<{ content: string }>(
              `${api}/file?${new URLSearchParams({ path, ref: tree.sha })}`,
              signal,
            )
          ).content;
        } catch (error) {
          if (error instanceof RepositoryReadError && [401, 403, 404].includes(error.status)) {
            throw error;
          }
          fileProblem = error instanceof Error ? error.message : "File could not be loaded.";
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
      if (!signal.aborted) {
        setProblem(error instanceof Error ? error.message : "Repository could not be loaded.");
      }
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

  if (problem) {
    return (
      <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3">
        <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">Code</h2>
        <p
          className="rounded-lg px-4 py-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger m-0"
          role="alert"
        >
          {problem}
        </p>
        <div className="flex flex-wrap gap-2.5 items-center">
          <button
            className={compactSecondaryBtn}
            onClick={() => setRetry((value) => value + 1)}
          >
            Retry
          </button>
          <a className="text-[13px] font-medium leading-[18px]" href={base}>
            Repository root
          </a>
        </div>
      </section>
    );
  }
  if (!data) return <p role="status">Loading repository…</p>;

  const folder = data.file ? data.path.split("/").slice(0, -1).join("/") : data.path;
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
        Number(b.type === "tree") - Number(a.type === "tree") || a.path.localeCompare(b.path),
    );
  const matchingEntries = fileQuery.trim()
    ? data.entries.filter((entry) =>
        entry.path.toLowerCase().includes(fileQuery.trim().toLowerCase()),
      )
    : folderRows;

  return (
    <section className="flex flex-col gap-[18px] m-0" aria-labelledby="code-title">
      <h2 id="code-title" className="sr-only">
        Code
      </h2>

      {/* Code Toolbar */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <form
            className="flex items-center gap-2"
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
              className="min-h-10 py-1.5 px-3 border border-control rounded-lg bg-surface text-on-surface font-mono text-[13px]"
            >
              {!data.branches.some((item) => item.name === data.branch) && (
                <option value={data.branch}>{data.branch}</option>
              )}
              {data.branches.map((item) => (
                <option key={item.name}>{item.name}</option>
              ))}
            </select>
            <button className={compactSecondaryBtn} disabled={!selectedBranch}>
              Go
            </button>
          </form>

          <span
            className="inline-flex items-center px-2.5 py-1 rounded-full bg-raised border border-hairline font-mono text-[13px] leading-[19px] tabular-nums"
            title={data.sha}
          >
            {data.pinned ? "Pinned revision" : "Branch revision"}: {data.sha.slice(0, 8)}
          </span>
          <span className="sr-only">{data.sha}</span>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <label className="sr-only" htmlFor="find-file-input">
            Find a file
          </label>
          <input
            id="find-file-input"
            type="search"
            value={fileQuery}
            onChange={(e) => setFileQuery(e.target.value)}
            placeholder="Find a file…"
            className="flex-[1_1_200px] min-w-[160px] min-h-[38px] px-2.5 py-1.5 border border-control rounded-lg bg-surface text-on-surface text-sm leading-5"
          />
          <button
            type="button"
            className={compactSecondaryBtn}
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
        <p
          className="flex items-start gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px] m-0"
          role="status"
        >
          Only part of this tree is available here.
        </p>
      )}

      {copied && (
        <p className="text-[13px] leading-[19px] text-success m-0" role="status">
          {copied}
        </p>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-[260px_minmax(0,1fr)] gap-5 items-start">
        <nav
          aria-label="Repository files"
          className="bg-surface border border-hairline rounded-xl p-3.5 max-h-[70vh] overflow-y-auto"
        >
          <a
            href={codeURL("")}
            className="block py-1.5 px-2 rounded text-[13px] font-semibold font-mono text-link no-underline hover:bg-raised hover:text-on-surface"
          >
            Repository root
          </a>
          {folder && (
            <a
              href={codeURL(folder.split("/").slice(0, -1).join("/"))}
              className="block py-1.5 px-2 rounded text-[13px] font-mono text-on-surface no-underline hover:bg-raised"
            >
              ↑ Parent directory
            </a>
          )}
          <ul className="list-none m-0 p-0 grid gap-0.5">
            {matchingEntries.slice(0, 80).map((entry) => (
              <li key={entry.path}>
                <button
                  type="button"
                  className="border-0 py-1.5 px-2 w-full flex items-center gap-1.5 text-left rounded bg-transparent text-on-surface cursor-pointer font-mono text-[13px] hover:bg-raised"
                  onClick={() => window.location.assign(codeURL(entry.path))}
                >
                  <span aria-hidden="true">{entry.type === "tree" ? "▸" : "▤"}</span>
                  <span className="[overflow-wrap:anywhere]">
                    {fileQuery.trim() ? entry.path : entry.path.split("/").at(-1)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
          {!matchingEntries.length && (
            <p className="text-[13px] leading-[19px] text-muted mt-1.5 mb-0">
              No files match this filter.
            </p>
          )}
        </nav>

        <article className="min-w-0 flex flex-col gap-4" aria-label={data.path || "Repository directory"}>
          {/* Latest Commit Strip */}
          <div className="bg-surface border border-hairline rounded-xl px-4 py-3 flex flex-wrap items-center justify-between gap-3">
            <div>
              <strong className="text-sm leading-[21px]">
                {data.commit?.title || "Branch tip"}
              </strong>
              {data.tip && (
                <p className="text-[13px] leading-[19px] text-muted mt-0.5 mb-0">
                  <code className="font-mono text-[13px] leading-[19px]">
                    {data.tip.slice(0, 7)}
                  </code>{" "}
                  {data.commit?.author_name}
                </p>
              )}
            </div>
            <a
              className="text-[13px] font-medium leading-[18px]"
              href={`${base}?${new URLSearchParams({
                tab: "activity",
                ...(data.branch && { ref: data.branch }),
              })}`}
            >
              Commit history
            </a>
          </div>

          {data.file ? (
            <div className="bg-surface border border-hairline rounded-xl overflow-hidden">
              <div className="bg-raised border-b border-hairline px-4 py-2.5 flex flex-wrap items-center justify-between gap-2.5">
                <div className="flex flex-wrap items-center gap-2.5">
                  <a className="text-[13px] font-medium leading-[18px]" href={codeURL(folder)}>
                    ← Back to files
                  </a>
                  <h3 className="font-mono text-[13px] leading-[19px] font-semibold [overflow-wrap:anywhere] m-0">
                    {data.path}
                  </h3>
                </div>
                <div className="flex flex-wrap gap-2.5 items-center">
                  <button
                    className={compactSecondaryBtn}
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
                        className={compactSecondaryBtn}
                        type="button"
                        onClick={() => void copy(data.content ?? "", "File copied.")}
                      >
                        Copy file
                      </button>
                      <button
                        className={compactSecondaryBtn}
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
                  className={`m-0 max-h-[65vh] overflow-auto [tab-size:2] p-4 bg-surface text-on-surface font-mono text-[13px] leading-[1.55] ${
                    wrap ? "whitespace-pre-wrap [overflow-wrap:anywhere]" : "whitespace-pre"
                  }`}
                >
                  {data.content}
                </pre>
              ) : (
                <p className="text-sm leading-[21px] text-muted p-4 m-0">
                  {data.fileProblem || "This file type or size cannot be previewed."}
                </p>
              )}
            </div>
          ) : (
            <>
              <div className="bg-surface border border-hairline rounded-xl overflow-hidden">
                <div className="bg-raised px-4 py-2.5 border-b border-hairline flex items-center justify-between gap-3 font-mono text-[13px] font-semibold">
                  <span className="[overflow-wrap:anywhere]">{folder || "Repository root"}</span>
                  <span className="text-xs font-medium leading-4 text-muted">
                    {folderRows.length} entries
                  </span>
                </div>
                {folderRows.map((entry) => (
                  <div
                    className="px-4 py-2.5 border-b border-hairline last:border-b-0 flex items-center justify-between gap-3 font-mono text-[13px]"
                    key={entry.path}
                  >
                    <a
                      href={codeURL(entry.path)}
                      className="no-underline font-medium hover:underline"
                    >
                      {entry.type === "tree" ? "📁 " : "📄 "}
                      {entry.path.split("/").at(-1)}
                    </a>
                    <span className="text-[13px] leading-[19px] text-muted tabular-nums">
                      {entry.type === "blob" ? `${entry.size} bytes` : "Directory"}
                    </span>
                  </div>
                ))}
              </div>

              {data.readme !== null && (
                <section className="bg-surface border border-hairline rounded-xl overflow-hidden">
                  <div className="bg-raised border-b border-hairline px-4 py-2.5 flex flex-wrap items-center justify-between gap-2.5">
                    <h4 className="font-mono text-[13px] leading-[19px] m-0 font-semibold">
                      README at this revision
                    </h4>
                    <span className="text-xs font-medium leading-4 text-muted">
                      Sanitized preview
                    </span>
                  </div>
                  <pre className="m-0 max-h-[65vh] overflow-auto [tab-size:2] p-4 bg-surface text-on-surface font-mono text-[13px] leading-[1.55] whitespace-pre-wrap [overflow-wrap:anywhere]">
                    {data.readme}
                  </pre>
                </section>
              )}
            </>
          )}
        </article>
      </div>
    </section>
  );
}
