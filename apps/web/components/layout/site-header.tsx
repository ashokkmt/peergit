"use client";

import { useEffect, useRef, useState } from "react";
import { usePathname } from "next/navigation";
import { fetchSession } from "@/lib/http";
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

const navLinkClass =
  "text-muted no-underline text-sm font-medium leading-5 py-1.5 border-b-2 border-transparent whitespace-nowrap hover:text-on-surface aria-[current=page]:text-on-surface aria-[current=page]:border-link";

const menuItemClass =
  "block w-full px-2.5 py-2.5 border-0 rounded-lg bg-transparent text-on-surface text-left no-underline text-[13px] font-medium cursor-pointer hover:bg-raised";

export function SiteHeader({ workspace = false }: { workspace?: boolean }) {
  const [session, setSession] = useState<Session>({ authenticated: false });
  const menu = useRef<HTMLDetailsElement>(null);
  const pathname = usePathname();

  useEffect(() => {
    let active = true;
    void fetchSession()
      .then((r) => (r.ok && r.headers.get("content-type")?.includes("json") ? r.json() : {}))
      .then((v: { data?: Session }) => {
        if (active) setSession(v.data ?? { authenticated: false });
      })
      .catch(() => {
        if (active) setSession({ authenticated: false });
      });
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && menu.current?.open) {
        menu.current.open = false;
        menu.current.querySelector("summary")?.focus();
      }
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      active = false;
      document.removeEventListener("keydown", closeOnEscape);
    };
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
    <header className="sticky top-0 z-30 bg-surface border-b border-hairline">
      <div
        className={
          workspace
            ? "w-[min(1440px,calc(100%-24px))] md:w-[min(1440px,calc(100%-64px))] mx-auto min-h-16 md:h-16 flex items-center justify-between gap-2 md:gap-4 lg:grid lg:grid-cols-[1fr_auto_1fr] lg:gap-6"
            : "w-[min(1240px,calc(100%-24px))] md:w-[min(1240px,calc(100%-64px))] mx-auto min-h-16 md:h-[72px] flex items-center justify-between gap-2 md:gap-4 lg:grid lg:grid-cols-[1fr_auto_1fr] lg:gap-6"
        }
      >
        <div className="flex items-center gap-2.5 min-w-0 justify-self-start">
          <BrandLogo />
          <details className="relative md:hidden">
            <summary className="list-none inline-flex items-center justify-center min-h-9 min-[420px]:min-h-10 px-2.5 min-[420px]:px-3 border border-hairline rounded-lg bg-surface text-on-surface text-xs min-[420px]:text-[13px] font-medium cursor-pointer [&::-webkit-details-marker]:hidden">
              Menu
            </summary>
            <nav
              className="fixed z-40 left-3 right-3 top-[72px] w-auto max-w-[260px] grid gap-0.5 p-2 bg-surface border border-hairline rounded-xl shadow-[var(--shadow)]"
              aria-label="Mobile navigation"
            >
              <a className={menuItemClass} href="/explore">
                Explore projects
              </a>
              <a className={menuItemClass} href="/#how-it-works">
                How it works
              </a>
              <a className={menuItemClass} href="/institutes">
                For institutes
              </a>
              {session.user?.college_id && (
                <>
                  <a className={menuItemClass} href="/home">
                    My workspace
                  </a>
                  <a className={menuItemClass} href="/feed">
                    Campus feed
                  </a>
                  <a className={menuItemClass} href="/messages">
                    Messages
                  </a>
                  <a className={menuItemClass} href="/applications">
                    My applications
                  </a>
                  <a className={menuItemClass} href="/inbox">
                    Inbox
                  </a>
                </>
              )}
              {!session.authenticated && (
                <>
                  <a className={menuItemClass} href="/login">
                    Sign in
                  </a>
                  <a className={menuItemClass} href="/signup">
                    Join PeerGit
                  </a>
                </>
              )}
            </nav>
          </details>
        </div>

        <nav
          className="hidden md:flex items-center justify-self-center gap-5 lg:gap-6"
          aria-label="Main navigation"
        >
          <a
            className={navLinkClass}
            href="/explore"
            aria-current={pathname === "/explore" ? "page" : undefined}
          >
            Explore projects
          </a>
          {!session.user?.college_id ? (
            <>
              <a className={navLinkClass} href="/#how-it-works">
                How it works
              </a>
              <a
                className={navLinkClass}
                href="/institutes"
                aria-current={pathname === "/institutes" ? "page" : undefined}
              >
                For institutes
              </a>
            </>
          ) : (
            <>
              <a
                className={navLinkClass}
                href="/home"
                aria-current={pathname === "/home" ? "page" : undefined}
              >
                My workspace
              </a>
              <a
                className={navLinkClass}
                href="/feed"
                aria-current={pathname === "/feed" ? "page" : undefined}
              >
                Campus feed
              </a>
              <a
                className={navLinkClass}
                href="/messages"
                aria-current={pathname.startsWith("/messages") ? "page" : undefined}
              >
                Messages
              </a>
              <a
                className={navLinkClass}
                href="/inbox"
                aria-current={pathname === "/inbox" ? "page" : undefined}
              >
                Inbox
              </a>
            </>
          )}
        </nav>

        <div className="flex items-center gap-1.5 min-[420px]:gap-2 shrink-0 justify-self-end">
          <AppearanceControl />
          {session.authenticated && session.user ? (
            <details className="account-menu relative" ref={menu}>
              <summary
                aria-label={`Account menu for ${session.user.display_name}`}
                className="inline-flex items-center gap-2 cursor-pointer border border-hairline rounded-lg bg-surface py-1 pr-2.5 pl-1 min-h-10 list-none [&::-webkit-details-marker]:hidden"
              >
                <span
                  className="grid place-items-center w-[30px] h-[30px] rounded-full bg-raised text-on-surface text-xs font-semibold shrink-0"
                  aria-hidden="true"
                >
                  {session.user.display_name.slice(0, 1).toUpperCase()}
                </span>
                <span className="hidden sm:inline text-[13px] font-medium text-on-surface">
                  {session.user.display_name}
                </span>
              </summary>
              <div className="absolute z-40 right-0 top-[calc(100%+8px)] w-[min(280px,calc(100vw-24px))] grid gap-0.5 p-2 bg-surface border border-hairline rounded-xl shadow-[var(--shadow)]">
                <p className="mt-1 mx-2.5 mb-0 font-semibold text-sm text-on-surface">
                  {session.user.display_name}
                </p>
                <p className="mt-0.5 mx-2.5 mb-2 text-muted text-xs">
                  {session.user.campus_status === "verified"
                    ? "Campus verified"
                    : session.user.campus_status === "inactive"
                      ? "Campus access inactive"
                      : session.user.campus_status === "external"
                        ? "External account"
                        : "Campus verification needed"}
                </p>
                {session.user.campus_status !== "verified" && (
                  <a className={menuItemClass} href="/onboarding">
                    Verify campus
                  </a>
                )}
                <a className={menuItemClass} href="/account/profile">
                  Profile
                </a>
                <a className={menuItemClass} href="/account/portfolio">
                  Portfolio
                </a>
                <a className={menuItemClass} href="/projects">
                  My projects
                </a>
                <a className={menuItemClass} href="/applications">
                  My applications
                </a>
                <a className={menuItemClass} href="/saved">
                  Saved projects
                </a>
                <a className={menuItemClass} href="/account/github">
                  GitHub connection
                </a>
                <a className={menuItemClass} href="/account/settings">
                  Account settings
                </a>
                <button
                  type="button"
                  className={menuItemClass}
                  onClick={() => void signOut()}
                >
                  Sign out
                </button>
              </div>
            </details>
          ) : (
            <nav className="flex items-center gap-2" aria-label="Sign in">
              <a
                className="hidden sm:inline-flex min-h-[38px] items-center justify-center gap-2 rounded-lg px-3.5 py-1.5 text-[13px] font-medium leading-5 no-underline whitespace-nowrap border border-control bg-surface text-on-surface hover:bg-raised transition-colors duration-100"
                href="/login"
              >
                Sign in
              </a>
              <a
                className="inline-flex min-h-9 min-[420px]:min-h-[38px] items-center justify-center gap-2 rounded-lg px-2.5 min-[420px]:px-3.5 py-1.5 text-xs min-[420px]:text-[13px] font-medium leading-5 no-underline whitespace-nowrap border border-primary bg-primary text-on-primary hover:opacity-90 transition-opacity duration-100"
                href="/signup"
              >
                Join PeerGit
              </a>
            </nav>
          )}
        </div>
      </div>
    </header>
  );
}
