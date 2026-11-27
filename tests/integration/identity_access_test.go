package integration_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	"peergit/migrations"
)

func TestPhase2SessionConsentAdminAndTenantIsolation(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run identity/campus integration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	key := []byte("phase2 integration signing key with enough entropy")
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	var collegeID, adminID, targetID string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES('phase2-test','Phase 2 Test') RETURNING id::text`).Scan(&collegeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO college_domains(college_id,domain,verified_at) VALUES($1,'phase2.test',now())`, collegeID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at)
		VALUES($1,'admin@phase2.test','admin@phase2.test','Campus Admin','phase2_admin','campus',now()) RETURNING id::text`, collegeID).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at)
		VALUES($1,'student@phase2.test','student@phase2.test','Test Student','phase2_student','campus',now()) RETURNING id::text`, collegeID).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{adminID, targetID} {
		if _, err := pool.Exec(ctx, `INSERT INTO campus_verifications(college_id,user_id,source) VALUES($1,$2,'administrator_review')`, collegeID, userID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO campus_roles(college_id,user_id,role) VALUES($1,$2,'campus_admin')`, collegeID, adminID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, targetID, collegeID, sessionHash(key, "target-session"), sessionCSRFHash(key, "target-session")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE sessions SET college_id=NULL WHERE user_id=$1`, targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO mfa_credentials(user_id,encrypted_secret,enabled_at) VALUES($1,decode('0001','hex'),now())`, adminID); err != nil {
		t.Fatal(err)
	}
	token := "opaque-session-token-for-phase2"
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,mfa_verified_at,expires_at) VALUES($1,$2,$3,$4,now(),now()+interval '1 hour')`, adminID, collegeID, sessionHash(key, token), sessionCSRFHash(key, token)); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	errors := errormanager.NewManager(logger)
	auth := identity.NewHandler(pool, identity.Config{AppOrigin: "http://peergit.test", SessionHashKey: string(key), MFAEncryptionKey: "phase2 mfa encryption secret long enough", CookieSecure: false}, logger, errors)
	campusHandler := campus.NewHandler(pool, auth, "http://peergit.test", errors, string(key), "phase2 delivery encryption secret")
	mediaHandler := media.NewHandler(pool, storage.New("http://127.0.0.1:1", "test", "us-east-1", "key", "secret"), auth, errors)
	router := httpserver.NewRouter(health.NewHandler(errors, pool), logger, errors, auth.Register, campusHandler.Register, mediaHandler.Register)
	if _, err := pool.Exec(ctx, `UPDATE sessions SET college_id=NULL WHERE user_id=$1`, targetID); err != nil {
		t.Fatal(err)
	}
	restored := httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/session", nil)
	restored.AddCookie(&http.Cookie{Name: "peergit_session", Value: "target-session"})
	restoredResponse := httptest.NewRecorder()
	router.ServeHTTP(restoredResponse, restored)
	var restoredEnvelope struct {
		Data struct {
			User struct {
				CollegeID    string `json:"college_id"`
				CampusStatus string `json:"campus_status"`
			} `json:"user"`
		} `json:"data"`
	}
	if restoredResponse.Code != http.StatusOK || json.Unmarshal(restoredResponse.Body.Bytes(), &restoredEnvelope) != nil || restoredEnvelope.Data.User.CollegeID != collegeID || restoredEnvelope.Data.User.CampusStatus != "verified" {
		t.Fatalf("durable campus verification was not restored after a null-campus session: status=%d body=%s", restoredResponse.Code, restoredResponse.Body.String())
	}
	get := httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/session", nil)
	get.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	getResponse := httptest.NewRecorder()
	var originalSeen, updatedSeen time.Time
	if err := pool.QueryRow(ctx, `UPDATE sessions SET last_seen_at=now()-interval '6 minutes' WHERE user_id=$1 RETURNING last_seen_at`, adminID).Scan(&originalSeen); err != nil {
		t.Fatal(err)
	}
	router.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("session status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}
	if err := pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE user_id=$1`, adminID).Scan(&updatedSeen); err != nil || !updatedSeen.After(originalSeen) {
		t.Fatalf("session activity was not refreshed: previous=%v current=%v err=%v", originalSeen, updatedSeen, err)
	}
	router.ServeHTTP(httptest.NewRecorder(), get.Clone(ctx))
	var repeatedSeen time.Time
	if err := pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE user_id=$1`, adminID).Scan(&repeatedSeen); err != nil || !repeatedSeen.Equal(updatedSeen) {
		t.Fatalf("recent session activity should not be rewritten: previous=%v current=%v err=%v", updatedSeen, repeatedSeen, err)
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(getResponse.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	csrfValue, _ := envelope.Data["csrf_token"].(string)
	if csrfValue == "" {
		t.Fatal("authenticated session did not return its CSRF token")
	}
	mutate := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://peergit.test"+path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://peergit.test")
		req.Header.Set("X-CSRF-Token", csrfValue)
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("POST %s status=%d want=%d body=%s", path, rec.Code, want, rec.Body.String())
		}
		return rec
	}
	for _, purpose := range []string{"terms", "privacy"} {
		mutate(http.MethodPut, "/api/v1/me/consent", `{"purpose":"`+purpose+`","policy_version":"draft-1","granted":true}`, http.StatusOK)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'terms','draft-1'),($1,'privacy','draft-1')`, targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO campus_roles(college_id,user_id,role) VALUES($1,$2,'campus_admin')`, collegeID, targetID); err != nil {
		t.Fatal(err)
	}
	noMFA := httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/admin/users", nil)
	noMFA.AddCookie(&http.Cookie{Name: "peergit_session", Value: "target-session"})
	noMFAResponse := httptest.NewRecorder()
	router.ServeHTTP(noMFAResponse, noMFA)
	if noMFAResponse.Code != http.StatusForbidden || !strings.Contains(noMFAResponse.Body.String(), "mfa_required") {
		t.Fatalf("unverified administrator status=%d body=%s, want MFA denial", noMFAResponse.Code, noMFAResponse.Body.String())
	}
	if _, err := pool.Exec(ctx, `DELETE FROM campus_roles WHERE college_id=$1 AND user_id=$2 AND role='campus_admin'`, collegeID, targetID); err != nil {
		t.Fatal(err)
	}
	mutate(http.MethodPost, "/api/v1/admin/roles", `{"user_id":"`+targetID+`","role":"faculty"}`, http.StatusOK)
	var facultyRole bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM campus_roles WHERE college_id=$1 AND user_id=$2 AND role='faculty')`, collegeID, targetID).Scan(&facultyRole); err != nil || !facultyRole {
		t.Fatalf("audited campus role grant missing: exists=%v err=%v", facultyRole, err)
	}
	var privateMediaID string
	if err := pool.QueryRow(ctx, `INSERT INTO media(college_id,owner_user_id,object_key,mime_type,byte_size,sha256,scan_status,visibility)
		VALUES($1,$2,'private-test','image/png',1,decode(repeat('00',32),'hex'),'clean','private') RETURNING id::text`, collegeID, targetID).Scan(&privateMediaID); err != nil {
		t.Fatal(err)
	}
	privateRead := httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/media/"+privateMediaID, nil)
	privateRead.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	privateResponse := httptest.NewRecorder()
	router.ServeHTTP(privateResponse, privateRead)
	if privateResponse.Code != http.StatusNotFound {
		t.Fatalf("another campus user read private media: status=%d body=%s", privateResponse.Code, privateResponse.Body.String())
	}
	profileRequest := httptest.NewRequest(http.MethodPatch, "http://peergit.test/api/v1/me/profile", bytes.NewBufferString(`{"display_name":"Campus Admin","headline":"Builder","bio":"A short bio","skills":["Go","C++"]}`))
	profileRequest.Header.Set("Content-Type", "application/json")
	profileRequest.Header.Set("Origin", "http://peergit.test")
	profileRequest.Header.Set("X-CSRF-Token", csrfValue)
	profileRequest.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	profileResponse := httptest.NewRecorder()
	router.ServeHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("profile status=%d body=%s", profileResponse.Code, profileResponse.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE campus_verifications SET revoked_at=now() WHERE college_id=$1 AND user_id=$2 AND revoked_at IS NULL`, collegeID, targetID); err != nil {
		t.Fatal(err)
	}
	revokedSession := httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/session", nil)
	revokedSession.AddCookie(&http.Cookie{Name: "peergit_session", Value: "target-session"})
	revokedResponse := httptest.NewRecorder()
	router.ServeHTTP(revokedResponse, revokedSession)
	var revokedEnvelope struct {
		Data struct {
			User struct {
				CollegeID    string `json:"college_id"`
				CampusStatus string `json:"campus_status"`
			} `json:"user"`
		} `json:"data"`
	}
	if revokedResponse.Code != http.StatusOK || json.Unmarshal(revokedResponse.Body.Bytes(), &revokedEnvelope) != nil || revokedEnvelope.Data.User.CollegeID != "" || revokedEnvelope.Data.User.CampusStatus != "inactive" {
		t.Fatalf("revoked campus session retained access: status=%d body=%s", revokedResponse.Code, revokedResponse.Body.String())
	}
	revokedAdminRead := httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/admin/users", nil)
	revokedAdminRead.AddCookie(&http.Cookie{Name: "peergit_session", Value: "target-session"})
	revokedAdminResponse := httptest.NewRecorder()
	router.ServeHTTP(revokedAdminResponse, revokedAdminRead)
	if revokedAdminResponse.Code != http.StatusForbidden {
		t.Fatalf("revoked user accessed campus administration: status=%d body=%s", revokedAdminResponse.Code, revokedAdminResponse.Body.String())
	}
	mutate(http.MethodPost, "/api/v1/admin/organizations", `{"kind":"club","slug":"robotics-club","name":"Robotics Club"}`, http.StatusCreated)
	mutate(http.MethodPost, "/api/v1/admin/invitations", `{"email":"guest@outside.test","account_type":"external","external_access_kind":"mentor","expires_hours":24}`, http.StatusCreated)
	mutate(http.MethodPost, "/api/v1/admin/users/"+targetID+"/status", `{"suspended":true}`, http.StatusOK)
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM users WHERE id=$1`, targetID).Scan(&status); err != nil || status != "suspended" {
		t.Fatalf("suspended user status=%q err=%v", status, err)
	}
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1`, targetID).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("suspended user retained %d sessions err=%v", sessions, err)
	}
	var deletedID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,email_normalized,display_name,handle,account_type) VALUES('deleted@contact.test','deleted@contact.test','Deleted User','phase2_deleted','unverified') RETURNING id::text`).Scan(&deletedID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,now()+interval '1 hour')`, deletedID, sessionHash(key, "deleted-user-session"), sessionCSRFHash(key, "deleted-user-session")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET status='deleted' WHERE id=$1`, deletedID); err != nil {
		t.Fatal(err)
	}
	deletedSession := httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/session", nil)
	deletedSession.AddCookie(&http.Cookie{Name: "peergit_session", Value: "deleted-user-session"})
	deletedResponse := httptest.NewRecorder()
	router.ServeHTTP(deletedResponse, deletedSession)
	var deletedEnvelope struct {
		Data struct {
			Authenticated bool `json:"authenticated"`
		} `json:"data"`
	}
	if deletedResponse.Code != http.StatusOK || json.Unmarshal(deletedResponse.Body.Bytes(), &deletedEnvelope) != nil || deletedEnvelope.Data.Authenticated {
		t.Fatalf("deleted user retained session access: status=%d body=%s", deletedResponse.Code, deletedResponse.Body.String())
	}
	wrongOrigin := httptest.NewRequest(http.MethodPut, "http://peergit.test/api/v1/me/consent", bytes.NewBufferString(`{"purpose":"portfolio","policy_version":"v1","granted":true}`))
	wrongOrigin.Header.Set("Content-Type", "application/json")
	wrongOrigin.Header.Set("Origin", "https://attacker.test")
	wrongOrigin.Header.Set("X-CSRF-Token", csrfValue)
	wrongOrigin.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	wrongOriginResponse := httptest.NewRecorder()
	router.ServeHTTP(wrongOriginResponse, wrongOrigin)
	if wrongOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-origin mutation status=%d, want 403", wrongOriginResponse.Code)
	}
	var otherCollege, otherUser, otherOrg string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES('phase2-other','Other Test') RETURNING id::text`).Scan(&otherCollege); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type) VALUES($1,'person@other.test','person@other.test','Other','other_user','campus') RETURNING id::text`, otherCollege).Scan(&otherUser); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(college_id,kind,slug,name) VALUES($1,'club','other-club','Other Club') RETURNING id::text`, otherCollege).Scan(&otherOrg); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_members(college_id,organization_id,user_id,role) VALUES($1,$2,$3,'member')`, collegeID, otherOrg, adminID); err == nil {
		t.Fatal("cross-campus organization membership passed composite constraints")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO media(college_id,owner_user_id,object_key,mime_type,byte_size,sha256,scan_status) VALUES($1,$2,'tenant-test','image/png',1,decode(repeat('00',32),'hex'),'clean')`, otherCollege, adminID); err == nil {
		t.Fatal("cross-campus media owner passed composite constraints")
	}
}

func sessionHash(key []byte, token string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(token))
	return h.Sum(nil)
}

func sessionCSRFHash(key []byte, sessionToken string) []byte {
	csrf := base64.RawURLEncoding.EncodeToString(sessionHash(key, "peergit-csrf:"+sessionToken))
	return sessionHash(key, csrf)
}
