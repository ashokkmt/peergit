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
	"peergit/internal/platform/http/request"
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
}

type Handler struct {
	pool  *pgxpool.Pool
	auth  *identity.Handler
	api   *github.Client
	store *storage.Store
	app   AppConfig
	log   *slog.Logger
	err   *errormanager.Manager
}

func NewHandler(pool *pgxpool.Pool, auth *identity.Handler, api *github.Client, store *storage.Store, app AppConfig, logger *slog.Logger, errors *errormanager.Manager) *Handler {
	return &Handler{pool: pool, auth: auth, api: api, store: store, app: app, log: logger, err: errors}
}

func (h *Handler) Register(r chi.Router) {
	r.Post("/github/webhooks", h.webhook)
	r.Group(func(private chi.Router) {
		private.Use(h.auth.Middleware)
		private.With(h.auth.RequireCSRF).Post("/github/installations/start", h.startAccountInstallation)
		private.Get("/github/installations/callback", h.finishAccountInstallation)
		private.Get("/github/authorization/callback", h.finishRepositoryAuthorization)
		private.Get("/me/github/repositories", h.myGitHubRepositories)
		private.Get("/me/github/installations", h.myGitHubInstallations)
		private.With(h.auth.RequireCSRF).Post("/projects/import/github", h.startGitHubImport)
		private.With(h.auth.RequireCSRF).Post("/projects/{id}/github/link", h.startLegacyGitHubLink)
		private.Get("/my/projects", h.myProjects)
		private.Get("/projects/{id}/repository", h.projectRepository)
		private.With(h.auth.RequireCSRF).Post("/projects/{id}/repository/snapshots", h.requestSnapshot)
		private.Get("/projects/{id}/repository/snapshots/{snapshotID}", h.snapshotStatus)
		private.With(h.auth.RequireCSRF).Post("/projects/{id}/repository/snapshots/{snapshotID}/retry", h.retrySnapshot)
		private.Get("/projects/{id}/repository/snapshots/{snapshotID}/download", h.downloadSnapshot)
	})
}

func (h *Handler) projectRepository(w http.ResponseWriter, r *http.Request) {
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
		_ = response.OK(w, map[string]any{"repository": nil, "contributions": []any{}, "snapshots": []any{}})
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var binding map[string]any
	var bindingID, owner, name, access, syncHealth string
	var externalID int64
	var synced *time.Time
	var permissions []byte
	var htmlURL, description, readme string
	var topics []string
	var languages []byte
	var defaultBranch string
	err = h.pool.QueryRow(r.Context(), `SELECT b.id::text,b.external_repository_id,b.owner_login,b.repository_name,b.default_branch,b.access_state,b.sync_health,b.last_synced_at,i.permissions,b.html_url,b.description,b.topics,b.languages,b.readme_markdown FROM repository_bindings b JOIN github_installations i ON i.id=b.installation_id AND i.college_id=b.college_id WHERE b.repository_id=$1 AND b.college_id=$2 AND b.is_current`, repositoryID, s.CollegeID).Scan(&bindingID, &externalID, &owner, &name, &defaultBranch, &access, &syncHealth, &synced, &permissions, &htmlURL, &description, &topics, &languages, &readme)
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
		binding = map[string]any{"id": bindingID, "external_repository_id": externalID, "owner": owner, "name": name, "full_name": owner + "/" + name, "html_url": htmlURL, "description": description, "default_branch": defaultBranch, "topics": topics, "languages": languageMap, "readme_markdown": readme, "access_state": access, "sync_health": syncHealth, "last_synced_at": synced, "capabilities": grants, "limitations": "Git LFS objects and submodule contents are not independently captured."}
	}
	contributions := make([]map[string]any, 0)
	rows, err := h.pool.Query(r.Context(), `SELECT c.id::text,c.kind,c.canonical_id,c.github_author_id,c.author_login,c.author_name,c.title,c.summary,c.occurred_at,c.additions,c.deletions,c.changed_files,u.id::text
		FROM contributions c LEFT JOIN user_identities ui ON ui.provider='github' AND ui.subject=c.github_author_id::text LEFT JOIN users u ON u.id=ui.user_id AND u.college_id=c.college_id AND u.status='active'
		WHERE c.repository_id=$1 AND c.college_id=$2 ORDER BY c.occurred_at DESC NULLS LAST,c.id DESC LIMIT 100`, repositoryID, s.CollegeID)
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
		if err = rows.Scan(&id, &kind, &canonical, &githubID, &login, &author, &title, &summary, &occurred, &additions, &deletions, &files, &linked); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		item = map[string]any{"id": id, "kind": kind, "canonical_id": canonical, "github_author_id": githubID, "author_login": login, "author_name": author, "title": title, "summary": summary, "occurred_at": occurred, "additions": additions, "deletions": deletions, "changed_files": files, "linked_user_id": linked}
		contributions = append(contributions, item)
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	snapshots := make([]map[string]any, 0)
	srows, err := h.pool.Query(r.Context(), `SELECT s.id::text,s.requested_ref,s.commit_sha,s.state,s.receipt_at,s.archive_bytes,s.archive_sha256,s.failure_code,COALESCE(j.state='dead',false) FROM repository_snapshots s LEFT JOIN jobs j ON j.id=s.job_id WHERE s.project_id=$1 AND s.college_id=$2 ORDER BY s.created_at DESC,s.id DESC LIMIT 50`, projectID, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer srows.Close()
	for srows.Next() {
		var m map[string]any
		var id, ref, sha, state string
		var receipt time.Time
		var n *int64
		var hash, code *string
		var retryAvailable bool
		if err = srows.Scan(&id, &ref, &sha, &state, &receipt, &n, &hash, &code, &retryAvailable); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		m = map[string]any{"id": id, "ref": ref, "commit_sha": sha, "state": state, "receipt_at": receipt, "archive_bytes": n, "archive_sha256": hash, "failure_code": code, "retry_available": retryAvailable}
		snapshots = append(snapshots, m)
	}
	_ = response.OK(w, map[string]any{"repository": binding, "contributions": contributions, "snapshots": snapshots})
}

type snapshotRequest struct {
	BindingID string `json:"binding_id"`
	Ref       string `json:"ref"`
}

func (in *snapshotRequest) Validate() error {
	in.Ref = strings.TrimSpace(in.Ref)
	if !uuidOK(in.BindingID) || in.Ref == "" || len(in.Ref) > 255 || strings.ContainsAny(in.Ref, "\r\n\x00") {
		return errors.New("binding ID and a valid branch, tag, or SHA are required")
	}
	return nil
}

func (h *Handler) requestSnapshot(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	projectID := chi.URLParam(r, "id")
	if !h.isProjectMember(r.Context(), projectID, s) {
		h.fail(w, r, 404, "project_not_found", "project was not found", nil)
		return
	}
	in, err := request.Decode[snapshotRequest](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 200 {
		h.fail(w, r, 400, "idempotency_key_required", "a valid Idempotency-Key header is required", nil)
		return
	}
	var owner, name, repoID, collegeID, installationID string
	var externalRepo int64
	err = h.pool.QueryRow(r.Context(), `SELECT b.owner_login,b.repository_name,b.repository_id::text,b.college_id::text,i.external_installation_id::text,b.external_repository_id
		FROM repository_bindings b JOIN repositories rp ON rp.id=b.repository_id AND rp.college_id=b.college_id JOIN github_installations i ON i.id=b.installation_id AND i.college_id=b.college_id
		WHERE b.id=$1 AND b.repository_id=(SELECT id FROM repositories WHERE project_id=$2 AND college_id=$3) AND b.college_id=$3 AND b.is_current AND b.access_state='active' AND i.status='active'`, in.BindingID, projectID, s.CollegeID).Scan(&owner, &name, &repoID, &collegeID, &installationID, &externalRepo)
	if err != nil {
		h.fail(w, r, 404, "repository_not_available", "active repository connection was not found", err)
		return
	}
	if h.store == nil {
		h.fail(w, r, 503, "snapshot_storage_unavailable", "snapshot storage is not configured", nil)
		return
	}
	jwt, err := github.JWT(h.app.ID, h.app.PrivateKey, time.Now())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	token, err := h.api.Token(r.Context(), installationID, jwt)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	sha, err := h.api.Resolve(r.Context(), owner+"/"+name, in.Ref, token)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	body, _ := json.Marshal(in)
	hash := sha256.Sum256(body)
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	var authorized string
	err = tx.QueryRow(r.Context(), `SELECT pm.project_id::text FROM project_members pm JOIN projects p ON p.id=pm.project_id AND p.college_id=pm.college_id WHERE pm.project_id=$1 AND pm.college_id=$2 AND pm.user_id=$3 AND p.lifecycle<>'archived' FOR SHARE OF pm,p`, projectID, s.CollegeID, s.UserID).Scan(&authorized)
	if err != nil {
		h.fail(w, r, 404, "project_not_found", "project was not found", err)
		return
	}
	var stillBound bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM repository_bindings b JOIN github_installations i ON i.id=b.installation_id AND i.college_id=b.college_id WHERE b.id=$1 AND b.repository_id=$2 AND b.college_id=$3 AND b.is_current AND b.access_state='active' AND i.status='active')`, in.BindingID, repoID, s.CollegeID).Scan(&stillBound); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !stillBound {
		h.fail(w, r, 409, "repository_access_changed", "repository access changed before the snapshot receipt was recorded", nil)
		return
	}
	var members []byte
	err = tx.QueryRow(r.Context(), `SELECT COALESCE(jsonb_agg(jsonb_build_object('user_id',pm.user_id::text,'role',pm.role) ORDER BY pm.user_id),'[]'::jsonb) FROM project_members pm WHERE pm.project_id=$1 AND pm.college_id=$2`, projectID, s.CollegeID).Scan(&members)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var snapshotID string
	err = tx.QueryRow(r.Context(), `INSERT INTO repository_snapshots(college_id,project_id,repository_id,binding_id,requested_by,idempotency_key,request_hash,requested_ref,commit_sha,membership_snapshot)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT(college_id,requested_by,idempotency_key) DO NOTHING RETURNING id::text`, s.CollegeID, projectID, repoID, in.BindingID, s.UserID, key, hash[:], in.Ref, sha, members).Scan(&snapshotID)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingHash []byte
		err = tx.QueryRow(r.Context(), `SELECT id::text,request_hash FROM repository_snapshots WHERE college_id=$1 AND requested_by=$2 AND idempotency_key=$3 FOR UPDATE`, s.CollegeID, s.UserID, key).Scan(&snapshotID, &existingHash)
		if err == nil && !hmac.Equal(existingHash, hash[:]) {
			err = errors.New("snapshot idempotency key was reused with a different request")
		}
	}
	if err != nil {
		h.fail(w, r, 409, "snapshot_request_conflict", "snapshot request could not be accepted", err)
		return
	}
	var state string
	if err = tx.QueryRow(r.Context(), `SELECT state FROM repository_snapshots WHERE id=$1`, snapshotID).Scan(&state); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if state == "pending" {
		payload, _ := json.Marshal(map[string]string{"snapshot_id": snapshotID})
		jobID, enqueueErr := jobs.EnqueueTx(r.Context(), tx, s.CollegeID, s.UserID, "snapshot", "snapshot:"+snapshotID, payload, nil)
		if enqueueErr != nil {
			err = enqueueErr
			h.err.Handle(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE repository_snapshots SET job_id=$2 WHERE id=$1 AND job_id IS NULL`, snapshotID, jobID); err != nil {
			h.err.Handle(w, r, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": snapshotID, "commit_sha": sha, "state": state, "receipt_at": time.Now().UTC()})
}

func (h *Handler) snapshotStatus(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "snapshotID")
	projectID := chi.URLParam(r, "id")
	var state, ref, sha string
	var at time.Time
	var size *int64
	var code *string
	var retryAvailable bool
	err := h.pool.QueryRow(r.Context(), `SELECT s.state,s.requested_ref,s.commit_sha,s.receipt_at,s.archive_bytes,s.failure_code,COALESCE(j.state='dead',false) FROM repository_snapshots s LEFT JOIN jobs j ON j.id=s.job_id WHERE s.id=$1 AND s.project_id=$2 AND s.college_id=$3 AND EXISTS(SELECT 1 FROM project_members WHERE project_id=$2 AND user_id=$4 AND college_id=$3)`, id, projectID, s.CollegeID, s.UserID).Scan(&state, &ref, &sha, &at, &size, &code, &retryAvailable)
	if err != nil {
		h.fail(w, r, 404, "snapshot_not_found", "snapshot was not found", err)
		return
	}
	_ = response.OK(w, map[string]any{"id": id, "state": state, "ref": ref, "commit_sha": sha, "receipt_at": at, "archive_bytes": size, "failure_code": code, "retry_available": retryAvailable})
}

func (h *Handler) retrySnapshot(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	snapshotID, projectID := chi.URLParam(r, "snapshotID"), chi.URLParam(r, "id")
	if !uuidOK(snapshotID) || !uuidOK(projectID) {
		h.fail(w, r, 404, "snapshot_not_found", "snapshot was not found", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	var jobID string
	err = tx.QueryRow(r.Context(), `SELECT s.job_id::text FROM repository_snapshots s JOIN repository_bindings b ON b.id=s.binding_id AND b.repository_id=s.repository_id AND b.college_id=s.college_id JOIN github_installations i ON i.id=b.installation_id AND i.college_id=b.college_id WHERE s.id=$1 AND s.project_id=$2 AND s.college_id=$3 AND s.state='failed' AND b.is_current AND b.access_state='active' AND i.status='active' AND EXISTS(SELECT 1 FROM project_members WHERE project_id=$2 AND college_id=$3 AND user_id=$4) FOR UPDATE OF s`, snapshotID, projectID, s.CollegeID, s.UserID).Scan(&jobID)
	if err != nil {
		h.fail(w, r, 404, "snapshot_retry_unavailable", "failed snapshot is not eligible for retry", err)
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE jobs SET state='ready',attempts=0,generation=generation+1,available_at=clock_timestamp(),worker=NULL,lease_until=NULL,last_error_code=NULL,updated_at=clock_timestamp() WHERE id=$1 AND state='dead' AND (deadline_at IS NULL OR deadline_at>clock_timestamp())`, jobID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if tag.RowsAffected() != 1 {
		h.fail(w, r, 409, "snapshot_retry_unavailable", "snapshot retry is not available yet", nil)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE repository_snapshots SET state='pending',failure_code=NULL,capture_started_at=NULL WHERE id=$1`, snapshotID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"id": snapshotID, "state": "pending"})
}

func (h *Handler) downloadSnapshot(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	if h.store == nil {
		h.fail(w, r, 503, "snapshot_storage_unavailable", "snapshot storage is not configured", nil)
		return
	}
	var key, sha string
	var projectID = chi.URLParam(r, "id")
	var id = chi.URLParam(r, "snapshotID")
	err := h.pool.QueryRow(r.Context(), `SELECT object_key,commit_sha FROM repository_snapshots WHERE id=$1 AND project_id=$2 AND college_id=$3 AND state='verified' AND EXISTS(SELECT 1 FROM projects p WHERE p.id=$2 AND p.college_id=$3 AND p.lifecycle<>'archived') AND EXISTS(SELECT 1 FROM project_members WHERE project_id=$2 AND college_id=$3 AND user_id=$4)`, id, projectID, s.CollegeID, s.UserID).Scan(&key, &sha)
	if err != nil {
		h.fail(w, r, 404, "snapshot_not_found", "verified snapshot is no longer available", err)
		return
	}
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
	err := h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$3)`, projectID, s.CollegeID, s.UserID).Scan(&yes)
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
	if github.IsDenied(err) {
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
