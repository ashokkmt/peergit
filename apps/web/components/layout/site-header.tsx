"use client";

import { useEffect, useRef, useState } from "react";

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
    <a className="brand" href="/" aria-label="PeerGit home"><span className="brand-mark" aria-hidden="true">P</span>PeerGit</a>
    <nav aria-label="Main navigation"><a href="/">Discover</a>{session.user?.college_id && <a href="/projects">Projects</a>}</nav>
    {session.authenticated && session.user ? <details className="account-menu" ref={menu}>
      <summary aria-label={`Account menu for ${session.user.display_name}`}><span className="avatar" aria-hidden="true">{session.user.display_name.slice(0, 1).toUpperCase()}</span><span className="account-menu-name">{session.user.display_name}</span></summary>
      <div className="account-menu-panel"><p className="menu-account-name">{session.user.display_name}</p><p className="menu-campus">{session.user.campus_status === "verified" ? "Campus verified" : session.user.campus_status === "inactive" ? "Campus access inactive" : session.user.campus_status === "external" ? "External account" : "Campus verification needed"}</p>
        <a href="/account/profile">Profile</a><a href="/projects">Projects</a><a href="/account/github">GitHub connections</a><a href="/account/settings">Settings</a><button type="button" onClick={() => void signOut()}>Sign out</button>
      </div>
    </details> : <nav aria-label="Sign in"><a href="/login">Log in</a><a href="/signup">Sign up</a></nav>}
  </header>;
}
