"use client";

import { useEffect, useState } from "react";
import { CampusConsoleShell } from "@/components/campus/campus-console-shell";

type Review = {
  id: string;
  user_id: string;
  email: string;
  reason: string;
  created_at: string;
};
type Envelope<T> = { data?: T; error?: { message: string } };

export default function CampusReviewsPage() {
  const [csrf, setCSRF] = useState("");
  const [items, setItems] = useState<Review[]>([]);
  const [reason, setReason] = useState("");
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const session = (await fetch("/api/v1/session", { cache: "no-store" }).then((r) =>
        r.json(),
      )) as Envelope<{
        authenticated: boolean;
        user?: { campus_admin: boolean; mfa_verified: boolean };
        csrf_token?: string;
      }>;
      if (!session.data?.authenticated) {
        location.assign("/login");
        return;
      }
      setCSRF(session.data.csrf_token ?? "");
      if (!session.data.user?.campus_admin || !session.data.user.mfa_verified) {
        setProblem("A campus administrator session with fresh MFA is required.");
        return;
      }
      const result = (await fetch("/api/v1/admin/campus-review-requests", {
        cache: "no-store",
      }).then((r) => r.json())) as Envelope<{ items: Review[] }>;
      if (result.error) throw new Error(result.error.message);
      setItems(result.data?.items ?? []);
    } catch (e) {
      setProblem(
        e instanceof Error ? e.message : "Campus review queue could not load.",
      );
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
      const result = (await response.json()) as Envelope<{ state: string }>;
      if (!response.ok || result.error)
        throw new Error(result.error?.message ?? "Review decision failed.");
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
      <section id="reviews" aria-labelledby="review-title" style={{ display: "flex", flexDirection: "column", gap: "20px" }}>
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "flex-start", justifyContent: "space-between", gap: "12px" }}>
          <div>
            <p className="eyebrow">Campus administration</p>
            <h1 id="review-title" className="t-headline-md">
              Campus verification reviews
            </h1>
            <p className="t-body-md muted" style={{ margin: "6px 0 0", maxWidth: "60ch" }}>
              Review campus affiliation requests for your campus. Your decision needs fresh MFA and
              a reason. Approval grants student access but does not verify a mailbox.
            </p>
          </div>
          <a className="secondary-button pg-btn-compact" href="/home">
            Workspace
          </a>
        </div>

        <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
          <label className="pg-field" htmlFor="review-reason">
            Decision reason
            <textarea
              id="review-reason"
              className="pg-textarea"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              minLength={8}
              maxLength={2000}
              rows={3}
              placeholder="Explain why this request is approved or rejected (min 8 characters)..."
            />
          </label>

          {problem && (
            <p className="form-error" role="alert" style={{ margin: 0 }}>
              {problem}
            </p>
          )}
          {message && (
            <p className="form-success" role="status" style={{ margin: 0 }}>
              {message}
            </p>
          )}

          {items.length > 0 ? (
            <ul className="campus-users">
              {items.map((item) => (
                <li key={item.id}>
                  <div>
                    <strong>{item.email}</strong>
                    <p className="t-body-sm muted" style={{ margin: "4px 0" }}>
                      {item.reason}
                    </p>
                    <small className="muted">
                      {new Date(item.created_at).toLocaleString()}
                    </small>
                  </div>
                  <span className="member-actions">
                    <button
                      className="primary-button pg-btn-compact"
                      disabled={reason.trim().length < 8}
                      onClick={() => void decide(item.id, "approved")}
                    >
                      Approve student access
                    </button>
                    <button
                      className="secondary-button pg-btn-compact"
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
              <p role="status" className="t-body-md muted" style={{ margin: 0 }}>
                No pending requests.
              </p>
            )
          )}
        </section>
      </section>
    </CampusConsoleShell>
  );
}
