"use client";

import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { fetchSession, readApiEnvelope } from "@/lib/http";

const root = "/api/v1/academic/pbl";
const button = "inline-flex min-h-10 items-center rounded-lg border border-control bg-surface px-3 text-sm font-medium text-on-surface hover:bg-raised disabled:opacity-50";
const field = "min-h-10 min-w-0 w-full rounded-lg border border-control bg-surface px-3 py-2 text-sm text-on-surface";
const stageNames = ["Idea presentation", "Phase 1", "Phase 2", "Phase 3"];
type Member = { id: string; name: string; section: string; user_id?: string; roll_number?: string };
type Checkpoint = { stage: number; sha: string; deadline: string; receipt_at: string; capture_state: string; archive_sha256?: string; feedback: string };
type Team = {
  id: string; offering_id: string; title: string; confirmed: boolean; is_leader: boolean; can_review: boolean;
  faculty_name: string; project_id?: string; idea: string; idea_state: string; idea_feedback: string;
  members: Member[]; attendance: { student_id: string; stage: number; present: boolean }[]; checkpoints: Checkpoint[];
};
type Offering = { id: string; course: string; course_code: string; term: string; closes_at: string; eligible: boolean };
type Workspace = { coordinator: boolean; offerings: Offering[]; teams: Team[] };
type Choice = { id: string; label?: string; title?: string; name?: string; status?: string };
type Repository = { state: string; mode: string; can_provision: boolean; can_git: boolean; clone_url?: string; storage_limit_kib: number; project_id?: string; campus_visible?: boolean };
type Source = { id: number; installation_id: string; full_name: string };
type Credential = { username: string; password: string; expires_at: string };
type Branch = { name: string; commit: { id: string } };
type FileEntry = { path: string; name: string; type: string; content?: string; encoding?: string };

async function read<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { ...init, cache: "no-store" });
  if (res.status === 204) return undefined as T;
  const result = await readApiEnvelope<T>(res, "This PBL request could not be completed.");
  if (!res.ok || !result.data || result.error) throw new Error(result.error?.message ?? "This PBL request could not be completed.");
  return result.data;
}

function Panel({ title, children }: { title: string; children: ReactNode }) {
  return <section className="grid min-w-0 gap-3 rounded-xl border border-hairline bg-surface p-4 sm:p-5"><h2 className="m-0 font-display text-xl font-semibold">{title}</h2>{children}</section>;
}

function Field({ label, name, type = "text", maxLength, required = true, defaultValue }: { label: string; name: string; type?: string; maxLength?: number; required?: boolean; defaultValue?: string }) {
  return <label className="grid min-w-0 gap-1 text-sm font-medium">{label}<input className={field} name={name} type={type} maxLength={maxLength} required={required} defaultValue={defaultValue} /></label>;
}

function Select({ label, name, items }: { label: string; name: string; items: { id: string; label: string }[] }) {
  return <label className="grid min-w-0 gap-1 text-sm font-medium">{label}<select name={name} className={field} required defaultValue="">
    <option value="">Choose {label.toLowerCase()}</option>
    {items.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}
  </select></label>;
}

type Action = <T>(path: string, method: string, data?: unknown) => Promise<T>;
type FormAction = (event: FormEvent<HTMLFormElement>, action: (data: FormData) => Promise<unknown>, message: string) => void;

export function PBLWorkspace() {
  const [data, setData] = useState<Workspace>();
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [csrf, setCSRF] = useState("");
  const [selected, setSelected] = useState("");
  const [catalog, setCatalog] = useState<{ courses: Choice[]; terms: Choice[]; faculty: Choice[] }>();
  const [registrationSize, setRegistrationSize] = useState(1);

  async function load() {
    setError("");
    setLoading(true);
    try {
      const [session, workspace] = await Promise.all([fetchSession().then((res) => readApiEnvelope<{ csrf_token?: string }>(res, "Sign in to continue.")), read<Workspace>(root)]);
      setCSRF(session.data?.csrf_token ?? "");
      setData(workspace);
      if (workspace.coordinator) {
        const [courses, terms, faculty] = await Promise.all(["courses", "terms", "faculty"].map((name) => read<{ items: Choice[] }>(`/api/v1/academic/${name}`)));
        setCatalog({ courses: courses.items.filter((item) => item.status === "active"), terms: terms.items, faculty: faculty.items });
      }
    } catch (reason) {
      setData(undefined);
      setError(reason instanceof Error ? reason.message : "Could not load PBL.");
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => { void load(); }, []);

  const action: Action = (path, method, payload) => read(path, {
    method, headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: payload === undefined ? undefined : JSON.stringify(payload),
  });
  const submit: FormAction = (e, operation, message) => {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setError("");
    setStatus("");
    void operation(form).then(async () => {
      setStatus(message);
      await load();
    }).catch((reason: unknown) => setError(reason instanceof Error ? reason.message : "The action failed."));
  };
  const team = data?.teams.find((item) => item.id === selected) ?? data?.teams[0];
  const offering = data?.offerings.find((item) => item.id === team?.offering_id);
  const options = (items: Choice[]) => items.map((item) => ({ id: item.id, label: item.label ?? item.title ?? item.name ?? item.id }));

  return <main id="main-content" className="mx-auto grid w-full max-w-6xl gap-5 px-4 py-6 sm:px-6">
    <header className="flex flex-wrap items-end justify-between gap-3">
      <div>
        <a href="/academics" className="text-sm font-medium text-link hover:underline">Courses and academics</a>
        <h1 className="mt-2 mb-1 font-display text-3xl font-bold">Project-based learning</h1>
        <p className="m-0 text-sm text-muted">Manage your coursework team, private PeerGit repository, and checkpoint evidence.</p>
      </div>
      <button type="button" className={button} onClick={() => void load()} disabled={loading}>Refresh</button>
    </header>
    {error && <p role="alert" className="m-0 rounded-lg border border-danger/40 bg-danger/10 p-3 text-sm text-danger">{error}</p>}
    {status && <p role="status" className="m-0 text-sm text-success">{status}</p>}
    {loading && !data && <p role="status" className="text-sm text-muted">Loading PBL workspace…</p>}
    {data && <>
      <div className="grid min-w-0 gap-4 lg:grid-cols-2">
        {data.coordinator && catalog && <Panel title="Publish registration">
          <form className="grid gap-3 sm:grid-cols-2" onSubmit={(e) => submit(e, (form) => action(root + "/offerings", "POST", {
            course_id: form.get("course_id"), term_id: form.get("term_id"),
            ...Object.fromEntries(["closes_at", "phase_one_due", "phase_two_due", "phase_three_due"].map((key) => [key, new Date(String(form.get(key))).toISOString()])),
          }), "Registrations opened.")}>
            <Select label="Subject" name="course_id" items={options(catalog.courses)} />
            <Select label="Term" name="term_id" items={options(catalog.terms)} />
            {["closes_at", "phase_one_due", "phase_two_due", "phase_three_due"].map((key, index) =>
              <Field key={key} label={["Registration closes", "Phase 1 deadline", "Phase 2 deadline", "Phase 3 deadline"][index]} name={key} type="datetime-local" />)}
            <button className={button} disabled={!csrf}>Publish form</button>
          </form>
        </Panel>}
        <Panel title="Register a team">
          <form className="grid gap-3" onSubmit={(e) => submit(e, (form) => action(root + "/offerings/" + form.get("offering") + "/teams", "POST", {
            title: form.get("title"),
            members: Array.from({ length: registrationSize }, (_, index) => ({ name: form.get(`name_${index}`), roll_number: form.get(`roll_${index}`), section: form.get(`section_${index}`) })),
          }), "Roster submitted for reconciliation.")}>
            <Select label="Offering" name="offering" items={data.offerings.filter((o) => o.eligible && new Date(o.closes_at) > new Date()).map((o) => ({ id: o.id, label: `${o.course_code} · ${o.term}` }))} />
            <Field label="Team project name" name="title" maxLength={120} />
            {Array.from({ length: registrationSize }, (_, index) => <fieldset key={index} className="grid gap-2 rounded-lg border border-hairline p-3 sm:grid-cols-3">
              <legend className="px-1 text-sm font-medium">{index === 0 ? "Team leader" : `Member ${index + 1}`}</legend>
              <Field label="Student name" name={`name_${index}`} maxLength={120} />
              <Field label="Roll number" name={`roll_${index}`} maxLength={40} />
              <Field label="Section" name={`section_${index}`} maxLength={40} />
            </fieldset>)}
            <div className="flex flex-wrap gap-2">
              <button type="button" className={button} disabled={registrationSize >= 12} onClick={() => setRegistrationSize((size) => size + 1)}>Add member</button>
              {registrationSize > 1 && <button type="button" className={button} onClick={() => setRegistrationSize((size) => size - 1)}>Remove last member</button>}
              <button className={button} disabled={!csrf}>Submit roster</button>
            </div>
          </form>
        </Panel>
      </div>
      {team ? <>
        <label className="grid gap-1 text-sm font-medium">Team
          <select className={field} value={team.id} onChange={(e) => setSelected(e.target.value)}>
            {data.teams.map((item) => <option key={item.id} value={item.id}>{item.title} · {data.offerings.find((o) => o.id === item.offering_id)?.course_code}</option>)}
          </select>
        </label>
        <TeamPanel key={team.id} team={team} offering={offering} action={action} submit={submit} coordinator={data.coordinator} faculty={options(catalog?.faculty ?? [])} />
      </> : <p className="text-sm text-muted">No teams are available to this account yet.</p>}
    </>}
  </main>;
}

function TeamPanel({ team, offering, action, submit, coordinator, faculty }: {
  team: Team; offering?: Offering; action: Action; submit: FormAction; coordinator: boolean; faculty: { id: string; label: string }[];
}) {
  const banner = root + "/teams/" + team.id;
  const [roomError, setRoomError] = useState("");
  const openRoom = async (scope: string) => {
    setRoomError("");
    try {
      const room = await action<{ id: string }>(banner + "/rooms/" + scope, "POST", {});
      window.location.assign("/messages/" + room.id);
    } catch (reason) {
      setRoomError(reason instanceof Error ? reason.message : "This room is unavailable.");
    }
  };
  return <div className="grid min-w-0 gap-4">
    <Panel title={team.title}>
      <p className="m-0 text-sm text-muted">{[offering?.course, offering?.term, team.confirmed ? "Roster confirmed" : "Roster pending", team.faculty_name || "Faculty unassigned"].filter(Boolean).join(" · ")}</p>
      <ul className="m-0 grid gap-1 pl-5 text-sm">{team.members.map((m) => <li key={m.id}>{m.name} · {m.section}{m.roll_number ? ` · ${m.roll_number}` : ""}</li>)}</ul>
      {coordinator && !team.confirmed && <form className="grid gap-2" onSubmit={(e) => submit(e, (form) => action(banner + "/roster", "PUT", {
        matches: team.members.map((m) => ({ entry_id: m.id, handle: form.get(m.id) })),
      }), "Roster confirmed.")}>
        {team.members.map((m) => <Field key={m.id} label={`Verified handle for ${m.name}`} name={m.id} maxLength={30} />)}
        <button className={button}>Confirm roster</button>
      </form>}
      {coordinator && team.confirmed && <form className="grid gap-2 sm:grid-cols-[1fr_auto] sm:items-end" onSubmit={(e) => submit(e, (form) => action(banner + "/faculty", "PUT", { faculty_id: form.get("faculty") }), "Faculty assignment saved.")}>
        <Select label="Faculty" name="faculty" items={faculty} />
        <button className={button}>Assign faculty</button>
      </form>}
      {team.confirmed && <div className="flex flex-wrap gap-2">
        <button type="button" className={button} onClick={() => void openRoom("team")}>Open team room</button>
        {team.faculty_name && <button type="button" className={button} onClick={() => void openRoom("faculty")}>Open faculty group</button>}
      </div>}
      {roomError && <p role="alert" className="m-0 text-sm text-danger">{roomError}</p>}
    </Panel>
    {team.confirmed && <>
      <GitPanel banner={banner} action={action} leader={team.is_leader} />
      <Panel title="Project idea">
        <p className="m-0 text-sm font-medium">{team.idea_state === "revise" ? "Revision requested" : team.idea_state.replaceAll("_", " ")}</p>
        {team.idea && <p className="m-0 whitespace-pre-wrap break-words text-sm">{team.idea}</p>}
        {team.idea_feedback && <p className="m-0 whitespace-pre-wrap break-words text-sm text-muted">{team.idea_feedback}</p>}
        {team.is_leader && ["draft", "revise"].includes(team.idea_state) && <form className="grid gap-2" onSubmit={(e) => submit(e, (form) => action(banner + "/idea", "POST", { idea: form.get("idea") }), "Idea submitted.")}>
          <label className="grid gap-1 text-sm font-medium">Idea description<textarea name="idea" className={field} required maxLength={4000} rows={4} defaultValue={team.idea} /></label>
          <button className={button}>Submit idea</button>
        </form>}
        {team.can_review && team.idea_state === "submitted" && <form className="grid gap-2" onSubmit={(e) => submit(e, (form) => action(banner + "/idea", "PUT", { decision: form.get("decision"), feedback: form.get("feedback") }), "Idea review saved.")}>
          <Select name="decision" label="Decision" items={[{ id: "approved", label: "Approve" }, { id: "revise", label: "Request revision" }]} />
          <Field label="Feedback" name="feedback" maxLength={2000} required={false} />
          <button className={button}>Save idea review</button>
        </form>}
      </Panel>
      <Panel title="Attendance and checkpoints">
        {(coordinator || team.can_review) && <a className="w-fit text-sm text-link hover:underline" href={banner + "/audit.csv"}>Download team audit CSV</a>}
        {team.attendance.length > 0 && <ul className="m-0 grid gap-1 pl-5 text-sm">{team.attendance.map((a) => <li key={`${a.student_id}-${a.stage}`}>{team.members.find((m) => m.user_id === a.student_id)?.name} · {stageNames[a.stage]} · {a.present ? "Present" : "Absent"}</li>)}</ul>}
        {team.can_review && <form className="grid gap-2 sm:grid-cols-2" onSubmit={(e) => submit(e, (form) => action(banner + "/attendance", "PUT", {
          student_id: form.get("student"), stage: Number(form.get("stage")), present: form.get("attendance") === "present",
        }), "Attendance saved.")}>
          <Select label="Student" name="student" items={team.members.flatMap((m) => m.user_id ? [{ id: m.user_id, label: m.name }] : [])} />
          <Select label="Stage" name="stage" items={stageNames.map((label, index) => ({ id: String(index), label }))} />
          <Select label="Attendance" name="attendance" items={[{ id: "present", label: "Present" }, { id: "absent", label: "Absent" }]} />
          <button className={button}>Save attendance</button>
        </form>}
        {team.is_leader && team.idea_state === "approved" && team.project_id && <form className="grid gap-2 sm:grid-cols-2" onSubmit={(e) => submit(e, (form) => action(`${banner}/checkpoints/${form.get("stage")}`, "POST", { sha: form.get("sha") }), "Checkpoint receipt recorded.")}>
          <Select label="Checkpoint" name="stage" items={stageNames.slice(1).map((label, index) => ({ id: String(index + 1), label }))} />
          <Field label="Pushed commit SHA" name="sha" maxLength={40} />
          <button className={button}>Submit checkpoint</button>
        </form>}
        {team.checkpoints.map((item) => <div key={item.stage} className="min-w-0 rounded-lg border border-hairline p-3 text-sm">
          <strong>{stageNames[item.stage]}</strong>
          <p className="mb-1 mt-1 break-all font-mono text-xs">{item.sha}</p>
          <p className="m-0">{new Date(item.receipt_at) <= new Date(item.deadline) ? "Received on time" : "Received after deadline"} · Archive {item.capture_state}</p>
          <p className="m-0 text-muted">{new Date(item.receipt_at).toLocaleString()}</p>
          {item.archive_sha256 && <p className="mb-1 mt-1 break-all font-mono text-xs">SHA-256 {item.archive_sha256}</p>}
          {item.capture_state === "verified" && <a className="text-link hover:underline" href={`${banner}/checkpoints/${item.stage}/download`}>Download verified archive</a>}
          {team.is_leader && item.capture_state === "failed" && <form onSubmit={(e) => submit(e, () => action(`${banner}/checkpoints/${item.stage}/retry`, "POST", {}), "Capture retried for the same commit.")}><button className={button}>Retry capture</button></form>}
          {item.feedback && <p className="mb-1 mt-1 whitespace-pre-wrap break-words text-muted">{item.feedback}</p>}
          {team.can_review && <form className="mt-2 grid gap-2" onSubmit={(e) => submit(e, (form) => action(`${banner}/checkpoints/${item.stage}`, "PUT", { feedback: form.get("feedback") }), "Checkpoint feedback saved.")}>
            <Field label="Checkpoint feedback" name="feedback" maxLength={2000} defaultValue={item.feedback} required={false} /><button className={button}>Save feedback</button>
          </form>}
        </div>)}
      </Panel>
    </>}
  </div>;
}

function GitPanel({ banner, action, leader }: { banner: string; action: Action; leader: boolean }) {
  const [repo, setRepo] = useState<Repository>();
  const [candidates, setCandidates] = useState<Source[]>([]);
  const [credential, setCredential] = useState<Credential>();
  const [message, setMessage] = useState("");
  const [problem, setProblem] = useState("");
  const [pending, setPending] = useState(false);
  const [showSources, setShowSources] = useState(false);

  const refresh = async () => {
    try { setRepo(await read<Repository>(banner + "/repository")); }
    catch (reason) { setRepo(undefined); setCredential(undefined); setProblem(reason instanceof Error ? reason.message : "Repository unavailable."); }
  };
  useEffect(() => { void refresh(); }, [banner]);
  async function perform(run: () => Promise<unknown>) {
    setPending(true);
    setProblem("");
    try { await run(); }
    catch (reason) { setProblem(reason instanceof Error ? reason.message : "Repository action failed."); }
    finally { setPending(false); }
  }
  async function provision(source?: Source) {
    const result = await action<{ authorization_url?: string }>(banner + "/repository", "POST", source ? { repository_id: source.id, installation_id: source.installation_id } : {});
    if (result.authorization_url) window.location.assign(result.authorization_url);
    else { setMessage("Repository request recorded."); await refresh(); }
  }

  return <Panel title="Private PeerGit repository">
    {problem && <p role="alert" className="m-0 text-sm text-danger">{problem}</p>}
    {message && <p role="status" className="m-0 text-sm text-success">{message}</p>}
    {repo ? <p className="m-0 text-sm text-muted">Status {repo.state.replaceAll("_", " ")} · Storage limit {repo.storage_limit_kib / 1024} MiB</p> : <p className="m-0 text-sm text-muted">No repository details available.</p>}
    <div className="flex flex-wrap gap-2">
      {repo?.can_provision && ["unconfigured", "awaiting_authorization", "failed"].includes(repo.state) && <>
        {(repo.state !== "failed" || repo.mode === "empty") && <button className={button} disabled={pending} type="button" onClick={() => void perform(() => provision())}>{repo.state === "failed" ? "Retry creation" : "Create on PeerGit"}</button>}
        {(repo.state !== "failed" || repo.mode === "github") && <button className={button} disabled={pending} type="button" onClick={() => void perform(async () => {
          setCandidates((await read<{ items: Source[] }>("/api/v1/me/github/repositories")).items);
          setShowSources(true);
        })}>Copy from GitHub</button>}
      </>}
      <button className={button} disabled={pending} type="button" onClick={() => void perform(refresh)}>Refresh repository</button>
    </div>
    {showSources && <form className="grid gap-2" onSubmit={(e) => {
      e.preventDefault();
      const i = Number(new FormData(e.currentTarget).get("source"));
      if (candidates[i]) void perform(() => provision(candidates[i]));
    }}>
      <p className="m-0 text-sm text-muted">One-time copy of branches and tags. Issues, pull requests, settings, secrets, LFS objects, and submodules stay on GitHub.</p>
      {candidates.length ? <>
        <Select name="source" label="GitHub repository" items={candidates.map((item, i) => ({ id: String(i), label: item.full_name }))} />
        <button className={button} disabled={pending}>Authorize GitHub copy</button>
      </> : <a className="text-sm text-link hover:underline" href="/account/github">Refresh your GitHub connections</a>}
    </form>}
    {repo?.can_git && repo.clone_url && <>
      <label className="grid gap-1 text-sm font-medium">PeerGit clone URL<input className={field + " font-mono"} readOnly value={repo.clone_url} /></label>
      <p className="m-0 text-sm text-muted">Use this remote for PBL pushes. For a new repository, push main first, then use pull requests to update it.</p>
      <div className="flex flex-wrap gap-2">
        <button className={button} disabled={pending} type="button" onClick={() => void perform(async () => {
          setCredential(await action<Credential>(banner + "/git-credential", "POST"));
        })}>Issue Git credential</button>
        <button className={button} disabled={pending} type="button" onClick={() => void perform(async () => {
          await action(banner + "/git-credential", "DELETE"); setCredential(undefined); setMessage("Git credential revoked.");
        })}>Revoke credential</button>
      </div>
      {credential && <div className="grid gap-2 rounded-lg border border-control p-3">
        <p className="m-0 text-sm">Username <code>{credential.username}</code> · Expires {new Date(credential.expires_at).toLocaleString()}</p>
        <label className="grid gap-1 text-sm">One-time Git password<input className={field + " font-mono"} value={credential.password} readOnly /></label>
        <button className={button} type="button" onClick={() => setCredential(undefined)}>Hide password</button>
      </div>}
    </>}
    {repo?.state === "ready" && <SourceBrowser banner={banner} />}
    {repo?.state === "ready" && <RepositoryWork banner={banner} action={action} canWrite={repo.can_git} leader={leader} />}
    {repo?.state === "ready" && repo.can_provision && <div className="flex flex-wrap items-center gap-2">
      <button type="button" className={button} disabled={pending} onClick={() => void perform(async () => {
        await action(banner + "/campus-card", "PUT", { visible: !repo.campus_visible });
        await refresh();
      })}>{repo.campus_visible ? "Hide campus card" : "Share campus card"}</button>
      <p className="m-0 text-sm text-muted">The card shows the project name and summary. Coursework and source stay private.</p>
    </div>}
  </Panel>;
}

type WorkItem = {
  id: number; number: number; title: string; body?: string; state: string; merged?: boolean;
  user: { login: string }; head?: { ref: string; sha: string }; base?: { ref: string };
  assignees?: { login: string }[]; labels?: { id: number; name: string }[];
};
type WorkList = { issues: WorkItem[] | null; pulls: WorkItem[] | null; labels: { id: number; name: string }[] | null; members: Record<string, string> };
type WorkDetail = { item: WorkItem; comments: WorkItem[] | null; reviews: WorkItem[] | null };

function RepositoryWork({ banner, action, canWrite, leader }: { banner: string; action: Action; canWrite: boolean; leader: boolean }) {
  const [work, setWork] = useState<WorkList>();
  const [kind, setKind] = useState<"issues" | "pulls">("issues");
  const [detail, setDetail] = useState<WorkDetail>();
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [branches, setBranches] = useState<{ branches: Branch[]; default_branch: string }>();
  const route = banner + "/work";
  async function perform(fn: () => Promise<void>) {
    setError(""); setPending(true);
    try { await fn(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Repository work is unavailable."); }
    finally { setPending(false); }
  }
  const choose = async (number: number, selected = kind) => {
    setDetail(await read<WorkDetail>(route + "?" + new URLSearchParams({ kind: selected, number: String(number) })));
  };
  async function transact(e: FormEvent<HTMLFormElement>, type: string, item?: WorkItem) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    await perform(async () => {
      const result = await action<{ number?: number }>(route, "POST", {
        action: type, number: item?.number, sha: item?.head?.sha,
        title: form.get("title"), body: form.get("body"), head: form.get("head"), member: form.get("member"),
        decision: form.get("decision"), labels: form.getAll("label").map(Number),
      });
      setWork(await read<WorkList>(route));
      if (item || result.number) await choose(item?.number ?? result.number!);
      setNotice("Repository work saved.");
    });
  }
  const item = detail?.item;
  const shown = (login: string) => work?.members[login.replace(/^pbl-/, "")] ?? "Repository participant";
  return <section className="grid min-w-0 gap-3 border-t border-hairline pt-4" aria-label="PBL repository work">
    <h3 className="m-0 text-sm font-semibold">Issues and pull requests</h3>
    <div className="flex flex-wrap gap-2">
      {(["issues", "pulls"] as const).map((selected) => <button key={selected} type="button" className={button} disabled={pending} aria-pressed={kind === selected && Boolean(work)} onClick={() => {
        setDetail(undefined); setKind(selected); setNotice("");
        void perform(async () => {
          setWork(await read<WorkList>(route));
          if (selected === "pulls" && canWrite) setBranches(await read(banner + "/source"));
        });
      }}>{selected === "issues" ? "Issues" : "Pull requests"}</button>)}
    </div>
    {error && <p role="alert" className="m-0 text-sm text-danger">{error}</p>}
    {notice && <p role="status" className="m-0 text-sm text-success">{notice}</p>}
    {work && <>
      {(work[kind] ?? []).length === 0 && <p className="m-0 text-sm text-muted">There are no {kind === "issues" ? "issues" : "pull requests"} yet.</p>}
      {(work[kind] ?? []).map((entry) => <button key={entry.id} type="button" className={button + " justify-start whitespace-normal break-words"} disabled={pending}
        onClick={() => void perform(() => choose(entry.number))}>#{entry.number} {entry.title} · {entry.merged ? "merged" : entry.state}</button>)}
      {item && <div className="grid gap-2 rounded-lg border border-control p-3 text-sm">
        <strong className="break-words">#{item.number} {item.title}</strong>
        <p className="m-0">{shown(item.user.login)} · {item.merged ? "Merged" : item.state}</p>
        {item.head && <p className="m-0 break-all">{item.head.ref} → {item.base?.ref} · {item.head.sha}</p>}
        {item.assignees?.length ? <p className="m-0">Assigned to {item.assignees.map((assignee) => shown(assignee.login)).join(", ")}</p> : null}
        {item.labels?.length ? <p className="m-0">{item.labels.map((label) => label.name).join(", ")}</p> : null}
        {item.body && <p className="m-0 whitespace-pre-wrap break-words">{item.body}</p>}
        {[...(detail?.comments ?? []), ...(detail?.reviews ?? [])].map((comment) => <p key={comment.id + ":" + comment.state} className="m-0 whitespace-pre-wrap break-words text-muted">{shown(comment.user.login)} {comment.state ? "· " + comment.state.toLowerCase().replaceAll("_", " ") : ""}{comment.body ? " · " + comment.body : ""}</p>)}
        {canWrite && <form className="grid gap-2" onSubmit={(e) => void transact(e, "comment", item)}>
          <Field label="Comment" name="body" maxLength={8000} /><button className={button} disabled={pending}>Post comment</button>
        </form>}
        {canWrite && kind === "issues" && <form onSubmit={(e) => void transact(e, "issue_state", item)}>
          <input type="hidden" name="decision" value={item.state === "open" ? "closed" : "open"} /><button className={button} disabled={pending}>{item.state === "open" ? "Close issue" : "Reopen issue"}</button>
        </form>}
        {canWrite && item.head && item.state === "open" && <>
          <form className="grid gap-2" onSubmit={(e) => void transact(e, "review", item)}>
            <Select name="decision" label="Review decision" items={[{ id: "COMMENT", label: "Comment" }, { id: "APPROVED", label: "Approve" }, { id: "REQUEST_CHANGES", label: "Request changes" }]} />
            <Field label="Review comment" name="body" maxLength={8000} required={false} /><button className={button} disabled={pending}>Submit review</button>
          </form>
          {leader && <form onSubmit={(e) => void transact(e, "merge", item)}><button className={button} disabled={pending}>Merge approved pull request</button></form>}
        </>}
      </div>}
      {canWrite && <form className="grid gap-2 rounded-lg border border-control p-3" onSubmit={(e) => void transact(e, kind === "issues" ? "issue" : "pull")}>
        <Field label={kind === "issues" ? "Issue title" : "Pull request title"} name="title" maxLength={160} />
        <label className="grid gap-1 text-sm font-medium">Description<textarea name="body" maxLength={8000} rows={3} className={field} /></label>
        {kind === "pulls" && branches && <Select name="head" label="Source branch" items={branches.branches.filter((branch) => branch.name !== branches.default_branch).map((branch) => ({ id: branch.name, label: branch.name }))} />}
        {kind === "pulls" && branches && <p className="m-0 text-sm text-muted">Merges target {branches.default_branch}.</p>}
        <label className="grid gap-1 text-sm font-medium">{kind === "issues" ? "Assignee" : "Reviewer"}
          <select className={field} name="member" required={kind === "pulls"} defaultValue="">
            <option value="">{kind === "issues" ? "Unassigned" : "Choose a reviewer"}</option>
            {Object.entries(work.members).map(([id, name]) => <option key={id} value={id}>{name}</option>)}
          </select>
        </label>
        {kind === "issues" && work.labels?.length ? <label className="grid gap-1 text-sm font-medium">Labels<select multiple className={field} name="label">{work.labels.map((label) => <option key={label.id} value={label.id}>{label.name}</option>)}</select></label> : null}
        <button className={button} disabled={pending}>{kind === "issues" ? "Create issue" : "Open pull request"}</button>
      </form>}
      {canWrite && kind === "issues" && <form className="grid gap-2 sm:grid-cols-[1fr_auto] sm:items-end" onSubmit={(e) => void transact(e, "label")}>
        <Field label="New label" name="title" maxLength={40} /><button className={button} disabled={pending}>Create label</button>
      </form>}
    </>}
  </section>;
}

function SourceBrowser({ banner }: { banner: string }) {
  const [branches, setBranches] = useState<Branch[]>([]);
  const [tags, setTags] = useState<{ name: string; commit: { sha: string } }[]>([]);
  const [history, setHistory] = useState<{ sha: string; commit: { message: string } }[]>([]);
  const [diff, setDiff] = useState("");
  const [revision, setRevision] = useState("");
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [file, setFile] = useState<FileEntry>();
  const [pushes, setPushes] = useState<{ ref: string; oid: string; pusher: string; observed_at: string }[]>([]);
  const [commits, setCommits] = useState<{ canonical_id: string; title: string; author_name: string; verified_author: string }[]>([]);
  const [problem, setProblem] = useState("");

  async function readPath(sha: string, path = "") {
    setEntries([]); setFile(undefined); setProblem(""); setDiff("");
    try {
      const content = await read<{ entries?: FileEntry[]; file?: FileEntry }>(banner + "/source?" + new URLSearchParams({ ref: sha, path }));
      setEntries(content.entries ?? []); setFile(content.file);
    } catch (reason) { setProblem(reason instanceof Error ? reason.message : "Source unavailable."); }
  }
  return <section className="grid min-w-0 gap-2 border-t border-hairline pt-4" aria-label="PBL source and activity">
    <button type="button" className={button} onClick={() => {
      setProblem("");
      void Promise.all([read<{ branches: Branch[]; tags: typeof tags }>(banner + "/source"), read<{ commits: typeof commits; pushes: typeof pushes }>(banner + "/evidence")])
        .then(([source, evidence]) => { setBranches(source.branches ?? []); setTags(source.tags); setPushes(evidence.pushes); setCommits(evidence.commits); })
        .catch((reason: Error) => setProblem(reason.message));
    }}>Load branches and activity</button>
    {problem && <p role="alert" className="m-0 text-sm text-danger">{problem}</p>}
    {(branches.length > 0 || tags.length > 0) && <label className="grid gap-1 text-sm">Branch or tag at exact revision<select className={field} defaultValue="" onChange={(e) => {
      const [kind, name] = e.target.value.split(":", 2);
      const branch = branches.find((ref) => kind === "branch" && ref.name === name);
      const sha = branch?.commit.id ?? tags.find((ref) => kind === "tag" && ref.name === name)?.commit.sha;
      setHistory([]);
      if (sha) { setRevision(sha); void readPath(sha); }
      if (branch) void read<{ history: typeof history }>(banner + "/source?" + new URLSearchParams({ branch_history: name }))
        .then((result) => setHistory(result.history ?? [])).catch((reason: Error) => setProblem(reason.message));
    }}><option value="">Choose branch or tag</option>
      <optgroup label="Branches">{branches.map((branch) => <option key={branch.name} value={"branch:" + branch.name}>{branch.name}</option>)}</optgroup>
      <optgroup label="Tags">{tags.map((tag) => <option key={tag.name} value={"tag:" + tag.name}>{tag.name}</option>)}</optgroup>
    </select></label>}
    {revision && <div className="flex flex-wrap gap-2">
      <button type="button" className={button} onClick={() => void readPath(revision)}>Repository root</button>
      <button type="button" className={button} onClick={() => void read<{ diff: string }>(banner + "/source?" + new URLSearchParams({ view: "diff", ref: revision }))
        .then((result) => setDiff(result.diff)).catch((reason: Error) => setProblem(reason.message))}>Show commit diff</button>
    </div>}
    {history.length > 0 && <details><summary className="cursor-pointer text-sm">Recent branch commits</summary>
      <ul className="m-0 grid gap-1 pl-5 text-sm">{history.map((commit) => <li key={commit.sha} className="min-w-0">
        <button type="button" className="max-w-full break-words text-left text-link hover:underline" onClick={() => { setRevision(commit.sha); void readPath(commit.sha); }}>
          {commit.sha.slice(0, 8)} {commit.commit.message.split("\n")[0]}
        </button>
      </li>)}</ul>
    </details>}
    {diff && <pre className="m-0 max-w-full overflow-auto rounded-lg bg-raised p-3 text-xs">{diff}</pre>}
    {entries.map((entry) => <button key={entry.path} type="button" className={button + " justify-start break-all"} onClick={() => void readPath(revision, entry.path)}>{entry.name} · {entry.type}</button>)}
    {file && <pre className="m-0 overflow-auto rounded-lg bg-raised p-3 text-xs">{file.content && file.encoding === "base64" ? new TextDecoder().decode(Uint8Array.from(atob(file.content.replace(/\s/g, "")), (char) => char.charCodeAt(0))) : "This file is unavailable for preview."}</pre>}
    {pushes.length > 0 && <div><h3 className="text-sm font-semibold">Authenticated push activity</h3><ul className="grid gap-1 pl-5 text-sm">{pushes.map((p) => <li key={p.observed_at + p.ref} className="break-all">{p.pusher} pushed {p.ref} · {p.oid}</li>)}</ul></div>}
    {commits.some((commit) => commit.verified_author) && <div><h3 className="text-sm font-semibold">Verified commit authors</h3><ul className="grid gap-1 pl-5 text-sm">{commits.filter((commit) => commit.verified_author).map((commit) => <li key={commit.canonical_id}>{commit.title} · {commit.verified_author}</li>)}</ul></div>}
    {commits.some((commit) => !commit.verified_author) && <div><h3 className="text-sm font-semibold">Unattributed commits</h3><ul className="grid gap-1 pl-5 text-sm">{commits.filter((commit) => !commit.verified_author).map((commit) => <li key={commit.canonical_id}>{commit.title} · {commit.author_name || "Unknown author"} (unverified)</li>)}</ul></div>}
  </section>;
}
