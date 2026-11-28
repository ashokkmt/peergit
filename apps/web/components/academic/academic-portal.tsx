"use client";

import { FormEvent, useEffect, useState } from "react";
import { SiteFooter } from "@/components/layout/site-footer";
import { SiteHeader } from "@/components/layout/site-header";
import { readApiEnvelope } from "@/lib/http";

type Section = "faculty" | "mentorship" | "academics" | "programs" | "admin";
type Item = Record<string, unknown>;

const links = [
  ["/faculty", "Faculty"],
  ["/mentorship", "Mentorship"],
  ["/academics", "Courses & PBL"],
  ["/academics/pbl", "PBL teams"],
  ["/programs", "Programs"],
  ["/admin/academic-reviews", "Review queue"],
] as const;

const heading: Record<
  Section,
  { eyebrow: string; title: string; description: string }
> = {
  faculty: {
    eyebrow: "Faculty workspace",
    title: "Guide campus research and project work",
    description:
      "Create a course context, open a research call, and leave a reversible endorsement on work you know.",
  },
  mentorship: {
    eyebrow: "Scoped mentorship",
    title: "Make a useful connection around a project",
    description:
      "Mentor access is invitation-scoped. A conversation opens only after a project team accepts an engagement.",
  },
  academics: {
    eyebrow: "Academic contexts",
    title: "Course schedules and assignments",
    description:
      "Courses, terms, cohorts, and assignment deadlines stay inside your verified campus context.",
  },
  programs: {
    eyebrow: "Programs & research",
    title: "Run a call from application to outcome",
    description:
      "Reviewers are assigned by faculty, conflicts are checked, and accepted applications can become a program cohort.",
  },
  admin: {
    eyebrow: "Institution administration",
    title: "Review faculty and mentor access",
    description:
      "Approve access with current campus authority and revoke active scopes when an affiliation changes.",
  },
};

function rows(value: unknown): Item[] {
  if (!value || typeof value !== "object" || !("items" in value)) return [];
  const items = (value as { items?: unknown }).items;
  return Array.isArray(items) ? (items as Item[]) : [];
}

function text(item: Item, key: string): string {
  const value = item[key];
  return typeof value === "string" ? value : "";
}

function formatDate(raw: string): string {
  if (!raw) return "—";
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  return date.toLocaleDateString();
}

function formatDateTime(raw: string): string {
  if (!raw) return "—";
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  return date.toLocaleString();
}

function stateTone(raw: string): string {
  const value = raw.toLowerCase();
  if (
    ["approved", "active", "open", "accepted", "complete", "campus", "verified"].includes(value)
  ) {
    return "border-success/35 bg-success/10 text-success";
  }
  if (["pending", "requested", "revise", "suspended", "draft"].includes(value)) {
    return "border-warning/35 bg-warning/10 text-warning";
  }
  if (["rejected", "revoked", "declined", "closed", "ended"].includes(value)) {
    return "border-danger/35 bg-danger/10 text-danger";
  }
  return "border-hairline bg-raised text-muted";
}

function Button({
  children,
  variant = "primary",
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "danger";
}) {
  const style =
    variant === "primary"
      ? "border-primary bg-primary text-on-primary hover:opacity-90"
      : variant === "danger"
        ? "border-danger/40 bg-surface text-danger hover:bg-raised"
        : "border-control bg-surface text-on-surface hover:bg-raised";
  return (
    <button
      {...props}
      className={`inline-flex min-h-10 cursor-pointer items-center justify-center rounded-lg border px-3.5 py-2 text-[13px] font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-55 ${style} ${props.className ?? ""}`}
    >
      {children}
    </button>
  );
}

function Field({
  label,
  ...props
}: React.InputHTMLAttributes<HTMLInputElement> & { label: string }) {
  return (
    <label className="grid gap-1.5 text-[13px] font-medium text-on-surface">
      {label}
      <input
        {...props}
        className={`min-h-10 w-full rounded-lg border border-control bg-canvas px-3 text-sm text-on-surface placeholder:text-muted focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-link ${props.className ?? ""}`}
      />
    </label>
  );
}

function TextField({
  label,
  ...props
}: React.TextareaHTMLAttributes<HTMLTextAreaElement> & { label: string }) {
  return (
    <label className="grid gap-1.5 text-[13px] font-medium text-on-surface">
      {label}
      <textarea
        {...props}
        className={`min-h-24 w-full resize-y rounded-lg border border-control bg-canvas px-3 py-2 text-sm text-on-surface placeholder:text-muted focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-link ${props.className ?? ""}`}
      />
    </label>
  );
}

function SelectField({
  label,
  name,
  options,
  required = true,
  defaultValue = "",
}: {
  label: string;
  name: string;
  options: { value: string; label: string }[];
  required?: boolean;
  defaultValue?: string;
}) {
  return (
    <label className="grid gap-1.5 text-[13px] font-medium text-on-surface">
      {label}
      <select
        name={name}
        className="min-h-10 w-full rounded-lg border border-control bg-canvas px-3 text-sm text-on-surface focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-link"
        required={required}
        defaultValue={defaultValue}
      >
        {!defaultValue && (
          <option value="" disabled>
            Choose an option
          </option>
        )}
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

function Panel({
  title,
  description,
  children,
  className = "",
}: {
  title: string;
  description?: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <section
      className={`grid min-w-0 max-w-full gap-4 rounded-2xl border border-hairline bg-surface p-5 md:p-6 ${className}`}
    >
      <div>
        <h2 className="m-0 text-lg font-semibold leading-6">{title}</h2>
        {description && (
          <p className="mb-0 mt-1 text-[13px] leading-5 text-muted">{description}</p>
        )}
      </div>
      {children}
    </section>
  );
}

export function AcademicPortal({ section }: { section: Section }) {
  const [data, setData] = useState<Record<string, unknown>>({});
  const [status, setStatus] = useState("Loading your campus workspace…");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [exportID, setExportID] = useState("");
  const [exportType, setExportType] = useState<"call" | "cohort">("call");
  const [userID, setUserID] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);

  async function get(path: string) {
    const response = await fetch(`/api/v1${path}`, { cache: "no-store" });
    const envelope = await readApiEnvelope<unknown>(response, "Campus data could not be loaded.");
    if (!response.ok) {
      throw new Error(envelope.error?.message ?? "Campus data could not be loaded.");
    }
    return envelope.data;
  }

  async function load() {
    setStatus("Loading your campus workspace…");
    setError("");
    try {
      const endpoints: Record<Section, string[]> = {
        faculty: [
          "/me/faculty-verification-requests",
          "/academic/courses",
          "/academic/terms",
          "/academic/cohorts",
          "/academic/calls",
          "/academic/faculty",
          "/academic/projects",
          "/academic/digest",
        ],
        mentorship: [
          "/me/mentor-profile",
          "/mentors",
          "/me/mentorship-requests",
          "/projects",
        ],
        academics: [
          "/academic/terms",
          "/academic/courses",
          "/academic/cohorts",
          "/academic/assignments",
          "/academic/faculty",
          "/academic/projects",
        ],
        programs: [
          "/academic/calls",
          "/academic/program-cohorts",
          "/academic/faculty",
          "/academic/projects",
        ],
        admin: [
          "/admin/faculty-verification-requests",
          "/admin/faculty-profiles",
          "/admin/mentor-profiles",
        ],
      };
      const [session, ...results] = await Promise.allSettled([
        get("/session"),
        ...endpoints[section].map((path) => get(path)),
      ]);
      if (session.status === "fulfilled" && session.value && typeof session.value === "object") {
        const user = (session.value as { user?: { id?: string; campus_admin?: boolean } }).user;
        setUserID(user?.id ?? "");
        setIsAdmin(user?.campus_admin === true);
      }
      const next: Record<string, unknown> = {};
      results.forEach((result, index) => {
        if (result.status === "fulfilled") next[endpoints[section][index]] = result.value;
      });
      setData(next);
      const failure = results.find((result) => result.status === "rejected");
      if (failure?.status === "rejected") {
        setError(
          failure.reason instanceof Error
            ? failure.reason.message
            : "Some campus data could not be loaded.",
        );
      }
      setStatus("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Campus data could not be loaded.");
      setStatus("");
    }
  }

  useEffect(() => {
    void load();
  }, [section]);

  async function submit(event: FormEvent<HTMLFormElement>, path: string, method = "POST") {
    event.preventDefault();
    setBusy(true);
    setError("");
    setNotice("");
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const body: Record<string, string | string[]> = Object.fromEntries(form.entries()) as Record<
      string,
      string
    >;
    const resolvedPath = path
      .replace("PROJECT_ID", encodeURIComponent(String(body.project_id ?? "")))
      .replace("ASSIGNMENT_ID", encodeURIComponent(String(body.assignment_id ?? "")))
      .replace("CALL_ID", encodeURIComponent(String(body.call_id ?? "")))
      .replace("COURSE_ID", encodeURIComponent(String(body.course_id ?? "")))
      .replace("COHORT_ID", encodeURIComponent(String(body.cohort_id ?? "")))
      .replace("APPLICATION_ID", encodeURIComponent(String(body.application_id ?? "")));
    if (typeof body.expertise === "string") {
      body.expertise = body.expertise
        .split(",")
        .map((value) => value.trim())
        .filter(Boolean);
    }
    for (const key of ["due_at", "closes_at"]) {
      const value = body[key];
      if (typeof value === "string" && value && !/[zZ]|[+-]\d\d:\d\d$/.test(value)) {
        body[key] = new Date(value).toISOString();
      }
    }
    try {
      const sessionResponse = await fetch("/api/v1/session", { cache: "no-store" });
      const session = await readApiEnvelope<{ csrf_token?: string }>(
        sessionResponse,
        "Session could not be verified.",
      );
      const response = await fetch(`/api/v1${resolvedPath}`, {
        method,
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": session.data?.csrf_token ?? "",
        },
        body: JSON.stringify(body),
      });
      const envelope = await readApiEnvelope<{ id?: string }>(
        response,
        "The requested change could not be saved.",
      );
      if (!response.ok) {
        throw new Error(envelope.error?.message ?? "The requested change could not be saved.");
      }
      if (path === "/academic/exports" && envelope.data?.id) {
        setExportID(envelope.data.id);
      }
      setNotice("Saved. Current access and context will be checked again on every request.");
      formElement.reset();
      await load();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "The requested change could not be saved.",
      );
    } finally {
      setBusy(false);
    }
  }

  async function action(path: string, body: Record<string, string>, method = "POST") {
    setBusy(true);
    setError("");
    try {
      const sessionResponse = await fetch("/api/v1/session", { cache: "no-store" });
      const session = await readApiEnvelope<{ csrf_token?: string }>(
        sessionResponse,
        "Session could not be verified.",
      );
      const response = await fetch(`/api/v1${path}`, {
        method,
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": session.data?.csrf_token ?? "",
        },
        body: JSON.stringify(body),
      });
      const envelope = await readApiEnvelope<unknown>(
        response,
        "The requested action could not be completed.",
      );
      if (!response.ok) {
        throw new Error(
          envelope.error?.message ?? "The requested action could not be completed.",
        );
      }
      setNotice("The action is complete.");
      await load();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "The requested action could not be completed.",
      );
    } finally {
      setBusy(false);
    }
  }

  const copy = heading[section];
  const projects = rows(data["/academic/projects"] ?? data["/projects"]);
  const courses = rows(data["/academic/courses"]);
  const terms = rows(data["/academic/terms"]);
  const cohorts = rows(data["/academic/cohorts"]);
  const facultyDirectory = rows(data["/academic/faculty"]);
  const programCohorts = rows(data["/academic/program-cohorts"]);
  const assignments = rows(data["/academic/assignments"]);
  const calls = rows(data["/academic/calls"]);
  const mentors = rows(data["/mentors"]);
  const requests = rows(data["/me/mentorship-requests"]);
  const facultyRequests = rows(data["/me/faculty-verification-requests"]);
  const facultyQueue = rows(data["/admin/faculty-verification-requests"]);
  const facultyProfiles = rows(data["/admin/faculty-profiles"]);
  const mentorQueue = rows(data["/admin/mentor-profiles"]);
  const digest = data["/academic/digest"] as
    | {
        cadence?: string;
        summary?: {
          assignments_due?: number;
          reviews_pending?: number;
          mentorship_requests?: number;
        };
      }
    | undefined;
  const canTeach =
    isAdmin || facultyDirectory.some((faculty) => text(faculty, "id") === userID);

  return (
    <>
      <a
        href="#main-content"
        className="fixed -top-20 left-4 z-50 rounded-lg bg-primary px-4 py-2.5 font-semibold text-on-primary focus:top-3"
      >
        Skip to content
      </a>
      <SiteHeader workspace />
      <main
        id="main-content"
        className="mx-auto flex w-[min(1240px,calc(100%-32px))] flex-1 flex-col gap-6 py-8 md:w-[min(1240px,calc(100%-64px))] md:py-10"
      >
        <header className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <p className="mb-1.5 mt-0 text-xs font-semibold tracking-[0.02em] text-link">
              {copy.eyebrow}
            </p>
            <h1 className="m-0 max-w-3xl font-display text-[clamp(1.5rem,2.6vw,2.15rem)] font-semibold leading-[1.18] tracking-[-0.02em]">
              {copy.title}
            </h1>
            <p className="mb-0 mt-1.5 max-w-[65ch] text-sm leading-[21px] text-muted">
              {copy.description}
            </p>
          </div>
          <Button variant="secondary" disabled={busy} onClick={() => void load()} type="button">
            Refresh
          </Button>
        </header>

        <nav
          className="flex items-center gap-1 overflow-x-auto border-b border-hairline [-webkit-overflow-scrolling:touch]"
          aria-label="Faculty and academic sections"
        >
          {links.map(([href, label]) => (
            <a
              key={href}
              href={href}
              aria-current={
                href === (section === "admin" ? "/admin/academic-reviews" : `/${section}`)
                  ? "page"
                  : undefined
              }
              className="whitespace-nowrap border-b-2 border-transparent px-3.5 py-2.5 text-[13px] font-medium text-muted no-underline hover:text-on-surface aria-[current=page]:border-b-link aria-[current=page]:font-semibold aria-[current=page]:text-on-surface"
            >
              {label}
            </a>
          ))}
        </nav>

        {error && (
          <div
            role="alert"
            className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-danger/35 bg-danger/10 p-4 text-sm font-medium text-danger"
          >
            <span>{error}</span>
            <Button variant="secondary" onClick={() => void load()} type="button">
              Retry
            </Button>
          </div>
        )}

        {notice && (
          <p
            role="status"
            className="m-0 rounded-xl border border-success/35 bg-success/10 p-4 text-sm font-medium text-success"
          >
            {notice}
          </p>
        )}

        {status && (
          <p
            role="status"
            className="m-0 rounded-xl border border-hairline bg-raised px-4 py-3 text-sm text-muted"
          >
            {status}
          </p>
        )}

        {section === "faculty" && (
          <div className="grid items-start gap-5 lg:grid-cols-2">
            <div className="grid gap-5">
              <Panel
                title="Faculty affiliation"
                description="Campus administrators verify faculty access. Decisions are recorded against your campus account."
              >
                <form
                  className="grid gap-3"
                  onSubmit={(event) => void submit(event, "/me/faculty-verification-requests")}
                >
                  <Field
                    name="title"
                    label="Academic title"
                    placeholder="Assistant Professor, Computer Science"
                    required
                    maxLength={120}
                  />
                  <Field
                    name="department_id"
                    label="Department organization ID"
                    placeholder="Optional department UUID"
                  />
                  <TextField
                    name="affiliation_note"
                    label="Affiliation details"
                    placeholder="Describe your role and institutional affiliation."
                    required
                    minLength={8}
                    maxLength={2000}
                  />
                  <div>
                    <Button disabled={busy}>Request verification</Button>
                  </div>
                </form>
                <ItemList
                  items={facultyRequests}
                  empty="No affiliation request yet."
                  render={(item) => (
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <strong>{text(item, "title")}</strong>
                      <State value={text(item, "state")} />
                    </div>
                  )}
                />
              </Panel>

              <Panel
                title="Endorse supervised project work"
                description="Endorse a project you supervise through a linked assignment, or a project you are a member of. Endorsements are reversible."
              >
                <form
                  className="grid gap-3"
                  onSubmit={(event) =>
                    void submit(event, "/academic/projects/PROJECT_ID/endorsements")
                  }
                >
                  <SelectField
                    name="project_id"
                    label="Project"
                    options={projects.map((project) => ({
                      value: text(project, "id"),
                      label: text(project, "title") || text(project, "name"),
                    }))}
                  />
                  <div className="grid gap-3 sm:grid-cols-2">
                    <SelectField
                      name="course_id"
                      label="Optional course context"
                      required={false}
                      options={courses.map((course) => ({
                        value: text(course, "id"),
                        label: `${text(course, "code")} · ${text(course, "title")}`,
                      }))}
                    />
                    <SelectField
                      name="cohort_id"
                      label="Optional cohort context"
                      required={false}
                      options={cohorts.map((cohort) => ({
                        value: text(cohort, "id"),
                        label: `${text(cohort, "course_code")} · ${text(cohort, "label")}`,
                      }))}
                    />
                  </div>
                  <SelectField
                    name="audience"
                    label="Audience"
                    defaultValue="campus"
                    options={[
                      { value: "campus", label: "Campus members" },
                      { value: "private", label: "Private to project & faculty" },
                    ]}
                  />
                  <TextField
                    name="statement"
                    label="Endorsement statement"
                    placeholder="Describe verified technical outcomes or leadership observed in this project."
                    minLength={10}
                    maxLength={2000}
                    required
                  />
                  <div>
                    <Button disabled={busy}>Add endorsement</Button>
                  </div>
                </form>
              </Panel>

              <Panel
                title="Academic digest"
                description="A seven-day descriptive summary of work due, assigned reviews, and requests. Message text is never included."
              >
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                  <div className="rounded-xl border border-hairline bg-canvas p-3.5">
                    <span className="block text-xs font-medium text-muted">Assignments due</span>
                    <strong className="mt-1 block font-display text-2xl font-semibold tabular-nums">
                      {digest?.summary?.assignments_due ?? 0}
                    </strong>
                  </div>
                  <div className="rounded-xl border border-hairline bg-canvas p-3.5">
                    <span className="block text-xs font-medium text-muted">Reviews pending</span>
                    <strong className="mt-1 block font-display text-2xl font-semibold tabular-nums">
                      {digest?.summary?.reviews_pending ?? 0}
                    </strong>
                  </div>
                  <div className="rounded-xl border border-hairline bg-canvas p-3.5">
                    <span className="block text-xs font-medium text-muted">Mentorship requests</span>
                    <strong className="mt-1 block font-display text-2xl font-semibold tabular-nums">
                      {digest?.summary?.mentorship_requests ?? 0}
                    </strong>
                  </div>
                </div>
                <form
                  className="grid gap-3 border-t border-hairline pt-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end"
                  onSubmit={(event) => void submit(event, "/me/academic-digest-preferences", "PUT")}
                >
                  <SelectField
                    name="cadence"
                    label="Digest frequency"
                    defaultValue={digest?.cadence === "off" ? "off" : "weekly"}
                    options={[
                      { value: "weekly", label: "Weekly summary" },
                      { value: "off", label: "Turn summaries off" },
                    ]}
                  />
                  <Button variant="secondary" disabled={busy}>
                    Save digest preference
                  </Button>
                </form>
              </Panel>
            </div>

            {canTeach && (
              <div className="grid gap-5">
                <Panel
                  title="Courses, terms & cohorts"
                  description="Verified faculty can define courses and academic terms, then group enrolled students into cohorts."
                >
                  <form
                    className="grid gap-3"
                    onSubmit={(event) => void submit(event, "/academic/courses")}
                  >
                    <div className="grid gap-3 sm:grid-cols-[140px_minmax(0,1fr)]">
                      <Field
                        name="code"
                        label="Course code"
                        placeholder="CS-340"
                        required
                        maxLength={40}
                      />
                      <Field
                        name="title"
                        label="Course title"
                        placeholder="Software Engineering Studio"
                        required
                        maxLength={160}
                      />
                    </div>
                    <TextField
                      name="description"
                      label="Description"
                      placeholder="What students will build and learn."
                      maxLength={4000}
                    />
                    <div>
                      <Button disabled={busy}>Create course</Button>
                    </div>
                  </form>
                  <ItemList
                    items={courses}
                    empty="No courses are visible in this scope."
                    render={(item) => (
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <span>
                          <strong className="font-mono">{text(item, "code")}</strong> ·{" "}
                          {text(item, "title")}
                        </span>
                      </div>
                    )}
                  />

                  <form
                    className="grid gap-3 border-t border-hairline pt-4"
                    onSubmit={(event) => void submit(event, "/academic/terms")}
                  >
                    <Field
                      name="label"
                      label="Term label"
                      placeholder="Fall 2026"
                      required
                      maxLength={100}
                    />
                    <div className="grid gap-3 sm:grid-cols-2">
                      <Field name="starts_at" label="Start date" type="date" required />
                      <Field name="ends_at" label="End date" type="date" required />
                    </div>
                    <div>
                      <Button variant="secondary" disabled={busy}>
                        Create term
                      </Button>
                    </div>
                  </form>
                  <ItemList
                    items={terms}
                    empty="No terms created yet."
                    render={(item) => (
                      <span>
                        <strong>{text(item, "label")}</strong> · starts{" "}
                        {formatDate(text(item, "starts_at"))}
                      </span>
                    )}
                  />

                  <form
                    className="grid gap-3 border-t border-hairline pt-4"
                    onSubmit={(event) => void submit(event, "/academic/courses/COURSE_ID/cohorts")}
                  >
                    <div className="grid gap-3 sm:grid-cols-2">
                      <SelectField
                        name="course_id"
                        label="Course"
                        options={courses.map((course) => ({
                          value: text(course, "id"),
                          label: `${text(course, "code")} · ${text(course, "title")}`,
                        }))}
                      />
                      <SelectField
                        name="term_id"
                        label="Term"
                        options={terms.map((term) => ({
                          value: text(term, "id"),
                          label: `${text(term, "label")} · ${formatDate(text(term, "starts_at"))}`,
                        }))}
                      />
                    </div>
                    <Field
                      name="label"
                      label="Cohort label"
                      placeholder="CS-340 / Section A"
                      required
                      maxLength={120}
                    />
                    <div>
                      <Button variant="secondary" disabled={busy}>
                        Create cohort
                      </Button>
                    </div>
                  </form>
                  <ItemList
                    items={cohorts}
                    empty="No course cohorts are available yet."
                    render={(cohort) => (
                      <span>
                        <strong className="font-mono">{text(cohort, "course_code")}</strong> ·{" "}
                        {text(cohort, "label")} · {text(cohort, "term")}
                      </span>
                    )}
                  />
                </Panel>

                <Panel
                  title="Publish research or program call"
                  description="Open a campus call for teams to apply with an existing PeerGit project."
                >
                  <form
                    className="grid gap-3"
                    onSubmit={(event) => void submit(event, "/academic/calls")}
                  >
                    <div className="grid gap-3 sm:grid-cols-2">
                      <SelectField
                        name="kind"
                        label="Opening type"
                        options={[
                          { value: "research", label: "Research opening" },
                          { value: "program", label: "Campus program" },
                        ]}
                      />
                      <SelectField
                        name="status"
                        label="Initial visibility"
                        options={[
                          { value: "open", label: "Open for applications" },
                          { value: "draft", label: "Save as draft" },
                        ]}
                      />
                    </div>
                    <Field
                      name="title"
                      label="Opening title"
                      placeholder="Campus sustainability research group"
                      required
                      maxLength={160}
                    />
                    <TextField
                      name="description"
                      label="Description"
                      placeholder="Describe the opportunity, expected work, and context."
                      required
                      maxLength={8000}
                    />
                    <Field
                      name="eligibility"
                      label="Eligibility"
                      placeholder="Open to verified students in any department"
                      maxLength={4000}
                    />
                    <div>
                      <Button disabled={busy}>Publish opening</Button>
                    </div>
                  </form>
                </Panel>
              </div>
            )}
          </div>
        )}

        {section === "mentorship" && (
          <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
            <div className="grid gap-5">
              <Panel
                title="Mentor profile"
                description="External mentors use the college scope from their invitation; they do not become campus members."
              >
                <form
                  className="grid gap-3"
                  onSubmit={(event) => void submit(event, "/me/mentor-profile", "PUT")}
                >
                  <TextField
                    name="bio"
                    label="Mentoring approach"
                    placeholder="How you can help project teams."
                    maxLength={2000}
                  />
                  <Field
                    name="expertise"
                    label="Expertise (comma-separated)"
                    placeholder="web, embedded systems, research methods"
                  />
                  <Field
                    name="capacity"
                    label="Active engagement capacity (1–10)"
                    type="number"
                    min={1}
                    max={10}
                    defaultValue={1}
                    required
                  />
                  <div>
                    <Button disabled={busy}>Save mentor profile</Button>
                  </div>
                </form>
              </Panel>

              <Panel
                title="Your invitation-scoped project list"
                description="Private project source stays with its members; mentors receive only the accepted engagement context."
              >
                <ItemList
                  items={projects}
                  empty="No projects are available in your account."
                  render={(project) => (
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <a
                        className="font-semibold text-on-surface no-underline hover:text-link hover:underline"
                        href={`/projects/${encodeURIComponent(text(project, "id"))}`}
                      >
                        {text(project, "title") || text(project, "name")}
                      </a>
                      <span className="font-mono text-xs text-muted">{text(project, "id")}</span>
                    </div>
                  )}
                />
              </Panel>
            </div>

            <div className="grid gap-5">
              <Panel
                title="Mentorship requests"
                description="Accept or decline incoming requests, open active mentorship chats, or close completed engagements."
              >
                <ItemList
                  items={requests}
                  empty="No mentorship requests in this tenant scope."
                  render={(item) => (
                    <MentorshipRequest item={item} busy={busy} onAction={action} />
                  )}
                />
              </Panel>

              <Panel
                title="Verified mentors"
                description="Requests are tied to a project you are a member of. Accepting opens a scoped conversation until ended or revoked."
              >
                <ItemList
                  items={mentors}
                  empty="No verified mentors are listed for this campus yet."
                  render={(mentor) => (
                    <MentorCard
                      mentor={mentor}
                      projects={projects}
                      busy={busy}
                      onSubmit={submit}
                    />
                  )}
                />
              </Panel>
            </div>
          </div>
        )}

        {section === "academics" && (
          <div className="grid items-start gap-5 lg:grid-cols-2">
            <div className="grid gap-5">
              <Panel
                title="Terms and courses"
                description="Visible only to assigned course staff, enrolled students, and authorized campus administrators."
              >
                {canTeach && (
                  <form
                    className="grid gap-3 border-b border-hairline pb-4"
                    onSubmit={(event) => void submit(event, "/academic/terms")}
                  >
                    <Field
                      name="label"
                      label="Term label"
                      placeholder="Fall 2026"
                      required
                      maxLength={100}
                    />
                    <div className="grid gap-3 sm:grid-cols-2">
                      <Field name="starts_at" label="Start date" type="date" required />
                      <Field name="ends_at" label="End date" type="date" required />
                    </div>
                    <div>
                      <Button disabled={busy}>Create term</Button>
                    </div>
                  </form>
                )}
                <div className="grid gap-2">
                  <h3 className="m-0 text-sm font-semibold">Terms</h3>
                  <ItemList
                    items={terms}
                    empty="No term context is available."
                    render={(term) => (
                      <span>
                        <strong>{text(term, "label")}</strong> · starts{" "}
                        {formatDate(text(term, "starts_at"))}
                      </span>
                    )}
                  />
                </div>
                <div className="grid gap-2">
                  <h3 className="m-0 text-sm font-semibold">Courses</h3>
                  <ItemList
                    items={courses}
                    empty="No course context is available."
                    render={(course) => (
                      <span>
                        <strong className="font-mono">{text(course, "code")}</strong> ·{" "}
                        {text(course, "title")}
                      </span>
                    )}
                  />
                </div>
              </Panel>


            </div>

            <Panel
              title="Course assignments"
              description="Review assignment briefs and deadlines. Repository snapshots and assessment receipts are reserved for PBL checkpoints."
            >
              {canTeach && (
                <form
                  className="grid gap-3 border-b border-hairline pb-4"
                  onSubmit={(event) =>
                    void submit(event, "/academic/cohorts/COHORT_ID/assignments")
                  }
                >
                  <SelectField
                    name="cohort_id"
                    label="Course cohort"
                    options={cohorts.map((cohort) => ({
                      value: text(cohort, "id"),
                      label: `${text(cohort, "course_code")} · ${text(cohort, "label")}`,
                    }))}
                  />
                  <Field
                    name="title"
                    label="Assignment title"
                    placeholder="Project checkpoint 1"
                    required
                    maxLength={160}
                  />
                  <TextField
                    name="brief"
                    label="Assignment brief"
                    placeholder="Deliverable and evaluation context."
                    required
                    maxLength={8000}
                  />
                  <div className="grid gap-3 sm:grid-cols-2">
                    <Field
                      name="due_at"
                      label="Due at"
                      type="datetime-local"
                      required
                    />
                  </div>
                  <div>
                    <Button disabled={busy}>Create assignment</Button>
                  </div>
                </form>
              )}

              <ItemList
                items={assignments}
                empty="No assignments are available."
                render={(assignment) => (
                  <div className="grid gap-3">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div>
                        <strong className="text-sm">{text(assignment, "title")}</strong>
                        <span className="mt-0.5 block text-xs text-muted">
                          <span className="font-mono">{text(assignment, "course_code")}</span> · due{" "}
                          {formatDateTime(text(assignment, "due_at"))}
                        </span>
                      </div>
                    </div>
                  </div>
                )}
              />
            </Panel>
          </div>
        )}

        {section === "programs" && (
          <div className="grid items-start gap-5 lg:grid-cols-2">
            <Panel
              title="Research and program calls"
              description="Applications use a project context and remain visible only within the owning institution."
            >
              {canTeach && (
                <form
                  className="grid gap-3 border-b border-hairline pb-4"
                  onSubmit={(event) => void submit(event, "/academic/calls")}
                >
                  <div className="grid gap-3 sm:grid-cols-2">
                    <SelectField
                      name="kind"
                      label="Call type"
                      options={[
                        { value: "research", label: "Research opening" },
                        { value: "program", label: "Campus program" },
                      ]}
                    />
                    <SelectField
                      name="status"
                      label="Initial visibility"
                      options={[
                        { value: "open", label: "Open for applications" },
                        { value: "draft", label: "Save as draft" },
                      ]}
                    />
                  </div>
                  <Field
                    name="title"
                    label="Title"
                    placeholder="PeerGit campus innovation cohort"
                    required
                    maxLength={160}
                  />
                  <TextField
                    name="description"
                    label="Description"
                    required
                    maxLength={8000}
                  />
                  <div className="grid gap-3 sm:grid-cols-2">
                    <Field
                      name="capacity"
                      label="Optional acceptance capacity"
                      type="number"
                      min={1}
                      max={500}
                    />
                    <Field
                      name="closes_at"
                      label="Optional closing time"
                      type="datetime-local"
                    />
                  </div>
                  <div>
                    <Button disabled={busy}>Create call</Button>
                  </div>
                </form>
              )}
              <ItemList
                items={calls}
                empty="No calls are open in this campus."
                render={(call) => (
                  <CallCard
                    call={call}
                    projects={projects}
                    busy={busy}
                    canReview={call.can_review === true}
                    onSubmit={submit}
                  />
                )}
              />
            </Panel>

            <Panel
              title="Review and grow a cohort"
              description="Assign verified faculty reviewers; conflicts with the applicant or project team prevent a review."
            >
              {canTeach && (
                <>
                  <form
                    className="grid gap-3"
                    onSubmit={(event) => void submit(event, "/academic/calls/CALL_ID/reviewers")}
                  >
                    <div className="grid gap-3 sm:grid-cols-2">
                      <SelectField
                        name="call_id"
                        label="Program call"
                        options={calls.map((call) => ({
                          value: text(call, "id"),
                          label: text(call, "title"),
                        }))}
                      />
                      <SelectField
                        name="user_id"
                        label="Verified faculty reviewer"
                        options={facultyDirectory
                          .filter((faculty) => text(faculty, "id") !== userID)
                          .map((faculty) => ({
                            value: text(faculty, "id"),
                            label: `${text(faculty, "name")} · ${text(faculty, "title")}`,
                          }))}
                      />
                    </div>
                    <div>
                      <Button variant="secondary" disabled={busy}>
                        Assign reviewer
                      </Button>
                    </div>
                  </form>

                  <form
                    className="grid gap-3 border-t border-hairline pt-4"
                    onSubmit={(event) => void submit(event, "/academic/calls/CALL_ID/cohorts")}
                  >
                    <div className="grid gap-3 sm:grid-cols-2">
                      <SelectField
                        name="call_id"
                        label="Program call"
                        options={calls
                          .filter((call) => text(call, "kind") === "program")
                          .map((call) => ({
                            value: text(call, "id"),
                            label: text(call, "title"),
                          }))}
                      />
                      <Field
                        name="label"
                        label="Cohort name"
                        placeholder="2026 Innovation Cohort"
                        required
                      />
                    </div>
                    <div>
                      <Button disabled={busy}>Create accepted-project cohort</Button>
                    </div>
                  </form>
                </>
              )}

              <ItemList
                items={programCohorts}
                empty="No program cohorts are available in your review scope."
                render={(cohort) => (
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span>
                      <strong>{text(cohort, "label")}</strong> · {text(cohort, "call")}
                    </span>
                    <State value={`${String(cohort.project_count ?? 0)} projects`} />
                  </div>
                )}
              />

              {canTeach && (
                <>
                  <form
                    className="grid gap-3 border-t border-hairline pt-4"
                    onSubmit={(event) => void submit(event, "/academic/exports")}
                  >
                    <label className="grid gap-1.5 text-[13px] font-medium text-on-surface">
                      Export scope
                      <select
                        name="type"
                        value={exportType}
                        onChange={(event) =>
                          setExportType(event.target.value as "call" | "cohort")
                        }
                        className="min-h-10 w-full rounded-lg border border-control bg-canvas px-3 text-sm text-on-surface focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-link"
                      >
                        <option value="call">Research or program call</option>
                        <option value="cohort">Program cohort</option>
                      </select>
                    </label>
                    <div className="grid gap-3 sm:grid-cols-2">
                      {exportType === "call" ? (
                        <SelectField
                          name="call_id"
                          label="Call"
                          options={calls.map((call) => ({
                            value: text(call, "id"),
                            label: text(call, "title"),
                          }))}
                        />
                      ) : (
                        <SelectField
                          name="cohort_id"
                          label="Program cohort"
                          options={programCohorts.map((cohort) => ({
                            value: text(cohort, "id"),
                            label: text(cohort, "label"),
                          }))}
                        />
                      )}
                      <Field name="idempotency_key" label="Export request key" required />
                    </div>
                    <div>
                      <Button variant="secondary" disabled={busy}>
                        Generate scoped CSV
                      </Button>
                    </div>
                  </form>

                  {exportID && (
                    <a
                      className="inline-flex min-h-10 w-fit items-center rounded-lg border border-primary bg-primary px-4 py-2 text-[13px] font-semibold text-on-primary no-underline hover:opacity-90"
                      href={`/api/v1/academic/exports/${exportID}`}
                    >
                      Download generated CSV →
                    </a>
                  )}

                  {programCohorts.length > 0 && (
                    <>
                      <form
                        className="grid gap-3 border-t border-hairline pt-4"
                        onSubmit={(event) =>
                          void submit(event, "/academic/cohorts/COHORT_ID/milestones")
                        }
                      >
                        <div className="grid gap-3 sm:grid-cols-2">
                          <SelectField
                            name="cohort_id"
                            label="Program cohort"
                            options={programCohorts.map((cohort) => ({
                              value: text(cohort, "id"),
                              label: text(cohort, "label"),
                            }))}
                          />
                          <Field
                            name="title"
                            label="Milestone title"
                            placeholder="Midpoint review"
                            required
                          />
                        </div>
                        <TextField
                          name="description"
                          label="Milestone details"
                          maxLength={4000}
                        />
                        <Field
                          name="due_at"
                          label="Optional due date and time"
                          type="datetime-local"
                        />
                        <div>
                          <Button variant="secondary" disabled={busy}>
                            Add milestone
                          </Button>
                        </div>
                      </form>

                      <form
                        className="grid gap-3 border-t border-hairline pt-4"
                        onSubmit={(event) =>
                          void submit(event, "/academic/cohorts/COHORT_ID/outcomes")
                        }
                      >
                        <div className="grid gap-3 sm:grid-cols-2">
                          <SelectField
                            name="cohort_id"
                            label="Program cohort"
                            options={programCohorts.map((cohort) => ({
                              value: text(cohort, "id"),
                              label: text(cohort, "label"),
                            }))}
                          />
                          <Field
                            name="project_id"
                            label="Accepted project ID"
                            placeholder="Project UUID"
                            required
                          />
                        </div>
                        <TextField
                          name="summary"
                          label="Outcome summary"
                          required
                          maxLength={4000}
                        />
                        <div className="grid gap-3 sm:grid-cols-2">
                          <Field name="result" label="Result" maxLength={1000} />
                        </div>
                        <div>
                          <Button variant="secondary" disabled={busy}>
                            Save program outcome
                          </Button>
                        </div>
                      </form>
                    </>
                  )}
                </>
              )}
            </Panel>
          </div>
        )}

        {section === "admin" && (
          <div className="grid items-start gap-5 lg:grid-cols-2">
            <Panel
              title="Faculty verification queue"
              description="Verify institutional affiliation before granting faculty controls."
            >
              <ItemList
                items={facultyQueue}
                empty="No faculty requests need review."
                render={(item) => (
                  <article className="grid gap-2">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <strong>
                        {text(item, "name")} · {text(item, "title")}
                      </strong>
                      <State value={text(item, "state") || "pending"} />
                    </div>
                    <span className="text-muted">
                      {text(item, "department")} · {text(item, "email")}
                    </span>
                    <p className="m-0">{text(item, "affiliation_note")}</p>
                    <div className="flex flex-wrap gap-2 pt-1">
                      <Button
                        disabled={busy}
                        onClick={() =>
                          void action(
                            `/admin/faculty-verification-requests/${text(item, "id")}/decision`,
                            {
                              decision: "approved",
                              reason: "Affiliation verified by campus administrator.",
                            },
                          )
                        }
                      >
                        Approve
                      </Button>
                      <Button
                        variant="secondary"
                        disabled={busy}
                        onClick={() =>
                          void action(
                            `/admin/faculty-verification-requests/${text(item, "id")}/decision`,
                            {
                              decision: "rejected",
                              reason:
                                "Please provide current institutional affiliation evidence.",
                            },
                          )
                        }
                      >
                        Reject
                      </Button>
                    </div>
                  </article>
                )}
              />
            </Panel>

            <Panel
              title="Faculty access"
              description="Suspend faculty access temporarily or revoke it when an affiliation ends. Restoring access records a new campus role grant."
            >
              <ItemList
                items={facultyProfiles}
                empty="No faculty profiles are registered for this campus."
                render={(item) => (
                  <article className="grid gap-2">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <strong>
                        {text(item, "name")} · {text(item, "title")}
                      </strong>
                      <State value={text(item, "status")} />
                    </div>
                    <div className="flex flex-wrap gap-2 pt-1">
                      {text(item, "status") !== "suspended" && (
                        <Button
                          variant="secondary"
                          disabled={busy}
                          onClick={() =>
                            void action(`/admin/faculty/${text(item, "user_id")}/status`, {
                              status: "suspended",
                            })
                          }
                        >
                          Suspend
                        </Button>
                      )}
                      {text(item, "status") !== "active" && (
                        <Button
                          disabled={busy}
                          onClick={() =>
                            void action(`/admin/faculty/${text(item, "user_id")}/status`, {
                              status: "active",
                            })
                          }
                        >
                          Restore access
                        </Button>
                      )}
                      {text(item, "status") !== "revoked" && (
                        <Button
                          variant="danger"
                          disabled={busy}
                          onClick={() =>
                            void action(`/admin/faculty/${text(item, "user_id")}/status`, {
                              status: "revoked",
                            })
                          }
                        >
                          Revoke
                        </Button>
                      )}
                    </div>
                  </article>
                )}
              />
            </Panel>

            <Panel
              title="Mentor access review"
              description="External invitations grant only mentor access for this campus. Revoking a profile closes active mentorship engagements."
              className="lg:col-span-2"
            >
              <ItemList
                items={mentorQueue}
                empty="No mentor profiles need review."
                render={(item) => (
                  <article className="grid gap-2">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <strong>{text(item, "name")}</strong>
                      <div className="flex flex-wrap items-center gap-2">
                        <State value={text(item, "status")} />
                        <span className="text-xs text-muted">
                          capacity {String(item.capacity ?? "—")}
                        </span>
                      </div>
                    </div>
                    <span className="text-xs text-muted">{text(item, "email")}</span>
                    <p className="m-0">{text(item, "bio")}</p>
                    <div className="flex flex-wrap gap-2 pt-1">
                      <Button
                        disabled={
                          busy ||
                          (item.external_grant_active !== true && item.campus_member !== true)
                        }
                        onClick={() =>
                          void action(`/admin/mentor-profiles/${text(item, "id")}/decision`, {
                            decision: "approved",
                          })
                        }
                      >
                        Approve
                      </Button>
                      <Button
                        variant="secondary"
                        disabled={busy}
                        onClick={() =>
                          void action(`/admin/mentor-profiles/${text(item, "id")}/decision`, {
                            decision: "suspended",
                          })
                        }
                      >
                        Suspend
                      </Button>
                      <Button
                        variant="danger"
                        disabled={busy}
                        onClick={() =>
                          void action(`/admin/mentor-profiles/${text(item, "id")}/decision`, {
                            decision: "revoked",
                          })
                        }
                      >
                        Revoke
                      </Button>
                    </div>
                  </article>
                )}
              />
            </Panel>
          </div>
        )}
      </main>
      <SiteFooter />
    </>
  );
}

function State({ value }: { value: string }) {
  const display = value ? value.replaceAll("_", " ") : "pending";
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${stateTone(value)}`}
    >
      {display}
    </span>
  );
}

function ItemList({
  items,
  empty,
  render,
}: {
  items: Item[];
  empty: string;
  render: (item: Item) => React.ReactNode;
}) {
  if (!items.length) {
    return (
      <p className="m-0 rounded-xl border border-dashed border-control bg-canvas p-4 text-[13px] text-muted">
        {empty}
      </p>
    );
  }
  return (
    <ul className="m-0 grid list-none gap-3 p-0">
      {items.map((item, index) => (
        <li
          key={text(item, "id") || index}
          className="grid gap-2 rounded-xl border border-hairline bg-canvas p-4 text-[13px] leading-5"
        >
          {render(item)}
        </li>
      ))}
    </ul>
  );
}

function MentorCard({
  mentor,
  projects,
  busy,
  onSubmit,
}: {
  mentor: Item;
  projects: Item[];
  busy: boolean;
  onSubmit: (event: FormEvent<HTMLFormElement>, path: string) => void;
}) {
  return (
    <article className="grid gap-3 rounded-xl border border-hairline bg-canvas p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <strong className="text-sm">{text(mentor, "name")}</strong>
        <State value={`${String(mentor.active_engagements ?? 0)}/${String(mentor.capacity ?? 1)} active`} />
      </div>
      {text(mentor, "bio") && (
        <p className="m-0 text-[13px] leading-5 text-muted">{text(mentor, "bio")}</p>
      )}
      {Array.isArray(mentor.expertise) && (mentor.expertise as string[]).length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {(mentor.expertise as string[]).map((tag) => (
            <span
              key={tag}
              className="inline-flex items-center rounded border border-hairline bg-surface px-2 py-0.5 text-xs font-medium text-on-surface"
            >
              {tag}
            </span>
          ))}
        </div>
      )}
      <form
        className="grid gap-2.5 border-t border-hairline pt-3"
        onSubmit={(event) => void onSubmit(event, "/projects/PROJECT_ID/mentorship-requests")}
      >
        <SelectField
          name="project_id"
          label="Project"
          options={projects.map((project) => ({
            value: text(project, "id"),
            label: text(project, "title") || text(project, "name"),
          }))}
        />
        <TextField
          name="learning_goal"
          label="What would you like help with?"
          minLength={10}
          maxLength={2000}
          required
        />
        <input name="mentor_id" type="hidden" value={text(mentor, "id")} readOnly />
        <div>
          <Button disabled={busy}>Request mentorship</Button>
        </div>
      </form>
    </article>
  );
}

function MentorshipRequest({
  item,
  busy,
  onAction,
}: {
  item: Item;
  busy: boolean;
  onAction: (path: string, body: Record<string, string>) => void;
}) {
  const id = text(item, "id");
  const state = text(item, "state");
  return (
    <article className="grid gap-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <strong>{text(item, "project")}</strong>
        <State value={state} />
      </div>
      <p className="m-0 text-muted">{text(item, "learning_goal")}</p>
      {state === "requested" && item.is_requester !== true && (
        <div className="flex flex-wrap gap-2 pt-1">
          <Button
            disabled={busy}
            onClick={() =>
              void onAction(`/mentorship-requests/${id}/decision`, {
                decision: "accepted",
                ends_at: new Date(Date.now() + 90 * 86400000).toISOString(),
              })
            }
          >
            Accept for 90 days
          </Button>
          <Button
            variant="secondary"
            disabled={busy}
            onClick={() =>
              void onAction(`/mentorship-requests/${id}/decision`, {
                decision: "declined",
              })
            }
          >
            Decline
          </Button>
        </div>
      )}
      {state === "accepted" && (
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <a
            className="inline-flex min-h-10 items-center justify-center rounded-lg border border-primary bg-primary px-3.5 py-2 text-[13px] font-semibold text-on-primary no-underline hover:opacity-90"
            href={`/mentorship/${text(item, "conversation_id")}`}
          >
            Open scoped conversation
          </a>
          <Button
            variant="danger"
            disabled={busy}
            onClick={() => void onAction(`/mentorship-requests/${id}/end`, {})}
          >
            End engagement
          </Button>
        </div>
      )}
    </article>
  );
}

function CallCard({
  call,
  projects,
  busy,
  canReview,
  onSubmit,
}: {
  call: Item;
  projects: Item[];
  busy: boolean;
  canReview: boolean;
  onSubmit: (event: FormEvent<HTMLFormElement>, path: string) => void;
}) {
  const [applications, setApplications] = useState<Item[]>([]);
  const [expanded, setExpanded] = useState(false);
  const [error, setError] = useState("");

  async function loadApplications() {
    setExpanded(true);
    setError("");
    try {
      const response = await fetch(`/api/v1/academic/calls/${text(call, "id")}/applications`, {
        cache: "no-store",
      });
      const envelope = await readApiEnvelope<{ items?: Item[] }>(
        response,
        "Applications are unavailable.",
      );
      if (!response.ok) {
        throw new Error(envelope.error?.message ?? "Applications are unavailable.");
      }
      setApplications(envelope.data?.items ?? []);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Applications are unavailable.");
    }
  }

  async function decision(applicationID: string, selected: string) {
    setError("");
    try {
      const sessionResponse = await fetch("/api/v1/session", { cache: "no-store" });
      const session = await readApiEnvelope<{ csrf_token?: string }>(
        sessionResponse,
        "Session expired.",
      );
      const response = await fetch(`/api/v1/academic/applications/${applicationID}/decision`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": session.data?.csrf_token ?? "",
        },
        body: JSON.stringify({ decision: selected }),
      });
      const envelope = await readApiEnvelope(response, "Application decision failed.");
      if (!response.ok) {
        throw new Error(envelope.error?.message ?? "Application decision failed.");
      }
      await loadApplications();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Application decision failed.");
    }
  }

  return (
    <article className="grid gap-3 rounded-xl border border-hairline bg-canvas p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <strong className="text-sm">{text(call, "title")}</strong>
        <div className="flex flex-wrap items-center gap-1.5">
          <State value={text(call, "kind")} />
          {text(call, "status") && <State value={text(call, "status")} />}
        </div>
      </div>
      <p className="m-0 text-[13px] leading-5 text-muted">{text(call, "description")}</p>
      {text(call, "eligibility") && (
        <p className="m-0 text-xs text-muted">Eligibility: {text(call, "eligibility")}</p>
      )}
      {text(call, "status") === "open" && (
        <form
          className="grid gap-2.5 border-t border-hairline pt-3"
          onSubmit={(event) =>
            void onSubmit(event, `/academic/calls/${text(call, "id")}/applications`)
          }
        >
          <SelectField
            name="project_id"
            label="Project"
            options={projects.map((project) => ({
              value: text(project, "id"),
              label: text(project, "title") || text(project, "name"),
            }))}
          />
          <TextField
            name="statement"
            label="Application statement"
            minLength={10}
            maxLength={4000}
            required
          />
          <div>
            <Button disabled={busy}>Apply with project</Button>
          </div>
        </form>
      )}
      {canReview && (
        <button
          type="button"
          onClick={() => void loadApplications()}
          className="cursor-pointer justify-self-start border-0 bg-transparent p-0 text-[13px] font-semibold text-link underline underline-offset-2"
        >
          {expanded ? "Refresh applications" : "Review applications"}
        </button>
      )}
      {error && (
        <p role="alert" className="m-0 text-[13px] text-danger">
          {error}
        </p>
      )}
      {expanded && (
        <ItemList
          items={applications}
          empty="No applications are visible in this reviewer scope."
          render={(application) => (
            <>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <strong>
                  {text(application, "project")} · {text(application, "applicant")}
                </strong>
                <State value={text(application, "state")} />
              </div>
              <p className="m-0 whitespace-pre-wrap text-muted">
                {text(application, "statement")}
              </p>
              <p className="m-0 text-xs text-muted">
                My current recommendation: {text(application, "my_recommendation") || "none"}
              </p>
              <form
                className="grid gap-2 border-t border-hairline pt-2"
                onSubmit={(event) =>
                  void onSubmit(
                    event,
                    `/academic/applications/${text(application, "id")}/review`,
                  )
                }
              >
                <SelectField
                  name="recommendation"
                  label="Recommendation"
                  defaultValue="support"
                  options={[
                    { value: "support", label: "Support" },
                    { value: "decline", label: "Decline" },
                    { value: "abstain", label: "Abstain" },
                  ]}
                />
                <TextField name="notes" label="Review notes" maxLength={4000} />
                <div>
                  <Button variant="secondary" disabled={busy}>
                    Save review
                  </Button>
                </div>
              </form>
              {text(application, "state") !== "accepted" &&
                text(application, "state") !== "rejected" && (
                  <div className="flex flex-wrap gap-2">
                    <Button
                      disabled={busy}
                      onClick={() => void decision(text(application, "id"), "accepted")}
                    >
                      Accept
                    </Button>
                    <Button
                      variant="secondary"
                      disabled={busy}
                      onClick={() => void decision(text(application, "id"), "rejected")}
                    >
                      Reject
                    </Button>
                  </div>
                )}
            </>
          )}
        />
      )}
    </article>
  );
}
