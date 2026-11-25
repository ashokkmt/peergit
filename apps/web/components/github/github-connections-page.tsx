"use client";

import { useEffect, useState } from "react";
import { GitHubImports } from "./github-imports";

export function GitHubConnectionsPage() {
  const [csrf, setCSRF] = useState("");
  const [problem, setProblem] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    void fetch("/api/v1/session", { cache: "no-store", signal: controller.signal })
      .then(async response => {
        const result = await response.json() as { data?: { authenticated: boolean; csrf_token?: string } };
        if (!result.data?.authenticated) throw new Error("Sign in to manage GitHub connections.");
        if (!result.data.csrf_token) throw new Error("Your session could not be verified.");
        setCSRF(result.data.csrf_token);
      })
      .catch(error => {
        if (!controller.signal.aborted) setProblem(error instanceof Error ? error.message : "Account session could not be loaded.");
      });
    return () => controller.abort();
  }, []);

  if (problem) return <section className="panel"><p className="form-error" role="alert">{problem}</p><a href="/login">Sign in with GitHub</a></section>;
  if (!csrf) return <p role="status">Loading GitHub account…</p>;
  return <GitHubImports csrf={csrf} />;
}
