CREATE TABLE project_updates (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    author_id uuid NOT NULL,
    body text NOT NULL CHECK (length(btrim(body)) BETWEEN 1 AND 5000),
    audience text NOT NULL DEFAULT 'members' CHECK (audience IN ('members','campus')),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    edited_at timestamptz,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, project_id, college_id),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (author_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX project_updates_project_time_idx ON project_updates(college_id, project_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE project_discussions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    author_id uuid NOT NULL,
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 3 AND 160),
    body text NOT NULL CHECK (length(btrim(body)) BETWEEN 1 AND 5000),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open','locked','hidden')),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    edited_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, project_id, college_id),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (author_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX project_discussions_project_time_idx ON project_discussions(college_id, project_id, created_at DESC, id DESC)
    WHERE status <> 'hidden';

CREATE TABLE project_milestones (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    created_by uuid NOT NULL,
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 3 AND 160),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 3000),
    due_at timestamptz,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open','completed','cancelled')),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, project_id, college_id),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (created_by, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX project_milestones_project_due_idx ON project_milestones(college_id, project_id, due_at, id);

CREATE TABLE project_tasks (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    milestone_id uuid,
    created_by uuid NOT NULL,
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 3 AND 180),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 5000),
    status text NOT NULL DEFAULT 'todo' CHECK (status IN ('todo','in_progress','blocked','done')),
    priority text NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high')),
    position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
    due_at timestamptz,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, project_id, college_id),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (milestone_id, project_id, college_id) REFERENCES project_milestones(id, project_id, college_id) ON DELETE SET NULL (milestone_id),
    FOREIGN KEY (created_by, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX project_tasks_board_idx ON project_tasks(college_id, project_id, status, position, created_at, id);

CREATE TABLE project_task_assignees (
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    user_id uuid NOT NULL,
    assigned_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, user_id),
    FOREIGN KEY (task_id, project_id, college_id) REFERENCES project_tasks(id, project_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (assigned_by, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX project_task_assignees_user_idx ON project_task_assignees(college_id, user_id, task_id);

CREATE TABLE project_task_events (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    event_type text NOT NULL CHECK (event_type IN ('created','updated','status_changed','assignees_changed','milestone_changed')),
    details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object' AND octet_length(details::text) <= 8192),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (task_id, version),
    FOREIGN KEY (task_id, project_id, college_id) REFERENCES project_tasks(id, project_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (actor_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX project_task_events_task_time_idx ON project_task_events(college_id, project_id, task_id, created_at DESC, id DESC);

CREATE TABLE project_comments (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    update_id uuid,
    discussion_id uuid,
    task_id uuid,
    parent_comment_id uuid,
    author_id uuid NOT NULL,
    body text NOT NULL CHECK (length(btrim(body)) BETWEEN 1 AND 2000),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    edited_at timestamptz,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, project_id, college_id),
    CHECK (num_nonnulls(update_id, discussion_id, task_id) = 1),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (update_id, project_id, college_id) REFERENCES project_updates(id, project_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (discussion_id, project_id, college_id) REFERENCES project_discussions(id, project_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (task_id, project_id, college_id) REFERENCES project_tasks(id, project_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (parent_comment_id, project_id, college_id) REFERENCES project_comments(id, project_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (author_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX project_comments_update_idx ON project_comments(college_id, update_id, created_at, id) WHERE deleted_at IS NULL;
CREATE INDEX project_comments_discussion_idx ON project_comments(college_id, discussion_id, created_at, id) WHERE deleted_at IS NULL;
CREATE INDEX project_comments_task_idx ON project_comments(college_id, task_id, created_at, id) WHERE deleted_at IS NULL;

CREATE TABLE project_reactions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    update_id uuid,
    comment_id uuid,
    discussion_id uuid,
    user_id uuid NOT NULL,
    reaction text NOT NULL CHECK (reaction IN ('like','celebrate','insightful','support')),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(update_id, comment_id, discussion_id) = 1),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (update_id, project_id, college_id) REFERENCES project_updates(id, project_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (comment_id, project_id, college_id) REFERENCES project_comments(id, project_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (discussion_id, project_id, college_id) REFERENCES project_discussions(id, project_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX project_reactions_update_user_uidx ON project_reactions(update_id, user_id) WHERE update_id IS NOT NULL;
CREATE UNIQUE INDEX project_reactions_comment_user_uidx ON project_reactions(comment_id, user_id) WHERE comment_id IS NOT NULL;
CREATE UNIQUE INDEX project_reactions_discussion_user_uidx ON project_reactions(discussion_id, user_id) WHERE discussion_id IS NOT NULL;

CREATE TABLE project_follows (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, user_id),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX project_follows_user_time_idx ON project_follows(college_id, user_id, created_at DESC, id DESC);

CREATE TABLE project_bookmarks (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, user_id),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX project_bookmarks_user_time_idx ON project_bookmarks(college_id, user_id, created_at DESC, id DESC);

CREATE TABLE project_content_reports (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    update_id uuid,
    comment_id uuid,
    discussion_id uuid,
    task_id uuid,
    reporter_id uuid NOT NULL,
    reason text NOT NULL CHECK (reason IN ('spam','harassment','unsafe','other')),
    details text NOT NULL DEFAULT '' CHECK (length(details) <= 1000),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(update_id, comment_id, discussion_id, task_id) = 1),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (update_id, project_id, college_id) REFERENCES project_updates(id, project_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (comment_id, project_id, college_id) REFERENCES project_comments(id, project_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (discussion_id, project_id, college_id) REFERENCES project_discussions(id, project_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (task_id, project_id, college_id) REFERENCES project_tasks(id, project_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (reporter_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX project_content_reports_update_user_uidx ON project_content_reports(update_id, reporter_id) WHERE update_id IS NOT NULL;
CREATE UNIQUE INDEX project_content_reports_comment_user_uidx ON project_content_reports(comment_id, reporter_id) WHERE comment_id IS NOT NULL;
CREATE UNIQUE INDEX project_content_reports_discussion_user_uidx ON project_content_reports(discussion_id, reporter_id) WHERE discussion_id IS NOT NULL;
CREATE UNIQUE INDEX project_content_reports_task_user_uidx ON project_content_reports(task_id, reporter_id) WHERE task_id IS NOT NULL;
CREATE INDEX project_content_reports_queue_idx ON project_content_reports(college_id, created_at DESC, id DESC);
