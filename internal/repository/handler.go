package repository

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"peergit/internal/github"
	"peergit/internal/identity"
	"peergit/internal/platform/errormanager"
	"peergit/internal/platform/http/response"
	"peergit/internal/platform/jobs"
	"peergit/internal/platform/storage"
)

const maxArchiveBytes int64 = 100 << 20

type AppConfig struct {
	ID, Slug, WebhookSecret, Origin                    string
	OAuthClientID, OAuthClientSecret, OAuthRedirectURL string
	OAuthAuthorizeURL, OAuthTokenURL, APIURL           string
	PrivateKey                                         []byte
	Forge                                              ForgeConfig
}

type Handler struct {
	pool         *pgxpool.Pool
	auth         *identity.Handler
	api          *github.Client
	store        *storage.Store
	app          AppConfig
	forge        ForgeConfig
	pblTransfers chan struct{}
	log          *slog.Logger
	err          *errormanager.Manager
}

func NewHandler(pool *pgxpool.Pool, auth *identity.Handler, api *github.Client, store *storage.Store, app AppConfig, logger *slog.Logger, errors *errormanager.Manager) *Handler {
	return &Handler{pool: pool, auth: auth, api: api, store: store, app: app, forge: app.Forge, pblTransfers: make(chan struct{}, 4), log: logger, err: errors}
}

func (h *Handler) Register(r chi.Router) {
	r.Post("/github/webhooks", h.webhook)
	r.HandleFunc("/pbl/git/{repository}/*", h.pblGit)
	r.Post("/pbl/forgejo/webhooks", h.pblWebhook)
	r.Group(func(private chi.Router) {
		private.Use(h.auth.Middleware)
		private.With(h.auth.RequireCSRF).Post("/github/installations/start", h.startAccountInstallation)
		private.Get("/github/installations/callback", h.finishAccountInstallation)
		private.Get("/github/authorization/callback", h.finishRepositoryAuthorization)
		private.Get("/me/github/repositories", h.myGitHubRepositories)
		private.Get("/me/github/installations", h.myGitHubInstallations)
		private.With(h.auth.RequireCSRF).Post("/me/github/installations/{installationID}/refresh", h.refreshGitHubInstallation)
		private.Get("/me/github/installations/{installationID}/imports", h.githubInstallationImports)
		private.With(h.auth.RequireCSRF).Post("/me/github/installations/{installationID}/revoke", h.revokeGitHubInstallation)
		private.Get("/me/github/retained-projects", h.retainedGitHubProjects)
		private.With(h.auth.RequireCSRF).Post("/projects/{id}/github/restore", h.restoreGitHubProject)
		private.With(h.auth.RequireCSRF).Post("/projects/import/github", h.startGitHubImport)
		private.With(h.auth.RequireCSRF).Post("/projects/{id}/github/link", h.startLegacyGitHubLink)
		private.Get("/my/projects", h.myProjects)
		private.Get("/projects/{id}/repository", h.projectRepository)
		private.Get("/projects/{id}/repository/tree", h.repositoryTree)
		private.Get("/projects/{id}/repository/branches", h.repositoryBranches)
		private.Get("/projects/{id}/repository/history", h.repositoryHistory)
		private.Get("/projects/{id}/repository/commit", h.repositoryCommit)
		private.Get("/projects/{id}/repository/file", h.repositoryFile)

		h.registerPBL(private)
	})
}

func (h *Handler) projectRepository(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	projectID := chi.URLParam(r, "id")
	if !h.isProjectMember(r.Context(), projectID, s) {
		h.fail(w, r, 404, "project_not_found", "project was not found", nil)
		return
	}
	var repositoryID string
	err := h.pool.QueryRow(r.Context(), `SELECT id::text FROM repositories WHERE project_id=$1 AND college_id=$2`, projectID, s.CollegeID).Scan(&repositoryID)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = response.OK(w, map[string]any{"repository": nil, "contributions": []any{}})
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var binding map[string]any
	var bindingID, owner, name, access, syncHealth, installationStatus string
	var externalID int64
	var synced *time.Time
	var permissions []byte
	var htmlURL, description, readme string
	var topics []string
	var languages []byte
	var defaultBranch string
	err = h.pool.QueryRow(r.Context(), `SELECT b.id::text,b.external_repository_id,b.owner_login,b.repository_name,b.default_branch,b.access_state,b.sync_health,b.last_synced_at,i.permissions,b.html_url,b.description,b.topics,b.languages,b.readme_markdown,i.status FROM repository_bindings b JOIN github_installations i ON i.id=b.installation_id AND i.college_id=b.college_id WHERE b.repository_id=$1 AND b.college_id=$2 AND b.is_current`, repositoryID, s.CollegeID).Scan(&bindingID, &externalID, &owner, &name, &defaultBranch, &access, &syncHealth, &synced, &permissions, &htmlURL, &description, &topics, &languages, &readme, &installationStatus)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.err.Handle(w, r, err)
		return
	}
	if err == nil {
		var grants map[string]string
		_ = json.Unmarshal(permissions, &grants)
		var languageMap map[string]int64
		_ = json.Unmarshal(languages, &languageMap)
		if languageMap == nil {
			languageMap = map[string]int64{}
		}
		if access != "active" || installationStatus != "active" {
			htmlURL, readme, description, topics, languageMap = "", "", "", nil, map[string]int64{}
		}
		binding = map[string]any{"id": bindingID, "external_repository_id": externalID, "owner": owner, "name": name, "full_name": owner + "/" + name, "html_url": htmlURL, "description": description, "default_branch": defaultBranch, "topics": topics, "languages": languageMap, "readme_markdown": readme, "access_state": access, "sync_health": syncHealth, "last_synced_at": synced, "capabilities": grants, "limitations": "Git LFS objects and submodule contents are not rendered in file views."}
	}
	if binding != nil {
		binding["provider"] = "github"
		binding["source_access"] = access == "active" && installationStatus == "active"
	}
	contributions := make([]map[string]any, 0)
	rows, err := h.pool.Query(r.Context(), `SELECT c.id::text,c.kind,c.canonical_id,c.github_author_id,c.author_login,c.author_name,c.title,c.summary,c.occurred_at,c.additions,c.deletions,c.changed_files,u.id::text,o.data,o.provider_updated_at,o.observed_at
		FROM contributions c LEFT JOIN user_identities ui ON ui.provider='github' AND ui.subject=c.github_author_id::text LEFT JOIN users u ON u.id=ui.user_id AND u.college_id=c.college_id AND u.status='active'
		LEFT JOIN LATERAL (SELECT data,provider_updated_at,observed_at FROM contribution_observations WHERE contribution_id=c.id AND binding_id=NULLIF($3,'')::uuid AND college_id=c.college_id ORDER BY provider_updated_at DESC NULLS LAST, observed_at DESC LIMIT 1) o ON true
		WHERE c.repository_id=$1 AND c.college_id=$2 AND $4 AND (c.kind='commit' OR o.data IS NOT NULL) ORDER BY c.occurred_at DESC NULLS LAST,c.id DESC LIMIT 100`, repositoryID, s.CollegeID, bindingID, access == "active" && installationStatus == "active")
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var item map[string]any
		var id, kind, canonical, login, author, title, summary string
		var githubID *int64
		var occurred *time.Time
		var additions, deletions, files *int
		var linked *string
		var observation []byte
		var updated, observed *time.Time
		if err = rows.Scan(&id, &kind, &canonical, &githubID, &login, &author, &title, &summary, &occurred, &additions, &deletions, &files, &linked, &observation, &updated, &observed); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		item = map[string]any{"id": id, "kind": kind, "canonical_id": canonical, "github_author_id": githubID, "author_login": login, "author_name": author, "title": title, "summary": summary, "occurred_at": occurred, "additions": additions, "deletions": deletions, "changed_files": files, "linked_user_id": linked}
		var details struct {
			Number int    `json:"number"`
			State  string `json:"state"`
		}
		if json.Unmarshal(observation, &details) == nil && details.Number > 0 {
			item["number"], item["state"] = details.Number, details.State
		}
		item["provider_updated_at"], item["observed_at"] = updated, observed
		contributions = append(contributions, item)
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"repository": binding, "contributions": contributions})
}

func (h *Handler) serveSnapshotFile(w http.ResponseWriter, r *http.Request, key, sha string) {
	body, err := h.store.Open(r.Context(), key)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="peergit-`+sha[:12]+`.tar.gz"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")
	if _, err = io.Copy(w, body); err != nil {
		h.err.Report(r, err)
	}
}

func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	if len(h.app.WebhookSecret) < 32 {
		http.Error(w, "webhook is unavailable", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		http.Error(w, "invalid webhook body", http.StatusRequestEntityTooLarge)
		return
	}
	if !validWebhookSignature(h.app.WebhookSecret, body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "invalid webhook signature", http.StatusUnauthorized)
		return
	}
	delivery := r.Header.Get("X-GitHub-Delivery")
	event := r.Header.Get("X-GitHub-Event")
	if !uuidOK(delivery) || event == "" || len(event) > 100 {
		http.Error(w, "invalid webhook headers", http.StatusBadRequest)
		return
	}
	var payload map[string]json.RawMessage
	if err = json.Unmarshal(body, &payload); err != nil || payload == nil {
		http.Error(w, "invalid webhook JSON", http.StatusBadRequest)
		return
	}
	var installation struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(payload["installation"], &installation)
	var repo struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(payload["repository"], &repo)
	action := ""
	_ = json.Unmarshal(payload["action"], &action)
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "webhook could not be saved", 500)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	var tenant, actor string
	if installation.ID > 0 {
		_ = tx.QueryRow(r.Context(), `SELECT college_id::text,added_by::text FROM github_installations WHERE external_installation_id=$1`, installation.ID).Scan(&tenant, &actor)
	}
	state := "pending"
	if tenant == "" {
		state = "unmatched"
	}
	tag, err := tx.Exec(r.Context(), `INSERT INTO github_webhook_deliveries(delivery_id,event_type,action,external_installation_id,college_id,payload,state) VALUES($1,$2,$3,NULLIF($4,0),NULLIF($5,'')::uuid,$6::jsonb,$7) ON CONFLICT(delivery_id) DO NOTHING`, delivery, event, action, installation.ID, tenant, string(body), state)
	if err != nil {
		http.Error(w, "webhook could not be saved", 500)
		return
	}
	if tag.RowsAffected() == 1 && tenant != "" {
		jobPayload, _ := json.Marshal(map[string]string{"delivery_id": delivery})
		if _, err = jobs.EnqueueTx(r.Context(), tx, tenant, actor, "github_webhook", "delivery:"+delivery, jobPayload, nil); err != nil {
			http.Error(w, "webhook could not be queued", 500)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		http.Error(w, "webhook could not be saved", 500)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func validWebhookSignature(secret string, body []byte, header string) bool {
	if len(secret) < 32 || !strings.HasPrefix(header, "sha256=") {
		return false
	}
	signature, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if err != nil || len(signature) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hmac.Equal(signature, mac.Sum(nil))
}

func (h *Handler) isProjectMember(ctx context.Context, projectID string, s identity.Session) bool {
	if !uuidOK(projectID) || s.CollegeID == "" {
		return false
	}
	var yes bool
	err := h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members pm JOIN projects p ON p.id=pm.project_id AND p.college_id=pm.college_id WHERE pm.project_id=$1 AND pm.college_id=$2 AND pm.user_id=$3 AND p.github_revoke_deadline IS NULL)`, projectID, s.CollegeID, s.UserID).Scan(&yes)
	return err == nil && yes
}
func (h *Handler) campusSession(w http.ResponseWriter, r *http.Request) (identity.Session, bool) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, 401, "authentication_required", "sign in is required", nil)
		return s, false
	}
	if s.CollegeID == "" || !s.TermsAccepted || !s.PrivacyAccepted {
		h.fail(w, r, 403, "campus_access_required", "verified campus access and current consent are required", nil)
		return s, false
	}
	return s, true
}
func (h *Handler) providerFailure(w http.ResponseWriter, r *http.Request, err error) {
	var status int
	var code, msg string
	var providerErr *github.HTTPError
	if errors.As(err, &providerErr) && providerErr.RateLimited {
		if providerErr.RetryAfter != "" {
			w.Header().Set("Retry-After", providerErr.RetryAfter)
		}
		status, code, msg = 429, "github_rate_limited", "GitHub rate limit reached. Try again shortly."
	} else if github.IsDenied(err) {
		status, code, msg = 403, "github_access_denied", "GitHub installation cannot access this resource"
	} else {
		status, code, msg = 502, "github_unavailable", "GitHub could not complete the request"
	}
	h.fail(w, r, status, code, msg, err)
}
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, status int, code, message string, cause error) {
	h.err.Handle(w, r, errormanager.New(status, code, message, cause))
}
func jsonValue(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

var numericPattern = regexp.MustCompile(`^[1-9][0-9]*$`)
var repoSegment = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

func numericID(s string) bool { return numericPattern.MatchString(s) }
func uuidOK(s string) bool    { return uuidPattern.MatchString(s) }
