"use client";

import { useCallback, useEffect, useState } from "react";
import { readApiEnvelope } from "@/lib/http";

type Preferences = {
  email: Record<string, boolean>;
  in_app: boolean;
};

const prefLoadError = "Notification settings could not be loaded right now.";

export function NotificationPreferences() {
  const [csrf, setCSRF] = useState("");
  const [signedIn, setSignedIn] = useState<boolean | null>(null);
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const sessionResponse = await fetch("/api/v1/session", { cache: "no-store" });
      const session = await readApiEnvelope<{ authenticated?: boolean; csrf_token?: string }>(
        sessionResponse,
        prefLoadError,
      );
      if (!sessionResponse.ok) {
        throw new Error(session.error?.message || prefLoadError);
      }
      const authenticated = Boolean(session.data?.authenticated);
      setSignedIn(authenticated);
      setCSRF(session.data?.csrf_token ?? "");
      if (!authenticated) {
        return;
      }
      const prefResponse = await fetch("/api/v1/notifications/preferences", { cache: "no-store" });
      const prefs = await readApiEnvelope<Preferences>(prefResponse, prefLoadError);
      if (!prefResponse.ok || !prefs.data) {
        throw new Error(prefs.error?.message || prefLoadError);
      }
      setEnabled(Boolean(prefs.data.email.project_activity));
    } catch (e) {
      const text =
        e instanceof Error &&
        !e.message.includes("<!DOCTYPE") &&
        !e.message.includes("not valid JSON")
          ? e.message
          : prefLoadError;
      setError(text);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function save(value: boolean) {
    setEnabled(value);
    setSaving(true);
    setMessage("");
    setError("");
    try {
      const response = await fetch("/api/v1/notifications/preferences", {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": csrf,
        },
        body: JSON.stringify({
          email: { applications: true, project_activity: value },
        }),
      });
      const result = await readApiEnvelope<Preferences>(
        response,
        "Setting could not be saved right now.",
      );
      if (!response.ok || !result.data) {
        throw new Error(result.error?.message || "Setting could not be saved.");
      }
      setEnabled(Boolean(result.data.email.project_activity));
      setMessage("Preference saved.");
    } catch (e) {
      setEnabled(!value);
      const text =
        e instanceof Error &&
        !e.message.includes("<!DOCTYPE") &&
        !e.message.includes("not valid JSON")
          ? e.message
          : "Setting could not be saved.";
      setError(text);
    } finally {
      setSaving(false);
    }
  }

  return (
    <section
      id="notifications"
      className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col gap-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
            Notifications
          </h2>
          <p className="text-sm leading-[21px] text-muted mt-1.5 mb-0">
            In-app notifications stay saved in your inbox. Choose which project activity can also
            reach your campus email.
          </p>
        </div>
        <a
          className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-control bg-surface text-on-surface hover:bg-raised"
          href="/inbox"
        >
          Open inbox
        </a>
      </div>

      {signedIn === false ? (
        <p className="text-[13px] leading-[19px] text-muted m-0">
          Sign in to manage email notification preferences for your campus account.
        </p>
      ) : (
        <div className="flex flex-col gap-3">
          <div className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl px-4 py-3.5">
            <label
              className={`flex items-start gap-2.5 text-sm leading-5 ${
                loading ? "cursor-wait" : "cursor-pointer"
              }`}
            >
              <input
                type="checkbox"
                checked={enabled}
                disabled={loading || saving}
                onChange={(e) => void save(e.currentTarget.checked)}
                className="w-4 h-4 mt-0.5 accent-primary shrink-0"
              />
              <span className="flex flex-col gap-0.5">
                <strong className="text-[13px] font-medium leading-[18px] text-on-surface">
                  Project activity email
                </strong>
                <span className="text-[13px] leading-[19px] text-muted">
                  Updates, discussions, tasks, milestones, and comments from projects you can
                  currently access.
                </span>
              </span>
            </label>
          </div>

          <div className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl px-4 py-3.5 flex flex-wrap items-center justify-between gap-3">
            <div className="flex-[1_1_240px] min-w-0">
              <p className="text-[13px] font-medium leading-[18px] m-0">
                Application decisions &amp; account security
              </p>
              <p className="text-[13px] leading-[19px] text-muted mt-0.5 mb-0">
                Important application decisions and campus verification or security notices remain
                enabled separately.
              </p>
            </div>
            <span className="inline-flex items-center px-2.5 py-1 rounded-full bg-surface border border-success text-success text-xs font-medium leading-4">
              Always enabled
            </span>
          </div>
        </div>
      )}

      {loading && (
        <p className="text-xs font-medium leading-4 text-muted m-0" role="status">
          Loading preferences…
        </p>
      )}
      {saving && (
        <p className="text-xs font-medium leading-4 text-muted m-0" role="status">
          Saving…
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
      {error && (
        <div
          className="rounded-lg px-4 py-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger m-0 flex flex-wrap items-center justify-between gap-3"
          role="alert"
        >
          <span>{error}</span>
          <button
            type="button"
            className="inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised"
            onClick={() => void load()}
          >
            Retry
          </button>
        </div>
      )}
    </section>
  );
}
