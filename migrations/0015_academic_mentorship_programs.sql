CREATE TABLE faculty_verification_requests (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    user_id uuid NOT NULL,
    department_id uuid,
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 2 AND 120),
    affiliation_note text NOT NULL CHECK (length(btrim(affiliation_note)) BETWEEN 8 AND 2000),
    state text NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'approved', 'rejected', 'withdrawn')),
    reviewed_by uuid,
    review_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    reviewed_at timestamptz,
    UNIQUE (id, college_id),
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (department_id, college_id) REFERENCES organizations(id, college_id),
    FOREIGN KEY (reviewed_by, college_id) REFERENCES users(id, college_id),
    CHECK ((state = 'pending') = (reviewed_at IS NULL))
);
CREATE UNIQUE INDEX faculty_verification_one_pending_idx
    ON faculty_verification_requests(college_id, user_id) WHERE state = 'pending';
CREATE INDEX faculty_verification_queue_idx
    ON faculty_verification_requests(college_id, created_at, id) WHERE state = 'pending';

CREATE TABLE faculty_profiles (
    college_id uuid NOT NULL,
    user_id uuid NOT NULL,
    department_id uuid,
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 2 AND 120),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'revoked')),
    verified_by uuid,
    verified_at timestamptz NOT NULL DEFAULT now(),
    revoked_by uuid,
    revoked_at timestamptz,
    PRIMARY KEY (college_id, user_id),
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (department_id, college_id) REFERENCES organizations(id, college_id),
    FOREIGN KEY (verified_by, college_id) REFERENCES users(id, college_id),
    FOREIGN KEY (revoked_by, college_id) REFERENCES users(id, college_id),
    CHECK ((status = 'revoked') = (revoked_at IS NOT NULL))
);

CREATE TABLE mentor_profiles (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bio text NOT NULL DEFAULT '' CHECK (length(bio) <= 2000),
    expertise text[] NOT NULL DEFAULT '{}',
    capacity smallint NOT NULL DEFAULT 1 CHECK (capacity BETWEEN 1 AND 10),
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'active', 'suspended', 'revoked', 'rejected')),
    verified_by uuid,
    verified_at timestamptz,
    revoked_by uuid,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (college_id, user_id),
    UNIQUE (id, college_id),
    FOREIGN KEY (verified_by, college_id) REFERENCES users(id, college_id),
    FOREIGN KEY (revoked_by, college_id) REFERENCES users(id, college_id),
    CHECK (status <> 'active' OR (verified_at IS NOT NULL AND revoked_at IS NULL)),
    CHECK ((status = 'revoked') = (revoked_at IS NOT NULL))
);
CREATE INDEX mentor_profiles_directory_idx
    ON mentor_profiles(college_id, status, updated_at DESC) WHERE status = 'active';

CREATE TABLE mentorship_requests (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    mentor_profile_id uuid NOT NULL,
    requester_id uuid NOT NULL,
    learning_goal text NOT NULL CHECK (length(btrim(learning_goal)) BETWEEN 10 AND 2000),
    state text NOT NULL DEFAULT 'requested'
        CHECK (state IN ('requested', 'accepted', 'declined', 'withdrawn', 'ended', 'revoked')),
    requested_at timestamptz NOT NULL DEFAULT now(),
    accepted_at timestamptz,
    ends_at timestamptz,
    closed_at timestamptz,
    closed_by uuid,
    close_reason text,
    UNIQUE (id, college_id),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (mentor_profile_id, college_id) REFERENCES mentor_profiles(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (requester_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (closed_by) REFERENCES users(id),
    CHECK ((state IN ('requested', 'accepted')) = (closed_at IS NULL)),
    CHECK (state <> 'accepted' OR (accepted_at IS NOT NULL AND ends_at IS NOT NULL)),
    CHECK (state NOT IN ('requested', 'declined', 'withdrawn') OR (accepted_at IS NULL AND ends_at IS NULL))
);
CREATE UNIQUE INDEX mentorship_request_project_scope_idx
    ON mentorship_requests(id, project_id, college_id);
CREATE UNIQUE INDEX mentorship_one_open_request_idx
    ON mentorship_requests(project_id, mentor_profile_id, requester_id)
    WHERE state IN ('requested', 'accepted');
CREATE INDEX mentorship_requests_mentor_queue_idx
    ON mentorship_requests(college_id, mentor_profile_id, state, requested_at DESC);
CREATE INDEX mentorship_requests_project_idx
    ON mentorship_requests(college_id, project_id, state, requested_at DESC);

CREATE TABLE academic_terms (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    label text NOT NULL CHECK (length(btrim(label)) BETWEEN 2 AND 100),
    starts_at date NOT NULL,
    ends_at date NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, college_id),
    UNIQUE (college_id, label),
    FOREIGN KEY (created_by, college_id) REFERENCES users(id, college_id),
    CHECK (ends_at >= starts_at)
);

CREATE TABLE academic_courses (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    department_id uuid,
    course_code text NOT NULL CHECK (length(btrim(course_code)) BETWEEN 1 AND 40),
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 3 AND 160),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 4000),
    created_by uuid NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, college_id),
    UNIQUE (college_id, course_code),
    FOREIGN KEY (department_id, college_id) REFERENCES organizations(id, college_id),
    FOREIGN KEY (created_by, college_id) REFERENCES users(id, college_id)
);

CREATE TABLE academic_course_staff (
    college_id uuid NOT NULL,
    course_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text NOT NULL CHECK (role IN ('lead', 'evaluator')),
    assigned_by uuid NOT NULL,
    assigned_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (course_id, user_id),
    FOREIGN KEY (course_id, college_id) REFERENCES academic_courses(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (assigned_by, college_id) REFERENCES users(id, college_id)
);

CREATE TABLE academic_cohorts (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    course_id uuid NOT NULL,
    term_id uuid NOT NULL,
    label text NOT NULL CHECK (length(btrim(label)) BETWEEN 2 AND 120),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, college_id),
    UNIQUE (course_id, term_id, label),
    FOREIGN KEY (course_id, college_id) REFERENCES academic_courses(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (term_id, college_id) REFERENCES academic_terms(id, college_id),
    FOREIGN KEY (created_by, college_id) REFERENCES users(id, college_id)
);

CREATE TABLE academic_enrollments (
    college_id uuid NOT NULL,
    cohort_id uuid NOT NULL,
    user_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'withdrawn')),
    enrolled_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (cohort_id, user_id),
    FOREIGN KEY (cohort_id, college_id) REFERENCES academic_cohorts(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX academic_enrollments_user_idx ON academic_enrollments(college_id, user_id, status);

CREATE TABLE academic_assignments (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    cohort_id uuid NOT NULL,
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 3 AND 160),
    brief text NOT NULL CHECK (length(btrim(brief)) BETWEEN 1 AND 8000),
    due_at timestamptz NOT NULL,
    late_policy text NOT NULL DEFAULT 'accept' CHECK (late_policy IN ('accept', 'reject')),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, college_id),
    FOREIGN KEY (cohort_id, college_id) REFERENCES academic_cohorts(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (created_by, college_id) REFERENCES users(id, college_id)
);
CREATE INDEX academic_assignments_cohort_due_idx
    ON academic_assignments(college_id, cohort_id, due_at, id);

CREATE TABLE academic_assignment_evaluators (
    college_id uuid NOT NULL,
    assignment_id uuid NOT NULL,
    user_id uuid NOT NULL,
    assigned_by uuid NOT NULL,
    assigned_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (assignment_id, user_id),
    FOREIGN KEY (assignment_id, college_id) REFERENCES academic_assignments(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (assigned_by, college_id) REFERENCES users(id, college_id)
);

CREATE TABLE academic_submissions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    assignment_id uuid NOT NULL,
    project_id uuid NOT NULL,
    submitted_by uuid NOT NULL,
    snapshot_id uuid NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    due_at_receipt timestamptz NOT NULL,
    late boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, college_id),
    UNIQUE (assignment_id, project_id),
    UNIQUE (snapshot_id),
    UNIQUE (id, project_id, college_id),
    FOREIGN KEY (assignment_id, college_id) REFERENCES academic_assignments(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (submitted_by, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (snapshot_id, college_id) REFERENCES repository_snapshots(id, college_id) ON DELETE RESTRICT
);
CREATE INDEX academic_submissions_assignment_idx
    ON academic_submissions(college_id, assignment_id, received_at DESC, id DESC);

CREATE TABLE academic_evaluations (
    college_id uuid NOT NULL,
    submission_id uuid NOT NULL,
    evaluator_id uuid NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('complete', 'revise', 'not_assessed')),
    feedback text NOT NULL CHECK (length(btrim(feedback)) BETWEEN 1 AND 4000),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (submission_id, evaluator_id),
    FOREIGN KEY (submission_id, college_id) REFERENCES academic_submissions(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (evaluator_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);

CREATE TABLE project_endorsements (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    faculty_id uuid NOT NULL,
    course_id uuid,
    cohort_id uuid,
    statement text NOT NULL CHECK (length(btrim(statement)) BETWEEN 10 AND 2000),
    audience text NOT NULL DEFAULT 'campus' CHECK (audience IN ('campus', 'private')),
    revoked_at timestamptz,
    revoked_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, college_id),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (faculty_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (course_id, college_id) REFERENCES academic_courses(id, college_id),
    FOREIGN KEY (cohort_id, college_id) REFERENCES academic_cohorts(id, college_id),
    FOREIGN KEY (revoked_by, college_id) REFERENCES users(id, college_id),
    CHECK (course_id IS NULL OR cohort_id IS NULL)
);
CREATE INDEX project_endorsements_project_idx
    ON project_endorsements(college_id, project_id, created_at DESC) WHERE revoked_at IS NULL;

CREATE TABLE academic_calls (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('research', 'program')),
    organization_id uuid,
    created_by uuid NOT NULL,
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 3 AND 160),
    description text NOT NULL CHECK (length(btrim(description)) BETWEEN 1 AND 8000),
    eligibility text NOT NULL DEFAULT '' CHECK (length(eligibility) <= 4000),
    capacity integer CHECK (capacity IS NULL OR capacity BETWEEN 1 AND 500),
    closes_at timestamptz,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'open', 'closed', 'archived')),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, college_id),
    FOREIGN KEY (organization_id, college_id) REFERENCES organizations(id, college_id),
    FOREIGN KEY (created_by, college_id) REFERENCES users(id, college_id),
    CHECK ((kind = 'research' AND organization_id IS NULL) OR kind = 'program')
);
CREATE INDEX academic_calls_open_idx ON academic_calls(college_id, kind, closes_at, id) WHERE status = 'open';

CREATE TABLE academic_call_applications (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    call_id uuid NOT NULL,
    project_id uuid NOT NULL,
    applicant_id uuid NOT NULL,
    statement text NOT NULL CHECK (length(btrim(statement)) BETWEEN 10 AND 4000),
    state text NOT NULL DEFAULT 'submitted'
        CHECK (state IN ('submitted', 'under_review', 'accepted', 'rejected', 'withdrawn')),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    submitted_at timestamptz NOT NULL DEFAULT now(),
    decided_by uuid,
    decided_at timestamptz,
    UNIQUE (id, college_id),
    UNIQUE (id, call_id, college_id),
    UNIQUE (id, project_id, call_id, college_id),
    UNIQUE (call_id, project_id),
    FOREIGN KEY (call_id, college_id) REFERENCES academic_calls(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (applicant_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (decided_by, college_id) REFERENCES users(id, college_id),
    CHECK ((state IN ('accepted', 'rejected')) = (decided_at IS NOT NULL))
);
CREATE INDEX academic_call_applications_review_idx
    ON academic_call_applications(college_id, call_id, state, submitted_at DESC, id DESC);

CREATE TABLE academic_call_reviewers (
    college_id uuid NOT NULL,
    call_id uuid NOT NULL,
    reviewer_id uuid NOT NULL,
    assigned_by uuid NOT NULL,
    assigned_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (call_id, reviewer_id),
    FOREIGN KEY (call_id, college_id) REFERENCES academic_calls(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (reviewer_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (assigned_by, college_id) REFERENCES users(id, college_id)
);

CREATE TABLE academic_call_reviews (
    college_id uuid NOT NULL,
    call_id uuid NOT NULL,
    application_id uuid NOT NULL,
    reviewer_id uuid NOT NULL,
    recommendation text NOT NULL CHECK (recommendation IN ('support', 'decline', 'abstain')),
    notes text NOT NULL DEFAULT '' CHECK (length(notes) <= 4000),
    reviewed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (application_id, reviewer_id),
    FOREIGN KEY (application_id, call_id, college_id) REFERENCES academic_call_applications(id, call_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (call_id, reviewer_id) REFERENCES academic_call_reviewers(call_id, reviewer_id) ON DELETE CASCADE
);

CREATE TABLE academic_program_cohorts (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    call_id uuid NOT NULL,
    label text NOT NULL CHECK (length(btrim(label)) BETWEEN 2 AND 120),
    starts_at date,
    ends_at date,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, college_id),
    UNIQUE (id, call_id, college_id),
    UNIQUE (call_id, label),
    FOREIGN KEY (call_id, college_id) REFERENCES academic_calls(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (created_by, college_id) REFERENCES users(id, college_id),
    CHECK (starts_at IS NULL OR ends_at IS NULL OR ends_at >= starts_at)
);

CREATE TABLE academic_program_cohort_projects (
    college_id uuid NOT NULL,
    cohort_id uuid NOT NULL,
    call_id uuid NOT NULL,
    project_id uuid NOT NULL,
    application_id uuid NOT NULL,
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (cohort_id, project_id),
    FOREIGN KEY (cohort_id, call_id, college_id) REFERENCES academic_program_cohorts(id, call_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (application_id, project_id, call_id, college_id) REFERENCES academic_call_applications(id, project_id, call_id, college_id) ON DELETE CASCADE
);

CREATE TABLE academic_program_milestones (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    cohort_id uuid NOT NULL,
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 2 AND 160),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 4000),
    due_at timestamptz,
    status text NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'in_progress', 'complete', 'cancelled')),
    updated_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, college_id),
    FOREIGN KEY (cohort_id, college_id) REFERENCES academic_program_cohorts(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (updated_by, college_id) REFERENCES users(id, college_id)
);

CREATE TABLE academic_program_outcomes (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    cohort_id uuid NOT NULL,
    project_id uuid NOT NULL,
    summary text NOT NULL CHECK (length(btrim(summary)) BETWEEN 1 AND 4000),
    result text NOT NULL DEFAULT '' CHECK (length(result) <= 1000),
    evidence_submission_id uuid,
    updated_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, college_id),
    UNIQUE (cohort_id, project_id),
    FOREIGN KEY (cohort_id, college_id) REFERENCES academic_program_cohorts(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (evidence_submission_id, project_id, college_id) REFERENCES academic_submissions(id, project_id, college_id),
    FOREIGN KEY (updated_by, college_id) REFERENCES users(id, college_id)
);

CREATE TABLE academic_digest_preferences (
    college_id uuid NOT NULL,
    user_id uuid NOT NULL,
    cadence text NOT NULL DEFAULT 'weekly' CHECK (cadence IN ('off', 'weekly')),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (college_id, user_id),
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);

CREATE TABLE academic_exports (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    requested_by uuid NOT NULL,
    export_type text NOT NULL CHECK (export_type IN ('call', 'cohort')),
    call_id uuid,
    cohort_id uuid,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 200),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'ready', 'failed')),
    job_id uuid REFERENCES jobs(id) ON DELETE SET NULL,
    csv_content bytea CHECK (octet_length(csv_content) <= 5242880),
    csv_sha256 text CHECK (csv_sha256 ~ '^[a-f0-9]{64}$'),
    failure_code text,
    expires_at timestamptz NOT NULL DEFAULT now() + interval '7 days',
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE (id, college_id),
    UNIQUE (college_id, requested_by, idempotency_key),
    FOREIGN KEY (requested_by, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (call_id, college_id) REFERENCES academic_calls(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (cohort_id, college_id) REFERENCES academic_program_cohorts(id, college_id) ON DELETE CASCADE,
    CHECK ((export_type = 'call' AND call_id IS NOT NULL AND cohort_id IS NULL) OR
           (export_type = 'cohort' AND call_id IS NULL AND cohort_id IS NOT NULL)),
    CHECK ((state = 'ready') = (csv_content IS NOT NULL AND csv_sha256 IS NOT NULL AND completed_at IS NOT NULL))
);
CREATE INDEX academic_exports_owner_idx
    ON academic_exports(college_id, requested_by, created_at DESC, id DESC);

CREATE TABLE mentorship_conversations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    mentorship_request_id uuid NOT NULL,
    project_id uuid NOT NULL,
    state text NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'closed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    closed_at timestamptz,
    UNIQUE (id, college_id),
    UNIQUE (mentorship_request_id),
    FOREIGN KEY (mentorship_request_id, project_id, college_id) REFERENCES mentorship_requests(id, project_id, college_id) ON DELETE CASCADE,
    CHECK ((state = 'open') = (closed_at IS NULL))
);

CREATE TABLE mentorship_conversation_members (
    college_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, user_id),
    FOREIGN KEY (conversation_id, college_id) REFERENCES mentorship_conversations(id, college_id) ON DELETE CASCADE
);
CREATE INDEX mentorship_conversation_members_user_idx
    ON mentorship_conversation_members(college_id, user_id, conversation_id);

CREATE TABLE mentorship_messages (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    sender_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_message_id uuid NOT NULL,
    body text NOT NULL CHECK (length(btrim(body)) BETWEEN 1 AND 5000),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, conversation_id, college_id),
    UNIQUE (conversation_id, sender_id, client_message_id),
    FOREIGN KEY (conversation_id, college_id) REFERENCES mentorship_conversations(id, college_id) ON DELETE CASCADE
);
CREATE INDEX mentorship_messages_page_idx
    ON mentorship_messages(college_id, conversation_id, created_at DESC, id DESC);

CREATE TABLE mentorship_message_reports (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    message_id uuid NOT NULL,
    reporter_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason text NOT NULL CHECK (reason IN ('spam', 'harassment', 'unsafe', 'other')),
    details text NOT NULL DEFAULT '' CHECK (length(details) <= 1000),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (message_id, reporter_id),
    FOREIGN KEY (conversation_id, college_id) REFERENCES mentorship_conversations(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (message_id, conversation_id, college_id) REFERENCES mentorship_messages(id, conversation_id, college_id) ON DELETE CASCADE
);

CREATE TABLE mentorship_blocks (
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    blocker_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (college_id, blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
);
CREATE INDEX mentorship_blocks_reverse_idx
    ON mentorship_blocks(college_id, blocked_id, blocker_id);

CREATE OR REPLACE FUNCTION close_mentorship_conversation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.state IN ('declined', 'withdrawn', 'ended', 'revoked') AND OLD.state IS DISTINCT FROM NEW.state THEN
        UPDATE mentorship_conversations
        SET state = 'closed', closed_at = COALESCE(closed_at, now())
        WHERE mentorship_request_id = NEW.id
          AND college_id = NEW.college_id
          AND state = 'open';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER mentorship_requests_close_conversation
    AFTER UPDATE OF state ON mentorship_requests
    FOR EACH ROW EXECUTE FUNCTION close_mentorship_conversation();

CREATE OR REPLACE FUNCTION revoke_external_mentor_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.revoked_at IS NULL AND NEW.revoked_at IS NOT NULL AND NEW.access_kind = 'mentor' THEN
        UPDATE mentor_profiles
        SET status = 'revoked', revoked_by = NULL, revoked_at = NEW.revoked_at
        WHERE college_id = NEW.college_id AND user_id = NEW.user_id AND status <> 'revoked'
          AND NOT EXISTS (SELECT 1 FROM users WHERE id = NEW.user_id AND college_id = NEW.college_id);
        UPDATE mentorship_requests
        SET state = 'revoked', closed_at = NEW.revoked_at,
            close_reason = 'mentor_invitation_revoked'
        WHERE college_id = NEW.college_id
          AND state = 'accepted'
          AND mentor_profile_id IN (
              SELECT id FROM mentor_profiles
              WHERE college_id = NEW.college_id AND user_id = NEW.user_id
                AND NOT EXISTS (SELECT 1 FROM users WHERE id = NEW.user_id AND college_id = NEW.college_id)
          );
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER external_mentor_scope_revoked
    AFTER UPDATE OF revoked_at ON external_college_access
    FOR EACH ROW EXECUTE FUNCTION revoke_external_mentor_scope();

CREATE OR REPLACE FUNCTION touch_academic_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = clock_timestamp();
    RETURN NEW;
END $$;
CREATE TRIGGER mentor_profiles_updated_at BEFORE UPDATE ON mentor_profiles
    FOR EACH ROW EXECUTE FUNCTION touch_academic_updated_at();
CREATE TRIGGER academic_calls_updated_at BEFORE UPDATE ON academic_calls
    FOR EACH ROW EXECUTE FUNCTION touch_academic_updated_at();
CREATE TRIGGER academic_program_milestones_updated_at BEFORE UPDATE ON academic_program_milestones
    FOR EACH ROW EXECUTE FUNCTION touch_academic_updated_at();
CREATE TRIGGER academic_program_outcomes_updated_at BEFORE UPDATE ON academic_program_outcomes
    FOR EACH ROW EXECUTE FUNCTION touch_academic_updated_at();
