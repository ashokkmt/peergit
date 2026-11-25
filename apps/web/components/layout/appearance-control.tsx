"use client";

import { useSyncExternalStore } from "react";

const eventName = "peergit-appearance";

function subscribe(listener: () => void) {
  window.addEventListener(eventName, listener);
  window.addEventListener("storage", listener);
  return () => {
    window.removeEventListener(eventName, listener);
    window.removeEventListener("storage", listener);
  };
}

export function AppearanceControl() {
  const theme = useSyncExternalStore(subscribe, () => document.documentElement.dataset.theme ?? "system", () => "system");

  function changeTheme(value: string) {
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

  return <label className="appearance-control">
    <span className="sr-only">Appearance</span>
    <select className="theme-select" value={theme} onChange={event => changeTheme(event.target.value)}>
      <option value="system">System</option>
      <option value="light">Light</option>
      <option value="dark">Dark</option>
    </select>
  </label>;
}
