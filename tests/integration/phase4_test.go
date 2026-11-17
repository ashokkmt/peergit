package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

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
