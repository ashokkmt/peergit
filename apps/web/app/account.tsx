"use client";

import { FormEvent, useEffect, useState } from "react";

type Envelope<T> = { data?: T; error?: { code: string; message: string } };
type Me = {
  id: string; college_id: string; email: string; display_name: string; account_type: string;
  profile: { bio?: string; headline?: string; avatar_media_id?: string };
  skills: string[]; consents: string[]; campus_admin: boolean; mfa_enabled: boolean;
};
type Login = { authenticated: boolean; csrf_token?: string; user?: { campus_admin: boolean; mfa_enabled: boolean } };
type CampusUser = { id: string; email: string; display_name: string; status: string; roles: string[] };

export function AccountPanel() {
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

  useEffect(() => { void load(); }, []);

  async function api<T>(path: string, init?: RequestInit): Promise<T> {
    const response = await fetch(path, { ...init, cache: "no-store", headers: { ...(init?.body instanceof FormData ? {} : { "Content-Type": "application/json" }), ...(init?.headers ?? {}) } });
    if (response.status === 204) return undefined as T;
    const result = await response.json() as Envelope<T>;
    if (!response.ok || result.error) throw new Error(result.error?.message ?? `Request failed (${response.status}).`);
    return result.data as T;
  }

  async function load() {
    setLoading(true); setProblem("");
    try {
      const login = await api<Login>("/api/v1/session");
      setCSRF(login.csrf_token ?? "");
      if (!login.authenticated) { setMe(null); return; }
      const value = await api<Me>("/api/v1/me");
      setMe(value); setDisplayName(value.display_name); setHeadline(value.profile?.headline ?? "");
      setBio(value.profile?.bio ?? ""); setSkills(value.skills.join(", ")); setAvatarID(value.profile?.avatar_media_id ?? "");
      setConsents(value.consents ?? []);
    } catch (error) { setProblem(error instanceof Error ? error.message : "PeerGit could not load your account."); }
    finally { setLoading(false); }
  }

  async function post<T>(path: string, method: string, body: BodyInit): Promise<T> {
    return api<T>(path, { method, body, headers: { "X-CSRF-Token": csrf } });
  }

  async function saveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setProblem(""); setMessage("");
    if (!consents.includes("terms") || !consents.includes("privacy")) { setProblem("Accept the Terms and Privacy Notice to finish onboarding."); return; }
    try {
      await post("/api/v1/me/profile", "PATCH", JSON.stringify({ display_name: displayName, headline, bio, skills: skills.split(",").map((s) => s.trim()).filter(Boolean), avatar_media_id: avatarID }));
      for (const purpose of ["terms", "privacy", "profile_discovery"]) {
        const granted = consents.includes(purpose);
        if (purpose === "profile_discovery" || granted) await post("/api/v1/me/consent", "PUT", JSON.stringify({ purpose, policy_version: "draft-1", granted }));
      }
      setMessage("Your profile and consent choices are saved."); await load();
    } catch (error) { setProblem(error instanceof Error ? error.message : "Profile could not be saved."); }
  }

  async function uploadImage(file?: File) {
    if (!file) return;
    setProblem(""); setMessage("");
    const form = new FormData(); form.set("file", file); form.set("visibility", consents.includes("profile_discovery") ? "campus" : "private");
    try { const item = await post<{ id: string }>("/api/v1/media", "POST", form); setAvatarID(item.id); setMessage("Image uploaded and safely re-encoded. Save your profile to use it."); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Image upload failed."); }
  }

  async function enrollMFA() {
    try { const value = await post<{ secret: string }>("/api/v1/auth/mfa/enroll", "POST", "{}"); setMFASecret(value.secret); setMessage("Add this secret to your authenticator app, then verify a code."); }
    catch (error) { setProblem(error instanceof Error ? error.message : "MFA setup failed."); }
  }

  async function submitMFA(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      const path = mfaSecret ? "/api/v1/auth/mfa/confirm" : "/api/v1/auth/mfa/verify";
      const result = await post<{ recovery_codes?: string[] }>(path, "POST", JSON.stringify({ code: mfaCode }));
      setRecoveryCodes(result.recovery_codes ?? []); setMFASecret(""); setMFACode(""); setMessage(result.recovery_codes ? "MFA is enabled. Save these recovery codes somewhere safe." : "MFA verified for privileged actions."); await load();
    } catch (error) { setProblem(error instanceof Error ? error.message : "MFA verification failed."); }
  }

  async function invite(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const isCampus = inviteAccount === "campus";
    try { const result = await post<{ invitation_url: string }>("/api/v1/admin/invitations", "POST", JSON.stringify({ email: inviteEmail, account_type: inviteAccount, role: isCampus ? inviteRole : "", external_access_kind: isCampus ? "" : inviteRole, expires_hours: 72 })); setInviteURL(result.invitation_url); setInviteEmail(""); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Invitation could not be created."); }
  }

  async function searchUsers(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try { const result = await api<{ items: CampusUser[] }>(`/api/v1/admin/users?q=${encodeURIComponent(userQuery)}`); setCampusUsers(result.items); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Campus users could not be loaded."); }
  }

  async function setSuspended(user: CampusUser) {
    try { await post(`/api/v1/admin/users/${user.id}/status`, "POST", JSON.stringify({ suspended: user.status !== "suspended" })); setCampusUsers((items) => items.map((item) => item.id === user.id ? { ...item, status: item.status === "suspended" ? "active" : "suspended" } : item)); }
    catch (error) { setProblem(error instanceof Error ? error.message : "User status could not be updated."); }
  }

  async function grantRole(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    const userID = String(values.get("user_id"));
    const role = String(values.get("role"));
    try {
      await post("/api/v1/admin/roles", "POST", JSON.stringify({ user_id: userID, role }));
      setCampusUsers((items) => items.map((item) => item.id === userID ? { ...item, roles: [...new Set([...item.roles, role])] } : item));
      setMessage("Campus role granted.");
    } catch (error) { setProblem(error instanceof Error ? error.message : "Campus role could not be granted."); }
  }

  async function createOrganization(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try { await post("/api/v1/admin/organizations", "POST", JSON.stringify({ kind: orgKind, slug: orgSlug, name: orgName })); setMessage("Organization created."); setOrgName(""); setOrgSlug(""); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Organization could not be created."); }
  }

  async function logout() {
    try { await post("/api/v1/auth/logout", "POST", "{}"); setMe(null); setCSRF(""); setMessage("You are signed out."); }
    catch (error) { setProblem(error instanceof Error ? error.message : "Sign out failed."); }
  }

  if (loading) return <section className="account-panel" aria-live="polite"><p>Checking your PeerGit account…</p></section>;
  if (!me) return <section className="account-panel" aria-labelledby="account-title"><div><p className="eyebrow">Campus access</p><h2 id="account-title">Sign in to continue</h2><p>Use your verified Google account. Campus access is enabled by your institution or a PeerGit invitation.</p><a className="primary-link" href="/api/v1/auth/google">Continue with Google <span aria-hidden="true">→</span></a></div>{problem && <p className="form-error" role="alert">{problem}</p>}</section>;

  return <section className="account-panel" aria-labelledby="account-title">
    <div className="account-heading"><div><p className="eyebrow">Your campus account</p><h2 id="account-title">Welcome, {me.display_name}</h2><p>{me.email}{me.college_id ? " · verified campus account" : " · invited external account"}</p></div><button className="secondary-button" type="button" onClick={logout}>Sign out</button></div>
    {problem && <p className="form-error" role="alert">{problem}</p>}{message && <p className="form-success" role="status">{message}</p>}
    <form className="account-form" onSubmit={saveProfile}>
      <h3>Profile and privacy</h3>
      <label>Display name<input required maxLength={120} value={displayName} onChange={(e) => setDisplayName(e.target.value)} /></label>
      <label>Headline<input maxLength={160} value={headline} onChange={(e) => setHeadline(e.target.value)} placeholder="What are you interested in building?" /></label>
      <label>About you<textarea maxLength={2000} rows={4} value={bio} onChange={(e) => setBio(e.target.value)} /></label>
      <label>Skills, separated by commas<input value={skills} onChange={(e) => setSkills(e.target.value)} placeholder="Go, hardware, product design" /></label>
      <label>Profile image (PNG/JPEG, up to 5 MiB)<input type="file" accept="image/png,image/jpeg" onChange={(e) => void uploadImage(e.target.files?.[0])} /></label>
      {avatarID && <p>Image attached: <a href={`/api/v1/media/${avatarID}`}>View your uploaded image</a></p>}
      <fieldset><legend>Required policy choices</legend>
        <label className="check-row"><input type="checkbox" checked={consents.includes("terms")} onChange={(e) => setConsents((old) => e.target.checked ? [...new Set([...old, "terms"])] : old.filter((v) => v !== "terms"))} />I accept the <a href="/terms">Terms</a>.</label>
        <label className="check-row"><input type="checkbox" checked={consents.includes("privacy")} onChange={(e) => setConsents((old) => e.target.checked ? [...new Set([...old, "privacy"])] : old.filter((v) => v !== "privacy"))} />I acknowledge the <a href="/privacy">Privacy Notice</a>.</label>
      </fieldset>
      <fieldset><legend>Optional visibility</legend><label className="check-row"><input type="checkbox" checked={consents.includes("profile_discovery")} onChange={(e) => setConsents((old) => e.target.checked ? [...new Set([...old, "profile_discovery"])] : old.filter((v) => v !== "profile_discovery"))} />Allow my profile and image to be discoverable by signed-in people at my campus.</label></fieldset>
      <button className="primary-button" type="submit">Save profile</button>
    </form>
    <section className="account-form" aria-labelledby="mfa-title"><h3 id="mfa-title">Multi-factor authentication</h3><p>{me.mfa_enabled ? "Authenticator protection is enabled." : "Set up an authenticator before using campus administration."}</p>
      {!mfaSecret && !me.mfa_enabled && <button className="secondary-button" type="button" onClick={() => void enrollMFA()}>Set up authenticator</button>}
      {me.mfa_enabled && <form onSubmit={submitMFA}><label>Authenticator or unused recovery code<input required autoComplete="one-time-code" value={mfaCode} onChange={(e) => setMFACode(e.target.value)} /></label><button className="secondary-button" type="submit">Verify for 10 minutes</button></form>}
      {mfaSecret && <form onSubmit={submitMFA}><p>Setup secret: <code>{mfaSecret}</code></p><label>6-digit authenticator code<input required inputMode="numeric" autoComplete="one-time-code" value={mfaCode} onChange={(e) => setMFACode(e.target.value)} /></label><button className="primary-button" type="submit">Confirm MFA</button></form>}
      {recoveryCodes.length > 0 && <div className="recovery-codes"><p>Save these one-time codes now; they will not be shown again.</p><ul>{recoveryCodes.map((code) => <li key={code}><code>{code}</code></li>)}</ul></div>}
    </section>
    {me.campus_admin && <section className="account-form" aria-labelledby="admin-title"><h3 id="admin-title">Campus administration</h3><p>Administrative actions require a recent MFA verification.</p>
      <form onSubmit={invite}><label>Email<input type="email" required value={inviteEmail} onChange={(e) => setInviteEmail(e.target.value)} /></label><label>Account type<select value={inviteAccount} onChange={(e) => { setInviteAccount(e.target.value); setInviteRole(e.target.value === "campus" ? "student" : "mentor"); }}><option value="campus">Campus member</option><option value="external">Invited external</option></select></label><label>{inviteAccount === "campus" ? "Campus role" : "Scoped external access"}<select value={inviteRole} onChange={(e) => setInviteRole(e.target.value)}>{inviteAccount === "campus" ? <><option value="student">Student</option><option value="faculty">Faculty</option><option value="alumni_mentor">Alumni mentor</option><option value="moderator">Moderator</option></> : <><option value="mentor">Mentor</option><option value="recruiter">Recruiter</option><option value="organization_guest">Organization guest</option></>}</select></label><button className="secondary-button" type="submit">Create invitation link</button></form>
      {inviteURL && <p className="invite-link">Share this one-time link with the invited person: <a href={inviteURL}>{inviteURL}</a></p>}
      <form onSubmit={createOrganization}><label>Organization name<input required value={orgName} onChange={(e) => setOrgName(e.target.value)} /></label><label>Slug<input required pattern="[a-z0-9]+(-[a-z0-9]+)*" value={orgSlug} onChange={(e) => setOrgSlug(e.target.value)} /></label><label>Organization type<select value={orgKind} onChange={(e) => setOrgKind(e.target.value)}><option value="club">Club</option><option value="department">Department</option><option value="innovation_cell">Innovation cell</option><option value="placement_cell">Placement cell</option><option value="event_body">Event body</option></select></label><button className="secondary-button" type="submit">Create organization</button></form>
      <form onSubmit={searchUsers}><label>Find campus accounts<input value={userQuery} onChange={(e) => setUserQuery(e.target.value)} placeholder="Name or email" /></label><button className="secondary-button" type="submit">Search users</button></form>
      {campusUsers.length > 0 && <ul className="campus-users">{campusUsers.map((user) => <li key={user.id}><span><strong>{user.display_name}</strong><br />{user.email} · {user.status} · {user.roles.join(", ") || "no roles"}</span><form onSubmit={grantRole}><input type="hidden" name="user_id" value={user.id} /><label>Grant role<select name="role">{["student", "faculty", "alumni_mentor", "moderator"].filter((role) => !user.roles.includes(role)).map((role) => <option key={role} value={role}>{role.replaceAll("_", " ")}</option>)}</select></label><button className="secondary-button" type="submit" disabled={["student", "faculty", "alumni_mentor", "moderator"].every((role) => user.roles.includes(role))}>Grant</button></form><button className="secondary-button" type="button" disabled={user.roles.includes("campus_admin")} onClick={() => void setSuspended(user)}>{user.status === "suspended" ? "Reactivate" : "Suspend"}</button></li>)}</ul>}
    </section>}
  </section>;
}
