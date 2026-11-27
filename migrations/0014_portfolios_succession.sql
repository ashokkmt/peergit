ALTER TABLE organizations
    ADD COLUMN owner_user_id uuid;

ALTER TABLE organizations
    ADD CONSTRAINT organizations_owner_user_fk
    FOREIGN KEY (owner_user_id, college_id) REFERENCES users(id, college_id);

UPDATE organizations o
SET owner_user_id = (
    SELECT om.user_id FROM organization_members om
    WHERE om.organization_id = o.id AND om.college_id = o.college_id
    ORDER BY CASE om.role WHEN 'manager' THEN 0 ELSE 1 END, om.created_at, om.user_id LIMIT 1
)
WHERE o.owner_user_id IS NULL AND EXISTS (
    SELECT 1 FROM organization_members om WHERE om.organization_id = o.id AND om.college_id = o.college_id
);

UPDATE organizations o
SET owner_user_id = (
    SELECT al.actor_id FROM audit_log al
    WHERE al.tenant_id = o.college_id
      AND al.resource_type = 'organization'
      AND al.resource_id = o.id
      AND al.action = 'organization.created'
    ORDER BY al.occurred_at, al.id
    LIMIT 1
)
WHERE o.owner_user_id IS NULL AND EXISTS (
    SELECT 1 FROM audit_log al
    WHERE al.tenant_id = o.college_id
      AND al.resource_type = 'organization'
      AND al.resource_id = o.id
      AND al.action = 'organization.created'
);

ALTER TABLE organization_members DROP CONSTRAINT organization_members_role_check;
ALTER TABLE organization_members ADD CONSTRAINT organization_members_role_check
    CHECK (role IN ('member', 'manager', 'owner'));

UPDATE organization_members om
SET role = 'owner'
FROM organizations o
WHERE o.id = om.organization_id AND o.college_id = om.college_id
  AND o.owner_user_id = om.user_id;

INSERT INTO organization_members(college_id, organization_id, user_id, role)
SELECT college_id, id, owner_user_id, 'owner'
FROM organizations
WHERE owner_user_id IS NOT NULL
ON CONFLICT (organization_id, user_id)
DO UPDATE SET role = 'owner';

CREATE TABLE portfolio_items (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    user_id uuid NOT NULL,
    project_id uuid NOT NULL,
    caption text NOT NULL DEFAULT '' CHECK (length(caption) <= 500),
    sort_order integer NOT NULL DEFAULT 0 CHECK (sort_order BETWEEN 0 AND 49),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, project_id),
    UNIQUE (id, user_id, college_id),
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, user_id) REFERENCES project_members(project_id, user_id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE
);
CREATE INDEX portfolio_items_owner_idx ON portfolio_items(college_id, user_id, sort_order, id);

CREATE TABLE portfolio_share_grants (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    user_id uuid NOT NULL,
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    audience text NOT NULL CHECK (audience IN ('public', 'campus')),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id, college_id) REFERENCES users(id, college_id) ON DELETE CASCADE
);
CREATE INDEX portfolio_share_grants_owner_idx
    ON portfolio_share_grants(college_id, user_id, created_at DESC);

CREATE TABLE handover_requests (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    college_id uuid NOT NULL,
    resource_type text NOT NULL CHECK (resource_type IN ('project', 'organization')),
    project_id uuid,
    organization_id uuid,
    from_user_id uuid NOT NULL,
    to_user_id uuid NOT NULL,
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'accepted', 'declined', 'cancelled', 'conflicted', 'expired')),
    provider_state text NOT NULL DEFAULT 'not_applicable' CHECK (provider_state IN ('not_applicable', 'upstream_unchanged', 'source_unavailable')),
    message text NOT NULL DEFAULT '' CHECK (length(message) <= 500),
    expires_at timestamptz NOT NULL DEFAULT now() + interval '7 days',
    responded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (from_user_id <> to_user_id),
    CHECK ((resource_type = 'project' AND project_id IS NOT NULL AND organization_id IS NULL) OR
           (resource_type = 'organization' AND organization_id IS NOT NULL AND project_id IS NULL)),
    FOREIGN KEY (project_id, college_id) REFERENCES projects(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, college_id) REFERENCES organizations(id, college_id) ON DELETE CASCADE,
    FOREIGN KEY (from_user_id, college_id) REFERENCES users(id, college_id),
    FOREIGN KEY (to_user_id, college_id) REFERENCES users(id, college_id)
);
CREATE UNIQUE INDEX handover_one_pending_project_idx
    ON handover_requests(college_id, project_id) WHERE state = 'pending' AND resource_type = 'project';
CREATE UNIQUE INDEX handover_one_pending_organization_idx
    ON handover_requests(college_id, organization_id) WHERE state = 'pending' AND resource_type = 'organization';
CREATE INDEX handover_recipient_idx ON handover_requests(college_id, to_user_id, state, created_at DESC);
