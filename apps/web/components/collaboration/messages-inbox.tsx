"use client";

import { useEffect, useState } from "react";
import { readApiEnvelope } from "@/lib/http";

type Conversation = {
  id: string;
  context_type: "project" | "application" | "pbl_team" | "pbl_faculty";
  project_id: string;
  application_id?: string;
  project_title: string;
  last_message_at: string;
};

const errorText = "Your conversations could not be loaded. Refresh to check current campus access.";

export function MessagesInbox() {
  const [items, setItems] = useState<Conversation[]>([]);
  const [filter, setFilter] = useState<"all" | "project" | "application">("all");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try {
      const response = await fetch("/api/v1/me/conversations", { cache: "no-store" });
      const result = await readApiEnvelope<{ items: Conversation[] }>(response, errorText);
      if (!response.ok) throw new Error(result.error?.message ?? errorText);
      setItems(result.data?.items ?? []);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : errorText);
      setItems([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    const revalidate = () => {
      if (document.visibilityState === "visible") void load();
    };
    window.addEventListener("focus", revalidate);
    return () => window.removeEventListener("focus", revalidate);
  }, []);

  const visibleItems = items.filter((item) => filter === "all" || item.context_type === filter);

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="mb-1.5 mt-0 text-xs font-semibold tracking-[0.02em] text-link">
            Work stays in context
          </p>
          <h1 className="m-0 font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em]">
            Messages
          </h1>
          <p className="mb-0 mt-1.5 max-w-[62ch] text-sm leading-[21px] text-muted">
            Scoped project and accepted-application conversations restricted to current verified
            participants.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <a
            className="inline-flex min-h-[38px] items-center rounded-lg border border-control bg-surface px-3.5 py-1.5 text-[13px] font-medium text-on-surface no-underline hover:bg-raised"
            href="/inbox"
          >
            Notification inbox
          </a>
          <a
            className="inline-flex min-h-[38px] items-center rounded-lg border border-control bg-surface px-3.5 py-1.5 text-[13px] font-medium text-on-surface no-underline hover:bg-raised"
            href="/projects"
          >
            My projects
          </a>
        </div>
      </header>

      {error && (
        <div
          className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-danger/35 bg-danger/10 p-4 text-sm font-medium text-danger"
          role="alert"
        >
          <span>{error}</span>
          <button
            className="inline-flex min-h-[38px] cursor-pointer items-center rounded-lg border border-control bg-surface px-3.5 py-1.5 text-[13px] font-medium text-on-surface hover:bg-raised"
            onClick={() => void load()}
            type="button"
          >
            Retry
          </button>
        </div>
      )}

      {loading && (
        <div
          className="grid min-h-48 content-center justify-items-center rounded-2xl border border-dashed border-control bg-surface p-8 text-sm text-muted"
          role="status"
        >
          Loading conversations…
        </div>
      )}

      {!loading && !error && items.length === 0 && (
        <section className="grid min-h-56 content-center justify-items-center gap-3 rounded-2xl border border-dashed border-control bg-surface p-8 text-center">
          <h2 className="m-0 text-xl font-semibold">No conversations yet</h2>
          <p className="m-0 max-w-[54ch] text-sm text-muted">
            Open a project conversation from a project you belong to, or continue an accepted
            application thread.
          </p>
          <div className="flex flex-wrap justify-center gap-2.5">
            <a
              className="inline-flex min-h-11 items-center justify-center rounded-lg border border-primary bg-primary px-4 py-2 text-sm font-medium text-on-primary no-underline hover:opacity-90"
              href="/explore"
            >
              Explore campus projects
            </a>
            <a
              className="inline-flex min-h-11 items-center justify-center rounded-lg border border-control bg-surface px-4 py-2 text-sm font-medium text-on-surface no-underline hover:bg-raised"
              href="/applications"
            >
              My applications
            </a>
          </div>
        </section>
      )}

      {!loading && items.length > 0 && (
        <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-12">
          <section
            className="lg:col-span-5 overflow-hidden rounded-2xl border border-hairline bg-surface"
            aria-label="Your conversations"
          >
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-hairline px-4 py-3.5">
              <div
                className="inline-flex gap-1 rounded-lg border border-hairline bg-raised p-[3px]"
                role="tablist"
                aria-label="Conversation type"
              >
                {([
                  ["all", `All (${items.length})`],
                  ["project", "Projects"],
                  ["application", "Applications"],
                ] as const).map(([value, label]) => (
                  <button
                    key={value}
                    type="button"
                    role="tab"
                    aria-selected={filter === value}
                    onClick={() => setFilter(value)}
                    className="min-h-[32px] cursor-pointer rounded-[6px] border-0 bg-transparent px-2.5 py-1 text-xs font-medium text-muted aria-selected:bg-surface aria-selected:text-on-surface aria-selected:shadow-xs"
                  >
                    {label}
                  </button>
                ))}
              </div>
              <button
                type="button"
                onClick={() => void load()}
                className="cursor-pointer border-0 bg-transparent p-1 text-xs font-medium text-link hover:underline"
              >
                Refresh
              </button>
            </div>

            {visibleItems.length === 0 ? (
              <p className="m-0 p-6 text-center text-sm text-muted">
                No conversations match this filter.
              </p>
            ) : (
              <div className="divide-y divide-hairline">
                {visibleItems.map((item) => (
                  <a
                    key={item.id}
                    href={`/messages/${item.id}`}
                    className="grid gap-1.5 border-l-[3px] border-l-transparent p-4 text-on-surface no-underline transition-colors duration-100 hover:border-l-link hover:bg-raised"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span
                        className={`text-xs font-medium ${
                          item.context_type === "application" ? "text-link" : "text-success"
                        }`}
                      >
                        {item.context_type === "application"
                          ? "Accepted application"
                          : item.context_type === "pbl_team" ? "PBL team"
                          : item.context_type === "pbl_faculty" ? "PBL faculty"
                          : "Project team"}
                      </span>
                      <time className="text-xs text-muted" dateTime={item.last_message_at}>
                        {new Date(item.last_message_at).toLocaleDateString()}
                      </time>
                    </div>
                    <div className="flex items-center justify-between gap-2">
                      <strong className="text-base font-semibold leading-5 [overflow-wrap:anywhere]">
                        {item.project_title || "Project conversation"}
                      </strong>
                      <span className="shrink-0 text-sm text-muted" aria-hidden="true">
                        →
                      </span>
                    </div>
                  </a>
                ))}
              </div>
            )}
          </section>

          <aside
            className="lg:col-span-7 lg:sticky lg:top-24 grid gap-4 rounded-2xl border border-hairline bg-surface p-6"
            aria-label="About scoped messaging"
          >
            <div>
              <p className="m-0 text-xs font-semibold text-link">Context &amp; safety</p>
              <h2 className="mt-1 mb-0 text-lg font-semibold">
                Select a conversation to read or reply
              </h2>
              <p className="mt-1.5 mb-0 text-sm leading-[21px] text-muted">
                PeerGit keeps messaging anchored to an active project workspace or accepted role
                application. Access is rechecked when membership, blocking, or context status
                changes.
              </p>
            </div>
            <div className="grid gap-3 border-t border-hairline pt-4 sm:grid-cols-2">
              <div className="rounded-xl border border-hairline bg-canvas p-4">
                <h3 className="m-0 text-sm font-semibold">Participants stay transparent</h3>
                <p className="mt-1 mb-0 text-xs leading-5 text-muted">
                  Every thread lists current verified members with built-in report and block
                  controls.
                </p>
              </div>
              <div className="rounded-xl border border-hairline bg-canvas p-4">
                <h3 className="m-0 text-sm font-semibold">No general DMs</h3>
                <p className="mt-1 mb-0 text-xs leading-5 text-muted">
                  Open threads directly from a project header or from{" "}
                  <a href="/applications">My applications</a> once accepted.
                </p>
              </div>
            </div>
          </aside>
        </div>
      )}
    </>
  );
}
