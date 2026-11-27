"use client";

import { useEffect, useState } from "react";
import { fetchSession, readApiEnvelope } from "@/lib/http";

type SavedProject = {
  id: string;
  slug: string;
  title: string;
  summary: string;
  visibility: string;
  lifecycle: string;
  following: boolean;
  bookmarked: boolean;
};

export function SavedProjects() {
  const [items, setItems] = useState<SavedProject[]>([]);
  const [csrf, setCSRF] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  async function load() {
    setLoading(true);
    setError("");
    try {
      const [sessionResponse, savedResponse] = await Promise.all([
        fetchSession(),
        fetch("/api/v1/me/saved-projects", { cache: "no-store" }),
      ]);
      const session = await readApiEnvelope<{ csrf_token?: string }>(
        sessionResponse,
        "Saved projects are unavailable.",
      );
      const saved = await readApiEnvelope<{ items?: SavedProject[] }>(
        savedResponse,
        "Saved projects are unavailable.",
      );
      if (!sessionResponse.ok || !savedResponse.ok) {
        throw new Error(saved.error?.message ?? "Saved projects are unavailable.");
      }
      setCSRF(session.data?.csrf_token ?? "");
      setItems(saved.data?.items ?? []);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Saved projects could not be loaded.");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function remove(project: SavedProject, kind: "follow" | "bookmark") {
    try {
      const response = await fetch(`/api/v1/projects/${project.id}/${kind}`, {
        method: "DELETE",
        headers: { "X-CSRF-Token": csrf },
        credentials: "same-origin",
      });
      if (!response.ok) throw new Error("Could not update this saved project.");
      setItems((current) =>
        current
          .map((item) =>
            item.id === project.id
              ? { ...item, [kind === "follow" ? "following" : "bookmarked"]: false }
              : item,
          )
          .filter((item) => item.following || item.bookmarked),
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not update this saved project.");
    }
  }

  return (
    <section className="py-7 pb-16 flex flex-col gap-4">
      <div>
        <p className="mt-0 mb-1.5 text-link text-xs font-semibold tracking-[0.02em]">Your library</p>
        <h1 className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0">
          Saved projects
        </h1>
        <p className="text-sm leading-[21px] text-muted mt-1 mb-0">
          Projects you follow or have bookmarked for later.
        </p>
      </div>

      {error && (
        <div
          className="flex flex-wrap items-center justify-between gap-2.5 px-3.5 py-3 border border-danger rounded-lg bg-surface text-danger"
          role="alert"
        >
          <span>{error}</span>
          <button
            className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised"
            type="button"
            onClick={() => void load()}
          >
            Retry
          </button>
        </div>
      )}

      {loading ? (
        <div
          className="p-6 border border-hairline rounded-2xl bg-surface shadow-[var(--shadow)]"
          aria-live="polite"
        >
          Loading saved projects…
        </div>
      ) : items.length === 0 ? (
        <div className="grid justify-items-start max-w-[880px] p-8 border border-hairline rounded-2xl bg-surface shadow-[var(--shadow)]">
          <span className="grid place-items-center w-[46px] h-[46px] rounded-2xl bg-brand/14 text-primary text-[21px] font-bold">
            ☆
          </span>
          <h3 className="font-display text-xl font-bold leading-tight mt-4 mb-1">
            Your saved list is empty
          </h3>
          <p className="m-0 text-muted">
            Follow a project for updates or bookmark it to return later.
          </p>
          <a className="mt-4 font-semibold" href="/explore">
            Explore projects →
          </a>
        </div>
      ) : (
        <div className="grid gap-4 max-w-[880px]">
          {items.map((item) => (
            <article
              className="min-w-0 p-6 border border-hairline rounded-2xl bg-surface shadow-[var(--shadow)]"
              key={item.id}
            >
              <div className="flex flex-wrap items-center justify-between gap-3.5">
                <div>
                  <span className="inline-block px-2.5 py-1 rounded-full bg-raised text-muted text-xs capitalize">
                    {item.visibility}
                  </span>
                  <h2 className="font-display text-[19px] font-bold leading-snug mt-2 mb-0">
                    <a href={`/projects/${item.id}`}>{item.title}</a>
                  </h2>
                </div>
                <span className="px-2.5 py-1 rounded-full bg-raised text-muted text-xs">
                  {item.following ? "Following" : "Saved"}
                </span>
              </div>
              <p className="my-4 whitespace-pre-wrap [overflow-wrap:anywhere]">{item.summary}</p>
              <div className="flex flex-wrap items-center gap-3.5 border-t border-hairline pt-3">
                {item.following && (
                  <button
                    className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised"
                    type="button"
                    onClick={() => void remove(item, "follow")}
                  >
                    Unfollow
                  </button>
                )}
                {item.bookmarked && (
                  <button
                    className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised"
                    type="button"
                    onClick={() => void remove(item, "bookmark")}
                  >
                    Remove bookmark
                  </button>
                )}
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
