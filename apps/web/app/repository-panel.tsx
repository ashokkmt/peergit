"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";

type Envelope<T> = { data?: T; error?: { message: string } };
type Binding = { id: string; full_name: string; access_state: string; sync_health: string; last_synced_at?: string | null; capabilities?: Record<string, string>; limitations: string };
type RepositoryChoice = { installation_id: string; id: number; full_name: string; private: boolean; visibility: string; default_branch: string };
type Contribution = { id: string; kind: string; canonical_id: string; author_login: string; author_name: string; title: string; summary: string; occurred_at?: string | null; additions?: number | null; deletions?: number | null; linked_user_id?: string | null };
type Snapshot = { id: string; ref: string; commit_sha: string; state: string; receipt_at: string; archive_bytes?: number | null; failure_code?: string | null; retry_available?: boolean };
type RepositoryView = { repository: Binding | null; contributions: Contribution[]; snapshots: Snapshot[] };

export function RepositoryPanel({ projectID, csrf }: { projectID: string; csrf: string }) {
  const [data, setData] = useState<RepositoryView>({ repository: null, contributions: [], snapshots: [] });
  const [choices, setChoices] = useState<RepositoryChoice[]>([]);
  const [selected, setSelected] = useState("");
  const [ref, setRef] = useState("main");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");

  const api = useCallback(async <T,>(path: string, init?: RequestInit): Promise<T> => {
    const headers = new Headers(init?.headers);
    if (init?.body) headers.set("Content-Type", "application/json");
    if (init?.method && init.method !== "GET") headers.set("X-CSRF-Token", csrf);
    const response = await fetch(path, { ...init, headers, cache: "no-store", credentials: "same-origin" });
    const envelope = response.status === 204 ? { data: undefined } : await response.json() as Envelope<T>;
    if (!response.ok) throw new Error(envelope.error?.message ?? "Repository request failed.");
    return envelope.data as T;
  }, [csrf]);

  const refresh = useCallback(async () => {
    setLoading(true);
    try { setData(await api<RepositoryView>(`/api/v1/projects/${projectID}/repository`)); setProblem(""); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Repository evidence could not be loaded."); }
    finally { setLoading(false); }
  }, [api, projectID]);

  useEffect(() => { void refresh(); }, [refresh]);

  useEffect(() => {
    if (!data.snapshots.some(item => item.state === "pending" || item.state === "capturing")) return;
    const timer = window.setInterval(() => {
      void api<RepositoryView>(`/api/v1/projects/${projectID}/repository`)
        .then(next => { setData(next); setProblem(""); })
        .catch(error => setProblem(error instanceof Error ? error.message : "Snapshot status could not be refreshed."));
    }, 5000);
    return () => window.clearInterval(timer);
  }, [api, data.snapshots, projectID]);

  async function connectGitHub() {
    setBusy(true); setProblem("");
    try { const result = await api<{ installation_url: string }>(`/api/v1/projects/${projectID}/github/installations/start`, { method: "POST" }); window.location.assign(result.installation_url); }
    catch (error) { setProblem(error instanceof Error ? error.message : "GitHub could not be opened."); setBusy(false); }
  }

  async function loadChoices() {
    setBusy(true); setProblem("");
    try { const result = await api<{ items: RepositoryChoice[] }>(`/api/v1/projects/${projectID}/github/repositories`); setChoices(result.items); if (!result.items.length) setMessage("No repositories are currently granted to this campus installation."); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Granted repositories could not be loaded."); }
    finally { setBusy(false); }
  }

  async function bindRepository(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const choice = choices.find(item => `${item.installation_id}:${item.id}` === selected); if (!choice) return;
    const [owner, name] = choice.full_name.split("/", 2); setBusy(true); setProblem("");
    try { await api(`/api/v1/projects/${projectID}/github/repositories`, { method: "POST", body: JSON.stringify({ installation_id: choice.installation_id, repository_id: choice.id, owner, name }) }); setMessage("Repository connected. Initial evidence sync is queued."); await refresh(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Repository could not be connected."); }
    finally { setBusy(false); }
  }

  async function capture(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const binding = data.repository; if (!binding) return; setBusy(true); setProblem("");
    try { await api(`/api/v1/projects/${projectID}/repository/snapshots`, { method: "POST", headers: { "Idempotency-Key": crypto.randomUUID() }, body: JSON.stringify({ binding_id: binding.id, ref }) }); setMessage("Exact commit receipt recorded. Archive capture is running in the background."); await refresh(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Snapshot request could not be recorded."); }
    finally { setBusy(false); }
  }

  async function retry(snapshotID: string) {
    setBusy(true); setProblem("");
    try { await api(`/api/v1/projects/${projectID}/repository/snapshots/${snapshotID}/retry`, { method: "POST" }); setMessage("Snapshot retry was queued for the same commit."); await refresh(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Snapshot retry is unavailable."); }
    finally { setBusy(false); }
  }

  return <section className="repository-panel" aria-labelledby="repository-panel-title">
    <div className="repository-heading"><div><p className="eyebrow">GitHub evidence</p><h4 id="repository-panel-title">Repository</h4></div><button className="secondary-button" type="button" onClick={() => void refresh()} disabled={loading || busy}>Refresh</button></div>
    {problem && <p className="form-error" role="alert">{problem}</p>}{message && <p className="form-success" role="status">{message}</p>}
    {loading ? <p role="status">Loading repository evidence…</p> : !data.repository ? <div className="repository-empty"><p>Connect a GitHub installation and select a repository for this project. PeerGit requests read access only.</p><div className="repository-empty-actions"><button className="primary-button" type="button" onClick={() => void connectGitHub()} disabled={busy}>Install or connect GitHub App</button><button className="secondary-button" type="button" onClick={() => void loadChoices()} disabled={busy}>Choose from a connected installation</button></div></div> : <>
      <div className="repository-status"><strong>{data.repository.full_name}</strong><span>Access: {data.repository.access_state.replaceAll("_", " ")}</span><span>Sync: {data.repository.sync_health.replaceAll("_", " ")}{data.repository.last_synced_at ? ` · ${new Date(data.repository.last_synced_at).toLocaleString()}` : " · not synced yet"}</span></div>
      {(data.repository.access_state !== "active" || data.repository.sync_health === "failed" || data.repository.sync_health === "delayed") && <p className="form-error" role="status">GitHub evidence may be stale or inaccessible. Recheck the App installation and selected repository permissions.</p>}
      <p className="repository-limits">{data.repository.limitations} GitHub remains the live repository; PeerGit does not accept Git pushes.</p>
      {(!data.repository.capabilities?.pull_requests || !data.repository.capabilities?.issues) && <p className="repository-limits">Pull request and issue details appear only when the GitHub App grants the corresponding read permissions.</p>}
      <form className="repository-capture" onSubmit={capture}><label htmlFor="snapshot-ref">Capture an exact commit from a branch, tag, or SHA</label><div><input id="snapshot-ref" value={ref} onChange={event => setRef(event.target.value)} maxLength={255} required /><button className="primary-button" type="submit" disabled={busy || data.repository.access_state !== "active"}>Record snapshot</button></div></form>
      <h5>Recent evidence</h5>{data.contributions.length ? <ul className="repository-activity">{data.contributions.map(item => <li key={item.id}><div><strong>{item.title || item.canonical_id.slice(-12)}</strong><p>{item.kind.replaceAll("_", " ")} · {item.author_name || item.author_login || "Unlinked author"}{item.linked_user_id ? " · PeerGit account linked" : ""}</p>{item.summary && <p>{item.summary}</p>}</div><time>{item.occurred_at ? new Date(item.occurred_at).toLocaleString() : "Time unavailable"}</time></li>)}</ul> : <p>No GitHub activity has synced yet.</p>}
      <h5>Immutable snapshots</h5>{data.snapshots.length ? <ul className="repository-snapshots">{data.snapshots.map(item => <li key={item.id}><div><strong>{item.state.replaceAll("_", " ")}</strong><p>{item.ref} → <code>{item.commit_sha.slice(0, 12)}</code> · received {new Date(item.receipt_at).toLocaleString()}{item.failure_code ? ` · ${item.failure_code}` : ""}</p></div><div className="snapshot-actions">{item.state === "verified" && <a className="secondary-button" href={`/api/v1/projects/${projectID}/repository/snapshots/${item.id}/download`}>Download verified archive</a>}{item.retry_available && <button className="secondary-button" type="button" onClick={() => void retry(item.id)} disabled={busy}>Retry same commit</button>}</div></li>)}</ul> : <p>No snapshots have been requested.</p>}
    </>}
    {data.repository && <button className="secondary-button" type="button" onClick={() => void loadChoices()} disabled={busy}>Change connected repository</button>}
    {choices.length > 0 && <form className="repository-picker" onSubmit={bindRepository}><label htmlFor="repository-choice">Select a repository accessible to your campus installation</label><select id="repository-choice" required value={selected} onChange={event => setSelected(event.target.value)}><option value="">Choose repository</option>{choices.map(item => <option key={`${item.installation_id}:${item.id}`} value={`${item.installation_id}:${item.id}`}>{item.full_name}{item.private ? " · private" : " · public"}</option>)}</select><button className="primary-button" type="submit" disabled={busy || !selected}>Connect repository</button></form>}
  </section>;
}
