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

	"peergit/internal/academic"
	"peergit/internal/health"
	"peergit/internal/identity"
	"peergit/internal/platform/errormanager"
	httpserver "peergit/internal/platform/http"
	"peergit/migrations"
)

func TestPhase6MentorshipCapacityAndRevocation(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run Phase 6 mentorship and access checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	stamp := fmt.Sprintf("%d", time.Now().UnixNano())
	var college string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES($1,$2) RETURNING id::text`, "phase6-"+stamp, "Phase 6 Test Campus").Scan(&college); err != nil {
		t.Fatal(err)
	}
	users := map[string]string{}
	tokens := map[string]string{}
	key := []byte("phase6 integration session signing key")
	for _, actor := range []string{"student", "admin", "external"} {
		collegeID, accountType := college, "campus"
		if actor == "external" {
			collegeID, accountType = "", "external"
		}
		email := actor + "+" + stamp + "@phase6.test"
		var userID string
		if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at) VALUES(NULLIF($1,'')::uuid,$2,$2,$3,$4,$5,now()) RETURNING id::text`, collegeID, email, actor, "p6_"+actor+stamp[len(stamp)-8:], accountType).Scan(&userID); err != nil {
			t.Fatal(err)
		}
		users[actor] = userID
		if actor != "external" {
			if _, err := pool.Exec(ctx, `INSERT INTO campus_verifications(college_id,user_id,source) VALUES($1,$2,'administrator_review')`, college, userID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := pool.Exec(ctx, `INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'terms','draft-1'),($1,'privacy','draft-1')`, userID); err != nil {
			t.Fatal(err)
		}
		token := "phase6-session-" + actor + "-" + stamp
		tokens[actor] = token
		if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,expires_at,mfa_verified_at) VALUES($1,NULLIF($2,'')::uuid,$3,$4,now()+interval '1 hour',CASE WHEN $5='admin' THEN now() ELSE NULL END)`, userID, collegeID, sessionHash(key, token), sessionCSRFHash(key, token), actor); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO campus_roles(college_id,user_id,role) VALUES($1,$2,'campus_admin')`, college, users["admin"]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO mfa_credentials(user_id,encrypted_secret,enabled_at) VALUES($1,decode(repeat('00',32),'hex'),now())`, users["admin"]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO external_college_access(college_id,user_id,access_kind) VALUES($1,$2,'mentor')`, college, users["external"]); err != nil {
		t.Fatal(err)
	}
	var profileID string
	if err := pool.QueryRow(ctx, `INSERT INTO mentor_profiles(college_id,user_id,status) VALUES($1,$2,'pending') RETURNING id::text`, college, users["external"]).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	errs := errormanager.NewManager(logger)
	auth := identity.NewHandler(pool, identity.Config{AppOrigin: "http://phase6.test", SessionHashKey: string(key), MFAEncryptionKey: "phase6 integration mfa encryption key"}, logger, errs)
	academicHandler := academic.NewHandler(pool, auth, errs)
	router := httpserver.NewRouter(health.NewHandler(errs, pool), logger, errs, auth.Register, academicHandler.Register)
	csrf := map[string]string{}
	for actor, token := range tokens {
		req := httptest.NewRequest(http.MethodGet, "http://phase6.test/api/v1/session", nil)
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("session for %s status=%d body=%s", actor, res.Code, res.Body.String())
		}
		var envelope struct {
			Data struct {
				CSRF string `json:"csrf_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		csrf[actor] = envelope.Data.CSRF
	}
	do := func(actor, method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, "http://phase6.test/api/v1"+path, bytes.NewReader(data))
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: tokens[actor]})
		req.Header.Set("Content-Type", "application/json")
		if method != http.MethodGet {
			req.Header.Set("Origin", "http://phase6.test")
			req.Header.Set("X-CSRF-Token", csrf[actor])
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	approved := do("admin", http.MethodPost, "/admin/mentor-profiles/"+profileID+"/decision", map[string]string{"decision": "approved"})
	if approved.Code != http.StatusOK {
		t.Fatalf("mentor approval status=%d body=%s", approved.Code, approved.Body.String())
	}
	projectIDs := make([]string, 2)
	for i := range projectIDs {
		if err := pool.QueryRow(ctx, `INSERT INTO projects(college_id,slug,title,summary,project_type,visibility,lifecycle,created_by) VALUES($1,$2,$3,'A project for mentorship testing','side_project','private','active',$4) RETURNING id::text`, college, fmt.Sprintf("phase6-project-%d-%s", i, stamp), fmt.Sprintf("Mentorship Test Project %d", i), users["student"]).Scan(&projectIDs[i]); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO project_members(college_id,project_id,user_id,role) VALUES($1,$2,$3,'owner')`, college, projectIDs[i], users["student"]); err != nil {
			t.Fatal(err)
		}
	}
	requestIDs := make([]string, 2)
	for i, projectID := range projectIDs {
		created := do("student", http.MethodPost, "/projects/"+projectID+"/mentorship-requests", map[string]string{"mentor_id": profileID, "learning_goal": fmt.Sprintf("Please review my project approach %d", i)})
		if created.Code != http.StatusCreated {
			t.Fatalf("mentorship request %d status=%d body=%s", i, created.Code, created.Body.String())
		}
		var envelope struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		requestIDs[i] = envelope.Data.ID
	}
	endsAt := time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	results := make([]int, len(requestIDs))
	var wait sync.WaitGroup
	for i, requestID := range requestIDs {
		wait.Add(1)
		go func(index int, id string) {
			defer wait.Done()
			res := do("external", http.MethodPost, "/mentorship-requests/"+id+"/decision", map[string]string{"decision": "accepted", "ends_at": endsAt})
			results[index] = res.Code
		}(i, requestID)
	}
	wait.Wait()
	accepted := 0
	var conversationID, acceptedRequestID string
	for i, code := range results {
		if code == http.StatusOK {
			accepted++
			var row struct {
				ConversationID string `json:"conversation_id"`
			}
			if err := pool.QueryRow(ctx, `SELECT id::text FROM mentorship_conversations WHERE mentorship_request_id=$1`, requestIDs[i]).Scan(&row.ConversationID); err != nil {
				t.Fatal(err)
			}
			conversationID = row.ConversationID
			acceptedRequestID = requestIDs[i]
		} else if code != http.StatusConflict {
			t.Errorf("concurrent acceptance status=%d, want 200 or 409", code)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted engagements=%d, want exactly one (responses=%v)", accepted, results)
	}
	guessed := do("external", http.MethodGet, "/mentorship/conversations/00000000-0000-4000-8000-000000000099/messages", nil)
	if guessed.Code != http.StatusNotFound {
		t.Fatalf("guessed conversation status=%d body=%s, want 404", guessed.Code, guessed.Body.String())
	}
	sent := do("student", http.MethodPost, "/mentorship/conversations/"+conversationID+"/messages", map[string]string{"client_message_id": "6f8c83e0-7a79-4a80-9574-58bbfcdbe179", "body": "Could you review the project architecture?"})
	if sent.Code != http.StatusCreated {
		t.Fatalf("mentorship message send status=%d body=%s", sent.Code, sent.Body.String())
	}
	var messageEnvelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(sent.Body.Bytes(), &messageEnvelope); err != nil {
		t.Fatal(err)
	}
	reported := do("external", http.MethodPost, "/mentorship/conversations/"+conversationID+"/reports", map[string]string{"message_id": messageEnvelope.Data.ID, "reason": "other", "details": "fixture report"})
	if reported.Code != http.StatusCreated {
		t.Fatalf("mentorship report status=%d body=%s", reported.Code, reported.Body.String())
	}
	read := do("student", http.MethodGet, "/mentorship/conversations/"+conversationID+"/messages", nil)
	if read.Code != http.StatusOK {
		t.Fatalf("student conversation read status=%d body=%s", read.Code, read.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE external_college_access SET revoked_at=now() WHERE college_id=$1 AND user_id=$2 AND access_kind='mentor'`, college, users["external"]); err != nil {
		t.Fatal(err)
	}
	var requestState, conversationState string
	if err := pool.QueryRow(ctx, `SELECT mr.state,mc.state FROM mentorship_requests mr JOIN mentorship_conversations mc ON mc.mentorship_request_id=mr.id WHERE mr.id=$1`, acceptedRequestID).Scan(&requestState, &conversationState); err != nil {
		t.Fatal(err)
	}
	if requestState != "revoked" || conversationState != "closed" {
		t.Fatalf("revoked mentor request=%q conversation=%q, want revoked/closed", requestState, conversationState)
	}
	denied := do("external", http.MethodGet, "/mentorship/conversations/"+conversationID+"/messages", nil)
	if denied.Code != http.StatusNotFound {
		t.Fatalf("revoked mentor read status=%d body=%s, want 404", denied.Code, denied.Body.String())
	}
	blocked := do("student", http.MethodPost, "/mentorship/conversations/"+conversationID+"/block", map[string]string{})
	if blocked.Code != http.StatusOK {
		t.Fatalf("mentorship block status=%d body=%s", blocked.Code, blocked.Body.String())
	}

	facultyRequest := do("student", http.MethodPost, "/me/faculty-verification-requests", map[string]string{
		"title":            "Teaching Assistant",
		"affiliation_note": "Assigned to the campus software engineering lab.",
	})
	facultyRequestID := phase6ResponseID(t, facultyRequest)
	verifiedFaculty := do("admin", http.MethodPost, "/admin/faculty-verification-requests/"+facultyRequestID+"/decision", map[string]string{
		"decision": "approved",
		"reason":   "Campus affiliation confirmed.",
	})
	if verifiedFaculty.Code != http.StatusOK {
		t.Fatalf("faculty verification status=%d body=%s", verifiedFaculty.Code, verifiedFaculty.Body.String())
	}
	if _, err := pool.Exec(ctx, `INSERT INTO faculty_profiles(college_id,user_id,title,status,verified_by) VALUES($1,$2,'Campus Administrator','active',$3) ON CONFLICT DO NOTHING`, college, users["admin"], users["admin"]); err != nil {
		t.Fatal(err)
	}

	term := do("admin", http.MethodPost, "/academic/terms", map[string]string{
		"label":     "Phase 6 " + stamp,
		"starts_at": time.Now().UTC().Format("2006-01-02"),
		"ends_at":   time.Now().AddDate(0, 4, 0).UTC().Format("2006-01-02"),
	})
	termID := phase6ResponseID(t, term)
	course := do("admin", http.MethodPost, "/academic/courses", map[string]string{
		"code":        "P6-" + stamp[len(stamp)-6:],
		"title":       "Phase 6 integration course",
		"description": "Snapshot-linked evaluation fixture.",
	})
	courseID := phase6ResponseID(t, course)
	cohort := do("admin", http.MethodPost, "/academic/courses/"+courseID+"/cohorts", map[string]string{
		"term_id": termID,
		"label":   "Section " + stamp,
	})
	cohortID := phase6ResponseID(t, cohort)
	enrollment := do("admin", http.MethodPost, "/academic/cohorts/"+cohortID+"/enrollments", map[string]string{"user_id": users["student"]})
	if enrollment.Code != http.StatusNoContent {
		t.Fatalf("course enrollment status=%d body=%s", enrollment.Code, enrollment.Body.String())
	}
	assignment := do("admin", http.MethodPost, "/academic/cohorts/"+cohortID+"/assignments", map[string]any{
		"title":       "Snapshot checkpoint",
		"brief":       "Submit the exact repository snapshot used for review.",
		"due_at":      time.Now().Add(24 * time.Hour).UTC(),
		"late_policy": "accept",
	})
	assignmentID := phase6ResponseID(t, assignment)

	var installationID, repositoryID, snapshotID string
	externalID := time.Now().UnixNano() % 1000000000000
	if err := pool.QueryRow(ctx, `INSERT INTO github_installations(college_id,external_installation_id,account_external_id,account_login,target_type,repository_selection,permissions,status,added_by) VALUES($1,$2,$3,'phase6-fixture','User','selected','{"contents":"read"}','active',$4) RETURNING id::text`, college, externalID, externalID+1000, users["student"]).Scan(&installationID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO repositories(college_id,project_id,created_by) VALUES($1,$2,$3) RETURNING id::text`, college, projectIDs[0], users["student"]).Scan(&repositoryID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO repository_bindings(college_id,repository_id,installation_id,external_repository_id,owner_login,repository_name,default_branch,visibility,access_state,linked_by) VALUES($1,$2,$3,$4,'phase6-fixture','submission','main','private','active',$5)`, college, repositoryID, installationID, externalID+2000, users["student"]); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO repository_snapshots(college_id,project_id,repository_id,binding_id,requested_by,idempotency_key,request_hash,requested_ref,commit_sha,membership_snapshot) SELECT $1,$2,$3,b.id,$4,$5,decode(repeat('11',32),'hex'),'main',repeat('b',40),'[]'::jsonb FROM repository_bindings b WHERE b.repository_id=$3 RETURNING id::text`, college, projectIDs[0], repositoryID, users["student"], "phase6-"+stamp).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	submission := do("student", http.MethodPost, "/academic/assignments/"+assignmentID+"/submissions", map[string]string{
		"project_id":  projectIDs[0],
		"snapshot_id": snapshotID,
	})
	submissionID := phase6ResponseID(t, submission)
	evaluation := do("admin", http.MethodPost, "/academic/submissions/"+submissionID+"/evaluation", map[string]string{
		"outcome":  "revise",
		"feedback": "Add one more test for the route handler.",
	})
	if evaluation.Code != http.StatusNoContent {
		t.Fatalf("snapshot-linked evaluation status=%d body=%s", evaluation.Code, evaluation.Body.String())
	}

	programProject := fmt.Sprintf("phase6-program-%s", stamp)
	var programProjectID string
	if err := pool.QueryRow(ctx, `INSERT INTO projects(college_id,slug,title,summary,project_type,visibility,lifecycle,created_by) VALUES($1,$2,'Program review project','A project submitted to an innovation program.','side_project','campus','active',$3) RETURNING id::text`, college, programProject, users["admin"]).Scan(&programProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO project_members(college_id,project_id,user_id,role) VALUES($1,$2,$3,'owner')`, college, programProjectID, users["admin"]); err != nil {
		t.Fatal(err)
	}
	programCall := do("admin", http.MethodPost, "/academic/calls", map[string]string{
		"kind": "program", "title": "Campus innovation program", "description": "A scoped review and cohort fixture.", "status": "open",
	})
	callID := phase6ResponseID(t, programCall)
	application := do("admin", http.MethodPost, "/academic/calls/"+callID+"/applications", map[string]string{
		"project_id": programProjectID,
		"statement":  "Our campus team will build and evaluate a useful service.",
	})
	applicationID := phase6ResponseID(t, application)
	assigned := do("admin", http.MethodPost, "/academic/calls/"+callID+"/reviewers", map[string]string{"user_id": users["student"]})
	if assigned.Code != http.StatusNoContent {
		t.Fatalf("program reviewer assignment status=%d body=%s", assigned.Code, assigned.Body.String())
	}
	if _, err := pool.Exec(ctx, `INSERT INTO project_members(college_id,project_id,user_id,role) VALUES($1,$2,$3,'member')`, college, programProjectID, users["student"]); err != nil {
		t.Fatal(err)
	}
	conflict := do("student", http.MethodPost, "/academic/applications/"+applicationID+"/review", map[string]string{"recommendation": "support", "notes": "Conflict fixture."})
	if conflict.Code != http.StatusForbidden {
		t.Fatalf("conflicted reviewer status=%d body=%s, want 403", conflict.Code, conflict.Body.String())
	}
	if _, err := pool.Exec(ctx, `DELETE FROM project_members WHERE project_id=$1 AND user_id=$2`, programProjectID, users["student"]); err != nil {
		t.Fatal(err)
	}
	review := do("student", http.MethodPost, "/academic/applications/"+applicationID+"/review", map[string]string{"recommendation": "support", "notes": "The scope and delivery plan fit the call."})
	if review.Code != http.StatusNoContent {
		t.Fatalf("program review status=%d body=%s", review.Code, review.Body.String())
	}
	decision := do("student", http.MethodPost, "/academic/applications/"+applicationID+"/decision", map[string]string{"decision": "accepted"})
	if decision.Code != http.StatusOK {
		t.Fatalf("program acceptance status=%d body=%s", decision.Code, decision.Body.String())
	}
	programCohort := do("student", http.MethodPost, "/academic/calls/"+callID+"/cohorts", map[string]string{"label": "Phase 6 cohort " + stamp})
	programCohortID := phase6ResponseID(t, programCohort)
	milestone := do("student", http.MethodPost, "/academic/cohorts/"+programCohortID+"/milestones", map[string]string{"title": "Midpoint review", "description": "Check the prototype against its intended users."})
	if milestone.Code != http.StatusCreated {
		t.Fatalf("program milestone status=%d body=%s", milestone.Code, milestone.Body.String())
	}
	outcome := do("student", http.MethodPost, "/academic/cohorts/"+programCohortID+"/outcomes", map[string]string{"project_id": programProjectID, "summary": "The team delivered the working prototype."})
	if outcome.Code != http.StatusOK {
		t.Fatalf("program outcome status=%d body=%s", outcome.Code, outcome.Body.String())
	}
	export := do("student", http.MethodPost, "/academic/exports", map[string]string{"type": "call", "call_id": callID, "idempotency_key": "phase6-" + stamp})
	exportID := phase6ResponseID(t, export)
	downloaded := do("student", http.MethodGet, "/academic/exports/"+exportID, nil)
	if downloaded.Code != http.StatusOK || !strings.Contains(downloaded.Body.String(), programProject) {
		t.Fatalf("authorized scoped export status=%d body=%s", downloaded.Code, downloaded.Body.String())
	}
	revokedFaculty := do("admin", http.MethodPost, "/admin/faculty/"+users["student"]+"/status", map[string]string{"status": "revoked"})
	if revokedFaculty.Code != http.StatusOK {
		t.Fatalf("faculty revocation status=%d body=%s", revokedFaculty.Code, revokedFaculty.Body.String())
	}
	deniedFacultyAction := do("student", http.MethodPost, "/academic/calls", map[string]string{
		"kind": "research", "title": "Revoked access test", "description": "This must be denied.", "status": "open",
	})
	if deniedFacultyAction.Code != http.StatusForbidden {
		t.Fatalf("revoked faculty could create a call: status=%d body=%s", deniedFacultyAction.Code, deniedFacultyAction.Body.String())
	}
}

func phase6ResponseID(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	if response.Code != http.StatusCreated && response.Code != http.StatusOK {
		t.Fatalf("request status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ID == "" {
		t.Fatalf("response did not include an id: %s", response.Body.String())
	}
	return envelope.Data.ID
}
