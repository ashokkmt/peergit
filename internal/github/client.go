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
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const APIVersion = "2026-03-10"

type Client struct {
	HTTP *http.Client
	API  string
}
type Repository struct {
	ID            int64           `json:"id"`
	FullName      string          `json:"full_name"`
	HTMLURL       string          `json:"html_url"`
	Description   string          `json:"description"`
	Private       bool            `json:"private"`
	Visibility    string          `json:"visibility"`
	DefaultBranch string          `json:"default_branch"`
	Topics        []string        `json:"topics"`
	Language      string          `json:"language"`
	Permissions   map[string]bool `json:"permissions"`
	PushedAt      time.Time       `json:"pushed_at"`
}
type Installation struct {
	ID                  int64             `json:"id"`
	Account             InstallationOwner `json:"account"`
	TargetType          string            `json:"target_type"`
	RepositorySelection string            `json:"repository_selection"`
	Permissions         map[string]string `json:"permissions"`
	SuspendedAt         *time.Time        `json:"suspended_at"`
}
type InstallationOwner struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}
type Commit struct {
	SHA    string       `json:"sha"`
	Commit CommitDetail `json:"commit"`
	Author *User        `json:"author"`
	Stats  CommitStats  `json:"stats"`
}
type CommitDetail struct {
	Message string       `json:"message"`
	Author  CommitPerson `json:"author"`
}
type CommitPerson struct {
	Name string    `json:"name"`
	Date time.Time `json:"date"`
}
type CommitStats struct {
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	Total     int `json:"total"`
}
type User struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

func (c *Client) User(ctx context.Context, token string) (User, error) {
	resp, err := c.request(ctx, http.MethodGet, "/user", token)
	if err != nil {
		return User{}, err
	}
	var user User
	err = decode(resp, &user)
	if err == nil && user.ID <= 0 {
		err = errors.New("GitHub user ID missing")
	}
	return user, err
}

func (c *Client) UserInstallations(ctx context.Context, token string) ([]Installation, error) {
	var installations []Installation
	for page := 1; page <= 100; page++ {
		resp, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/user/installations?per_page=100&page=%d", page), token)
		if err != nil {
			return nil, err
		}
		var out struct {
			Installations []Installation `json:"installations"`
		}
		if err = decode(resp, &out); err != nil {
			return nil, err
		}
		installations = append(installations, out.Installations...)
		if len(out.Installations) < 100 {
			return installations, nil
		}
	}
	return nil, errors.New("GitHub installation list exceeded the supported limit")
}

func (c *Client) UserInstallationRepositories(ctx context.Context, installationID int64, token string) ([]Repository, error) {
	if installationID <= 0 {
		return nil, errors.New("installation ID is invalid")
	}
	var repositories []Repository
	for page := 1; page <= 100; page++ {
		path := fmt.Sprintf("/user/installations/%d/repositories?per_page=100&page=%d", installationID, page)
		resp, err := c.request(ctx, http.MethodGet, path, token)
		if err != nil {
			return nil, err
		}
		var out struct {
			Repositories []Repository `json:"repositories"`
		}
		if err = decode(resp, &out); err != nil {
			return nil, err
		}
		repositories = append(repositories, out.Repositories...)
		if len(out.Repositories) < 100 {
			return repositories, nil
		}
	}
	return nil, errors.New("GitHub user installation repository list exceeded the supported limit")
}

type Activity struct {
	Number      int64     `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Comments    int       `json:"comments"`
	User        *User     `json:"user"`
	PullRequest *struct{} `json:"pull_request"`
}
type HTTPError struct {
	Status      int
	RateLimited bool
	RetryAfter  string
	Denied      bool
}

func (e *HTTPError) Error() string { return fmt.Sprintf("GitHub request failed (HTTP %d)", e.Status) }
func (e *HTTPError) RetryAfterDuration(now time.Time) time.Duration {
	if e == nil || e.RetryAfter == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(e.RetryAfter); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(e.RetryAfter); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}
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
func (c *Client) Installation(ctx context.Context, installation, jwt string) (Installation, error) {
	if !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(installation) {
		return Installation{}, errors.New("installation ID must be numeric")
	}
	resp, err := c.request(ctx, http.MethodGet, "/app/installations/"+installation, jwt)
	if err != nil {
		return Installation{}, err
	}
	var out Installation
	err = decode(resp, &out)
	return out, err
}

func (c *Client) InstallationRepositories(ctx context.Context, token string) ([]Repository, error) {
	var repositories []Repository
	for page := 1; page <= 100; page++ {
		resp, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/installation/repositories?per_page=100&page=%d", page), token)
		if err != nil {
			return nil, err
		}
		var out struct {
			Repositories []Repository `json:"repositories"`
		}
		if err = decode(resp, &out); err != nil {
			return nil, err
		}
		repositories = append(repositories, out.Repositories...)
		if len(out.Repositories) < 100 {
			return repositories, nil
		}
	}
	return nil, errors.New("installation repository list exceeded the 10000 repository safety limit")
}

func (c *Client) Commits(ctx context.Context, repo, token string, since time.Time, page int) ([]Commit, error) {
	path, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	if page < 1 || page > 100 {
		return nil, errors.New("commit page is outside the supported range")
	}
	query := url.Values{"per_page": {"100"}, "page": {fmt.Sprint(page)}}
	if !since.IsZero() {
		query.Set("since", since.UTC().Format(time.RFC3339))
	}
	resp, err := c.request(ctx, http.MethodGet, path+"/commits?"+query.Encode(), token)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	if err = decode(resp, &commits); err != nil {
		return nil, err
	}
	for _, commit := range commits {
		if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(commit.SHA) || commit.Stats.Additions < 0 || commit.Stats.Deletions < 0 {
			return nil, errors.New("GitHub returned an invalid commit record")
		}
	}
	return commits, nil
}

func (c *Client) PullRequests(ctx context.Context, repo, token string, page int) ([]Activity, error) {
	return c.activity(ctx, repo, token, "pulls", time.Time{}, page)
}

func (c *Client) Issues(ctx context.Context, repo, token string, since time.Time, page int) ([]Activity, error) {
	return c.activity(ctx, repo, token, "issues", since, page)
}

func (c *Client) activity(ctx context.Context, repo, token, kind string, since time.Time, page int) ([]Activity, error) {
	path, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	if kind != "pulls" && kind != "issues" || page < 1 || page > 100 {
		return nil, errors.New("invalid GitHub activity request")
	}
	query := url.Values{"per_page": {"100"}, "page": {fmt.Sprint(page)}, "state": {"all"}, "sort": {"updated"}, "direction": {"desc"}}
	if !since.IsZero() {
		query.Set("since", since.UTC().Format(time.RFC3339))
	}
	resp, err := c.request(ctx, http.MethodGet, path+"/"+kind+"?"+query.Encode(), token)
	if err != nil {
		return nil, err
	}
	var items []Activity
	if err = decode(resp, &items); err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Number <= 0 || item.Comments < 0 || len(item.Title) > 500 || len(item.Body) > 65536 {
			return nil, errors.New("GitHub returned an invalid activity record")
		}
	}
	return items, nil
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

func (c *Client) Languages(ctx context.Context, repo, token string) (map[string]int64, error) {
	path, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	resp, err := c.request(ctx, http.MethodGet, path+"/languages", token)
	if err != nil {
		return nil, err
	}
	var languages map[string]int64
	err = decode(resp, &languages)
	if err == nil && languages == nil {
		languages = map[string]int64{}
	}
	return languages, err
}

func (c *Client) Readme(ctx context.Context, repo, token string) (string, error) {
	path, err := repoPath(repo)
	if err != nil {
		return "", err
	}
	resp, err := c.request(ctx, http.MethodGet, path+"/readme", token)
	if err != nil {
		return "", err
	}
	var out struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		Size     int64  `json:"size"`
	}
	if err = decode(resp, &out); err != nil {
		return "", err
	}
	if out.Size > 1<<20 || out.Encoding != "base64" {
		return "", errors.New("README exceeds the supported size or encoding")
	}
	data, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(out.Content, "\n", ""))
	if err != nil || len(data) > 1<<20 || !utf8.Valid(data) {
		return "", errors.New("GitHub returned an invalid or oversized README")
	}
	return string(data), nil
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
