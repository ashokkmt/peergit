"use client";

import { useEffect, useState } from "react";

type Candidate = { id: number; full_name: string; installation_id: string; visibility: string };
type Envelope<T> = { data?: T; error?: { message?: string } };

export function LegacyRepositoryLink({ projectID, csrf }: { projectID: string; csrf: string }) {
  const [items, setItems] = useState<Candidate[]>([]);
  const [selected, setSelected] = useState("");
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    void fetch("/api/v1/me/github/repositories", { cache: "no-store" }).then(async response => {
      const result = await response.json() as Envelope<{ items: Candidate[] }>;
      if (!response.ok) throw new Error(result.error?.message ?? "GitHub repositories could not be loaded.");
      setItems(result.data?.items ?? []);
    }).catch(error => setProblem(error instanceof Error ? error.message : "GitHub repositories could not be loaded."));
  }, []);
  async function link() {
    setBusy(true); setProblem("");
    try {
      const response = await fetch(`/api/v1/projects/${projectID}/github/link`, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify({ repository_id: Number(selected) }) });
      const result = await response.json() as Envelope<{ authorization_url: string }>;
      if (!response.ok || !result.data?.authorization_url) throw new Error(result.error?.message ?? "Repository connection could not be started.");
      window.location.assign(result.data.authorization_url);
    } catch (error) { setProblem(error instanceof Error ? error.message : "Repository connection could not be started."); setBusy(false); }
  }
  return <section className="account-form" aria-labelledby="legacy-repository-title"><h2 id="legacy-repository-title">Connect a GitHub repository</h2><p>This existing project stays private until you connect a repository and publish it.</p>{problem && <p className="form-error" role="alert">{problem}</p>}{items.length ? <><label htmlFor="legacy-repository">Repository you administer</label><select id="legacy-repository" value={selected} onChange={event => setSelected(event.target.value)}><option value="">Choose a repository</option>{items.map(item => <option key={item.id} value={item.id}>{item.full_name} · {item.visibility}</option>)}</select><button className="primary-button" type="button" disabled={!selected || busy} onClick={() => void link()}>{busy ? "Connecting…" : "Connect repository"}</button></> : <p>No available repository. <a href="/account/github">Connect or refresh GitHub</a> first.</p>}</section>;
}
