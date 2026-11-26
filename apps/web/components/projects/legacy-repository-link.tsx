"use client";

import { useEffect, useState } from "react";
import { readApiEnvelope } from "@/lib/http";

type Candidate = { id: number; full_name: string; installation_id: string; visibility: string };

export function LegacyRepositoryLink({ projectID, csrf }: { projectID: string; csrf: string }) {
  const [items, setItems] = useState<Candidate[]>([]);
  const [selected, setSelected] = useState("");
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    void fetch("/api/v1/me/github/repositories", { cache: "no-store" })
      .then(async (response) => {
        const result = await readApiEnvelope<{ items: Candidate[] }>(
          response,
          "GitHub repositories could not be loaded.",
        );
        if (!response.ok) {
          throw new Error(result.error?.message ?? "GitHub repositories could not be loaded.");
        }
        setItems(result.data?.items ?? []);
      })
      .catch((error) =>
        setProblem(
          error instanceof Error ? error.message : "GitHub repositories could not be loaded.",
        ),
      );
  }, []);

  async function link() {
    setBusy(true);
    setProblem("");
    try {
      const response = await fetch(`/api/v1/projects/${projectID}/github/link`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ repository_id: Number(selected) }),
      });
      const result = await readApiEnvelope<{ authorization_url: string }>(
        response,
        "Repository connection could not be started.",
      );
      if (!response.ok || !result.data?.authorization_url) {
        throw new Error(result.error?.message ?? "Repository connection could not be started.");
      }
      window.location.assign(result.data.authorization_url);
    } catch (error) {
      setProblem(
        error instanceof Error ? error.message : "Repository connection could not be started.",
      );
      setBusy(false);
    }
  }

  return (
    <section
      className="min-w-0 max-w-[38rem] bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-3.5"
      aria-labelledby="legacy-repository-title"
    >
      <h2
        id="legacy-repository-title"
        className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0"
      >
        Connect a GitHub repository
      </h2>
      <p className="text-sm leading-[21px] text-muted m-0">
        This existing project stays private until you connect a repository and publish it.
      </p>
      {problem && (
        <p
          className="rounded-lg px-4 py-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger m-0"
          role="alert"
        >
          {problem}
        </p>
      )}
      {items.length ? (
        <div className="grid gap-3">
          <label
            className="grid gap-1.5 text-[13px] font-medium text-on-surface"
            htmlFor="legacy-repository"
          >
            Repository you administer
            <select
              id="legacy-repository"
              className="w-full min-h-11 px-3.5 py-2.5 border border-control rounded-lg bg-surface text-on-surface text-sm leading-5"
              value={selected}
              onChange={(event) => setSelected(event.target.value)}
            >
              <option value="">Choose a repository</option>
              {items.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.full_name} · {item.visibility}
                </option>
              ))}
            </select>
          </label>
          <div>
            <button
              className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium border border-primary bg-primary text-on-primary cursor-pointer hover:opacity-90 disabled:opacity-55 disabled:cursor-not-allowed"
              type="button"
              disabled={!selected || busy}
              onClick={() => void link()}
            >
              {busy ? "Connecting…" : "Connect repository"}
            </button>
          </div>
        </div>
      ) : (
        <p className="text-sm leading-[21px] text-muted m-0">
          No available repository. <a href="/account/github">Connect or refresh GitHub</a> first.
        </p>
      )}
    </section>
  );
}
