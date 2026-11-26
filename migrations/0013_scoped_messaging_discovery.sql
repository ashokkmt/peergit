CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

ALTER TABLE users
    ADD COLUMN profile_search_document tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(display_name, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(handle, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(profile->>'headline', '')), 'B') ||
        setweight(to_tsvector('english', coalesce(profile->>'bio', '')), 'C')
    ) STORED;

CREATE INDEX users_profile_search_idx
    ON users USING gin(profile_search_document);
CREATE INDEX users_name_trgm_idx
    ON users USING gin(display_name public.gin_trgm_ops);

ALTER TABLE projects
    ADD COLUMN search_document tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(summary, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(description, '')), 'C')
    ) STORED;

CREATE INDEX projects_search_idx ON projects USING gin(search_document);
CREATE INDEX projects_title_trgm_idx ON projects USING gin(title public.gin_trgm_ops);
CREATE INDEX projects_summary_trgm_idx ON projects USING gin(summary public.gin_trgm_ops);
CREATE INDEX users_handle_trgm_idx ON users USING gin(handle public.gin_trgm_ops);
CREATE INDEX project_roles_title_trgm_idx ON project_roles USING gin(title public.gin_trgm_ops);

CREATE TABLE conversations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    context_type text NOT NULL CHECK (context_type IN ('project', 'application')),
    project_id uuid,
    application_id uuid,
    state text NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'closed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    closed_at timestamptz,
    UNIQUE (id, college_id),
    CHECK (project_id IS NOT NULL AND
           ((context_type = 'project' AND application_id IS NULL) OR
            (context_type = 'application' AND application_id IS NOT NULL))),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (application_id, college_id) REFERENCES applications(id, college_id) ON DELETE CASCADE,
    CHECK ((state = 'open' AND closed_at IS NULL) OR (state = 'closed' AND closed_at IS NOT NULL))
);
CREATE UNIQUE INDEX conversations_project_uidx
    ON conversations(college_id, project_id) WHERE context_type = 'project';
CREATE UNIQUE INDEX conversations_application_uidx
    ON conversations(college_id, application_id) WHERE context_type = 'application';

CREATE TABLE conversation_participants (
    college_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    user_id uuid NOT NULL,
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, user_id),
    FOREIGN KEY (conversation_id, college_id) REFERENCES conversations(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX conversation_participants_user_idx
    ON conversation_participants(college_id, user_id, conversation_id);

CREATE TABLE conversation_messages (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    sender_id uuid NOT NULL,
    client_message_id uuid NOT NULL,
    body text NOT NULL CHECK (length(btrim(body)) BETWEEN 1 AND 5000),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, conversation_id, college_id),
    UNIQUE (conversation_id, sender_id, client_message_id),
    FOREIGN KEY (conversation_id, college_id) REFERENCES conversations(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (sender_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX conversation_messages_page_idx
    ON conversation_messages(college_id, conversation_id, created_at DESC, id DESC);

CREATE TABLE user_blocks (
    college_id uuid NOT NULL,
    blocker_id uuid NOT NULL,
    blocked_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id),
    FOREIGN KEY (blocker_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (blocked_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX user_blocks_reverse_idx ON user_blocks(college_id, blocked_id, blocker_id);

CREATE TABLE message_reports (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    message_id uuid NOT NULL,
    reporter_id uuid NOT NULL,
    reason text NOT NULL CHECK (reason IN ('spam', 'harassment', 'unsafe', 'other')),
    details text NOT NULL DEFAULT '' CHECK (length(details) <= 1000),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (message_id, reporter_id),
    FOREIGN KEY (conversation_id, college_id) REFERENCES conversations(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (message_id, conversation_id, college_id) REFERENCES conversation_messages(id, conversation_id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (reporter_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX message_reports_queue_idx ON message_reports(college_id, created_at DESC, id DESC);

ALTER TABLE notification_preferences DROP CONSTRAINT notification_preferences_category_check;
ALTER TABLE notification_preferences
    ADD CONSTRAINT notification_preferences_category_check
    CHECK (category IN ('applications', 'project_activity', 'messages'));
ALTER TABLE notifications DROP CONSTRAINT notifications_category_check;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_category_check
    CHECK (category IN ('applications', 'project_activity', 'messages'));
ALTER TABLE notifications DROP CONSTRAINT notifications_check;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_context_check
    CHECK ((category = 'applications' AND application_id IS NOT NULL) OR category IN ('project_activity', 'messages'));

CREATE TABLE project_collaboration_events (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    project_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    source_type text NOT NULL CHECK (source_type IN (
        'commit', 'pull_request', 'issue', 'application_accepted', 'task_transition',
        'project_update', 'discussion', 'comment', 'milestone_completed'
    )),
    source_key text NOT NULL CHECK (length(source_key) BETWEEN 1 AND 200),
    occurred_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (college_id, source_type, source_key),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (actor_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX project_collaboration_events_window_idx
    ON project_collaboration_events(college_id, occurred_at, project_id, actor_id);

ALTER TABLE project_milestones
    ADD COLUMN updated_by uuid;
ALTER TABLE project_milestones
    ADD CONSTRAINT project_milestones_updated_by_fk
    FOREIGN KEY (updated_by, college_id) REFERENCES users(id, college_id);

CREATE OR REPLACE FUNCTION record_project_collaboration_event(
    p_college uuid,
    p_project uuid,
    p_actor uuid,
    p_source_type text,
    p_source_key text,
    p_occurred_at timestamptz
) RETURNS void LANGUAGE sql AS $$
    INSERT INTO project_collaboration_events(college_id, project_id, actor_id, source_type, source_key, occurred_at)
    SELECT p_college, p_project, p_actor, p_source_type, p_source_key, p_occurred_at
    WHERE EXISTS (
        SELECT 1 FROM project_members
        WHERE college_id = p_college AND project_id = p_project AND user_id = p_actor
    )
    ON CONFLICT (college_id, source_type, source_key) DO NOTHING
$$;

CREATE OR REPLACE FUNCTION record_project_update_wacp() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM record_project_collaboration_event(NEW.college_id, NEW.project_id, NEW.author_id, 'project_update', NEW.id::text, NEW.created_at);
    RETURN NEW;
END $$;
CREATE TRIGGER project_updates_wacp AFTER INSERT ON project_updates
    FOR EACH ROW EXECUTE FUNCTION record_project_update_wacp();

CREATE OR REPLACE FUNCTION record_discussion_wacp() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM record_project_collaboration_event(NEW.college_id, NEW.project_id, NEW.author_id, 'discussion', NEW.id::text, NEW.created_at);
    RETURN NEW;
END $$;
CREATE TRIGGER project_discussions_wacp AFTER INSERT ON project_discussions
    FOR EACH ROW EXECUTE FUNCTION record_discussion_wacp();

CREATE OR REPLACE FUNCTION record_comment_wacp() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM record_project_collaboration_event(NEW.college_id, NEW.project_id, NEW.author_id, 'comment', NEW.id::text, NEW.created_at);
    RETURN NEW;
END $$;
CREATE TRIGGER project_comments_wacp AFTER INSERT ON project_comments
    FOR EACH ROW EXECUTE FUNCTION record_comment_wacp();

CREATE OR REPLACE FUNCTION record_task_transition_wacp() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.event_type = 'status_changed' THEN
        PERFORM record_project_collaboration_event(NEW.college_id, NEW.project_id, NEW.actor_id, 'task_transition', NEW.id::text, NEW.created_at);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER project_task_events_wacp AFTER INSERT ON project_task_events
    FOR EACH ROW EXECUTE FUNCTION record_task_transition_wacp();

CREATE OR REPLACE FUNCTION record_milestone_wacp() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status = 'completed' AND OLD.status <> 'completed' AND NEW.updated_by IS NOT NULL THEN
        PERFORM record_project_collaboration_event(NEW.college_id, NEW.project_id, NEW.updated_by, 'milestone_completed', NEW.id::text || ':' || NEW.version::text, NEW.updated_at);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER project_milestones_wacp AFTER UPDATE ON project_milestones
    FOR EACH ROW EXECUTE FUNCTION record_milestone_wacp();

CREATE OR REPLACE FUNCTION record_application_accepted_wacp() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_project uuid;
BEGIN
    IF NEW.status = 'accepted' AND OLD.status <> 'accepted' THEN
        SELECT project_id INTO target_project FROM project_roles WHERE id = NEW.project_role_id AND college_id = NEW.college_id;
        PERFORM record_project_collaboration_event(NEW.college_id, target_project, NEW.decided_by, 'application_accepted', NEW.id::text, NEW.decided_at);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER applications_accepted_wacp AFTER UPDATE ON applications
    FOR EACH ROW EXECUTE FUNCTION record_application_accepted_wacp();

CREATE OR REPLACE FUNCTION record_contribution_wacp() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_project uuid;
DECLARE target_actor uuid;
DECLARE source_kind text;
BEGIN
    SELECT project_id INTO target_project FROM repositories WHERE id = NEW.repository_id AND college_id = NEW.college_id;
    SELECT user_id INTO target_actor FROM user_identities WHERE provider = 'github' AND subject = NEW.github_author_id::text;
    source_kind := CASE NEW.kind WHEN 'commit' THEN 'commit' WHEN 'pull_request' THEN 'pull_request' ELSE 'issue' END;
    IF target_project IS NOT NULL AND target_actor IS NOT NULL AND NEW.occurred_at IS NOT NULL THEN
        PERFORM record_project_collaboration_event(NEW.college_id, target_project, target_actor, source_kind, NEW.repository_id::text || ':' || NEW.kind || ':' || NEW.canonical_id, NEW.occurred_at);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER contributions_wacp AFTER INSERT ON contributions
    FOR EACH ROW EXECUTE FUNCTION record_contribution_wacp();

CREATE OR REPLACE FUNCTION rebuild_project_collaboration_events() RETURNS bigint LANGUAGE plpgsql AS $$
DECLARE inserted bigint;
BEGIN
    INSERT INTO project_collaboration_events(college_id, project_id, actor_id, source_type, source_key, occurred_at)
    SELECT u.college_id, u.project_id, u.author_id, 'project_update', u.id::text, u.created_at
    FROM project_updates u JOIN project_members m ON m.project_id = u.project_id AND m.user_id = u.author_id AND m.college_id = u.college_id
    UNION ALL
    SELECT d.college_id, d.project_id, d.author_id, 'discussion', d.id::text, d.created_at
    FROM project_discussions d JOIN project_members m ON m.project_id = d.project_id AND m.user_id = d.author_id AND m.college_id = d.college_id
    UNION ALL
    SELECT c.college_id, c.project_id, c.author_id, 'comment', c.id::text, c.created_at
    FROM project_comments c JOIN project_members m ON m.project_id = c.project_id AND m.user_id = c.author_id AND m.college_id = c.college_id
    UNION ALL
    SELECT e.college_id, e.project_id, e.actor_id, 'task_transition', e.id::text, e.created_at
    FROM project_task_events e JOIN project_members m ON m.project_id = e.project_id AND m.user_id = e.actor_id AND m.college_id = e.college_id
    WHERE e.event_type = 'status_changed'
    UNION ALL
    SELECT a.college_id, r.project_id, a.decided_by, 'application_accepted', a.id::text, a.decided_at
    FROM applications a JOIN project_roles r ON r.id = a.project_role_id AND r.college_id = a.college_id
    JOIN project_members m ON m.project_id = r.project_id AND m.user_id = a.decided_by AND m.college_id = a.college_id
    WHERE a.status = 'accepted' AND a.decided_at IS NOT NULL
    UNION ALL
    SELECT m.college_id, m.project_id, m.updated_by, 'milestone_completed', m.id::text || ':' || m.version::text, m.updated_at
    FROM project_milestones m JOIN project_members pm ON pm.project_id = m.project_id AND pm.user_id = m.updated_by AND pm.college_id = m.college_id
    WHERE m.status = 'completed' AND m.updated_by IS NOT NULL
    UNION ALL
    SELECT c.college_id, r.project_id, ui.user_id, c.kind, r.id::text || ':' || c.kind || ':' || c.canonical_id, c.occurred_at
    FROM contributions c JOIN repositories r ON r.id = c.repository_id AND r.college_id = c.college_id
    JOIN user_identities ui ON ui.provider = 'github' AND ui.subject = c.github_author_id::text
    JOIN project_members pm ON pm.project_id = r.project_id AND pm.user_id = ui.user_id AND pm.college_id = c.college_id
    WHERE c.occurred_at IS NOT NULL AND c.kind IN ('commit', 'pull_request', 'issue')
    ON CONFLICT (college_id, source_type, source_key) DO NOTHING;
    GET DIAGNOSTICS inserted = ROW_COUNT;
    RETURN inserted;
END $$;

SELECT rebuild_project_collaboration_events();

CREATE OR REPLACE VIEW weekly_active_collaborative_projects AS
SELECT college_id, count(*)::bigint AS project_count
FROM (
    SELECT e.college_id, e.project_id
    FROM project_collaboration_events e
    JOIN project_members pm ON pm.college_id=e.college_id AND pm.project_id=e.project_id AND pm.user_id=e.actor_id
    JOIN projects p ON p.college_id=e.college_id AND p.id=e.project_id
    WHERE e.occurred_at >= statement_timestamp() - interval '7 days'
      AND e.occurred_at <= statement_timestamp()
      AND p.lifecycle='active' AND p.github_revoke_deadline IS NULL
    GROUP BY e.college_id, e.project_id
    HAVING count(DISTINCT e.actor_id) >= 2
) active_projects
GROUP BY college_id;
