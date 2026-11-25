"use client";

import { FormEvent, useEffect, useState } from "react";

type Envelope<T> = { data?: T; error?: { code: string; message: string } };
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
    const result = (await response.json()) as Envelope<T>;
    if (!response.ok || result.error)
      throw new Error(result.error?.message ?? `Request failed (${response.status}).`);
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
      for (const purpose of ["terms", "privacy", "profile_discovery"]) {
        const granted = consents.includes(purpose);
        if (purpose === "profile_discovery" || granted)
          await post(
            "/api/v1/me/consent",
            "PUT",
            JSON.stringify({ purpose, policy_version: "draft-1", granted }),
          );
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

  if (loading)
    return (
      <section className="account-panel" aria-live="polite">
        <p>Checking your PeerGit account…</p>
      </section>
    );
  if (!me)
    return (
      <section className="account-panel" aria-labelledby="account-title" style={{ display: "flex", flexDirection: "column", gap: "14px" }}>
        <p className="eyebrow" style={{ margin: 0 }}>Campus access</p>
        <h1 id="account-title" className="t-headline-md">Sign in to continue</h1>
        <p className="t-body-md muted" style={{ margin: 0 }}>
          Start with GitHub, then verify a campus email to use campus features.
        </p>
        <div>
          <a className="primary-link" href="/api/v1/auth/github">
            Continue with GitHub <span aria-hidden="true">→</span>
          </a>
        </div>
        {problem && (
          <p className="form-error" role="alert">
            {problem}
          </p>
        )}
      </section>
    );

  return (
    <section className="account-panel" aria-labelledby="account-title">
      <div className="account-heading">
        <div style={{ display: "flex", gap: "16px", alignItems: "flex-start" }}>
          {section === "profile" &&
            (me.profile?.avatar_media_id ? (
              <img
                className="profile-avatar"
                src={`/api/v1/media/${encodeURIComponent(me.profile.avatar_media_id)}`}
                alt=""
              />
            ) : (
              <span className="avatar-lg" aria-hidden="true">
                {me.display_name.slice(0, 1).toUpperCase()}
              </span>
            ))}
          <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
            <p className="eyebrow" style={{ margin: 0 }}>
              Your PeerGit account
            </p>
            <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: "10px" }}>
              <h1 id="account-title" className="t-headline-md">
                {section === "profile" ? me.display_name : "Account settings"}
              </h1>
              <span className={`pg-pill ${me.college_id ? "pg-pill-success" : "pg-pill-warning"}`}>
                {me.college_id
                  ? "Campus verified"
                  : me.campus_status === "inactive"
                    ? "Campus access inactive"
                    : me.account_type === "unverified"
                      ? "Campus verification needed"
                      : "External account"}
              </span>
            </div>
            <p className="t-body-sm muted" style={{ margin: 0 }}>
              {me.email}
            </p>
            {section === "profile" && me.profile?.headline && (
              <p className="t-body-md" style={{ margin: 0 }}>
                {me.profile.headline}
              </p>
            )}
            {section === "profile" && me.skills.length > 0 && (
              <ul className="skill-list" aria-label="Skills" style={{ marginTop: "4px" }}>
                {me.skills.map((skill) => (
                  <li key={skill}>{skill}</li>
                ))}
              </ul>
            )}
            {me.campus_status === "unverified" && (
              <a className="t-label-md" href="/onboarding">
                Continue campus onboarding
              </a>
            )}
          </div>
        </div>
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

      {section === "profile" && (
        <form className="account-form" onSubmit={saveProfile}>
          <h3 className="t-title-md">Profile and privacy</h3>
          <label>
            Display name
            <input
              required
              maxLength={120}
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
            />
          </label>
          <label>
            Headline
            <input
              maxLength={160}
              value={headline}
              onChange={(e) => setHeadline(e.target.value)}
              placeholder="What are you interested in building?"
            />
          </label>
          <label>
            About you
            <textarea
              maxLength={2000}
              rows={4}
              value={bio}
              onChange={(e) => setBio(e.target.value)}
            />
          </label>
          <label>
            Skills, separated by commas
            <input
              value={skills}
              onChange={(e) => setSkills(e.target.value)}
              placeholder="Go, hardware, product design"
            />
          </label>
          <label>
            Profile image (PNG/JPEG, up to 5 MiB)
            <input
              type="file"
              accept="image/png,image/jpeg"
              onChange={(e) => void uploadImage(e.target.files?.[0])}
            />
          </label>
          {avatarID && (
            <p className="t-body-sm">
              Image attached:{" "}
              <a href={`/api/v1/media/${avatarID}`}>View your uploaded image</a>
            </p>
          )}
          <fieldset>
            <legend>Required policy choices</legend>
            <label className="check-row">
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
              />
              <span>
                I accept the <a href="/terms">Terms</a>.
              </span>
            </label>
            <label className="check-row">
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
              />
              <span>
                I acknowledge the <a href="/privacy">Privacy Notice</a>.
              </span>
            </label>
          </fieldset>
          <fieldset>
            <legend>Optional visibility</legend>
            <label className="check-row">
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
              />
              <span>
                Allow my profile and image to be discoverable by signed-in people at my campus.
              </span>
            </label>
          </fieldset>
          <div>
            <button className="primary-button" type="submit">
              Save profile
            </button>
          </div>
        </form>
      )}

      {section === "settings" && (
        <section className="account-form" aria-labelledby="mfa-title">
          <h3 id="mfa-title" className="t-title-md">
            Multi-factor authentication
          </h3>
          <p className="t-body-md muted" style={{ margin: 0 }}>
            {me.mfa_enabled
              ? "Authenticator protection is enabled."
              : "Set up an authenticator before using campus administration."}
          </p>
          {!mfaSecret && !me.mfa_enabled && (
            <div>
              <button
                className="secondary-button"
                type="button"
                onClick={() => void enrollMFA()}
              >
                Set up authenticator
              </button>
            </div>
          )}
          {me.mfa_enabled && (
            <form onSubmit={submitMFA}>
              <label>
                Authenticator or unused recovery code
                <input
                  required
                  autoComplete="one-time-code"
                  value={mfaCode}
                  onChange={(e) => setMFACode(e.target.value)}
                />
              </label>
              <div>
                <button className="secondary-button" type="submit">
                  Verify for 10 minutes
                </button>
              </div>
            </form>
          )}
          {mfaSecret && (
            <form onSubmit={submitMFA}>
              <p className="t-body-sm">
                Setup secret: <code>{mfaSecret}</code>
              </p>
              <label>
                6-digit authenticator code
                <input
                  required
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  value={mfaCode}
                  onChange={(e) => setMFACode(e.target.value)}
                />
              </label>
              <div>
                <button className="primary-button" type="submit">
                  Confirm MFA
                </button>
              </div>
            </form>
          )}
          {recoveryCodes.length > 0 && (
            <div className="recovery-codes">
              <p className="t-body-sm">
                Save these one-time codes now; they will not be shown again.
              </p>
              <ul>
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
        <section className="account-form" aria-labelledby="admin-title">
          <h3 id="admin-title" className="t-title-md">
            Campus administration
          </h3>
          <p className="t-body-md muted" style={{ margin: 0 }}>
            Administrative actions require a recent MFA verification.{" "}
            <a href="/admin/campus-reviews">Review campus verification requests</a>.
          </p>
          <form onSubmit={invite}>
            <label>
              Email
              <input
                type="email"
                required
                value={inviteEmail}
                onChange={(e) => setInviteEmail(e.target.value)}
              />
            </label>
            <label>
              Account type
              <select
                value={inviteAccount}
                onChange={(e) => {
                  setInviteAccount(e.target.value);
                  setInviteRole(e.target.value === "campus" ? "student" : "mentor");
                }}
              >
                <option value="campus">Campus member</option>
                <option value="external">Invited external</option>
              </select>
            </label>
            <label>
              {inviteAccount === "campus" ? "Campus role" : "Scoped external access"}
              <select value={inviteRole} onChange={(e) => setInviteRole(e.target.value)}>
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
              <button className="secondary-button" type="submit">
                Create invitation link
              </button>
            </div>
          </form>
          {inviteURL && (
            <p className="invite-link">
              Share this one-time link with the invited person:{" "}
              <a href={inviteURL}>{inviteURL}</a>
            </p>
          )}
          <form onSubmit={createOrganization}>
            <label>
              Organization name
              <input
                required
                value={orgName}
                onChange={(e) => setOrgName(e.target.value)}
              />
            </label>
            <label>
              Slug
              <input
                required
                pattern="[a-z0-9]+(-[a-z0-9]+)*"
                value={orgSlug}
                onChange={(e) => setOrgSlug(e.target.value)}
              />
            </label>
            <label>
              Organization type
              <select value={orgKind} onChange={(e) => setOrgKind(e.target.value)}>
                <option value="club">Club</option>
                <option value="department">Department</option>
                <option value="innovation_cell">Innovation cell</option>
                <option value="placement_cell">Placement cell</option>
                <option value="event_body">Event body</option>
              </select>
            </label>
            <div>
              <button className="secondary-button" type="submit">
                Create organization
              </button>
            </div>
          </form>
          <form onSubmit={searchUsers}>
            <label>
              Find campus accounts
              <input
                value={userQuery}
                onChange={(e) => setUserQuery(e.target.value)}
                placeholder="Name or email"
              />
            </label>
            <div>
              <button className="secondary-button" type="submit">
                Search users
              </button>
            </div>
          </form>
          {campusUsers.length > 0 && (
            <ul className="campus-users">
              {campusUsers.map((user) => (
                <li key={user.id}>
                  <span>
                    <strong>{user.display_name}</strong>
                    <br />
                    {user.email} · {user.status} · {user.roles.join(", ") || "no roles"}
                  </span>
                  <form onSubmit={grantRole} className="button-row">
                    <input type="hidden" name="user_id" value={user.id} />
                    <label>
                      Grant role
                      <select name="role">
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
                      className="secondary-button pg-btn-compact"
                      type="submit"
                      disabled={["student", "faculty", "alumni_mentor", "moderator"].every((role) =>
                        user.roles.includes(role),
                      )}
                    >
                      Grant
                    </button>
                  </form>
                  <button
                    className="secondary-button pg-btn-compact"
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
