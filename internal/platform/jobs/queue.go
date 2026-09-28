package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrConflict = errors.New("idempotency key has a different request hash")
	ErrStale    = errors.New("job lease no longer belongs to this attempt")
)

type Queue struct{ Pool *pgxpool.Pool }
type Claim struct {
	ID                  string
	Worker              string
	Generation, Attempt int
}

func (q Queue) Enqueue(ctx context.Context, tenant, actor, key, hash string) (string, error) {
	tx, err := q.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id, storedHash string
	err = tx.QueryRow(ctx, `INSERT INTO receipts (tenant_id,actor_id,operation,idempotency_key,request_hash)
	 VALUES ($1,$2,'snapshot',$3,$4) ON CONFLICT (tenant_id,actor_id,operation,idempotency_key)
	 DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key RETURNING id::text,request_hash`, tenant, actor, key, hash).Scan(&id, &storedHash)
	if err != nil {
		return "", err
	}
	if storedHash != hash {
		return "", ErrConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO jobs (receipt_id) VALUES ($1) ON CONFLICT (receipt_id) DO NOTHING`, id)
	if err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

func (q Queue) Claim(ctx context.Context, worker string, lease time.Duration) (Claim, error) {
	if worker == "" || lease <= 0 {
		return Claim{}, errors.New("worker and positive lease are required")
	}
	// A single statement holds row locks only during the claim; external work follows outside it.
	var c Claim
	c.Worker = worker
	err := q.Pool.QueryRow(ctx, `WITH next AS (
	 SELECT id FROM jobs WHERE attempts<3 AND available_at<=clock_timestamp()
	 AND (state='ready' OR (state='running' AND lease_until<=clock_timestamp()))
	 ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT 1)
	 UPDATE jobs SET state='running',worker=$1,generation=generation+1,attempts=attempts+1,
	 lease_until=clock_timestamp()+$2::bigint*interval '1 millisecond'
	 FROM next WHERE jobs.id=next.id RETURNING jobs.id::text,generation,attempts`, worker, lease.Milliseconds()).Scan(&c.ID, &c.Generation, &c.Attempt)
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
	if delay < 0 {
		return errors.New("retry delay must not be negative")
	}
	tag, err := q.Pool.Exec(ctx, `UPDATE jobs SET state=CASE WHEN attempts>=3 THEN 'dead' ELSE 'ready' END,
	 available_at=clock_timestamp()+$4::bigint*interval '1 millisecond',lease_until=NULL,worker=NULL
	 WHERE id=$1 AND worker=$2 AND generation=$3 AND state='running' AND lease_until>clock_timestamp()`, c.ID, c.Worker, c.Generation, delay.Milliseconds())
	if err == nil && tag.RowsAffected() != 1 {
		return ErrStale
	}
	return err
}

func (q Queue) RecoverExhausted(ctx context.Context) error {
	_, err := q.Pool.Exec(ctx, `UPDATE jobs SET state='dead',lease_until=NULL,worker=NULL
	 WHERE state='running' AND attempts>=3 AND lease_until<=clock_timestamp()`)
	return err
}

func IsEmpty(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
