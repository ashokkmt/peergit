package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
	"peergit/internal/github"
	"peergit/internal/identity"
	"peergit/internal/platform/audit"
	"peergit/internal/platform/errormanager"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
	"peergit/internal/platform/jobs"
)

const githubCandidateTTL = 15 * time.Minute
const maxGitHubCandidates = 1000

type repositoryAuthorizationState struct {
	CollegeID, UserID, Purpose, InstallationID, ProjectID string
	ExternalRepositoryID                                  *int64
	Verifier                                              string
}

func (h *Handler) startAccountInstallation(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	if h.app.Slug == "" || h.app.ID == "" || len(h.app.PrivateKey) == 0 {
		h.fail(w, r, 503, "github_app_unavailable", "GitHub repository access is not configured", nil)
		return
	}
	state, hash, err := randomState()
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_, err = h.pool.Exec(r.Context(), `DELETE FROM github_install_setup_states WHERE expires_at<=clock_timestamp()`)
	if err == nil {
		_, err = h.pool.Exec(r.Context(), `INSERT INTO github_install_setup_states(state_hash,college_id,user_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '10 minutes')`, hash, s.CollegeID, s.UserID)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	installURL := "https://github.com/apps/" + url.PathEscape(h.app.Slug) + "/installations/new?state=" + url.QueryEscape(state)
	_ = response.OK(w, map[string]string{"installation_url": installURL})
}

func (h *Handler) finishAccountInstallation(w http.ResponseWriter, r *http.Request) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok || s.CollegeID == "" || !s.TermsAccepted || !s.PrivacyAccepted {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if r.URL.Query().Get("setup_action") == "request" {
		http.Redirect(w, r, h.app.Origin+"/account/github?github_error=installation_cancelled", http.StatusSeeOther)
		return
	}
	state, externalID := r.URL.Query().Get("state"), r.URL.Query().Get("installation_id")
	if len(state) < 40 || len(state) > 100 || !numericID(externalID) {
		h.fail(w, r, 400, "installation_callback_invalid", "GitHub installation response is invalid or expired", nil)
		return
	}
	_, stateHash := hashState(state)
	var boundUser, boundCollege string
	if err := h.pool.QueryRow(r.Context(), `SELECT user_id::text,college_id::text FROM github_install_setup_states WHERE state_hash=$1 AND user_id=$2 AND college_id=$3 AND expires_at>clock_timestamp()`, stateHash, s.UserID, s.CollegeID).Scan(&boundUser, &boundCollege); err != nil {
		h.fail(w, r, 400, "installation_state_invalid", "GitHub installation request expired or was already used", err)
		return
	}
	appJWT, err := github.JWT(h.app.ID, h.app.PrivateKey, time.Now())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	installation, err := h.api.Installation(r.Context(), externalID, appJWT)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	if installation.ID == 0 {
		installation.ID, _ = strconv.ParseInt(externalID, 10, 64)
	}
	if installation.ID <= 0 || installation.Account.ID <= 0 || installation.Account.Login == "" || (installation.TargetType != "User" && installation.TargetType != "Organization") || installation.Permissions["contents"] != "read" {
		h.fail(w, r, 403, "installation_invalid", "GitHub installation is not valid for repository evidence", nil)
		return
	}
	status := "active"
	if installation.SuspendedAt != nil {
		status = "suspended"
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var consumed string
	err = tx.QueryRow(r.Context(), `DELETE FROM github_install_setup_states WHERE state_hash=$1 AND user_id=$2 AND college_id=$3 AND expires_at>clock_timestamp() RETURNING user_id::text`, stateHash, s.UserID, s.CollegeID).Scan(&consumed)
	if err != nil {
		h.fail(w, r, 400, "installation_state_invalid", "GitHub installation request expired or was already used", err)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO github_installations(college_id,external_installation_id,account_external_id,account_login,target_type,repository_selection,permissions,status,added_by,installed_at,removed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,clock_timestamp(),NULL)
		ON CONFLICT(external_installation_id) DO UPDATE SET account_external_id=EXCLUDED.account_external_id,account_login=EXCLUDED.account_login,target_type=EXCLUDED.target_type,repository_selection=EXCLUDED.repository_selection,permissions=EXCLUDED.permissions,status=EXCLUDED.status,added_by=EXCLUDED.added_by,checked_at=clock_timestamp(),removed_at=NULL
		WHERE github_installations.college_id=EXCLUDED.college_id`, s.CollegeID, installation.ID, installation.Account.ID, installation.Account.Login, installation.TargetType, installation.RepositorySelection, jsonValue(installation.Permissions), status, s.UserID)
	if err != nil {
		h.fail(w, r, 409, "installation_tenant_conflict", "GitHub installation is already linked to another campus", err)
		return
	}
	var localInstallation string
	if err = tx.QueryRow(r.Context(), `SELECT id::text FROM github_installations WHERE external_installation_id=$1 AND college_id=$2`, installation.ID, s.CollegeID).Scan(&localInstallation); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "github.installation.candidate", ResourceType: "github_installation", ResourceID: localInstallation}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	authorizeURL, err := h.beginRepositoryAuthorization(r.Context(), s, "connect", localInstallation, nil, "")
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	http.Redirect(w, r, authorizeURL, http.StatusSeeOther)
}

func (h *Handler) oauthConfig() oauth2.Config {
	authorizeURL, tokenURL := h.app.OAuthAuthorizeURL, h.app.OAuthTokenURL
	if authorizeURL == "" {
		authorizeURL = "https://github.com/login/oauth/authorize"
	}
	if tokenURL == "" {
		tokenURL = "https://github.com/login/oauth/access_token"
	}
	return oauth2.Config{ClientID: h.app.OAuthClientID, ClientSecret: h.app.OAuthClientSecret, RedirectURL: h.app.OAuthRedirectURL, Endpoint: oauth2.Endpoint{AuthURL: authorizeURL, TokenURL: tokenURL}}
}

func (h *Handler) beginRepositoryAuthorization(ctx context.Context, s identity.Session, purpose, installationID string, repositoryID *int64, projectID string) (string, error) {
	if h.app.OAuthClientID == "" || h.app.OAuthClientSecret == "" || h.app.OAuthRedirectURL == "" {
		return "", errors.New("repository GitHub App authorization is not configured")
	}
	state, hash, err := randomState()
	if err != nil {
		return "", err
	}
	verifier, err := randomVerifier()
	if err != nil {
		return "", err
	}
	var projectValue any
	if projectID != "" {
		projectValue = projectID
	}
	_, err = h.pool.Exec(ctx, `INSERT INTO github_repository_authorization_states(state_hash,college_id,user_id,purpose,installation_id,project_id,external_repository_id,pkce_verifier,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,clock_timestamp()+interval '10 minutes')`, hash, s.CollegeID, s.UserID, purpose, installationID, projectValue, repositoryID, verifier)
	if err != nil {
		return "", err
	}
	config := h.oauthConfig()
	challenge := oauth2.S256ChallengeFromVerifier(verifier)
	return config.AuthCodeURL(state, oauth2.SetAuthURLParam("code_challenge", challenge), oauth2.SetAuthURLParam("code_challenge_method", "S256")), nil
}

func (h *Handler) finishRepositoryAuthorization(w http.ResponseWriter, r *http.Request) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok || s.CollegeID == "" || !s.TermsAccepted || !s.PrivacyAccepted {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, h.app.Origin+"/account/github?github_error=authorization_cancelled", http.StatusSeeOther)
		return
	}
	state := r.URL.Query().Get("state")
	if len(state) < 40 || len(state) > 100 {
		h.fail(w, r, 400, "authorization_state_invalid", "GitHub authorization expired; try again", nil)
		return
	}
	_, hash := hashState(state)
	var auth repositoryAuthorizationState
	err := h.pool.QueryRow(r.Context(), `DELETE FROM github_repository_authorization_states WHERE state_hash=$1 AND user_id=$2 AND college_id=$3 AND expires_at>clock_timestamp()
		RETURNING college_id::text,user_id::text,purpose,installation_id::text,COALESCE(project_id::text,''),external_repository_id,pkce_verifier`, hash, s.UserID, s.CollegeID).Scan(&auth.CollegeID, &auth.UserID, &auth.Purpose, &auth.InstallationID, &auth.ProjectID, &auth.ExternalRepositoryID, &auth.Verifier)
	if err != nil {
		h.fail(w, r, 400, "authorization_state_invalid", "GitHub authorization expired or was already used", err)
		return
	}
	config := h.oauthConfig()
	providerToken, err := config.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(auth.Verifier))
	if err != nil {
		h.fail(w, r, 502, "github_authorization_failed", "GitHub authorization could not be completed", err)
		return
	}
	userToken := providerToken.AccessToken
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	user, err := h.api.User(ctx, userToken)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	var subject string
	if err = h.pool.QueryRow(ctx, `SELECT subject FROM user_identities WHERE user_id=$1 AND provider='github'`, s.UserID).Scan(&subject); err != nil || subject != strconv.FormatInt(user.ID, 10) {
		h.fail(w, r, 403, "github_identity_mismatch", "authorize GitHub with the same account used to sign in to PeerGit", err)
		return
	}
	installations, err := h.api.UserInstallations(ctx, userToken)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	var externalInstall int64
	if err = h.pool.QueryRow(ctx, `SELECT external_installation_id FROM github_installations WHERE id=$1 AND college_id=$2 AND status='active'`, auth.InstallationID, s.CollegeID).Scan(&externalInstall); err != nil {
		h.fail(w, r, 403, "installation_unavailable", "the GitHub App installation is no longer available", err)
		return
	}
	var authorized bool
	for _, item := range installations {
		if item.ID == externalInstall {
			authorized = true
			break
		}
	}
	if !authorized {
		h.fail(w, r, 403, "installation_not_authorized", "this GitHub account cannot access the selected App installation", nil)
		return
	}
	if auth.Purpose == "connect" {
		if err = h.saveRepositoryCandidates(ctx, s, auth.InstallationID, externalInstall, user.ID, userToken); err != nil {
			h.providerFailure(w, r, err)
			return
		}
		http.Redirect(w, r, h.app.Origin+"/account/github?connected=1", http.StatusSeeOther)
		return
	}
	if auth.Purpose == "link" && auth.ExternalRepositoryID != nil && auth.ProjectID != "" {
		if err = h.completeLegacyGitHubLink(w, r, ctx, s, auth.InstallationID, externalInstall, auth.ProjectID, *auth.ExternalRepositoryID, userToken); err != nil {
			var appErr *errormanager.Error
			if errors.As(err, &appErr) {
				h.err.Handle(w, r, err)
			} else {
				h.providerFailure(w, r, err)
			}
		}
		return
	}
	if auth.Purpose != "import" || auth.ExternalRepositoryID == nil {
		h.fail(w, r, 400, "import_state_invalid", "repository import authorization is invalid", nil)
		return
	}
	if err = h.completeGitHubImport(w, r, ctx, s, auth.InstallationID, externalInstall, *auth.ExternalRepositoryID, user.ID, userToken); err != nil {
		var appErr *errormanager.Error
		if errors.As(err, &appErr) {
			h.err.Handle(w, r, err)
		} else {
			h.providerFailure(w, r, err)
		}
		return
	}
}

func (h *Handler) saveRepositoryCandidates(ctx context.Context, s identity.Session, localInstallation string, externalInstallation, userID int64, userToken string) error {
	repos, err := h.api.UserInstallationRepositories(ctx, externalInstallation, userToken)
	if err != nil {
		return err
	}
	if len(repos) > maxGitHubCandidates {
		return errors.New("installation has too many repositories for the bounded import picker")
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO github_installation_user_grants(college_id,user_id,installation_id,external_user_id,confirmed_at,revoked_at)
		VALUES($1,$2,$3,$4,clock_timestamp(),NULL) ON CONFLICT(college_id,user_id,installation_id) DO UPDATE SET external_user_id=EXCLUDED.external_user_id,confirmed_at=EXCLUDED.confirmed_at,revoked_at=NULL`, s.CollegeID, s.UserID, localInstallation, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM github_repository_candidates WHERE college_id=$1 AND user_id=$2`, s.CollegeID, s.UserID); err != nil {
		return err
	}
	for _, repo := range repos {
		if repo.ID <= 0 || !strings.Contains(repo.FullName, "/") {
			continue
		}
		admin, known := repo.Permissions["admin"]
		if !known {
			repo, err = h.api.Repository(ctx, repo.FullName, userToken)
			if err != nil {
				if github.IsDenied(err) {
					continue
				}
				return err
			}
			admin = repo.Permissions["admin"]
		}
		if !admin {
			continue
		}
		parts := strings.SplitN(repo.FullName, "/", 2)
		if len(parts) != 2 || !repoSegment.MatchString(parts[0]) || !repoSegment.MatchString(parts[1]) {
			continue
		}
		visibility := repo.Visibility
		if visibility == "" {
			if repo.Private {
				visibility = "private"
			} else {
				visibility = "public"
			}
		}
		if visibility != "public" && visibility != "private" && visibility != "internal" {
			continue
		}
		if _, err = tx.Exec(ctx, `INSERT INTO github_repository_candidates(college_id,user_id,installation_id,external_repository_id,owner_login,repository_name,description,default_branch,visibility,topics,expires_at,checked_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,clock_timestamp()+$11::interval,clock_timestamp()) ON CONFLICT(college_id,user_id,external_repository_id) DO UPDATE SET installation_id=EXCLUDED.installation_id,owner_login=EXCLUDED.owner_login,repository_name=EXCLUDED.repository_name,description=EXCLUDED.description,default_branch=EXCLUDED.default_branch,visibility=EXCLUDED.visibility,topics=EXCLUDED.topics,expires_at=EXCLUDED.expires_at,checked_at=EXCLUDED.checked_at`, s.CollegeID, s.UserID, localInstallation, repo.ID, parts[0], parts[1], truncate(repo.Description, 1000), repo.DefaultBranch, visibility, repo.Topics, githubCandidateTTL.String()); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (h *Handler) myGitHubRepositories(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT c.external_repository_id,c.installation_id::text,c.owner_login,c.repository_name,c.description,c.default_branch,c.visibility,c.topics,c.expires_at
		FROM github_repository_candidates c JOIN github_installation_user_grants g ON g.college_id=c.college_id AND g.user_id=c.user_id AND g.installation_id=c.installation_id AND g.revoked_at IS NULL
		WHERE c.college_id=$1 AND c.user_id=$2 AND c.expires_at>clock_timestamp() ORDER BY lower(c.owner_login),lower(c.repository_name) LIMIT 1000`, s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var installation, owner, name, description, branch, vis string
		var topics []string
		var expires time.Time
		if err = rows.Scan(&id, &installation, &owner, &name, &description, &branch, &vis, &topics, &expires); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "installation_id": installation, "owner": owner, "name": name, "full_name": owner + "/" + name, "description": description, "default_branch": branch, "visibility": vis, "topics": topics, "expires_at": expires})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) myGitHubInstallations(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT i.id::text,i.external_installation_id::text,i.account_login,i.target_type,i.status,g.confirmed_at FROM github_installation_user_grants g JOIN github_installations i ON i.id=g.installation_id AND i.college_id=g.college_id WHERE g.college_id=$1 AND g.user_id=$2 AND g.revoked_at IS NULL ORDER BY i.account_login`, s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID             string    `json:"id"`
		InstallationID string    `json:"installation_id"`
		Account        string    `json:"account"`
		Target         string    `json:"target_type"`
		Status         string    `json:"status"`
		ConfirmedAt    time.Time `json:"confirmed_at"`
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.ID, &v.InstallationID, &v.Account, &v.Target, &v.Status, &v.ConfirmedAt); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

type importRepositoryRequest struct {
	RepositoryID int64 `json:"repository_id"`
}

func (h *Handler) startGitHubImport(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[importRepositoryRequest](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.RepositoryID <= 0 {
		h.fail(w, r, 400, "repository_invalid", "select an available GitHub repository", nil)
		return
	}
	var install string
	err = h.pool.QueryRow(r.Context(), `SELECT c.installation_id::text FROM github_repository_candidates c JOIN github_installation_user_grants g ON g.college_id=c.college_id AND g.user_id=c.user_id AND g.installation_id=c.installation_id AND g.revoked_at IS NULL
		WHERE c.college_id=$1 AND c.user_id=$2 AND c.external_repository_id=$3 AND c.expires_at>clock_timestamp()`, s.CollegeID, s.UserID, in.RepositoryID).Scan(&install)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 409, "repository_candidate_expired", "reconnect GitHub to refresh available repositories", err)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	authorizeURL, err := h.beginRepositoryAuthorization(r.Context(), s, "import", install, &in.RepositoryID, "")
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]string{"authorization_url": authorizeURL})
}

type linkRepositoryRequest struct {
	RepositoryID int64 `json:"repository_id"`
}

func (h *Handler) startLegacyGitHubLink(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	projectID := chi.URLParam(r, "id")
	if !uuidOK(projectID) {
		h.fail(w, r, 404, "project_not_found", "project was not found", nil)
		return
	}
	in, err := request.Decode[linkRepositoryRequest](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.RepositoryID <= 0 {
		h.fail(w, r, 400, "repository_invalid", "select an available GitHub repository", nil)
		return
	}
	var installation string
	err = h.pool.QueryRow(r.Context(), `SELECT c.installation_id::text FROM github_repository_candidates c
		JOIN github_installation_user_grants g ON g.college_id=c.college_id AND g.user_id=c.user_id AND g.installation_id=c.installation_id AND g.revoked_at IS NULL
		JOIN project_members pm ON pm.college_id=c.college_id AND pm.project_id=$4 AND pm.user_id=c.user_id AND pm.role IN ('owner','maintainer')
		JOIN projects p ON p.id=pm.project_id AND p.college_id=pm.college_id
		WHERE c.college_id=$1 AND c.user_id=$2 AND c.external_repository_id=$3 AND c.expires_at>clock_timestamp()
		AND NOT EXISTS (SELECT 1 FROM repositories rp JOIN repository_bindings b ON b.repository_id=rp.id AND b.college_id=rp.college_id AND b.is_current WHERE rp.project_id=p.id AND rp.college_id=p.college_id)`, s.CollegeID, s.UserID, in.RepositoryID, projectID).Scan(&installation)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "legacy_project_not_linkable", "project or GitHub repository is unavailable", err)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	authorizeURL, err := h.beginRepositoryAuthorization(r.Context(), s, "link", installation, &in.RepositoryID, projectID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]string{"authorization_url": authorizeURL})
}

func (h *Handler) completeLegacyGitHubLink(w http.ResponseWriter, r *http.Request, ctx context.Context, s identity.Session, localInstallation string, externalInstallation int64, projectID string, repositoryID int64, userToken string) error {
	repos, err := h.api.UserInstallationRepositories(ctx, externalInstallation, userToken)
	if err != nil {
		return err
	}
	var selected *github.Repository
	for i := range repos {
		if repos[i].ID == repositoryID {
			selected = &repos[i]
			break
		}
	}
	if selected == nil {
		return errormanager.New(403, "repository_access_revoked", "GitHub no longer grants access to this repository", nil)
	}
	if _, known := selected.Permissions["admin"]; !known {
		details, e := h.api.Repository(ctx, selected.FullName, userToken)
		if e != nil {
			return e
		}
		selected = &details
	}
	if !selected.Permissions["admin"] {
		return errormanager.New(403, "repository_admin_required", "GitHub confirmed that you do not administer this repository", nil)
	}
	appJWT, err := github.JWT(h.app.ID, h.app.PrivateKey, time.Now())
	if err != nil {
		return err
	}
	var extID string
	if err = h.pool.QueryRow(ctx, `SELECT external_installation_id::text FROM github_installations WHERE id=$1 AND college_id=$2 AND status='active'`, localInstallation, s.CollegeID).Scan(&extID); err != nil {
		return err
	}
	installationToken, err := h.api.Token(ctx, extID, appJWT)
	if err != nil {
		return err
	}
	appRepo, err := h.api.Repository(ctx, selected.FullName, installationToken)
	if err != nil {
		return err
	}
	if appRepo.ID != repositoryID {
		return errormanager.New(403, "repository_access_revoked", "GitHub App installation cannot access this repository", nil)
	}
	parts := strings.SplitN(selected.FullName, "/", 2)
	if len(parts) != 2 || !repoSegment.MatchString(parts[0]) || !repoSegment.MatchString(parts[1]) {
		return errors.New("GitHub returned invalid repository identity")
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var localRepository string
	if err = tx.QueryRow(ctx, `SELECT rp.id::text FROM repositories rp JOIN project_members pm ON pm.college_id=rp.college_id AND pm.project_id=rp.project_id AND pm.user_id=$3 AND pm.role IN ('owner','maintainer') WHERE rp.project_id=$1 AND rp.college_id=$2 FOR UPDATE`, projectID, s.CollegeID, s.UserID).Scan(&localRepository); errors.Is(err, pgx.ErrNoRows) {
		if err = tx.QueryRow(ctx, `SELECT id::text FROM projects WHERE id=$1 AND college_id=$2 AND EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$3 AND role IN ('owner','maintainer')) FOR UPDATE`, projectID, s.CollegeID, s.UserID).Scan(new(string)); err != nil {
			return errormanager.New(404, "legacy_project_not_linkable", "project or GitHub repository is unavailable", err)
		}
		if err = tx.QueryRow(ctx, `INSERT INTO repositories(college_id,project_id,created_by) VALUES($1,$2,$3) RETURNING id::text`, s.CollegeID, projectID, s.UserID).Scan(&localRepository); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	visibility := selected.Visibility
	if visibility == "" {
		if selected.Private {
			visibility = "private"
		} else {
			visibility = "public"
		}
	}
	var bindingID string
	if err = tx.QueryRow(ctx, `INSERT INTO repository_bindings(college_id,repository_id,installation_id,external_repository_id,owner_login,repository_name,default_branch,visibility,linked_by,html_url,description,topics) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id::text`, s.CollegeID, localRepository, localInstallation, repositoryID, parts[0], parts[1], selected.DefaultBranch, visibility, s.UserID, safeGitHubURL(selected.HTMLURL, selected.FullName), truncate(selected.Description, 1000), selected.Topics).Scan(&bindingID); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"binding_id": bindingID})
	if _, err = jobs.EnqueueTx(ctx, tx, s.CollegeID, s.UserID, "github_reconcile", "initial:"+bindingID, payload, nil); err != nil {
		return err
	}
	if _, err = jobs.EnqueueTx(ctx, tx, s.CollegeID, s.UserID, "github_overview", "overview:"+bindingID, payload, nil); err != nil {
		return err
	}
	if err = audit.Append(ctx, tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "github.repository.linked", ResourceType: "repository_binding", ResourceID: bindingID}); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	http.Redirect(w, r, h.app.Origin+"/projects/"+url.PathEscape(projectID), http.StatusSeeOther)
	return nil
}

func (h *Handler) completeGitHubImport(w http.ResponseWriter, r *http.Request, ctx context.Context, s identity.Session, localInstallation string, externalInstallation, repositoryID, userID int64, userToken string) error {
	repos, err := h.api.UserInstallationRepositories(ctx, externalInstallation, userToken)
	if err != nil {
		return err
	}
	var selected *github.Repository
	for i := range repos {
		if repos[i].ID == repositoryID {
			selected = &repos[i]
			break
		}
	}
	if selected == nil {
		return errormanager.New(403, "repository_access_revoked", "GitHub no longer grants access to this repository", nil)
	}
	if _, ok := selected.Permissions["admin"]; !ok {
		details, e := h.api.Repository(ctx, selected.FullName, userToken)
		if e != nil {
			return e
		}
		selected = &details
	}
	if !selected.Permissions["admin"] {
		return errormanager.New(403, "repository_admin_required", "GitHub confirmed that you do not administer this repository", nil)
	}
	appJWT, err := github.JWT(h.app.ID, h.app.PrivateKey, time.Now())
	if err != nil {
		return err
	}
	var extID string
	if err = h.pool.QueryRow(ctx, `SELECT external_installation_id::text FROM github_installations WHERE id=$1 AND college_id=$2 AND status='active'`, localInstallation, s.CollegeID).Scan(&extID); err != nil {
		return err
	}
	installationToken, err := h.api.Token(ctx, extID, appJWT)
	if err != nil {
		return err
	}
	appRepo, err := h.api.Repository(ctx, selected.FullName, installationToken)
	if err != nil {
		return err
	}
	if appRepo.ID != repositoryID {
		return errormanager.New(403, "repository_access_revoked", "GitHub App installation cannot access this repository", nil)
	}
	parts := strings.SplitN(selected.FullName, "/", 2)
	if len(parts) != 2 || !repoSegment.MatchString(parts[0]) || !repoSegment.MatchString(parts[1]) {
		return errors.New("GitHub returned invalid repository identity")
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var existingProject, viewer string
	err = tx.QueryRow(ctx, `SELECT p.id::text,COALESCE(pm.role,'') FROM repository_bindings b JOIN repositories rp ON rp.id=b.repository_id AND rp.college_id=b.college_id JOIN projects p ON p.id=rp.project_id AND p.college_id=rp.college_id LEFT JOIN project_members pm ON pm.project_id=p.id AND pm.college_id=p.college_id AND pm.user_id=$3 WHERE b.college_id=$1 AND b.provider='github' AND b.external_repository_id=$2 ORDER BY b.created_at DESC LIMIT 1`, s.CollegeID, repositoryID, s.UserID).Scan(&existingProject, &viewer)
	if err == nil {
		if viewer == "" {
			return errormanager.New(409, "repository_already_imported", "this repository is already connected to a campus project", nil)
		}
		_ = tx.Rollback(ctx)
		http.Redirect(w, r, h.app.Origin+"/projects/"+url.PathEscape(existingProject), http.StatusSeeOther)
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	title := strings.TrimSpace(parts[1])
	if title == "" {
		title = parts[1]
	}
	if len(title) > 120 {
		title = title[:120]
	}
	if len(title) < 3 {
		title = fmt.Sprintf("GitHub repository %d", repositoryID)
	}
	summary := importedProjectSummary(selected.Description, selected.FullName)
	slug := fmt.Sprintf("github-%d", repositoryID)
	var projectID, localRepository, bindingID string
	err = tx.QueryRow(ctx, `INSERT INTO projects(college_id,slug,title,summary,description,project_type,visibility,lifecycle,created_by) VALUES($1,$2,$3,$4,$5,'open_source','private','draft',$6) RETURNING id::text`, s.CollegeID, slug, title, summary, truncate(selected.Description, 12000), s.UserID).Scan(&projectID)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO project_members(college_id,project_id,user_id,role) VALUES($1,$2,$3,'owner')`, s.CollegeID, projectID, s.UserID); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO repositories(college_id,project_id,created_by) VALUES($1,$2,$3) RETURNING id::text`, s.CollegeID, projectID, s.UserID).Scan(&localRepository); err != nil {
		return err
	}
	visibility := selected.Visibility
	if visibility == "" {
		if selected.Private {
			visibility = "private"
		} else {
			visibility = "public"
		}
	}
	if err = tx.QueryRow(ctx, `INSERT INTO repository_bindings(college_id,repository_id,installation_id,external_repository_id,owner_login,repository_name,default_branch,visibility,linked_by,html_url,description,topics)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id::text`, s.CollegeID, localRepository, localInstallation, repositoryID, parts[0], parts[1], selected.DefaultBranch, visibility, s.UserID, safeGitHubURL(selected.HTMLURL, selected.FullName), truncate(selected.Description, 1000), selected.Topics).Scan(&bindingID); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"binding_id": bindingID})
	if _, err = jobs.EnqueueTx(ctx, tx, s.CollegeID, s.UserID, "github_reconcile", "initial:"+bindingID, payload, nil); err != nil {
		return err
	}
	if _, err = jobs.EnqueueTx(ctx, tx, s.CollegeID, s.UserID, "github_overview", "overview:"+bindingID, payload, nil); err != nil {
		return err
	}
	if err = audit.Append(ctx, tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "github.repository.imported", ResourceType: "repository_binding", ResourceID: bindingID}); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	// The redirect is kept as a same-origin path; the handler owns no browser state after this point.
	http.Redirect(w, r, h.app.Origin+"/projects/"+url.PathEscape(projectID), http.StatusSeeOther)
	return nil
}

func (h *Handler) myProjects(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT p.id::text,p.slug,p.title,p.summary,p.visibility,p.lifecycle,p.version,p.updated_at,b.owner_login,b.repository_name,b.external_repository_id,b.default_branch,b.access_state,b.sync_health
		FROM project_members pm JOIN projects p ON p.id=pm.project_id AND p.college_id=pm.college_id LEFT JOIN repositories rp ON rp.project_id=p.id AND rp.college_id=p.college_id LEFT JOIN repository_bindings b ON b.repository_id=rp.id AND b.college_id=rp.college_id AND b.is_current
		WHERE pm.user_id=$1 AND pm.college_id=$2 ORDER BY p.updated_at DESC,p.id DESC LIMIT 100`, s.UserID, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, slug, title, summary, vis, lifecycle string
		var version int
		var updated time.Time
		var owner, name, branch, access, health *string
		var externalID *int64
		if err = rows.Scan(&id, &slug, &title, &summary, &vis, &lifecycle, &version, &updated, &owner, &name, &externalID, &branch, &access, &health); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if externalID == nil {
			vis = "private"
			if lifecycle == "active" {
				lifecycle = "draft"
			}
		}
		items = append(items, map[string]any{"id": id, "slug": slug, "title": title, "summary": summary, "visibility": vis, "lifecycle": lifecycle, "version": version, "updated_at": updated, "owner": owner, "repository": name, "repository_id": externalID, "default_branch": branch, "access_state": access, "sync_health": health})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func randomState() (string, []byte, error) {
	state, err := randomVerifier()
	if err != nil {
		return "", nil, err
	}
	_, hash := hashState(state)
	return state, hash, nil
}
func randomVerifier() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
func hashState(state string) (string, []byte) {
	hash := sha256.Sum256([]byte(state))
	return state, hash[:]
}
func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return strings.TrimSpace(value[:max])
}

func importedProjectSummary(description, fullName string) string {
	summary := truncate(strings.TrimSpace(description), 500)
	if summary == "" {
		return "Imported from GitHub: " + fullName
	}
	return summary
}

func safeGitHubURL(value, fullName string) string {
	parsed, err := url.Parse(value)
	if err == nil && parsed.Scheme == "https" && parsed.Hostname() == "github.com" && parsed.User == nil {
		return parsed.String()
	}
	return "https://github.com/" + fullName
}
