package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"peergit/internal/platform/audit"
	"peergit/internal/platform/idempotency"
	"peergit/internal/platform/jobs"
	"peergit/internal/platform/outbox"
	"peergit/migrations"
)

func TestPlatformMigrationsAndDurableWork(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatalf("blank-to-head migrations: %v", err)
	}
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatalf("repeat migrations: %v", err)
	}
	var migrationCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&migrationCount); err != nil || migrationCount != 4 {
		t.Fatalf("migration history count=%d err=%v, want 4", migrationCount, err)
	}

	queue := jobs.Queue{Pool: pool}
	tenant, actor := "00000000-0000-7000-8000-000000000001", "00000000-0000-7000-8000-000000000002"
	id, err := queue.EnqueueWork(ctx, tenant, actor, "integration", "same-key", []byte(`{"n":1}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := queue.EnqueueWork(ctx, tenant, actor, "integration", "same-key", []byte(`{"n":1}`), nil)
	if err != nil || replay != id {
		t.Fatalf("same deduplication key returned %q err=%v, want %q", replay, err, id)
	}
	if _, err := queue.EnqueueWork(ctx, tenant, actor, "integration", "same-key", []byte(`{"n":2}`), nil); !errors.Is(err, jobs.ErrDedupeConflict) {
		t.Fatalf("same deduplication key with different payload error=%v, want conflict", err)
	}
	scope := idempotency.Scope{TenantID: tenant, ActorID: actor, Operation: "test-response", Key: "response-key"}
	responseHash := idempotency.RequestHash([]byte(`{"input":true}`))
	transaction, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := idempotency.Reserve(ctx, transaction, scope, responseHash); err != nil || result.Replay {
		t.Fatalf("first HTTP idempotency reservation = %#v, %v", result, err)
	}
	if err := idempotency.Complete(ctx, transaction, scope, 201, []byte(`{"id":"created"}`)); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	transaction, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := idempotency.Reserve(ctx, transaction, scope, responseHash)
	_ = transaction.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var storedBody map[string]string
	if err := json.Unmarshal(result.Body, &storedBody); err != nil {
		t.Fatal(err)
	}
	if !result.Replay || result.Status != 201 || storedBody["id"] != "created" {
		t.Fatalf("HTTP idempotency replay = %#v, %v", result, err)
	}
	transaction, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = idempotency.Reserve(ctx, transaction, scope, idempotency.RequestHash([]byte(`{"input":false}`)))
	_ = transaction.Rollback(ctx)
	if !errors.Is(err, idempotency.ErrConflict) {
		t.Fatalf("HTTP idempotency mismatch = %v, want conflict", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE request_idempotency SET expires_at=clock_timestamp()-interval '1 second'
		WHERE tenant_id=$1 AND actor_id=$2 AND operation=$3 AND idempotency_key=$4`, scope.TenantID, scope.ActorID, scope.Operation, scope.Key); err != nil {
		t.Fatal(err)
	}
	removed, err := idempotency.PurgeExpired(ctx, pool, 10)
	if err != nil || removed != 1 {
		t.Fatalf("expired HTTP response cleanup removed=%d err=%v, want 1", removed, err)
	}
	atomicScope := idempotency.Scope{TenantID: tenant, ActorID: actor, Operation: "test-atomic", Key: "atomic-key"}
	atomicHash := idempotency.RequestHash([]byte(`{"job":true}`))
	transaction, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idempotency.Reserve(ctx, transaction, atomicScope, atomicHash); err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.EnqueueTx(ctx, transaction, tenant, actor, strings.Repeat("x", 121), "atomic-key", []byte(`{}`), nil); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("job with invalid type was enqueued")
	}
	_ = transaction.Rollback(ctx)
	transaction, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idempotency.Reserve(ctx, transaction, atomicScope, atomicHash); err != nil {
		t.Fatalf("failed enqueue left an idempotency reservation behind: %v", err)
	}
	atomicJobID, err := jobs.EnqueueTx(ctx, transaction, tenant, actor, "atomic", "atomic-key", []byte(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := idempotency.Complete(ctx, transaction, atomicScope, 202, []byte(`{"queued":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(ctx); err != nil || atomicJobID == "" {
		t.Fatalf("atomic idempotency/job transaction failed: id=%q err=%v", atomicJobID, err)
	}
	first, err := queue.ClaimTypes(ctx, "worker-a", 30*time.Millisecond, []string{"integration"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	second, err := queue.ClaimTypes(ctx, "worker-b", time.Second, []string{"integration"})
	if err != nil || second.ID != first.ID || second.Generation <= first.Generation {
		t.Fatalf("expired lease was not reclaimed with a new fence: first=%#v second=%#v err=%v", first, second, err)
	}
	if err := queue.Complete(ctx, first, ""); !errors.Is(err, jobs.ErrStale) {
		t.Fatalf("stale completion error=%v, want ErrStale", err)
	}
	if err := queue.Complete(ctx, second, ""); err != nil {
		t.Fatalf("current completion: %v", err)
	}

	const concurrent = 8
	for n := 0; n < concurrent; n++ {
		body := fmt.Sprintf(`{"n":%d}`, n)
		if _, err := queue.EnqueueWork(ctx, tenant, actor, "parallel", fmt.Sprintf("parallel-%d", n), []byte(body), nil); err != nil {
			t.Fatal(err)
		}
	}
	claimed := make(chan string, concurrent)
	errs := make(chan error, concurrent)
	var group sync.WaitGroup
	for n := 0; n < concurrent; n++ {
		group.Add(1)
		go func(n int) {
			defer group.Done()
			claim, err := queue.ClaimTypes(ctx, fmt.Sprintf("parallel-%d", n), time.Second, []string{"parallel"})
			if err != nil {
				errs <- err
				return
			}
			claimed <- claim.ID
		}(n)
	}
	group.Wait()
	close(claimed)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for id := range claimed {
		if seen[id] {
			t.Fatalf("job %s was claimed twice concurrently", id)
		}
		seen[id] = true
	}
	if len(seen) != concurrent {
		t.Fatalf("claimed %d concurrent jobs, want %d", len(seen), concurrent)
	}
	deadline := time.Now().Add(-time.Second)
	if _, err := queue.EnqueueWork(ctx, tenant, actor, "deadline", "expired", []byte(`{}`), &deadline); err != nil {
		t.Fatal(err)
	}
	if err := queue.RecoverExhausted(ctx); err != nil {
		t.Fatal(err)
	}
	dead, err := queue.ListDead(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	foundExpired := false
	for _, job := range dead {
		if job.Type == "deadline" {
			foundExpired = true
		}
	}
	if !foundExpired {
		t.Fatal("expired job deadline was not moved to operator repair")
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE crash_effects(job_id uuid PRIMARY KEY, count integer NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	crashType := "crash_" + fmt.Sprint(time.Now().UnixNano())
	crashJobID, err := queue.EnqueueWork(ctx, tenant, actor, crashType, crashType, []byte(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	childURL, err := crashTestURL(databaseURL, pool.Config().ConnConfig.RuntimeParams["search_path"])
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestWorkerCrashHelper$")
	command.Env = filteredEnv("TEST_DATABASE_URL", "TEST_WORKER_CRASH_HELPER", "TEST_CRASH_TYPE")
	command.Env = append(command.Env, "TEST_WORKER_CRASH_HELPER=true", "TEST_CRASH_TYPE="+crashType, "TEST_DATABASE_URL="+childURL)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("crashed worker child failed: %v\n%s", err, output)
	}
	var childID string
	var childGeneration int
	if _, err := fmt.Sscanf(string(output), "%s %d", &childID, &childGeneration); err != nil || childID != crashJobID {
		t.Fatalf("unexpected crash worker claim %q, err=%v", output, err)
	}
	var effectCount int
	if err := pool.QueryRow(ctx, `SELECT count FROM crash_effects WHERE job_id=$1`, crashJobID).Scan(&effectCount); err != nil || effectCount != 1 {
		t.Fatalf("durable external effect before process crash count=%d err=%v", effectCount, err)
	}
	time.Sleep(100 * time.Millisecond)
	recovered, err := queue.ClaimTypes(ctx, "recovery-worker", time.Second, []string{crashType})
	if err != nil || recovered.ID != crashJobID || recovered.Generation <= childGeneration {
		t.Fatalf("process-crashed job was not fenced/recovered: child_generation=%d recovered=%#v err=%v", childGeneration, recovered, err)
	}
	if err := queue.Complete(ctx, jobs.Claim{ID: childID, Worker: "crash-child", Generation: childGeneration}, ""); !errors.Is(err, jobs.ErrStale) {
		t.Fatalf("crashed worker completion error=%v, want stale", err)
	}

	transaction, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rollbackEvent := outbox.Event{TenantID: tenant, AggregateType: "test", AggregateID: actor, EventType: "test.rolled_back", Version: 1, Payload: []byte(`{}`)}
	if err := outbox.Append(ctx, transaction, rollbackEvent, []string{"integration-handler"}); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatal(err)
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var rolledBack int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type='test.rolled_back'`).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("outbox row survived domain transaction rollback: count=%d err=%v", rolledBack, err)
	}
	transaction, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	event := outbox.Event{TenantID: tenant, AggregateType: "test", AggregateID: actor, EventType: "test.created", Version: 1, Payload: []byte(`{"ok":true}`)}
	if err := outbox.Append(ctx, transaction, event, []string{"integration-handler"}); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatal(err)
	}
	if err := transaction.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	deliveries := outbox.Queue{Pool: pool}
	delivery, err := deliveries.Claim(ctx, "integration-handler", "outbox-a", 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if delivery.TenantID != tenant || delivery.EventType != event.EventType || delivery.AggregateID != actor || delivery.Version != event.Version {
		t.Fatalf("outbox claim omitted event authorization/routing metadata: %#v", delivery)
	}
	time.Sleep(50 * time.Millisecond)
	reclaimed, err := deliveries.Claim(ctx, "integration-handler", "outbox-b", time.Second)
	if err != nil || reclaimed.Generation <= delivery.Generation {
		t.Fatalf("outbox lease was not reclaimed: %v %#v", err, reclaimed)
	}
	if err := deliveries.Complete(ctx, delivery); err == nil {
		t.Fatal("stale outbox delivery completed")
	}
	if err := deliveries.Complete(ctx, reclaimed); err != nil {
		t.Fatal(err)
	}
	transaction, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	failedEvent := outbox.Event{TenantID: tenant, AggregateType: "test", AggregateID: actor, EventType: "test.failed", Version: 2, Payload: []byte(`{}`)}
	if err := outbox.Append(ctx, transaction, failedEvent, []string{"dead-handler"}); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatal(err)
	}
	if err := transaction.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		failed, err := deliveries.Claim(ctx, "dead-handler", "outbox-worker", time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err := deliveries.Retry(ctx, failed, 0, "handler_failed"); err != nil {
			t.Fatal(err)
		}
	}
	deadDeliveries, err := deliveries.ListDead(ctx, 10)
	if err != nil || len(deadDeliveries) != 1 || deadDeliveries[0].Handler != "dead-handler" {
		t.Fatalf("outbox delivery did not reach operator repair: %#v, %v", deadDeliveries, err)
	}
	if err := deliveries.RequeueDead(ctx, deadDeliveries[0].EventID, deadDeliveries[0].Handler); err != nil {
		t.Fatal(err)
	}
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	go func() {
		workerDone <- deliveries.Run(workerCtx, "integration-worker", map[string]outbox.Handler{
			"dead-handler": func(context.Context, outbox.Delivery) error { return nil },
		}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		var state string
		if err := pool.QueryRow(ctx, `SELECT state FROM outbox_deliveries WHERE event_id=$1 AND handler='dead-handler'`, deadDeliveries[0].EventID).Scan(&state); err == nil && state == "done" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	var deliveryState string
	if err := pool.QueryRow(ctx, `SELECT state FROM outbox_deliveries WHERE event_id=$1 AND handler='dead-handler'`, deadDeliveries[0].EventID).Scan(&deliveryState); err != nil || deliveryState != "done" {
		stopWorker()
		t.Fatalf("registered outbox handler did not complete delivery: state=%q err=%v", deliveryState, err)
	}
	stopWorker()
	if err := <-workerDone; err != nil {
		t.Fatal(err)
	}

	transaction, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := audit.Append(ctx, transaction, audit.Entry{TenantID: tenant, ActorID: actor, Action: "test", ResourceType: "integration", ResourceID: actor}); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatal(err)
	}
	if err := transaction.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_log SET action='changed' WHERE actor_id=$1`, actor); err == nil {
		t.Fatal("audit log update was allowed")
	}
}

func TestWorkerCrashHelper(t *testing.T) {
	if os.Getenv("TEST_WORKER_CRASH_HELPER") != "true" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	claim, err := (jobs.Queue{Pool: pool}).ClaimTypes(ctx, "crash-child", 50*time.Millisecond, []string{os.Getenv("TEST_CRASH_TYPE")})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO crash_effects(job_id,count) VALUES($1,1)`, claim.ID); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Printf("%s %d", claim.ID, claim.Generation)
	// Simulate abrupt worker loss after a durable side effect but before queue completion.
	os.Exit(0)
}

func crashTestURL(databaseURL, schema string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("options", "-csearch_path="+schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func filteredEnv(keys ...string) []string {
	var env []string
	for _, entry := range os.Environ() {
		filtered := false
		for _, key := range keys {
			if strings.HasPrefix(entry, key+"=") {
				filtered = true
				break
			}
		}
		if !filtered {
			env = append(env, entry)
		}
	}
	return env
}

func testPool(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool {
	t.Helper()
	adminConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	var schema string
	if err := admin.QueryRow(ctx, `SELECT 'test_' || replace(gen_random_uuid()::text,'-','')`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`)
	})
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	return pool
}
