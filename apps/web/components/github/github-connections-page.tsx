"use client";

import { useEffect, useState } from "react";
import { GitHubImports } from "./github-imports";
import { readApiEnvelope } from "@/lib/http";

export function GitHubConnectionsPage() {
  const [csrf, setCSRF] = useState("");
  const [problem, setProblem] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    void fetch("/api/v1/session", { cache: "no-store", signal: controller.signal })
      .then(async (response) => {
        const result = await readApiEnvelope<{ authenticated: boolean; csrf_token?: string }>(
          response,
          "Account session could not be loaded.",
        );
        if (!result.data?.authenticated) throw new Error("Sign in to manage GitHub connections.");
        if (!result.data.csrf_token) throw new Error("Your session could not be verified.");
        setCSRF(result.data.csrf_token);
      })
      .catch((error) => {
        if (!controller.signal.aborted) {
          setProblem(
            error instanceof Error ? error.message : "Account session could not be loaded.",
          );
        }
      });
    return () => controller.abort();
  }, []);

  if (problem) {
    return (
      <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3">
        <p
          className="rounded-lg px-4 py-3 m-0 text-sm font-medium bg-danger/12 border border-danger/35 text-danger"
          role="alert"
        >
          {problem}
        </p>
        <div>
          <a href="/login">Sign in with GitHub</a>
        </div>
      </section>
    );
  }
  if (!csrf) return <p role="status">Loading GitHub account…</p>;
  return <GitHubImports csrf={csrf} />;
}
