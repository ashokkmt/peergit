ALTER TABLE github_repository_authorization_states
    DROP CONSTRAINT github_repository_authorization_states_purpose_check;
ALTER TABLE github_repository_authorization_states
    ADD CONSTRAINT github_repository_authorization_states_purpose_check
    CHECK (purpose IN ('connect','refresh','import','link','pbl_copy'));

ALTER TABLE github_repository_authorization_states
    DROP CONSTRAINT github_repository_authorization_states_check;
ALTER TABLE github_repository_authorization_states
    ADD CONSTRAINT github_repository_authorization_states_check
    CHECK (
        (purpose IN ('connect','refresh') AND external_repository_id IS NULL AND project_id IS NULL) OR
        (purpose='import' AND external_repository_id IS NOT NULL AND project_id IS NULL) OR
        (purpose='link' AND external_repository_id IS NULL AND project_id IS NOT NULL) OR
        (purpose='pbl_copy' AND external_repository_id IS NOT NULL AND project_id IS NOT NULL)
    );

ALTER TABLE github_repository_candidates DROP CONSTRAINT github_repository_candidates_pkey;
ALTER TABLE github_repository_candidates ADD PRIMARY KEY (college_id,user_id,installation_id,external_repository_id);

ALTER TABLE projects
    ADD COLUMN github_revoke_deadline timestamptz,
    ADD COLUMN github_revoke_owner uuid,
    ADD COLUMN github_revoke_installation uuid,
    ADD COLUMN github_revoke_previous_visibility text,
    ADD COLUMN github_revoke_previous_lifecycle text,
    ADD CONSTRAINT projects_github_revoke_state_check CHECK (
        (github_revoke_deadline IS NULL AND github_revoke_owner IS NULL AND github_revoke_installation IS NULL
            AND github_revoke_previous_visibility IS NULL AND github_revoke_previous_lifecycle IS NULL)
        OR
        (github_revoke_deadline IS NOT NULL AND github_revoke_owner IS NOT NULL AND github_revoke_installation IS NOT NULL
            AND github_revoke_previous_visibility IN ('private','campus','public')
            AND github_revoke_previous_lifecycle IN ('draft','active','on_hold','completed','archived'))
    );
CREATE INDEX projects_github_revoke_expiry_idx ON projects(github_revoke_deadline) WHERE github_revoke_deadline IS NOT NULL;
