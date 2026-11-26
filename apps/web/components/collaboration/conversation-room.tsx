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

const errorText = "Messages could not be loaded. This context may have closed or your access may have changed.";
const primaryButton =
  "inline-flex min-h-11 items-center justify-center rounded-lg border border-primary bg-primary px-4 py-2 text-sm font-medium text-on-primary hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60";
const secondaryButton =
  "inline-flex min-h-10 items-center justify-center rounded-lg border border-control bg-surface px-3.5 py-2 text-sm font-medium text-on-surface hover:bg-raised disabled:cursor-not-allowed disabled:opacity-60";

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

  const load = useCallback(async (cursor?: string, replace = false) => {
    if (replace) setLoading(true);
    setError("");
    try {
      const url = new URL(`/api/v1/conversations/${conversationID}/messages`, window.location.origin);
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
  }, [conversationID]);

  useEffect(() => {
    let active = true;
    void fetch("/api/v1/session", { cache: "no-store" })
      .then(async (response) => {
        const result = await readApiEnvelope<{ csrf_token?: string; user?: { id?: string } }>(response, errorText);
        if (!response.ok) throw new Error(result.error?.message ?? errorText);
        if (active) {
          setCSRF(result.data?.csrf_token ?? "");
          setViewerID(result.data?.user?.id ?? "");
        }
      })
      .catch((reason) => active && setError(reason instanceof Error ? reason.message : errorText));
    void load(undefined, true);
    return () => { active = false; };
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
      const result = await readApiEnvelope<{ id: string }>(response, "Message could not be sent. Retry without losing its text.");
      if (!response.ok) throw new Error(result.error?.message ?? "Message could not be sent.");
      setDraft("");
      setPendingID("");
      await load();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Message could not be sent. Retry the same message.");
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
      const result = await readApiEnvelope<{ blocked: boolean }>(response, "This account could not be blocked.");
      if (!response.ok) throw new Error(result.error?.message ?? "This account could not be blocked.");
      setNotice("This account is blocked. Their messages and message alerts are hidden from you.");
      await load(undefined, true);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "This account could not be blocked.");
    }
  }

  if (loading && !context) {
    return <p className="m-0 text-sm text-muted" role="status">Opening conversation…</p>;
  }

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <a className="text-sm font-medium" href="/messages">← All messages</a>
          <h1 className="mb-0 mt-3 font-display text-3xl font-semibold tracking-tight">
            {context?.context_type === "application" ? "Application conversation" : "Project team conversation"}
          </h1>
          <p className="mb-0 mt-2 text-sm text-muted">
            {context ? `Project ${context.project_id}${context.context_type === "application" ? ` · Application ${context.application_id}` : ""}` : "Context unavailable"}
          </p>
        </div>
        <button className={secondaryButton} onClick={() => void load(undefined, true)} type="button">Refresh messages</button>
      </header>

      {context && (
        <section className="grid gap-2 rounded-xl border border-hairline bg-raised p-4" aria-label="Conversation participants">
          <h2 className="m-0 text-sm font-semibold">Participants</h2>
          <div className="flex flex-wrap gap-2">
            {context.participants.map((person) => (
              <span className="rounded-full border border-hairline bg-surface px-3 py-1.5 text-xs" key={person.id}>
                {person.id === viewerID ? `${person.display_name} (you)` : person.display_name}
              </span>
            ))}
          </div>
          <p className="m-0 text-xs text-muted">Membership and application access are checked again whenever this conversation is opened or refreshed.</p>
        </section>
      )}

      {error && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-danger/35 bg-danger/10 p-4 text-sm text-danger" role="alert">
          <span>{error}</span>
          <button className={secondaryButton} onClick={() => void load(undefined, true)} type="button">Retry</button>
        </div>
      )}
      {notice && <p className="m-0 rounded-xl border border-success/35 bg-success/10 p-4 text-sm text-success" role="status">{notice}</p>}

      {olderCursor && (
        <button className={secondaryButton} disabled={loading} onClick={() => void load(olderCursor)} type="button">
          {loading ? "Loading…" : "Load older messages"}
        </button>
      )}

      <section className="grid min-h-64 content-start gap-3 rounded-2xl border border-hairline bg-surface p-4 sm:p-6" aria-label="Conversation messages" aria-busy={loading}>
        {messages.length === 0 && !loading && <p className="m-0 py-12 text-center text-sm text-muted">No messages yet. Keep the project context with the conversation.</p>}
        {messages.map((message) => (
          <article className={`w-fit min-w-0 max-w-full rounded-2xl border border-hairline p-4 ${message.sender_id === viewerID ? "justify-self-end bg-raised" : "justify-self-start bg-canvas"}`} key={message.id}>
            <header className="flex flex-wrap items-center justify-between gap-3">
              <strong className="text-sm">{message.sender_id === viewerID ? "You" : message.sender_name}</strong>
              <time className="text-xs text-muted" dateTime={message.created_at}>{new Date(message.created_at).toLocaleString()}</time>
            </header>
            <p className="mb-0 mt-2 whitespace-pre-wrap wrap-break-word text-sm leading-6">{message.body}</p>
            {message.sender_id !== viewerID && (
              <div className="mt-3 flex flex-wrap gap-3">
                <details className="text-xs">
                  <summary className="cursor-pointer text-muted">Report message</summary>
                  <ReportMessage conversationID={conversationID} csrf={csrf} messageID={message.id} onResult={setNotice} onError={setError} />
                </details>
                <button className="border-0 bg-transparent p-0 text-xs text-muted underline" onClick={() => void blockUser(message.sender_id)} type="button">Block {message.sender_name}</button>
              </div>
            )}
          </article>
        ))}
      </section>

      {context?.state === "open" && (
        <form className="grid gap-3 rounded-2xl border border-hairline bg-surface p-4 sm:p-5" onSubmit={(event) => void send(event)}>
          <label className="grid gap-1.5 text-sm font-medium" htmlFor="message-body">Write a message
            <textarea className="min-h-28 w-full rounded-lg border border-control bg-surface p-3 text-sm text-on-surface" id="message-body" maxLength={5000} onChange={(event) => setDraft(event.target.value)} placeholder="Keep your message connected to this project…" value={draft} />
          </label>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <span className="text-xs text-muted">Messages are retained in the project or application context. No typing or presence tracking.</span>
            <button className={primaryButton} disabled={sending || !draft.trim()} type="submit">{sending ? "Sending…" : "Send message"}</button>
          </div>
        </form>
      )}
      {!context && !loading && !error && <p className="m-0 text-sm text-muted">This context is closed.</p>}
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
      const result = await readApiEnvelope<{ reported: boolean }>(response, "Report could not be submitted.");
      if (!response.ok) throw new Error(result.error?.message ?? "Report could not be submitted.");
      onResult("Report received. A moderator review queue will be available in a later phase.");
    } catch (error) {
      onError(error instanceof Error ? error.message : "Report could not be submitted.");
    } finally {
      setSending(false);
    }
  }

  return (
    <form className="mt-3 grid min-w-[min(320px,80vw)] gap-2 rounded-xl border border-hairline bg-surface p-3" onSubmit={(event) => void submit(event)}>
      <label className="grid gap-1">Reason
        <select className="min-h-9 rounded-lg border border-control bg-surface px-2 text-on-surface" onChange={(event) => setReason(event.target.value)} value={reason}>
          <option value="spam">Spam</option>
          <option value="harassment">Harassment</option>
          <option value="unsafe">Unsafe content</option>
          <option value="other">Other</option>
        </select>
      </label>
      <label className="grid gap-1">Details (optional)
        <textarea className="min-h-16 rounded-lg border border-control bg-surface p-2 text-on-surface" maxLength={1000} onChange={(event) => setDetails(event.target.value)} value={details} />
      </label>
      <button className={secondaryButton} disabled={sending} type="submit">{sending ? "Sending…" : "Submit report"}</button>
    </form>
  );
}
