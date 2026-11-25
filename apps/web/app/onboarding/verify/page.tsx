"use client";

import { FormEvent, useEffect, useRef, useState } from "react";
import { BrandLogo } from "@/components/layout/brand-logo";
import { AppearanceControl } from "@/components/layout/appearance-control";

type Envelope<T> = { data?: T; error?: { message: string } };

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
      .then(response => response.json() as Promise<Envelope<{ authenticated: boolean; csrf_token?: string }>>)
      .then(result => {
        if (!result.data?.authenticated) {
          location.assign("/login");
          return;
        }
        setCSRF(result.data.csrf_token ?? "");
      })
      .catch(() => {
        if (!controller.signal.aborted) setProblem("Could not load your session. Please sign in again.");
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
        body: JSON.stringify(token ? { method: "link", token } : { method: "otp", challenge_id: challengeID, otp }),
      });
      const result = await response.json() as Envelope<{ verified: boolean }>;
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

  return <main className="account-shell">
    <a className="skip-link" href="#verify-content">Skip to content</a>
    <header className="topbar"><BrandLogo mark /><AppearanceControl /><nav aria-label="Account"><a href="/onboarding">Back to verification</a></nav></header>
    <section id="verify-content" className="account-card" aria-labelledby="verify-title">
      <p className="eyebrow">Campus verification</p>
      <h1 id="verify-title">Check your campus email</h1>
      <p>Both verification methods expire ten minutes after they are issued. Submit the form to confirm.</p>
      {complete
        ? <p className="form-success" role="status">Campus email verified. <a href="/onboarding">Continue to onboarding</a>.</p>
        : <>
          <form className="account-form" onSubmit={confirm}>
            {token
              ? <p>A campus email link is ready to confirm for your signed-in account.</p>
              : <label htmlFor="verification-code">Six-digit code
                <input id="verification-code" name="otp" type="text" inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" maxLength={6} required />
              </label>}
            <button className="primary-button" type="submit" disabled={!csrf || (!token && !challengeID) || busy}>
              {busy ? "Confirming…" : "Confirm campus email"}
            </button>
          </form>
          <p><a href="/onboarding">Change your campus email or request a new code</a></p>
        </>}
      {problem && <p className="form-error" role="alert">{problem}</p>}
    </section>
  </main>;
}
