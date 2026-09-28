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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const APIVersion = "2026-03-10"

type Client struct {
	HTTP *http.Client
	API  string
}
type Repository struct {
	ID      int64 `json:"id"`
	Private bool  `json:"private"`
}
type HTTPError struct {
	Status      int
	RateLimited bool
	RetryAfter  string
	Denied      bool
}

func (e *HTTPError) Error() string { return fmt.Sprintf("GitHub request failed (HTTP %d)", e.Status) }
func IsDenied(err error) bool {
	var e *HTTPError
	return errors.As(err, &e) && e.Denied && !e.RateLimited
}

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != "https" || req.URL.Host != "api.github.com" || req.URL.User != nil {
			return errors.New("untrusted API redirect")
		}
		return nil
	}}, API: "https://api.github.com"}
}
func JWT(appID string, keyPEM []byte, now time.Time) (string, error) {
	if !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(appID) {
		return "", errors.New("app ID must be numeric")
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return "", errors.New("invalid PEM key")
	}
	var key *rsa.PrivateKey
	if parsed, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = parsed
	} else {
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return "", errors.New("invalid RSA private key")
		}
		var ok bool
		key, ok = parsed.(*rsa.PrivateKey)
		if !ok {
			return "", errors.New("private key must be RSA")
		}
	}
	if key.N.BitLen() < 2048 {
		return "", errors.New("RSA key must be at least 2048 bits")
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{"iss": appID, "iat": now.Add(-time.Minute).Unix(), "exp": now.Add(9 * time.Minute).Unix()})
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	hash := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", errors.New("JWT signing failed")
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (c *Client) request(ctx context.Context, method, path, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.API+path, nil)
	if err != nil {
		return nil, errors.New("invalid GitHub request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", APIVersion)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, errors.New("GitHub transport failed")
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	limited := resp.StatusCode == 429 || resp.Header.Get("X-RateLimit-Remaining") == "0" || (resp.StatusCode == 403 && (strings.Contains(strings.ToLower(string(body)), "rate limit") || resp.Header.Get("Retry-After") != ""))
	denied := resp.StatusCode == 401 || resp.StatusCode == 404 || (resp.StatusCode == 403 && (strings.Contains(strings.ToLower(string(body)), "suspend") || strings.Contains(string(body), "Resource not accessible by integration")))
	return nil, &HTTPError{resp.StatusCode, limited, resp.Header.Get("Retry-After"), denied}
}
func decode(resp *http.Response, dst any) error {
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return errors.New("GitHub response exceeded limit or was interrupted")
	}
	if err = json.Unmarshal(data, dst); err != nil {
		return errors.New("invalid GitHub JSON response")
	}
	return nil
}

func (c *Client) Token(ctx context.Context, installation, jwt string) (string, error) {
	if !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(installation) {
		return "", errors.New("installation ID must be numeric")
	}
	resp, err := c.request(ctx, http.MethodPost, "/app/installations/"+installation+"/access_tokens", jwt)
	if err != nil {
		return "", err
	}
	var out struct {
		Token string `json:"token"`
	}
	if err = decode(resp, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", errors.New("installation token missing")
	}
	return out.Token, nil
}
func repoPath(repo string) (string, error) {
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(repo) {
		return "", errors.New("repository must be owner/name")
	}
	return "/repos/" + repo, nil
}
func (c *Client) Repository(ctx context.Context, repo, token string) (Repository, error) {
	path, err := repoPath(repo)
	if err != nil {
		return Repository{}, err
	}
	resp, err := c.request(ctx, http.MethodGet, path, token)
	if err != nil {
		return Repository{}, err
	}
	var out Repository
	err = decode(resp, &out)
	if err == nil && out.ID <= 0 {
		err = errors.New("repository ID missing")
	}
	return out, err
}
func (c *Client) Resolve(ctx context.Context, repo, ref, token string) (string, error) {
	path, err := repoPath(repo)
	if err != nil {
		return "", err
	}
	resp, err := c.request(ctx, http.MethodGet, path+"/commits/"+url.PathEscape(ref), token)
	if err != nil {
		return "", err
	}
	var out struct {
		SHA string `json:"sha"`
	}
	if err = decode(resp, &out); err != nil {
		return "", err
	}
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(out.SHA) {
		return "", errors.New("invalid GitHub commit SHA")
	}
	return out.SHA, nil
}
func (c *Client) Archive(ctx context.Context, repo, sha, token string) (io.ReadCloser, error) {
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(sha) {
		return nil, errors.New("archive requires exact SHA")
	}
	path, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	apiHTTP := *c.HTTP
	apiHTTP.Timeout = 30 * time.Second
	apiHTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.API+path+"/tarball/"+sha, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", APIVersion)
	resp, err := apiHTTP.Do(req)
	if err != nil {
		return nil, errors.New("archive authorization request failed")
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		return nil, &HTTPError{Status: resp.StatusCode, RateLimited: resp.StatusCode == 429 || resp.Header.Get("X-RateLimit-Remaining") == "0", Denied: resp.StatusCode == 401 || resp.StatusCode == 404}
	}
	location, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || location.Scheme != "https" || location.Host != "codeload.github.com" || location.User != nil || location.Fragment != "" {
		return nil, errors.New("untrusted archive redirect")
	}
	archiveHTTP := *c.HTTP
	archiveHTTP.Timeout = 180 * time.Second
	archiveHTTP.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != "https" || req.URL.Host != "codeload.github.com" || req.URL.User != nil {
			return errors.New("untrusted archive redirect")
		}
		req.Header.Del("Authorization")
		return nil
	}
	// A separate request never forwards the installation token to a signed download URL.
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, location.String(), nil)
	if err != nil {
		return nil, errors.New("invalid archive location")
	}
	resp, err = archiveHTTP.Do(req)
	if err != nil {
		return nil, errors.New("archive download failed")
	}
	if resp.StatusCode != 200 {
		_ = resp.Body.Close()
		return nil, errors.New("archive download was rejected")
	}
	return resp.Body, nil
}
