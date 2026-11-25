"use client";

import { FormEvent, useEffect, useState } from "react";
import { AccountPanel } from "@/components/account/account-panel";
import { BrandLogo } from "@/components/layout/brand-logo";
import { AppearanceControl } from "@/components/layout/appearance-control";

type Envelope<T> = { data?: T; error?: { message: string } };
type Status = {
  campus_verified: boolean;
  campus_id: string;
  account_type: string;
  pending_challenge?: { challenge_id: string; email: string; expires_at: string };
};

export default function OnboardingPage() {
  const [csrf, setCSRF] = useState("");
  const [email, setEmail] = useState("");
  const [campus, setCampus] = useState("");
  const [reviewReason, setReviewReason] = useState("");
  const [status, setStatus] = useState<Status | null>(null);
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const session = (await fetch("/api/v1/session", { cache: "no-store" }).then((r) =>
        r.json(),
      )) as Envelope<{ authenticated: boolean; csrf_token?: string }>;
      if (!session.data?.authenticated) {
        location.assign("/login");
        return;
      }
      setCSRF(session.data.csrf_token ?? "");
      const result = (await fetch("/api/v1/me/onboarding", { cache: "no-store" }).then((r) =>
        r.json(),
      )) as Envelope<Status>;
      if (result.error) throw new Error(result.error.message);
      setStatus(result.data ?? null);
    } catch (e) {
      setProblem(e instanceof Error ? e.message : "Onboarding could not load.");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function requestCode(event: FormEvent) {
    event.preventDefault();
    setProblem("");
    setMessage("");
    try {
      const response = await fetch("/api/v1/me/campus-verification/challenges", {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ campus_email: email }),
      });
      const result = (await response.json()) as Envelope<{ challenge_id: string }>;
      if (!response.ok || result.error)
        throw new Error(result.error?.message ?? "Could not send the verification email.");
      setMessage("A verification link and six-digit code were sent. Check your campus inbox.");
      await load();
    } catch (e) {
      setProblem(e instanceof Error ? e.message : "Could not send the verification email.");
    }
  }

  async function requestReview(event: FormEvent) {
    event.preventDefault();
    setProblem("");
    setMessage("");
    try {
      const response = await fetch("/api/v1/me/campus-review-requests", {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ campus_id: campus, campus_email: email, reason: reviewReason }),
      });
      const result = (await response.json()) as Envelope<{ state: string }>;
      if (!response.ok || result.error)
        throw new Error(result.error?.message ?? "Campus review request could not be submitted.");
      setMessage("Your request is in the campus review queue.");
    } catch (e) {
      setProblem(
        e instanceof Error ? e.message : "Campus review request could not be submitted.",
      );
    }
  }

  return (
    <div className="account-shell">
      <a className="skip-link" href="#onboarding-content">
        Skip to content
      </a>
      <header className="topbar">
        <div className="topbar-inner">
          <div className="topbar-left">
            <BrandLogo />
            <nav className="topbar-nav" aria-label="Account">
              <a href="/explore">Explore projects</a>
              <a href="/account/profile">Profile</a>
            </nav>
          </div>
          <div className="topbar-right">
            <AppearanceControl />
          </div>
        </div>
      </header>

      <main
        id="onboarding-content"
        className="pg-container"
        style={{ maxWidth: "780px", padding: "32px 0 64px", display: "flex", flexDirection: "column", gap: "24px" }}
      >
        <section style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
          <p className="eyebrow">One more step</p>
          <h1 id="onboarding-title" className="t-headline-lg">
            Set up your PeerGit account
          </h1>
          <p className="t-body-md muted" style={{ margin: 0 }}>
            Your GitHub sign-in does not verify campus membership. Campus access starts after you
            confirm your campus email.
          </p>
        </section>

        <AccountPanel />

        <section
          className="account-panel"
          aria-labelledby="campus-verify-title"
          style={{ display: "flex", flexDirection: "column", gap: "16px" }}
        >
          <div
            style={{
              display: "flex",
              flexWrap: "wrap",
              alignItems: "center",
              justifyContent: "space-between",
              gap: "12px",
            }}
          >
            <div>
              <p className="eyebrow">Campus verification</p>
              <h2 id="campus-verify-title" className="t-headline-md">
                {status?.campus_verified
                  ? "Campus verified"
                  : status?.campus_id
                    ? "Campus access inactive"
                    : "Verify your campus email"}
              </h2>
            </div>
            <span
              className={`pg-pill ${
                status?.campus_verified
                  ? "pg-pill-success"
                  : status?.pending_challenge
                    ? "pg-pill-warning"
                    : ""
              }`}
            >
              {status?.campus_verified
                ? "Verified"
                : status?.pending_challenge
                  ? "Pending"
                  : status?.campus_id
                    ? "Inactive"
                    : "Unverified"}
            </span>
          </div>

          {status?.campus_verified ? (
            <div style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
              <p className="t-body-md muted" style={{ margin: 0 }}>
                Your campus access is active. You can explore campus projects, import GitHub
                repositories, and apply for open roles.
              </p>
              <div className="button-row">
                <a className="primary-link" href="/explore">
                  Explore projects
                </a>
                <a className="secondary-button" href="/">
                  Continue to PeerGit
                </a>
              </div>
            </div>
          ) : status?.campus_id ? (
            <p className="t-body-md muted" style={{ margin: 0 }}>
              Your campus access is inactive. Your account and personal profile remain available;
              contact a campus administrator about restoring access.
            </p>
          ) : (
            <>
              <p className="t-body-md muted" style={{ margin: 0 }}>
                Use an approved campus email independently from GitHub. Requests expire after 10
                minutes and include both an email link and a 6-digit code.
              </p>
              <form className="account-form" onSubmit={requestCode} style={{ marginTop: 0, paddingTop: "12px" }}>
                <label htmlFor="campus-email">
                  Campus email
                  <input
                    id="campus-email"
                    type="email"
                    autoComplete="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="you@campus.edu"
                  />
                </label>
                <button className="primary-button" type="submit" disabled={!csrf || !email}>
                  Send verification email
                </button>
              </form>

              {status?.pending_challenge && (
                <div className="pg-callout" role="status" style={{ justifyContent: "space-between", alignItems: "center", flexWrap: "wrap" }}>
                  <span>
                    A message was sent to <strong>{status.pending_challenge.email}</strong>.
                  </span>
                  <a
                    className="t-label-md"
                    href={`/onboarding/verify?challenge_id=${encodeURIComponent(status.pending_challenge.challenge_id)}`}
                  >
                    Enter its code
                  </a>
                </div>
              )}

              <details className="pg-card-canvas" style={{ padding: "16px" }}>
                <summary className="t-title-sm" style={{ cursor: "pointer" }}>
                  My campus email is not supported
                </summary>
                <p className="t-body-sm muted" style={{ marginTop: "8px" }}>
                  Request administrator review with your campus ID and a short note. Limited
                  accounts can still browse public projects and edit profile settings.
                </p>
                <form className="account-form" onSubmit={requestReview}>
                  <label htmlFor="campus-id">
                    Campus ID or slug
                    <input
                      id="campus-id"
                      required
                      value={campus}
                      onChange={(e) => setCampus(e.target.value)}
                    />
                  </label>
                  <label htmlFor="review-reason">
                    Why should the campus review this request?
                    <textarea
                      id="review-reason"
                      required
                      minLength={8}
                      maxLength={2000}
                      rows={4}
                      value={reviewReason}
                      onChange={(e) => setReviewReason(e.target.value)}
                    />
                  </label>
                  <button className="secondary-button" type="submit" disabled={!csrf}>
                    Request campus review
                  </button>
                </form>
              </details>
            </>
          )}

          {problem && (
            <p className="form-error" role="alert">
              {problem}
            </p>
          )}
          {message && (
            <p className="form-success" role="status">
              {message}
            </p>
          )}
        </section>
      </main>
    </div>
  );
}
