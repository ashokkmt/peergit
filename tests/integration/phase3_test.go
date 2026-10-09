package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"log/slog"

	"peergit/internal/campus"
	"peergit/internal/health"
	"peergit/internal/identity"
	"peergit/internal/media"
	"peergit/internal/platform/errormanager"
	httpserver "peergit/internal/platform/http"
	"peergit/internal/platform/storage"
	"peergit/internal/project"
	"peergit/internal/recruitment"
	"peergit/migrations"
)

func TestPhase3ProjectRecruitmentAndOwnershipFlows(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run project/recruitment integration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	var college string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES('phase3-test','Phase 3 Test') RETURNING id::text`).Scan(&college); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO college_domains(college_id,domain,verified_at) VALUES($1,'phase3.test',now())`, college); err != nil {
		t.Fatal(err)
	}
	key := []byte("phase3 integration session signing key with entropy")
	users := map[string]string{}
	tokens := map[string]string{}
	csrfTokens := map[string]string{}
	for _, name := range []string{"lead", "applicant", "applicant2", "invitee", "race"} {
		email := name + "@phase3.test"
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at) VALUES($1,$2,$2,$3,$4,'campus',now()) RETURNING id::text`, college, email, name, "phase3_"+name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		users[name] = id
		if _, err := pool.Exec(ctx, `INSERT INTO campus_verifications(college_id,user_id,source) VALUES($1,$2,'administrator_review')`, college, id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'terms','draft-1'),($1,'privacy','draft-1')`, id); err != nil {
			t.Fatal(err)
		}
		token := "phase3-session-" + name
		tokens[name] = token
		if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, id, college, sessionHash(key, token), sessionCSRFHash(key, token)); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	errs := errormanager.NewManager(logger)
	auth := identity.NewHandler(pool, identity.Config{AppOrigin: "http://peergit.test", SessionHashKey: string(key), MFAEncryptionKey: "phase3 integration mfa encryption key", CookieSecure: false}, logger, errs)
	projectHandler := project.NewHandler(pool, auth, logger, errs)
	recruitmentHandler := recruitment.NewHandler(pool, auth, logger, errs)
	mediaHandler := media.NewHandler(pool, storage.New("http://127.0.0.1:1", "test", "us-east-1", "key", "secret"), auth, errs)
	campusHandler := campus.NewHandler(pool, auth, "http://peergit.test", errs, string(key), "phase3 delivery encryption secret")
	router := httpserver.NewRouter(health.NewHandler(logger, errs, pool), logger, errs, auth.Register, campusHandler.Register, mediaHandler.Register, projectHandler.Register, recruitmentHandler.Register)
	loadCSRF := func(actor string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/session", nil)
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: tokens[actor]})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("session for %s status=%d body=%s", actor, res.Code, res.Body.String())
		}
		var envelope struct {
			Data struct {
				CSRFToken string `json:"csrf_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil || len(envelope.Data.CSRFToken) < 32 {
			t.Fatalf("session for %s did not return a valid CSRF token: %v", actor, err)
		}
		csrfTokens[actor] = envelope.Data.CSRFToken
	}
	for actor := range tokens {
		loadCSRF(actor)
	}
	call := func(actor, method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://peergit.test"+path, bytes.NewBufferString(body))
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: tokens[actor]})
		if method != "GET" {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://peergit.test")
			req.Header.Set("X-CSRF-Token", csrfTokens[actor])
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, path, res.Code, want, res.Body.String())
		}
		return res
	}
	decodeID := func(rec *httptest.ResponseRecorder, path ...string) string {
		t.Helper()
		var env struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		var value any = env.Data
		for _, key := range path {
			next, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("response has no %q: %s", key, rec.Body.String())
			}
			value = next[key]
		}
		id, ok := value.(string)
		if !ok || id == "" {
			t.Fatalf("response value is not an ID: %v", value)
		}
		return id
	}
	seedImportedProject := func(slug, title, summary, visibility, lifecycle string, repositoryID int64) string {
		t.Helper()
		var id, installation, repository string
		if err := pool.QueryRow(ctx, `INSERT INTO projects(college_id,slug,title,summary,project_type,visibility,lifecycle,created_by) VALUES($1,$2,$3,$4,'open_source',$5,$6,$7) RETURNING id::text`, college, slug, title, summary, visibility, lifecycle, users["lead"]).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO project_members(college_id,project_id,user_id,role) VALUES($1,$2,$3,'owner')`, college, id, users["lead"]); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO github_installations(college_id,external_installation_id,account_external_id,account_login,target_type,repository_selection,permissions,status,added_by) VALUES($1,$2,$3,'phase3-fixture','User','selected','{"contents":"read"}','active',$4) RETURNING id::text`, college, repositoryID+100000, repositoryID+200000, users["lead"]).Scan(&installation); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO repositories(college_id,project_id,created_by) VALUES($1,$2,$3) RETURNING id::text`, college, id, users["lead"]).Scan(&repository); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO repository_bindings(college_id,repository_id,installation_id,external_repository_id,owner_login,repository_name,default_branch,visibility,linked_by) VALUES($1,$2,$3,$4,'phase3-fixture',$5,'main','private',$6)`, college, repository, installation, repositoryID, slug, users["lead"]); err != nil {
			t.Fatal(err)
		}
		return id
	}
	call("lead", http.MethodPost, "/api/v1/projects", `{"slug":"must-not-exist","title":"Form project","summary":"Repository-free project creation is disabled","project_type":"side_project","visibility":"private","lifecycle":"draft"}`, http.StatusConflict)
	hiddenProjectID := seedImportedProject("phase3-private", "Private notes", "A private workspace", "private", "draft", 930001)
	hiddenRoleID := decodeID(call("lead", http.MethodPost, "/api/v1/projects/"+hiddenProjectID+"/roles", `{"title":"Private role","description":"Do not disclose this role","openings":1}`, http.StatusCreated), "id")
	var prerequisiteCount int
	if err := pool.QueryRow(ctx, `SELECT cardinality(prerequisite_skills) FROM project_roles WHERE id=$1`, hiddenRoleID).Scan(&prerequisiteCount); err != nil || prerequisiteCount != 0 {
		t.Fatalf("role without prerequisite_skills stored %d skills: %v", prerequisiteCount, err)
	}
	visibleRoles := call("applicant", http.MethodGet, "/api/v1/projects/"+hiddenProjectID+"/roles", "", http.StatusOK)
	var roleEnvelope struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(visibleRoles.Body.Bytes(), &roleEnvelope); err != nil {
		t.Fatal(err)
	}
	if len(roleEnvelope.Data.Items) != 0 {
		t.Fatalf("private project exposed roles to a non-member: role=%s", hiddenRoleID)
	}
	var existingSkillID string
	if err := pool.QueryRow(ctx, `INSERT INTO skills(slug,name) VALUES('go','Go') RETURNING id::text`).Scan(&existingSkillID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_skills(user_id,skill_id) VALUES($1,$2)`, users["lead"], existingSkillID); err != nil {
		t.Fatal(err)
	}
	projectID := seedImportedProject("phase3-robots", "Robotics team", "Build helpful robots", "campus", "active", 930002)
	if _, err := pool.Exec(ctx, `INSERT INTO project_skills(college_id,project_id,skill_id) VALUES($1,$2,$3)`, college, projectID, existingSkillID); err != nil {
		t.Fatal(err)
	}
	publicProjectID := seedImportedProject("phase3-public", "Public project", "A public preview", "public", "active", 930003)
	guestList := httptest.NewRecorder()
	router.ServeHTTP(guestList, httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/projects", nil))
	if guestList.Code != http.StatusOK {
		t.Fatalf("guest public project list status=%d body=%s", guestList.Code, guestList.Body.String())
	}
	var guestProjects struct {
		Data struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(guestList.Body.Bytes(), &guestProjects); err != nil {
		t.Fatal(err)
	}
	publicFound, privateFound := false, false
	for _, item := range guestProjects.Data.Items {
		publicFound = publicFound || item.ID == publicProjectID
		privateFound = privateFound || item.ID == hiddenProjectID || item.ID == projectID
	}
	if !publicFound || privateFound {
		t.Fatalf("guest project scope public=%v private_or_campus=%v items=%+v", publicFound, privateFound, guestProjects.Data.Items)
	}
	guestDetail := httptest.NewRecorder()
	router.ServeHTTP(guestDetail, httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/projects/"+publicProjectID, nil))
	var publicPreview struct {
		Data struct {
			Description string           `json:"description"`
			Members     []map[string]any `json:"members"`
			ViewerRole  string           `json:"viewer_role"`
		} `json:"data"`
	}
	if guestDetail.Code != http.StatusOK || json.Unmarshal(guestDetail.Body.Bytes(), &publicPreview) != nil {
		t.Fatalf("guest public project detail status=%d body=%s", guestDetail.Code, guestDetail.Body.String())
	}
	if publicPreview.Data.Description != "" || publicPreview.Data.Members == nil || len(publicPreview.Data.Members) != 0 || publicPreview.Data.ViewerRole != "" {
		t.Fatalf("guest public project response disclosed member-only fields: %+v", publicPreview.Data)
	}
	guestPrivate := httptest.NewRecorder()
	router.ServeHTTP(guestPrivate, httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/projects/"+projectID, nil))
	if guestPrivate.Code != http.StatusNotFound {
		t.Fatalf("guest read campus project status=%d body=%s", guestPrivate.Code, guestPrivate.Body.String())
	}
	var projectSkillID string
	if err := pool.QueryRow(ctx, `SELECT ps.skill_id::text FROM project_skills ps JOIN skills s ON s.id=ps.skill_id WHERE ps.project_id=$1 AND s.slug='go'`, projectID).Scan(&projectSkillID); err != nil || projectSkillID != existingSkillID {
		t.Fatalf("project skill=%q err=%v, want existing user skill %q", projectSkillID, err, existingSkillID)
	}
	roleID := decodeID(call("lead", http.MethodPost, "/api/v1/projects/"+projectID+"/roles", `{"title":"Builder","description":"Build a robot module","openings":2,"good_first_task":true,"difficulty":"beginner"}`, http.StatusCreated), "id")
	call("applicant", http.MethodGet, "/api/v1/projects/"+projectID+"/applications", "", http.StatusForbidden)
	call("applicant", http.MethodGet, "/api/v1/projects/"+projectID+"/people?q=cam", "", http.StatusForbidden)
	app1 := call("applicant", http.MethodPost, "/api/v1/projects/"+projectID+"/applications", `{"role_id":"`+roleID+`","message":"I want to build the sensor module"}`, http.StatusCreated)
	app1ID := decodeID(app1, "id")
	call("applicant", http.MethodPost, "/api/v1/projects/"+projectID+"/applications", `{"role_id":"`+roleID+`","message":"duplicate"}`, http.StatusConflict)
	call("lead", http.MethodPost, "/api/v1/projects/"+projectID+"/applications/"+app1ID+"/decision", `{"decision":"accepted","version":1}`, http.StatusOK)
	var member bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2 AND role='member')`, projectID, users["applicant"]).Scan(&member); err != nil || !member {
		t.Fatalf("accepted applicant is not a member: member=%v err=%v", member, err)
	}
	app2ID := decodeID(call("applicant2", http.MethodPost, "/api/v1/projects/"+projectID+"/applications", `{"role_id":"`+roleID+`","message":"I can test the robot"}`, http.StatusCreated), "id")
	call("applicant2", http.MethodPost, "/api/v1/projects/"+projectID+"/applications/"+app2ID+"/withdraw", `{"version":1}`, http.StatusOK)
	reappliedID := decodeID(call("applicant2", http.MethodPost, "/api/v1/projects/"+projectID+"/applications", `{"role_id":"`+roleID+`","message":"A new application after withdrawal"}`, http.StatusCreated), "id")
	call("lead", http.MethodPost, "/api/v1/projects/"+projectID+"/applications/"+reappliedID+"/decision", `{"decision":"rejected","version":1}`, http.StatusOK)
	raceRoleID := decodeID(call("lead", http.MethodPost, "/api/v1/projects/"+projectID+"/roles", `{"title":"Test role","description":"Verify the prototype","openings":1}`, http.StatusCreated), "id")
	raceApp1 := decodeID(call("applicant2", http.MethodPost, "/api/v1/projects/"+projectID+"/applications", `{"role_id":"`+raceRoleID+`","message":"I can test"}`, http.StatusCreated), "id")
	raceApp2 := decodeID(call("race", http.MethodPost, "/api/v1/projects/"+projectID+"/applications", `{"role_id":"`+raceRoleID+`","message":"I can also test"}`, http.StatusCreated), "id")
	statuses := make(chan int, 2)
	for _, app := range []string{raceApp1, raceApp2} {
		go func(app string) {
			req := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/projects/"+projectID+"/applications/"+app+"/decision", bytes.NewBufferString(`{"decision":"accepted","version":1}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://peergit.test")
			req.Header.Set("X-CSRF-Token", csrfTokens["lead"])
			req.AddCookie(&http.Cookie{Name: "peergit_session", Value: tokens["lead"]})
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			statuses <- res.Code
		}(app)
	}
	first, second := <-statuses, <-statuses
	if !((first == http.StatusOK && second == http.StatusConflict) || (first == http.StatusConflict && second == http.StatusOK)) {
		t.Fatalf("concurrent accept statuses=(%d,%d), want one success and one capacity conflict", first, second)
	}
	var acceptedRaceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM applications WHERE project_role_id=$1 AND status='accepted'`, raceRoleID).Scan(&acceptedRaceCount); err != nil || acceptedRaceCount != 1 {
		t.Fatalf("concurrent role accepted count=%d err=%v", acceptedRaceCount, err)
	}
	var raceRoleStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM project_roles WHERE id=$1`, raceRoleID).Scan(&raceRoleStatus); err != nil || raceRoleStatus != "closed" {
		t.Fatalf("filled role status=%q err=%v, want closed", raceRoleStatus, err)
	}
	call("lead", http.MethodDelete, "/api/v1/projects/"+projectID+"/members/"+users["lead"], "", http.StatusConflict)
	inviteResponse := call("lead", http.MethodPost, "/api/v1/projects/"+projectID+"/invitations", `{"user_id":"`+users["invitee"]+`","role":"maintainer"}`, http.StatusCreated)
	inviteID := decodeID(inviteResponse, "id")
	var inviteEnvelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(inviteResponse.Body.Bytes(), &inviteEnvelope); err != nil {
		t.Fatal(err)
	}
	call("invitee", http.MethodPost, "/api/v1/project-invitations/"+inviteID+"/accept", `{"token":"wrong-token-that-has-more-than-32-characters"}`, http.StatusNotFound)
	call("invitee", http.MethodPost, "/api/v1/project-invitations/"+inviteID+"/accept", `{"token":"`+inviteEnvelope.Data.Token+`"}`, http.StatusOK)
	var invitationMember bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2 AND role='maintainer')`, projectID, users["invitee"]).Scan(&invitationMember); err != nil || !invitationMember {
		t.Fatalf("accepted invitation did not create membership: member=%v err=%v", invitationMember, err)
	}
	call("lead", http.MethodDelete, "/api/v1/projects/"+projectID+"/members/"+users["invitee"], "", http.StatusNoContent)
	call("invitee", http.MethodGet, "/api/v1/projects/"+projectID+"/applications", "", http.StatusForbidden)
	call("invitee", http.MethodPost, "/api/v1/projects/"+projectID+"/roles", `{"title":"No longer authorized","description":"Must be denied","openings":1}`, http.StatusNotFound)
	var auditCount, outboxCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='project.role_created' AND resource_id IN (SELECT id FROM project_roles WHERE project_id=$2)`, college, projectID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE tenant_id=$1 AND aggregate_type='application' AND aggregate_id IN (SELECT id FROM applications WHERE project_role_id IN (SELECT id FROM project_roles WHERE project_id=$2))`, college, projectID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if auditCount < 1 || outboxCount < 1 {
		t.Fatalf("recruitment work not recorded: audit=%d outbox=%d", auditCount, outboxCount)
	}
	var otherCollege string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES('phase3-other','Phase 3 Other') RETURNING id::text`).Scan(&otherCollege); err != nil {
		t.Fatal(err)
	}
	otherEmail := "other@phase3.other"
	var otherUser string
	if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at) VALUES($1,$2,$2,'Other Student','phase3_other','campus',now()) RETURNING id::text`, otherCollege, otherEmail).Scan(&otherUser); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO campus_verifications(college_id,user_id,source) VALUES($1,$2,'administrator_review')`, otherCollege, otherUser); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'terms','draft-1'),($1,'privacy','draft-1')`, otherUser); err != nil {
		t.Fatal(err)
	}
	otherToken := "phase3-session-other"
	tokens["other"] = otherToken
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, otherUser, otherCollege, sessionHash(key, otherToken), sessionCSRFHash(key, otherToken)); err != nil {
		t.Fatal(err)
	}
	loadCSRF("other")
	call("other", http.MethodGet, "/api/v1/projects/"+projectID, "", http.StatusNotFound)
}
