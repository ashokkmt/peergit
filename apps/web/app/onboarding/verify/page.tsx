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
      .then(
        (response) =>
          response.json() as Promise<Envelope<{ authenticated: boolean; csrf_token?: string }>>,
      )
      .then((result) => {
        if (!result.data?.authenticated) {
          location.assign("/login");
          return;
        }
        setCSRF(result.data.csrf_token ?? "");
      })
      .catch(() => {
        if (!controller.signal.aborted)
          setProblem("Could not load your session. Please sign in again.");
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
      const result = (await response.json()) as Envelope<{ verified: boolean }>;
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
    <div className="account-shell">
      <a className="skip-link" href="#verify-content">
        Skip to content
      </a>
      <header className="topbar">
        <div className="topbar-inner">
          <div className="topbar-left">
            <BrandLogo mark />
            <nav className="topbar-nav" aria-label="Account">
              <a href="/onboarding">Back to verification</a>
            </nav>
          </div>
          <div className="topbar-right">
            <AppearanceControl />
          </div>
        </div>
      </header>

      <main id="verify-content" className="account-card" aria-labelledby="verify-title">
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "12px" }}>
          <p className="eyebrow" style={{ margin: 0 }}>
            Campus verification
          </p>
          <span className={`pg-pill ${complete ? "pg-pill-success" : "pg-pill-warning"}`}>
            {complete ? "Verified" : "Pending"}
          </span>
        </div>

        <h1 id="verify-title" className="t-headline-md">
          Check your campus email
        </h1>
        <p className="t-body-md muted" style={{ margin: 0 }}>
          Both verification methods expire ten minutes after they are issued. Submit the form to
          confirm.
        </p>

        {complete ? (
          <div className="form-success" role="status">
            Campus email verified. <a href="/onboarding">Continue to onboarding</a>.
          </div>
        ) : (
          <>
            <form className="account-form" onSubmit={confirm} style={{ marginTop: "4px" }}>
              {token ? (
                <div className="pg-callout">
                  <span>A campus email link is ready to confirm for your signed-in account.</span>
                </div>
              ) : (
                <label htmlFor="verification-code">
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
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: "18px",
                      letterSpacing: "0.25em",
                      textAlign: "center",
                      minHeight: "48px",
                    }}
                  />
                </label>
              )}
              <button
                className="primary-button"
                type="submit"
                disabled={!csrf || (!token && !challengeID) || busy}
              >
                {busy ? "Confirming…" : "Confirm campus email"}
              </button>
            </form>
            <p className="t-body-sm muted" style={{ margin: 0 }}>
              <a href="/onboarding">Change your campus email or request a new code</a>
            </p>
          </>
        )}

        {problem && (
          <p className="form-error" role="alert">
            {problem}
          </p>
        )}
      </main>
    </div>
  );
}
