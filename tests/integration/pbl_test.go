package integration_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"peergit/internal/academic"
	"peergit/internal/campus"
	"peergit/internal/collaboration"
	"peergit/internal/github"
	"peergit/internal/health"
	"peergit/internal/identity"
	"peergit/internal/platform/errormanager"
	httpserver "peergit/internal/platform/http"
	"peergit/internal/platform/jobs"
	"peergit/internal/platform/storage"
	"peergit/internal/repository"
	"peergit/migrations"
)

func TestPhase6APBLRelease(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for the Phase 6A PostgreSQL checks")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	pool := testPool(t, ctx, dsn)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	execSQL := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	id := func(query string, args ...any) string {
		t.Helper()
		var result string
		if err := pool.QueryRow(ctx, query, args...).Scan(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	tenant := id(`INSERT INTO colleges(slug,name) VALUES('pbl','PBL Test Campus') RETURNING id::text`)
	other := id(`INSERT INTO colleges(slug,name) VALUES('pbl-other','Other Test Campus') RETURNING id::text`)
	key := []byte("phase 6a synthetic integration signing key")
	actors := map[string]string{}
	for _, actor := range []string{"leader", "member", "faculty", "coordinator", "outsider"} {
		college := tenant
		if actor == "outsider" {
			college = other
		}
		uid := id(`INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type)
			VALUES($1,$2,$2,$3,$3,'campus') RETURNING id::text`, college, actor+"@fixtures.test", actor)
		actors[actor] = uid
		execSQL(`INSERT INTO campus_verifications(college_id,user_id,source) VALUES($1,$2,'administrator_review')`, college, uid)
		execSQL(`INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'terms','draft-1'),($1,'privacy','draft-1')`, uid)
		execSQL(`INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,expires_at,mfa_verified_at)
			VALUES($1,$2,$3,$4,now()+interval '1 hour',CASE WHEN $5='coordinator' THEN now() ELSE NULL END)`,
			uid, college, sessionHash(key, actor), sessionCSRFHash(key, actor), actor)
	}
	execSQL(`INSERT INTO campus_roles(college_id,user_id,role) VALUES($1,$2,'campus_admin')`, tenant, actors["coordinator"])
	execSQL(`INSERT INTO mfa_credentials(user_id,encrypted_secret,enabled_at) VALUES($1,decode(repeat('00',32),'hex'),now())`, actors["coordinator"])
	execSQL(`INSERT INTO faculty_profiles(college_id,user_id,title,status,verified_by) VALUES($1,$2,'Instructor','active',$3),($1,$3,'Coordinator','active',$3)`,
		tenant, actors["faculty"], actors["coordinator"])
	term := id(`INSERT INTO academic_terms(college_id,label,starts_at,ends_at,created_by)
		VALUES($1,'Term 1',current_date,current_date+90,$2) RETURNING id::text`, tenant, actors["coordinator"])
	offerings := map[string]string{}
	for _, subject := range []string{"OS", "DBMS"} {
		course := id(`INSERT INTO academic_courses(college_id,course_code,title,created_by) VALUES($1,$2,$3,$4) RETURNING id::text`,
			tenant, subject, "Course "+subject, actors["coordinator"])
		cohort := id(`INSERT INTO academic_cohorts(college_id,course_id,term_id,label,created_by) VALUES($1,$2,$3,'A1',$4) RETURNING id::text`,
			tenant, course, term, actors["coordinator"])
		for _, actor := range []string{"leader", "member"} {
			execSQL(`INSERT INTO academic_enrollments(college_id,cohort_id,user_id) VALUES($1,$2,$3)`, tenant, cohort, actors[actor])
		}
		offerings[subject] = course
	}
	forge := repository.ForgeConfig{URL: os.Getenv("TEST_PBL_FORGE_URL"), Owner: os.Getenv("TEST_PBL_FORGE_OWNER"), Token: os.Getenv("TEST_PBL_FORGE_TOKEN"),
		WebhookURL: "http://host.docker.internal:8080/api/v1/pbl/forgejo/webhooks", WebhookSecret: "phase6a synthetic webhook signing key"}
	var objects *storage.Store
	if forge.URL != "" {
		objects = storage.New("http://127.0.0.1:8333", "peergit-local", "us-east-1", "local-object-access", "local-object-secret")
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	errs := errormanager.NewManager(logger)
	auth := identity.NewHandler(pool, identity.Config{AppOrigin: "http://pbl.test", SessionHashKey: string(key)}, logger, errs)
	academicHandler := academic.NewHandler(pool, auth, errs)
	fakeGitHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			if r.ParseForm() != nil || r.Form.Get("code_verifier") == "" {
				http.Error(w, "missing fixture PKCE", 400)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"synthetic-pbl-token","token_type":"bearer"}`)
		case "/user":
			_, _ = io.WriteString(w, `{"id":49101,"login":"fixture"}`)
		case "/user/installations":
			_, _ = io.WriteString(w, `{"installations":[{"id":49102}]}`)
		case "/user/installations/49102/repositories":
			_, _ = io.WriteString(w, `{"repositories":[{"id":49103,"full_name":"fixture/private-pbl","permissions":{"admin":true}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer fakeGitHub.Close()
	gh := github.New()
	gh.API = fakeGitHub.URL
	app := repository.AppConfig{Forge: forge, Origin: "http://pbl.test", OAuthClientID: "pbl-fixture",
		OAuthClientSecret: "synthetic-test-secret", OAuthRedirectURL: "http://pbl.test/api/v1/github/authorization/callback",
		OAuthTokenURL: fakeGitHub.URL + "/oauth/token", OAuthAuthorizeURL: fakeGitHub.URL + "/authorize"}
	repo := repository.NewHandler(pool, auth, gh, objects, app, logger, errs)
	comms := collaboration.NewHandler(pool, auth, logger, errs, key)
	campusHandler := campus.NewHandler(pool, auth, "http://pbl.test", errs, "", "")
	router := httpserver.NewRouter(health.NewHandler(errs, pool), logger, errs, auth.Register, academicHandler.Register, repo.Register, comms.Register, campusHandler.Register)
	csrf := map[string]string{}
	do := func(actor, method, route string, input any, want int) map[string]any {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, "http://pbl.test/api/v1"+route, bytes.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: actor})
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://pbl.test")
		req.Header.Set("X-CSRF-Token", csrf[actor])
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s as %s: got %d want %d: %s", method, route, actor, res.Code, want, res.Body.String())
		}
		var envelope struct{ Data map[string]any }
		_ = json.Unmarshal(res.Body.Bytes(), &envelope)
		return envelope.Data
	}
	for _, actor := range []string{"leader", "member", "faculty", "coordinator", "outsider"} {
		csrf[actor] = do(actor, "GET", "/session", nil, 200)["csrf_token"].(string)
	}
	root := "/academic/pbl"
	now := time.Now().UTC()
	for subject, course := range offerings {
		body := map[string]any{"course_id": course, "term_id": term, "closes_at": now.Add(time.Hour), "phase_one_due": now.Add(2 * time.Hour),
			"phase_two_due": now.Add(3 * time.Hour), "phase_three_due": now.Add(4 * time.Hour)}
		do("leader", "POST", root+"/offerings", body, 403)
		offerings[subject] = do("coordinator", "POST", root+"/offerings", body, 200)["id"].(string)
	}
	roster := map[string]any{"title": "Coursework team", "members": []map[string]string{
		{"name": "Leader", "roll_number": "private-001", "section": "A1"},
		{"name": "Member", "roll_number": "private-002", "section": "B1"},
	}}
	do("outsider", "POST", root+"/offerings/"+offerings["OS"]+"/teams", roster, 404)
	var osTeam string
	var dbmsTeam string
	for _, subject := range []string{"OS", "DBMS"} {
		team := do("leader", "POST", root+"/offerings/"+offerings[subject]+"/teams", roster, 200)["id"].(string)
		if subject == "OS" {
			osTeam = team
		} else {
			dbmsTeam = team
		}
		base := root + "/teams/" + team
		do("leader", "POST", base+"/rooms/team", map[string]string{}, 404)
		match := func(a, b string) map[string]any {
			return map[string]any{"matches": []map[string]string{
				{"entry_id": id(`SELECT id::text FROM pbl_team_members WHERE team_id=$1 AND student_name='Leader'`, team), "handle": a},
				{"entry_id": id(`SELECT id::text FROM pbl_team_members WHERE team_id=$1 AND student_name='Member'`, team), "handle": b},
			}}
		}
		do("coordinator", "PUT", base+"/roster", match("leader", "outsider"), 409)
		do("coordinator", "PUT", base+"/roster", match("leader", "member"), 200)
		do("leader", "PUT", base+"/faculty", map[string]string{"faculty_id": actors["faculty"]}, 403)
		do("coordinator", "PUT", base+"/faculty", map[string]string{"faculty_id": actors["faculty"]}, 200)
		memberView, _ := json.Marshal(do("member", "GET", root, nil, 200))
		if bytes.Contains(memberView, []byte("PRIVATE-")) || bytes.Contains(memberView, []byte("roll_number")) {
			t.Fatal("private roster identifier leaked to team workspace")
		}
	}
	base := root + "/teams/" + osTeam
	room := do("member", "POST", base+"/rooms/faculty", map[string]string{}, 201)["id"].(string)
	teamRoom := do("member", "POST", base+"/rooms/team", map[string]string{}, 201)["id"].(string)
	do("member", "POST", "/conversations/"+teamRoom+"/messages", map[string]string{"client_message_id": "00000000-0000-7000-8000-000000000017", "body": "Coursework update"}, 201)
	do("faculty", "GET", "/conversations/"+teamRoom+"/messages", nil, 200)
	do("outsider", "GET", "/conversations/"+room+"/messages", nil, 404)
	do("leader", "POST", base+"/idea", map[string]string{"idea": "Compare indexing strategies."}, 200)
	do("faculty", "PUT", base+"/idea", map[string]string{"decision": "approved", "feedback": "Compare the query plans."}, 200)
	for stage := 0; stage <= 3; stage++ {
		do("faculty", "PUT", base+"/attendance", map[string]any{"student_id": actors["member"], "stage": stage, "present": true}, 200)
	}
	do("coordinator", "PUT", base+"/faculty", map[string]string{"faculty_id": actors["coordinator"]}, 200)
	do("faculty", "GET", "/conversations/"+teamRoom+"/messages", nil, 404)
	do("member", "GET", "/conversations/"+room+"/messages", nil, 404)
	do("leader", "PUT", "/me/consent", map[string]any{"purpose": "recruiter_discovery", "policy_version": "draft-1", "granted": true}, 400)
	do("leader", "PATCH", "/me/profile", map[string]any{"display_name": "Leader", "academic": map[string]any{"course_of_study": "Computing", "year": 2, "semester": 3, "section": "A1"}}, 200)
	do("leader", "PUT", "/me/consent", map[string]any{"purpose": "profile_discovery", "policy_version": "draft-1", "granted": true}, 200)
	search := do("member", "GET", "/search?course_of_study=computing&year=2&semester=3&section=a1", nil, 200)["people"].([]any)
	if len(search) != 1 {
		t.Fatalf("academic filter returned %d people, want 1", len(search))
	}
	do("leader", "PUT", "/me/consent", map[string]any{"purpose": "profile_discovery", "policy_version": "draft-1", "granted": false}, 200)
	search = do("member", "GET", "/search?course_of_study=computing&year=2&semester=3&section=a1", nil, 200)["people"].([]any)
	if len(search) != 0 {
		t.Fatal("discovery returned a student after consent revocation")
	}
	if forge.URL == "" {
		t.Run("real Forgejo", func(t *testing.T) { t.Skip("start the isolated Forgejo fixture through scripts/test.mjs") })
		return
	}
	ordinary := id(`INSERT INTO projects(college_id,slug,title,summary,project_type,created_by)
		VALUES($1,'ordinary-fixture','Ordinary fixture','Ordinary GitHub project fixture','side_project',$2) RETURNING id::text`, tenant, actors["leader"])
	do("leader", "POST", root+"/teams/"+ordinary+"/repository", map[string]string{}, 404)
	do("leader", "POST", root+"/teams/"+ordinary+"/git-credential", nil, 404)
	do("member", "POST", base+"/repository", map[string]string{}, 404)
	first := do("leader", "POST", base+"/repository", map[string]string{}, 200)
	second := do("leader", "POST", base+"/repository", map[string]string{}, 200)
	if first["project_id"] != second["project_id"] {
		t.Fatal("provisioning retry allocated a second project")
	}
	handlers := repository.PBLJobHandlers(pool, github.New(), "", nil, objects, logger, forge)
	queue := jobs.Queue{Pool: pool}
	runJob := func(name string) {
		t.Helper()
		claim, err := queue.ClaimTypes(ctx, "pbl-test", time.Minute, []string{name})
		if err != nil {
			t.Fatal(err)
		}
		key, err := handlers[name](ctx, claim)
		if err != nil {
			t.Fatalf("%s failed: %v", name, err)
		}
		if err = queue.Complete(ctx, claim, key); err != nil {
			t.Fatal(err)
		}
	}
	runJob("pbl_provision")
	status := do("leader", "GET", base+"/repository", nil, 200)
	if status["state"] != "ready" || status["can_git"] != true {
		t.Fatal("ready private Forgejo repo was not exposed to the confirmed leader")
	}
	do("outsider", "POST", base+"/git-credential", nil, 404)
	if protected := do("leader", "GET", "/my/projects", nil, 200)["items"].([]any); len(protected) != 0 {
		t.Fatal("ordinary project picker exposed the private Forgejo repository")
	}
	credential := do("leader", "POST", base+"/git-credential", nil, 201)["password"].(string)
	svr := httptest.NewServer(router)
	defer svr.Close()
	remote := svr.URL + "/api/v1/pbl/git/" + osTeam + ".git"
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0")
		if len(args) > 0 && (args[0] == "push" || args[0] == "ls-remote" || args[0] == "clone") {
			cmd.Env = append(cmd.Env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.extraHeader",
				"GIT_CONFIG_VALUE_0=Authorization: Basic "+testBasicCredential("peergit", credential))
		}
		result, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\n%s", args[0], err, result)
		}
		return strings.TrimSpace(string(result))
	}
	git("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Synthetic coursework"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-m", "Coursework commit")
	sha := git("rev-parse", "HEAD")
	git("push", remote, "main")
	if !strings.Contains(git("ls-remote", remote), sha) {
		t.Fatal("the pushed revision is not visible to the Git client")
	}
	clone := filepath.Join(t.TempDir(), "cloned")
	git("clone", remote, clone)
	if got := git("-C", clone, "rev-parse", "HEAD"); got != sha {
		t.Fatalf("private clone resolved %s instead of %s", got, sha)
	}
	historyURL := svr.URL + "/api/v1" + base + "/source?branch_history=main"
	for deadline := time.Now().Add(10 * time.Second); ; {
		req, err := http.NewRequestWithContext(ctx, "GET", historyURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: "leader"})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode == 200 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Forgejo did not index the first push; source history returned %d", res.StatusCode)
		}
		time.Sleep(100 * time.Millisecond)
	}
	do("leader", "GET", base+"/source?ref="+sha, nil, 200)
	do("leader", "GET", base+"/source?view=diff&ref="+sha, nil, 200)
	do("leader", "GET", base+"/source?branch_history=main", nil, 200)
	runJob("pbl_reconcile")
	evidence := do("leader", "GET", base+"/evidence", nil, 200)
	commits, pushes := evidence["commits"].([]any), evidence["pushes"].([]any)
	if len(commits) != 1 || len(pushes) != 1 || commits[0].(map[string]any)["verified_author"] != "" ||
		pushes[0].(map[string]any)["pusher"] != "Leader" {
		t.Fatalf("push authentication and unsigned commit authorship were not kept separate: %#v", evidence)
	}
	receipt := do("leader", "POST", base+"/checkpoints/1", map[string]string{"sha": sha}, 201)
	if receipt["on_time"] != true || receipt["sha"] != sha {
		t.Fatal("checkpoint did not return the exact pushed commit")
	}
	do("leader", "POST", base+"/checkpoints/1", map[string]string{"sha": sha}, 200)
	do("leader", "POST", base+"/checkpoints/1", map[string]string{"sha": strings.Repeat("a", 40)}, 409)
	runJob("pbl_snapshot")
	do("coordinator", "GET", base+"/checkpoints/1/download", nil, 200)
	do("outsider", "GET", base+"/checkpoints/1/download", nil, 404)
	do("coordinator", "GET", base+"/audit.csv", nil, 200)
	do("outsider", "GET", base+"/audit.csv", nil, 404)
	do("outsider", "GET", base+"/work", nil, 404)
	do("coordinator", "POST", base+"/work", map[string]string{"action": "issue", "title": "Denied"}, 403)
	do("leader", "POST", base+"/work", map[string]string{"action": "issue", "title": "Review coursework", "member": actors["member"]}, 200)
	do("member", "POST", base+"/work", map[string]any{"action": "comment", "number": 1, "body": "Synthetic review comment"}, 200)
	do("leader", "GET", base+"/work?kind=issues&number=1", nil, 200)
	git("switch", "-c", "coursework")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Reviewed coursework"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-m", "Reviewed coursework")
	head := git("rev-parse", "HEAD")
	git("push", remote, "coursework")
	pull := do("leader", "POST", base+"/work", map[string]string{"action": "pull", "title": "Coursework review", "head": "coursework", "member": actors["member"]}, 200)
	number, ok := pull["number"].(float64)
	if !ok || number != 2 {
		t.Fatalf("expected pull request #2: %v", pull)
	}
	do("leader", "POST", base+"/work", map[string]any{"action": "merge", "number": number, "sha": head}, 409)
	do("member", "POST", base+"/work", map[string]any{"action": "review", "number": number, "sha": head, "decision": "APPROVED"}, 200)
	do("leader", "POST", base+"/work", map[string]any{"action": "merge", "number": number, "sha": head}, 200)
	do("coordinator", "GET", base+"/work?kind=pulls&number=2", nil, 200)
	download := httptest.NewRequest("GET", svr.URL+"/api/v1"+base+"/checkpoints/1/download", nil)
	download.AddCookie(&http.Cookie{Name: "peergit_session", Value: "coordinator"})
	archive := httptest.NewRecorder()
	router.ServeHTTP(archive, download)
	archiveDigest := sha256.Sum256(archive.Body.Bytes())
	wantDigest := id(`SELECT s.archive_sha256 FROM pbl_checkpoints c JOIN repository_snapshots s ON s.id=c.snapshot_id
		WHERE c.team_id=$1 AND c.stage=1 AND s.commit_sha=$2`, osTeam, sha)
	if archive.Code != 200 || hex.EncodeToString(archiveDigest[:]) != wantDigest {
		t.Fatal("the published merge changed the checkpoint's original archive")
	}
	do("leader", "DELETE", base+"/git-credential", nil, 204)
	req, _ := http.NewRequest("GET", remote+"/info/refs?service=git-upload-pack", nil)
	req.SetBasicAuth("peergit", credential)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("revoked credential returned %d", res.StatusCode)
	}
	repoID := id(`SELECT b.external_repository_id::text FROM repository_bindings b JOIN repositories p ON p.id=b.repository_id
		WHERE p.project_id=$1 AND b.provider='forgejo'`, first["project_id"])
	evt := []byte(`{"repository":{"id":` + repoID + `}}`)
	mac := hmac.New(sha256.New, []byte(forge.WebhookSecret))
	_, _ = mac.Write(evt)
	for range 2 {
		hook := httptest.NewRequest("POST", "/api/v1/pbl/forgejo/webhooks", bytes.NewReader(evt))
		hook.Header.Set("X-Forgejo-Delivery", "00000000-0000-7000-8000-000000000055")
		hook.Header.Set("X-Forgejo-Signature", hex.EncodeToString(mac.Sum(nil)))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, hook)
		if response.Code != 202 {
			t.Fatalf("signed replay returned %d: %s", response.Code, response.Body.String())
		}
	}
	if seen := id(`SELECT count(*)::text FROM pbl_forge_deliveries`); seen != "1" {
		t.Fatalf("signed delivery replay recorded %s deliveries", seen)
	}

	execSQL(`INSERT INTO user_identities(user_id,provider,subject) VALUES($1,'github','49101')`, actors["leader"])
	installation := id(`INSERT INTO github_installations(college_id,external_installation_id,account_external_id,account_login,target_type,repository_selection,permissions,added_by)
		VALUES($1,49102,49101,'fixture','User','selected','{}',$2) RETURNING id::text`, tenant, actors["leader"])
	execSQL(`INSERT INTO github_installation_user_grants(college_id,user_id,installation_id,external_user_id)
		VALUES($1,$2,$3,49101)`, tenant, actors["leader"], installation)
	execSQL(`INSERT INTO github_repository_candidates(college_id,user_id,installation_id,external_repository_id,owner_login,repository_name,visibility,expires_at)
		VALUES($1,$2,$3,49103,'fixture','private-pbl','private',now()+interval '10 minutes')`, tenant, actors["leader"], installation)
	logical := id(`INSERT INTO repositories(college_id,project_id,created_by) VALUES($1,$2,$3) RETURNING id::text`, tenant, ordinary, actors["leader"])
	execSQL(`INSERT INTO repository_bindings(college_id,repository_id,installation_id,external_repository_id,owner_login,repository_name,linked_by)
		VALUES($1,$2,$3,49103,'fixture','private-pbl',$4)`, tenant, logical, installation, actors["leader"])
	request := map[string]any{"installation_id": installation, "repository_id": 49103}
	copyTeam := root + "/teams/" + dbmsTeam + "/repository"
	copyOrder := do("leader", "POST", copyTeam, request, 200)
	authorizeURL, err := url.Parse(copyOrder["authorization_url"].(string))
	if err != nil || authorizeURL.Query().Get("code_challenge_method") != "S256" {
		t.Fatal("PBL copy did not start a PKCE repository authorization")
	}
	do("leader", "GET", "/github/authorization/callback?code=fixture&state="+url.QueryEscape(authorizeURL.Query().Get("state")), nil, 303)
	if state := do("leader", "GET", copyTeam, nil, 200)["state"]; state != "queued" {
		t.Fatalf("authorized copy was not queued: %v", state)
	}
	execSQL(`UPDATE github_installation_user_grants SET revoked_at=now() WHERE installation_id=$1 AND user_id=$2`, installation, actors["leader"])
	claim, err := queue.ClaimTypes(ctx, "pbl-test", time.Minute, []string{"pbl_provision"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = handlers["pbl_provision"](ctx, claim)
	var revoked *jobs.PermanentError
	if !errors.As(err, &revoked) || revoked.Code != "pbl_source_revoked" {
		t.Fatalf("worker did not reject an authorized copy after source revocation: %v", err)
	}
	if got := id(`SELECT count(*)::text FROM repository_bindings WHERE provider='github' AND repository_id=$1 AND is_current`, logical); got != "1" {
		t.Fatal("PBL source authorization altered the ordinary GitHub binding")
	}
}

func testBasicCredential(user, password string) string {
	req := new(http.Request)
	req.Header = make(http.Header)
	req.SetBasicAuth(user, password)
	return strings.TrimPrefix(req.Header.Get("Authorization"), "Basic ")
}
