"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { PreviewBadge } from "@/components/layout/preview-banner";
import { readApiEnvelope } from "@/lib/http";

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
    const response = await fetch("/api/v1/session", { cache: "no-store" });
    const result = await readApiEnvelope<{ csrf_token?: string }>(response, "");
    return result.data?.csrf_token ?? "";
  } catch {
    return "";
  }
}

function formatCategory(category: string): string {
  return category === "applications" ? "Application" : "Project activity";
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

  const lastSequence = data?.last_sequence;
  useEffect(() => {
    if (lastSequence === undefined || needsSignIn) return;
    const stream = new EventSource(`/api/v1/notifications/stream?after=${lastSequence}`);
    stream.onopen = () => setConnected(true);
    stream.onerror = () => setConnected(false);
    stream.addEventListener("notification", () => {
      void load(false);
    });
    stream.addEventListener("access-check", () => {
      void load(false);
    });
    return () => {
      stream.close();
      setConnected(false);
    };
  }, [lastSequence, needsSignIn, load]);

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
      <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px", maxWidth: "38rem" }}>
        <h2 className="t-title-lg">Sign in to view your inbox</h2>
        <p className="t-body-md muted" style={{ margin: 0 }}>
          Project updates, application alerts, and notification preferences are linked to your
          verified PeerGit account.
        </p>
        <div>
          <a className="primary-link" href="/login">
            Sign in with GitHub
          </a>
        </div>
      </section>
    );
  }

  const items = data?.items ?? [];
  const selected = items.find((item) => item.id === selectedId) ?? items[0] ?? null;

  const filterBar = (
    <div
      style={{
        padding: "14px 16px",
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        gap: "12px",
        borderBottom: "1px solid var(--hairline)",
        flexWrap: "wrap",
      }}
    >
      <div className="inbox-tabs" role="tablist" aria-label="Notification filter">
        <button
          type="button"
          role="tab"
          aria-selected={filter === "all"}
          onClick={() => setFilter("all")}
        >
          All
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={filter === "unread"}
          onClick={() => setFilter("unread")}
        >
          Unread ({data?.unread_count ?? 0})
        </button>
      </div>

      <div style={{ display: "flex", alignItems: "center", gap: "10px", flexWrap: "wrap" }}>
        <span
          className={`service-status ${connected ? "ready" : ""}`}
          role="status"
          style={{ marginTop: 0 }}
        >
          <span className="status-dot" aria-hidden="true" />
          {connected
            ? "Live updates connected"
            : "Live updates reconnecting · inbox is saved"}
        </span>
        <button
          type="button"
          className="secondary-button pg-btn-compact"
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
      <section aria-labelledby="inbox-title" className="split-list-panel">
        {filterBar}
        <p className="t-body-md muted" style={{ padding: "24px", margin: 0 }} role="status">
          Loading your inbox…
        </p>
      </section>
    );
  }

  if (items.length === 0) {
    return (
      <section aria-labelledby="inbox-title" className="split-list-panel">
        {filterBar}
        {error && (
          <div
            className="form-error"
            role="alert"
            style={{
              margin: "16px",
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              gap: "12px",
              flexWrap: "wrap",
            }}
          >
            <span>{error}</span>
            <button
              type="button"
              className="secondary-button pg-btn-compact"
              onClick={() => void load(false)}
            >
              Retry
            </button>
          </div>
        )}
        <div className="empty-state" style={{ border: 0, borderRadius: 0, minHeight: "260px" }}>
          <h2 className="t-title-md">You’re all caught up</h2>
          <p className="t-body-md muted">
            New application and project updates will appear here.
          </p>
        </div>
      </section>
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
      {error && (
        <div
          className="form-error"
          role="alert"
          style={{
            margin: 0,
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: "12px",
            flexWrap: "wrap",
          }}
        >
          <span>{error}</span>
          <button
            type="button"
            className="secondary-button pg-btn-compact"
            onClick={() => void load(false)}
          >
            Retry
          </button>
        </div>
      )}

      <div className="split-layout inbox-split">
        <section className="split-list-panel" aria-label="Notifications">
          {filterBar}

          <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
            {items.map((item) => {
              const isSelected = selected?.id === item.id;
              return (
                <li
                  key={item.id}
                  style={{ borderBottom: "1px solid var(--hairline)" }}
                >
                  <button
                    type="button"
                    className="list-item"
                    aria-current={isSelected ? "true" : undefined}
                    onClick={() => setSelectedId(item.id)}
                    style={{
                      display: "flex",
                      flexDirection: "column",
                      gap: "6px",
                      borderBottom: 0,
                    }}
                  >
                    <div
                      style={{
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "space-between",
                        gap: "8px",
                        width: "100%",
                      }}
                    >
                      <span
                        className="t-label-sm"
                        style={{
                          color: item.category === "applications" ? "var(--link)" : "var(--muted)",
                        }}
                      >
                        {formatCategory(item.category)}
                      </span>
                      {!item.read_at ? (
                        <span className="brand-dot" aria-label="Unread" />
                      ) : (
                        <span className="t-label-sm muted">Read</span>
                      )}
                    </div>

                    <h2 className="t-title-sm" style={{ margin: 0, overflowWrap: "anywhere" }}>
                      {item.title}
                    </h2>

                    <p
                      className="t-body-sm muted"
                      style={{ margin: 0, overflowWrap: "anywhere" }}
                    >
                      {item.body}
                    </p>

                    <small style={{ marginTop: "2px" }}>{formatTimestamp(item.created_at)}</small>
                  </button>
                </li>
              );
            })}
          </ul>

          {data && data.next_cursor > 0 && (
            <div
              style={{
                padding: "14px 16px",
                borderTop: "1px solid var(--hairline)",
                textAlign: "center",
              }}
            >
              <button
                type="button"
                className="secondary-button pg-btn-compact"
                onClick={() => void load(true)}
              >
                Load older notifications
              </button>
            </div>
          )}
        </section>

        {selected && (
          <section
            className="pg-card"
            aria-label="Notification details"
            style={{
              padding: 0,
              overflow: "hidden",
              display: "flex",
              flexDirection: "column",
            }}
          >
            <div
              style={{
                padding: "20px 24px",
                borderBottom: "1px solid var(--hairline)",
                display: "flex",
                flexWrap: "wrap",
                alignItems: "flex-start",
                justifyContent: "space-between",
                gap: "12px",
              }}
            >
              <div style={{ minWidth: 0, flex: "1 1 220px" }}>
                <p className="t-label-sm muted" style={{ margin: 0 }}>
                  {formatCategory(selected.category)} · {formatTimestamp(selected.created_at)}
                </p>
                <p
                  className="t-title-md"
                  style={{ margin: "4px 0 0", overflowWrap: "anywhere" }}
                >
                  {selected.title}
                </p>
              </div>

              <div style={{ display: "flex", alignItems: "center", gap: "8px", flexWrap: "wrap" }}>
                {!selected.read_at ? (
                  <button
                    type="button"
                    className="secondary-button pg-btn-compact"
                    onClick={() => void markRead(selected.id)}
                  >
                    Mark read
                  </button>
                ) : (
                  <span className="pg-pill pg-pill-raised">Read</span>
                )}
              </div>
            </div>

            <div
              style={{
                padding: "24px",
                display: "flex",
                flexDirection: "column",
                gap: "16px",
              }}
            >
              <div className="pg-card-canvas">
                <p
                  className="t-body-md"
                  style={{ margin: 0, overflowWrap: "anywhere", whiteSpace: "pre-wrap" }}
                >
                  {selected.body}
                </p>
              </div>

              {(selected.project_id || selected.application_id) && (
                <div className="button-row">
                  {selected.project_id && (
                    <a
                      className="primary-link pg-btn-compact"
                      href={`/projects/${encodeURIComponent(selected.project_id)}?tab=work`}
                    >
                      Open project updates
                    </a>
                  )}
                  {selected.application_id && (
                    <a
                      className="secondary-button pg-btn-compact"
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

              <div className="pg-callout" style={{ alignItems: "center", justifyContent: "space-between", flexWrap: "wrap" }}>
                <span>
                  Contextual direct replies for project and application threads ship separately from
                  notifications.
                </span>
                <PreviewBadge phase="Phase 5C" />
              </div>
            </div>

            <form
              style={{
                padding: "16px 24px",
                borderTop: "1px solid var(--hairline)",
                display: "flex",
                flexWrap: "wrap",
                gap: "10px",
              }}
            >
              <label className="sr-only" htmlFor="reply-box">
                Contextual messaging preview
              </label>
              <input
                id="reply-box"
                className="pg-input"
                style={{ flex: "1 1 200px" }}
                disabled
                placeholder="Direct replies will get implemented in a later phase"
              />
              <button className="primary-button pg-btn-compact" disabled type="button">
                Send
              </button>
            </form>
          </section>
        )}
      </div>
    </div>
  );
}
