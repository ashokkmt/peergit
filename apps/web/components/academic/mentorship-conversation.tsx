"use client";

import { FormEvent, useEffect, useState } from "react";
import { SiteFooter } from "@/components/layout/site-footer";
import { SiteHeader } from "@/components/layout/site-header";
import { readApiEnvelope } from "@/lib/http";

type Message = {
  id: string;
  sender_id: string;
  body: string;
  created_at: string;
};

type Participant = { user_id: string; display_name: string };

const secondaryBtn =
  "inline-flex min-h-[38px] cursor-pointer items-center justify-center rounded-lg border border-control bg-surface px-3.5 py-1.5 text-[13px] font-medium text-on-surface no-underline whitespace-nowrap hover:bg-raised transition-colors duration-100 disabled:cursor-not-allowed disabled:opacity-55";

export function MentorshipConversation({ conversationID }: { conversationID: string }) {
  const [viewerID, setViewerID] = useState("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [participants, setParticipants] = useState<Participant[]>([]);
  const [notice, setNotice] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try {
      const [sessionRes, msgRes] = await Promise.all([
        fetch("/api/v1/session", { cache: "no-store" }),
        fetch(`/api/v1/mentorship/conversations/${conversationID}/messages`, {
          cache: "no-store",
        }),
      ]);
      const sessionEnv = await readApiEnvelope<{ user?: { id?: string } }>(sessionRes, "");
      if (sessionRes.ok && sessionEnv.data?.user?.id) {
        setViewerID(sessionEnv.data.user.id);
      }
      const envelope = await readApiEnvelope<{
        items: Message[];
        participants?: Participant[];
      }>(msgRes, "The mentorship conversation is unavailable.");
      if (!msgRes.ok || !envelope.data) {
        throw new Error(
          envelope.error?.message ?? "The mentorship conversation is unavailable.",
        );
      }
      setMessages(envelope.data.items ?? []);
      setParticipants(envelope.data.participants ?? []);
    } catch (cause) {
      setMessages([]);
      setError(
        cause instanceof Error
          ? cause.message
          : "The mentorship conversation is unavailable.",
      );
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [conversationID]);

  async function send(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const body = new FormData(form).get("body");
    if (typeof body !== "string" || !body.trim()) return;
    setSending(true);
    setError("");
    try {
      const sessionResponse = await fetch("/api/v1/session", { cache: "no-store" });
      const session = await readApiEnvelope<{ csrf_token?: string }>(
        sessionResponse,
        "Session expired.",
      );
      const response = await fetch(
        `/api/v1/mentorship/conversations/${conversationID}/messages`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": session.data?.csrf_token ?? "",
          },
          body: JSON.stringify({
            client_message_id: crypto.randomUUID(),
            body: body.trim(),
          }),
        },
      );
      const envelope = await readApiEnvelope(response, "Message could not be sent.");
      if (!response.ok) {
        throw new Error(envelope.error?.message ?? "Message could not be sent.");
      }
      form.reset();
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Message could not be sent.");
    } finally {
      setSending(false);
    }
  }

  async function moderate(path: string, payload: Record<string, string>) {
    setError("");
    setNotice("");
    try {
      const sessionResponse = await fetch("/api/v1/session", { cache: "no-store" });
      const session = await readApiEnvelope<{ csrf_token?: string }>(
        sessionResponse,
        "Session expired.",
      );
      const response = await fetch(
        `/api/v1/mentorship/conversations/${conversationID}/${path}`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": session.data?.csrf_token ?? "",
          },
          body: JSON.stringify(payload),
        },
      );
      const envelope = await readApiEnvelope(
        response,
        "The moderation action could not be completed.",
      );
      if (!response.ok) {
        throw new Error(
          envelope.error?.message ?? "The moderation action could not be completed.",
        );
      }
      setNotice(
        path === "block"
          ? "The participant is blocked and this conversation is closed."
          : "Report submitted to campus moderators.",
      );
      await load();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "The moderation action could not be completed.",
      );
    }
  }

  const participantNames = new Map(participants.map((p) => [p.user_id, p.display_name]));

  return (
    <>
      <a
        href="#main-content"
        className="fixed -top-20 left-4 z-50 rounded-lg bg-primary px-4 py-2.5 font-semibold text-on-primary focus:top-3"
      >
        Skip to content
      </a>
      <SiteHeader workspace />
      <main
        id="main-content"
        className="mx-auto flex w-[min(1240px,calc(100%-32px))] flex-1 flex-col gap-6 py-8 md:w-[min(1240px,calc(100%-64px))] md:py-10"
      >
        <header className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <a
              href="/mentorship"
              className="text-sm font-medium text-link no-underline hover:underline"
            >
              ← Mentorship workspace
            </a>
            <p className="mb-1 mt-3 text-xs font-semibold tracking-[0.02em] text-link">
              Private context
            </p>
            <h1 className="m-0 font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em]">
              Project mentorship conversation
            </h1>
            <p className="mb-0 mt-1.5 max-w-[65ch] text-sm leading-[21px] text-muted">
              Only the project requester and verified mentor can read this conversation. Access is
              rechecked against the active, unexpired engagement on every request.
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <button className={secondaryBtn} onClick={() => void load()} type="button">
              Refresh
            </button>
            <button
              type="button"
              onClick={() => void moderate("block", {})}
              className="inline-flex min-h-[38px] cursor-pointer items-center justify-center rounded-lg border border-danger/40 bg-surface px-3.5 py-1.5 text-[13px] font-semibold text-danger hover:bg-raised"
            >
              Block participant and close this conversation
            </button>
          </div>
        </header>

        {error && (
          <div
            role="alert"
            className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-danger/35 bg-danger/10 p-4 text-sm font-medium text-danger"
          >
            <span>{error}</span>
            <button className={secondaryBtn} onClick={() => void load()} type="button">
              Retry
            </button>
          </div>
        )}

        {notice && (
          <div
            role="status"
            className="rounded-xl border border-success/35 bg-success/10 p-4 text-sm font-medium text-success"
          >
            {notice}
          </div>
        )}

        <section
          aria-label="Messages"
          aria-live="polite"
          className="flex flex-col overflow-hidden rounded-2xl border border-hairline bg-surface"
        >
          {participants.length > 0 && (
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-hairline bg-surface px-5 py-4">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-xs font-semibold text-on-surface">Participants:</span>
                {participants.map((participant) => (
                  <span
                    key={participant.user_id}
                    className="inline-flex items-center gap-2 rounded-full border border-hairline bg-raised px-3 py-1 text-xs font-medium"
                  >
                    <span
                      className="grid h-5 w-5 place-items-center rounded-full bg-surface text-[11px] font-semibold"
                      aria-hidden="true"
                    >
                      {participant.display_name.slice(0, 1).toUpperCase()}
                    </span>
                    {participant.user_id === viewerID
                      ? `${participant.display_name} (you)`
                      : participant.display_name}
                  </span>
                ))}
              </div>
              <span className="text-xs text-muted">Active engagement checked per request</span>
            </div>
          )}

          <div className="grid min-h-64 content-start gap-3 bg-canvas/35 p-4 md:p-6">
            {loading && (
              <p role="status" className="m-0 text-sm text-muted">
                Loading conversation…
              </p>
            )}
            {!loading && messages.length === 0 && !error && (
              <p className="m-0 rounded-xl border border-dashed border-control bg-surface p-6 text-center text-sm text-muted">
                No messages yet. Start with a question about the project goal.
              </p>
            )}
            {messages.map((message) => {
              const isOwn = Boolean(viewerID && message.sender_id === viewerID);
              const senderLabel = isOwn
                ? "You"
                : participantNames.get(message.sender_id) ?? "Participant";
              return (
                <article
                  key={message.id}
                  className={`grid w-fit min-w-0 max-w-full gap-2 rounded-2xl border p-4 sm:max-w-[min(560px,100%)] ${
                    isOwn
                      ? "justify-self-end border-link/30 bg-raised"
                      : "justify-self-start border-hairline bg-surface"
                  }`}
                >
                  <header className="flex flex-wrap items-center justify-between gap-3">
                    <strong className="text-sm">{senderLabel}</strong>
                    <time className="text-xs text-muted" dateTime={message.created_at}>
                      {new Date(message.created_at).toLocaleString()}
                    </time>
                  </header>
                  <p className="m-0 whitespace-pre-wrap wrap-break-word text-sm leading-6">
                    {message.body}
                  </p>
                  {!isOwn && (
                    <details className="mt-1 border-t border-hairline pt-2 text-xs">
                      <summary className="cursor-pointer font-medium text-muted hover:text-danger">
                        Report this message
                      </summary>
                      <form
                        className="mt-2.5 grid w-full max-w-full gap-2.5 rounded-xl border border-hairline bg-canvas p-3 sm:min-w-[280px]"
                        onSubmit={(event) => {
                          event.preventDefault();
                          const values = Object.fromEntries(
                            new FormData(event.currentTarget).entries(),
                          );
                          void moderate("reports", {
                            message_id: message.id,
                            reason: String(values.reason ?? "other"),
                            details: String(values.details ?? ""),
                          });
                        }}
                      >
                        <label className="grid gap-1 font-medium">
                          Reason
                          <select
                            name="reason"
                            className="min-h-9 w-full rounded-lg border border-control bg-surface px-2.5 text-sm text-on-surface"
                          >
                            <option value="spam">Spam</option>
                            <option value="harassment">Harassment</option>
                            <option value="unsafe">Unsafe content</option>
                            <option value="other">Other</option>
                          </select>
                        </label>
                        <label className="grid gap-1 font-medium">
                          Details (optional)
                          <textarea
                            name="details"
                            maxLength={1000}
                            className="min-h-16 w-full rounded-lg border border-control bg-surface p-2 text-sm text-on-surface"
                          />
                        </label>
                        <button type="submit" className={secondaryBtn}>
                          Submit report
                        </button>
                      </form>
                    </details>
                  )}
                </article>
              );
            })}
          </div>

          <form
            onSubmit={(event) => void send(event)}
            className="grid gap-3 border-t border-hairline bg-surface p-4 md:p-6"
          >
            <label htmlFor="message-body" className="text-[13px] font-semibold">
              Write a message
            </label>
            <textarea
              id="message-body"
              name="body"
              maxLength={5000}
              required
              className="min-h-24 resize-y rounded-xl border border-control bg-canvas p-3 text-sm text-on-surface focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-link"
              placeholder="Share a useful question, suggestion, or next step."
            />
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="m-0 text-xs leading-5 text-muted">
                Messages are available only while this mentorship context is active.
              </p>
              <button
                type="submit"
                disabled={sending}
                className="inline-flex min-h-10 cursor-pointer items-center justify-center rounded-lg border border-primary bg-primary px-4 py-2 text-[13px] font-semibold text-on-primary hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-55"
              >
                {sending ? "Sending…" : "Send message"}
              </button>
            </div>
          </form>
        </section>
      </main>
      <SiteFooter />
    </>
  );
}
