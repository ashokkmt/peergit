"use client";

import { useCallback, useEffect, useState } from "react";
import { readApiEnvelope } from "@/lib/http";

type FeedItem = {
  id: string;
  project_id: string;
  project_title: string;
  actor_name: string;
  kind: "update" | "discussion";
  summary: string;
  created_at: string;
};

type FeedPage = {
  items: FeedItem[];
  next_cursor?: string | null;
};

const errorText = "Campus activity could not be loaded. Check your campus access and retry.";
const buttonClass =
  "inline-flex min-h-10 items-center justify-center rounded-lg border border-control bg-surface px-3.5 py-2 text-sm font-medium text-on-surface hover:bg-raised disabled:cursor-not-allowed disabled:opacity-60";

export function CampusFeed() {
  const [scope, setScope] = useState<"campus" | "following">("campus");
  const [items, setItems] = useState<FeedItem[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async (nextScope: "campus" | "following", nextCursor?: string) => {
    setLoading(true);
    setError("");
    try {
      const url = new URL("/api/v1/feed", window.location.origin);
      url.searchParams.set("scope", nextScope);
      if (nextCursor) url.searchParams.set("cursor", nextCursor);
      const response = await fetch(url, { cache: "no-store" });
      const result = await readApiEnvelope<FeedPage>(response, errorText);
      if (!response.ok) {
        if ([401, 403, 404].includes(response.status)) {
          setItems([]);
          setCursor(null);
        }
        throw new Error(result.error?.message ?? errorText);
      }
      setItems((current) => (nextCursor ? [...current, ...(result.data?.items ?? [])] : result.data?.items ?? []));
      setCursor(result.data?.next_cursor ?? null);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : errorText);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load(scope);
  }, [load, scope]);

  useEffect(() => {
    const revalidate = () => {
      if (document.visibilityState === "visible") void load(scope);
    };
    window.addEventListener("focus", revalidate);
    return () => window.removeEventListener("focus", revalidate);
  }, [load, scope]);

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="mb-1.5 mt-0 text-xs font-semibold text-link">Your campus, in motion</p>
          <h1 className="m-0 font-display text-3xl font-semibold tracking-tight">Project feed</h1>
          <p className="mb-0 mt-2 max-w-[58ch] text-sm text-muted">
            Follow the work students choose to share. Activity is campus-scoped and never a productivity score.
          </p>
        </div>
        <a className={buttonClass} href="/explore">Search campus work</a>
      </header>

      <div className="flex gap-2" role="group" aria-label="Feed scope">
        {(["campus", "following"] as const).map((value) => (
          <button
            className={`min-h-10 rounded-lg border px-4 text-sm font-medium ${scope === value ? "border-primary bg-primary text-on-primary" : "border-control bg-surface text-on-surface hover:bg-raised"}`}
            key={value}
            type="button"
            aria-pressed={scope === value}
            onClick={() => setScope(value)}
          >
            {value === "campus" ? "Campus" : "Following"}
          </button>
        ))}
      </div>

      {error && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-danger/35 bg-danger/10 p-4 text-sm text-danger" role="alert">
          <span>{error}</span>
          <button className={buttonClass} onClick={() => void load(scope)} type="button">Retry</button>
        </div>
      )}

      {loading && items.length === 0 && <p className="m-0 text-sm text-muted" role="status">Loading campus activity…</p>}
      {!loading && !error && items.length === 0 && (
        <section className="grid min-h-56 content-center justify-items-center gap-3 rounded-2xl border border-dashed border-control bg-surface p-8 text-center">
          <h2 className="m-0 text-xl font-semibold">{scope === "campus" ? "The campus feed is quiet" : "Follow a project to start your feed"}</h2>
          <p className="m-0 max-w-[52ch] text-sm text-muted">
            {scope === "campus" ? "New project updates and discussions will appear here when teams share them." : "Following uses your current project follows and respects each project's visibility."}
          </p>
          <a className={buttonClass} href="/explore">Explore projects</a>
        </section>
      )}

      <section className="grid gap-3" aria-label="Campus project activity" aria-busy={loading}>
        {items.map((item) => (
          <article className="grid gap-3 rounded-2xl border border-hairline bg-surface p-5 sm:p-6" key={item.id}>
            <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted">
              <span>{item.kind === "update" ? "Project update" : "Discussion"} · {item.actor_name}</span>
              <time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString()}</time>
            </div>
            <h2 className="m-0 text-lg font-semibold">
              <a className="text-on-surface" href={`/projects/${item.project_id}`}>{item.project_title}</a>
            </h2>
            <p className="m-0 whitespace-pre-wrap text-sm leading-6 text-muted">{item.summary}</p>
          </article>
        ))}
      </section>

      {cursor && !error && (
        <div className="flex justify-center">
          <button className={buttonClass} disabled={loading} onClick={() => void load(scope, cursor)} type="button">
            {loading ? "Loading…" : "Load older activity"}
          </button>
        </div>
      )}
    </>
  );
}
