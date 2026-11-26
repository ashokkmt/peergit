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
      className="pg-card"
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
          <h2 className="t-title-lg">Notifications</h2>
          <p className="t-body-md muted" style={{ margin: "6px 0 0" }}>
            In-app notifications stay saved in your inbox. Choose which project activity can also
            reach your campus email.
          </p>
        </div>
        <a className="secondary-button pg-btn-compact" href="/inbox">
          Open inbox
        </a>
      </div>

      {signedIn === false ? (
        <p className="t-body-sm muted" style={{ margin: 0 }}>
          Sign in to manage email notification preferences for your campus account.
        </p>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
          <div className="pg-card-canvas" style={{ padding: "14px 16px" }}>
            <label
              className="check-row"
              style={{ margin: 0, cursor: loading ? "wait" : "pointer" }}
            >
              <input
                type="checkbox"
                checked={enabled}
                disabled={loading || saving}
                onChange={(e) => void save(e.currentTarget.checked)}
              />
              <span style={{ display: "flex", flexDirection: "column", gap: "2px" }}>
                <strong className="t-label-md" style={{ color: "var(--text)" }}>
                  Project activity email
                </strong>
                <span className="t-body-sm muted">
                  Updates, discussions, tasks, milestones, and comments from projects you can
                  currently access.
                </span>
              </span>
            </label>
          </div>

          <div
            className="pg-card-canvas"
            style={{
              padding: "14px 16px",
              display: "flex",
              flexWrap: "wrap",
              alignItems: "center",
              justifyContent: "space-between",
              gap: "12px",
            }}
          >
            <div style={{ flex: "1 1 240px", minWidth: 0 }}>
              <p className="t-label-md" style={{ margin: 0 }}>
                Application decisions &amp; account security
              </p>
              <p className="t-body-sm muted" style={{ margin: "2px 0 0" }}>
                Important application decisions and campus verification or security notices remain
                enabled separately.
              </p>
            </div>
            <span className="pg-pill pg-pill-success">Always enabled</span>
          </div>
        </div>
      )}

      {loading && (
        <p className="t-label-sm muted" style={{ margin: 0 }} role="status">
          Loading preferences…
        </p>
      )}
      {saving && (
        <p className="t-label-sm muted" style={{ margin: 0 }} role="status">
          Saving…
        </p>
      )}
      {message && (
        <p className="form-success" style={{ margin: 0 }} role="status">
          {message}
        </p>
      )}
      {error && (
        <div
          className="form-error"
          style={{
            margin: 0,
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "space-between",
            gap: "12px",
          }}
          role="alert"
        >
          <span>{error}</span>
          <button
            type="button"
            className="secondary-button pg-btn-compact"
            onClick={() => void load()}
          >
            Retry
          </button>
        </div>
      )}
    </section>
  );
}
