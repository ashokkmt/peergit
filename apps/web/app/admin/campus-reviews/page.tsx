"use client";

import { useEffect, useState } from "react";
import { CampusConsoleShell } from "@/components/campus/campus-console-shell";
import { readApiEnvelope } from "@/lib/http";

type Review = {
  id: string;
  user_id: string;
  email: string;
  reason: string;
  created_at: string;
};

export default function CampusReviewsPage() {
  const [csrf, setCSRF] = useState("");
  const [items, setItems] = useState<Review[]>([]);
  const [reason, setReason] = useState("");
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const sessionRes = await fetch("/api/v1/session", { cache: "no-store" });
      const session = await readApiEnvelope<{
        authenticated: boolean;
        user?: { campus_admin: boolean; mfa_verified: boolean };
        csrf_token?: string;
      }>(sessionRes, "Campus review queue could not load.");
      if (!session.data?.authenticated) {
        location.assign("/login");
        return;
      }
      setCSRF(session.data.csrf_token ?? "");
      if (!session.data.user?.campus_admin || !session.data.user.mfa_verified) {
        setProblem("A campus administrator session with fresh MFA is required.");
        return;
      }
      const listRes = await fetch("/api/v1/admin/campus-review-requests", {
        cache: "no-store",
      });
      const result = await readApiEnvelope<{ items: Review[] }>(
        listRes,
        "Campus review queue could not load.",
      );
      if (result.error) throw new Error(result.error.message);
      setItems(result.data?.items ?? []);
    } catch (e) {
      setProblem(e instanceof Error ? e.message : "Campus review queue could not load.");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function decide(id: string, decision: "approved" | "rejected") {
    setProblem("");
    setMessage("");
    try {
      const response = await fetch(`/api/v1/admin/campus-review-requests/${id}/decision`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ decision, reason }),
      });
      const result = await readApiEnvelope<{ state: string }>(
        response,
        "Review decision failed.",
      );
      if (!response.ok || result.error) {
        throw new Error(result.error?.message ?? "Review decision failed.");
      }
      setItems((value) => value.filter((item) => item.id !== id));
      setReason("");
      setMessage(
        `Request ${decision}. Administrative approval does not verify mailbox ownership.`,
      );
    } catch (e) {
      setProblem(e instanceof Error ? e.message : "Review decision failed.");
    }
  }

  return (
    <CampusConsoleShell title="Campus administration" subtitle="Fresh MFA required">
      <section id="reviews" aria-labelledby="review-title" className="flex flex-col gap-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p className="mt-0 mb-1.5 text-link text-xs font-semibold tracking-[0.02em]">
              Campus administration
            </p>
            <h1
              id="review-title"
              className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0"
            >
              Campus verification reviews
            </h1>
            <p className="text-sm leading-[21px] text-muted mt-1.5 mb-0 max-w-[60ch]">
              Review campus affiliation requests for your campus. Your decision needs fresh MFA and
              a reason. Approval grants student access but does not verify a mailbox.
            </p>
          </div>
          <a
            className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-control bg-surface text-on-surface hover:bg-raised"
            href="/home"
          >
            Workspace
          </a>
        </div>

        <section className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-4">
          <label
            className="grid gap-1.5 text-[13px] font-medium text-on-surface"
            htmlFor="review-reason"
          >
            Decision reason
            <textarea
              id="review-reason"
              className="w-full min-h-24 px-3.5 py-2.5 border border-control rounded-lg bg-surface text-on-surface text-sm resize-y leading-normal"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              minLength={8}
              maxLength={2000}
              rows={3}
              placeholder="Explain why this request is approved or rejected (min 8 characters)..."
            />
          </label>

          {problem && (
            <p
              className="rounded-lg px-4 py-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger m-0"
              role="alert"
            >
              {problem}
            </p>
          )}
          {message && (
            <p
              className="rounded-lg px-4 py-3 text-sm font-medium bg-success/12 border border-success/35 text-success m-0"
              role="status"
            >
              {message}
            </p>
          )}

          {items.length > 0 ? (
            <ul className="list-none m-0 p-0 border-t border-hairline">
              {items.map((item) => (
                <li
                  key={item.id}
                  className="flex flex-wrap justify-between items-center gap-3 py-3.5 border-b border-hairline [overflow-wrap:anywhere]"
                >
                  <div>
                    <strong>{item.email}</strong>
                    <p className="text-[13px] leading-[19px] text-muted my-1">{item.reason}</p>
                    <small className="text-muted">
                      {new Date(item.created_at).toLocaleString()}
                    </small>
                  </div>
                  <span className="flex flex-wrap gap-2">
                    <button
                      className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-primary bg-primary text-on-primary cursor-pointer hover:opacity-90 disabled:opacity-55 disabled:cursor-not-allowed"
                      disabled={reason.trim().length < 8}
                      onClick={() => void decide(item.id, "approved")}
                    >
                      Approve student access
                    </button>
                    <button
                      className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised disabled:opacity-55 disabled:cursor-not-allowed"
                      disabled={reason.trim().length < 8}
                      onClick={() => void decide(item.id, "rejected")}
                    >
                      Reject
                    </button>
                  </span>
                </li>
              ))}
            </ul>
          ) : (
            !problem && (
              <p role="status" className="text-sm leading-[21px] text-muted m-0">
                No pending requests.
              </p>
            )
          )}
        </section>
      </section>
    </CampusConsoleShell>
  );
}
