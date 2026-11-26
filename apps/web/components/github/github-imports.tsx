"use client";

import { useCallback, useEffect, useState } from "react";
import { PreviewBadge } from "@/components/layout/preview-banner";
import { readApiEnvelope } from "@/lib/http";

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

const cardClass = "min-w-0 max-w-full bg-surface border border-hairline rounded-2xl p-6 flex flex-col";
const primaryBtn =
  "inline-flex min-h-11 items-center justify-center rounded-lg px-[18px] py-2 text-sm font-medium border border-primary bg-primary text-on-primary cursor-pointer hover:opacity-90 disabled:opacity-55 disabled:cursor-not-allowed";
const compactPrimaryBtn =
  "inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium whitespace-nowrap border border-primary bg-primary text-on-primary cursor-pointer hover:opacity-90 disabled:opacity-55 disabled:cursor-not-allowed";
const compactSecondaryBtn =
  "inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline whitespace-nowrap border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised disabled:opacity-55 disabled:cursor-not-allowed";
const compactDangerBtn =
  "inline-flex min-h-[38px] items-center justify-center rounded-lg px-3.5 py-1.5 text-[13px] font-medium whitespace-nowrap border border-danger bg-danger text-on-primary cursor-pointer hover:opacity-90 disabled:opacity-55 disabled:cursor-not-allowed";

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
      if (response.status === 204) return undefined as T;
      const result = await readApiEnvelope<T>(response, `Request failed (${response.status}).`);
      if (!response.ok) {
        throw new Error(result.error?.message ?? `Request failed (${response.status}).`);
      }
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
    <section className="w-full flex flex-col gap-5" aria-labelledby="github-import-title">
      <div>
        <p className="mt-0 mb-1.5 text-link text-xs font-semibold tracking-[0.02em]">
          GitHub connection
        </p>
        <h1
          id="github-import-title"
          className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0"
        >
          Import a project from GitHub
        </h1>
        <p className="text-sm leading-[21px] text-muted mt-1.5 mb-0">
          Signed-in GitHub identity is separate from repository installations. PeerGit imports
          repositories where GitHub confirms you have administrator access.
        </p>
      </div>

      {problem && (
        <p
          className="rounded-lg px-4 py-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger m-0"
          role="alert"
        >
          {problem}
        </p>
      )}
      {message && (
        <p
          className="rounded-lg px-4 py-3 text-sm font-medium bg-success/12 border border-success/35 text-success m-0"
          role="status"
        >
          {message}
        </p>
      )}

      <section className={`${cardClass} gap-4`}>
        <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
          Connected installations
        </h2>
        {installations.length ? (
          <ul className="list-none p-0 m-0 grid gap-3">
            {installations.map((item) => (
              <li
                key={item.id}
                className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl p-5 flex flex-wrap items-center justify-between gap-3"
              >
                <div>
                  <strong className="text-base font-semibold leading-[22px]">{item.account}</strong>{" "}
                  <span className="text-[13px] leading-[19px] text-muted">
                    · {item.target_type} · {item.status}
                  </span>
                </div>
                <div className="flex flex-wrap gap-2.5 items-center">
                  <a
                    className={compactSecondaryBtn}
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
                    className={compactSecondaryBtn}
                    type="button"
                    disabled={busy || item.status !== "active"}
                    onClick={() => void refreshInstallation(item)}
                  >
                    Refresh repositories
                  </button>
                  <button
                    className={compactDangerBtn}
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
          <div className="flex flex-col gap-3 items-start">
            <p className="text-sm leading-[21px] text-muted m-0">
              No verified GitHub installation is connected yet.
            </p>
            <button
              className={primaryBtn}
              type="button"
              onClick={() => void connect()}
              disabled={busy}
            >
              Connect GitHub repository App
            </button>
          </div>
        )}

        <div className="flex items-start gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px]">
          <span>
            <strong className="text-on-surface">Revocation policy:</strong> Revoking an installation
            hides affected owned projects privately for a 30-day recovery window before purge. Fresh
            authorization restores them privately.
          </span>
        </div>
      </section>

      {retained.length > 0 && (
        <section className={`${cardClass} gap-3`}>
          <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
            Recover recently revoked imports
          </h2>
          <p className="text-[13px] leading-[19px] text-muted m-0">
            Restored projects are always private. Reconnect the GitHub App and refresh repository
            access before restoring.
          </p>
          <ul className="list-none p-0 m-0 grid gap-2.5">
            {retained.map((item) => (
              <li
                key={item.id}
                className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl p-5 flex flex-wrap items-center justify-between gap-3"
              >
                <span className="text-sm leading-[21px]">
                  <strong>{item.title}</strong> · recover until{" "}
                  {new Date(item.delete_after).toLocaleString()}
                </span>
                <button
                  className={compactSecondaryBtn}
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

      <section className={`${cardClass} gap-3.5`}>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
            Repositories you administer
          </h2>
          {installations.length === 0 && (
            <button
              className={primaryBtn}
              type="button"
              onClick={() => void connect()}
              disabled={busy}
            >
              Connect GitHub repository App
            </button>
          )}
        </div>

        {loading ? (
          <p role="status" className="m-0">
            Loading authorized repositories…
          </p>
        ) : repositories.length ? (
          <ul className="list-none m-0 p-0 grid gap-3">
            {repositories.map((repo) => (
              <li
                key={`${repo.installation_id}:${repo.id}`}
                className="flex flex-wrap items-center justify-between gap-3.5 p-4 bg-canvas border border-hairline rounded-xl"
              >
                <div>
                  <h3 className="font-mono text-[13px] leading-[19px] font-semibold mt-0 mb-1">
                    {repo.full_name}
                  </h3>
                  <p className="text-[13px] leading-[19px] text-muted mt-0 mb-1">
                    {repo.description || "No description"}
                  </p>
                  <p className="text-[13px] leading-[19px] text-muted m-0">
                    {repo.visibility} · default branch: {repo.default_branch || "not set"}
                    {repo.topics.length ? ` · ${repo.topics.join(", ")}` : ""}
                  </p>
                </div>
                <button
                  className={compactPrimaryBtn}
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
          <p className="text-sm leading-[21px] text-muted m-0">
            No current repository candidates. Connect or refresh the GitHub App authorization to
            check access.
          </p>
        )}

        {expiredCount > 0 && (
          <p
            role="status"
            className="flex items-start gap-3 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px] m-0"
          >
            Your previous repository list expired. Refresh a connected installation above to check
            GitHub for current access.
          </p>
        )}
        <p className="text-[13px] leading-[19px] text-muted m-0">
          Repository administration and installation access are checked again by GitHub when the
          import completes.
        </p>
      </section>

      <section className={`${cardClass} gap-3`}>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <span className="text-xs font-medium leading-4 text-link">
              Qualified Pilot (Stage B1)
            </span>
            <h2 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] mt-0.5 mb-0">
              Scoped HTTPS Git credentials
            </h2>
          </div>
          <PreviewBadge phase="Stage B1" />
        </div>
        <div className="min-w-0 max-w-full bg-canvas border border-hairline rounded-2xl p-5 flex flex-wrap items-center justify-between gap-3">
          <div>
            <p className="font-mono text-[13px] leading-[19px] m-0">
              pg_tok_northgate_9a4c••••••••
            </p>
            <p className="text-[13px] leading-[19px] text-muted mt-1 mb-0">
              Sample credential preview · Expires 30 days after issue
            </p>
          </div>
          <button type="button" className={compactSecondaryBtn} disabled>
            Generate credential
          </button>
        </div>
      </section>
    </section>
  );
}
