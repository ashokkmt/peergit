CREATE TABLE github_install_link_states (
    state_hash bytea PRIMARY KEY CHECK (octet_length(state_hash)=32),
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    project_id uuid NOT NULL,
    user_id uuid NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id,college_id) REFERENCES projects(id,college_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id,college_id) REFERENCES users(id,college_id) ON DELETE CASCADE
);
CREATE INDEX github_install_link_states_expiry_idx ON github_install_link_states(expires_at);

CREATE TABLE github_installations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    external_installation_id bigint NOT NULL UNIQUE CHECK (external_installation_id>0),
    account_external_id bigint NOT NULL CHECK (account_external_id>0),
    account_login text NOT NULL CHECK (length(account_login) BETWEEN 1 AND 100),
    target_type text NOT NULL CHECK (target_type IN ('User','Organization','Enterprise')),
    repository_selection text NOT NULL CHECK (repository_selection IN ('all','selected')),
    permissions jsonb NOT NULL CHECK (jsonb_typeof(permissions)='object'),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','removed')),
    added_by uuid NOT NULL,
    installed_at timestamptz,
    checked_at timestamptz NOT NULL DEFAULT now(),
    removed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id,college_id),
    FOREIGN KEY (added_by,college_id) REFERENCES users(id,college_id)
);
CREATE INDEX github_installations_campus_idx ON github_installations(college_id,status,account_login);

CREATE TABLE repositories (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL UNIQUE,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id,college_id),
    FOREIGN KEY (project_id,college_id) REFERENCES projects(id,college_id) ON DELETE CASCADE,
    FOREIGN KEY (created_by,college_id) REFERENCES users(id,college_id)
);

CREATE TABLE repository_bindings (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    installation_id uuid NOT NULL,
    provider text NOT NULL DEFAULT 'github' CHECK (provider='github'),
    external_repository_id bigint NOT NULL CHECK (external_repository_id>0),
    owner_login text NOT NULL CHECK (length(owner_login) BETWEEN 1 AND 100),
    repository_name text NOT NULL CHECK (length(repository_name) BETWEEN 1 AND 100),
    default_branch text NOT NULL DEFAULT '',
    visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('public','private','internal')),
    is_current boolean NOT NULL DEFAULT true,
    access_state text NOT NULL DEFAULT 'active' CHECK (access_state IN ('active','stale','permission_lost','inactive')),
    sync_health text NOT NULL DEFAULT 'pending' CHECK (sync_health IN ('pending','healthy','delayed','failed')),
    last_synced_at timestamptz,
    last_error_code text,
    linked_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id,repository_id,college_id),
    FOREIGN KEY (repository_id,college_id) REFERENCES repositories(id,college_id) ON DELETE CASCADE,
    FOREIGN KEY (installation_id,college_id) REFERENCES github_installations(id,college_id) ON DELETE CASCADE,
    FOREIGN KEY (linked_by,college_id) REFERENCES users(id,college_id)
);
CREATE UNIQUE INDEX repository_bindings_one_current_idx ON repository_bindings(repository_id) WHERE is_current;
CREATE UNIQUE INDEX repository_bindings_provider_current_idx ON repository_bindings(provider,external_repository_id) WHERE is_current AND access_state IN ('active','stale');
CREATE INDEX repository_bindings_tenant_idx ON repository_bindings(college_id,repository_id,access_state);

CREATE TABLE github_webhook_deliveries (
    delivery_id uuid PRIMARY KEY,
    event_type text NOT NULL CHECK (length(event_type) BETWEEN 1 AND 100),
    action text NOT NULL DEFAULT '' CHECK (length(action)<=100),
    external_installation_id bigint CHECK (external_installation_id>0),
    college_id uuid REFERENCES colleges(id) ON DELETE SET NULL,
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload)='object' AND octet_length(payload::text)<=1048576),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','processing','done','unmatched','failed')),
    attempts smallint NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 3),
    last_error_code text,
    received_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz
);
CREATE INDEX github_webhook_pending_idx ON github_webhook_deliveries(received_at,delivery_id) WHERE state IN ('pending','failed','unmatched');

CREATE TABLE repository_sync_cursors (
    college_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    binding_id uuid NOT NULL,
    scope text NOT NULL CHECK (scope IN ('commits','pull_requests','issues')),
    cursor_at timestamptz,
    cursor_sha text,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (binding_id,scope),
    FOREIGN KEY (binding_id,repository_id,college_id) REFERENCES repository_bindings(id,repository_id,college_id) ON DELETE CASCADE
);

CREATE TABLE contributions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('commit','pull_request','issue')),
    canonical_id text NOT NULL CHECK (length(canonical_id) BETWEEN 1 AND 180),
    github_author_id bigint CHECK (github_author_id>0),
    author_login text NOT NULL DEFAULT '' CHECK (length(author_login)<=100),
    author_name text NOT NULL DEFAULT '' CHECK (length(author_name)<=160),
    title text NOT NULL DEFAULT '' CHECK (length(title)<=500),
    summary text NOT NULL DEFAULT '' CHECK (length(summary)<=2000),
    occurred_at timestamptz,
    additions integer CHECK (additions>=0),
    deletions integer CHECK (deletions>=0),
    changed_files integer CHECK (changed_files>=0),
    canonical_data jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(canonical_data)='object'),
    first_observed_at timestamptz NOT NULL DEFAULT now(),
    last_observed_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (repository_id,kind,canonical_id),
    UNIQUE (id,repository_id,college_id),
    FOREIGN KEY (repository_id,college_id) REFERENCES repositories(id,college_id) ON DELETE CASCADE
);
CREATE INDEX contributions_timeline_idx ON contributions(college_id,repository_id,occurred_at DESC,id DESC);
CREATE INDEX contributions_author_idx ON contributions(college_id,github_author_id,occurred_at DESC) WHERE github_author_id IS NOT NULL;

CREATE TABLE contribution_observations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    binding_id uuid NOT NULL,
    contribution_id uuid NOT NULL,
    provider_updated_at timestamptz,
    observed_at timestamptz NOT NULL DEFAULT now(),
    observation_hash bytea NOT NULL CHECK (octet_length(observation_hash)=32),
    data jsonb NOT NULL CHECK (jsonb_typeof(data)='object'),
    UNIQUE (binding_id,contribution_id,observation_hash),
    FOREIGN KEY (binding_id,repository_id,college_id) REFERENCES repository_bindings(id,repository_id,college_id) ON DELETE CASCADE,
    FOREIGN KEY (contribution_id,repository_id,college_id) REFERENCES contributions(id,repository_id,college_id) ON DELETE CASCADE
);
CREATE INDEX contribution_observations_recent_idx ON contribution_observations(college_id,repository_id,observed_at DESC);

CREATE TABLE repository_snapshots (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    binding_id uuid NOT NULL,
    requested_by uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 200),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash)=32),
    requested_ref text NOT NULL CHECK (length(requested_ref) BETWEEN 1 AND 255),
    commit_sha text NOT NULL CHECK (commit_sha ~ '^[a-f0-9]{40}$'),
    canonical_commit_id text GENERATED ALWAYS AS ('git:sha1:'||commit_sha) STORED,
    receipt_at timestamptz NOT NULL DEFAULT now(),
    membership_snapshot jsonb NOT NULL CHECK (jsonb_typeof(membership_snapshot)='array'),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','capturing','verified','failed')),
    attempt_generation bigint NOT NULL DEFAULT 0 CHECK (attempt_generation>=0),
    object_key text,
    archive_bytes bigint CHECK (archive_bytes>=0),
    archive_sha256 text CHECK (archive_sha256 ~ '^[a-f0-9]{64}$'),
    capture_started_at timestamptz,
    verified_at timestamptz,
    failure_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (college_id,requested_by,idempotency_key),
    UNIQUE (id,college_id),
    FOREIGN KEY (project_id,college_id) REFERENCES projects(id,college_id) ON DELETE CASCADE,
    FOREIGN KEY (repository_id,college_id) REFERENCES repositories(id,college_id) ON DELETE CASCADE,
    FOREIGN KEY (binding_id,repository_id,college_id) REFERENCES repository_bindings(id,repository_id,college_id) ON DELETE RESTRICT,
    FOREIGN KEY (requested_by,college_id) REFERENCES users(id,college_id),
    CHECK ((state='verified')=(object_key IS NOT NULL AND archive_bytes IS NOT NULL AND archive_sha256 IS NOT NULL AND verified_at IS NOT NULL))
);
CREATE INDEX repository_snapshots_project_idx ON repository_snapshots(college_id,project_id,created_at DESC,id DESC);
CREATE INDEX repository_snapshots_binding_sha_idx ON repository_snapshots(binding_id,commit_sha,created_at DESC);

CREATE FUNCTION touch_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at=clock_timestamp();
    RETURN NEW;
END;
$$;

CREATE TRIGGER github_installations_updated_at BEFORE UPDATE ON github_installations FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER repositories_updated_at BEFORE UPDATE ON repositories FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER repository_bindings_updated_at BEFORE UPDATE ON repository_bindings FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER repository_snapshots_updated_at BEFORE UPDATE ON repository_snapshots FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
