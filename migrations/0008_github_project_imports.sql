-- Account-scoped GitHub App installation and just-in-time user authorization.
CREATE TABLE github_install_setup_states (
    state_hash bytea PRIMARY KEY CHECK (octet_length(state_hash)=32),
    college_id uuid NOT NULL,
    user_id uuid NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id,college_id) REFERENCES users(id,college_id) ON DELETE CASCADE
);
CREATE INDEX github_install_setup_states_expiry_idx ON github_install_setup_states(expires_at);

CREATE TABLE github_installation_user_grants (
    college_id uuid NOT NULL,
    user_id uuid NOT NULL,
    installation_id uuid NOT NULL,
    external_user_id bigint NOT NULL CHECK (external_user_id>0),
    confirmed_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    PRIMARY KEY (college_id,user_id,installation_id),
    FOREIGN KEY (user_id,college_id) REFERENCES users(id,college_id) ON DELETE CASCADE,
    FOREIGN KEY (installation_id,college_id) REFERENCES github_installations(id,college_id) ON DELETE CASCADE
);
CREATE INDEX github_installation_user_grants_current_idx ON github_installation_user_grants(college_id,user_id,confirmed_at DESC) WHERE revoked_at IS NULL;

CREATE TABLE github_repository_authorization_states (
    state_hash bytea PRIMARY KEY CHECK (octet_length(state_hash)=32),
    college_id uuid NOT NULL,
    user_id uuid NOT NULL,
    purpose text NOT NULL CHECK (purpose IN ('connect','import','link')),
    installation_id uuid NOT NULL,
    project_id uuid,
    external_repository_id bigint CHECK (external_repository_id>0),
    pkce_verifier text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id,college_id) REFERENCES users(id,college_id) ON DELETE CASCADE,
    FOREIGN KEY (installation_id,college_id) REFERENCES github_installations(id,college_id) ON DELETE CASCADE,
    FOREIGN KEY (project_id,college_id) REFERENCES projects(id,college_id) ON DELETE CASCADE,
    CHECK ((purpose='connect' AND external_repository_id IS NULL AND project_id IS NULL) OR (purpose='import' AND external_repository_id IS NOT NULL AND project_id IS NULL) OR (purpose='link' AND external_repository_id IS NULL AND project_id IS NOT NULL))
);
CREATE INDEX github_repository_authorization_states_expiry_idx ON github_repository_authorization_states(expires_at);

CREATE TABLE github_repository_candidates (
    college_id uuid NOT NULL,
    user_id uuid NOT NULL,
    installation_id uuid NOT NULL,
    external_repository_id bigint NOT NULL CHECK (external_repository_id>0),
    owner_login text NOT NULL CHECK (length(owner_login) BETWEEN 1 AND 100),
    repository_name text NOT NULL CHECK (length(repository_name) BETWEEN 1 AND 100),
    description text NOT NULL DEFAULT '' CHECK (length(description)<=1000),
    default_branch text NOT NULL DEFAULT '',
    visibility text NOT NULL CHECK (visibility IN ('public','private','internal')),
    topics text[] NOT NULL DEFAULT '{}',
    expires_at timestamptz NOT NULL,
    checked_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (college_id,user_id,external_repository_id),
    FOREIGN KEY (user_id,college_id) REFERENCES users(id,college_id) ON DELETE CASCADE,
    FOREIGN KEY (installation_id,college_id) REFERENCES github_installations(id,college_id) ON DELETE CASCADE
);
CREATE INDEX github_repository_candidates_expiry_idx ON github_repository_candidates(expires_at);

ALTER TABLE repository_bindings
    ADD COLUMN html_url text NOT NULL DEFAULT '',
    ADD COLUMN description text NOT NULL DEFAULT '' CHECK (length(description)<=1000),
    ADD COLUMN topics text[] NOT NULL DEFAULT '{}',
    ADD COLUMN languages jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(languages)='object'),
    ADD COLUMN readme_markdown text NOT NULL DEFAULT '' CHECK (length(readme_markdown)<=1048576),
    ADD COLUMN overview_checked_at timestamptz;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM repository_bindings
        GROUP BY college_id,provider,external_repository_id HAVING count(*)>1
    ) THEN
        RAISE EXCEPTION 'duplicate GitHub repository bindings within a campus; review repository_bindings before applying migration 0008';
    END IF;
END $$;

DROP INDEX repository_bindings_provider_current_idx;
CREATE UNIQUE INDEX repository_bindings_campus_repository_idx
    ON repository_bindings(college_id,provider,external_repository_id);
