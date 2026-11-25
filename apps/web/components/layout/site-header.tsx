"use client";

import { useEffect, useRef, useState } from "react";
import { usePathname } from "next/navigation";
import { BrandLogo } from "./brand-logo";
import { AppearanceControl } from "./appearance-control";

type Session = {
  authenticated: boolean;
  csrf_token?: string;
  user?: {
    display_name: string;
    account_type: string;
    campus_status: string;
    college_id: string;
  };
};

export function SiteHeader({ workspace = false }: { workspace?: boolean }) {
  const [session, setSession] = useState<Session>({ authenticated: false });
  const menu = useRef<HTMLDetailsElement>(null);
  const pathname = usePathname();

  useEffect(() => {
    void fetch("/api/v1/session", { cache: "no-store" })
      .then((r) => (r.ok && r.headers.get("content-type")?.includes("json") ? r.json() : {}))
      .then((v: { data?: Session }) => setSession(v.data ?? { authenticated: false }))
      .catch(() => setSession({ authenticated: false }));
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && menu.current?.open) {
        menu.current.open = false;
        menu.current.querySelector("summary")?.focus();
      }
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, []);

  async function signOut() {
    await fetch("/api/v1/auth/logout", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": session.csrf_token ?? "",
      },
      body: "{}",
      credentials: "same-origin",
    });
    window.location.assign("/");
  }

  return (
    <header className={workspace ? "topbar topbar-workspace" : "topbar"}>
      <div className="topbar-inner">
        <div className="topbar-left">
          <BrandLogo />
          <details className="mobile-main-menu">
            <summary>Menu</summary>
            <nav className="account-menu-panel" aria-label="Mobile navigation">
              <a href="/explore">Explore projects</a>
              <a href="/#how-it-works">How it works</a>
              <a href="/institutes">For institutes</a>
              {session.user?.college_id && (
                <>
                  <a href="/home">My workspace</a>
                  <a href="/inbox">Inbox</a>
                </>
              )}
              {!session.authenticated && (
                <>
                  <a href="/login">Sign in</a>
                  <a href="/signup">Join PeerGit</a>
                </>
              )}
            </nav>
          </details>
        </div>

        <nav className="topbar-nav" aria-label="Main navigation">
          <a href="/explore" aria-current={pathname === "/explore" ? "page" : undefined}>
            Explore projects
          </a>
          <a href="/#how-it-works">How it works</a>
          <a href="/institutes" aria-current={pathname === "/institutes" ? "page" : undefined}>
            For institutes
          </a>
          {session.user?.college_id && (
            <>
              <a href="/home" aria-current={pathname === "/home" ? "page" : undefined}>
                My workspace
              </a>
              <a href="/inbox" aria-current={pathname === "/inbox" ? "page" : undefined}>
                Inbox
              </a>
            </>
          )}
        </nav>

        <div className="topbar-right">
          <AppearanceControl />
          {session.authenticated && session.user ? (
            <details className="account-menu" ref={menu}>
              <summary aria-label={`Account menu for ${session.user.display_name}`}>
                <span className="avatar" aria-hidden="true">
                  {session.user.display_name.slice(0, 1).toUpperCase()}
                </span>
                <span className="account-menu-name">{session.user.display_name}</span>
              </summary>
              <div className="account-menu-panel">
                <p className="menu-account-name">{session.user.display_name}</p>
                <p className="menu-campus">
                  {session.user.campus_status === "verified"
                    ? "Campus verified"
                    : session.user.campus_status === "inactive"
                      ? "Campus access inactive"
                      : session.user.campus_status === "external"
                        ? "External account"
                        : "Campus verification needed"}
                </p>
                {session.user.campus_status !== "verified" && (
                  <a href="/onboarding">Verify campus</a>
                )}
                <a href="/account/profile">Profile</a>
                <a href="/projects">My projects</a>
                <a href="/account/github">GitHub connection</a>
                <a href="/account/settings">Account settings</a>
                <button type="button" onClick={() => void signOut()}>
                  Sign out
                </button>
              </div>
            </details>
          ) : (
            <nav className="topbar-auth-links" aria-label="Sign in">
              <a className="pg-btn-secondary pg-btn-compact pg-hide-xs" href="/login">
                Sign in
              </a>
              <a className="primary-link pg-btn-compact" href="/signup">
                Join PeerGit
              </a>
            </nav>
          )}
        </div>
      </div>
    </header>
  );
}
