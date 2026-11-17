package repository

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"peergit/internal/platform/audit"
	"peergit/internal/platform/http/response"
	"peergit/internal/platform/jobs"
	"peergit/internal/platform/storage"
)

func (h *Handler) refreshGitHubInstallation(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	install := chi.URLParam(r, "installationID")
	var external string
	err := h.pool.QueryRow(r.Context(), `SELECT i.external_installation_id::text FROM github_installations i JOIN github_installation_user_grants g ON g.installation_id=i.id AND g.college_id=i.college_id WHERE i.id=$1 AND i.college_id=$2 AND g.user_id=$3 AND g.revoked_at IS NULL AND i.status='active'`, install, s.CollegeID, s.UserID).Scan(&external)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "github_installation_not_found", "active GitHub connection was not found", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	url, err := h.beginRepositoryAuthorization(r.Context(), s, "refresh", install, nil, "")
	if err != nil {
		h.fail(w, r, 503, "github_authorization_unavailable", "GitHub authorization could not be started", err)
		return
	}
	_ = response.OK(w, map[string]string{"authorization_url": url})
}

func (h *Handler) githubInstallationImports(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	var items []map[string]any
	rows, err := h.pool.Query(r.Context(), `SELECT p.id::text,p.title,p.visibility,p.lifecycle FROM projects p JOIN repositories rp ON rp.project_id=p.id AND rp.college_id=p.college_id JOIN repository_bindings b ON b.repository_id=rp.id AND b.college_id=rp.college_id WHERE p.college_id=$1 AND p.created_by=$2 AND b.linked_by=$2 AND b.installation_id=$3 AND p.github_revoke_deadline IS NULL ORDER BY p.created_at DESC`, s.CollegeID, s.UserID, chi.URLParam(r, "installationID"))
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items = make([]map[string]any, 0)
	for rows.Next() {
		var id, title, vis, life string
		if err = rows.Scan(&id, &title, &vis, &life); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "title": title, "visibility": vis, "lifecycle": life})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) revokeGitHubInstallation(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	install := chi.URLParam(r, "installationID")
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var found bool
	err = tx.QueryRow(r.Context(), `UPDATE github_installation_user_grants SET revoked_at=clock_timestamp() WHERE college_id=$1 AND user_id=$2 AND installation_id=$3 AND revoked_at IS NULL RETURNING true`, s.CollegeID, s.UserID, install).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "github_connection_not_found", "active GitHub connection was not found", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_, err = tx.Exec(r.Context(), `DELETE FROM github_repository_candidates WHERE college_id=$1 AND user_id=$2 AND installation_id=$3`, s.CollegeID, s.UserID, install)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	rows, err := tx.Query(r.Context(), `UPDATE projects p SET github_revoke_deadline=clock_timestamp()+interval '30 days',github_revoke_owner=$2,github_revoke_installation=$3,github_revoke_previous_visibility=p.visibility,github_revoke_previous_lifecycle=p.lifecycle,visibility='private',lifecycle='archived',version=version+1,updated_at=clock_timestamp() FROM repositories rp,repository_bindings b WHERE rp.project_id=p.id AND rp.college_id=p.college_id AND b.repository_id=rp.id AND b.college_id=rp.college_id AND p.college_id=$1 AND p.created_by=$2 AND b.linked_by=$2 AND b.installation_id=$3 AND p.github_revoke_deadline IS NULL RETURNING p.id::text`, s.CollegeID, s.UserID, install)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	projectIDs := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			h.err.Handle(w, r, err)
			return
		}
		projectIDs = append(projectIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	for _, pid := range projectIDs {
		if _, err = tx.Exec(r.Context(), `UPDATE repository_bindings SET access_state='inactive',sync_health='failed',last_error_code='user_connection_revoked',updated_at=clock_timestamp() WHERE repository_id=(SELECT id FROM repositories WHERE project_id=$1 AND college_id=$2) AND installation_id=$3 AND is_current`, pid, s.CollegeID, install); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "github.project.import_revoked", ResourceType: "project", ResourceID: pid}); err != nil {
			h.err.Handle(w, r, err)
			return
		}
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "github.connection.revoked", ResourceType: "github_installation", ResourceID: install}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"retained_projects": projectIDs, "delete_after": time.Now().UTC().Add(30 * 24 * time.Hour)})
}

func (h *Handler) retainedGitHubProjects(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT id::text,title,github_revoke_deadline FROM projects WHERE college_id=$1 AND github_revoke_owner=$2 AND github_revoke_deadline>clock_timestamp() ORDER BY github_revoke_deadline`, s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, title string
		var due time.Time
		if err = rows.Scan(&id, &title, &due); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "title": title, "delete_after": due})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) restoreGitHubProject(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	pid := chi.URLParam(r, "id")
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var vis, life, install, repo string
	err = tx.QueryRow(r.Context(), `SELECT p.github_revoke_previous_visibility,p.github_revoke_previous_lifecycle,p.github_revoke_installation::text,b.external_repository_id::text FROM projects p JOIN repositories rp ON rp.project_id=p.id AND rp.college_id=p.college_id JOIN repository_bindings b ON b.repository_id=rp.id AND b.college_id=rp.college_id JOIN github_installations i ON i.id=b.installation_id AND i.college_id=b.college_id AND i.status='active' WHERE p.id=$1 AND p.college_id=$2 AND p.created_by=$3 AND p.github_revoke_owner=$3 AND p.github_revoke_deadline>clock_timestamp()`, pid, s.CollegeID, s.UserID).Scan(&vis, &life, &install, &repo)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "retained_project_not_found", "recoverable imported project was not found", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var allowed bool
	err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM github_repository_candidates c JOIN github_installation_user_grants g ON g.college_id=c.college_id AND g.user_id=c.user_id AND g.installation_id=c.installation_id WHERE c.college_id=$1 AND c.user_id=$2 AND c.installation_id=$3 AND c.external_repository_id=$4 AND c.expires_at>clock_timestamp() AND g.revoked_at IS NULL)`, s.CollegeID, s.UserID, install, repo).Scan(&allowed)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !allowed {
		h.fail(w, r, 409, "github_reauthorization_required", "reconnect GitHub and refresh this repository before restoring", nil)
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE projects SET visibility='private',lifecycle=CASE WHEN github_revoke_previous_lifecycle='archived' THEN 'draft' ELSE github_revoke_previous_lifecycle END,github_revoke_deadline=NULL,github_revoke_owner=NULL,github_revoke_installation=NULL,github_revoke_previous_visibility=NULL,github_revoke_previous_lifecycle=NULL,version=version+1,updated_at=clock_timestamp() WHERE id=$1 AND college_id=$2`, pid, s.CollegeID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE repository_bindings SET access_state='active',sync_health='stale',last_error_code=NULL,updated_at=clock_timestamp() WHERE repository_id=(SELECT id FROM repositories WHERE project_id=$1 AND college_id=$2) AND installation_id=$3 AND is_current`, pid, s.CollegeID, install)
	}
	if err == nil {
		err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "github.project.import_restored", ResourceType: "project", ResourceID: pid})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	payload, _ := json.Marshal(bindingJobForProject(r.Context(), h, pid, s.CollegeID))
	_, _ = (jobs.Queue{Pool: h.pool}).EnqueueWork(r.Context(), s.CollegeID, s.UserID, "github_reconcile", "restore:"+pid, payload, nil)
	_ = response.OK(w, map[string]string{"id": pid, "visibility": "private", "previous_lifecycle": life})
}

func bindingJobForProject(ctx context.Context, h *Handler, pid, tenant string) map[string]string {
	var id string
	_ = h.pool.QueryRow(ctx, `SELECT b.id::text FROM repository_bindings b JOIN repositories r ON r.id=b.repository_id AND r.college_id=b.college_id WHERE r.project_id=$1 AND r.college_id=$2 AND b.is_current`, pid, tenant).Scan(&id)
	return map[string]string{"binding_id": id}
}

// CleanupExpiredGitHubImports removes expired project records only after their stored archives are deleted.
func CleanupExpiredGitHubImports(ctx context.Context, pool interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Begin(context.Context) (pgx.Tx, error)
}, store *storage.Store) error {
	rows, err := pool.Query(ctx, `SELECT p.id::text,p.college_id::text,p.github_revoke_owner::text FROM projects p WHERE p.github_revoke_deadline<=clock_timestamp() ORDER BY p.github_revoke_deadline LIMIT 50`)
	if err != nil {
		return err
	}
	type target struct{ id, tenant, owner string }
	targets := []target{}
	for rows.Next() {
		var t target
		if err = rows.Scan(&t.id, &t.tenant, &t.owner); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, t := range targets {
		keys, err := pool.Query(ctx, `SELECT object_key FROM repository_snapshots WHERE project_id=$1 AND college_id=$2 AND object_key IS NOT NULL`, t.id, t.tenant)
		if err != nil {
			return err
		}
		var objects []string
		for keys.Next() {
			var key string
			if err = keys.Scan(&key); err != nil {
				keys.Close()
				return err
			}
			objects = append(objects, key)
		}
		err = keys.Err()
		keys.Close()
		if err != nil {
			return err
		}
		for _, key := range objects {
			if store == nil {
				return errors.New("object storage is required to purge expired GitHub imports")
			}
			if err = store.Delete(ctx, key); err != nil {
				return err
			}
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM repository_snapshots WHERE project_id=$1 AND college_id=$2`, t.id, t.tenant)
		if err == nil {
			err = audit.Append(ctx, tx, audit.Entry{TenantID: t.tenant, ActorID: t.owner, Action: "github.project.import_retention_expired", ResourceType: "project", ResourceID: t.id})
		}
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM projects WHERE id=$1 AND college_id=$2 AND github_revoke_deadline<=clock_timestamp()`, t.id, t.tenant)
		}
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
