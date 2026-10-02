CREATE TABLE request_idempotency (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    operation text NOT NULL,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 200),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    response_status smallint,
    response_body jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    UNIQUE (tenant_id, actor_id, operation, idempotency_key),
    CHECK ((response_status IS NULL) = (response_body IS NULL))
);

CREATE TABLE jobs (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    idempotency_id uuid NOT NULL UNIQUE REFERENCES request_idempotency(id),
    job_type text NOT NULL CHECK (length(job_type) BETWEEN 1 AND 120),
    dedupe_key text NOT NULL CHECK (length(dedupe_key) BETWEEN 1 AND 200),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    state text NOT NULL DEFAULT 'ready' CHECK (state IN ('ready','running','done','dead')),
    available_at timestamptz NOT NULL DEFAULT now(),
    attempts smallint NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 3),
    generation bigint NOT NULL DEFAULT 0 CHECK (generation >= 0),
    worker text,
    lease_until timestamptz,
    object_key text,
    last_error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, job_type, dedupe_key),
    CHECK ((state = 'running' AND worker IS NOT NULL AND lease_until IS NOT NULL) OR
           (state <> 'running' AND worker IS NULL AND lease_until IS NULL)),
    CHECK ((state = 'done') = (object_key IS NOT NULL) OR job_type <> 'snapshot')
);
CREATE INDEX jobs_ready_idx ON jobs (available_at, id) WHERE state = 'ready';
CREATE INDEX jobs_expired_idx ON jobs (lease_until, id) WHERE state = 'running';
CREATE INDEX jobs_dead_idx ON jobs (created_at, id) WHERE state = 'dead';

CREATE TABLE outbox_events (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    event_type text NOT NULL,
    aggregate_version bigint NOT NULL CHECK (aggregate_version > 0),
    payload jsonb NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, aggregate_type, aggregate_id, aggregate_version)
);

CREATE TABLE outbox_deliveries (
    event_id uuid NOT NULL REFERENCES outbox_events(id) ON DELETE RESTRICT,
    handler text NOT NULL,
    state text NOT NULL DEFAULT 'ready' CHECK (state IN ('ready','running','done','dead')),
    available_at timestamptz NOT NULL DEFAULT now(),
    attempts smallint NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 3),
    generation bigint NOT NULL DEFAULT 0 CHECK (generation >= 0),
    worker text,
    lease_until timestamptz,
    last_error_code text,
    PRIMARY KEY (event_id, handler),
    CHECK ((state = 'running' AND worker IS NOT NULL AND lease_until IS NOT NULL) OR
           (state <> 'running' AND worker IS NULL AND lease_until IS NULL))
);
CREATE INDEX outbox_deliveries_ready_idx ON outbox_deliveries (available_at, event_id) WHERE state = 'ready';
CREATE INDEX outbox_deliveries_expired_idx ON outbox_deliveries (lease_until, event_id) WHERE state = 'running';

CREATE TABLE audit_log (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid,
    request_id text,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_tenant_time_idx ON audit_log (tenant_id, occurred_at DESC, id DESC);

CREATE FUNCTION reject_audit_log_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only';
END;
$$;
CREATE TRIGGER audit_log_no_update_delete
    BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION reject_audit_log_change();
