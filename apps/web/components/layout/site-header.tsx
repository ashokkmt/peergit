"use client";

import { useEffect, useRef, useState } from "react";
import { BrandLogo } from "./brand-logo";
import { AppearanceControl } from "./appearance-control";

type Session = { authenticated: boolean; csrf_token?: string; user?: { display_name: string; account_type: string; campus_status: string; college_id: string } };

export function SiteHeader() {
  const [session, setSession] = useState<Session>({ authenticated: false });
  const menu = useRef<HTMLDetailsElement>(null);

  useEffect(() => {
    void fetch("/api/v1/session", { cache: "no-store" }).then((r) => r.json()).then((v: { data?: Session }) => setSession(v.data ?? { authenticated: false })).catch(() => setSession({ authenticated: false }));
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === "Escape" && menu.current?.open) { menu.current.open = false; menu.current.querySelector("summary")?.focus(); } };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, []);

  async function signOut() {
    await fetch("/api/v1/auth/logout", { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": session.csrf_token ?? "" }, body: "{}", credentials: "same-origin" });
    window.location.assign("/");
  }

  return <header className="topbar">
    <BrandLogo />
    <nav aria-label="Main navigation"><a href="/explore">Explore projects</a><a href="/#how-it-works">How it works</a><a href="/institutes">For institutes</a>{session.user?.college_id && <a href="/home">My workspace</a>}</nav>
    <details className="mobile-main-menu">
      <summary>Menu</summary>
      <nav className="account-menu-panel" aria-label="Mobile navigation">
        <a href="/explore">Explore projects</a><a href="/#how-it-works">How it works</a><a href="/institutes">For institutes</a>
        {session.user?.college_id && <a href="/home">My workspace</a>}
        {!session.authenticated && <a href="/signup">Join PeerGit</a>}
      </nav>
    </details>
    <AppearanceControl />
    {session.authenticated && session.user ? <details className="account-menu" ref={menu}>
      <summary aria-label={`Account menu for ${session.user.display_name}`}><span className="avatar" aria-hidden="true">{session.user.display_name.slice(0, 1).toUpperCase()}</span><span className="account-menu-name">{session.user.display_name}</span></summary>
      <div className="account-menu-panel"><p className="menu-account-name">{session.user.display_name}</p><p className="menu-campus">{session.user.campus_status === "verified" ? "Campus verified" : session.user.campus_status === "inactive" ? "Campus access inactive" : session.user.campus_status === "external" ? "External account" : "Campus verification needed"}</p>
        <a href="/account/profile">Profile</a><a href="/projects">My projects</a><a href="/account/github">GitHub connection</a><a href="/account/settings">Account settings</a><button type="button" onClick={() => void signOut()}>Sign out</button>
      </div>
    </details> : <nav aria-label="Sign in"><a href="/login">Sign in</a><a className="primary-link" href="/signup">Join PeerGit</a></nav>}
  </header>;
}
