"use client";

import { useCallback, useEffect, useState } from "react";

type Envelope<T> = { data?: T; error?: { message?: string } };
type Repository = { id: number; full_name: string; description: string; visibility: string; default_branch: string; topics: string[]; expires_at: string };
type Installation = { id: string; installation_id: string; account: string; target_type: string; status: string; confirmed_at: string };

export function GitHubImports({ csrf }: { csrf: string }) {
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [installations, setInstallations] = useState<Installation[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [problem, setProblem] = useState("");

  const api = useCallback(async <T,>(path: string, init?: RequestInit): Promise<T> => {
    const headers = new Headers(init?.headers);
    if (init?.body) headers.set("Content-Type", "application/json");
    if (init?.method && init.method !== "GET") headers.set("X-CSRF-Token", csrf);
    const response = await fetch(path, { ...init, headers, credentials: "same-origin", cache: "no-store" });
    const result = response.status === 204 ? { data: undefined } : await response.json() as Envelope<T>;
    if (!response.ok) throw new Error(result.error?.message ?? `Request failed (${response.status}).`);
    return result.data as T;
  }, [csrf]);

  const refresh = useCallback(async () => {
    setLoading(true); setProblem("");
    try {
      const [repos, installs] = await Promise.all([
        api<{ items: Repository[] }>("/api/v1/me/github/repositories"),
        api<{ items: Installation[] }>("/api/v1/me/github/installations"),
      ]);
      setRepositories(repos.items); setInstallations(installs.items);
    } catch (error) { setProblem(error instanceof Error ? error.message : "GitHub connections could not be loaded."); }
    finally { setLoading(false); }
  }, [api]);

  useEffect(() => { void refresh(); }, [refresh]);

  async function connect() {
    setBusy(true); setProblem("");
    try { const result = await api<{ installation_url: string }>("/api/v1/github/installations/start", { method: "POST", body: "{}" }); window.location.assign(result.installation_url); }
    catch (error) { setProblem(error instanceof Error ? error.message : "GitHub could not be opened."); setBusy(false); }
  }

  async function importRepository(repository: Repository) {
    setBusy(true); setProblem(""); setMessage("");
    try {
      const result = await api<{ authorization_url: string }>("/api/v1/projects/import/github", { method: "POST", body: JSON.stringify({ repository_id: repository.id }) });
      window.location.assign(result.authorization_url);
    } catch (error) { setProblem(error instanceof Error ? error.message : "Repository import could not be started."); setBusy(false); }
  }

  return <section className="content account-content" aria-labelledby="github-import-title">
    <p className="eyebrow">GitHub connections</p><h1 id="github-import-title">Import a project from GitHub</h1>
    <p>PeerGit imports repositories where GitHub confirms you have administrator access. Imports start private. GitHub remains the source of truth for code.</p>
    {problem && <p className="form-error" role="alert">{problem}</p>}{message && <p className="form-success" role="status">{message}</p>}
    <section className="account-form"><h2>Connected installations</h2>{installations.length ? <ul>{installations.map(item => <li key={item.id}>{item.account} · {item.target_type} · {item.status}</li>)}</ul> : <p>No verified GitHub installation is connected yet.</p>}<button className="primary-button" type="button" onClick={() => void connect()} disabled={busy}>Connect GitHub repository App</button></section>
    <section className="account-form"><div className="section-heading"><h2>Repositories you administer</h2><button className="secondary-button" type="button" onClick={() => void refresh()} disabled={loading || busy}>Refresh</button></div>
      {loading ? <p role="status">Loading authorized repositories…</p> : repositories.length ? <ul className="github-repository-list">{repositories.map(repo => <li key={repo.id}><div><h3>{repo.full_name}</h3><p>{repo.description || "No description"}</p><p>{repo.visibility} · default branch: {repo.default_branch || "not set"}{repo.topics.length ? ` · ${repo.topics.join(", ")}` : ""}</p></div><button className="primary-button" type="button" disabled={busy} onClick={() => void importRepository(repo)}>Import privately</button></li>)}</ul> : <p>No current repository candidates. Connect or refresh the GitHub App authorization to check access.</p>}
      <p className="muted">Repository administration and installation access are checked again by GitHub when the import completes.</p>
    </section>
  </section>;
}
