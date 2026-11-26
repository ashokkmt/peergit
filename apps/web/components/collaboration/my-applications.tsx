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

function statusBadgeClass(status: Application["status"]): string {
  if (status === "accepted") {
    return "border-success/35 bg-success/10 text-success";
  }
  if (status === "pending") {
    return "border-warning/35 bg-warning/10 text-warning";
  }
  return "border-hairline bg-raised text-muted";
}

function statusLabel(status: Application["status"]): string {
  if (status === "rejected") return "Declined";
  return status.charAt(0).toUpperCase() + status.slice(1);
}

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
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="mb-1.5 mt-0 text-xs font-semibold tracking-[0.02em] text-link">
            Team formation
          </p>
          <h1 className="m-0 font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em]">
            My applications
          </h1>
          <p className="mb-0 mt-1.5 text-sm leading-[21px] text-muted">
            Track role decisions and continue accepted applications in a context-scoped
            conversation.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <a
            className="inline-flex min-h-[38px] items-center justify-center rounded-lg border border-control bg-surface px-3.5 py-1.5 text-[13px] font-medium text-on-surface no-underline hover:bg-raised"
            href="/messages"
          >
            All messages
          </a>
          <a
            className="inline-flex min-h-[38px] items-center justify-center rounded-lg border border-control bg-surface px-3.5 py-1.5 text-[13px] font-medium text-on-surface no-underline hover:bg-raised"
            href="/explore"
          >
            Explore roles
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
            className="inline-flex min-h-[38px] cursor-pointer items-center justify-center rounded-lg border border-control bg-surface px-3.5 py-1.5 text-[13px] font-medium text-on-surface hover:bg-raised"
            onClick={() => void load()}
            type="button"
          >
            Retry
          </button>
        </div>
      )}

      {loading && (
        <p className="m-0 text-sm text-muted" role="status">
          Loading applications…
        </p>
      )}

      {!loading && !error && items.length === 0 && (
        <section className="grid min-h-56 content-center justify-items-center gap-3 rounded-2xl border border-dashed border-control bg-surface p-8 text-center">
          <h2 className="m-0 text-xl font-semibold">No applications yet</h2>
          <p className="m-0 max-w-[52ch] text-sm text-muted">
            Explore open roles to find a project that fits what you want to learn.
          </p>
          <a
            className="inline-flex min-h-11 items-center justify-center rounded-lg border border-primary bg-primary px-4 py-2 text-sm font-medium text-on-primary no-underline hover:opacity-90"
            href="/explore"
          >
            Find open roles
          </a>
        </section>
      )}

      {items.length > 0 && (
        <section className="grid gap-3" aria-label="Your project applications">
          {items.map((item) => (
            <article
              className="flex flex-wrap items-center justify-between gap-4 rounded-2xl border border-hairline bg-surface p-5 sm:p-6"
              key={item.id}
            >
              <div className="grid min-w-0 gap-1.5">
                <div className="flex flex-wrap items-center gap-2">
                  <span
                    className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${statusBadgeClass(item.status)}`}
                  >
                    {statusLabel(item.status)}
                  </span>
                  <time className="text-xs text-muted" dateTime={item.created_at}>
                    Submitted {new Date(item.created_at).toLocaleDateString()}
                  </time>
                </div>
                <a
                  className="text-base font-semibold text-on-surface no-underline hover:text-link hover:underline"
                  href={`/projects/${item.project_id}`}
                >
                  {item.project_title}
                </a>
                <span className="text-sm text-muted">Role: {item.role_title}</span>
              </div>
              <div className="flex flex-wrap items-center gap-2.5">
                <a
                  className="inline-flex min-h-[38px] items-center justify-center rounded-lg border border-control bg-surface px-3.5 py-1.5 text-[13px] font-medium text-on-surface no-underline hover:bg-raised"
                  href={`/projects/${item.project_id}?tab=open-roles`}
                >
                  View project →
                </a>
                {item.status === "accepted" && (
                  <ConversationLaunchButton contextType="application" resourceID={item.id} />
                )}
              </div>
            </article>
          ))}
        </section>
      )}
    </>
  );
}
