"use client";

import { useEffect, useState } from "react";
import styles from "./project-work.module.css";

type SavedProject = { id: string; slug: string; title: string; summary: string; visibility: string; lifecycle: string; following: boolean; bookmarked: boolean };
type Envelope = { data?: { csrf_token?: string; items?: SavedProject[] }; error?: { message?: string } };

export function SavedProjects() {
  const [items, setItems] = useState<SavedProject[]>([]);
  const [csrf, setCSRF] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  async function load() {
    setLoading(true); setError("");
    try {
      const [sessionResponse, savedResponse] = await Promise.all([
        fetch("/api/v1/session", { cache: "no-store" }),
        fetch("/api/v1/me/saved-projects", { cache: "no-store" }),
      ]);
      const session = (await sessionResponse.json()) as Envelope;
      const saved = (await savedResponse.json()) as Envelope;
      if (!sessionResponse.ok || !savedResponse.ok) throw new Error(saved.error?.message ?? "Saved projects are unavailable.");
      setCSRF(session.data?.csrf_token ?? ""); setItems(saved.data?.items ?? []);
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Saved projects could not be loaded."); }
    finally { setLoading(false); }
  }
  useEffect(() => { void load(); }, []);
  async function remove(project: SavedProject, kind: "follow" | "bookmark") {
    try {
      const response = await fetch(`/api/v1/projects/${project.id}/${kind}`, { method: "DELETE", headers: { "X-CSRF-Token": csrf }, credentials: "same-origin" });
      if (!response.ok) throw new Error("Could not update this saved project.");
      setItems((current) => current.map((item) => item.id === project.id ? { ...item, [kind === "follow" ? "following" : "bookmarked"]: false } : item).filter((item) => item.following || item.bookmarked));
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Could not update this saved project."); }
  }
  return <section className={styles.workspace}>
    <p className="eyebrow">Your library</p><h1 className="t-headline-md">Saved projects</h1><p className="muted">Projects you follow or have bookmarked for later.</p>
    {error && <div className={styles.error} role="alert">{error}<button className="secondary-button" onClick={() => void load()}>Retry</button></div>}
    {loading ? <div className={styles.status} aria-live="polite">Loading saved projects…</div> : items.length === 0 ? <div className={styles.empty}><span className={styles.emptyMark}>☆</span><h3>Your saved list is empty</h3><p>Follow a project for updates or bookmark it to return later.</p><a href="/explore">Explore projects →</a></div> : <div className={styles.milestones}>{items.map((item) => <article className={styles.card} key={item.id}><div className={styles.milestoneHead}><div><span className={styles.audience}>{item.visibility}</span><h2 className={styles.topic}><a href={`/projects/${item.id}`}>{item.title}</a></h2></div><span className={styles.audience}>{item.following ? "Following" : "Saved"}</span></div><p className={styles.prose}>{item.summary}</p><div className={styles.actions}>{item.following && <button className="secondary-button" onClick={() => void remove(item, "follow")}>Unfollow</button>}{item.bookmarked && <button className="secondary-button" onClick={() => void remove(item, "bookmark")}>Remove bookmark</button>}</div></article>)}</div>}
  </section>;
}
