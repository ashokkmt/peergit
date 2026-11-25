"use client";

import { useCallback, useEffect, useState } from "react";
import { PreviewBadge } from "@/components/layout/preview-banner";

type Envelope<T> = { data?: T; error?: { message?: string } };
type Repository = {
  id: number;
  installation_id: string;
  full_name: string;
  description: string;
  visibility: string;
  default_branch: string;
  topics: string[];
  expires_at: string;
};
type Installation = {
  id: string;
  installation_id: string;
  account: string;
  target_type: string;
  status: string;
  confirmed_at: string;
};
type RetainedProject = { id: string; title: string; delete_after: string };

export function GitHubImports({ csrf }: { csrf: string }) {
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [installations, setInstallations] = useState<Installation[]>([]);
  const [retained, setRetained] = useState<RetainedProject[]>([]);
  const [expiredCount, setExpiredCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [problem, setProblem] = useState("");

  const api = useCallback(
    async <T,>(path: string, init?: RequestInit): Promise<T> => {
      const headers = new Headers(init?.headers);
      if (init?.body) headers.set("Content-Type", "application/json");
      if (init?.method && init.method !== "GET") headers.set("X-CSRF-Token", csrf);
      const response = await fetch(path, {
        ...init,
        headers,
        credentials: "same-origin",
        cache: "no-store",
      });
      const result =
        response.status === 204 ? { data: undefined } : ((await response.json()) as Envelope<T>);
      if (!response.ok)
        throw new Error(result.error?.message ?? `Request failed (${response.status}).`);
      return result.data as T;
    },
    [csrf],
  );

  const refresh = useCallback(async () => {
    setLoading(true);
    setProblem("");
    try {
      const [repos, installs, recovery] = await Promise.all([
        api<{ items: Repository[]; expired_count: number }>("/api/v1/me/github/repositories"),
        api<{ items: Installation[] }>("/api/v1/me/github/installations"),
        api<{ items: RetainedProject[] }>("/api/v1/me/github/retained-projects"),
      ]);
      setRepositories(repos.items);
      setInstallations(installs.items);
      setExpiredCount(repos.expired_count);
      setRetained(recovery.items);
    } catch (error) {
      setProblem(
        error instanceof Error ? error.message : "GitHub connections could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [api]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function connect() {
    setBusy(true);
    setProblem("");
    try {
      const result = await api<{ installation_url: string }>("/api/v1/github/installations/start", {
        method: "POST",
        body: "{}",
      });
      window.location.assign(result.installation_url);
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "GitHub could not be opened.");
      setBusy(false);
    }
  }

  async function refreshInstallation(item: Installation) {
    setBusy(true);
    setProblem("");
    try {
      const result = await api<{ authorization_url: string }>(
        `/api/v1/me/github/installations/${item.id}/refresh`,
        { method: "POST", body: "{}" },
      );
      window.location.assign(result.authorization_url);
    } catch (error) {
      setProblem(
        error instanceof Error ? error.message : "Repository access could not be refreshed.",
      );
      setBusy(false);
    }
  }

  async function revokeInstallation(item: Installation) {
    setBusy(true);
    setProblem("");
    try {
      const owned = await api<{ items: { title: string }[] }>(
        `/api/v1/me/github/installations/${item.id}/imports`,
      );
      const titles = owned.items.length
        ? `\n\nAffected projects:\n${owned.items.map((x) => `• ${x.title}`).join("\n")}`
        : "";
      if (
        !window.confirm(
          `Revoke this PeerGit connection? Its imported projects will disappear immediately. Their project, team, evidence, and snapshots remain recoverable for 30 days, then are permanently deleted.${titles}\n\nThis does not uninstall the GitHub App.`,
        )
      ) {
        setBusy(false);
        return;
      }
      await api(`/api/v1/me/github/installations/${item.id}/revoke`, {
        method: "POST",
        body: "{}",
      });
      setMessage(
        "Connection revoked. Its imports are hidden and can be restored for 30 days after reconnecting and refreshing GitHub.",
      );
      await refresh();
    } catch (error) {
      setProblem(
        error instanceof Error ? error.message : "GitHub connection could not be revoked.",
      );
    } finally {
      setBusy(false);
    }
  }

  async function restoreProject(project: RetainedProject) {
    setBusy(true);
    setProblem("");
    try {
      await api(`/api/v1/projects/${project.id}/github/restore`, {
        method: "POST",
        body: "{}",
      });
      setMessage(`${project.title} restored privately.`);
      await refresh();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Project could not be restored.");
    } finally {
      setBusy(false);
    }
  }

  async function importRepository(repository: Repository) {
    setBusy(true);
    setProblem("");
    setMessage("");
    try {
      const result = await api<{ authorization_url: string }>("/api/v1/projects/import/github", {
        method: "POST",
        body: JSON.stringify({
          repository_id: repository.id,
          installation_id: repository.installation_id,
        }),
      });
      window.location.assign(result.authorization_url);
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Repository import could not be started.");
      setBusy(false);
    }
  }

  return (
    <section
      className="content account-content"
      aria-labelledby="github-import-title"
      style={{ display: "flex", flexDirection: "column", gap: "20px" }}
    >
      <div>
        <p className="eyebrow">GitHub connection</p>
        <h1 id="github-import-title" className="t-headline-md">
          Import a project from GitHub
        </h1>
        <p className="t-body-md muted" style={{ margin: "6px 0 0" }}>
          Signed-in GitHub identity is separate from repository installations. PeerGit imports
          repositories where GitHub confirms you have administrator access.
        </p>
      </div>

      {problem && (
        <p className="form-error" role="alert">
          {problem}
        </p>
      )}
      {message && (
        <p className="form-success" role="status">
          {message}
        </p>
      )}

      <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
        <h2 className="t-title-md">Connected installations</h2>
        {installations.length ? (
          <ul
            style={{
              listStyle: "none",
              padding: 0,
              margin: 0,
              display: "grid",
              gap: "12px",
            }}
          >
            {installations.map((item) => (
              <li
                key={item.id}
                className="pg-card-canvas"
                style={{
                  display: "flex",
                  flexWrap: "wrap",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: "12px",
                }}
              >
                <div>
                  <strong className="t-title-sm">{item.account}</strong>{" "}
                  <span className="t-body-sm muted">
                    · {item.target_type} · {item.status}
                  </span>
                </div>
                <div className="button-row">
                  <a
                    className="secondary-button pg-btn-compact"
                    href={
                      item.target_type === "Organization"
                        ? `https://github.com/organizations/${encodeURIComponent(item.account)}/settings/installations/${item.installation_id}`
                        : `https://github.com/settings/installations/${item.installation_id}`
                    }
                    target="_blank"
                    rel="noreferrer"
                  >
                    Manage repository access on GitHub
                  </a>
                  <button
                    className="secondary-button pg-btn-compact"
                    type="button"
                    disabled={busy || item.status !== "active"}
                    onClick={() => void refreshInstallation(item)}
                  >
                    Refresh repositories
                  </button>
                  <button
                    className="danger-button pg-btn-compact"
                    type="button"
                    disabled={busy}
                    onClick={() => void revokeInstallation(item)}
                  >
                    Revoke PeerGit connection
                  </button>
                </div>
              </li>
            ))}
          </ul>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: "12px", alignItems: "flex-start" }}>
            <p className="t-body-md muted" style={{ margin: 0 }}>
              No verified GitHub installation is connected yet.
            </p>
            <button
              className="primary-button"
              type="button"
              onClick={() => void connect()}
              disabled={busy}
            >
              Connect GitHub repository App
            </button>
          </div>
        )}

        <div className="pg-callout">
          <span>
            <strong>Revocation policy:</strong> Revoking an installation hides affected owned
            projects privately for a 30-day recovery window before purge. Fresh authorization
            restores them privately.
          </span>
        </div>
      </section>

      {retained.length > 0 && (
        <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
          <h2 className="t-title-md">Recover recently revoked imports</h2>
          <p className="t-body-sm muted" style={{ margin: 0 }}>
            Restored projects are always private. Reconnect the GitHub App and refresh repository
            access before restoring.
          </p>
          <ul style={{ listStyle: "none", padding: 0, margin: 0, display: "grid", gap: "10px" }}>
            {retained.map((item) => (
              <li
                key={item.id}
                className="pg-card-canvas"
                style={{
                  display: "flex",
                  flexWrap: "wrap",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: "12px",
                }}
              >
                <span className="t-body-md">
                  <strong>{item.title}</strong> · recover until{" "}
                  {new Date(item.delete_after).toLocaleString()}
                </span>
                <button
                  className="secondary-button pg-btn-compact"
                  disabled={busy}
                  onClick={() => void restoreProject(item)}
                >
                  Restore privately
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}

      <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "14px" }}>
        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "space-between",
            gap: "12px",
          }}
        >
          <h2 className="t-title-md">Repositories you administer</h2>
          {installations.length === 0 && (
            <button
              className="primary-button"
              type="button"
              onClick={() => void connect()}
              disabled={busy}
            >
              Connect GitHub repository App
            </button>
          )}
        </div>

        {loading ? (
          <p role="status">Loading authorized repositories…</p>
        ) : repositories.length ? (
          <ul className="github-repository-list">
            {repositories.map((repo) => (
              <li key={`${repo.installation_id}:${repo.id}`}>
                <div>
                  <h3 className="t-mono-sm" style={{ fontWeight: 600, marginBottom: "4px" }}>
                    {repo.full_name}
                  </h3>
                  <p className="t-body-sm muted" style={{ margin: "0 0 4px" }}>
                    {repo.description || "No description"}
                  </p>
                  <p className="t-body-sm muted" style={{ margin: 0 }}>
                    {repo.visibility} · default branch: {repo.default_branch || "not set"}
                    {repo.topics.length ? ` · ${repo.topics.join(", ")}` : ""}
                  </p>
                </div>
                <button
                  className="primary-button pg-btn-compact"
                  type="button"
                  disabled={busy}
                  onClick={() => void importRepository(repo)}
                >
                  Import privately
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="t-body-md muted" style={{ margin: 0 }}>
            No current repository candidates. Connect or refresh the GitHub App authorization to
            check access.
          </p>
        )}

        {expiredCount > 0 && (
          <p role="status" className="pg-callout">
            Your previous repository list expired. Refresh a connected installation above to check
            GitHub for current access.
          </p>
        )}
        <p className="t-body-sm muted" style={{ margin: 0 }}>
          Repository administration and installation access are checked again by GitHub when the
          import completes.
        </p>
      </section>

      <section className="pg-card" style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "space-between",
            gap: "12px",
          }}
        >
          <div>
            <span className="t-label-sm" style={{ color: "var(--link)" }}>
              Qualified Pilot (Stage B1)
            </span>
            <h2 className="t-title-md" style={{ marginTop: "2px" }}>
              Scoped HTTPS Git credentials
            </h2>
          </div>
          <PreviewBadge phase="Stage B1" />
        </div>
        <div
          className="pg-card-canvas"
          style={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "space-between",
            gap: "12px",
          }}
        >
          <div>
            <p className="t-mono-sm" style={{ margin: 0 }}>
              pg_tok_northgate_9a4c••••••••
            </p>
            <p className="t-body-sm muted" style={{ margin: "4px 0 0" }}>
              Sample credential preview · Expires 30 days after issue
            </p>
          </div>
          <button type="button" className="secondary-button pg-btn-compact" disabled>
            Generate credential
          </button>
        </div>
      </section>
    </section>
  );
}
