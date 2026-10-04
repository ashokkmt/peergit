# Identity, campus setup, and profile media

Phase 2 uses GitHub App user authorization, PostgreSQL sessions and campus verification, and the local S3-compatible object service for profile images. Configure local values in the ignored `.env`; use separate GitHub App credentials and secrets for staging and production. See [GitHub OAuth setup](../plans/setup-github-oauth.md) for App registration and callback setup.

## GitHub sign-in

Create a GitHub App with user authorization enabled and read-only Email addresses access. Register the exact callback URL for each environment. Local Caddy serves the app at `https://localhost`, so use:

```text
Callback URL: https://localhost/api/v1/auth/github/callback
```

Set `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, and `GITHUB_REDIRECT_URL` in `.env`. Use `APP_ORIGIN=https://localhost` and keep `COOKIE_SECURE=true` with Caddy TLS. Leave all three GitHub values blank together to run without sign-in configured. PeerGit identifies accounts by GitHub's numeric ID and obtains the verified primary address from the authenticated email API; it never grants campus membership from the GitHub email domain.

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

The first administrator creates an account with GitHub and proves campus affiliation through the campus-email challenge. If no administrator exists yet, an authorized institution operator verifies the person's identity out of band, then performs the one-time audited bootstrap below using that account's PeerGit ID. This bootstrap procedure is distinct from normal self-service role APIs, which cannot grant campus administrator:

```sql
BEGIN;
WITH target AS (
  SELECT u.id AS user_id,'<college-id>'::uuid AS college_id
  FROM users u
  WHERE u.id='<verified-user-id>' AND u.status='active'
    AND u.account_type='unverified' AND u.college_id IS NULL
  FOR UPDATE
), grant_role AS (
  UPDATE users SET college_id=(SELECT college_id FROM target),account_type='campus',updated_at=now()
  WHERE id=(SELECT user_id FROM target)
  RETURNING id,college_id
), proof AS (
  INSERT INTO campus_verifications(college_id,user_id,source,verified_by)
  SELECT college_id,id,'administrator_review',id FROM grant_role
  RETURNING college_id,user_id
), student_role AS (
  INSERT INTO campus_roles(college_id,user_id,role)
  SELECT college_id,user_id,'student' FROM proof ON CONFLICT DO NOTHING
  RETURNING college_id,user_id
), grant_admin AS (
  INSERT INTO campus_roles(college_id,user_id,role)
  SELECT college_id,user_id,'campus_admin' FROM student_role
  ON CONFLICT DO NOTHING
  RETURNING college_id,user_id
)
INSERT INTO audit_log(tenant_id,actor_id,action,resource_type,resource_id,details)
SELECT college_id,user_id,'campus_admin.bootstrapped','user',user_id,
       '{"reason":"institution-approved initial administrator bootstrap; mailbox proof recorded separately if performed"}'::jsonb
FROM grant_admin;
COMMIT;
```

After sign-in, the administrator sets up TOTP MFA and saves the one-time recovery codes. Administrative APIs require a fresh MFA verification. Regular campus administrators can create campus invitations, scoped external invitations, organizations, grant non-admin campus roles, and suspend/reactivate non-admin accounts. Suspending an account revokes its sessions immediately. Administrator suspension/promotion requires the separately documented break-glass operator procedure.

Campus verification email delivers the expiring link and OTP through the PostgreSQL-backed worker. Administrative approval is audited and does not claim mailbox ownership. Email-bound invitations still require GitHub identity matching and do not bypass campus verification; external grants remain scoped. Production delivery requires an approved authenticated-TLS email provider.

## Local object storage and image rules

The local Compose profile already provides SeaweedFS. The example `.env` points the API at its local S3 endpoint. Profile images are limited to 5 MiB compressed and 20 megapixels, PNG or JPEG only. PeerGit decodes and re-encodes them before storage, which strips metadata and rejects active formats such as SVG. Image objects are private by default. Campus-visible delivery requires current profile-discovery consent and a same-campus session; consent revocation denies subsequent downloads.

No general file, document, or executable uploads are accepted in Phase 2. Production object storage, malware controls for any later broader media types, retention, independent backup, and recovery qualification remain release gates.

The Terms and Privacy screens are marked `draft-1`; their presence is an onboarding implementation, not legal approval. Replace and version the policy text only after institutional review. A new policy version requires fresh user acceptance before campus administration and media features are available.
