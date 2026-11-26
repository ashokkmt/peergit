"use client";

import { useEffect, useState } from "react";
import { readApiEnvelope } from "@/lib/http";

type Conversation = {
  id: string;
  context_type: "project" | "application";
  project_id: string;
  application_id?: string;
  project_title: string;
  last_message_at: string;
};

const errorText = "Your conversations could not be loaded. Refresh to check current campus access.";

export function MessagesInbox() {
  const [items, setItems] = useState<Conversation[]>([]);
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

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="mb-1.5 mt-0 text-xs font-semibold text-link">Work stays in context</p>
          <h1 className="m-0 font-display text-3xl font-semibold tracking-tight">Messages</h1>
          <p className="mb-0 mt-2 max-w-[60ch] text-sm text-muted">
            Conversations are available only to current project teammates and accepted application participants.
          </p>
        </div>
        <a className="inline-flex min-h-10 items-center rounded-lg border border-control bg-surface px-3.5 py-2 text-sm font-medium text-on-surface no-underline hover:bg-raised" href="/projects">
          My projects
        </a>
      </header>

      {error && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-danger/35 bg-danger/10 p-4 text-sm text-danger" role="alert">
          <span>{error}</span>
          <button className="rounded-lg border border-control bg-surface px-3.5 py-2 text-sm text-on-surface" onClick={() => void load()} type="button">Retry</button>
        </div>
      )}
      {loading && <p className="m-0 text-sm text-muted" role="status">Loading conversations…</p>}
      {!loading && !error && items.length === 0 && (
        <section className="grid min-h-56 content-center justify-items-center gap-3 rounded-2xl border border-dashed border-control bg-surface p-8 text-center">
          <h2 className="m-0 text-xl font-semibold">No conversations yet</h2>
          <p className="m-0 max-w-[54ch] text-sm text-muted">Open a project conversation from a project you belong to, or an accepted application thread.</p>
          <a className="rounded-lg border border-primary bg-primary px-4 py-2.5 text-sm font-medium text-on-primary no-underline" href="/explore">Explore campus projects</a>
        </section>
      )}
      <section className="grid gap-3" aria-label="Your conversations">
        {items.map((item) => (
          <a className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-hairline bg-surface p-5 text-on-surface no-underline hover:border-link" href={`/messages/${item.id}`} key={item.id}>
            <span className="grid gap-1">
              <strong>{item.project_title || "Project conversation"}</strong>
              <span className="text-sm text-muted">{item.context_type === "application" ? "Accepted application" : "Project team"}</span>
            </span>
            <time className="text-xs text-muted" dateTime={item.last_message_at}>{new Date(item.last_message_at).toLocaleString()}</time>
          </a>
        ))}
      </section>
    </>
  );
}
