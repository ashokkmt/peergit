package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrConflict   = errors.New("idempotency key was used with a different request")
	ErrInProgress = errors.New("idempotent request has no committed response")
)

type Scope struct {
	TenantID, ActorID, Operation, Key string
}

type Result struct {
	Replay bool
	Status int
	Body   json.RawMessage
}

// Reserve and Complete must share the caller's transaction with the domain write.
func Reserve(ctx context.Context, tx pgx.Tx, scope Scope, requestHash [sha256.Size]byte) (Result, error) {
	if scope.TenantID == "" || scope.ActorID == "" || scope.Operation == "" || len(scope.Key) < 1 || len(scope.Key) > 200 {
		return Result{}, errors.New("tenant, actor, operation, and a 1-200 character idempotency key are required")
	}
	var status *int16
	var body []byte
	err := tx.QueryRow(ctx, `INSERT INTO request_idempotency
		(tenant_id,actor_id,operation,idempotency_key,request_hash,expires_at)
		VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '24 hours')
		ON CONFLICT(tenant_id,actor_id,operation,idempotency_key) DO UPDATE SET
		 request_hash=EXCLUDED.request_hash,response_status=NULL,response_body=NULL,
		 expires_at=EXCLUDED.expires_at,created_at=clock_timestamp()
		WHERE request_idempotency.expires_at<=clock_timestamp()
		RETURNING response_status,response_body`, scope.TenantID, scope.ActorID, scope.Operation, scope.Key, requestHash[:]).Scan(&status, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		var storedHash []byte
		err = tx.QueryRow(ctx, `SELECT request_hash,response_status,response_body FROM request_idempotency
			WHERE tenant_id=$1 AND actor_id=$2 AND operation=$3 AND idempotency_key=$4 FOR UPDATE`, scope.TenantID, scope.ActorID, scope.Operation, scope.Key).Scan(&storedHash, &status, &body)
		if err != nil {
			return Result{}, err
		}
		if string(storedHash) != string(requestHash[:]) {
			return Result{}, ErrConflict
		}
		if status == nil || body == nil {
			return Result{}, ErrInProgress
		}
		return Result{Replay: true, Status: int(*status), Body: body}, nil
	}
	if err != nil {
		return Result{}, err
	}
	return Result{}, nil
}

func Complete(ctx context.Context, tx pgx.Tx, scope Scope, status int, body json.RawMessage) error {
	if status < 200 || status > 599 || !json.Valid(body) || len(body) > 1<<20 {
		return errors.New("valid response status and JSON body up to 1 MiB are required")
	}
	tag, err := tx.Exec(ctx, `UPDATE request_idempotency SET response_status=$5,response_body=$6
		WHERE tenant_id=$1 AND actor_id=$2 AND operation=$3 AND idempotency_key=$4 AND response_status IS NULL`,
		scope.TenantID, scope.ActorID, scope.Operation, scope.Key, status, body)
	if err == nil && tag.RowsAffected() != 1 {
		return errors.New("idempotency reservation is missing or already completed")
	}
	return err
}

func RequestHash(body []byte) [sha256.Size]byte { return sha256.Sum256(body) }

func PurgeExpired(ctx context.Context, pool *pgxpool.Pool, limit int) (int64, error) {
	if limit < 1 || limit > 1000 {
		return 0, errors.New("idempotency cleanup limit must be between 1 and 1000")
	}
	tag, err := pool.Exec(ctx, `WITH expired AS (
		SELECT i.id FROM request_idempotency i
		WHERE i.expires_at<=clock_timestamp()
		AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.idempotency_id=i.id)
		ORDER BY i.expires_at,i.id LIMIT $1 FOR UPDATE OF i SKIP LOCKED
	)
	DELETE FROM request_idempotency i USING expired e WHERE i.id=e.id`, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func RunCleanup(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) {
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		count, err := PurgeExpired(cleanupCtx, pool, 1000)
		if err != nil {
			logger.ErrorContext(ctx, "idempotency cleanup failed", "error", err)
		} else if count > 0 {
			logger.InfoContext(ctx, "expired idempotency records removed", "count", count)
		}
	}
	cleanup()
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}
