"use client";

import { useEffect, useState } from "react";
import { readApiEnvelope } from "@/lib/http";

type Project = {
  id: string;
  title: string;
  summary: string;
  visibility: string;
  lifecycle: string;
  owner?: string | null;
  repository?: string | null;
  access_state?: string | null;
  sync_health?: string | null;
};

const apiLoadError = "Projects could not be loaded right now. Please try again shortly.";

export function MyProjects() {
  const [items, setItems] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [problem, setProblem] = useState("");

  useEffect(() => {
    void fetch("/api/v1/my/projects", { cache: "no-store" })
      .then(async (r) => {
        const v = await readApiEnvelope<{ items: Project[] }>(r, apiLoadError);
        if (!r.ok) throw new Error(v.error?.message ?? apiLoadError);
        setItems(v.data?.items ?? []);
      })
      .catch((e) =>
        setProblem(
          e instanceof Error &&
            !e.message.includes("<!DOCTYPE") &&
            !e.message.includes("not valid JSON")
            ? e.message
            : apiLoadError,
        ),
      )
      .finally(() => setLoading(false));
  }, []);

  return (
    <section className="w-full flex flex-col gap-4" aria-labelledby="my-projects-title">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="mt-0 mb-1.5 text-link text-xs font-semibold tracking-[0.02em]">Your work</p>
          <h1
            id="my-projects-title"
            className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0"
          >
            My projects
          </h1>
          <p className="text-sm leading-[21px] text-muted mt-1 mb-0">
            Projects start by importing a GitHub repository you administer.
          </p>
        </div>
        <a
          className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-primary bg-primary text-on-primary hover:opacity-90"
          href="/projects/import"
        >
          Import a GitHub repository
        </a>
      </div>

      {problem && (
        <p
          className="rounded-lg px-4 py-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger m-0"
          role="alert"
        >
          {problem}
        </p>
      )}

      {loading ? (
        <p role="status" className="m-0">
          Loading your projects…
        </p>
      ) : items.length ? (
        <div className="grid gap-3.5">
          {items.map((item) => (
            <a
              key={item.id}
              className="bg-surface border border-hairline rounded-2xl p-6 no-underline text-on-surface flex flex-row flex-wrap items-center justify-between gap-4 hover:border-link transition-colors duration-100"
              href={`/projects/${item.id}`}
            >
              <div className="flex flex-col gap-1.5 flex-[1_1_260px]">
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
                    {item.title}
                  </h2>
                  <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-raised border border-hairline text-xs font-medium leading-4">
                    {item.visibility} · {item.lifecycle}
                  </span>
                </div>
                <p className="text-sm leading-[21px] text-muted m-0">{item.summary}</p>
                <p className="text-[13px] leading-[19px] text-muted m-0">
                  {item.repository
                    ? `GitHub · ${item.owner}/${item.repository} · ${item.sync_health ?? item.access_state}`
                    : "Legacy project without a repository connection"}
                </p>
              </div>
              <span className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium whitespace-nowrap border border-control bg-surface text-on-surface">
                Open project →
              </span>
            </a>
          ))}
        </div>
      ) : (
        <div className="grid justify-items-start gap-2.5 p-8 border border-dashed border-control rounded-2xl bg-surface">
          <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
            No projects yet
          </h2>
          <p className="text-sm leading-[21px] text-muted max-w-[500px] m-0">
            Connect GitHub and choose an administered repository to import it as a private project.
          </p>
          <a
            className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium no-underline border border-primary bg-primary text-on-primary hover:opacity-90"
            href="/account/github"
          >
            Connect GitHub repository
          </a>
        </div>
      )}
    </section>
  );
}
