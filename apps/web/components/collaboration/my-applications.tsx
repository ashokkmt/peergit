"use client";

import { useEffect, useState } from "react";
import { ConversationLaunchButton } from "@/components/collaboration/conversation-launch-button";
import { readApiEnvelope } from "@/lib/http";

type Application = {
  id: string;
  project_id: string;
  project_title: string;
  role_title: string;
  status: "pending" | "accepted" | "rejected" | "withdrawn";
  created_at: string;
};

const errorText = "Your applications could not be loaded. Try again when campus access is available.";

export function MyApplications() {
  const [items, setItems] = useState<Application[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try {
      const response = await fetch("/api/v1/my/applications", { cache: "no-store" });
      const result = await readApiEnvelope<{ items: Application[] }>(response, errorText);
      if (!response.ok) throw new Error(result.error?.message ?? errorText);
      setItems(result.data?.items ?? []);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : errorText);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <>
      <header>
        <p className="mb-1.5 mt-0 text-xs font-semibold text-link">Team formation</p>
        <h1 className="m-0 font-display text-3xl font-semibold tracking-tight">My applications</h1>
        <p className="mb-0 mt-2 text-sm text-muted">Accepted applications can continue in a context-scoped conversation.</p>
      </header>
      {error && <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-danger/35 bg-danger/10 p-4 text-sm text-danger" role="alert"><span>{error}</span><button className="rounded-lg border border-control bg-surface px-3.5 py-2 text-sm text-on-surface" onClick={() => void load()} type="button">Retry</button></div>}
      {loading && <p className="m-0 text-sm text-muted" role="status">Loading applications…</p>}
      {!loading && !error && items.length === 0 && <section className="grid min-h-56 content-center justify-items-center gap-3 rounded-2xl border border-dashed border-control bg-surface p-8 text-center"><h2 className="m-0 text-xl font-semibold">No applications yet</h2><p className="m-0 text-sm text-muted">Explore open roles to find a project that fits what you want to learn.</p><a className="rounded-lg border border-primary bg-primary px-4 py-2.5 text-sm font-medium text-on-primary no-underline" href="/explore">Find open roles</a></section>}
      <section className="grid gap-3" aria-label="Your project applications">
        {items.map((item) => (
          <article className="flex flex-wrap items-center justify-between gap-4 rounded-2xl border border-hairline bg-surface p-5" key={item.id}>
            <div className="grid gap-1">
              <a className="font-semibold text-on-surface" href={`/projects/${item.project_id}`}>{item.project_title}</a>
              <span className="text-sm text-muted">{item.role_title} · {item.status}</span>
              <time className="text-xs text-muted" dateTime={item.created_at}>{new Date(item.created_at).toLocaleDateString()}</time>
            </div>
            {item.status === "accepted" && <ConversationLaunchButton contextType="application" resourceID={item.id} />}
          </article>
        ))}
      </section>
    </>
  );
}
