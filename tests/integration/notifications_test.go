package integration_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"peergit/internal/campus"
	"peergit/internal/health"
	"peergit/internal/identity"
	"peergit/internal/notification"
	"peergit/internal/platform/errormanager"
	httpserver "peergit/internal/platform/http"
	"peergit/internal/platform/jobs"
	"peergit/internal/platform/outbox"
	"peergit/migrations"
)

func TestPhase5BOutboxMaterializationIsDurableAndIdempotent(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run durable notification integration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	var college, owner, member, project, updateID, eventID string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES('phase5b-notifications','Phase 5B notifications') RETURNING id::text`).Scan(&college); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name   string
		target *string
	}{{"owner", &owner}, {"member", &member}} {
		email := row.name + "@phase5b.test"
		if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at) VALUES($1,$2,$2,$3,$4,'campus',now()) RETURNING id::text`, college, email, row.name, "p5b"+row.name).Scan(row.target); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO campus_verifications(college_id,user_id,source) VALUES($1,$2,'administrator_review')`, college, *row.target); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO projects(college_id,slug,title,summary,project_type,visibility,lifecycle,created_by) VALUES($1,'notifications','Notifications project','A test project','side_project','campus','active',$2) RETURNING id::text`, college, owner).Scan(&project); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ user, role string }{{owner, "owner"}, {member, "member"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO project_members(college_id,project_id,user_id,role) VALUES($1,$2,$3,$4)`, college, project, entry.user, entry.role); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO project_updates(college_id,project_id,author_id,body,audience) VALUES($1,$2,$3,'Progress update','members') RETURNING id::text`, college, project, owner).Scan(&updateID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notification_preferences(college_id,user_id,category,email_enabled) VALUES($1,$2,'project_activity',true)`, college, member); err != nil {
		t.Fatal(err)
	}
	key := []byte("phase5b integration session signing key with entropy")
	token := "phase5b-member-session"
	if _, err := pool.Exec(ctx, `INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'terms','draft-1'),($1,'privacy','draft-1')`, member); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, member, college, sessionHash(key, token), sessionCSRFHash(key, token)); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"project_id": project})
	if err = outbox.Append(ctx, tx, outbox.Event{TenantID: college, AggregateType: "project_update", AggregateID: updateID, EventType: "project.update.created", Version: 1, Payload: payload}, []string{"project.activity"}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT id::text FROM outbox_events WHERE aggregate_id=$1`, updateID).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	delivery := outbox.Delivery{EventID: eventID, Handler: "project.activity", TenantID: college, AggregateType: "project_update", AggregateID: updateID, EventType: "project.update.created", Version: 1, Payload: payload}
	handler := notification.OutboxHandler(pool, notification.Mail{AppOrigin: "http://peergit.test"})
	// Simulate a worker crash after its durable transaction and before outbox completion.
	if err = handler(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	if err = handler(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	var notifications, jobCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE source_event_id=$1 AND user_id=$2`, eventID, member).Scan(&notifications); err != nil || notifications != 1 {
		t.Fatalf("notifications=%d err=%v", notifications, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE job_type='notification_email' AND dedupe_key LIKE 'notification/%'`).Scan(&jobCount); err != nil || jobCount != 1 {
		t.Fatalf("notification email jobs=%d err=%v", jobCount, err)
	}
	var sequence int64
	if err = pool.QueryRow(ctx, `SELECT last_sequence FROM notification_cursors WHERE college_id=$1 AND user_id=$2`, college, member).Scan(&sequence); err != nil || sequence != 1 {
		t.Fatalf("notification sequence=%d err=%v", sequence, err)
	}
	var deliveryID, jobID string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM notification_email_deliveries WHERE user_id=$1`, member).Scan(&deliveryID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT id::text FROM jobs WHERE job_type='notification_email' AND dedupe_key='notification/'||$1`, deliveryID).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	var jobPayload json.RawMessage
	if err = pool.QueryRow(ctx, `SELECT payload FROM jobs WHERE id=$1`, jobID).Scan(&jobPayload); err != nil {
		t.Fatal(err)
	}
	jobHandler := notification.EmailJobHandler(pool, notification.Mail{AppOrigin: "http://peergit.test", Sender: failedNotificationSender{}}, campus.MailConfig{})
	if _, err = jobHandler(ctx, jobs.Claim{ID: jobID, TenantID: college, Worker: "phase5b-test", Generation: 1, Attempt: 1, Payload: jobPayload}); err == nil {
		t.Fatal("expected simulated email outage")
	} else if err.Error() != "test provider outage" {
		t.Fatalf("email handler failed before provider attempt: %v", err)
	}
	var emailStatus string
	var attempts int
	if err = pool.QueryRow(ctx, `SELECT status,attempts FROM notification_email_deliveries WHERE id=$1`, deliveryID).Scan(&emailStatus, &attempts); err != nil || emailStatus != "failed" || attempts != 1 {
		t.Fatalf("email status=%s attempts=%d err=%v", emailStatus, attempts, err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	successHandler := notification.EmailJobHandler(pool, notification.Mail{AppOrigin: "http://peergit.test", Sender: successfulNotificationSender{}}, campus.MailConfig{})
	if _, err = successHandler(ctx, jobs.Claim{ID: jobID, TenantID: college, Worker: "phase5b-test", Generation: 2, Attempt: 2, Payload: jobPayload}); err != nil {
		t.Fatalf("email retry: %v", err)
	}
	var providerMessage string
	if err = pool.QueryRow(ctx, `SELECT status,attempts,provider_message_id FROM notification_email_deliveries WHERE id=$1`, deliveryID).Scan(&emailStatus, &attempts, &providerMessage); err != nil || emailStatus != "accepted" || attempts != 2 || providerMessage != "notification-resend-message" {
		t.Fatalf("email retry status=%s attempts=%d provider_message=%s err=%v", emailStatus, attempts, providerMessage, err)
	}
	secretBytes := []byte("phase5b webhook signing secret with entropy")
	webhookSecret := "whsec_" + base64.StdEncoding.EncodeToString(secretBytes)
	webhook := campus.NewResendWebhookHandler(pool, webhookSecret, "phase5b webhook suppression key with entropy", logger)
	webhookRouter := httpserver.NewRouter(health.NewHandler(errormanager.NewManager(logger), pool), logger, errormanager.NewManager(logger), webhook.Register)
	sendWebhook := func(eventID, eventType string, occurred time.Time) *httptest.ResponseRecorder {
		t.Helper()
		body, marshalErr := json.Marshal(map[string]any{"type": eventType, "created_at": occurred.UTC(), "data": map[string]any{"email_id": "notification-resend-message", "to": []string{"member@phase5b.test"}}})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, secretBytes)
		_, _ = mac.Write([]byte(eventID + "." + timestamp + "."))
		_, _ = mac.Write(body)
		req := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/webhooks/resend", bytes.NewReader(body))
		req.Header.Set("svix-id", eventID)
		req.Header.Set("svix-timestamp", timestamp)
		req.Header.Set("svix-signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		res := httptest.NewRecorder()
		webhookRouter.ServeHTTP(res, req)
		return res
	}
	if res := sendWebhook("phase5b-delivered-event", "email.delivered", time.Now()); res.Code != http.StatusOK {
		t.Fatalf("delivery webhook returned %d: %s", res.Code, res.Body.String())
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM notification_email_deliveries WHERE id=$1`, deliveryID).Scan(&emailStatus); err != nil || emailStatus != "delivered" {
		t.Fatalf("webhook delivery status=%s err=%v", emailStatus, err)
	}
	if res := sendWebhook("phase5b-bounced-event", "email.bounced", time.Now().Add(time.Second)); res.Code != http.StatusOK {
		t.Fatalf("bounce webhook returned %d: %s", res.Code, res.Body.String())
	}
	var suppressions int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM email_suppressions) + (SELECT count(*) FROM notification_email_deliveries WHERE id=$1 AND status='bounced')`, deliveryID).Scan(&suppressions); err != nil || suppressions != 2 {
		t.Fatalf("bounce suppression/status count=%d err=%v", suppressions, err)
	}
	errorManager := errormanager.NewManager(logger)
	auth := identity.NewHandler(pool, identity.Config{AppOrigin: "http://peergit.test", SessionHashKey: string(key)}, logger, errorManager)
	api := notification.NewHandler(pool, auth)
	router := httpserver.NewRouter(health.NewHandler(errorManager, pool), logger, errorManager, auth.Register, api.Register)
	call := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://peergit.test"+path, bytes.NewBufferString(body))
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
		if method != http.MethodGet {
			req.Header.Set("Origin", "http://peergit.test")
			req.Header.Set("X-CSRF-Token", base64.RawURLEncoding.EncodeToString(sessionHash(key, "peergit-csrf:"+token)))
			req.Header.Set("Content-Type", "application/json")
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s returned %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res
	}
	inbox := call(http.MethodGet, "/api/v1/notifications", "", http.StatusOK)
	if !bytes.Contains(inbox.Body.Bytes(), []byte("New project update")) {
		t.Fatalf("inbox did not show durable notification: %s", inbox.Body.String())
	}
	streamServer := httptest.NewServer(router)
	defer streamServer.Close()
	streamClient := &http.Client{Transport: &http.Transport{MaxConnsPerHost: 8}}
	openStream := func(streamContext context.Context, lastEventID string) *http.Response {
		req, requestErr := http.NewRequestWithContext(streamContext, http.MethodGet, streamServer.URL+"/api/v1/notifications/stream", nil)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if lastEventID != "" {
			req.Header.Set("Last-Event-ID", lastEventID)
		}
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
		res, requestErr := streamClient.Do(req)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		return res
	}
	firstStream, secondStream := openStream(context.Background(), "0"), openStream(context.Background(), "0")
	if firstStream.StatusCode != http.StatusOK || secondStream.StatusCode != http.StatusOK {
		t.Fatalf("stream statuses %d, %d", firstStream.StatusCode, secondStream.StatusCode)
	}
	thirdStream := openStream(context.Background(), "0")
	if thirdStream.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("third stream status=%d want 429", thirdStream.StatusCode)
	}
	_ = thirdStream.Body.Close()
	_ = firstStream.Body.Close()
	_ = secondStream.Body.Close()
	time.Sleep(100 * time.Millisecond)
	replay := openStream(context.Background(), "0")
	if replay.StatusCode != http.StatusOK {
		t.Fatalf("replay stream status=%d", replay.StatusCode)
	}
	scanner := bufio.NewScanner(replay.Body)
	seenID := false
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "id: 1") {
			seenID = true
			break
		}
	}
	_ = replay.Body.Close()
	if !seenID {
		t.Fatalf("Last-Event-ID replay did not include notification sequence 1")
	}
	call(http.MethodPost, "/api/v1/notifications/read-all", "{}", http.StatusNoContent)
	call(http.MethodPut, "/api/v1/notifications/preferences", `{"email":{"applications":true,"project_activity":false}}`, http.StatusOK)
	if _, err = pool.Exec(ctx, `DELETE FROM project_members WHERE project_id=$1 AND user_id=$2`, project, member); err != nil {
		t.Fatal(err)
	}
	inbox = call(http.MethodGet, "/api/v1/notifications", "", http.StatusOK)
	if bytes.Contains(inbox.Body.Bytes(), []byte(updateID)) {
		t.Fatalf("inbox returned project notification after membership revocation: %s", inbox.Body.String())
	}
	streamCtx, streamCancel := context.WithTimeout(ctx, 5*time.Second)
	defer streamCancel()
	activeStream := openStream(streamCtx, "0")
	if activeStream.StatusCode != http.StatusOK {
		t.Fatalf("session revalidation stream status=%d", activeStream.StatusCode)
	}
	if _, err = pool.Exec(ctx, `UPDATE sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, sessionHash(key, token)); err != nil {
		t.Fatal(err)
	}
	revokedScanner := bufio.NewScanner(activeStream.Body)
	for revokedScanner.Scan() {
	}
	if revokedScanner.Err() != nil {
		t.Fatalf("stream did not close after session expiry: %v", revokedScanner.Err())
	}
	_ = activeStream.Body.Close()
}

type failedNotificationSender struct{}

func (failedNotificationSender) SendNotification(context.Context, string, string, string, string, string, string) (string, error) {
	return "", errors.New("test provider outage")
}

type successfulNotificationSender struct{}

func (successfulNotificationSender) SendNotification(context.Context, string, string, string, string, string, string) (string, error) {
	return "notification-resend-message", nil
}
