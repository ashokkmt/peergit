package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"peergit/internal/collaboration"
	"peergit/internal/health"
	"peergit/internal/identity"
	"peergit/internal/platform/errormanager"
	httpserver "peergit/internal/platform/http"
	"peergit/migrations"
)

func TestPhase5ADailyCollaborationAccessAndConcurrency(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run collaboration integration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	var college, otherCollege string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES('phase5a-one','Phase 5A One') RETURNING id::text`).Scan(&college); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES('phase5a-two','Phase 5A Two') RETURNING id::text`).Scan(&otherCollege); err != nil {
		t.Fatal(err)
	}
	key := []byte("phase5a integration session signing key with entropy")
	tokens := map[string]string{}
	csrf := map[string]string{}
	users := map[string]string{}
	for _, data := range []struct{ name, tenant string }{{"owner", college}, {"member", college}, {"visitor", college}, {"other", otherCollege}} {
		var userID string
		email := data.name + "@" + data.name + ".phase5a.test"
		if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at) VALUES($1,$2,$2,$3,$4,'campus',now()) RETURNING id::text`, data.tenant, email, data.name, "p5a_"+data.name).Scan(&userID); err != nil {
			t.Fatal(err)
		}
		users[data.name] = userID
		if _, err := pool.Exec(ctx, `INSERT INTO campus_verifications(college_id,user_id,source) VALUES($1,$2,'administrator_review')`, data.tenant, userID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'terms','draft-1'),($1,'privacy','draft-1')`, userID); err != nil {
			t.Fatal(err)
		}
		token := "phase5a-session-" + data.name
		tokens[data.name] = token
		if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, userID, data.tenant, sessionHash(key, token), sessionCSRFHash(key, token)); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	errs := errormanager.NewManager(logger)
	auth := identity.NewHandler(pool, identity.Config{AppOrigin: "http://peergit.test", SessionHashKey: string(key), CookieSecure: false}, logger, errs)
	work := collaboration.NewHandler(pool, auth, logger, errs, []byte("phase5a integration cursor signing key"))
	router := httpserver.NewRouter(health.NewHandler(errs, pool), logger, errs, auth.Register, work.Register)
	for name, token := range tokens {
		req := httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/session", nil)
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("load session %s: %d %s", name, res.Code, res.Body.String())
		}
		var env struct {
			Data struct {
				CSRF string `json:"csrf_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &env); err != nil || len(env.Data.CSRF) < 32 {
			t.Fatalf("load CSRF for %s: %v", name, err)
		}
		csrf[name] = env.Data.CSRF
	}
	call := func(actor, method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://peergit.test"+path, bytes.NewBufferString(body))
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: tokens[actor]})
		if method != http.MethodGet {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://peergit.test")
			req.Header.Set("X-CSRF-Token", csrf[actor])
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, path, res.Code, want, res.Body.String())
		}
		return res
	}
	anonymous := httptest.NewRecorder()
	router.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/projects/not-a-project/updates", nil))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous update read status=%d want=%d", anonymous.Code, http.StatusUnauthorized)
	}
	decodeID := func(res *httptest.ResponseRecorder, key string) string {
		t.Helper()
		var env struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		id, ok := env.Data[key].(string)
		if !ok || id == "" {
			t.Fatalf("missing %s in %s", key, res.Body.String())
		}
		return id
	}
	var projectID string
	if err := pool.QueryRow(ctx, `INSERT INTO projects(college_id,slug,title,summary,project_type,visibility,lifecycle,created_by) VALUES($1,'phase5a-collab','Phase 5A Collaboration','Test project','side_project','campus','active',$2) RETURNING id::text`, college, users["owner"]).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	for user, role := range map[string]string{"owner": "owner", "member": "member"} {
		if _, err := pool.Exec(ctx, `INSERT INTO project_members(college_id,project_id,user_id,role) VALUES($1,$2,$3,$4)`, college, projectID, users[user], role); err != nil {
			t.Fatal(err)
		}
	}
	base := "/api/v1/projects/" + projectID
	updateID := decodeID(call("owner", http.MethodPost, base+"/updates", `{"body":"<script>window.bad=true</script>Progress update","audience":"campus"}`, http.StatusCreated), "id")
	call("member", http.MethodGet, base+"/updates", "", http.StatusOK)
	call("visitor", http.MethodPost, base+"/updates", `{"body":"not a member"}`, http.StatusNotFound)
	call("other", http.MethodGet, base+"/updates", "", http.StatusNotFound)
	commentID := decodeID(call("member", http.MethodPost, base+"/comments", fmt.Sprintf(`{"target_type":"update","target_id":%q,"body":"Comment"}`, updateID), http.StatusCreated), "id")
	replyID := decodeID(call("owner", http.MethodPost, base+"/comments", fmt.Sprintf(`{"target_type":"update","target_id":%q,"parent_id":%q,"body":"Reply"}`, updateID, commentID), http.StatusCreated), "id")
	call("member", http.MethodPost, base+"/comments", fmt.Sprintf(`{"target_type":"update","target_id":%q,"parent_id":%q,"body":"Nested reply"}`, updateID, replyID), http.StatusBadRequest)
	call("member", http.MethodGet, base+"/comments?target_type=update&target_id="+updateID, "", http.StatusOK)
	call("member", http.MethodPut, base+"/reactions", fmt.Sprintf(`{"target_type":"update","target_id":%q,"reaction":"support"}`, updateID), http.StatusOK)
	call("member", http.MethodPut, base+"/reactions", fmt.Sprintf(`{"target_type":"update","target_id":%q,"reaction":"support"}`, updateID), http.StatusOK)
	var reactionCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM project_reactions WHERE update_id=$1 AND user_id=$2`, updateID, users["member"]).Scan(&reactionCount); err != nil || reactionCount != 1 {
		t.Fatalf("reaction upsert count=%d err=%v", reactionCount, err)
	}
	discussionID := decodeID(call("member", http.MethodPost, base+"/discussions", `{"title":"Design question","body":"Should we make the board keyboard-first?"}`, http.StatusCreated), "id")
	call("owner", http.MethodPost, base+"/comments", fmt.Sprintf(`{"target_type":"discussion","target_id":%q,"body":"Yes"}`, discussionID), http.StatusCreated)
	call("owner", http.MethodPatch, base+"/discussions/"+discussionID, `{"title":"Design question","body":"Should we make the board keyboard-first?","status":"locked","version":1}`, http.StatusOK)
	call("member", http.MethodPost, base+"/comments", fmt.Sprintf(`{"target_type":"discussion","target_id":%q,"body":"No more replies"}`, discussionID), http.StatusConflict)
	call("visitor", http.MethodPost, base+"/milestones", `{"title":"Forbidden milestone"}`, http.StatusNotFound)
	milestoneID := decodeID(call("owner", http.MethodPost, base+"/milestones", `{"title":"First release","description":"Ship the core workflow"}`, http.StatusCreated), "id")
	if listed := call("owner", http.MethodGet, base+"/milestones", "", http.StatusOK).Body.String(); !strings.Contains(listed, `"title":"First release"`) {
		t.Fatalf("milestone listing did not use the JSON API contract: %s", listed)
	}
	taskID := decodeID(call("member", http.MethodPost, base+"/tasks", fmt.Sprintf(`{"title":"Add list view","description":"Accessible alternative to the board","status":"todo","priority":"normal","milestone_id":%q,"assignee_ids":[%q]}`, milestoneID, users["member"]), http.StatusCreated), "id")
	if listed := call("member", http.MethodGet, base+"/tasks", "", http.StatusOK).Body.String(); !strings.Contains(listed, `"title":"Add list view"`) || !strings.Contains(listed, `"can_edit":true`) {
		t.Fatalf("task listing did not expose expected project/task permissions: %s", listed)
	}
	call("other", http.MethodPost, base+"/tasks", `{"title":"Cross-campus task","status":"todo","priority":"normal"}`, http.StatusNotFound)
	call("member", http.MethodPatch, base+"/tasks/"+taskID, `{"title":"Add list view","description":"Accessible alternative to the board","status":"in_progress","priority":"normal","milestone_id":"","version":1}`, http.StatusOK)

	var successes, conflicts int
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for _, title := range []string{"Update A", "Update B"} {
		wg.Add(1)
		go func(title string) {
			defer wg.Done()
			body := fmt.Sprintf(`{"title":%q,"description":"concurrent edit","status":"blocked","priority":"normal","milestone_id":"","version":2}`, title)
			req := httptest.NewRequest(http.MethodPatch, "http://peergit.test"+base+"/tasks/"+taskID, bytes.NewBufferString(body))
			req.AddCookie(&http.Cookie{Name: "peergit_session", Value: tokens["member"]})
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://peergit.test")
			req.Header.Set("X-CSRF-Token", csrf["member"])
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			results <- res.Code
		}(title)
	}
	wg.Wait()
	close(results)
	for status := range results {
		if status == http.StatusOK {
			successes++
		} else if status == http.StatusConflict {
			conflicts++
		} else {
			t.Fatalf("concurrent task edit returned %d", status)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent versions: successes=%d conflicts=%d", successes, conflicts)
	}
	call("visitor", http.MethodPut, base+"/follow", `{}`, http.StatusOK)
	call("visitor", http.MethodPut, base+"/follow", `{}`, http.StatusOK)
	call("visitor", http.MethodPut, base+"/bookmark", `{}`, http.StatusOK)
	call("visitor", http.MethodPut, base+"/bookmark", `{}`, http.StatusOK)
	call("visitor", http.MethodGet, "/api/v1/me/saved-projects", "", http.StatusOK)
	var signals int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM project_follows WHERE project_id=$1 AND user_id=$2`, projectID, users["visitor"]).Scan(&signals); err != nil || signals != 1 {
		t.Fatalf("follow duplicates=%d err=%v", signals, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM project_bookmarks WHERE project_id=$1 AND user_id=$2`, projectID, users["visitor"]).Scan(&signals); err != nil || signals != 1 {
		t.Fatalf("bookmark duplicates=%d err=%v", signals, err)
	}
	reportBody := fmt.Sprintf(`{"target_type":"update","target_id":%q,"reason":"spam","details":"repeated promotion"}`, updateID)
	call("visitor", http.MethodPost, base+"/reports", reportBody, http.StatusCreated)
	call("visitor", http.MethodPost, base+"/reports", reportBody, http.StatusConflict)
	call("member", http.MethodPatch, base+"/updates/"+updateID, `{"body":"stale","audience":"campus","version":1}`, http.StatusConflict)

	if _, err := pool.Exec(ctx, `DELETE FROM project_members WHERE project_id=$1 AND user_id=$2`, projectID, users["member"]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE projects SET visibility='private' WHERE id=$1`, projectID); err != nil {
		t.Fatal(err)
	}
	call("member", http.MethodGet, base+"/updates", "", http.StatusNotFound)
	call("visitor", http.MethodGet, "/api/v1/me/saved-projects", "", http.StatusOK)
	var storedEvents, taskEvents int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE tenant_id=$1 AND aggregate_id IN($2,$3,$4)`, college, updateID, discussionID, taskID).Scan(&storedEvents); err != nil || storedEvents < 3 {
		t.Fatalf("collaboration outbox events=%d err=%v", storedEvents, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM project_task_events WHERE task_id=$1`, taskID).Scan(&taskEvents); err != nil || taskEvents < 3 {
		t.Fatalf("task history events=%d err=%v", taskEvents, err)
	}
}
