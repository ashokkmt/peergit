"use client";

import { useCallback, useEffect, useState } from "react";
import { PreviewBadge } from "@/components/layout/preview-banner";
import { readApiEnvelope } from "@/lib/http";

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
type Data = {
  repository: Binding | null;
  contributions: Contribution[];
};

const cardClass = "min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col";
const compactSecondaryBtn =
  "inline-flex min-h-[38px] items-center justify-center gap-2 rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised disabled:opacity-55 disabled:cursor-not-allowed";

function renderDiffLines(patch: string) {
  return patch.split("\n").map((line, index) => {
    if (line.startsWith("+") && !line.startsWith("+++")) {
      return (
        <span key={index} className="block px-2 bg-success/14 text-success" aria-label="Added line">
          {line}
        </span>
      );
    }
    if (line.startsWith("-") && !line.startsWith("---")) {
      return (
        <span key={index} className="block px-2 bg-danger/14 text-danger" aria-label="Removed line">
          {line}
        </span>
      );
    }
    if (line.startsWith("@@")) {
      return (
        <span key={index} className="block px-2 text-muted">
          {line}
        </span>
      );
    }
    return (
      <span key={index} className="block px-2">
        {line}
      </span>
    );
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
  });
  const [loading, setLoading] = useState(true);
  const [problem, setProblem] = useState("");
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
      if (r.status === 204) return undefined as T;
      const v = await readApiEnvelope<T>(r, "Repository request failed.");
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
      setData({ repository: null, contributions: [] });
      setDiff(null);
      setProblem(
        e instanceof Error ? e.message : "Repository activity could not be loaded.",
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
        if (!controller.signal.aborted) {
          setHistoryProblem(
            "Live branch history is unavailable. Showing synchronized commit evidence.",
          );
        }
      });
    return () => controller.abort();
  }, [api, projectID, tab]);

  const commits = history ?? data.contributions.filter((item) => item.kind === "commit");
  const [selection, setSelection] = useState("");
  useEffect(() => {
    setSelection(
      new URLSearchParams(location.search).get(tab === "activity" ? "commit" : "item") ?? "",
    );
  }, [tab]);

  useEffect(() => {
    if (tab !== "activity" || !selection || !/^[a-f0-9]{40}$/.test(selection)) return;
    const controller = new AbortController();
    void api<NonNullable<typeof diff>>(
      `/api/v1/projects/${projectID}/repository/commit?sha=${selection}`,
      { signal: controller.signal },
    )
      .then((value) => {
        if (!controller.signal.aborted) setDiff(value);
      })
      .catch((e) => {
        if (!controller.signal.aborted) {
          setProblem(e instanceof Error ? e.message : "Commit diff could not be loaded.");
        }
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
    <section className="flex flex-col gap-5" aria-label="Repository evidence">
      {problem && (
        <p
          className="rounded-lg px-4 py-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger m-0"
          role="alert"
        >
          {problem}
        </p>
      )}
      {loading ? (
        <p role="status" className="m-0">
          Loading repository…
        </p>
      ) : !data.repository ? (
        <div className="grid justify-items-center content-center gap-2.5 min-h-[220px] p-8 border border-dashed border-control rounded-2xl text-center bg-surface">
          <p className="m-0 text-muted">This older project has no connected repository yet.</p>
          <a
            className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium no-underline border border-primary bg-primary text-on-primary hover:opacity-90"
            href="/account/github"
          >
            Connect GitHub and import a repository
          </a>
        </div>
      ) : (
        <>
          {tab === "code" && (
            <div className={`${cardClass} flex-row flex-wrap items-center justify-between gap-4`}>
              <div className="flex flex-col gap-1.5">
                <div className="flex flex-wrap items-center gap-2.5">
                  <strong className="text-base font-semibold leading-[22px]">
                    {providerURL ? (
                      <a href={providerURL} target="_blank" rel="noreferrer">
                        {data.repository.full_name} ↗
                      </a>
                    ) : (
                      data.repository.full_name
                    )}
                  </strong>
                  <span className="text-[13px] leading-[19px] text-muted">
                    {data.repository.access_state.replaceAll("_", " ")} · sync{" "}
                    {data.repository.sync_health.replaceAll("_", " ")}
                    {data.repository.last_synced_at
                      ? ` · ${new Date(data.repository.last_synced_at).toLocaleString()}`
                      : ""}
                  </span>
                </div>
                <p className="text-[13px] leading-[19px] text-muted m-0">
                  {data.repository.description || "No repository description."} · Default branch:{" "}
                  <code className="font-mono text-[13px] leading-[19px]">
                    {data.repository.default_branch || "not set"}
                  </code>
                  {Object.keys(data.repository.languages).length > 0
                    ? ` · Languages: ${Object.keys(data.repository.languages).join(", ")}`
                    : ""}
                </p>
                {data.repository.limitations && (
                  <p className="text-[13px] leading-[19px] text-muted m-0">
                    {data.repository.limitations}
                  </p>
                )}
              </div>
              <button
                className={compactSecondaryBtn}
                type="button"
                disabled={loading}
                onClick={() => void refresh()}
              >
                Refresh
              </button>
            </div>
          )}

          {tab === "activity" && (
            <div className="flex flex-col gap-[18px]">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
                    Commit activity
                  </h2>
                  <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">
                    Commit records are observations from the connected GitHub repository. Unmatched
                    authors stay unlinked.
                  </p>
                </div>
                <div className="flex flex-wrap gap-2.5 items-center">
                  <a className={compactSecondaryBtn} href={`${base}?tab=issues`}>
                    Observed issues
                  </a>
                  <a className={compactSecondaryBtn} href={`${base}?tab=pull-requests`}>
                    Observed pull requests
                  </a>
                </div>
              </div>

              {historyProblem && (
                <p
                  className="flex items-start gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px] m-0"
                  role="status"
                >
                  {historyProblem}
                </p>
              )}

              <div className="grid grid-cols-1 lg:grid-cols-[360px_minmax(0,1fr)] gap-5 items-start">
                <div className="bg-surface border border-hairline rounded-xl overflow-hidden">
                  <div className="px-4 py-3 border-b border-hairline flex justify-between items-center">
                    <span className="text-base font-semibold leading-[22px]">Commits</span>
                    <span className="text-xs font-medium leading-4 text-muted">
                      {commits.length} observed
                    </span>
                  </div>
                  {commits.length ? (
                    <div className="flex flex-col">
                      {commits.map((c) => {
                        const sha = c.canonical_id.replace(/^git:sha1:/, "");
                        return (
                          <button
                            key={c.id}
                            type="button"
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
                            className="block w-full py-3.5 px-4 border-0 border-b border-hairline last:border-b-0 border-l-[3px] border-l-transparent bg-surface text-on-surface text-left cursor-pointer hover:bg-raised aria-[current=true]:bg-raised aria-[current=true]:border-l-link"
                          >
                            <strong className="block text-sm leading-[21px] [overflow-wrap:anywhere]">
                              {c.title || c.canonical_id}
                            </strong>
                            <small className="block mt-1 text-muted text-xs">
                              {c.author_name || c.author_login || "Unlinked GitHub account"} ·{" "}
                              <code className="font-mono text-[13px] leading-[19px]">
                                {sha.slice(0, 7)}
                              </code>{" "}
                              ·{" "}
                              {c.occurred_at
                                ? new Date(c.occurred_at).toLocaleDateString()
                                : "Date unavailable"}
                            </small>
                          </button>
                        );
                      })}
                    </div>
                  ) : (
                    <p className="text-sm leading-[21px] text-muted p-4 m-0">
                      Commit evidence has not synced yet.
                    </p>
                  )}
                </div>

                <div className="min-w-0 flex flex-col gap-4">
                  {diff ? (
                    <>
                      <div className={`${cardClass} flex-row flex-wrap items-center justify-between gap-3`}>
                        <div>
                          <h3 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                            Commit {diff.sha.slice(0, 12)}
                          </h3>
                          <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">
                            {diff.files.length} {diff.files.length === 1 ? "file" : "files"} changed
                          </p>
                        </div>
                        <a
                          className={compactSecondaryBtn}
                          href={`${base}?tab=code&sha=${diff.sha}`}
                        >
                          Browse tree at SHA
                        </a>
                      </div>

                      {diff.files.map((file) => (
                        <article
                          key={file.filename}
                          className="bg-surface border border-hairline rounded-xl overflow-hidden"
                        >
                          <div className="bg-raised border-b border-hairline px-4 py-2.5 flex flex-wrap items-center justify-between gap-2.5">
                            <code className="font-mono text-[13px] leading-[19px] font-semibold [overflow-wrap:anywhere]">
                              {file.filename}
                            </code>
                            <span className="text-xs font-medium leading-4 text-muted">
                              {file.status} · Unified diff
                            </span>
                          </div>
                          <pre className="m-0 max-h-[65vh] overflow-auto whitespace-pre [tab-size:2] p-4 bg-surface text-on-surface font-mono text-[13px] leading-[1.55]">
                            {file.patch
                              ? renderDiffLines(file.patch)
                              : "Diff omitted by GitHub for a binary or large file."}
                          </pre>
                        </article>
                      ))}
                    </>
                  ) : (
                    <div className={cardClass}>
                      <p className="text-sm leading-[21px] text-muted m-0">
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
                <div className="flex flex-col gap-[18px]">
                  <div className="flex flex-wrap items-end justify-between gap-4">
                    <div>
                      <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
                        {label}
                      </h2>
                      <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">
                        PeerGit displays synchronized summaries. Open GitHub for the complete record
                        and to make changes.
                      </p>
                    </div>
                    {providerURL && (
                      <a
                        className={compactSecondaryBtn}
                        href={providerURL + (isIssue ? "/issues" : "/pulls")}
                      >
                        Open on GitHub ↗
                      </a>
                    )}
                  </div>

                  {selection && !selected && (
                    <p
                      className="flex items-start gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px] m-0"
                      role="status"
                    >
                      This record is unavailable in the current observations.
                    </p>
                  )}

                  <div className="grid grid-cols-1 lg:grid-cols-[360px_minmax(0,1fr)] gap-5 items-start">
                    <nav
                      className="bg-surface border border-hairline rounded-xl overflow-hidden flex flex-col"
                      aria-label={label}
                    >
                      {records.map((item) => (
                        <a
                          key={item.id}
                          aria-current={selected?.id === item.id ? "true" : undefined}
                          href={base + "?" + new URLSearchParams({ tab, item: item.id })}
                          className="block py-3.5 px-4 border-b border-hairline last:border-b-0 border-l-[3px] border-l-transparent bg-surface text-on-surface no-underline text-left hover:bg-raised aria-[current=true]:bg-raised aria-[current=true]:border-l-link"
                        >
                          <div className="flex justify-between gap-2 mb-1">
                            <span className="font-mono text-[13px] leading-[19px] text-muted">
                              {item.number ? `#${item.number}` : ""}
                            </span>
                            <span className="inline-flex items-center px-2.5 py-0.5 rounded-full bg-surface border border-success text-success text-xs font-medium leading-4">
                              {item.state || "State unavailable"}
                            </span>
                          </div>
                          <strong className="block text-base font-semibold leading-[22px]">
                            {item.title || "Title unavailable"}
                          </strong>
                        </a>
                      ))}
                      {!records.length && (
                        <p className="text-sm leading-[21px] text-muted p-4 m-0">
                          No synchronized summaries available.
                        </p>
                      )}
                    </nav>

                    <div className="min-w-0">
                      {selected ? (
                        <article className={`${cardClass} gap-4`}>
                          <div className="flex flex-wrap items-start justify-between gap-3 pb-3.5 border-b border-hairline">
                            <div>
                              <div className="flex flex-wrap items-center gap-2">
                                <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-success text-success text-xs font-medium leading-4">
                                  {selected.state || "State unavailable"} · GitHub{" "}
                                  {isIssue ? "Issue" : "PR"}
                                  {selected.number ? ` #${selected.number}` : ""}
                                </span>
                                <span className="text-xs font-medium leading-4 text-muted">
                                  {isIssue
                                    ? "Repository issue (distinct from Project tasks)"
                                    : "Synchronized pull request observation"}
                                </span>
                              </div>
                              <h3 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] mt-2 mb-0">
                                {selected.title || "Title unavailable"}
                              </h3>
                            </div>
                            {providerURL && selected.number && (
                              <a
                                className={compactSecondaryBtn}
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

                          <p className="text-sm leading-[21px] [overflow-wrap:anywhere] whitespace-pre-wrap m-0">
                            {selected.summary || "Summary unavailable."}
                          </p>
                          <p className="text-[13px] leading-[19px] text-muted m-0">
                            Observed{" "}
                            {selected.observed_at
                              ? new Date(selected.observed_at).toLocaleString()
                              : "at an unknown time"}
                            .
                          </p>

                          <div className="flex flex-col gap-1.5 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px]">
                            <div className="flex flex-wrap items-center justify-between gap-2 w-full">
                              <strong className="text-[13px] font-medium leading-[18px] text-link">
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
                        <div className={cardClass}>
                          <p className="text-sm leading-[21px] text-muted m-0">
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
            <div className="flex flex-col gap-5">
              <div>
                <p className="mt-0 mb-1.5 text-link text-xs font-semibold tracking-[0.02em]">
                  Access and source protection
                </p>
                <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
                  Security
                </h2>
              </div>

              <div className={`${cardClass} gap-4`}>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h3 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                    Repository access
                  </h3>
                  <span className="text-xs font-medium leading-4 text-muted">
                    Last observed{" "}
                    {data.repository.last_synced_at
                      ? new Date(data.repository.last_synced_at).toLocaleString()
                      : "time unavailable"}
                  </span>
                </div>

                <div className="grid grid-cols-1 min-[900px]:grid-cols-3 gap-6">
                  <div className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl p-5">
                    <span className="text-xs font-medium leading-4 text-muted">
                      Provider authority
                    </span>
                    <strong className="block text-base font-semibold leading-[22px] mt-1 text-success">
                      {data.repository.full_name} · {data.repository.sync_health.replaceAll("_", " ")}
                    </strong>
                  </div>
                  <div className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl p-5">
                    <span className="text-xs font-medium leading-4 text-muted">Effective role</span>
                    <strong className="block text-base font-semibold leading-[22px] mt-1">
                      {viewerRole || "Project member"}
                    </strong>
                  </div>
                  <div className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl p-5">
                    <span className="text-xs font-medium leading-4 text-muted">Source access</span>
                    <strong className="block text-base font-semibold leading-[22px] mt-1">
                      {data.repository.source_access === false
                        ? "Unavailable"
                        : "Authorized members"}
                    </strong>
                  </div>
                </div>

                <p className="text-[13px] leading-[19px] text-muted m-0">
                  Default branch protection and repository credentials are managed with GitHub and
                  your account connection. Project publication never changes repository visibility.
                </p>

                <div className="flex flex-wrap gap-2.5 items-center">
                  <a className="text-[13px] font-medium leading-[18px]" href="/account/github">
                    Review GitHub connection →
                  </a>
                  {providerURL && (
                    <a className="text-[13px] font-medium leading-[18px]" href={`${providerURL}/settings`}>
                      Open GitHub repository settings ↗
                    </a>
                  )}
                </div>
              </div>

              <div className="flex items-start gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px]">
                <span>
                  PeerGit does not run hosted Actions, source scans or dependency alerts. Source
                  remains governed by GitHub access.
                </span>
              </div>
            </div>
          )}


        </>
      )}
    </section>
  );
}
