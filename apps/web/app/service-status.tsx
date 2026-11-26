"use client";
import { useEffect, useState } from "react";
type ServiceState = "checking" | "ready" | "unavailable";

export function ServiceStatus() {
  const [service, setService] = useState<ServiceState>("checking");
  const [requestID, setRequestID] = useState("");

  useEffect(() => {
    let active = true;
    fetch("/readyz", { cache: "no-store" })
      .then((response) => {
        if (!active) return;
        setRequestID(response.headers.get("X-Request-ID") ?? "");
        setService(response.ok ? "ready" : "unavailable");
      })
      .catch(() => active && setService("unavailable"));
    return () => {
      active = false;
    };
  }, []);

  const dotClass =
    service === "ready"
      ? "bg-success"
      : service === "unavailable"
        ? "bg-danger"
        : "bg-warning";

  return (
    <div
      className={`service-status ${service} inline-flex flex-wrap items-center gap-2 mt-2 text-muted text-xs`}
      role="status"
      aria-live="polite"
    >
      <span className={`w-2 h-2 rounded-full ${dotClass}`} aria-hidden="true" />
      {service === "checking" && "Checking service status…"}
      {service === "ready" && "Local API is ready"}
      {service === "unavailable" &&
        "The API is unavailable. You can still explore this preview."}
      {requestID && <span className="font-mono text-xs">Request ID: {requestID}</span>}
    </div>
  );
}
