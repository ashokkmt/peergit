"use client";

import { FormEvent, useEffect, useState } from "react";
import { readApiEnvelope } from "@/lib/http";

type Me = {
  id: string;
  college_id: string;
  campus_status?: string;
  email: string;
  display_name: string;
  account_type: string;
  profile: { bio?: string; headline?: string; avatar_media_id?: string };
  skills: string[];
  consents: string[];
  campus_admin: boolean;
  mfa_enabled: boolean;
};
type Login = {
  authenticated: boolean;
  csrf_token?: string;
  user?: { campus_admin: boolean; mfa_enabled: boolean };
};
type CampusUser = {
  id: string;
  email: string;
  display_name: string;
  status: string;
  roles: string[];
};

const fieldClass = "grid gap-1.5 text-[13px] font-medium text-on-surface";
const inputClass =
  "w-full min-h-11 px-3.5 py-2.5 border border-control rounded-lg bg-surface text-on-surface text-sm leading-5";
const primaryBtn =
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-[18px] py-2 text-sm font-medium leading-5 no-underline whitespace-nowrap border border-primary bg-primary text-on-primary cursor-pointer hover:opacity-90 transition-opacity duration-100";
const secondaryBtn =
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-[18px] py-2 text-sm font-medium leading-5 no-underline whitespace-nowrap border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised transition-colors duration-100";
const compactSecondaryBtn =
  "inline-flex min-h-[38px] items-center justify-center gap-2 rounded-lg px-3.5 py-1.5 text-[13px] font-medium leading-5 no-underline whitespace-nowrap border border-control bg-surface text-on-surface cursor-pointer hover:bg-raised disabled:opacity-55 disabled:cursor-not-allowed";

export function AccountPanel({ section = "profile" }: { section?: "profile" | "settings" }) {
  const [csrf, setCSRF] = useState("");
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState("");
  const [problem, setProblem] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [headline, setHeadline] = useState("");
  const [bio, setBio] = useState("");
  const [skills, setSkills] = useState("");
  const [avatarID, setAvatarID] = useState("");
  const [consents, setConsents] = useState<string[]>([]);
  const [mfaSecret, setMFASecret] = useState("");
  const [mfaCode, setMFACode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteURL, setInviteURL] = useState("");
  const [orgName, setOrgName] = useState("");
  const [orgKind, setOrgKind] = useState("club");
  const [orgSlug, setOrgSlug] = useState("");
  const [inviteAccount, setInviteAccount] = useState("campus");
  const [inviteRole, setInviteRole] = useState("student");
  const [userQuery, setUserQuery] = useState("");
  const [campusUsers, setCampusUsers] = useState<CampusUser[]>([]);

  useEffect(() => {
    void load();
  }, []);

  async function api<T>(path: string, init?: RequestInit): Promise<T> {
    const response = await fetch(path, {
      ...init,
      cache: "no-store",
      headers: {
        ...(init?.body instanceof FormData ? {} : { "Content-Type": "application/json" }),
        ...(init?.headers ?? {}),
      },
    });
    if (response.status === 204) return undefined as T;
    const result = await readApiEnvelope<T>(response, `Request failed (${response.status}).`);
    if (!response.ok || result.error) {
      throw new Error(result.error?.message ?? `Request failed (${response.status}).`);
    }
    return result.data as T;
  }

  async function load() {
    setLoading(true);
    setProblem("");
    try {
      const login = await api<Login>("/api/v1/session");
      setCSRF(login.csrf_token ?? "");
      if (!login.authenticated) {
        setMe(null);
        return;
      }
      const value = await api<Me>("/api/v1/me");
      setMe(value);
      setDisplayName(value.display_name);
      setHeadline(value.profile?.headline ?? "");
      setBio(value.profile?.bio ?? "");
      setSkills(value.skills.join(", "));
      setAvatarID(value.profile?.avatar_media_id ?? "");
      setConsents(value.consents ?? []);
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "PeerGit could not load your account.");
    } finally {
      setLoading(false);
    }
  }

  async function post<T>(path: string, method: string, body: BodyInit): Promise<T> {
    return api<T>(path, { method, body, headers: { "X-CSRF-Token": csrf } });
  }

  async function saveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setProblem("");
    setMessage("");
    if (!consents.includes("terms") || !consents.includes("privacy")) {
      setProblem("Accept the Terms and Privacy Notice to finish onboarding.");
      return;
    }
    try {
      await post(
        "/api/v1/me/profile",
        "PATCH",
        JSON.stringify({
          display_name: displayName,
          headline,
          bio,
          skills: skills
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean),
          avatar_media_id: avatarID,
        }),
      );
      for (const purpose of ["terms", "privacy", "profile_discovery", "portfolio"]) {
        const granted = consents.includes(purpose);
        if (purpose === "profile_discovery" || purpose === "portfolio" || granted) {
          await post(
            "/api/v1/me/consent",
            "PUT",
            JSON.stringify({ purpose, policy_version: "draft-1", granted }),
          );
        }
      }
      setMessage("Your profile and consent choices are saved.");
      await load();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Profile could not be saved.");
    }
  }

  async function uploadImage(file?: File) {
    if (!file) return;
    setProblem("");
    setMessage("");
    const form = new FormData();
    form.set("file", file);
    try {
      const intent = await post<{ id: string }>(
        "/api/v1/media/intents",
        "POST",
        JSON.stringify({
          visibility:
            me?.college_id && consents.includes("profile_discovery") ? "campus" : "private",
        }),
      );
      const item = await post<{ id: string }>(
        `/api/v1/media/intents/${intent.id}/content`,
        "POST",
        form,
      );
      setAvatarID(item.id);
      setMessage("Image uploaded and safely re-encoded. Save your profile to use it.");
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Image upload failed.");
    }
  }

  async function enrollMFA() {
    try {
      const value = await post<{ secret: string }>("/api/v1/auth/mfa/enroll", "POST", "{}");
      setMFASecret(value.secret);
      setMessage("Add this secret to your authenticator app, then verify a code.");
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "MFA setup failed.");
    }
  }

  async function submitMFA(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      const path = mfaSecret ? "/api/v1/auth/mfa/confirm" : "/api/v1/auth/mfa/verify";
      const result = await post<{ recovery_codes?: string[] }>(
        path,
        "POST",
        JSON.stringify({ code: mfaCode }),
      );
      setRecoveryCodes(result.recovery_codes ?? []);
      setMFASecret("");
      setMFACode("");
      setMessage(
        result.recovery_codes
          ? "MFA is enabled. Save these recovery codes somewhere safe."
          : "MFA verified for privileged actions.",
      );
      await load();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "MFA verification failed.");
    }
  }

  async function invite(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const isCampus = inviteAccount === "campus";
    try {
      const result = await post<{ invitation_url: string }>(
        "/api/v1/admin/invitations",
        "POST",
        JSON.stringify({
          email: inviteEmail,
          account_type: inviteAccount,
          role: isCampus ? inviteRole : "",
          external_access_kind: isCampus ? "" : inviteRole,
          expires_hours: 72,
        }),
      );
      setInviteURL(result.invitation_url);
      setInviteEmail("");
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Invitation could not be created.");
    }
  }

  async function searchUsers(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      const result = await api<{ items: CampusUser[] }>(
        `/api/v1/admin/users?q=${encodeURIComponent(userQuery)}`,
      );
      setCampusUsers(result.items);
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Campus users could not be loaded.");
    }
  }

  async function setSuspended(user: CampusUser) {
    try {
      await post(
        `/api/v1/admin/users/${user.id}/status`,
        "POST",
        JSON.stringify({ suspended: user.status !== "suspended" }),
      );
      setCampusUsers((items) =>
        items.map((item) =>
          item.id === user.id
            ? { ...item, status: item.status === "suspended" ? "active" : "suspended" }
            : item,
        ),
      );
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "User status could not be updated.");
    }
  }

  async function grantRole(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    const userID = String(values.get("user_id"));
    const role = String(values.get("role"));
    try {
      await post("/api/v1/admin/roles", "POST", JSON.stringify({ user_id: userID, role }));
      setCampusUsers((items) =>
        items.map((item) =>
          item.id === userID ? { ...item, roles: [...new Set([...item.roles, role])] } : item,
        ),
      );
      setMessage("Campus role granted.");
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Campus role could not be granted.");
    }
  }

  async function createOrganization(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      await post(
        "/api/v1/admin/organizations",
        "POST",
        JSON.stringify({ kind: orgKind, slug: orgSlug, name: orgName }),
      );
      setMessage("Organization created.");
      setOrgName("");
      setOrgSlug("");
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Organization could not be created.");
    }
  }

  if (loading) {
    return (
      <section
        className="account-panel bg-surface border border-hairline rounded-2xl p-[clamp(20px,3vw,28px)]"
        aria-live="polite"
      >
        <p className="m-0">Checking your PeerGit account…</p>
      </section>
    );
  }

  if (!me) {
    return (
      <section
        className="account-panel bg-surface border border-hairline rounded-2xl p-[clamp(20px,3vw,28px)] flex flex-col gap-3.5"
        aria-labelledby="account-title"
      >
        <p className="m-0 text-link text-xs font-semibold tracking-[0.02em]">Campus access</p>
        <h1
          id="account-title"
          className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0"
        >
          Sign in to continue
        </h1>
        <p className="text-sm leading-[21px] text-muted m-0">
          Start with GitHub, then verify a campus email to use campus features.
        </p>
        <div>
          <a className={primaryBtn} href="/api/v1/auth/github">
            Continue with GitHub <span aria-hidden="true">→</span>
          </a>
        </div>
        {problem && (
          <p
            className="rounded-lg px-4 py-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger m-0"
            role="alert"
          >
            {problem}
          </p>
        )}
      </section>
    );
  }

  const statusPillClass = me.college_id
    ? "border-success text-success"
    : "border-warning text-warning";

  return (
    <section
      className="account-panel bg-surface border border-hairline rounded-2xl p-[clamp(20px,3vw,28px)]"
      aria-labelledby="account-title"
    >
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex gap-4 items-start">
          {section === "profile" &&
            (me.profile?.avatar_media_id ? (
              <img
                className="w-[72px] h-[72px] rounded-full border border-hairline object-cover shrink-0"
                src={`/api/v1/media/${encodeURIComponent(me.profile.avatar_media_id)}`}
                alt=""
              />
            ) : (
              <span
                className="w-[72px] h-[72px] rounded-full border border-hairline bg-raised text-on-surface grid place-items-center font-display text-[28px] font-semibold shrink-0"
                aria-hidden="true"
              >
                {me.display_name.slice(0, 1).toUpperCase()}
              </span>
            ))}
          <div className="flex flex-col gap-1.5">
            <p className="m-0 text-link text-xs font-semibold tracking-[0.02em]">
              Your PeerGit account
            </p>
            <div className="flex flex-wrap items-center gap-2.5">
              <h1
                id="account-title"
                className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0"
              >
                {section === "profile" ? me.display_name : "Account settings"}
              </h1>
              <span
                className={`inline-flex items-center px-2.5 py-1 rounded-full bg-surface border text-xs font-medium leading-4 max-w-full [overflow-wrap:anywhere] ${statusPillClass}`}
              >
                {me.college_id
                  ? "Campus verified"
                  : me.campus_status === "inactive"
                    ? "Campus access inactive"
                    : me.account_type === "unverified"
                      ? "Campus verification needed"
                      : "External account"}
              </span>
            </div>
            <p className="text-[13px] leading-[19px] text-muted m-0">{me.email}</p>
            {section === "profile" && me.profile?.headline && (
              <p className="text-sm leading-[21px] m-0">{me.profile.headline}</p>
            )}
            {section === "profile" && me.skills.length > 0 && (
              <ul className="flex flex-wrap gap-1.5 list-none m-0 p-0 mt-1" aria-label="Skills">
                {me.skills.map((skill) => (
                  <li
                    key={skill}
                    className="inline-flex items-center px-2.5 py-1 rounded bg-raised text-on-surface text-xs font-medium leading-4"
                  >
                    {skill}
                  </li>
                ))}
              </ul>
            )}
            {me.campus_status === "unverified" && (
              <a className="text-[13px] font-medium leading-[18px]" href="/onboarding">
                Continue campus onboarding
              </a>
            )}
          </div>
        </div>
      </div>

      {problem && (
        <p
          className="rounded-lg px-4 py-3 my-3 text-sm font-medium bg-danger/12 border border-danger/35 text-danger"
          role="alert"
        >
          {problem}
        </p>
      )}
      {message && (
        <p
          className="rounded-lg px-4 py-3 my-3 text-sm font-medium bg-success/12 border border-success/35 text-success"
          role="status"
        >
          {message}
        </p>
      )}

      {section === "profile" && (
        <form className="grid gap-3.5 mt-5 pt-5 border-t border-hairline" onSubmit={saveProfile}>
          <h3 className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
            Profile and privacy
          </h3>
          <label className={fieldClass}>
            Display name
            <input
              required
              maxLength={120}
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              className={inputClass}
            />
          </label>
          <label className={fieldClass}>
            Headline
            <input
              maxLength={160}
              value={headline}
              onChange={(e) => setHeadline(e.target.value)}
              placeholder="What are you interested in building?"
              className={inputClass}
            />
          </label>
          <label className={fieldClass}>
            About you
            <textarea
              maxLength={2000}
              rows={4}
              value={bio}
              onChange={(e) => setBio(e.target.value)}
              className={`${inputClass} min-h-24 resize-y leading-normal`}
            />
          </label>
          <label className={fieldClass}>
            Skills, separated by commas
            <input
              value={skills}
              onChange={(e) => setSkills(e.target.value)}
              placeholder="Go, hardware, product design"
              className={inputClass}
            />
          </label>
          <label className={fieldClass}>
            Profile image (PNG/JPEG, up to 5 MiB)
            <input
              type="file"
              accept="image/png,image/jpeg"
              onChange={(e) => void uploadImage(e.target.files?.[0])}
              className={inputClass}
            />
          </label>
          {avatarID && (
            <p className="text-[13px] leading-[19px] m-0">
              Image attached:{" "}
              <a href={`/api/v1/media/${avatarID}`}>View your uploaded image</a>
            </p>
          )}
          <fieldset className="border border-hairline rounded-xl px-4 py-3.5 m-0">
            <legend className="font-semibold text-[13px] px-1.5">Required policy choices</legend>
            <label className="flex items-start gap-2.5 my-1.5 text-sm leading-5">
              <input
                type="checkbox"
                checked={consents.includes("terms")}
                onChange={(e) =>
                  setConsents((old) =>
                    e.target.checked
                      ? [...new Set([...old, "terms"])]
                      : old.filter((v) => v !== "terms"),
                  )
                }
                className="w-4 h-4 mt-0.5 accent-primary shrink-0"
              />
              <span>
                I accept the <a href="/terms">Terms</a>.
              </span>
            </label>
            <label className="flex items-start gap-2.5 my-1.5 text-sm leading-5">
              <input
                type="checkbox"
                checked={consents.includes("privacy")}
                onChange={(e) =>
                  setConsents((old) =>
                    e.target.checked
                      ? [...new Set([...old, "privacy"])]
                      : old.filter((v) => v !== "privacy"),
                  )
                }
                className="w-4 h-4 mt-0.5 accent-primary shrink-0"
              />
              <span>
                I acknowledge the <a href="/privacy">Privacy Notice</a>.
              </span>
            </label>
          </fieldset>
          <fieldset className="border border-hairline rounded-xl px-4 py-3.5 m-0">
            <legend className="font-semibold text-[13px] px-1.5">Optional visibility</legend>
            <label className="flex items-start gap-2.5 my-1.5 text-sm leading-5">
              <input
                type="checkbox"
                checked={consents.includes("profile_discovery")}
                onChange={(e) =>
                  setConsents((old) =>
                    e.target.checked
                      ? [...new Set([...old, "profile_discovery"])]
                      : old.filter((v) => v !== "profile_discovery"),
                  )
                }
                className="w-4 h-4 mt-0.5 accent-primary shrink-0"
              />
              <span>
                Allow my profile and image to be discoverable by signed-in people at my campus.
              </span>
            </label>
            <label className="flex items-start gap-2.5 my-1.5 text-sm leading-5">
              <input
                type="checkbox"
                checked={consents.includes("portfolio")}
                onChange={(e) =>
                  setConsents((old) =>
                    e.target.checked
                      ? [...new Set([...old, "portfolio"])]
                      : old.filter((v) => v !== "portfolio"),
                  )
                }
                className="w-4 h-4 mt-0.5 accent-primary shrink-0"
              />
              <span>
                Allow the work I select for my portfolio to appear on my public page or on active
                share links. Revoking this hides the page and disables every share link.
              </span>
            </label>
          </fieldset>
          <div>
            <button className={primaryBtn} type="submit">
              Save profile
            </button>
          </div>
        </form>
      )}

      {section === "settings" && (
        <section
          className="grid gap-4 mt-5 pt-5 border-t border-hairline"
          aria-labelledby="mfa-title"
        >
          <h3 id="mfa-title" className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
            Multi-factor authentication
          </h3>
          <p className="text-sm leading-[21px] text-muted m-0">
            {me.mfa_enabled
              ? "Authenticator protection is enabled."
              : "Set up an authenticator before using campus administration."}
          </p>
          {!mfaSecret && !me.mfa_enabled && (
            <div>
              <button className={secondaryBtn} type="button" onClick={() => void enrollMFA()}>
                Set up authenticator
              </button>
            </div>
          )}
          {me.mfa_enabled && (
            <form className="grid gap-3.5" onSubmit={submitMFA}>
              <label className={fieldClass}>
                Authenticator or unused recovery code
                <input
                  required
                  autoComplete="one-time-code"
                  value={mfaCode}
                  onChange={(e) => setMFACode(e.target.value)}
                  className={inputClass}
                />
              </label>
              <div>
                <button className={secondaryBtn} type="submit">
                  Verify for 10 minutes
                </button>
              </div>
            </form>
          )}
          {mfaSecret && (
            <form className="grid gap-3.5" onSubmit={submitMFA}>
              <p className="text-[13px] leading-[19px] m-0">
                Setup secret: <code>{mfaSecret}</code>
              </p>
              <label className={fieldClass}>
                6-digit authenticator code
                <input
                  required
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  value={mfaCode}
                  onChange={(e) => setMFACode(e.target.value)}
                  className={inputClass}
                />
              </label>
              <div>
                <button className={primaryBtn} type="submit">
                  Confirm MFA
                </button>
              </div>
            </form>
          )}
          {recoveryCodes.length > 0 && (
            <div className="[overflow-wrap:anywhere] bg-raised border border-hairline rounded-lg p-3.5">
              <p className="text-[13px] leading-[19px] m-0 mb-2">
                Save these one-time codes now; they will not be shown again.
              </p>
              <ul className="columns-1 sm:columns-2 m-0 pl-5">
                {recoveryCodes.map((code) => (
                  <li key={code}>
                    <code>{code}</code>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </section>
      )}

      {section === "settings" && me.campus_admin && (
        <section
          className="grid gap-4 mt-5 pt-5 border-t border-hairline"
          aria-labelledby="admin-title"
        >
          <h3 id="admin-title" className="text-[18px] font-semibold leading-6 tracking-[-0.005em] m-0">
            Campus administration
          </h3>
          <p className="text-sm leading-[21px] text-muted m-0">
            Administrative actions require a recent MFA verification.{" "}
            <a href="/admin/campus-reviews">Review campus verification requests</a>.
          </p>
          <form className="grid gap-3.5" onSubmit={invite}>
            <label className={fieldClass}>
              Email
              <input
                type="email"
                required
                value={inviteEmail}
                onChange={(e) => setInviteEmail(e.target.value)}
                className={inputClass}
              />
            </label>
            <label className={fieldClass}>
              Account type
              <select
                value={inviteAccount}
                onChange={(e) => {
                  setInviteAccount(e.target.value);
                  setInviteRole(e.target.value === "campus" ? "student" : "mentor");
                }}
                className={inputClass}
              >
                <option value="campus">Campus member</option>
                <option value="external">Invited external</option>
              </select>
            </label>
            <label className={fieldClass}>
              {inviteAccount === "campus" ? "Campus role" : "Scoped external access"}
              <select
                value={inviteRole}
                onChange={(e) => setInviteRole(e.target.value)}
                className={inputClass}
              >
                {inviteAccount === "campus" ? (
                  <>
                    <option value="student">Student</option>
                    <option value="faculty">Faculty</option>
                    <option value="alumni_mentor">Alumni mentor</option>
                    <option value="moderator">Moderator</option>
                  </>
                ) : (
                  <>
                    <option value="mentor">Mentor</option>
                    <option value="recruiter">Recruiter</option>
                    <option value="organization_guest">Organization guest</option>
                  </>
                )}
              </select>
            </label>
            <div>
              <button className={secondaryBtn} type="submit">
                Create invitation link
              </button>
            </div>
          </form>
          {inviteURL && (
            <p className="[overflow-wrap:anywhere] bg-raised border border-hairline rounded-lg p-3.5 m-0">
              Share this one-time link with the invited person:{" "}
              <a href={inviteURL}>{inviteURL}</a>
            </p>
          )}
          <form className="grid gap-3.5" onSubmit={createOrganization}>
            <label className={fieldClass}>
              Organization name
              <input
                required
                value={orgName}
                onChange={(e) => setOrgName(e.target.value)}
                className={inputClass}
              />
            </label>
            <label className={fieldClass}>
              Slug
              <input
                required
                pattern="[a-z0-9]+(-[a-z0-9]+)*"
                value={orgSlug}
                onChange={(e) => setOrgSlug(e.target.value)}
                className={inputClass}
              />
            </label>
            <label className={fieldClass}>
              Organization type
              <select
                value={orgKind}
                onChange={(e) => setOrgKind(e.target.value)}
                className={inputClass}
              >
                <option value="club">Club</option>
                <option value="department">Department</option>
                <option value="innovation_cell">Innovation cell</option>
                <option value="placement_cell">Placement cell</option>
                <option value="event_body">Event body</option>
              </select>
            </label>
            <div>
              <button className={secondaryBtn} type="submit">
                Create organization
              </button>
            </div>
          </form>
          <form className="grid gap-3.5" onSubmit={searchUsers}>
            <label className={fieldClass}>
              Find campus accounts
              <input
                value={userQuery}
                onChange={(e) => setUserQuery(e.target.value)}
                placeholder="Name or email"
                className={inputClass}
              />
            </label>
            <div>
              <button className={secondaryBtn} type="submit">
                Search users
              </button>
            </div>
          </form>
          {campusUsers.length > 0 && (
            <ul className="list-none m-0 p-0 border-t border-hairline">
              {campusUsers.map((user) => (
                <li
                  key={user.id}
                  className="flex flex-wrap justify-between items-center gap-3 py-3.5 border-b border-hairline [overflow-wrap:anywhere]"
                >
                  <span>
                    <strong>{user.display_name}</strong>
                    <br />
                    {user.email} · {user.status} · {user.roles.join(", ") || "no roles"}
                  </span>
                  <form onSubmit={grantRole} className="flex flex-wrap gap-2.5 items-center">
                    <input type="hidden" name="user_id" value={user.id} />
                    <label className={fieldClass}>
                      Grant role
                      <select name="role" className={inputClass}>
                        {["student", "faculty", "alumni_mentor", "moderator"]
                          .filter((role) => !user.roles.includes(role))
                          .map((role) => (
                            <option key={role} value={role}>
                              {role.replaceAll("_", " ")}
                            </option>
                          ))}
                      </select>
                    </label>
                    <button
                      className={compactSecondaryBtn}
                      type="submit"
                      disabled={["student", "faculty", "alumni_mentor", "moderator"].every((role) =>
                        user.roles.includes(role),
                      )}
                    >
                      Grant
                    </button>
                  </form>
                  <button
                    className={compactSecondaryBtn}
                    type="button"
                    disabled={user.roles.includes("campus_admin")}
                    onClick={() => void setSuspended(user)}
                  >
                    {user.status === "suspended" ? "Reactivate" : "Suspend"}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
    </section>
  );
}
