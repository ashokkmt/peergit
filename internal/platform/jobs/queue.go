package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrConflict       = errors.New("idempotency key has a different request hash")
	ErrDedupeConflict = errors.New("job deduplication key has different work")
	ErrStale          = errors.New("job lease no longer belongs to this attempt")
)

type Queue struct{ Pool *pgxpool.Pool }
type Claim struct {
	ID                  string
	Worker              string
	TenantID, ActorID   string
	Generation, Attempt int
	Type                string
	Payload             json.RawMessage
}

func (q Queue) Enqueue(ctx context.Context, tenant, actor, key, hash string) (string, error) {
	return q.EnqueueJob(ctx, tenant, actor, "snapshot", key, hash, "snapshot", json.RawMessage(`{}`))
}

// EnqueueJob preserves the original queue API; new request handlers should use
// idempotency.Reserve, EnqueueTx, and idempotency.Complete in one transaction.
func (q Queue) EnqueueJob(ctx context.Context, tenant, actor, operation, key, hash, jobType string, payload json.RawMessage) (string, error) {
	if tenant == "" || actor == "" || operation == "" || key == "" || hash == "" || jobType == "" || len(payload) > 1<<20 || !json.Valid(payload) {
		return "", errors.New("tenant, actor, operation, idempotency key, request hash, job type, and valid payload are required")
	}
	hashBytes, err := hex.DecodeString(hash)
	if err != nil || len(hashBytes) != sha256.Size {
		return "", errors.New("request hash must be a SHA-256 hex digest")
	}
	tx, err := q.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id, storedHash string
	err = tx.QueryRow(ctx, `INSERT INTO request_idempotency (tenant_id,actor_id,operation,idempotency_key,request_hash,expires_at)
	 VALUES ($1,$2,$3,$4,$5,clock_timestamp()+interval '24 hours')
	 ON CONFLICT (tenant_id,actor_id,operation,idempotency_key) DO NOTHING RETURNING id::text,encode(request_hash,'hex')`, tenant, actor, operation, key, hashBytes).Scan(&id, &storedHash)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id::text,encode(request_hash,'hex') FROM request_idempotency
		 WHERE tenant_id=$1 AND actor_id=$2 AND operation=$3 AND idempotency_key=$4 FOR UPDATE`, tenant, actor, operation, key).Scan(&id, &storedHash)
	}
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(storedHash, hash) {
		return "", ErrConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO jobs (tenant_id,actor_id,idempotency_id,job_type,dedupe_key,payload)
	 VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (tenant_id,job_type,dedupe_key) DO NOTHING`, tenant, actor, id, jobType, key, payload)
	if err != nil {
		return "", err
	}
	var jobID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM jobs WHERE idempotency_id=$1 OR (tenant_id=$2 AND job_type=$3 AND dedupe_key=$4) LIMIT 1`, id, tenant, jobType, key).Scan(&jobID); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return jobID, nil
}

func (q Queue) EnqueueWork(ctx context.Context, tenant, actor, jobType, dedupeKey string, payload json.RawMessage, deadline *time.Time) (string, error) {
	tx, err := q.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	id, err := EnqueueTx(ctx, tx, tenant, actor, jobType, dedupeKey, payload, deadline)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// EnqueueTx keeps job creation in the caller's domain/idempotency transaction.
func EnqueueTx(ctx context.Context, tx pgx.Tx, tenant, actor, jobType, dedupeKey string, payload json.RawMessage, deadline *time.Time) (string, error) {
	if tenant == "" || actor == "" || jobType == "" || dedupeKey == "" || len(dedupeKey) > 200 || len(jobType) > 120 || len(payload) > 1<<20 || !json.Valid(payload) {
		return "", errors.New("tenant, actor, job type, bounded deduplication key, and valid payload are required")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO jobs(tenant_id,actor_id,job_type,dedupe_key,payload,deadline_at)
		VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id,job_type,dedupe_key) DO NOTHING`, tenant, actor, jobType, dedupeKey, payload, deadline); err != nil {
		return "", err
	}
	var id string
	var matches bool
	err := tx.QueryRow(ctx, `SELECT id::text,actor_id=$4 AND payload=$5::jsonb AND deadline_at IS NOT DISTINCT FROM $6
		FROM jobs WHERE tenant_id=$1 AND job_type=$2 AND dedupe_key=$3`, tenant, jobType, dedupeKey, actor, payload, deadline).Scan(&id, &matches)
	if err == nil && !matches {
		return "", ErrDedupeConflict
	}
	return id, err
}

func (q Queue) Claim(ctx context.Context, worker string, lease time.Duration) (Claim, error) {
	return q.ClaimTypes(ctx, worker, lease, nil)
}

func (q Queue) ClaimTypes(ctx context.Context, worker string, lease time.Duration, types []string) (Claim, error) {
	if worker == "" || lease <= 0 {
		return Claim{}, errors.New("worker and positive lease are required")
	}
	// A single statement holds row locks only during the claim; external work follows outside it.
	var c Claim
	c.Worker = worker
	err := q.Pool.QueryRow(ctx, `WITH next AS (
	 SELECT id FROM jobs WHERE attempts<3 AND available_at<=clock_timestamp()
	 AND (state='ready' OR (state='running' AND lease_until<=clock_timestamp()))
	 AND (deadline_at IS NULL OR deadline_at>clock_timestamp())
	 AND ($3::text[] IS NULL OR job_type=ANY($3))
	 ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT 1)
	 UPDATE jobs SET state='running',worker=$1,generation=generation+1,attempts=attempts+1,
	 lease_until=clock_timestamp()+$2::bigint*interval '1 millisecond',updated_at=clock_timestamp()
	 FROM next WHERE jobs.id=next.id RETURNING jobs.id::text,tenant_id::text,actor_id::text,generation,attempts,job_type,payload`, worker, lease.Milliseconds(), types).Scan(&c.ID, &c.TenantID, &c.ActorID, &c.Generation, &c.Attempt, &c.Type, &c.Payload)
	return c, err
}

func (q Queue) Heartbeat(ctx context.Context, c Claim, lease time.Duration) error {
	if lease <= 0 {
		return errors.New("heartbeat lease must be positive")
	}
	tag, err := q.Pool.Exec(ctx, `UPDATE jobs SET lease_until=clock_timestamp()+$4::bigint*interval '1 millisecond'
	 WHERE id=$1 AND worker=$2 AND generation=$3 AND state='running' AND lease_until>clock_timestamp()`, c.ID, c.Worker, c.Generation, lease.Milliseconds())
	if err == nil && tag.RowsAffected() != 1 {
		return ErrStale
	}
	return err
}

func (q Queue) Complete(ctx context.Context, c Claim, objectKey string) error {
	tag, err := q.Pool.Exec(ctx, `UPDATE jobs SET state='done',object_key=$4,lease_until=NULL,worker=NULL
	 WHERE id=$1 AND worker=$2 AND generation=$3 AND state='running' AND lease_until>clock_timestamp()`, c.ID, c.Worker, c.Generation, objectKey)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrStale
	}
	return err
}

func (q Queue) Retry(ctx context.Context, c Claim, delay time.Duration) error {
	return q.RetryWithCode(ctx, c, delay, "retry_requested")
}

func (q Queue) RetryWithCode(ctx context.Context, c Claim, delay time.Duration, errorCode string) error {
	if delay < 0 || !safeCode(errorCode) {
		return errors.New("retry delay must be non-negative and error code is required")
	}
	tag, err := q.Pool.Exec(ctx, `UPDATE jobs SET state=CASE WHEN attempts>=3 THEN 'dead' ELSE 'ready' END,
	 available_at=clock_timestamp()+$4::bigint*interval '1 millisecond',lease_until=NULL,worker=NULL,last_error_code=$5,updated_at=clock_timestamp()
	 WHERE id=$1 AND worker=$2 AND generation=$3 AND state='running' AND lease_until>clock_timestamp()`, c.ID, c.Worker, c.Generation, delay.Milliseconds(), errorCode)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrStale
	}
	return err
}

func (q Queue) RecoverExhausted(ctx context.Context) error {
	_, err := q.Pool.Exec(ctx, `UPDATE jobs SET state='dead',lease_until=NULL,worker=NULL,generation=generation+1,updated_at=clock_timestamp()
	 WHERE (state='running' AND attempts>=3 AND lease_until<=clock_timestamp())
	 OR (state IN ('ready','running') AND deadline_at<=clock_timestamp())`)
	return err
}

func (q Queue) RequeueDead(ctx context.Context, id string) error {
	tag, err := q.Pool.Exec(ctx, `UPDATE jobs SET state='ready',attempts=0,generation=generation+1,
	 available_at=clock_timestamp(),last_error_code=NULL,updated_at=clock_timestamp()
		 WHERE id=$1 AND state='dead' AND (deadline_at IS NULL OR deadline_at>clock_timestamp())`, id)
	if err == nil && tag.RowsAffected() != 1 {
		return errors.New("dead job was not found or its deadline has expired")
	}
	return err
}

func (q Queue) ListDead(ctx context.Context, limit int) ([]Claim, error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("dead job limit must be between 1 and 100")
	}
	rows, err := q.Pool.Query(ctx, `SELECT id::text,job_type,attempts,generation FROM jobs WHERE state='dead' ORDER BY created_at,id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []Claim
	for rows.Next() {
		var job Claim
		if err := rows.Scan(&job.ID, &job.Type, &job.Attempt, &job.Generation); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func IsEmpty(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func safeCode(code string) bool {
	if len(code) < 1 || len(code) > 100 {
		return false
	}
	for _, r := range code {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r)) {
			return false
		}
	}
	return true
}
