ALTER TABLE media ADD CONSTRAINT media_id_college_unique UNIQUE (id, college_id);

CREATE TABLE projects (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    slug text NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 3 AND 120),
    summary text NOT NULL CHECK (length(btrim(summary)) BETWEEN 1 AND 500),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 12000),
    project_type text NOT NULL CHECK (project_type IN ('side_project','coursework','research','startup','open_source','hackathon')),
    visibility text NOT NULL DEFAULT 'campus' CHECK (visibility IN ('private','campus','public')),
    lifecycle text NOT NULL DEFAULT 'draft' CHECK (lifecycle IN ('draft','active','on_hold','completed','archived')),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, college_id),
    UNIQUE (college_id, slug),
    FOREIGN KEY (created_by, college_id) REFERENCES users(id, college_id)
);
CREATE INDEX projects_discovery_idx ON projects(college_id, updated_at DESC, id DESC) WHERE lifecycle='active' AND visibility IN ('campus','public');

CREATE TABLE project_members (
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text NOT NULL CHECK (role IN ('owner','maintainer','member','mentor')),
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX project_members_user_idx ON project_members(college_id, user_id, project_id);

CREATE TABLE skills (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    name text NOT NULL UNIQUE CHECK (length(btrim(name)) BETWEEN 1 AND 80)
);
CREATE TABLE project_skills (
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    skill_id uuid NOT NULL REFERENCES skills(id),
    PRIMARY KEY (project_id, skill_id),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE
);
CREATE TABLE project_media (
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    media_id uuid NOT NULL,
    PRIMARY KEY (project_id, media_id),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (media_id, college_id) REFERENCES media(id, college_id) ON DELETE CASCADE
);

CREATE TABLE project_roles (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 2 AND 100),
    description text NOT NULL CHECK (length(btrim(description)) BETWEEN 1 AND 3000),
    openings integer NOT NULL CHECK (openings BETWEEN 1 AND 20),
    difficulty text NOT NULL DEFAULT 'beginner' CHECK (difficulty IN ('novice','beginner','intermediate','advanced')),
    good_first_task boolean NOT NULL DEFAULT false,
    prerequisite_skills text[] NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed')),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, college_id),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE
);
CREATE INDEX project_roles_open_idx ON project_roles(college_id, project_id, created_at) WHERE status='open';

CREATE TABLE applications (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_role_id uuid NOT NULL,
    applicant_user_id uuid NOT NULL,
    message text NOT NULL CHECK (length(btrim(message)) BETWEEN 1 AND 2000),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','rejected','withdrawn')),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    decided_by uuid,
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_role_id, college_id) REFERENCES project_roles(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (applicant_user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (decided_by, college_id) REFERENCES users(id, college_id),
    CHECK ((status IN ('pending','withdrawn') AND decided_by IS NULL AND decided_at IS NULL) OR
           (status IN ('accepted','rejected') AND decided_by IS NOT NULL AND decided_at IS NOT NULL))
);
CREATE UNIQUE INDEX applications_one_pending ON applications(project_role_id, applicant_user_id) WHERE status='pending';
CREATE INDEX applications_role_status_idx ON applications(college_id, project_role_id, status, created_at);

CREATE TABLE project_invitations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    invited_user_id uuid NOT NULL,
    role text NOT NULL CHECK (role IN ('maintainer','member','mentor')),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash)=32),
    invited_by uuid NOT NULL,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (invited_user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (invited_by, college_id) REFERENCES users(id, college_id),
    CHECK ((accepted_at IS NULL) OR revoked_at IS NULL)
);
CREATE INDEX project_invitations_user_idx ON project_invitations(invited_user_id, expires_at) WHERE accepted_at IS NULL AND revoked_at IS NULL;

