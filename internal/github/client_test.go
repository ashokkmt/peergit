package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJWTSignatureAndLifetime(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	now := time.Now()
	jwt, err := JWT("123", encoded, now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatal("invalid JWT shape")
	}
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	if err = rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, hash[:], signature); err != nil {
		t.Fatal(err)
	}
	claims, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Iss      string
		Iat, Exp int64
	}
	if err = json.Unmarshal(claims, &got); err != nil {
		t.Fatal(err)
	}
	if got.Iss != "123" || got.Exp-got.Iat != 600 || got.Iat >= now.Unix() {
		t.Fatalf("unexpected JWT claims: %#v", got)
	}
	if _, err = JWT("not-an-id", encoded, now); err == nil {
		t.Fatal("invalid App ID accepted")
	}
	if _, err = JWT("123", []byte("bad"), now); err == nil {
		t.Fatal("invalid key accepted")
	}
}

func TestRateLimitIsNotRevocationAndErrorsAreSafe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(403)
		_, _ = io.WriteString(w, `{"message":"rate limit secret-url"}`)
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), API: server.URL}
	_, err := c.Repository(context.Background(), "owner/repo", "private-token")
	if err == nil || IsDenied(err) || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private-token") {
		t.Fatalf("unexpected safe rate-limit error: %v", err)
	}
}

func TestRetryAfterDurationSupportsSecondsAndDates(t *testing.T) {
	seconds := &HTTPError{RetryAfter: "12"}
	if got := seconds.RetryAfterDuration(time.Now()); got != 12*time.Second {
		t.Fatalf("seconds Retry-After = %s", got)
	}
	date := time.Now().UTC().Add(20 * time.Second).Truncate(time.Second)
	formatted := &HTTPError{RetryAfter: date.Format(http.TimeFormat)}
	if got := formatted.RetryAfterDuration(date.Add(-5 * time.Second)); got != 5*time.Second {
		t.Fatalf("date Retry-After = %s, want 5s", got)
	}
	if got := (&HTTPError{RetryAfter: "bad"}).RetryAfterDuration(time.Now()); got != 0 {
		t.Fatalf("invalid Retry-After = %s, want zero", got)
	}
}

func TestInstallationRepositoryPaginationUsesInstallationToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer installation-token" {
			t.Fatal("repository listing did not use installation token")
		}
		switch r.URL.Query().Get("page") {
		case "1":
			_, _ = io.WriteString(w, `{"repositories":[{"id":41,"full_name":"campus/one","private":true}]}`)
		case "2":
			_, _ = io.WriteString(w, `{"repositories":[]}`)
		default:
			t.Fatalf("unexpected repository page %s", r.URL.RawQuery)
		}
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), API: server.URL}
	repos, err := c.InstallationRepositories(context.Background(), "installation-token")
	if err != nil || len(repos) != 1 || repos[0].ID != 41 || repos[0].FullName != "campus/one" {
		t.Fatalf("repositories = %#v, %v", repos, err)
	}
}

func TestInstallationReadsScopeWithAppJWT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/app/installations/42" || r.Header.Get("Authorization") != "Bearer app-jwt" {
			t.Fatal("installation request did not use the App JWT and exact installation path")
		}
		_, _ = io.WriteString(w, `{"repository_selection":"selected","permissions":{"contents":"read","metadata":"read"}}`)
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), API: server.URL}
	installation, err := c.Installation(context.Background(), "42", "app-jwt")
	if err != nil || installation.RepositorySelection != "selected" || installation.Permissions["contents"] != "read" {
		t.Fatalf("installation = %#v, %v", installation, err)
	}
	if _, err = c.Installation(context.Background(), "not-numeric", "app-jwt"); err == nil {
		t.Fatal("non-numeric installation ID accepted")
	}
}

func TestUserRepositoryAuthorizationIncludesAdminPermissionAndBoundsReadme(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer user-token" {
			t.Fatalf("authorization request used unexpected token: %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/user/installations/42/repositories":
			_, _ = io.WriteString(w, `{"repositories":[{"id":41003,"full_name":"test-owner/fixture","permissions":{"admin":true,"push":true}}]}`)
		case "/repos/test-owner/fixture/readme":
			_, _ = io.WriteString(w, `{"encoding":"base64","size":5,"content":"SGVsbG8="}`)
		case "/repos/test-owner/large/readme":
			_, _ = io.WriteString(w, `{"encoding":"base64","size":1048577,"content":"eA=="}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), API: server.URL}
	repositories, err := c.UserInstallationRepositories(context.Background(), 42, "user-token")
	if err != nil || len(repositories) != 1 || repositories[0].ID != 41003 || !repositories[0].Permissions["admin"] {
		t.Fatalf("authorized repositories = %#v, %v", repositories, err)
	}
	readme, err := c.Readme(context.Background(), "test-owner/fixture", "user-token")
	if err != nil || readme != "Hello" {
		t.Fatalf("README = %q, %v", readme, err)
	}
	if _, err = c.Readme(context.Background(), "test-owner/large", "user-token"); err == nil {
		t.Fatal("oversized README accepted")
	}
	if _, err = c.UserInstallationRepositories(context.Background(), 0, "user-token"); err == nil {
		t.Fatal("invalid installation ID accepted")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestArchivePinsSHAAndDoesNotForwardToken(t *testing.T) {
	sha := strings.Repeat("a", 40)
	calls := 0
	c := New()
	c.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			if !strings.HasSuffix(r.URL.Path, "/tarball/"+sha) || r.Header.Get("Authorization") != "Bearer private-token" {
				t.Fatal("archive was not SHA-pinned and authorized")
			}
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://codeload.github.com/owner/repo/tar.gz/" + sha + "?signed=private"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatal("installation token leaked to download host")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("opaque")), Request: r}, nil
	})
	body, err := c.Archive(context.Background(), "owner/repo", sha, "private-token")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil || string(got) != "opaque" || calls != 2 {
		t.Fatalf("archive = %q, %v", got, err)
	}
	if _, err = c.Archive(context.Background(), "owner/repo", "main", "private-token"); err == nil {
		t.Fatal("mutable ref accepted as archive identity")
	}
}

func TestArchiveRejectsUntrustedRedirect(t *testing.T) {
	for _, location := range []string{"http://codeload.github.com/file", "https://evil.example/file", "https://user:secret@codeload.github.com/file"} {
		t.Run(location, func(t *testing.T) {
			c := New()
			c.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{location}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			})
			_, err := c.Archive(context.Background(), "owner/repo", strings.Repeat("a", 40), "token")
			if err == nil {
				t.Fatal("unsafe redirect accepted")
			}
		})
	}
}

func TestResolveRejectsMalformedAndOversizedProviderResponses(t *testing.T) {
	for _, body := range []string{`{"sha":"main"}`, strings.Repeat("x", (1<<20)+1), `{"sha":`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
		c := &Client{HTTP: server.Client(), API: server.URL}
		_, err := c.Resolve(context.Background(), "owner/repo", "main", "token")
		server.Close()
		if err == nil {
			t.Fatal("invalid provider response accepted")
		}
	}
}

func TestResolveRefReturnsImmutableCommitSHA(t *testing.T) {
	sha := strings.Repeat("b", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/commits/main" || r.Header.Get("Authorization") != "Bearer installation-token" {
			t.Fatalf("unexpected ref request path=%q authorization=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"sha":"`+sha+`"}`)
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), API: server.URL}
	got, err := c.Resolve(context.Background(), "owner/repo", "main", "installation-token")
	if err != nil || got != sha {
		t.Fatalf("resolved SHA = %q, %v; want %q", got, err, sha)
	}
}
