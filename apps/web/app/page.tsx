"use client";

import { useEffect, useState } from "react";
import { AccountPanel } from "./account";
import { ProjectsPanel } from "./projects";

type ServiceState = "checking" | "ready" | "unavailable";

export default function Home() {
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
    return () => { active = false; };
  }, []);

  return (
    <main>
      <a className="skip-link" href="#main-content">Skip to content</a>
      <header className="topbar">
        <a className="brand" href="/" aria-label="PeerGit home"><span className="brand-mark" aria-hidden="true">P</span>PeerGit</a>
        <nav aria-label="Main navigation"><a aria-current="page" href="#projects">Projects</a><a href="#how-it-works">How it works</a></nav>
        <span className="environment">Local preview</span>
      </header>

      <section className="hero" id="main-content" aria-labelledby="welcome-title">
        <p className="eyebrow">Build something together</p>
        <h1 id="welcome-title">Good projects grow<br />with good people.</h1>
        <p className="intro">Find campus projects, bring your skills, and make your work visible.</p>
        <a className="primary-link" href="#projects">Explore projects <span aria-hidden="true">→</span></a>
        <div className={`service-status ${service}`} role="status" aria-live="polite">
          <span className="status-dot" aria-hidden="true" />
          {service === "checking" && "Checking service status…"}
          {service === "ready" && "Local API is ready"}
          {service === "unavailable" && "The API is unavailable. You can still explore this preview."}
          {requestID && <span className="request-id">Request ID: {requestID}</span>}
        </div>
      </section>

      <AccountPanel />

      <ProjectsPanel />

      <section className="how" id="how-it-works" aria-labelledby="how-title">
        <p className="eyebrow">A shared starting point</p><h2 id="how-title">Make your work count.</h2>
        <div className="steps"><article><span>01</span><h3>Find a team</h3><p>Explore ideas and see what skills a project needs.</p></article><article><span>02</span><h3>Do meaningful work</h3><p>Collaborate with people who want to build and learn.</p></article><article><span>03</span><h3>Show your progress</h3><p>Keep project work and verified contributions connected.</p></article></div>
      </section>
      <footer><span>PeerGit</span><span>Local development preview</span></footer>
    </main>
  );
}
