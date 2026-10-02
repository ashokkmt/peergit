package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Event struct {
	TenantID, AggregateType, EventType string
	AggregateID                        string
	Version                            int64
	Payload                            json.RawMessage
}
type Delivery struct {
	EventID, Handler, Worker                        string
	TenantID, AggregateType, AggregateID, EventType string
	Generation, Attempt                             int
	Version                                         int64
	Payload                                         json.RawMessage
}
type DeadDelivery struct {
	EventID, Handler, LastErrorCode string
	Attempts                        int
}
type Queue struct{ Pool *pgxpool.Pool }

// Append is called inside the same transaction as the domain write.
func Append(ctx context.Context, tx pgx.Tx, event Event, handlers []string) error {
	if event.TenantID == "" || event.AggregateType == "" || event.AggregateID == "" || event.EventType == "" || event.Version < 1 || len(event.Payload) > 1<<20 || !json.Valid(event.Payload) {
		return errors.New("invalid outbox event")
	}
	if len(handlers) == 0 {
		return errors.New("at least one outbox handler is required")
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO outbox_events(tenant_id,aggregate_type,aggregate_id,event_type,aggregate_version,payload)
	 VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, event.TenantID, event.AggregateType, event.AggregateID, event.EventType, event.Version, event.Payload).Scan(&id); err != nil {
		return err
	}
	for _, handler := range handlers {
		if handler == "" {
			return errors.New("outbox handler name is required")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO outbox_deliveries(event_id,handler) VALUES($1,$2)`, id, handler); err != nil {
			return err
		}
	}
	return nil
}

func (q Queue) Claim(ctx context.Context, handler, worker string, lease time.Duration) (Delivery, error) {
	if handler == "" || worker == "" || lease <= 0 {
		return Delivery{}, errors.New("handler, worker, and positive lease are required")
	}
	var delivery Delivery
	delivery.Handler, delivery.Worker = handler, worker
	err := q.Pool.QueryRow(ctx, `WITH next AS (
	 SELECT d.event_id,d.handler FROM outbox_deliveries d
	 WHERE d.handler=$1 AND d.attempts<3 AND d.available_at<=clock_timestamp()
	 AND (d.state='ready' OR (d.state='running' AND d.lease_until<=clock_timestamp()))
	 ORDER BY d.available_at,d.event_id FOR UPDATE SKIP LOCKED LIMIT 1)
	 UPDATE outbox_deliveries d SET state='running',worker=$2,generation=d.generation+1,attempts=d.attempts+1,
	 lease_until=clock_timestamp()+$3::bigint*interval '1 millisecond'
	 FROM next JOIN outbox_events e ON e.id=next.event_id
	 WHERE d.event_id=next.event_id AND d.handler=next.handler
		 RETURNING d.event_id::text,e.tenant_id::text,e.aggregate_type,e.aggregate_id::text,e.event_type,e.aggregate_version,
		 d.generation,d.attempts,e.payload`, handler, worker, lease.Milliseconds()).Scan(
		&delivery.EventID, &delivery.TenantID, &delivery.AggregateType, &delivery.AggregateID,
		&delivery.EventType, &delivery.Version, &delivery.Generation, &delivery.Attempt, &delivery.Payload)
	return delivery, err
}

func (q Queue) Complete(ctx context.Context, d Delivery) error {
	tag, err := q.Pool.Exec(ctx, `UPDATE outbox_deliveries SET state='done',worker=NULL,lease_until=NULL
	 WHERE event_id=$1 AND handler=$2 AND worker=$3 AND generation=$4 AND state='running' AND lease_until>clock_timestamp()`, d.EventID, d.Handler, d.Worker, d.Generation)
	if err == nil && tag.RowsAffected() != 1 {
		return errors.New("outbox delivery lease is stale")
	}
	return err
}

func (q Queue) Retry(ctx context.Context, d Delivery, delay time.Duration, errorCode string) error {
	if delay < 0 || !safeCode(errorCode) {
		return errors.New("non-negative delay and safe error code are required")
	}
	tag, err := q.Pool.Exec(ctx, `UPDATE outbox_deliveries SET state=CASE WHEN attempts>=3 THEN 'dead' ELSE 'ready' END,
	 available_at=clock_timestamp()+$5::bigint*interval '1 millisecond',worker=NULL,lease_until=NULL,last_error_code=$6
	 WHERE event_id=$1 AND handler=$2 AND worker=$3 AND generation=$4 AND state='running' AND lease_until>clock_timestamp()`, d.EventID, d.Handler, d.Worker, d.Generation, delay.Milliseconds(), errorCode)
	if err == nil && tag.RowsAffected() != 1 {
		return errors.New("outbox delivery lease is stale")
	}
	return err
}

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

func (q Queue) RecoverExhausted(ctx context.Context) error {
	_, err := q.Pool.Exec(ctx, `UPDATE outbox_deliveries SET state='dead',worker=NULL,lease_until=NULL
	 WHERE state='running' AND attempts>=3 AND lease_until<=clock_timestamp()`)
	return err
}

func (q Queue) ListDead(ctx context.Context, limit int) ([]DeadDelivery, error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("dead delivery limit must be between 1 and 100")
	}
	rows, err := q.Pool.Query(ctx, `SELECT event_id::text,handler,attempts,COALESCE(last_error_code,'')
		FROM outbox_deliveries WHERE state='dead' ORDER BY available_at,event_id,handler LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dead []DeadDelivery
	for rows.Next() {
		var delivery DeadDelivery
		if err := rows.Scan(&delivery.EventID, &delivery.Handler, &delivery.Attempts, &delivery.LastErrorCode); err != nil {
			return nil, err
		}
		dead = append(dead, delivery)
	}
	return dead, rows.Err()
}

func (q Queue) RequeueDead(ctx context.Context, eventID, handler string) error {
	if eventID == "" || handler == "" {
		return errors.New("event ID and handler are required")
	}
	tag, err := q.Pool.Exec(ctx, `UPDATE outbox_deliveries SET state='ready',attempts=0,generation=generation+1,
		available_at=clock_timestamp(),worker=NULL,lease_until=NULL,last_error_code=NULL
		WHERE event_id=$1 AND handler=$2 AND state='dead'`, eventID, handler)
	if err == nil && tag.RowsAffected() != 1 {
		return errors.New("dead outbox delivery was not found")
	}
	return err
}
