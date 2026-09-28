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
