"use client";

import { FormEvent, useEffect, useState } from "react";
import { AccountPanel } from "@/components/account/account-panel";
import { BrandLogo } from "@/components/layout/brand-logo";
import { AppearanceControl } from "@/components/layout/appearance-control";
import { readApiEnvelope } from "@/lib/http";

type Status = {
  campus_verified: boolean;
  campus_id: string;
  account_type: string;
  pending_challenge?: { challenge_id: string; email: string; expires_at: string };
};

const fieldClass = "grid gap-1.5 text-[13px] font-medium text-on-surface";
const inputClass =
  "w-full min-h-11 px-3.5 py-2.5 border border-control rounded-lg bg-surface text-on-surface text-sm leading-5";

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
      const sessionRes = await fetch("/api/v1/session", { cache: "no-store" });
      const session = await readApiEnvelope<{ authenticated: boolean; csrf_token?: string }>(
        sessionRes,
        "Onboarding could not load.",
      );
      if (!session.data?.authenticated) {
        location.assign("/login");
        return;
      }
      setCSRF(session.data.csrf_token ?? "");
      const onboardingRes = await fetch("/api/v1/me/onboarding", { cache: "no-store" });
      const result = await readApiEnvelope<Status>(onboardingRes, "Onboarding could not load.");
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
      const result = await readApiEnvelope<{ challenge_id: string }>(
        response,
        "Could not send the verification email.",
      );
      if (!response.ok || result.error) {
        throw new Error(result.error?.message ?? "Could not send the verification email.");
      }
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
      const result = await readApiEnvelope<{ state: string }>(
        response,
        "Campus review request could not be submitted.",
      );
      if (!response.ok || result.error) {
        throw new Error(result.error?.message ?? "Campus review request could not be submitted.");
      }
      setMessage("Your request is in the campus review queue.");
    } catch (e) {
      setProblem(
        e instanceof Error ? e.message : "Campus review request could not be submitted.",
      );
    }
  }

  const pillColor = status?.campus_verified
    ? "border-success text-success"
    : status?.pending_challenge
      ? "border-warning text-warning"
      : "border-hairline text-on-surface";

  return (
    <div className="min-h-screen bg-canvas flex flex-col">
      <a
        className="fixed z-100 left-4 -top-20 px-4 py-2.5 bg-primary text-on-primary rounded-lg font-semibold no-underline focus:top-3"
        href="#onboarding-content"
      >
        Skip to content
      </a>
      <header className="sticky top-0 z-30 bg-surface border-b border-hairline">
        <div className="w-[min(1240px,calc(100%-24px))] md:w-[min(1240px,calc(100%-64px))] mx-auto min-h-16 md:h-[72px] flex items-center justify-between gap-2 md:gap-4 lg:grid lg:grid-cols-[1fr_auto_1fr] lg:gap-6">
          <div className="flex items-center gap-2.5 min-w-0 justify-self-start">
            <BrandLogo />
          </div>
          <nav className="hidden md:flex items-center justify-self-center gap-5 lg:gap-6" aria-label="Account">
            <a
              className="text-muted no-underline text-sm font-medium leading-5 py-1.5 border-b-2 border-transparent hover:text-on-surface"
              href="/explore"
            >
              Explore projects
            </a>
            <a
              className="text-muted no-underline text-sm font-medium leading-5 py-1.5 border-b-2 border-transparent hover:text-on-surface"
              href="/account/profile"
            >
              Profile
            </a>
          </nav>
          <div className="flex items-center gap-2 shrink-0 justify-self-end">
            <AppearanceControl />
          </div>
        </div>
      </header>

      <main
        id="onboarding-content"
        className="w-[min(780px,calc(100%-32px))] mx-auto pt-8 pb-16 flex flex-col gap-6"
      >
        <section className="flex flex-col gap-2">
          <p className="m-0 text-link text-xs font-semibold tracking-[0.02em]">One more step</p>
          <h1
            id="onboarding-title"
            className="font-display text-[clamp(1.75rem,3.2vw,2.75rem)] font-bold leading-[1.14] tracking-[-0.02em] m-0"
          >
            Set up your PeerGit account
          </h1>
          <p className="text-sm leading-[21px] text-muted m-0">
            Your GitHub sign-in does not verify campus membership. Campus access starts after you
            confirm your campus email.
          </p>
        </section>

        <AccountPanel />

        <section
          className="bg-surface border border-hairline rounded-2xl p-[clamp(20px,3vw,28px)] flex flex-col gap-4"
          aria-labelledby="campus-verify-title"
        >
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <p className="mt-0 mb-1.5 text-link text-xs font-semibold tracking-[0.02em]">
                Campus verification
              </p>
              <h2
                id="campus-verify-title"
                className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0"
              >
                {status?.campus_verified
                  ? "Campus verified"
                  : status?.campus_id
                    ? "Campus access inactive"
                    : "Verify your campus email"}
              </h2>
            </div>
            <span
              className={`inline-flex items-center px-2.5 py-1 rounded-full bg-surface border text-xs font-medium leading-4 ${pillColor}`}
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
            <div className="flex flex-col gap-3">
              <p className="text-sm leading-[21px] text-muted m-0">
                Your campus access is active. You can explore campus projects, import GitHub
                repositories, and apply for open roles.
              </p>
              <div className="flex flex-wrap gap-2.5 items-center">
                <a
                  className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium no-underline border border-primary bg-primary text-on-primary hover:opacity-90"
                  href="/explore"
                >
                  Explore projects
                </a>
                <a
                  className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium no-underline border border-control bg-surface text-on-surface hover:bg-raised"
                  href="/"
                >
                  Continue to PeerGit
                </a>
              </div>
            </div>
          ) : status?.campus_id ? (
            <p className="text-sm leading-[21px] text-muted m-0">
              Your campus access is inactive. Your account and personal profile remain available;
              contact a campus administrator about restoring access.
            </p>
          ) : (
            <>
              <p className="text-sm leading-[21px] text-muted m-0">
                Use an approved campus email independently from GitHub. Requests expire after 10
                minutes and include both an email link and a 6-digit code.
              </p>
              <form className="grid gap-3.5 pt-3 border-t border-hairline" onSubmit={requestCode}>
                <label className={fieldClass} htmlFor="campus-email">
                  Campus email
                  <input
                    id="campus-email"
                    type="email"
                    autoComplete="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="you@campus.edu"
                    className={inputClass}
                  />
                </label>
                <button
                  className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium border border-primary bg-primary text-on-primary cursor-pointer hover:opacity-90 disabled:opacity-55 disabled:cursor-not-allowed"
                  type="submit"
                  disabled={!csrf || !email}
                >
                  Send verification email
                </button>
              </form>

              {status?.pending_challenge && (
                <div
                  className="flex flex-wrap items-center justify-between gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px]"
                  role="status"
                >
                  <span>
                    A message was sent to{" "}
                    <strong className="text-on-surface">{status.pending_challenge.email}</strong>.
                  </span>
                  <a
                    className="text-[13px] font-medium leading-[18px]"
                    href={`/onboarding/verify?challenge_id=${encodeURIComponent(status.pending_challenge.challenge_id)}`}
                  >
                    Enter its code
                  </a>
                </div>
              )}

              <details className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl p-4">
                <summary className="text-base font-semibold leading-[22px] cursor-pointer">
                  My campus email is not supported
                </summary>
                <p className="text-[13px] leading-[19px] text-muted mt-2 mb-0">
                  Request administrator review with your campus ID and a short note. Limited
                  accounts can still browse public projects and edit profile settings.
                </p>
                <form className="grid gap-3.5 mt-4 pt-4 border-t border-hairline" onSubmit={requestReview}>
                  <label className={fieldClass} htmlFor="campus-id">
                    Campus ID or slug
                    <input
                      id="campus-id"
                      required
                      value={campus}
                      onChange={(e) => setCampus(e.target.value)}
                      className={inputClass}
                    />
                  </label>
                  <label className={fieldClass} htmlFor="review-reason">
                    Why should the campus review this request?
                    <textarea
                      id="review-reason"
                      required
                      minLength={8}
                      maxLength={2000}
                      rows={4}
                      value={reviewReason}
                      onChange={(e) => setReviewReason(e.target.value)}
                      className={`${inputClass} min-h-24 resize-y leading-normal`}
                    />
                  </label>
                  <button
                    className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised disabled:opacity-55 disabled:cursor-not-allowed"
                    type="submit"
                    disabled={!csrf}
                  >
                    Request campus review
                  </button>
                </form>
              </details>
            </>
          )}

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
        </section>
      </main>
    </div>
  );
}
