"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { readApiEnvelope } from "@/lib/http";

type Participant = { id: string; handle: string; display_name: string };
type Message = {
  id: string;
  sender_id: string;
  sender_name: string;
  body: string;
  created_at: string;
};
type ConversationData = {
  items: Message[];
  next_cursor?: string | null;
  context_type: "project" | "application";
  project_id: string;
  application_id?: string;
  state: "open" | "closed";
  participants: Participant[];
};

const errorText =
  "Messages could not be loaded. This context may have closed or your access may have changed.";
const primaryButton =
  "inline-flex min-h-11 cursor-pointer items-center justify-center rounded-lg border border-primary bg-primary px-4 py-2 text-sm font-medium text-on-primary hover:opacity-90 transition-opacity duration-100 disabled:cursor-not-allowed disabled:opacity-60";
const secondaryButton =
  "inline-flex min-h-[38px] cursor-pointer items-center justify-center rounded-lg border border-control bg-surface px-3.5 py-1.5 text-[13px] font-medium text-on-surface no-underline whitespace-nowrap hover:bg-raised transition-colors duration-100 disabled:cursor-not-allowed disabled:opacity-60";

function mergeMessages(current: Message[], incoming: Message[]) {
  const byID = new Map(current.map((item) => [item.id, item]));
  for (const item of incoming) byID.set(item.id, item);
  return [...byID.values()].sort((a, b) => a.created_at.localeCompare(b.created_at));
}

export function ConversationRoom({ conversationID }: { conversationID: string }) {
  const [csrf, setCSRF] = useState("");
  const [viewerID, setViewerID] = useState("");
  const [context, setContext] = useState<ConversationData | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [olderCursor, setOlderCursor] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [pendingID, setPendingID] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const load = useCallback(
    async (cursor?: string, replace = false) => {
      if (replace) setLoading(true);
      setError("");
      try {
        const url = new URL(
          `/api/v1/conversations/${conversationID}/messages`,
          window.location.origin,
        );
        if (cursor) url.searchParams.set("cursor", cursor);
        const response = await fetch(url, { cache: "no-store" });
        const result = await readApiEnvelope<ConversationData>(response, errorText);
        if (!response.ok || !result.data) {
          if ([401, 403, 404].includes(response.status)) {
            setContext(null);
            setMessages([]);
            setOlderCursor(null);
          }
          throw new Error(result.error?.message ?? "Conversation is closed or no longer available.");
        }
        setContext(result.data);
        setMessages((current) => mergeMessages(replace ? [] : current, result.data?.items ?? []));
        setOlderCursor(result.data.next_cursor ?? null);
      } catch (reason) {
        setError(reason instanceof Error ? reason.message : errorText);
        if (replace) {
          setContext(null);
          setMessages([]);
        }
      } finally {
        if (replace) setLoading(false);
      }
    },
    [conversationID],
  );

  useEffect(() => {
    let active = true;
    void fetch("/api/v1/session", { cache: "no-store" })
      .then(async (response) => {
        const result = await readApiEnvelope<{ csrf_token?: string; user?: { id?: string } }>(
          response,
          errorText,
        );
        if (!response.ok) throw new Error(result.error?.message ?? errorText);
        if (active) {
          setCSRF(result.data?.csrf_token ?? "");
          setViewerID(result.data?.user?.id ?? "");
        }
      })
      .catch((reason) => active && setError(reason instanceof Error ? reason.message : errorText));
    void load(undefined, true);
    return () => {
      active = false;
    };
  }, [load]);

  useEffect(() => {
    const revalidate = () => {
      if (document.visibilityState === "visible") void load();
    };
    window.addEventListener("focus", revalidate);
    return () => window.removeEventListener("focus", revalidate);
  }, [load]);

  async function send(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const body = draft.trim();
    if (!body || sending) return;
    const clientMessageID = pendingID || crypto.randomUUID();
    setPendingID(clientMessageID);
    setSending(true);
    setError("");
    setNotice("");
    try {
      const response = await fetch(`/api/v1/conversations/${conversationID}/messages`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ client_message_id: clientMessageID, body }),
      });
      const result = await readApiEnvelope<{ id: string }>(
        response,
        "Message could not be sent. Retry without losing its text.",
      );
      if (!response.ok) throw new Error(result.error?.message ?? "Message could not be sent.");
      setDraft("");
      setPendingID("");
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Message could not be sent. Retry the same message.",
      );
    } finally {
      setSending(false);
    }
  }

  async function blockUser(userID: string) {
    setError("");
    setNotice("");
    try {
      const response = await fetch(`/api/v1/blocks/${userID}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: "{}",
      });
      const result = await readApiEnvelope<{ blocked: boolean }>(
        response,
        "This account could not be blocked.",
      );
      if (!response.ok) throw new Error(result.error?.message ?? "This account could not be blocked.");
      setNotice("This account is blocked. Their messages and message alerts are hidden from you.");
      await load(undefined, true);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "This account could not be blocked.");
    }
  }

  if (loading && !context) {
    return (
      <p className="m-0 text-sm text-muted" role="status">
        Opening conversation…
      </p>
    );
  }

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <a className="text-sm font-medium text-link no-underline hover:underline" href="/messages">
            ← All messages
          </a>
          <div className="mt-2.5 flex flex-wrap items-center gap-2">
            <h1 className="m-0 font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em]">
              {context?.context_type === "application"
                ? "Application conversation"
                : "Project team conversation"}
            </h1>
            {context && (
              <span
                className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${
                  context.state === "open"
                    ? "border-success/35 bg-success/10 text-success"
                    : "border-hairline bg-raised text-muted"
                }`}
              >
                {context.state === "open" ? "Open" : "Closed"}
              </span>
            )}
          </div>
          <p className="mb-0 mt-1.5 text-sm text-muted">
            {context ? (
              <>
                Project{" "}
                <a className="font-mono text-xs text-link" href={`/projects/${context.project_id}`}>
                  {context.project_id}
                </a>
                {context.context_type === "application" && context.application_id
                  ? ` · Application ${context.application_id}`
                  : ""}
              </>
            ) : (
              "Context unavailable"
            )}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {context && (
            <a className={secondaryButton} href={`/projects/${context.project_id}`}>
              Open project →
            </a>
          )}
          <button
            className={secondaryButton}
            onClick={() => void load(undefined, true)}
            type="button"
          >
            Refresh messages
          </button>
        </div>
      </header>

      {error && (
        <div
          className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-danger/35 bg-danger/10 p-4 text-sm font-medium text-danger"
          role="alert"
        >
          <span>{error}</span>
          <button
            className={secondaryButton}
            onClick={() => void load(undefined, true)}
            type="button"
          >
            Retry
          </button>
        </div>
      )}

      {notice && (
        <p
          className="m-0 rounded-xl border border-success/35 bg-success/10 p-4 text-sm font-medium text-success"
          role="status"
        >
          {notice}
        </p>
      )}

      <section
        className="flex flex-col overflow-hidden rounded-2xl border border-hairline bg-surface"
        aria-label="Conversation workspace"
      >
        {context && (
          <div
            className="grid gap-2 border-b border-hairline bg-surface p-5 sm:px-6"
            aria-label="Conversation participants"
          >
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h2 className="m-0 text-sm font-semibold">Participants</h2>
              <span className="text-xs text-muted">
                Access checked against active membership &amp; blocks
              </span>
            </div>
            <div className="flex flex-wrap gap-2">
              {context.participants.map((person) => (
                <span
                  className="inline-flex items-center gap-2 rounded-full border border-hairline bg-raised px-3 py-1 text-xs font-medium"
                  key={person.id}
                >
                  <span
                    className="grid h-5 w-5 place-items-center rounded-full bg-surface text-[11px] font-semibold text-on-surface"
                    aria-hidden="true"
                  >
                    {person.display_name.slice(0, 1).toUpperCase()}
                  </span>
                  {person.id === viewerID ? `${person.display_name} (you)` : person.display_name}
                </span>
              ))}
            </div>
          </div>
        )}

        <div
          className="grid min-h-64 content-start gap-3 bg-canvas/35 p-4 sm:p-6"
          aria-label="Conversation messages"
          aria-busy={loading}
        >
          {olderCursor && (
            <div className="flex justify-center">
              <button
                className={secondaryButton}
                disabled={loading}
                onClick={() => void load(olderCursor)}
                type="button"
              >
                {loading ? "Loading…" : "Load older messages"}
              </button>
            </div>
          )}

          {messages.length === 0 && !loading && (
            <p className="m-0 py-12 text-center text-sm text-muted">
              No messages yet. Keep project decisions and coordination in context here.
            </p>
          )}

          {messages.map((message) => {
            const isOwn = message.sender_id === viewerID;
            return (
              <article
                className={`w-fit min-w-0 max-w-full sm:max-w-[min(560px,100%)] rounded-2xl border p-4 ${
                  isOwn
                    ? "justify-self-end border-link/30 bg-raised"
                    : "justify-self-start border-hairline bg-surface"
                }`}
                key={message.id}
              >
                <header className="flex flex-wrap items-center justify-between gap-3">
                  <strong className="text-sm">{isOwn ? "You" : message.sender_name}</strong>
                  <time className="text-xs text-muted" dateTime={message.created_at}>
                    {new Date(message.created_at).toLocaleString()}
                  </time>
                </header>
                <p className="mb-0 mt-2 whitespace-pre-wrap wrap-break-word text-sm leading-6">
                  {message.body}
                </p>
                {!isOwn && (
                  <div className="mt-3 flex flex-wrap items-center gap-3 border-t border-hairline pt-2.5">
                    <details className="w-full text-xs">
                      <summary className="cursor-pointer font-medium text-muted hover:text-danger">
                        Report message
                      </summary>
                      <ReportMessage
                        conversationID={conversationID}
                        csrf={csrf}
                        messageID={message.id}
                        onResult={setNotice}
                        onError={setError}
                      />
                    </details>
                    <button
                      className="cursor-pointer border-0 bg-transparent p-0 text-xs font-medium text-muted underline hover:text-danger"
                      onClick={() => void blockUser(message.sender_id)}
                      type="button"
                    >
                      Block {message.sender_name}
                    </button>
                  </div>
                )}
              </article>
            );
          })}
        </div>

        {context?.state === "open" ? (
          <form
            className="grid gap-3 border-t border-hairline bg-surface p-4 sm:px-6 sm:py-5"
            onSubmit={(event) => void send(event)}
          >
            <label className="grid gap-1.5 text-sm font-medium" htmlFor="message-body">
              Write a message
              <textarea
                className="min-h-24 w-full rounded-lg border border-control bg-surface p-3 text-sm text-on-surface"
                id="message-body"
                maxLength={5000}
                onChange={(event) => setDraft(event.target.value)}
                placeholder="Keep your message connected to this project…"
                value={draft}
              />
            </label>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <span className="text-xs text-muted">
                Retained in context only. No typing indicators or online presence tracking.
              </span>
              <button
                className={primaryButton}
                disabled={sending || !draft.trim()}
                type="submit"
              >
                {sending ? "Sending…" : "Send message"}
              </button>
            </div>
          </form>
        ) : (
          !loading &&
          !error && (
            <p className="m-0 border-t border-hairline bg-raised p-4 text-sm text-muted">
              This conversation context is closed to new replies.
            </p>
          )
        )}
      </section>
    </>
  );
}

function ReportMessage({
  conversationID,
  csrf,
  messageID,
  onResult,
  onError,
}: {
  conversationID: string;
  csrf: string;
  messageID: string;
  onResult: (message: string) => void;
  onError: (message: string) => void;
}) {
  const [reason, setReason] = useState("harassment");
  const [details, setDetails] = useState("");
  const [sending, setSending] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSending(true);
    onError("");
    try {
      const response = await fetch(`/api/v1/conversations/${conversationID}/reports`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ message_id: messageID, reason, details }),
      });
      const result = await readApiEnvelope<{ reported: boolean }>(
        response,
        "Report could not be submitted.",
      );
      if (!response.ok) throw new Error(result.error?.message ?? "Report could not be submitted.");
      onResult("Report received. A moderator review queue will be available in a later phase.");
    } catch (error) {
      onError(error instanceof Error ? error.message : "Report could not be submitted.");
    } finally {
      setSending(false);
    }
  }

  return (
    <form
      className="mt-2.5 grid w-full max-w-full sm:min-w-[280px] gap-2.5 rounded-xl border border-hairline bg-canvas p-3"
      onSubmit={(event) => void submit(event)}
    >
      <label className="grid gap-1 font-medium text-on-surface">
        Reason
        <select
          className="min-h-9 w-full rounded-lg border border-control bg-surface px-2.5 text-on-surface"
          onChange={(event) => setReason(event.target.value)}
          value={reason}
        >
          <option value="spam">Spam</option>
          <option value="harassment">Harassment</option>
          <option value="unsafe">Unsafe content</option>
          <option value="other">Other</option>
        </select>
      </label>
      <label className="grid gap-1 font-medium text-on-surface">
        Details (optional)
        <textarea
          className="min-h-16 w-full rounded-lg border border-control bg-surface p-2 text-on-surface"
          maxLength={1000}
          onChange={(event) => setDetails(event.target.value)}
          value={details}
        />
      </label>
      <button className={secondaryButton} disabled={sending} type="submit">
        {sending ? "Sending…" : "Submit report"}
      </button>
    </form>
  );
}
