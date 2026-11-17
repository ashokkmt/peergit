package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"peergit/internal/identity"
	"peergit/internal/platform/errormanager"
	"peergit/migrations"
)

func TestGitHubOAuthUsesPKCEStableIDAndVerifiedPrivatePrimaryEmail(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run GitHub OAuth integration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	var challenge string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/authorize":
			challenge = r.URL.Query().Get("code_challenge")
			w.WriteHeader(http.StatusOK)
		case "/access_token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "form", http.StatusBadRequest)
				return
			}
			if r.Form.Get("code") == "token-failure" {
				http.Error(w, "temporary provider failure", http.StatusBadGateway)
				return
			}
			hash := sha256String(r.Form.Get("code_verifier"))
			clientID := r.Form.Get("client_id")
			if clientID == "" {
				clientID, _, _ = r.BasicAuth()
			}
			if hash != challenge || clientID != "test-client" {
				http.Error(w, "pkce", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": r.Form.Get("code"), "token_type": "bearer", "expires_in": 3600})
		case "/api/user":
			if r.Header.Get("Authorization") == "Bearer profile-failure" {
				http.Error(w, "temporary provider failure", http.StatusBadGateway)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if r.Header.Get("Authorization") == "Bearer email-conflict" {
				_, _ = io.WriteString(w, `{"id":981235,"login":"conflict-handle","name":"Conflicting Email"}`)
			} else {
				_, _ = io.WriteString(w, `{"id":981234,"login":"stable-handle","name":"Test Person"}`)
			}
		case "/api/user/emails":
			w.Header().Set("Content-Type", "application/json")
			if r.Header.Get("Authorization") == "Bearer email-failure" {
				http.Error(w, "temporary provider failure", http.StatusBadGateway)
				return
			}
			if r.Header.Get("Authorization") != "Bearer verified" && r.Header.Get("Authorization") != "Bearer unverified" && r.Header.Get("Authorization") != "Bearer email-conflict" {
				http.Error(w, "auth", http.StatusUnauthorized)
				return
			}
			verified := r.Header.Get("Authorization") != "Bearer unverified"
			_ = json.NewEncoder(w).Encode([]map[string]any{{"email": "private@contact.test", "primary": true, "verified": verified, "visibility": nil}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := errormanager.NewManager(logger)
	auth := identity.NewHandler(pool, identity.Config{GitHubClientID: "test-client", GitHubClientSecret: "test-secret", GitHubRedirectURL: "http://peergit.test/api/v1/auth/github/callback", GitHubAuthorizeURL: provider.URL + "/authorize", GitHubTokenURL: provider.URL + "/access_token", GitHubAPIURL: provider.URL + "/api", AppOrigin: "http://peergit.test", SessionHashKey: "github integration session key with entropy", CookieSecure: false}, logger, manager)
	router := chi.NewRouter()
	auth.Register(router)
	start := func() (string, *http.Cookie) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/auth/github", nil)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusFound {
			t.Fatalf("start=%d %s", res.Code, res.Body.String())
		}
		loc, err := url.Parse(res.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		if loc.Query().Get("code_challenge_method") != "S256" || loc.Query().Get("state") == "" || loc.Query().Get("code_challenge") == "" {
			t.Fatalf("missing OAuth state/PKCE: %s", loc)
		}
		challenge = loc.Query().Get("code_challenge")
		for _, cookie := range res.Result().Cookies() {
			if cookie.Name == "peergit_github_state" {
				return loc.Query().Get("state"), cookie
			}
		}
		t.Fatal("state cookie missing")
		return "", nil
	}
	callback := func(state, code string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?state="+url.QueryEscape(state)+"&code="+url.QueryEscape(code), nil)
		req.AddCookie(cookie)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	state, cookie := start()
	res := callback(state, "verified", &http.Cookie{Name: cookie.Name, Value: "wrong"})
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("mismatched state accepted: %d", res.Code)
	}
	res = callback(state, "verified", cookie)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("callback=%d %s", res.Code, res.Body.String())
	}
	var session *http.Cookie
	for _, c := range res.Result().Cookies() {
		if c.Name == "peergit_session" {
			session = c
		}
	}
	if session == nil {
		t.Fatal("session cookie missing")
	}
	var userID, email, providerID, accountType string
	if err := pool.QueryRow(ctx, `SELECT u.id::text,u.email_normalized,i.subject,u.account_type FROM user_identities i JOIN users u ON u.id=i.user_id WHERE i.provider='github' AND i.subject='981234'`).Scan(&userID, &email, &providerID, &accountType); err != nil {
		t.Fatal(err)
	}
	if email != "private@contact.test" || providerID != "981234" || accountType != "unverified" {
		t.Fatalf("identity email=%s subject=%s type=%s", email, providerID, accountType)
	}
	if replay := callback(state, "verified", cookie); replay.Code != http.StatusUnauthorized {
		t.Fatalf("replayed state accepted: %d", replay.Code)
	}
	var campusID string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES('github-campus-test','GitHub Campus Test') RETURNING id::text`).Scan(&campusID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET college_id=$2,account_type='campus' WHERE id=$1`, userID, campusID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO campus_verifications(college_id,user_id,source) VALUES($1,$2,'administrator_review')`, campusID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO campus_roles(college_id,user_id,role) VALUES($1,$2,'student')`, campusID, userID); err != nil {
		t.Fatal(err)
	}
	sessionRequest := httptest.NewRequest(http.MethodGet, "/session", nil)
	sessionRequest.AddCookie(session)
	sessionResponse := httptest.NewRecorder()
	router.ServeHTTP(sessionResponse, sessionRequest)
	var sessionEnvelope struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	if sessionResponse.Code != http.StatusOK || json.Unmarshal(sessionResponse.Body.Bytes(), &sessionEnvelope) != nil || sessionEnvelope.Data.CSRF == "" {
		t.Fatalf("campus session before logout status=%d body=%s", sessionResponse.Code, sessionResponse.Body.String())
	}
	logout := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	logout.Header.Set("Origin", "http://peergit.test")
	logout.Header.Set("X-CSRF-Token", sessionEnvelope.Data.CSRF)
	logout.AddCookie(session)
	logoutResponse := httptest.NewRecorder()
	router.ServeHTTP(logoutResponse, logout)
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", logoutResponse.Code, logoutResponse.Body.String())
	}
	state, cookie = start()
	relogin := callback(state, "verified", cookie)
	if relogin.Code != http.StatusSeeOther {
		t.Fatalf("campus relogin status=%d body=%s", relogin.Code, relogin.Body.String())
	}
	var reloginSession *http.Cookie
	for _, c := range relogin.Result().Cookies() {
		if c.Name == "peergit_session" {
			reloginSession = c
		}
	}
	if reloginSession == nil {
		t.Fatal("campus relogin did not issue a session")
	}
	currentSession := httptest.NewRequest(http.MethodGet, "/session", nil)
	currentSession.AddCookie(reloginSession)
	currentSessionResponse := httptest.NewRecorder()
	router.ServeHTTP(currentSessionResponse, currentSession)
	var currentSessionEnvelope struct {
		Data struct {
			User struct {
				CollegeID    string `json:"college_id"`
				CampusStatus string `json:"campus_status"`
			} `json:"user"`
		} `json:"data"`
	}
	if currentSessionResponse.Code != http.StatusOK || json.Unmarshal(currentSessionResponse.Body.Bytes(), &currentSessionEnvelope) != nil || currentSessionEnvelope.Data.User.CollegeID != campusID || currentSessionEnvelope.Data.User.CampusStatus != "verified" {
		t.Fatalf("campus affiliation did not survive sign-out/sign-in: status=%d body=%s", currentSessionResponse.Code, currentSessionResponse.Body.String())
	}
	for _, code := range []string{"token-failure", "profile-failure", "email-failure", "email-conflict"} {
		state, cookie = start()
		failed := callback(state, code, cookie)
		if failed.Code == http.StatusSeeOther {
			t.Fatalf("provider failure or email conflict %q created a login session", code)
		}
		for _, issued := range failed.Result().Cookies() {
			if issued.Name == "peergit_session" && issued.Value != "" {
				t.Fatalf("provider failure or email conflict %q issued a session", code)
			}
		}
	}
	var conflictingUsers, identities int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE email_normalized='private@contact.test'`).Scan(&conflictingUsers); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_identities WHERE provider='github' AND subject IN ('981234','981235')`).Scan(&identities); err != nil {
		t.Fatal(err)
	}
	if conflictingUsers != 1 || identities != 1 {
		t.Fatalf("failed provider/conflicting email created extra identity state: users=%d identities=%d", conflictingUsers, identities)
	}
	state, cookie = start()
	if rejected := callback(state, "unverified", cookie); rejected.Code != http.StatusForbidden {
		t.Fatalf("unverified primary email status=%d body=%s", rejected.Code, rejected.Body.String())
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_identities WHERE provider='github' AND subject='981234'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("immutable provider identity count=%d err=%v", count, err)
	}
}

func sha256String(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
