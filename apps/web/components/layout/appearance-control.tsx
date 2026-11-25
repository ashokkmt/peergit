"use client";

import { useSyncExternalStore } from "react";

const eventName = "peergit-appearance";

function subscribe(listener: () => void) {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  window.addEventListener(eventName, listener);
  window.addEventListener("storage", listener);
  media.addEventListener("change", listener);
  return () => {
    window.removeEventListener(eventName, listener);
    window.removeEventListener("storage", listener);
    media.removeEventListener("change", listener);
  };
}

function getStoredTheme(): string {
  return document.documentElement.dataset.theme ?? "system";
}

function getEffectiveDark(): boolean {
  const explicit = document.documentElement.dataset.theme;
  if (explicit === "dark") return true;
  if (explicit === "light") return false;
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

function applyTheme(value: string) {
  if (value === "system") delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = value;
  try {
    if (value === "system") localStorage.removeItem("peergit-theme");
    else localStorage.setItem("peergit-theme", value);
  } catch {
    // The current page can still change appearance when browser storage is unavailable.
  }
  window.dispatchEvent(new Event(eventName));
}

export function AppearanceControl({
  variant = "toggle",
}: {
  variant?: "toggle" | "select";
}) {
  const storedTheme = useSyncExternalStore(subscribe, getStoredTheme, () => "system");
  const isDark = useSyncExternalStore(subscribe, getEffectiveDark, () => false);

  if (variant === "select") {
    return (
      <label className="pg-field" style={{ maxWidth: "240px" }}>
        <span>Theme preference</span>
        <select
          className="theme-select"
          aria-label="Appearance"
          value={storedTheme}
          onChange={(event) => applyTheme(event.target.value)}
        >
          <option value="system">System default</option>
          <option value="light">Light</option>
          <option value="dark">Dark</option>
        </select>
      </label>
    );
  }

  return (
    <button
      type="button"
      className="theme-toggle-btn"
      onClick={() => applyTheme(isDark ? "light" : "dark")}
      aria-label="Toggle color theme"
      aria-pressed={isDark}
      title={isDark ? "Switch to light theme" : "Switch to dark theme"}
    >
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <circle cx="12" cy="12" r="9" stroke="currentColor" strokeWidth="1.9" />
        <path d="M12 3a9 9 0 0 1 0 18V3z" fill="currentColor" />
      </svg>
    </button>
  );
}
