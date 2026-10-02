CREATE TABLE colleges (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 2 AND 160),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, slug)
);

CREATE TABLE college_domains (
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    domain text NOT NULL CHECK (domain = lower(domain) AND position('@' in domain) = 0),
    verified_at timestamptz,
    PRIMARY KEY (college_id, domain),
    UNIQUE (domain)
);

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid REFERENCES colleges(id),
    email text NOT NULL,
    email_normalized text NOT NULL UNIQUE,
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 120),
    handle text NOT NULL UNIQUE CHECK (handle ~ '^[a-z0-9_]{3,30}$'),
    account_type text NOT NULL DEFAULT 'campus' CHECK (account_type IN ('campus','external','platform')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','deletion_pending','deleted')),
    email_verified_at timestamptz,
    profile jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (id, college_id),
    CHECK ((account_type = 'campus' AND college_id IS NOT NULL) OR
           (account_type IN ('external','platform') AND college_id IS NULL))
);
CREATE INDEX users_campus_idx ON users(college_id, id) WHERE status = 'active';

CREATE TABLE campus_verifications (
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    user_id uuid NOT NULL,
    source text NOT NULL CHECK (source IN ('verified_domain','administrator_invitation')),
    verified_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    PRIMARY KEY (college_id, user_id),
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);

CREATE TABLE external_college_access (
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    access_kind text NOT NULL CHECK (access_kind IN ('mentor','recruiter','organization_guest')),
    granted_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    PRIMARY KEY (college_id, user_id, access_kind)
);

CREATE TABLE user_identities (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider text NOT NULL CHECK (provider = 'google'),
    subject text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, subject),
    UNIQUE (user_id, provider)
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL,
    college_id uuid,
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    csrf_hash bytea NOT NULL CHECK (octet_length(csrf_hash) = 32),
    mfa_verified_at timestamptz,
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX sessions_user_idx ON sessions(user_id, expires_at);

CREATE TABLE oidc_login_states (
    state_hash bytea PRIMARY KEY CHECK (octet_length(state_hash) = 32),
    nonce text NOT NULL,
    pkce_verifier text NOT NULL,
    invitation_token_hash bytea,
    expires_at timestamptz NOT NULL
);
CREATE INDEX oidc_login_states_expiry_idx ON oidc_login_states(expires_at);

CREATE TABLE campus_roles (
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    user_id uuid NOT NULL,
    role text NOT NULL CHECK (role IN ('student','faculty','alumni_mentor','moderator','campus_admin')),
    granted_by uuid REFERENCES users(id) ON DELETE SET NULL,
    granted_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (college_id, user_id, role),
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);

CREATE TABLE organizations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('department','club','innovation_cell','placement_cell','event_body')),
    slug text NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 2 AND 160),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 2000),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, college_id),
    UNIQUE (college_id, slug)
);

CREATE TABLE organization_members (
    college_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text NOT NULL CHECK (role IN ('member','manager')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id),
    FOREIGN KEY (organization_id, college_id) REFERENCES organizations(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);

CREATE TABLE invitations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    email_normalized text NOT NULL,
    account_type text NOT NULL CHECK (account_type IN ('campus','external')),
    role text CHECK (role IN ('student','faculty','alumni_mentor','moderator','campus_admin')),
    external_access_kind text CHECK (external_access_kind IN ('mentor','recruiter','organization_guest')),
    organization_id uuid,
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    invited_by uuid REFERENCES users(id) ON DELETE SET NULL,
    expires_at timestamptz NOT NULL,
    accepted_by uuid REFERENCES users(id) ON DELETE SET NULL,
    accepted_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id, college_id) REFERENCES organizations(id, college_id),
    CHECK ((account_type = 'campus' AND role IS NOT NULL AND external_access_kind IS NULL) OR
           (account_type = 'external' AND role IS NULL AND external_access_kind IS NOT NULL)),
    CHECK ((accepted_by IS NULL) = (accepted_at IS NULL))
);
CREATE INDEX invitations_pending_email_idx ON invitations(college_id, email_normalized, expires_at)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

CREATE TABLE skills (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    name text NOT NULL UNIQUE
);
CREATE TABLE user_skills (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    skill_id uuid NOT NULL REFERENCES skills(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, skill_id)
);

CREATE TABLE consent_records (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose text NOT NULL CHECK (purpose IN ('terms','privacy','profile_discovery','portfolio','recruiter_discovery','analytics')),
    policy_version text NOT NULL,
    granted_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    UNIQUE (user_id, purpose, policy_version, granted_at)
);
CREATE INDEX consent_current_idx ON consent_records(user_id, purpose, granted_at DESC) WHERE revoked_at IS NULL;

CREATE TABLE mfa_credentials (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    encrypted_secret bytea NOT NULL,
    enabled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE mfa_recovery_codes (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash bytea NOT NULL CHECK (octet_length(code_hash) = 32),
    used_at timestamptz,
    PRIMARY KEY (user_id, code_hash)
);

CREATE TABLE login_rate_limits (
    bucket_hash bytea PRIMARY KEY CHECK (octet_length(bucket_hash) = 32),
    window_started_at timestamptz NOT NULL,
    request_count integer NOT NULL CHECK (request_count > 0),
    blocked_until timestamptz
);
CREATE INDEX login_rate_limits_window_idx ON login_rate_limits(window_started_at);

CREATE TABLE media (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    owner_user_id uuid NOT NULL,
    object_key text NOT NULL UNIQUE,
    mime_type text NOT NULL CHECK (mime_type IN ('image/png','image/jpeg')),
    byte_size bigint NOT NULL CHECK (byte_size BETWEEN 1 AND 5242880),
    sha256 bytea NOT NULL CHECK (octet_length(sha256) = 32),
    scan_status text NOT NULL CHECK (scan_status IN ('pending','clean','rejected')),
    visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','campus')),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (owner_user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
