"use client";

import { FormEvent, useEffect, useState } from "react";
import { AccountPanel } from "./account";

type ServiceState = "checking" | "ready" | "unavailable";

export default function Home() {
  const [service, setService] = useState<ServiceState>("checking");
  const [requestID, setRequestID] = useState("");
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");

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

  function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmittedQuery(query.trim());
  }

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

      <section className="content" id="projects" aria-labelledby="projects-title">
        <div className="section-heading">
          <div><p className="eyebrow">Find your next step</p><h2 id="projects-title">Explore projects</h2></div>
          <span className="result-count">0 projects</span>
        </div>
        <form className="search" onSubmit={search} role="search">
          <label htmlFor="project-search">Search projects</label>
          <div className="search-row"><input id="project-search" name="q" type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Try design, robotics, or research" /><button type="submit">Search</button></div>
        </form>
        <div className="empty-state" aria-live="polite">
          <span className="empty-icon" aria-hidden="true">✳</span>
          <h3>{submittedQuery ? `No projects found for “${submittedQuery}”` : "Your next idea starts here"}</h3>
          <p>{submittedQuery ? "Try a different search. Projects will appear here when they are available." : "Project listings are not connected yet. This preview shows where campus opportunities will live."}</p>
        </div>
      </section>

      <section className="how" id="how-it-works" aria-labelledby="how-title">
        <p className="eyebrow">A shared starting point</p><h2 id="how-title">Make your work count.</h2>
        <div className="steps"><article><span>01</span><h3>Find a team</h3><p>Explore ideas and see what skills a project needs.</p></article><article><span>02</span><h3>Do meaningful work</h3><p>Collaborate with people who want to build and learn.</p></article><article><span>03</span><h3>Show your progress</h3><p>Keep project work and verified contributions connected.</p></article></div>
      </section>
      <footer><span>PeerGit</span><span>Local development preview</span></footer>
    </main>
  );
}
