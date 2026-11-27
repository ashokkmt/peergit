"use client";

import { useCallback, useEffect, useEffectEvent, useRef, useState } from "react";
import { ConversationLaunchButton } from "@/components/collaboration/conversation-launch-button";
import { fetchSession, readApiEnvelope } from "@/lib/http";

type Item = {
  id: string;
  sequence: number;
  category: string;
  event_type: string;
  title: string;
  body: string;
  project_id?: string;
  application_id?: string;
  created_at: string;
  read_at?: string;
};

type InboxData = {
  items: Item[];
  next_cursor: number;
  unread_count: number;
  last_sequence: number;
};

const inboxLoadError = "Your inbox could not be loaded right now. Try again shortly.";

async function getCsrfToken(): Promise<string> {
  try {
    const response = await fetchSession();
    const result = await readApiEnvelope<{ csrf_token?: string }>(response, "");
    return result.data?.csrf_token ?? "";
  } catch {
    return "";
  }
}

function formatCategory(category: string): string {
  if (category === "applications") return "Application";
  if (category === "messages") return "Message";
  return "Project activity";
}

function formatTimestamp(iso: string): string {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleString();
}

export function Inbox() {
  const [data, setData] = useState<InboxData | null>(null);
  const [filter, setFilter] = useState<"all" | "unread">("all");
  const [selectedId, setSelectedId] = useState("");
  const [error, setError] = useState("");
  const [needsSignIn, setNeedsSignIn] = useState(false);
  const [connected, setConnected] = useState(false);
  const [loading, setLoading] = useState(true);
  const nextCursorRef = useRef(0);

  const load = useCallback(
    async (older = false) => {
      try {
        const cursor = older ? nextCursorRef.current : 0;
        const response = await fetch(
          `/api/v1/notifications?filter=${filter}${cursor ? `&after=${cursor}` : ""}`,
          { cache: "no-store" },
        );
        if (response.status === 401) {
          setNeedsSignIn(true);
          setData({ items: [], next_cursor: 0, unread_count: 0, last_sequence: 0 });
          setError("");
          return;
        }
        const result = await readApiEnvelope<InboxData>(response, inboxLoadError);
        const page = result.data;
        if (!response.ok || !page) {
          throw new Error(result.error?.message || inboxLoadError);
        }
        nextCursorRef.current = page.next_cursor;
        setNeedsSignIn(false);
        setData((previous) =>
          older && previous ? { ...page, items: [...previous.items, ...page.items] } : page,
        );
        setSelectedId((current) => {
          if (current && page.items.some((item) => item.id === current)) return current;
          return page.items[0]?.id ?? "";
        });
        setError("");
      } catch (e) {
        const message =
          e instanceof Error &&
          !e.message.includes("<!DOCTYPE") &&
          !e.message.includes("not valid JSON")
            ? e.message
            : inboxLoadError;
        setError(message);
      } finally {
        setLoading(false);
      }
    },
    [filter],
  );

  useEffect(() => {
    setLoading(true);
    void load(false);
  }, [load]);

  const canStream = data !== null && !needsSignIn;
  const streamCursor = useEffectEvent(() => data?.last_sequence ?? 0);
  const refreshInbox = useEffectEvent(() => {
    void load(false);
  });

  useEffect(() => {
    if (!canStream) return;
    // EventSource handles reconnection and replay independently of the inbox filter.
    const stream = new EventSource(`/api/v1/notifications/stream?after=${streamCursor()}`);
    stream.onopen = () => setConnected(true);
    stream.onerror = () => setConnected(false);
    stream.addEventListener("notification", refreshInbox);
    stream.addEventListener("access-check", refreshInbox);
    return () => {
      stream.close();
      setConnected(false);
    };
  }, [canStream]);

  async function markRead(id: string) {
    const response = await fetch(`/api/v1/notifications/${encodeURIComponent(id)}/read`, {
      method: "POST",
      headers: { "X-CSRF-Token": await getCsrfToken() },
    });
    if (response.ok) {
      void load(false);
    } else {
      setError("This notification could not be marked as read.");
    }
  }

  async function markAll() {
    const response = await fetch("/api/v1/notifications/read-all", {
      method: "POST",
      headers: { "X-CSRF-Token": await getCsrfToken() },
    });
    if (response.ok) {
      void load(false);
    } else {
      setError("Notifications could not be marked as read.");
    }
  }

  if (needsSignIn) {
    return (
      <section className="min-w-0 max-w-[38rem] bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3">
        <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
          Sign in to view your inbox
        </h2>
        <p className="text-sm leading-[21px] text-muted m-0">
          Project updates, application alerts, and notification preferences are linked to your
          verified PeerGit account.
        </p>
        <div>
          <a
            className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium no-underline border border-primary bg-primary text-on-primary hover:opacity-90"
            href="/login"
          >
            Sign in with GitHub
          </a>
        </div>
      </section>
    );
  }

  const items = data?.items ?? [];
  const selected = items.find((item) => item.id === selectedId) ?? items[0] ?? null;

  const filterBar = (
    <div className="px-4 py-3.5 flex items-center justify-between gap-3 border-b border-hairline flex-wrap">
      <div
        className="inline-flex p-[3px] rounded-lg bg-raised border border-hairline gap-[3px]"
        role="tablist"
        aria-label="Notification filter"
      >
        <button
          type="button"
          role="tab"
          aria-selected={filter === "all"}
          onClick={() => setFilter("all")}
          className="min-h-[34px] px-3 py-1 border-0 rounded-[6px] bg-transparent text-muted text-[13px] font-medium cursor-pointer aria-selected:bg-surface aria-selected:text-on-surface aria-selected:shadow-xs"
        >
          All
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={filter === "unread"}
          onClick={() => setFilter("unread")}
          className="min-h-[34px] px-3 py-1 border-0 rounded-[6px] bg-transparent text-muted text-[13px] font-medium cursor-pointer aria-selected:bg-surface aria-selected:text-on-surface aria-selected:shadow-xs"
        >
          Unread ({data?.unread_count ?? 0})
        </button>
      </div>

      <div className="flex items-center gap-2.5 flex-wrap">
        <span className="inline-flex flex-wrap items-center gap-2 text-muted text-xs" role="status">
          <span
            className={`w-2 h-2 rounded-full ${connected ? "bg-success" : "bg-warning"}`}
            aria-hidden="true"
          />
          {connected
            ? "Live updates connected"
            : "Live updates reconnecting · inbox is saved"}
        </span>
        <button
          type="button"
          className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium whitespace-nowrap border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised disabled:opacity-55 disabled:cursor-not-allowed"
          onClick={() => void markAll()}
          disabled={!data?.unread_count}
        >
          Mark all read
        </button>
      </div>
    </div>
  );

  if (loading && !data) {
    return (
      <section
        aria-labelledby="inbox-title"
        className="bg-surface border border-hairline rounded-xl overflow-hidden"
      >
        {filterBar}
        <p className="text-sm leading-[21px] text-muted p-6 m-0" role="status">
          Loading your inbox…
        </p>
      </section>
    );
  }

  if (items.length === 0) {
    return (
      <section
        aria-labelledby="inbox-title"
        className="bg-surface border border-hairline rounded-xl overflow-hidden"
      >
        {filterBar}
        {error && (
          <div
            className="rounded-lg px-4 py-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger m-4 flex items-center justify-between gap-3 flex-wrap"
            role="alert"
          >
            <span>{error}</span>
            <button
              type="button"
              className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised"
              onClick={() => void load(false)}
            >
              Retry
            </button>
          </div>
        )}
        <div className="grid justify-items-center content-center gap-2.5 min-h-[260px] p-8 text-center bg-surface">
          <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
            You’re all caught up
          </h2>
          <p className="text-sm leading-[21px] text-muted max-w-[500px] m-0">
            New application and project updates will appear here.
          </p>
        </div>
      </section>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {error && (
        <div
          className="rounded-lg px-4 py-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger flex items-center justify-between gap-3 flex-wrap"
          role="alert"
        >
          <span>{error}</span>
          <button
            type="button"
            className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised"
            onClick={() => void load(false)}
          >
            Retry
          </button>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-[minmax(360px,5fr)_minmax(0,7fr)] gap-5 items-start">
        <section
          className="bg-surface border border-hairline rounded-xl overflow-hidden"
          aria-label="Notifications"
        >
          {filterBar}

          <ul className="list-none p-0 m-0">
            {items.map((item) => {
              const isSelected = selected?.id === item.id;
              return (
                <li key={item.id} className="border-b border-hairline last:border-b-0">
                  <button
                    type="button"
                    aria-current={isSelected ? "true" : undefined}
                    onClick={() => setSelectedId(item.id)}
                    className="w-full py-3.5 px-4 border-0 border-l-[3px] border-l-transparent bg-surface text-on-surface text-left cursor-pointer flex flex-col gap-1.5 hover:bg-raised aria-[current=true]:bg-raised aria-[current=true]:border-l-link"
                  >
                    <div className="flex items-center justify-between gap-2 w-full">
                      <span
                        className={`text-xs font-medium leading-4 ${
                          item.category === "applications" ? "text-link" : "text-muted"
                        }`}
                      >
                        {formatCategory(item.category)}
                      </span>
                      {!item.read_at ? (
                        <span
                          className="inline-block w-2 h-2 rounded-full bg-brand shrink-0"
                          aria-label="Unread"
                        />
                      ) : (
                        <span className="text-xs font-medium leading-4 text-muted">Read</span>
                      )}
                    </div>

                    <h2 className="text-base font-semibold leading-[22px] m-0 [overflow-wrap:anywhere]">
                      {item.title}
                    </h2>

                    <p className="text-[13px] leading-[19px] text-muted m-0 [overflow-wrap:anywhere]">
                      {item.body}
                    </p>

                    <small className="block mt-0.5 text-muted text-xs">
                      {formatTimestamp(item.created_at)}
                    </small>
                  </button>
                </li>
              );
            })}
          </ul>

          {data && data.next_cursor > 0 && (
            <div className="px-4 py-3.5 border-t border-hairline text-center">
              <button
                type="button"
                className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised"
                onClick={() => void load(true)}
              >
                Load older notifications
              </button>
            </div>
          )}
        </section>

        {selected && (
          <section
            className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl overflow-hidden flex flex-col"
            aria-label="Notification details"
          >
            <div className="px-6 py-5 border-b border-hairline flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0 flex-[1_1_220px]">
                <p className="text-xs font-medium leading-4 text-muted m-0">
                  {formatCategory(selected.category)} · {formatTimestamp(selected.created_at)}
                </p>
                <p className="text-[18px] font-semibold leading-6 tracking-[-0.005em] mt-1 mb-0 [overflow-wrap:anywhere]">
                  {selected.title}
                </p>
              </div>

              <div className="flex items-center gap-2 flex-wrap">
                {!selected.read_at ? (
                  <button
                    type="button"
                    className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised"
                    onClick={() => void markRead(selected.id)}
                  >
                    Mark read
                  </button>
                ) : (
                  <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-raised border border-hairline text-xs font-medium leading-4">
                    Read
                  </span>
                )}
              </div>
            </div>

            <div className="p-6 flex flex-col gap-4">
              <div className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl p-5">
                <p className="text-sm leading-[21px] m-0 [overflow-wrap:anywhere] whitespace-pre-wrap">
                  {selected.body}
                </p>
              </div>

              {(selected.project_id || selected.application_id) && (
                <div className="flex flex-wrap gap-2.5 items-center">
                  {selected.project_id && (
                    <a
                      className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-primary bg-primary text-on-primary hover:opacity-90"
                      href={`/projects/${encodeURIComponent(selected.project_id)}?tab=work`}
                    >
                      Open project updates
                    </a>
                  )}
                  {selected.application_id && (
                    <a
                      className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-control bg-surface text-on-surface hover:bg-raised"
                      href={
                        selected.project_id
                          ? `/projects/${encodeURIComponent(selected.project_id)}?tab=roles`
                          : "/home"
                      }
                    >
                      View application context
                    </a>
                  )}
                </div>
              )}

              <div className="flex items-center justify-between flex-wrap gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px]">
                <span>
                  Direct replies happen in scoped project and application conversations with current
                  participants.
                </span>
                <div className="flex flex-wrap items-center gap-2">
                  {(selected.application_id || selected.project_id) && (
                    <ConversationLaunchButton
                      contextType={selected.application_id ? "application" : "project"}
                      resourceID={selected.application_id ?? selected.project_id!}
                      label={
                        selected.application_id
                          ? "Open application conversation"
                          : "Open project conversation"
                      }
                    />
                  )}
                  <a
                    className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-control bg-surface text-on-surface hover:bg-raised"
                    href="/messages"
                  >
                    All messages →
                  </a>
                </div>
              </div>
            </div>
          </section>
        )}
      </div>
    </div>
  );
}
