package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"peergit/internal/collaboration"
	"peergit/internal/health"
	"peergit/internal/identity"
	"peergit/internal/notification"
	"peergit/internal/platform/errormanager"
	httpserver "peergit/internal/platform/http"
	"peergit/internal/platform/outbox"
	"peergit/migrations"
)

func TestPhase5CScopedMessagingAndDiscovery(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run Phase 5C scoped messaging and discovery checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	stamp := fmt.Sprintf("%d", time.Now().UnixNano())
	var college, otherCollege string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES($1,$2) RETURNING id::text`, "phase5c-"+stamp, "Phase 5C "+stamp).Scan(&college); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES($1,$2) RETURNING id::text`, "phase5c-other-"+stamp, "Phase 5C Other "+stamp).Scan(&otherCollege); err != nil {
		t.Fatal(err)
	}
	key := []byte("phase5c integration session signing key")
	cursorKey := []byte("phase5c integration cursor signing key")
	tokens := map[string]string{}
	csrf := map[string]string{}
	users := map[string]string{}
	for _, fixture := range []struct {
		name, tenant, display string
	}{{"owner", college, "Project Owner"}, {"member", college, "Searchable Student"}, {"visitor", college, "Campus Visitor"}, {"other", otherCollege, "Other Campus"}} {
		var userID string
		email := fixture.name + "+" + stamp + "@phase5c.test"
		handle := "p5c_" + fixture.name + "_" + stamp[len(stamp)-12:]
		if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at,profile)
			VALUES($1,$2,$2,$3,$4,'campus',now(),'{"headline":"Search systems","bio":"Builds data tools"}') RETURNING id::text`, fixture.tenant, email, fixture.display, handle).Scan(&userID); err != nil {
			t.Fatal(err)
		}
		users[fixture.name] = userID
		if _, err := pool.Exec(ctx, `INSERT INTO campus_verifications(college_id,user_id,source) VALUES($1,$2,'administrator_review')`, fixture.tenant, userID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'terms','draft-1'),($1,'privacy','draft-1')`, userID); err != nil {
			t.Fatal(err)
		}
		if fixture.name == "owner" {
			if _, err := pool.Exec(ctx, `INSERT INTO campus_roles(college_id,user_id,role) VALUES($1,$2,'campus_admin')`, fixture.tenant, userID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'profile_discovery','draft-1')`, userID); err != nil {
				t.Fatal(err)
			}
		}
		token := "phase5c-session-" + fixture.name + "-" + stamp
		tokens[fixture.name] = token
		if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, userID, fixture.tenant, sessionHash(key, token), sessionCSRFHash(key, token)); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	errs := errormanager.NewManager(logger)
	auth := identity.NewHandler(pool, identity.Config{AppOrigin: "http://phase5c.test", SessionHashKey: string(key), CookieSecure: false}, logger, errs)
	work := collaboration.NewHandler(pool, auth, logger, errs, cursorKey)
	alerts := notification.NewHandler(pool, auth)
	router := httpserver.NewRouter(health.NewHandler(logger, errs, pool), logger, errs, auth.Register, work.Register, alerts.Register)
	for name, token := range tokens {
		req := httptest.NewRequest(http.MethodGet, "http://phase5c.test/api/v1/session", nil)
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("load session %s: %d %s", name, res.Code, res.Body.String())
		}
		var envelope struct {
			Data struct {
				CSRF string `json:"csrf_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil || len(envelope.Data.CSRF) < 32 {
			t.Fatalf("load CSRF for %s: %v", name, err)
		}
		csrf[name] = envelope.Data.CSRF
	}
	call := func(actor, method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://phase5c.test"+path, bytes.NewBufferString(body))
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: tokens[actor]})
		if method != http.MethodGet {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://phase5c.test")
			req.Header.Set("X-CSRF-Token", csrf[actor])
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, path, res.Code, want, res.Body.String())
		}
		return res
	}
	decodeID := func(res *httptest.ResponseRecorder, key string) string {
		t.Helper()
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		id, ok := envelope.Data[key].(string)
		if !ok || id == "" {
			t.Fatalf("missing %s in %s", key, res.Body.String())
		}
		return id
	}
	var projectID string
	if err := pool.QueryRow(ctx, `INSERT INTO projects(college_id,slug,title,summary,description,project_type,visibility,lifecycle,created_by)
		VALUES($1,$2,'Searchable project','Fixture project for search','Campus feed fixture','side_project','campus','active',$3) RETURNING id::text`, college, "phase5c-main-"+stamp, users["owner"]).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	for name, role := range map[string]string{"owner": "owner", "member": "member"} {
		if _, err := pool.Exec(ctx, `INSERT INTO project_members(college_id,project_id,user_id,role) VALUES($1,$2,$3,$4)`, college, projectID, users[name], role); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO project_roles(college_id,project_id,title,description,openings,difficulty,good_first_task) VALUES($1,$2,'Beginner search role','A role for search fixture',1,'beginner',true)`, college, projectID); err != nil {
		t.Fatal(err)
	}
	var applicationID string
	if err := pool.QueryRow(ctx, `INSERT INTO applications(college_id,project_role_id,applicant_user_id,message,status,version,decided_by,decided_at)
		SELECT $1,id,$2,'Ready to join','accepted',2,$3,now() FROM project_roles WHERE project_id=$4 RETURNING id::text`, college, users["member"], users["owner"], projectID).Scan(&applicationID); err != nil {
		t.Fatal(err)
	}
	projectPath := "/api/v1/projects/" + projectID
	ownerConversation := decodeID(call("owner", http.MethodPost, projectPath+"/conversation", `{}`, http.StatusCreated), "id")
	call("visitor", http.MethodGet, "/api/v1/conversations/"+ownerConversation+"/messages", "", http.StatusNotFound)
	call("other", http.MethodGet, "/api/v1/conversations/"+ownerConversation+"/messages", "", http.StatusNotFound)
	messageKey := "11111111-1111-4111-8111-111111111111"
	messagePath := "/api/v1/conversations/" + ownerConversation + "/messages"
	messageResponse := call("owner", http.MethodPost, messagePath, `{"client_message_id":"`+messageKey+`","body":"Use the short-lived fixture branch"}`, http.StatusCreated)
	messageID := decodeID(messageResponse, "id")
	call("owner", http.MethodPost, messagePath, `{"client_message_id":"`+messageKey+`","body":"Use the short-lived fixture branch"}`, http.StatusOK)
	call("owner", http.MethodPost, messagePath, `{"client_message_id":"`+messageKey+`","body":"Different body"}`, http.StatusConflict)
	if _, err := pool.Exec(ctx, `INSERT INTO conversation_messages(college_id,conversation_id,sender_id,client_message_id,body)
		SELECT $1,$2,$3,('22222222-2222-4222-8222-'||lpad(g::text,12,'0'))::uuid,'Cursor fixture message '||g
		FROM generate_series(1,21) g`, college, ownerConversation, users["owner"]); err != nil {
		t.Fatal(err)
	}
	var firstMessagePage struct {
		Data struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			NextCursor string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(call("member", http.MethodGet, messagePath, "", http.StatusOK).Body.Bytes(), &firstMessagePage); err != nil {
		t.Fatal(err)
	}
	if len(firstMessagePage.Data.Items) != 20 || firstMessagePage.Data.NextCursor == "" {
		t.Fatalf("first message page count=%d cursor=%q; want 20 and a continuation cursor", len(firstMessagePage.Data.Items), firstMessagePage.Data.NextCursor)
	}
	messageIDs := map[string]bool{}
	for _, item := range firstMessagePage.Data.Items {
		messageIDs[item.ID] = true
	}
	var secondMessagePage struct {
		Data struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	messageCursorURL := messagePath + "?cursor=" + url.QueryEscape(firstMessagePage.Data.NextCursor)
	if err := json.Unmarshal(call("member", http.MethodGet, messageCursorURL, "", http.StatusOK).Body.Bytes(), &secondMessagePage); err != nil {
		t.Fatal(err)
	}
	if len(secondMessagePage.Data.Items) != 2 {
		t.Fatalf("second message page count=%d; want remaining two messages", len(secondMessagePage.Data.Items))
	}
	for _, item := range secondMessagePage.Data.Items {
		if messageIDs[item.ID] {
			t.Fatalf("message %s appeared on both cursor pages", item.ID)
		}
	}
	var eventID string
	var eventPayload []byte
	if err := pool.QueryRow(ctx, `SELECT id::text,payload FROM outbox_events WHERE aggregate_id=$1 AND event_type='conversation.message.created'`, messageID).Scan(&eventID, &eventPayload); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(eventPayload, []byte("Use the short-lived fixture branch")) {
		t.Fatal("message body was copied into the durable notification event")
	}
	messageNotifications := notification.OutboxHandler(pool, notification.Mail{AppOrigin: "http://phase5c.test"})
	delivery := outbox.Delivery{EventID: eventID, Handler: "project.activity", TenantID: college, AggregateType: "conversation_message", AggregateID: messageID, EventType: "conversation.message.created", Version: 1, Payload: eventPayload}
	if err := messageNotifications(ctx, delivery); err != nil {
		t.Fatalf("materialize message notification: %v", err)
	}
	if err := messageNotifications(ctx, delivery); err != nil {
		t.Fatalf("retry message notification: %v", err)
	}
	var noticeCount, messageEmailCount int
	var noticeBody string
	if err := pool.QueryRow(ctx, `SELECT count(*),COALESCE(max(n.body),'') FROM notifications n WHERE n.source_event_id=$1 AND n.user_id=$2 AND n.category='messages'`, eventID, users["member"]).Scan(&noticeCount, &noticeBody); err != nil || noticeCount != 1 {
		t.Fatalf("message notifications=%d err=%v; want one durable notice", noticeCount, err)
	}
	if strings.Contains(noticeBody, "Use the short-lived fixture branch") {
		t.Fatal("message text appeared in the durable notification body")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_email_deliveries d JOIN notifications n ON n.id=d.notification_id WHERE n.source_event_id=$1`, eventID).Scan(&messageEmailCount); err != nil || messageEmailCount != 0 {
		t.Fatalf("message email deliveries=%d err=%v; message content must stay in-app", messageEmailCount, err)
	}
	call("member", http.MethodGet, messagePath, "", http.StatusOK)
	report := fmt.Sprintf(`{"message_id":%q,"reason":"spam","details":"fixture report"}`, messageID)
	call("member", http.MethodPost, "/api/v1/conversations/"+ownerConversation+"/reports", report, http.StatusCreated)
	call("member", http.MethodPost, "/api/v1/conversations/"+ownerConversation+"/reports", report, http.StatusConflict)
	call("member", http.MethodPut, "/api/v1/blocks/"+users["owner"], `{}`, http.StatusOK)
	blockedMessages := call("member", http.MethodGet, messagePath, "", http.StatusOK).Body.String()
	if strings.Contains(blockedMessages, "Use the short-lived fixture branch") {
		t.Fatal("blocked sender message remained visible")
	}
	call("member", http.MethodDelete, "/api/v1/blocks/"+users["owner"], `{}`, http.StatusNoContent)
	applicationConversation := decodeID(call("member", http.MethodPost, "/api/v1/applications/"+applicationID+"/conversation", `{}`, http.StatusCreated), "id")
	call("other", http.MethodPost, "/api/v1/applications/"+applicationID+"/conversation", `{}`, http.StatusNotFound)

	updateBody := `{"body":"Campus searchable update","audience":"campus"}`
	call("owner", http.MethodPost, projectPath+"/updates", updateBody, http.StatusCreated)
	secondUpdate := `{"body":"A second member records meaningful work","audience":"campus"}`
	call("member", http.MethodPost, projectPath+"/updates", secondUpdate, http.StatusCreated)
	var wacp int64
	if err := pool.QueryRow(ctx, `SELECT COALESCE((SELECT project_count FROM weekly_active_collaborative_projects WHERE college_id=$1),0)`, college).Scan(&wacp); err != nil || wacp != 1 {
		t.Fatalf("WACP = %d, err=%v; want one project with two distinct member actions", wacp, err)
	}
	var summary struct {
		Data struct {
			Value           int64     `json:"value"`
			Sources         []string  `json:"sources"`
			FreshnessAt     time.Time `json:"freshness_at"`
			DescriptiveOnly bool      `json:"descriptive_only"`
		} `json:"data"`
	}
	if err := json.Unmarshal(call("owner", http.MethodGet, "/api/v1/campus/insights/wacp", "", http.StatusOK).Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Data.Value != 1 || !summary.Data.DescriptiveOnly || summary.Data.FreshnessAt.IsZero() || len(summary.Data.Sources) == 0 {
		t.Fatalf("campus administrator WACP response is missing summary detail: %+v", summary.Data)
	}
	var inserted int64
	if err := pool.QueryRow(ctx, `SELECT rebuild_project_collaboration_events()`).Scan(&inserted); err != nil {
		t.Fatalf("rebuild collaboration events: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT rebuild_project_collaboration_events()`).Scan(&inserted); err != nil || inserted != 0 {
		t.Fatalf("rebuild was not idempotent: inserted=%d err=%v", inserted, err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM project_members WHERE project_id=$1 AND user_id=$2`, projectID, users["member"]); err != nil {
		t.Fatal(err)
	}
	call("member", http.MethodGet, messagePath, "", http.StatusNotFound)
	if err := pool.QueryRow(ctx, `SELECT COALESCE((SELECT project_count FROM weekly_active_collaborative_projects WHERE college_id=$1),0)`, college).Scan(&wacp); err != nil || wacp != 0 {
		t.Fatalf("WACP after membership removal = %d, err=%v; want zero", wacp, err)
	}
	if err := json.Unmarshal(call("owner", http.MethodGet, "/api/v1/campus/insights/wacp", "", http.StatusOK).Body.Bytes(), &summary); err != nil || summary.Data.Value != 0 {
		t.Fatalf("campus administrator summary retained removed-member activity: %+v err=%v", summary.Data, err)
	}
	if err := pool.QueryRow(ctx, `SELECT rebuild_project_collaboration_events()`).Scan(&inserted); err != nil || inserted != 0 {
		t.Fatalf("rebuild after membership removal changed the event ledger: inserted=%d err=%v", inserted, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE applications SET status='rejected',version=version+1,decided_by=$2,decided_at=now() WHERE id=$1`, applicationID, users["owner"]); err != nil {
		t.Fatal(err)
	}
	call("member", http.MethodGet, "/api/v1/conversations/"+applicationConversation+"/messages", "", http.StatusNotFound)

	call("visitor", http.MethodGet, "/api/v1/feed?scope=campus", "", http.StatusOK)
	if feed := call("visitor", http.MethodGet, "/api/v1/feed?scope=following", "", http.StatusOK).Body.String(); strings.Contains(feed, "Campus searchable update") {
		t.Fatal("following feed included a project that the viewer does not follow")
	}
	call("visitor", http.MethodPut, projectPath+"/follow", `{}`, http.StatusOK)
	following := call("visitor", http.MethodGet, "/api/v1/feed?scope=following", "", http.StatusOK).Body.String()
	if !strings.Contains(following, "Campus searchable update") {
		t.Fatal("following feed missed currently followed project activity")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO project_updates(college_id,project_id,author_id,body,audience)
		SELECT $1,$2,$3,'Feed cursor fixture '||g,'campus' FROM generate_series(1,20) g`, college, projectID, users["owner"]); err != nil {
		t.Fatal(err)
	}
	var firstFeedPage struct {
		Data struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			NextCursor string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(call("visitor", http.MethodGet, "/api/v1/feed?scope=following", "", http.StatusOK).Body.Bytes(), &firstFeedPage); err != nil {
		t.Fatal(err)
	}
	if len(firstFeedPage.Data.Items) != 20 || firstFeedPage.Data.NextCursor == "" {
		t.Fatalf("first feed page count=%d cursor=%q; want 20 and a continuation cursor", len(firstFeedPage.Data.Items), firstFeedPage.Data.NextCursor)
	}
	feedIDs := map[string]bool{}
	for _, item := range firstFeedPage.Data.Items {
		feedIDs[item.ID] = true
	}
	var secondFeedPage struct {
		Data struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	feedCursorURL := "/api/v1/feed?scope=following&cursor=" + url.QueryEscape(firstFeedPage.Data.NextCursor)
	if err := json.Unmarshal(call("visitor", http.MethodGet, feedCursorURL, "", http.StatusOK).Body.Bytes(), &secondFeedPage); err != nil {
		t.Fatal(err)
	}
	if len(secondFeedPage.Data.Items) != 2 {
		t.Fatalf("second feed page count=%d; want remaining two updates", len(secondFeedPage.Data.Items))
	}
	for _, item := range secondFeedPage.Data.Items {
		if feedIDs[item.ID] {
			t.Fatalf("feed item %s appeared on both cursor pages", item.ID)
		}
	}
	searchBeforeConsent := call("visitor", http.MethodGet, "/api/v1/search?q=Searchable%20Student", "", http.StatusOK).Body.String()
	if strings.Contains(searchBeforeConsent, users["member"]) {
		t.Fatal("profile without discovery consent appeared in campus search")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'profile_discovery','draft-1')`, users["member"]); err != nil {
		t.Fatal(err)
	}
	searchAfterConsent := call("visitor", http.MethodGet, "/api/v1/search?q=Searchable%20Student", "", http.StatusOK).Body.String()
	if !strings.Contains(searchAfterConsent, users["member"]) {
		t.Fatal("profile with discovery consent did not appear in campus search")
	}
	projectSearch := call("visitor", http.MethodGet, "/api/v1/search?q=Searchable", "", http.StatusOK).Body.String()
	if !strings.Contains(projectSearch, projectID) {
		t.Fatal("visible project did not appear in grouped search")
	}
	if _, err := pool.Exec(ctx, `UPDATE consent_records SET revoked_at=now() WHERE user_id=$1 AND purpose='profile_discovery' AND revoked_at IS NULL`, users["member"]); err != nil {
		t.Fatal(err)
	}
	searchAfterRevocation := call("visitor", http.MethodGet, "/api/v1/search?q=Searchable%20Student", "", http.StatusOK).Body.String()
	if strings.Contains(searchAfterRevocation, users["member"]) {
		t.Fatal("profile remained in search after discovery consent was revoked")
	}
	call("visitor", http.MethodGet, "/api/v1/campus/insights/wacp", "", http.StatusForbidden)

	if _, err := pool.Exec(ctx, `INSERT INTO projects(college_id,slug,title,summary,description,project_type,visibility,lifecycle,created_by)
		SELECT $1,'phase5c-fixture-'||$2||'-'||g,'Fixture project '||g,'Search fixture cardinality','Synthetic search row','side_project','campus','active',$3 FROM generate_series(1,1000) g`, college, stamp, users["owner"]); err != nil {
		t.Fatal(err)
	}
	durations := make([]time.Duration, 0, 12)
	for i := 0; i < 12; i++ {
		start := time.Now()
		call("visitor", http.MethodGet, "/api/v1/search?q=Fixture", "", http.StatusOK)
		durations = append(durations, time.Since(start))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[11]
	t.Logf("Phase 5C campus search p95 at 1,000 synthetic projects: %s", p95)
	if p95 >= 500*time.Millisecond {
		t.Fatalf("campus search p95=%s exceeds 500ms at the seeded fixture cardinality", p95)
	}
	var outboxCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE tenant_id=$1 AND aggregate_id=$2 AND event_type='conversation.message.created'`, college, messageID).Scan(&outboxCount); err != nil || outboxCount != 1 {
		t.Fatalf("message outbox count=%d err=%v", outboxCount, err)
	}
}
