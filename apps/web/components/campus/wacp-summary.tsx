"use client";

import { useEffect, useState } from "react";
import { readApiEnvelope } from "@/lib/http";

type Summary = {
  metric: string;
  value: number;
  freshness_at: string | null;
  sources: string[];
  descriptive_only: boolean;
};

export function WACPSummary() {
  const [summary, setSummary] = useState<Summary | null>(null);
  const [message, setMessage] = useState("Loading authorized campus summary…");

  async function load() {
    setMessage("Loading authorized campus summary…");
    try {
      const response = await fetch("/api/v1/campus/insights/wacp", { cache: "no-store" });
      const result = await readApiEnvelope<Summary>(response, "Collaboration summary is unavailable.");
      if (!response.ok || !result.data) {
        throw new Error(result.error?.message ?? "Campus administrator access is required for this aggregate.");
      }
      setSummary(result.data);
      setMessage("");
    } catch (error) {
      setSummary(null);
      setMessage(error instanceof Error ? error.message : "Collaboration summary is unavailable.");
    }
  }

  useEffect(() => {
    void load();
    const revalidate = () => {
      if (document.visibilityState === "visible") void load();
    };
    window.addEventListener("focus", revalidate);
    return () => window.removeEventListener("focus", revalidate);
  }, []);

  return (
    <section className="grid gap-4 rounded-2xl border border-hairline bg-surface p-6" aria-labelledby="wacp-title">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="mb-1 mt-0 text-xs font-semibold text-link">Trailing 7 days</p>
          <h2 id="wacp-title" className="m-0 text-xl font-semibold">Weekly active collaborating projects</h2>
        </div>
        {summary?.descriptive_only && <span className="rounded-full border border-hairline bg-raised px-3 py-1 text-xs font-medium">Descriptive aggregate</span>}
      </div>
      {summary ? (
        <div className="grid gap-4 sm:grid-cols-[auto_1fr] sm:items-end">
          <strong className="font-display text-5xl font-semibold tabular-nums">{summary.value}</strong>
          <div className="grid gap-1 text-sm text-muted">
            <span>{summary.sources.length ? `Observed sources: ${summary.sources.map((source) => source.replaceAll("_", " ")).join(", ")}` : "No qualifying activity observed in this window."}</span>
            <span>
              {summary.freshness_at
                ? `Source data recorded ${new Date(summary.freshness_at).toLocaleString()}`
                : "No qualifying source activity has been recorded in this window."}
            </span>
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap items-center justify-between gap-3 text-sm text-muted" role="status">
          <span>{message}</span>
          <button className="rounded-lg border border-control bg-surface px-3 py-1.5 text-sm text-on-surface" onClick={() => void load()} type="button">Retry</button>
        </div>
      )}
      <p className="m-0 border-t border-hairline pt-3 text-xs leading-5 text-muted">
        A project counts when at least two distinct members have a meaningful recorded action. Views, likes alone, messages, and duplicate provider syncs are excluded. This is not a student score or grade.
      </p>
    </section>
  );
}
