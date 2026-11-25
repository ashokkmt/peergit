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
    <section
      className="content account-content"
      aria-labelledby="my-projects-title"
      style={{ display: "flex", flexDirection: "column", gap: "18px" }}
    >
      <div className="section-heading" style={{ marginBottom: 0 }}>
        <div>
          <p className="eyebrow">Your work</p>
          <h1 id="my-projects-title" className="t-headline-md">
            My projects
          </h1>
          <p className="t-body-md muted" style={{ margin: "4px 0 0" }}>
            Projects start by importing a GitHub repository you administer.
          </p>
        </div>
        <a className="primary-link pg-btn-compact" href="/projects/import">
          Import a GitHub repository
        </a>
      </div>

      {problem && (
        <p className="form-error" role="alert">
          {problem}
        </p>
      )}

      {loading ? (
        <p role="status">Loading your projects…</p>
      ) : items.length ? (
        <div style={{ display: "grid", gap: "14px" }}>
          {items.map((item) => (
            <a
              key={item.id}
              className="pg-card-interactive"
              href={`/projects/${item.id}`}
              style={{
                flexDirection: "row",
                flexWrap: "wrap",
                alignItems: "center",
                gap: "16px",
              }}
            >
              <div style={{ display: "flex", flexDirection: "column", gap: "6px", flex: "1 1 260px" }}>
                <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: "8px" }}>
                  <h2 className="t-title-md">{item.title}</h2>
                  <span className="pg-pill pg-pill-raised">
                    {item.visibility} · {item.lifecycle}
                  </span>
                </div>
                <p className="t-body-md muted" style={{ margin: 0 }}>
                  {item.summary}
                </p>
                <p className="t-body-sm muted" style={{ margin: 0 }}>
                  {item.repository
                    ? `GitHub · ${item.owner}/${item.repository} · ${item.sync_health ?? item.access_state}`
                    : "Legacy project without a repository connection"}
                </p>
              </div>
              <span className="secondary-button pg-btn-compact">Open project →</span>
            </a>
          ))}
        </div>
      ) : (
        <div className="empty-state">
          <h2 className="t-title-md">No projects yet</h2>
          <p className="t-body-md muted">
            Connect your GitHub App and import a repository to create your first PeerGit project.
          </p>
        </div>
      )}
    </section>
  );
}
