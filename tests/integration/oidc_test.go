package integration_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"log/slog"
	"peergit/internal/identity"
	"peergit/internal/platform/errormanager"
	"peergit/migrations"
)

func TestOIDCFlowVerifiesStateNonceEmailAndPKCE(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run Google OIDC integration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	var collegeID string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES('oidc-test','OIDC Test') RETURNING id::text`).Scan(&collegeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO college_domains(college_id,domain,verified_at) VALUES($1,'oidc.test',now())`, collegeID); err != nil {
		t.Fatal(err)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var challenge, nonce string
	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": issuer.URL, "authorization_endpoint": issuer.URL + "/authorize",
				"token_endpoint": issuer.URL + "/token", "jwks_uri": issuer.URL + "/jwks",
				"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
				"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
			})
		case "/jwks":
			pub := key.PublicKey
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kty": "RSA", "kid": "phase2-test", "use": "sig", "alg": "RS256",
				"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			}}})
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			verifierHash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			clientID := r.Form.Get("client_id")
			if clientID == "" {
				clientID, _, _ = r.BasicAuth()
			}
			if base64.RawURLEncoding.EncodeToString(verifierHash[:]) != challenge || clientID != "phase2-client" {
				http.Error(w, "PKCE or client validation failed", http.StatusUnauthorized)
				return
			}
			code := r.Form.Get("code")
			claimNonce := nonce
			if code == "wrong-nonce" {
				claimNonce = "different-nonce"
			}
			email := code + "@oidc.test"
			if code == "guest" {
				email = "guest@outside.test"
			}
			idToken, err := signTestIDToken(key, issuer.URL, code, email, claimNonce, code != "unverified")
			if err != nil {
				http.Error(w, "could not sign token", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "opaque-access-token", "token_type": "Bearer", "expires_in": 3600, "id_token": idToken})
		default:
			http.NotFound(w, r)
		}
	}))
	defer issuer.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := errormanager.NewManager(logger)
	auth := identity.NewHandler(pool, identity.Config{
		Issuer: issuer.URL, ClientID: "phase2-client", ClientSecret: "phase2-secret",
		RedirectURL: "http://peergit.test/api/v1/auth/callback", AppOrigin: "http://peergit.test",
		SessionHashKey: "phase2 OIDC session hashing key with enough entropy", CookieSecure: false,
	}, logger, manager)
	router := chi.NewRouter()
	auth.Register(router)

	start := func(invitation string) (string, *http.Cookie) {
		t.Helper()
		path := "/auth/google"
		if invitation != "" {
			path += "?invite=" + url.QueryEscape(invitation)
		}
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusFound {
			t.Fatalf("OIDC start status=%d body=%s", res.Code, res.Body.String())
		}
		location, err := url.Parse(res.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		challenge, nonce = location.Query().Get("code_challenge"), location.Query().Get("nonce")
		if location.Query().Get("code_challenge_method") != "S256" || location.Query().Get("state") == "" || challenge == "" || nonce == "" {
			t.Fatalf("OIDC authorization URL omitted state, nonce, or S256 PKCE: %s", location)
		}
		for _, cookie := range res.Result().Cookies() {
			if cookie.Name == "peergit_oidc_state" {
				return location.Query().Get("state"), cookie
			}
		}
		t.Fatal("OIDC start did not set its state-binding cookie")
		return "", nil
	}
	callback := func(state, code string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+url.QueryEscape(state)+"&code="+url.QueryEscape(code), nil)
		req.AddCookie(cookie)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}

	state, stateCookie := start("")
	if got := callback(state, "verified", &http.Cookie{Name: stateCookie.Name, Value: "attacker-state"}); got.Code != http.StatusUnauthorized {
		t.Fatalf("callback accepted mismatched browser state: %d", got.Code)
	}
	verified := callback(state, "verified", stateCookie)
	if verified.Code != http.StatusSeeOther {
		t.Fatalf("verified callback status=%d body=%s", verified.Code, verified.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, cookie := range verified.Result().Cookies() {
		if cookie.Name == "peergit_session" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		t.Fatal("successful OIDC callback did not issue a session cookie")
	}
	sessionRequest := httptest.NewRequest(http.MethodGet, "/session", nil)
	sessionRequest.AddCookie(sessionCookie)
	sessionResponse := httptest.NewRecorder()
	router.ServeHTTP(sessionResponse, sessionRequest)
	if sessionResponse.Code != http.StatusOK || !strings.Contains(sessionResponse.Body.String(), `"authenticated":true`) {
		t.Fatalf("new OIDC session status=%d body=%s", sessionResponse.Code, sessionResponse.Body.String())
	}
	if replay := callback(state, "verified", stateCookie); replay.Code != http.StatusUnauthorized {
		t.Fatalf("OIDC state replay status=%d, want unauthorized", replay.Code)
	}
	for _, code := range []string{"unverified", "wrong-nonce"} {
		state, cookie := start("")
		if got := callback(state, code, cookie); got.Code != http.StatusUnauthorized {
			t.Fatalf("callback accepted invalid %s claim: status=%d body=%s", code, got.Code, got.Body.String())
		}
	}
	invitation := "one-time-invitation-token"
	invitationHash := sha256.Sum256([]byte(invitation))
	if _, err := pool.Exec(ctx, `INSERT INTO invitations(college_id,email_normalized,account_type,external_access_kind,token_hash,expires_at)
		VALUES($1,'guest@outside.test','external','mentor',$2,now()+interval '1 hour')`, collegeID, invitationHash[:]); err != nil {
		t.Fatal(err)
	}
	state, cookie := start(invitation)
	accepted := callback(state, "guest", cookie)
	if accepted.Code != http.StatusSeeOther {
		t.Fatalf("verified invited external callback status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	var granted, consumed bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM external_college_access WHERE college_id=$1 AND access_kind='mentor'),
		EXISTS(SELECT 1 FROM invitations WHERE token_hash=$2 AND accepted_at IS NOT NULL)`, collegeID, invitationHash[:]).Scan(&granted, &consumed); err != nil || !granted || !consumed {
		t.Fatalf("external invitation state: access=%v consumed=%v err=%v", granted, consumed, err)
	}
}

func signTestIDToken(key *rsa.PrivateKey, issuer, subject, email, nonce string, emailVerified bool) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "RS256", "kid": "phase2-test", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	claims, err := json.Marshal(map[string]any{
		"iss": issuer, "sub": subject, "aud": "phase2-client", "iat": now, "exp": now + 300,
		"nonce": nonce, "email": email, "email_verified": emailVerified, "name": "OIDC Student",
	})
	if err != nil {
		return "", err
	}
	message := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign test identity token: %w", err)
	}
	return message + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
