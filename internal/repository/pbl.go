package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"peergit/internal/identity"
	"peergit/internal/platform/audit"
	"peergit/internal/platform/errormanager"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
	"peergit/internal/platform/jobs"
)

func (h *Handler) registerPBL(private chi.Router) {
	private.Get("/academic/pbl/teams/{team}/repository", h.pblRepository)
	private.Get("/academic/pbl/teams/{team}/evidence", h.pblEvidence)
	private.Get("/academic/pbl/teams/{team}/source", h.pblBrowse)
	private.Get("/academic/pbl/teams/{team}/work", h.pblWork)
	private.Get("/academic/pbl/teams/{team}/checkpoints/{stage}/download", h.downloadPBLCheckpoint)
	write := private.With(h.auth.RequireCSRF)
	write.Post("/academic/pbl/teams/{team}/repository", h.requestPBLRepository)
	write.Post("/academic/pbl/teams/{team}/git-credential", h.issuePBLCredential)
	write.Delete("/academic/pbl/teams/{team}/git-credential", h.revokePBLCredential)
	write.Put("/academic/pbl/teams/{team}/campus-card", h.pblCampusCard)
	write.Post("/academic/pbl/teams/{team}/work", h.pblWork)
	write.Post("/academic/pbl/teams/{team}/checkpoints/{stage}", h.submitPBLCheckpoint)
	write.Put("/academic/pbl/teams/{team}/checkpoints/{stage}", h.reviewPBLCheckpoint)
	write.Post("/academic/pbl/teams/{team}/checkpoints/{stage}/retry", h.retryPBLCheckpoint)
}

func (h *Handler) pblRepository(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	team := chi.URLParam(r, "team")
	if !uuidOK(team) {
		h.fail(w, r, 404, "pbl_team_unavailable", "this team is unavailable", nil)
		return
	}
	var p, state, mode string
	var isMember, leader, campusVisible bool
	err := h.pool.QueryRow(r.Context(), `SELECT COALESCE(t.project_id::text,''),COALESCE(CASE
			WHEN n.state='queued' AND j.state='running' THEN 'provisioning'
			WHEN n.state='queued' AND j.state='dead' THEN 'failed'
			ELSE n.state END,'unconfigured'),CASE WHEN n.source_repository_id IS NOT NULL THEN 'github' ELSE 'empty' END,
			m.user_id IS NOT NULL,t.leader_id=$3,COALESCE(p.visibility='campus',false) FROM pbl_teams t
		LEFT JOIN pbl_provisions n ON n.team_id=t.id LEFT JOIN jobs j ON n.job_id=j.id
		LEFT JOIN projects p ON p.id=t.project_id AND p.college_id=t.college_id
		LEFT JOIN pbl_active_team_members m ON m.team_id=t.id AND m.user_id=$3
		WHERE t.id=$1 AND t.college_id=$2 AND (m.user_id IS NOT NULL
			OR EXISTS(SELECT 1 FROM pbl_active_faculty f WHERE f.team_id=t.id AND f.user_id=$3) OR $4)`,
		team, s.CollegeID, s.UserID, s.CampusAdmin && s.MFAEnabled && s.MFAFresh).Scan(&p, &state, &mode, &isMember, &leader, &campusVisible)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "pbl_team_unavailable", "this team is unavailable", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	out := map[string]any{"project_id": p, "state": state, "mode": mode, "can_provision": h.forge.URL != "" && isMember && leader,
		"can_git": state == "ready" && isMember && h.forge.URL != "", "campus_visible": campusVisible, "storage_limit_kib": pblRepositoryKiB}
	if out["can_git"] == true {
		out["clone_url"] = h.app.Origin + "/api/v1/pbl/git/" + team + ".git"
	}
	w.Header().Set("Cache-Control", "no-store")
	_ = response.OK(w, out)
}

func (h *Handler) pblCampusCard(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	team := chi.URLParam(r, "team")
	if !uuidOK(team) {
		h.fail(w, r, 404, "pbl_team_unavailable", "this team is unavailable", nil)
		return
	}
	in, err := request.Decode[struct {
		Visible *bool `json:"visible"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Visible == nil {
		h.fail(w, r, 400, "visibility_required", "choose whether to share the card on campus", nil)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var project string
	err = tx.QueryRow(ctx, `UPDATE projects p SET visibility=CASE WHEN $4 THEN 'campus' ELSE 'private' END,
		lifecycle='active',version=version+1,updated_at=clock_timestamp()
		FROM pbl_teams t JOIN pbl_provisions n ON n.team_id=t.id AND n.state='ready'
		WHERE t.id=$1 AND t.college_id=$2 AND t.leader_id=$3 AND p.id=t.project_id AND p.college_id=t.college_id
		AND EXISTS(SELECT 1 FROM pbl_active_team_members m WHERE m.team_id=t.id AND m.user_id=$3)
		RETURNING p.id::text`, team, s.CollegeID, s.UserID, *in.Visible).Scan(&project)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "pbl_team_unavailable", "only the confirmed leader can share a ready project card", nil)
		return
	}
	if err == nil {
		err = audit.Append(ctx, tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "pbl.card_visibility_changed", ResourceType: "project", ResourceID: project})
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) requestPBLRepository(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	if h.forge.URL == "" {
		h.fail(w, r, 503, "pbl_git_disabled", "PBL Git is not available in this environment", nil)
		return
	}
	in, err := request.Decode[importRepositoryRequest](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	team := chi.URLParam(r, "team")
	if !uuidOK(team) || (in.RepositoryID != 0 || in.InstallationID != "") && (in.RepositoryID <= 0 || !uuidOK(in.InstallationID)) {
		h.fail(w, r, 400, "pbl_source_invalid", "choose an empty repository or an authorized GitHub source", nil)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	project, state, err := h.reservePBLRepository(ctx, tx, s, team, in)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	out := map[string]string{"project_id": project, "state": state}
	if in.RepositoryID != 0 && state == "awaiting_authorization" {
		url, err := h.beginRepositoryAuthorization(ctx, s, "pbl_copy", in.InstallationID, &in.RepositoryID, project)
		if err != nil {
			h.err.Handle(w, r, err)
			return
		}
		out["authorization_url"] = url
	}
	_ = response.OK(w, out)
}

func (h *Handler) reservePBLRepository(ctx context.Context, tx pgx.Tx, s identity.Session, team string, in importRepositoryRequest) (string, string, error) {
	var p, title string
	err := tx.QueryRow(ctx, `SELECT COALESCE(t.project_id::text,''),t.title FROM pbl_teams t
		JOIN pbl_active_team_members m ON m.team_id=t.id AND m.user_id=$3
		WHERE t.id=$1 AND t.college_id=$2 AND t.leader_id=$3 FOR UPDATE OF t`, team, s.CollegeID, s.UserID).Scan(&p, &title)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", errormanager.New(404, "pbl_team_unavailable", "only a confirmed team leader may provision Git", nil)
	}
	if err != nil {
		return "", "", err
	}
	if p != "" {
		var state, install, jobState string
		var source int64
		err = tx.QueryRow(ctx, `SELECT n.state,COALESCE(n.source_installation_id::text,''),COALESCE(n.source_repository_id,0),COALESCE(j.state,'')
			FROM pbl_provisions n LEFT JOIN jobs j ON j.id=n.job_id WHERE n.team_id=$1 AND n.college_id=$2`,
			team, s.CollegeID).Scan(&state, &install, &source, &jobState)
		if err != nil {
			return "", "", err
		}
		if state != "awaiting_authorization" && (source != in.RepositoryID || install != in.InstallationID) {
			return "", "", errormanager.New(409, "pbl_source_conflict", "this team already chose a repository source", nil)
		}
		if state == "queued" && in.RepositoryID == 0 {
			// A retry keeps the original job identity and generation history.
			_, err = tx.Exec(ctx, `UPDATE jobs j SET state='ready',attempts=0,available_at=clock_timestamp(),last_error_code=NULL
				FROM pbl_provisions n WHERE n.team_id=$1 AND n.job_id=j.id AND j.state='dead'`, team)
		}
		if state == "queued" && in.RepositoryID != 0 && jobState == "dead" {
			return p, "awaiting_authorization", nil
		}
		if state != "awaiting_authorization" {
			return p, state, err
		}
	}
	var sourceName string
	if in.RepositoryID != 0 {
		err = tx.QueryRow(ctx, `SELECT c.owner_login||'/'||c.repository_name
			FROM github_repository_candidates c JOIN github_installation_user_grants g ON g.college_id=c.college_id
			AND g.installation_id=c.installation_id AND g.user_id=c.user_id AND g.revoked_at IS NULL
			JOIN github_installations i ON i.id=c.installation_id AND i.college_id=c.college_id AND i.status='active'
			WHERE c.college_id=$1 AND c.user_id=$2 AND c.installation_id=$3 AND c.external_repository_id=$4 AND c.expires_at>clock_timestamp()`,
			s.CollegeID, s.UserID, in.InstallationID, in.RepositoryID).Scan(&sourceName)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", errormanager.New(409, "repository_candidate_expired", "refresh your GitHub repository list and try again", nil)
		}
		if err != nil || !validRepositoryName(sourceName) {
			return "", "", errors.New("GitHub source identity could not be verified")
		}
	}
	if p == "" {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,6))`, s.CollegeID); err != nil {
			return "", "", err
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM pbl_provisions WHERE college_id=$1`, s.CollegeID).Scan(&count); err != nil {
			return "", "", err
		}
		if count >= pblRepositoryLimit {
			return "", "", errormanager.New(409, "pbl_repository_quota", "the campus PBL repository limit has been reached", nil)
		}
		err = tx.QueryRow(ctx, `INSERT INTO projects(college_id,slug,title,summary,project_type,project_context,visibility,created_by)
			VALUES($1,$2,$3,'Coursework project','coursework','pbl','private',$4) RETURNING id::text`, s.CollegeID, "pbl-"+team, title, s.UserID).Scan(&p)
		if err != nil {
			return "", "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO repositories(college_id,project_id,project_context,created_by) VALUES($1,$2,'pbl',$3)`, s.CollegeID, p, s.UserID); err != nil {
			return "", "", err
		}
		if _, err = tx.Exec(ctx, `UPDATE pbl_teams SET project_id=$2 WHERE id=$1`, team, p); err != nil {
			return "", "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO project_members(college_id,project_id,user_id,role)
			SELECT $1,$2,user_id,CASE WHEN user_id=$3 THEN 'owner' ELSE 'member' END
			FROM pbl_active_team_members WHERE team_id=$4 AND college_id=$1`, s.CollegeID, p, s.UserID, team); err != nil {
			return "", "", err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO pbl_provisions(team_id,college_id,project_id,leader_id,source_installation_id,source_repository_id,source_full_name)
		VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,NULLIF($6,0),NULLIF($7,''))
		ON CONFLICT(team_id) DO UPDATE SET source_installation_id=excluded.source_installation_id,
			source_repository_id=excluded.source_repository_id,source_full_name=excluded.source_full_name`,
		team, s.CollegeID, p, s.UserID, in.InstallationID, in.RepositoryID, sourceName); err != nil {
		return "", "", err
	}
	if in.RepositoryID != 0 {
		return p, "awaiting_authorization", nil
	}
	err = queuePBLRepository(ctx, tx, s, team)
	return p, "queued", err
}

func validRepositoryName(name string) bool {
	a, b, ok := strings.Cut(name, "/")
	return ok && repoSegment.MatchString(a) && repoSegment.MatchString(b) && a != "." && a != ".." && b != "." && b != ".."
}

func queuePBLRepository(ctx context.Context, tx pgx.Tx, s identity.Session, team string) error {
	payload, _ := json.Marshal(map[string]string{"team_id": team})
	jobID, err := jobs.EnqueueTx(ctx, tx, s.CollegeID, s.UserID, "pbl_provision", team, payload, nil)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE pbl_provisions SET job_id=$2,state='queued' WHERE team_id=$1`, team, jobID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE jobs SET state='ready',attempts=0,available_at=clock_timestamp(),last_error_code=NULL WHERE id=$1 AND state='dead'`, jobID); err != nil {
		return err
	}
	return audit.Append(ctx, tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "pbl.repository_queued", ResourceType: "pbl_team", ResourceID: team})
}

func (h *Handler) completePBLCopy(ctx context.Context, s identity.Session, state repositoryAuthorizationState, externalInstall int64, userToken string) error {
	if h.forge.URL == "" {
		return errormanager.New(503, "pbl_git_disabled", "PBL Git is not available", nil)
	}
	repos, err := h.api.UserInstallationRepositories(ctx, externalInstall, userToken)
	if err != nil {
		return err
	}
	var fullName string
	for _, item := range repos {
		if item.ID != *state.ExternalRepositoryID {
			continue
		}
		admin, known := item.Permissions["admin"]
		if !known {
			item, err = h.api.Repository(ctx, item.FullName, userToken)
			if err != nil {
				return err
			}
			admin = item.Permissions["admin"]
		}
		if admin && item.ID == *state.ExternalRepositoryID && validRepositoryName(item.FullName) {
			fullName = item.FullName
		}
		break
	}
	if fullName == "" {
		return errormanager.New(403, "repository_admin_required", "you must administer the GitHub source repository", nil)
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var team string
	err = tx.QueryRow(ctx, `UPDATE pbl_provisions n SET source_full_name=$7 WHERE n.project_id=$1 AND n.college_id=$2 AND n.leader_id=$3
		AND n.source_installation_id=$4 AND n.source_repository_id=$5 AND n.state IN ('awaiting_authorization','queued')
		AND EXISTS(SELECT 1 FROM pbl_active_team_members m WHERE m.team_id=n.team_id AND m.user_id=$3)
		AND EXISTS(SELECT 1 FROM github_installation_user_grants g JOIN github_installations i ON i.id=g.installation_id
			WHERE g.installation_id=$4 AND g.user_id=$3 AND g.college_id=$2 AND g.revoked_at IS NULL AND i.external_installation_id=$6 AND i.status='active')
		RETURNING team_id::text`, state.ProjectID, s.CollegeID, s.UserID, state.InstallationID, *state.ExternalRepositoryID, externalInstall, fullName).Scan(&team)
	if errors.Is(err, pgx.ErrNoRows) {
		return errormanager.New(409, "pbl_source_unavailable", "the team or GitHub grant is no longer available", nil)
	}
	if err == nil {
		err = queuePBLRepository(ctx, tx, s, team)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (h *Handler) issuePBLCredential(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	team := chi.URLParam(r, "team")
	if !uuidOK(team) || h.forge.URL == "" {
		h.fail(w, r, 404, "pbl_git_unavailable", "this PBL repository is unavailable", nil)
		return
	}
	password, _, err := randomState()
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	sum := sha256.Sum256([]byte(password))
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var leader string
	err = tx.QueryRow(ctx, `SELECT t.leader_id::text FROM pbl_teams t JOIN pbl_provisions n ON n.team_id=t.id AND n.state='ready'
		JOIN pbl_active_team_members m ON m.team_id=t.id AND m.user_id=$3
		WHERE t.id=$1 AND t.college_id=$2 FOR UPDATE OF t`, team, s.CollegeID, s.UserID).Scan(&leader)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "pbl_git_unavailable", "this PBL repository is unavailable", nil)
		return
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE pbl_git_credentials SET revoked_at=clock_timestamp() WHERE team_id=$1 AND user_id=$2 AND revoked_at IS NULL`, team, s.UserID)
	}
	var expires time.Time
	if err == nil {
		err = tx.QueryRow(ctx, `INSERT INTO pbl_git_credentials(college_id,team_id,user_id,secret_hash,expires_at)
			VALUES($1,$2,$3,$4,clock_timestamp()+interval '24 hours') RETURNING expires_at`, s.CollegeID, team, s.UserID, sum[:]).Scan(&expires)
	}
	if err == nil {
		err = audit.Append(ctx, tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "pbl.git_credential_issued", ResourceType: "pbl_team", ResourceID: team})
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	_ = response.Created(w, map[string]any{"username": "peergit", "password": password, "expires_at": expires})
}

func (h *Handler) revokePBLCredential(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	team := chi.URLParam(r, "team")
	if !uuidOK(team) {
		h.fail(w, r, 404, "pbl_team_unavailable", "this team is unavailable", nil)
		return
	}
	_, err := h.pool.Exec(r.Context(), `UPDATE pbl_git_credentials SET revoked_at=clock_timestamp()
		WHERE team_id=$1 AND college_id=$2 AND user_id=$3 AND revoked_at IS NULL`, team, s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
