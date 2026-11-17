package integration_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"peergit/internal/github"
	"peergit/internal/platform/jobs"
	"peergit/internal/repository"
	"peergit/migrations"
)

func TestPhase4RepositorySnapshotTenantContracts(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run Phase 4 repository and snapshot constraints")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	var college, user, project, installation, repository, binding string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES('phase4-schema-test','Phase 4 Schema Test') RETURNING id::text`).Scan(&college); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at) VALUES($1,'phase4@fixture.test','phase4@fixture.test','Phase Four','phase4_user','campus',now()) RETURNING id::text`, college).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO projects(college_id,slug,title,summary,project_type,visibility,lifecycle,created_by) VALUES($1,'phase4-fixture','Phase Four Fixture','Schema contract fixture','side_project','campus','active',$2) RETURNING id::text`, college, user).Scan(&project); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO project_members(college_id,project_id,user_id,role) VALUES($1,$2,$3,'owner')`, college, project, user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO github_installations(college_id,external_installation_id,account_external_id,account_login,target_type,repository_selection,permissions,status,added_by) VALUES($1,41001,41002,'fixture','Organization','selected','{"contents":"read","metadata":"read"}','active',$2) RETURNING id::text`, college, user).Scan(&installation); err != nil {
		t.Fatal(err)
	}
	var secondInstallation string
	if err := pool.QueryRow(ctx, `INSERT INTO github_installations(college_id,external_installation_id,account_external_id,account_login,target_type,repository_selection,permissions,status,added_by) VALUES($1,41011,41012,'fixture-two','User','selected','{"contents":"read","metadata":"read"}','active',$2) RETURNING id::text`, college, user).Scan(&secondInstallation); err != nil {
		t.Fatal(err)
	}
	for _, install := range []string{installation, secondInstallation} {
		if _, err := pool.Exec(ctx, `INSERT INTO github_installation_user_grants(college_id,user_id,installation_id,external_user_id) VALUES($1,$2,$3,41002)`, college, user, install); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO github_repository_candidates(college_id,user_id,installation_id,external_repository_id,owner_login,repository_name,visibility,expires_at) VALUES($1,$2,$3,41003,'fixture','repo','private',now()+interval '15 minutes')`, college, user, install); err != nil {
			t.Fatalf("same GitHub repository should remain addressable through each installation: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `DELETE FROM github_repository_candidates WHERE college_id=$1 AND user_id=$2 AND installation_id=$3`, college, user, installation); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM github_repository_candidates WHERE college_id=$1 AND user_id=$2 AND installation_id=$3`, college, user, secondInstallation).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("refresh/delete for one installation affected another installation: count=%d err=%v", remaining, err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO repositories(college_id,project_id,created_by) VALUES($1,$2,$3) RETURNING id::text`, college, project, user).Scan(&repository); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO repository_bindings(college_id,repository_id,installation_id,external_repository_id,owner_login,repository_name,default_branch,visibility,linked_by) VALUES($1,$2,$3,41003,'fixture','repo','main','private',$4) RETURNING id::text`, college, repository, installation, user).Scan(&binding); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO repository_snapshots(college_id,project_id,repository_id,binding_id,requested_by,idempotency_key,request_hash,requested_ref,commit_sha,membership_snapshot) VALUES($1,$2,$3,$4,$5,'receipt-1',decode(repeat('00',32),'hex'),'main',repeat('a',40),'[]'::jsonb)`, college, project, repository, binding, user); err != nil {
		t.Fatalf("valid project-scoped snapshot receipt rejected: %v", err)
	}
	var otherCollege, otherUser, otherInstallation string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES('phase4-other-test','Phase 4 Other Test') RETURNING id::text`).Scan(&otherCollege); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at) VALUES($1,'other@fixture.test','other@fixture.test','Other User','phase4_other','campus',now()) RETURNING id::text`, otherCollege).Scan(&otherUser); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO github_installations(college_id,external_installation_id,account_external_id,account_login,target_type,repository_selection,permissions,status,added_by) VALUES($1,42001,42002,'other','Organization','selected','{"contents":"read","metadata":"read"}','active',$2) RETURNING id::text`, otherCollege, otherUser).Scan(&otherInstallation); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO repository_bindings(college_id,repository_id,installation_id,external_repository_id,owner_login,repository_name,linked_by) VALUES($1,$2,$3,42003,'other','repo',$4)`, otherCollege, repository, otherInstallation, otherUser); err == nil {
		t.Fatal("cross-campus repository binding was accepted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO repository_bindings(college_id,repository_id,installation_id,external_repository_id,owner_login,repository_name,linked_by) VALUES($1,$2,$3,41004,'fixture','second',$4)`, college, repository, installation, user); err == nil {
		t.Fatal("second current binding for a logical repository was accepted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO repository_snapshots(college_id,project_id,repository_id,binding_id,requested_by,idempotency_key,request_hash,requested_ref,commit_sha,membership_snapshot) VALUES($1,$2,$3,$4,$5,'receipt-1',decode(repeat('00',32),'hex'),'main',repeat('b',40),'[]'::jsonb)`, college, project, repository, binding, user); err == nil {
		t.Fatal("same actor's duplicate idempotency key was accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE projects SET github_revoke_deadline=now()+interval '30 days',github_revoke_owner=$2,github_revoke_installation=$3,github_revoke_previous_visibility=visibility,github_revoke_previous_lifecycle=lifecycle,visibility='private',lifecycle='archived' WHERE id=$1`, project, user, installation); err != nil {
		t.Fatalf("30-day recovery state rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE projects SET github_revoke_deadline=NULL,github_revoke_owner=NULL,github_revoke_installation=NULL,github_revoke_previous_visibility=NULL,github_revoke_previous_lifecycle=NULL,visibility='private',lifecycle='draft' WHERE id=$1`, project); err != nil {
		t.Fatalf("private project recovery rejected: %v", err)
	}
}

func TestPhase4CommitReplayAddsOnlyNewCommits(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run GitHub reconciliation replay checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}

	commitCount := 100
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/43001/access_tokens":
			_, _ = w.Write([]byte(`{"token":"fixture-token"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/fixture/repo":
			_, _ = w.Write([]byte(`{"id":43003,"full_name":"fixture/repo","default_branch":"main","private":true,"visibility":"private"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/fixture/repo/commits":
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			start := (page - 1) * 100
			if start >= commitCount {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			end := min(start+100, commitCount)
			commits := make([]map[string]any, 0, end-start)
			now := time.Now().UTC()
			for i := start; i < end; i++ {
				sequence := commitCount - i
				commits = append(commits, map[string]any{
					"sha": fmt.Sprintf("%040x", sequence),
					"commit": map[string]any{
						"message": fmt.Sprintf("commit %d", sequence),
						"author":  map[string]any{"name": "Fixture Author", "date": now.Add(-time.Duration(i) * time.Minute)},
					},
					"author": map[string]any{"id": 43004, "login": "fixture-author"},
					"stats":  map[string]any{"additions": 1, "deletions": 0, "total": 1},
				})
			}
			_ = json.NewEncoder(w).Encode(commits)
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	slug := fmt.Sprintf("phase4-replay-%d", time.Now().UnixNano())
	handle := fmt.Sprintf("p4r_%d", time.Now().UnixNano()%1_000_000_000)
	var college, user, projectID, installation, repoID, binding string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES($1,'Phase 4 Replay') RETURNING id::text`, slug).Scan(&college); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at) VALUES($1,$2,$2,'Replay User',$3,'campus',now()) RETURNING id::text`, college, slug+"@fixture.test", handle).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO projects(college_id,slug,title,summary,project_type,visibility,lifecycle,created_by) VALUES($1,$2,'Replay Project','Phase 4 replay fixture','open_source','private','draft',$3) RETURNING id::text`, college, slug, user).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO project_members(college_id,project_id,user_id,role) VALUES($1,$2,$3,'owner')`, college, projectID, user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO github_installations(college_id,external_installation_id,account_external_id,account_login,target_type,repository_selection,permissions,status,added_by) VALUES($1,43001,43002,'fixture','Organization','selected','{"contents":"read"}','active',$2) RETURNING id::text`, college, user).Scan(&installation); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO repositories(college_id,project_id,created_by) VALUES($1,$2,$3) RETURNING id::text`, college, projectID, user).Scan(&repoID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO repository_bindings(college_id,repository_id,installation_id,external_repository_id,owner_login,repository_name,default_branch,visibility,linked_by) VALUES($1,$2,$3,43003,'fixture','repo','main','private',$4) RETURNING id::text`, college, repoID, installation, user).Scan(&binding); err != nil {
		t.Fatal(err)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	api := &github.Client{HTTP: provider.Client(), API: provider.URL}
	handler := repository.JobHandlers(pool, api, "43000", privateKey, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))["github_reconcile"]
	claim := jobs.Claim{Payload: json.RawMessage(fmt.Sprintf(`{"binding_id":%q}`, binding))}
	reconcile := func(want int) {
		t.Helper()
		if _, err := handler(ctx, claim); err != nil {
			t.Fatalf("reconcile failed: %v", err)
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM contributions WHERE repository_id=$1`, repoID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Fatalf("contribution count=%d, want %d", count, want)
		}
	}

	reconcile(100)
	commitCount = 130
	reconcile(130)
	reconcile(130)
}
