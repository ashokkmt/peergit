"use client";

import { useState } from "react";
import { readApiEnvelope } from "@/lib/http";

type ContextType = "project" | "application";

const buttonClass =
  "inline-flex min-h-10 items-center justify-center rounded-lg border border-control bg-surface px-3.5 py-2 text-sm font-medium text-on-surface hover:bg-raised disabled:cursor-not-allowed disabled:opacity-60";

export function ConversationLaunchButton({
  contextType,
  resourceID,
  label = "Open conversation",
}: {
  contextType: ContextType;
  resourceID: string;
  label?: string;
}) {
  const [csrf, setCSRF] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function openConversation() {
    setBusy(true);
    setError("");
    try {
      let token = csrf;
      if (!token) {
        const sessionResponse = await fetch("/api/v1/session", { cache: "no-store" });
        const session = await readApiEnvelope<{ csrf_token?: string }>(sessionResponse, "Sign in before opening a conversation.");
        if (!sessionResponse.ok) throw new Error(session.error?.message ?? "Sign in before opening a conversation.");
        token = session.data?.csrf_token ?? "";
        setCSRF(token);
      }
      const endpoint = contextType === "project"
        ? `/api/v1/projects/${resourceID}/conversation`
        : `/api/v1/applications/${resourceID}/conversation`;
      const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": token },
        body: "{}",
      });
      const result = await readApiEnvelope<{ id: string }>(response, "Conversation could not be opened.");
      if (!response.ok || !result.data?.id) {
        throw new Error(result.error?.message ?? "This project or application no longer allows messaging.");
      }
      window.location.assign(`/messages/${result.data.id}`);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Conversation could not be opened.");
      setBusy(false);
    }
  }

  return (
    <span className="inline-grid justify-items-start gap-1.5">
      <button className={buttonClass} disabled={busy} onClick={() => void openConversation()} type="button">
        {busy ? "Opening…" : label}
      </button>
      {error && <span className="max-w-[36ch] text-xs text-danger" role="alert">{error}</span>}
    </span>
  );
}
