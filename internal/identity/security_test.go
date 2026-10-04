package identity

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"peergit/internal/platform/errormanager"
)

func TestPKCEChallengeRFC7636(t *testing.T) {
	got := pkceChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("challenge = %q", got)
	}
}

func TestTOTPAndAESGCMSecretRoundTrip(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	code, err := totp(secret, time.Unix(59, 0))
	if err != nil || code != "287082" {
		t.Fatalf("RFC TOTP = %q, %v; want 287082", code, err)
	}
	if !verifyTOTP(secret, code, time.Unix(59, 0)) || verifyTOTP(secret, "000000", time.Unix(59, 0)) {
		t.Fatal("TOTP verifier did not accept only the matching code")
	}
	key := []byte("test master key")
	ciphertext, err := encryptSecret(key, []byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := decryptSecret(key, ciphertext)
	if err != nil || string(plaintext) != secret {
		t.Fatalf("decrypt = %q, %v", plaintext, err)
	}
	ciphertext[len(ciphertext)-1] ^= 1
	if _, err = decryptSecret(key, ciphertext); err == nil {
		t.Fatal("tampered MFA ciphertext was accepted")
	}
}

func TestOpaqueTokensAreUniqueAndURLSafe(t *testing.T) {
	a, err := randomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	b, err := randomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(a) < 40 {
		t.Fatalf("tokens are not independent opaque values: %q %q", a, b)
	}
	for _, r := range a {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			t.Fatalf("token has non URL-safe character %q", r)
		}
	}
}

func TestSessionCSRFTokenIsStableAndSessionBound(t *testing.T) {
	key := []byte("test session hash key with enough entropy")
	first := sessionCSRFToken(key, "session-one")
	if first != sessionCSRFToken(key, "session-one") {
		t.Fatal("CSRF token changed across reads for the same session")
	}
	if first == sessionCSRFToken(key, "session-two") {
		t.Fatal("different sessions received the same CSRF token")
	}
	if hmacEqual(keyedHash(key, []byte(first)), keyedHash(key, []byte("session-one"))) {
		t.Fatal("CSRF and session token derivation are not domain separated")
	}
}

func TestGitHubCallbackRequiresBrowserBoundState(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(nil, Config{AppOrigin: "http://peergit.test"}, logger, errormanager.NewManager(logger))
	for name, cookie := range map[string]string{"missing": "", "mismatched": "attacker-state"} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github/callback?state=real-state&code=code", nil)
			if cookie != "" {
				req.AddCookie(&http.Cookie{Name: "peergit_github_state", Value: cookie})
			}
			res := httptest.NewRecorder()
			handler.githubCallback(res, req)
			if res.Code != http.StatusUnauthorized {
				t.Fatalf("callback status=%d body=%s; want rejected state", res.Code, res.Body.String())
			}
		})
	}
}

func TestRateLimitAddressIgnoresEphemeralPort(t *testing.T) {
	if got := remoteHost("192.0.2.10:43122"); got != "192.0.2.10" {
		t.Fatalf("remote host = %q; want stable client address", got)
	}
	if got := remoteHost("192.0.2.10"); got != "192.0.2.10" {
		t.Fatalf("remote host for unqualified address = %q", got)
	}
}
