"use client";

import { FormEvent, useEffect, useRef, useState } from "react";
import { BrandLogo } from "@/components/layout/brand-logo";
import { AppearanceControl } from "@/components/layout/appearance-control";
import { readApiEnvelope } from "@/lib/http";

export default function VerifyCampusPage() {
  const fragment = useRef<string | null>(null);
  const [token, setToken] = useState("");
  const [challengeID, setChallengeID] = useState("");
  const [csrf, setCSRF] = useState("");
  const [problem, setProblem] = useState("");
  const [complete, setComplete] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    fragment.current ??= new URLSearchParams(location.hash.slice(1)).get("token") ?? "";
    setToken(fragment.current);
    setChallengeID(new URLSearchParams(location.search).get("challenge_id") ?? "");
    history.replaceState(history.state, "", location.pathname + location.search);

    const controller = new AbortController();
    void fetch("/api/v1/session", { cache: "no-store", signal: controller.signal })
      .then((response) =>
        readApiEnvelope<{ authenticated: boolean; csrf_token?: string }>(
          response,
          "Could not load your session.",
        ),
      )
      .then((result) => {
        if (!result.data?.authenticated) {
          location.assign("/login");
          return;
        }
        setCSRF(result.data.csrf_token ?? "");
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setProblem("Could not load your session. Please sign in again.");
        }
      });
    return () => controller.abort();
  }, []);

  async function confirm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!csrf || busy) return;
    const otp = String(new FormData(event.currentTarget).get("otp") ?? "");
    setProblem("");
    setBusy(true);
    try {
      const response = await fetch("/api/v1/me/campus-verification/confirm", {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify(
          token ? { method: "link", token } : { method: "otp", challenge_id: challengeID, otp },
        ),
      });
      const result = await readApiEnvelope<{ verified: boolean }>(
        response,
        "Verification could not be completed.",
      );
      if (!response.ok || !result.data?.verified) {
        throw new Error(result.error?.message ?? "Verification could not be completed.");
      }
      setComplete(true);
      setToken("");
      fragment.current = "";
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Verification could not be completed.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="min-h-screen bg-canvas flex flex-col">
      <a
        className="fixed z-100 left-4 -top-20 px-4 py-2.5 bg-primary text-on-primary rounded-lg font-semibold no-underline focus:top-3"
        href="#verify-content"
      >
        Skip to content
      </a>
      <header className="sticky top-0 z-30 bg-surface border-b border-hairline">
        <div className="w-[min(1240px,calc(100%-24px))] md:w-[min(1240px,calc(100%-64px))] mx-auto min-h-16 md:h-[72px] flex items-center justify-between gap-2 md:gap-4 lg:grid lg:grid-cols-[1fr_auto_1fr] lg:gap-6">
          <div className="flex items-center gap-2.5 min-w-0 justify-self-start">
            <BrandLogo mark />
          </div>
          <nav className="hidden md:flex items-center justify-self-center gap-6" aria-label="Account">
            <a
              className="text-muted no-underline text-sm font-medium leading-5 py-1.5 border-b-2 border-transparent hover:text-on-surface"
              href="/onboarding"
            >
              Back to verification
            </a>
          </nav>
          <div className="flex items-center gap-2 shrink-0 justify-self-end">
            <AppearanceControl />
          </div>
        </div>
      </header>

      <main
        id="verify-content"
        className="w-[min(560px,calc(100%-32px))] mt-8 mb-16 mx-auto p-[clamp(24px,4vw,36px)] bg-surface border border-hairline rounded-[20px] flex flex-col gap-4"
        aria-labelledby="verify-title"
      >
        <div className="flex items-center justify-between gap-3">
          <p className="m-0 text-link text-xs font-semibold tracking-[0.02em]">
            Campus verification
          </p>
          <span
            className={`inline-flex items-center px-2.5 py-1 rounded-full bg-surface border text-xs font-medium leading-4 ${
              complete ? "border-success text-success" : "border-warning text-warning"
            }`}
          >
            {complete ? "Verified" : "Pending"}
          </span>
        </div>

        <h1
          id="verify-title"
          className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0"
        >
          Check your campus email
        </h1>
        <p className="text-sm leading-[21px] text-muted m-0">
          Both verification methods expire ten minutes after they are issued. Submit the form to
          confirm.
        </p>

        {complete ? (
          <div
            className="rounded-lg px-4 py-3 text-sm font-medium bg-success/12 border border-success/35 text-success"
            role="status"
          >
            Campus email verified. <a href="/onboarding">Continue to onboarding</a>.
          </div>
        ) : (
          <>
            <form
              className="grid gap-3.5 mt-1 pt-5 border-t border-hairline"
              onSubmit={confirm}
            >
              {token ? (
                <div className="flex items-start gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px]">
                  <span>A campus email link is ready to confirm for your signed-in account.</span>
                </div>
              ) : (
                <label
                  className="grid gap-1.5 text-[13px] font-medium text-on-surface"
                  htmlFor="verification-code"
                >
                  Six-digit code
                  <input
                    id="verification-code"
                    name="otp"
                    type="text"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    pattern="[0-9]{6}"
                    maxLength={6}
                    required
                    className="w-full min-h-12 px-3.5 py-2.5 border border-control rounded-lg bg-surface text-on-surface font-mono text-[18px] tracking-[0.25em] text-center"
                  />
                </label>
              )}
              <button
                className="inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium border border-primary bg-primary text-on-primary cursor-pointer hover:opacity-90 disabled:opacity-55 disabled:cursor-not-allowed"
                type="submit"
                disabled={!csrf || (!token && !challengeID) || busy}
              >
                {busy ? "Confirming…" : "Confirm campus email"}
              </button>
            </form>
            <p className="text-[13px] leading-[19px] text-muted m-0">
              <a href="/onboarding">Change your campus email or request a new code</a>
            </p>
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
      </main>
    </div>
  );
}
