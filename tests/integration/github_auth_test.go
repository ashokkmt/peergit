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
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":981234,"login":"stable-handle","name":"Test Person"}`)
		case "/api/user/emails":
			w.Header().Set("Content-Type", "application/json")
			if r.Header.Get("Authorization") != "Bearer verified" && r.Header.Get("Authorization") != "Bearer unverified" {
				http.Error(w, "auth", http.StatusUnauthorized)
				return
			}
			verified := r.Header.Get("Authorization") == "Bearer verified"
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
