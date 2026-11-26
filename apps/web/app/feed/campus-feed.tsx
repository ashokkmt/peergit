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

type FeedPageData = {
  items: FeedItem[];
  next_cursor?: string | null;
};

const errorText = "Campus activity could not be loaded. Check your campus access and retry.";
const secondaryBtnClass =
  "inline-flex min-h-[38px] cursor-pointer items-center justify-center rounded-lg border border-control bg-surface px-3.5 py-1.5 text-[13px] font-medium text-on-surface no-underline whitespace-nowrap hover:bg-raised transition-colors duration-100 disabled:cursor-not-allowed disabled:opacity-60";

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
      const result = await readApiEnvelope<FeedPageData>(response, errorText);
      if (!response.ok) {
        if ([401, 403, 404].includes(response.status)) {
          setItems([]);
          setCursor(null);
        }
        throw new Error(result.error?.message ?? errorText);
      }
      setItems((current) =>
        nextCursor ? [...current, ...(result.data?.items ?? [])] : result.data?.items ?? [],
      );
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
          <p className="mb-1.5 mt-0 text-xs font-semibold tracking-[0.02em] text-link">
            Your campus, in motion
          </p>
          <h1 className="m-0 font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em]">
            Project feed
          </h1>
          <p className="mb-0 mt-1.5 max-w-[60ch] text-sm leading-[21px] text-muted">
            Updates and discussions shared across visible campus projects. Activity stays strictly
            campus-scoped and never scores contributors.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2.5">
          <div
            className="inline-flex rounded-[10px] border border-hairline bg-raised p-1 gap-1"
            role="group"
            aria-label="Feed scope"
          >
            {(["campus", "following"] as const).map((value) => (
              <button
                key={value}
                type="button"
                aria-pressed={scope === value}
                onClick={() => setScope(value)}
                className="min-h-[34px] cursor-pointer rounded-[7px] border-0 bg-transparent px-3.5 py-1 text-[13px] font-medium text-muted aria-pressed:bg-surface aria-pressed:text-on-surface aria-pressed:shadow-xs"
              >
                {value === "campus" ? "Campus" : "Following"}
              </button>
            ))}
          </div>
          <a className={secondaryBtnClass} href="/explore">
            Search campus work
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
            className={secondaryBtnClass}
            onClick={() => void load(scope)}
            type="button"
          >
            Retry
          </button>
        </div>
      )}

      {loading && items.length === 0 && (
        <div
          className="grid min-h-48 content-center justify-items-center rounded-2xl border border-dashed border-control bg-surface p-8 text-sm text-muted"
          role="status"
        >
          Loading campus activity…
        </div>
      )}

      {!loading && !error && items.length === 0 && (
        <section className="grid min-h-56 content-center justify-items-center gap-3 rounded-2xl border border-dashed border-control bg-surface p-8 text-center">
          <h2 className="m-0 text-xl font-semibold">
            {scope === "campus"
              ? "The campus feed is quiet"
              : "Follow a project to start your feed"}
          </h2>
          <p className="m-0 max-w-[52ch] text-sm text-muted">
            {scope === "campus"
              ? "New project updates and discussions will appear here when teams share them."
              : "Following uses your current project follows and respects each project's visibility."}
          </p>
          <a
            className="inline-flex min-h-11 items-center justify-center rounded-lg border border-primary bg-primary px-4 py-2 text-sm font-medium text-on-primary no-underline hover:opacity-90"
            href="/explore"
          >
            Explore projects
          </a>
        </section>
      )}

      {items.length > 0 && (
        <section
          className="grid gap-3.5"
          aria-label="Campus project activity"
          aria-busy={loading}
        >
          {items.map((item) => (
            <article
              className="grid gap-3 rounded-2xl border border-hairline bg-surface p-5 sm:p-6 transition-colors duration-100 hover:border-control"
              key={item.id}
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex flex-wrap items-center gap-2 text-xs">
                  <span
                    className={`inline-flex items-center rounded-full border px-2.5 py-0.5 font-medium ${
                      item.kind === "update"
                        ? "border-link/30 bg-link/10 text-link"
                        : "border-hairline bg-raised text-on-surface"
                    }`}
                  >
                    {item.kind === "update" ? "Project update" : "Discussion"}
                  </span>
                  <span className="font-medium text-on-surface">{item.actor_name}</span>
                </div>
                <time className="text-xs text-muted" dateTime={item.created_at}>
                  {new Date(item.created_at).toLocaleString()}
                </time>
              </div>

              <h2 className="m-0 text-lg font-semibold leading-6 tracking-[-0.01em]">
                <a
                  className="text-on-surface no-underline hover:text-link hover:underline"
                  href={`/projects/${item.project_id}`}
                >
                  {item.project_title}
                </a>
              </h2>

              <p className="m-0 whitespace-pre-wrap wrap-break-word text-sm leading-6 text-muted">
                {item.summary}
              </p>

              <div className="flex flex-wrap items-center justify-between gap-2 border-t border-hairline pt-3 text-xs">
                <a
                  className="font-medium text-link no-underline hover:underline"
                  href={`/projects/${item.project_id}?tab=work`}
                >
                  Open project updates →
                </a>
                <a
                  className="font-medium text-muted no-underline hover:text-on-surface"
                  href={`/projects/${item.project_id}`}
                >
                  View repository
                </a>
              </div>
            </article>
          ))}
        </section>
      )}

      {cursor && !error && (
        <div className="flex justify-center pt-2">
          <button
            className={secondaryBtnClass}
            disabled={loading}
            onClick={() => void load(scope, cursor)}
            type="button"
          >
            {loading ? "Loading…" : "Load older activity"}
          </button>
        </div>
      )}
    </>
  );
}
