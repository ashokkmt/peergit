CREATE TABLE notification_cursors (
    college_id uuid NOT NULL,
    user_id uuid NOT NULL,
    last_sequence bigint NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
    PRIMARY KEY (college_id, user_id),
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);

CREATE TABLE notification_preferences (
    college_id uuid NOT NULL,
    user_id uuid NOT NULL,
    category text NOT NULL CHECK (category IN ('applications','project_activity')),
    email_enabled boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (college_id, user_id, category),
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX applications_id_college_uidx ON applications(id, college_id);

CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    user_id uuid NOT NULL,
    source_event_id uuid NOT NULL REFERENCES outbox_events(id) ON DELETE RESTRICT,
    sequence bigint NOT NULL CHECK (sequence > 0),
    category text NOT NULL CHECK (category IN ('applications','project_activity')),
    event_type text NOT NULL,
    title text NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
    body text NOT NULL CHECK (length(body) BETWEEN 1 AND 240),
    project_id uuid,
    application_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    read_at timestamptz,
    UNIQUE (source_event_id, user_id),
    UNIQUE (college_id, user_id, sequence),
    UNIQUE (id, college_id, user_id),
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (application_id, college_id) REFERENCES applications(id, college_id) ON DELETE CASCADE,
    CHECK ((category='applications' AND application_id IS NOT NULL) OR category='project_activity')
);
CREATE INDEX notifications_inbox_idx ON notifications(college_id,user_id,sequence DESC);
CREATE INDEX notifications_unread_idx ON notifications(college_id,user_id,sequence DESC) WHERE read_at IS NULL;

CREATE TABLE notification_email_deliveries (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    notification_id uuid NOT NULL UNIQUE REFERENCES notifications(id) ON DELETE CASCADE,
    college_id uuid NOT NULL,
    user_id uuid NOT NULL,
    provider text CHECK (provider IN ('smtp','resend')),
    provider_message_id text,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','accepted','delivered','bounced','failed','suppressed')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 3),
    status_at timestamptz NOT NULL DEFAULT now(),
    last_error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (notification_id, college_id, user_id) REFERENCES notifications(id, college_id, user_id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX notification_email_provider_message_uidx ON notification_email_deliveries(provider,provider_message_id) WHERE provider_message_id IS NOT NULL;

CREATE TABLE notification_email_attempts (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    delivery_id uuid NOT NULL REFERENCES notification_email_deliveries(id) ON DELETE CASCADE,
    attempt integer NOT NULL CHECK (attempt BETWEEN 1 AND 3),
    provider text NOT NULL CHECK (provider IN ('smtp','resend')),
    result text NOT NULL CHECK (result IN ('started','accepted','failed','unknown','suppressed')),
    provider_message_id text,
    error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (delivery_id, attempt)
);

CREATE TABLE notification_stream_capacity (
    id smallint PRIMARY KEY CHECK (id=1),
    total_limit integer NOT NULL CHECK (total_limit BETWEEN 1 AND 1000),
    per_user_limit integer NOT NULL CHECK (per_user_limit BETWEEN 1 AND 10)
);
INSERT INTO notification_stream_capacity(id,total_limit,per_user_limit) VALUES(1,250,2);

CREATE TABLE notification_stream_leases (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    user_id uuid NOT NULL,
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX notification_stream_leases_expiry_idx ON notification_stream_leases(expires_at);

CREATE OR REPLACE FUNCTION cleanup_notifications() RETURNS bigint LANGUAGE plpgsql AS $$
DECLARE removed bigint;
BEGIN
    DELETE FROM notifications WHERE created_at < clock_timestamp() - interval '90 days';
    GET DIAGNOSTICS removed = ROW_COUNT;
    RETURN removed;
END $$;
