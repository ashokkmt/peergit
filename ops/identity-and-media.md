# Identity, campus setup, and profile media

Phase 2 uses Google OpenID Connect, PostgreSQL sessions and invitations, and the local S3-compatible object service for profile images. Configure local values in the ignored `.env`; use separate credentials for staging and production.

## Google sign-in

Create a Google OAuth **Web application** client. Register the exact JavaScript origin and callback URL for each environment. Local Caddy serves the app at `https://localhost`, so use:

```text
Authorized JavaScript origin: https://localhost
Authorized redirect URI:     https://localhost/api/v1/auth/callback
```

Set `GOOGLE_OIDC_ISSUER=https://accounts.google.com`, the client ID, client secret, and the matching redirect URL in `.env`. `APP_ORIGIN` must be the browser-visible origin. Keep `COOKIE_SECURE=true` when using Caddy TLS. Leave all Google OIDC values blank together to run the application without sign-in configured; sign-in then returns a clear unavailable response.

`SESSION_HASH_KEY` and `MFA_ENCRYPTION_KEY` must each be unique random values of at least 32 bytes in staging and production. For example, generate each independently with `openssl rand -base64 48`. Do not use the development examples outside a local environment. Session values and MFA seeds are stored as keyed hashes or authenticated ciphertext; do not rotate these keys without a planned session invalidation and MFA re-enrollment procedure.

## First campus and first administrator

The first campus and its verified email domain are provisioned by an operator after confirming domain ownership with the institution. Use the PostgreSQL migration database and a transaction; replace the example values:

```sql
BEGIN;
INSERT INTO colleges(slug,name) VALUES ('example-campus','Example Campus') RETURNING id;
-- Use the returned college ID in the next statement.
INSERT INTO college_domains(college_id,domain,verified_at)
VALUES ('<college-id>','example.edu',now());
COMMIT;
```

The first administrator signs in with a Google account whose verified email matches the configured domain. Then grant the initial campus administrator role in a recorded operator session. This is a one-time bootstrap because the public role endpoint deliberately cannot promote accounts to campus administrator:

```sql
BEGIN;
WITH target AS (
  SELECT u.id AS user_id,u.college_id
  FROM users u JOIN college_domains d ON d.college_id=u.college_id
  WHERE u.email_normalized='admin@example.edu'
    AND d.domain='example.edu' AND d.verified_at IS NOT NULL
    AND u.status='active'
  FOR UPDATE
), grant_role AS (
  INSERT INTO campus_roles(college_id,user_id,role)
  SELECT college_id,user_id,'campus_admin' FROM target
  ON CONFLICT DO NOTHING
  RETURNING college_id,user_id
)
INSERT INTO audit_log(tenant_id,actor_id,action,resource_type,resource_id,details)
SELECT college_id,user_id,'campus_admin.bootstrapped','user',user_id,
       '{"reason":"initial campus administrator bootstrap"}'::jsonb
FROM grant_role;
COMMIT;
```

After sign-in, the administrator sets up TOTP MFA and saves the one-time recovery codes. Administrative APIs require a fresh MFA verification. Regular campus administrators can create campus invitations, scoped external invitations, organizations, grant non-admin campus roles, and suspend/reactivate non-admin accounts. Suspending an account revokes its sessions immediately. Administrator suspension/promotion requires the separately documented break-glass operator procedure.

Invitations are email-bound, single-use links that expire within seven days. The current screen returns the link once for the administrator to share through an approved channel; it is never logged or stored in plaintext by PeerGit. Production invitation delivery through the transactional email provider remains an operational integration gate.

## Local object storage and image rules

The local Compose profile already provides SeaweedFS. The example `.env` points the API at its local S3 endpoint. Profile images are limited to 5 MiB compressed and 20 megapixels, PNG or JPEG only. PeerGit decodes and re-encodes them before storage, which strips metadata and rejects active formats such as SVG. Image objects are private by default. Campus-visible delivery requires current profile-discovery consent and a same-campus session; consent revocation denies subsequent downloads.

No general file, document, or executable uploads are accepted in Phase 2. Production object storage, malware controls for any later broader media types, retention, independent backup, and recovery qualification remain release gates.

The Terms and Privacy screens are marked `draft-1`; their presence is an onboarding implementation, not legal approval. Replace and version the policy text only after institutional review. A new policy version requires fresh user acceptance before campus administration and media features are available.
