package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"peergit/internal/health"
	"peergit/internal/identity"
	"peergit/internal/platform/errormanager"
	httpserver "peergit/internal/platform/http"
	"peergit/internal/portfolio"
	"peergit/migrations"
)

func TestPhase5DPortfolioConsentAndSuccession(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run portfolio and succession integration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	stamp := fmt.Sprintf("%d", time.Now().UnixNano())
	var college string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES($1,$2) RETURNING id::text`, "phase5d-"+stamp, "Phase 5D "+stamp).Scan(&college); err != nil {
		t.Fatal(err)
	}
	key := []byte("phase5d integration session signing key")
	users := map[string]string{}
	tokens := map[string]string{}
	csrf := map[string]string{}
	handleSuffix := stamp[len(stamp)-8:]
	for _, name := range []string{"owner", "successor", "visitor", "stale"} {
		email := name + "+" + stamp + "@phase5d.test"
		var userID string
		if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at,profile)
			VALUES($1,$2,$2,$3,$4,'campus',now(),' {"headline":"Student builder","bio":"Projects and campus work"}') RETURNING id::text`, college, email, name, "p5d_"+name+"_"+handleSuffix).Scan(&userID); err != nil {
			t.Fatal(err)
		}
		users[name] = userID
		if _, err := pool.Exec(ctx, `INSERT INTO campus_verifications(college_id,user_id,source) VALUES($1,$2,'administrator_review')`, college, users[name]); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'terms','draft-1'),($1,'privacy','draft-1')`, users[name]); err != nil {
			t.Fatal(err)
		}
		token := "phase5d-session-" + name + "-" + stamp
		tokens[name] = token
		if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,expires_at,created_at)
			VALUES($1,$2,$3,$4,now()+interval '1 hour',CASE WHEN $5='stale' THEN now()-interval '1 hour' ELSE now() END)`, users[name], college, sessionHash(key, token), sessionCSRFHash(key, token), name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'portfolio','draft-1')`, users["owner"]); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	errs := errormanager.NewManager(logger)
	auth := identity.NewHandler(pool, identity.Config{AppOrigin: "http://phase5d.test", SessionHashKey: string(key), MFAEncryptionKey: "phase5d integration mfa encryption key"}, logger, errs)
	portfolioHandler := portfolio.NewHandler(pool, auth, errs)
	router := httpserver.NewRouter(health.NewHandler(errs, pool), logger, errs, auth.Register, portfolioHandler.Register)
	for actor, token := range tokens {
		req := httptest.NewRequest(http.MethodGet, "http://phase5d.test/api/v1/session", nil)
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
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
		if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		csrf[actor] = envelope.Data.CSRFToken
	}
	var otherCollege string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES($1,$2) RETURNING id::text`, "phase5d-other-"+stamp, "Other Phase 5D campus").Scan(&otherCollege); err != nil {
		t.Fatal(err)
	}
	var otherUser string
	otherEmail := "other+" + stamp + "@phase5d.test"
	if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at,profile)
		VALUES($1,$2,$2,'Other visitor',$3,'campus',now(),'{}') RETURNING id::text`, otherCollege, otherEmail, "p5d_other_"+handleSuffix).Scan(&otherUser); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO campus_verifications(college_id,user_id,source) VALUES($1,$2,'administrator_review')`, otherCollege, otherUser); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'terms','draft-1'),($1,'privacy','draft-1')`, otherUser); err != nil {
		t.Fatal(err)
	}
	otherToken := "phase5d-session-other-" + stamp
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, otherUser, otherCollege, sessionHash(key, otherToken), sessionCSRFHash(key, otherToken)); err != nil {
		t.Fatal(err)
	}
	tokens["other"] = otherToken
	otherSession := httptest.NewRequest(http.MethodGet, "http://phase5d.test/api/v1/session", nil)
	otherSession.AddCookie(&http.Cookie{Name: "peergit_session", Value: otherToken})
	otherResponse := httptest.NewRecorder()
	router.ServeHTTP(otherResponse, otherSession)
	var otherEnvelope struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if otherResponse.Code != http.StatusOK || json.Unmarshal(otherResponse.Body.Bytes(), &otherEnvelope) != nil {
		t.Fatalf("other-campus session status=%d body=%s", otherResponse.Code, otherResponse.Body.String())
	}
	csrf["other"] = otherEnvelope.Data.CSRFToken
	call := func(actor, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://phase5d.test"+path, bytes.NewBufferString(body))
		if actor != "" {
			req.AddCookie(&http.Cookie{Name: "peergit_session", Value: tokens[actor]})
		}
		if method != http.MethodGet {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://phase5d.test")
			req.Header.Set("X-CSRF-Token", csrf[actor])
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	createProject := func(slug, title string, visibility string, repositoryState string) string {
		t.Helper()
		var projectID string
		if err := pool.QueryRow(ctx, `INSERT INTO projects(college_id,slug,title,summary,project_type,visibility,lifecycle,created_by)
			VALUES($1,$2,$3,'Selected project evidence','side_project',$4,'active',$5) RETURNING id::text`, college, slug, title, visibility, users["owner"]).Scan(&projectID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO project_members(college_id,project_id,user_id,role) VALUES($1,$2,$3,'owner'),($1,$2,$4,'member')`, college, projectID, users["owner"], users["successor"]); err != nil {
			t.Fatal(err)
		}
		if repositoryState != "" {
			var installationID, repositoryID string
			externalInstallation := time.Now().UnixNano() % 1000000000000
			externalRepository := externalInstallation + 1000
			if err := pool.QueryRow(ctx, `INSERT INTO github_installations(college_id,external_installation_id,account_external_id,account_login,target_type,repository_selection,permissions,status,added_by)
				VALUES($1,$2,$3,'phase5d-fixture','User','selected','{"contents":"read"}','active',$4) RETURNING id::text`, college, externalInstallation, externalInstallation+2000, users["owner"]).Scan(&installationID); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `INSERT INTO repositories(college_id,project_id,created_by) VALUES($1,$2,$3) RETURNING id::text`, college, projectID, users["owner"]).Scan(&repositoryID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO repository_bindings(college_id,repository_id,installation_id,external_repository_id,owner_login,repository_name,default_branch,visibility,access_state,linked_by)
				VALUES($1,$2,$3,$4,'phase5d-fixture',$5,'main','public',$6,$7)`, college, repositoryID, installationID, externalRepository, slug, repositoryState, users["owner"]); err != nil {
				t.Fatal(err)
			}
		}
		return projectID
	}
	publicProject := createProject("public-work", "Public work", "public", "active")
	campusProject := createProject("campus-work", "Campus work", "campus", "")
	privateProject := createProject("private-work", "Private work", "private", "")
	for index, projectID := range []string{publicProject, campusProject, privateProject} {
		if _, err := pool.Exec(ctx, `INSERT INTO portfolio_items(college_id,user_id,project_id,caption,sort_order) VALUES($1,$2,$3,$4,$5)`, college, users["owner"], projectID, "My selected contribution", index); err != nil {
			t.Fatal(err)
		}
	}
	publicProfile := call("", http.MethodGet, "/api/v1/portfolio/"+"p5d_owner_"+handleSuffix, "")
	if publicProfile.Code != http.StatusOK || publicProfile.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("public portfolio status=%d cache=%q body=%s", publicProfile.Code, publicProfile.Header().Get("Cache-Control"), publicProfile.Body.String())
	}
	var publicData struct {
		Data struct {
			Items []struct {
				ProjectID string `json:"project_id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(publicProfile.Body.Bytes(), &publicData); err != nil || len(publicData.Data.Items) != 1 || publicData.Data.Items[0].ProjectID != publicProject {
		t.Fatalf("public portfolio exposed unauthorized evidence: %s err=%v", publicProfile.Body.String(), err)
	}
	campusGrant := call("owner", http.MethodPost, "/api/v1/me/portfolio/shares", `{"audience":"campus","days":7}`)
	if campusGrant.Code != http.StatusCreated {
		t.Fatalf("campus share status=%d body=%s", campusGrant.Code, campusGrant.Body.String())
	}
	var grantData struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(campusGrant.Body.Bytes(), &grantData); err != nil {
		t.Fatal(err)
	}
	token := grantData.Data.URL[len("/portfolio/share/"):]
	campusView := call("visitor", http.MethodGet, "/api/v1/portfolio/share/"+token, "")
	if campusView.Code != http.StatusOK || campusView.Header().Get("Cache-Control") != "private, no-store" || bytes.Contains(campusView.Body.Bytes(), []byte("private-work")) {
		t.Fatalf("campus link exposed private evidence: status=%d body=%s", campusView.Code, campusView.Body.String())
	}
	crossCampusView := call("other", http.MethodGet, "/api/v1/portfolio/share/"+token, "")
	if crossCampusView.Code == http.StatusOK {
		t.Fatalf("campus share was visible cross-campus: %s", crossCampusView.Body.String())
	}
	expiringShare := call("owner", http.MethodPost, "/api/v1/me/portfolio/shares", `{"audience":"public","days":7}`)
	if expiringShare.Code != http.StatusCreated {
		t.Fatalf("expiring public share status=%d body=%s", expiringShare.Code, expiringShare.Body.String())
	}
	var expiringGrant struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(expiringShare.Body.Bytes(), &expiringGrant); err != nil {
		t.Fatal(err)
	}
	expiringToken := expiringGrant.Data.URL[len("/portfolio/share/"):]
	expiringHash := sha256.Sum256([]byte(expiringToken))
	if _, err := pool.Exec(ctx, `UPDATE portfolio_share_grants SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, expiringHash[:]); err != nil {
		t.Fatal(err)
	}
	if expiredView := call("", http.MethodGet, "/api/v1/portfolio/share/"+expiringToken, ""); expiredView.Code != http.StatusNotFound {
		t.Fatalf("expired share status=%d body=%s", expiredView.Code, expiredView.Body.String())
	}
	if _, err := pool.Exec(ctx, `INSERT INTO projects(college_id,slug,title,summary,project_type,visibility,lifecycle,created_by)
		VALUES($1,'organization-fixture','Organization fixture','Organization transfer fixture','side_project','private','draft',$2)`, college, users["owner"]); err != nil {
		t.Fatal(err)
	}
	var organizationID string
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(college_id,kind,slug,name,owner_user_id) VALUES($1,'club','club-fixture','Club fixture',$2) RETURNING id::text`, college, users["owner"]).Scan(&organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_members(college_id,organization_id,user_id,role) VALUES($1,$2,$3,'owner'),($1,$2,$4,'member')`, college, organizationID, users["owner"], users["successor"]); err != nil {
		t.Fatal(err)
	}
	orgProposal := call("owner", http.MethodPost, "/api/v1/organizations/"+organizationID+"/handover", fmt.Sprintf(`{"to_user_id":%q,"message":"Continue club projects"}`, users["successor"]))
	if orgProposal.Code != http.StatusCreated {
		t.Fatalf("organization proposal status=%d body=%s", orgProposal.Code, orgProposal.Body.String())
	}
	var orgRequest struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(orgProposal.Body.Bytes(), &orgRequest); err != nil {
		t.Fatal(err)
	}
	orgAccept := call("successor", http.MethodPost, "/api/v1/handovers/"+orgRequest.Data.ID+"/accept", `{}`)
	if orgAccept.Code != http.StatusOK {
		t.Fatalf("organization accept status=%d body=%s", orgAccept.Code, orgAccept.Body.String())
	}
	var organizationOwner string
	if err := pool.QueryRow(ctx, `SELECT owner_user_id::text FROM organizations WHERE id=$1`, organizationID).Scan(&organizationOwner); err != nil || organizationOwner != users["successor"] {
		t.Fatalf("organization ownership did not transfer: owner=%s err=%v", organizationOwner, err)
	}
	var acceptedAuditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND actor_id=$2 AND action IN ('organization.handover.accepted','project.handover.accepted')`, college, users["successor"]).Scan(&acceptedAuditCount); err != nil || acceptedAuditCount != 1 {
		t.Fatalf("ownership transfer audit count=%d err=%v", acceptedAuditCount, err)
	}

	projectID := createProject("handover-work", "Handover work", "campus", "active")
	staleProposal := call("stale", http.MethodPost, "/api/v1/projects/"+projectID+"/handover", fmt.Sprintf(`{"to_user_id":%q}`, users["successor"]))
	if staleProposal.Code != http.StatusForbidden {
		t.Fatalf("stale auth proposal status=%d body=%s", staleProposal.Code, staleProposal.Body.String())
	}
	proposal := call("owner", http.MethodPost, "/api/v1/projects/"+projectID+"/handover", fmt.Sprintf(`{"to_user_id":%q,"message":"Keep the project moving"}`, users["successor"]))
	if proposal.Code != http.StatusCreated {
		t.Fatalf("handover proposal status=%d body=%s", proposal.Code, proposal.Body.String())
	}
	var handover struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(proposal.Body.Bytes(), &handover); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE repository_bindings rb SET access_state='permission_lost'
		FROM repositories rp WHERE rp.id=rb.repository_id AND rp.project_id=$1 AND rb.is_current`, projectID); err != nil {
		t.Fatal(err)
	}
	var statuses [2]int
	var wg sync.WaitGroup
	for index := range statuses {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			statuses[index] = call("successor", http.MethodPost, "/api/v1/handovers/"+handover.Data.ID+"/accept", `{}`).Code
		}(index)
	}
	wg.Wait()
	accepted := 0
	for _, status := range statuses {
		if status == http.StatusOK {
			accepted++
		} else if status != http.StatusNotFound {
			t.Fatalf("concurrent accept returned unexpected status %d", status)
		}
	}
	if accepted != 1 {
		t.Fatalf("concurrent accepts transferred %d times, statuses=%v", accepted, statuses)
	}
	var ownerRole, successorRole string
	if err := pool.QueryRow(ctx, `SELECT role FROM project_members WHERE project_id=$1 AND user_id=$2`, projectID, users["owner"]).Scan(&ownerRole); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT role FROM project_members WHERE project_id=$1 AND user_id=$2`, projectID, users["successor"]).Scan(&successorRole); err != nil {
		t.Fatal(err)
	}
	if ownerRole != "maintainer" || successorRole != "owner" {
		t.Fatalf("handover did not preserve the prior owner and assign the successor: old=%s new=%s", ownerRole, successorRole)
	}
	var transferredState, transferredProviderState string
	if err := pool.QueryRow(ctx, `SELECT state,provider_state FROM handover_requests WHERE id=$1`, handover.Data.ID).Scan(&transferredState, &transferredProviderState); err != nil || transferredState != "accepted" || transferredProviderState != "source_unavailable" {
		t.Fatalf("accepted handover did not reconcile current provider access: state=%s provider=%s err=%v", transferredState, transferredProviderState, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND actor_id=$2 AND action='project.handover.accepted'`, college, users["successor"]).Scan(&acceptedAuditCount); err != nil || acceptedAuditCount != 1 {
		t.Fatalf("project ownership transfer audit count=%d err=%v", acceptedAuditCount, err)
	}
	providerUnavailableProject := createProject("provider-unavailable", "Provider unavailable", "campus", "permission_lost")
	providerProposal := call("owner", http.MethodPost, "/api/v1/projects/"+providerUnavailableProject+"/handover", fmt.Sprintf(`{"to_user_id":%q}`, users["successor"]))
	var providerData struct {
		Data struct {
			ProviderState string `json:"provider_state"`
		} `json:"data"`
	}
	if providerProposal.Code != http.StatusCreated || json.Unmarshal(providerProposal.Body.Bytes(), &providerData) != nil || providerData.Data.ProviderState != "source_unavailable" {
		t.Fatalf("provider-unavailable handover status=%d body=%s", providerProposal.Code, providerProposal.Body.String())
	}

	blockedProject := createProject("conflict-work", "Conflict work", "campus", "")
	conflictProposal := call("owner", http.MethodPost, "/api/v1/projects/"+blockedProject+"/handover", fmt.Sprintf(`{"to_user_id":%q}`, users["successor"]))
	if conflictProposal.Code != http.StatusCreated {
		t.Fatalf("conflict proposal status=%d body=%s", conflictProposal.Code, conflictProposal.Body.String())
	}
	var conflictRequest struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(conflictProposal.Body.Bytes(), &conflictRequest); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM project_members WHERE project_id=$1 AND user_id=$2`, blockedProject, users["successor"]); err != nil {
		t.Fatal(err)
	}
	conflictAccept := call("successor", http.MethodPost, "/api/v1/handovers/"+conflictRequest.Data.ID+"/accept", `{}`)
	if conflictAccept.Code != http.StatusConflict {
		t.Fatalf("changed eligibility accept status=%d body=%s", conflictAccept.Code, conflictAccept.Body.String())
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM project_members WHERE project_id=$1 AND role='owner'`, blockedProject).Scan(&accepted); err != nil || accepted != 1 {
		t.Fatalf("conflict lost the last owner: count=%d err=%v", accepted, err)
	}

	if revoked := call("owner", http.MethodPut, "/api/v1/me/consent", `{"purpose":"portfolio","policy_version":"draft-1","granted":false}`); revoked.Code != http.StatusOK {
		t.Fatalf("consent revoke status=%d body=%s", revoked.Code, revoked.Body.String())
	}
	var activeShares int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM portfolio_share_grants WHERE user_id=$1 AND revoked_at IS NULL`, users["owner"]).Scan(&activeShares); err != nil || activeShares != 0 {
		t.Fatalf("portfolio consent revoke left %d active shares, err=%v", activeShares, err)
	}
	if afterRevocation := call("", http.MethodGet, "/api/v1/portfolio/share/"+token, ""); afterRevocation.Code != http.StatusNotFound {
		t.Fatalf("revoked portfolio link status=%d body=%s", afterRevocation.Code, afterRevocation.Body.String())
	}
	if afterRevocation := call("", http.MethodGet, "/api/v1/portfolio/p5d_owner_"+handleSuffix, ""); afterRevocation.Code != http.StatusNotFound {
		t.Fatalf("revoked public portfolio status=%d body=%s", afterRevocation.Code, afterRevocation.Body.String())
	}
}
