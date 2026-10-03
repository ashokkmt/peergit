// Package recruitment owns project roles and their application lifecycle.
package recruitment

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"peergit/internal/identity"
	"peergit/internal/platform/audit"
	"peergit/internal/platform/errormanager"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
	"peergit/internal/platform/outbox"
)

type Handler struct {
	pool *pgxpool.Pool
	auth *identity.Handler
	log  *slog.Logger
	err  *errormanager.Manager
}

func NewHandler(pool *pgxpool.Pool, auth *identity.Handler, logger *slog.Logger, errors *errormanager.Manager) *Handler {
	return &Handler{pool: pool, auth: auth, log: logger, err: errors}
}

func (h *Handler) Register(r chi.Router) {
	r.Group(func(api chi.Router) {
		api.Use(h.auth.Middleware)
		api.Get("/my/applications", h.myApplications)
		api.Get("/projects/{id}/roles", h.roles)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/roles", h.createRole)
		api.With(h.auth.RequireCSRF).Patch("/projects/{id}/roles/{roleID}", h.updateRole)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/applications", h.apply)
		api.Get("/projects/{id}/applications", h.applications)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/applications/{applicationID}/decision", h.decide)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/applications/{applicationID}/withdraw", h.withdraw)
	})
}
func (h *Handler) myApplications(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT a.id::text,a.project_role_id::text,p.id::text,p.title,rr.title,a.status,a.version,a.created_at FROM applications a JOIN project_roles rr ON rr.id=a.project_role_id JOIN projects p ON p.id=rr.project_id AND p.college_id=a.college_id WHERE a.applicant_user_id=$1 AND a.college_id=$2 ORDER BY a.created_at DESC,a.id DESC LIMIT 50`, s.UserID, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, roleID, pid, projectTitle, roleTitle, status string
		var version int
		var created time.Time
		if err = rows.Scan(&id, &roleID, &pid, &projectTitle, &roleTitle, &status, &version, &created); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "role_id": roleID, "project_id": pid, "project_title": projectTitle, "role_title": roleTitle, "status": status, "version": version, "created_at": created})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) createRole(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	in, err := request.Decode[struct {
		Title              string   `json:"title"`
		Description        string   `json:"description"`
		Openings           int      `json:"openings"`
		Difficulty         string   `json:"difficulty"`
		GoodFirstTask      bool     `json:"good_first_task"`
		PrerequisiteSkills []string `json:"prerequisite_skills"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.PrerequisiteSkills == nil {
		in.PrerequisiteSkills = []string{}
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	if in.Difficulty == "" {
		in.Difficulty = "beginner"
	}
	if len(in.Title) < 2 || len(in.Title) > 100 || len(in.Description) < 1 || len(in.Description) > 3000 || in.Openings < 1 || in.Openings > 20 || !difficultyOK(in.Difficulty) || len(in.PrerequisiteSkills) > 20 {
		h.fail(w, r, 400, "role_invalid", "role details are invalid", nil)
		return
	}
	for i, skill := range in.PrerequisiteSkills {
		in.PrerequisiteSkills[i] = strings.TrimSpace(skill)
		if len(in.PrerequisiteSkills[i]) < 1 || len(in.PrerequisiteSkills[i]) > 80 {
			h.fail(w, r, 400, "role_invalid", "prerequisite skill is invalid", nil)
			return
		}
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var locked string
	if err = tx.QueryRow(r.Context(), `SELECT id::text FROM projects WHERE id=$1 AND college_id=$2 FOR UPDATE`, id, s.CollegeID).Scan(&locked); err != nil {
		h.fail(w, r, 404, "project_not_found_or_forbidden", "project not found or you cannot manage it", nil)
		return
	}
	var rid string
	err = tx.QueryRow(r.Context(), `INSERT INTO project_roles(college_id,project_id,title,description,openings,difficulty,good_first_task,prerequisite_skills) SELECT $2,p.id,$3,$4,$5,$7,$8,$9 FROM projects p WHERE p.id=$1 AND p.college_id=$2 AND p.lifecycle IN ('draft','active') AND EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=p.id AND m.user_id=$6 AND m.role IN ('owner','maintainer')) RETURNING id::text`, id, s.CollegeID, in.Title, in.Description, in.Openings, s.UserID, in.Difficulty, in.GoodFirstTask, in.PrerequisiteSkills).Scan(&rid)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "project_not_found_or_forbidden", "project not found or you cannot manage it", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "project.role_created", ResourceType: "project_role", ResourceID: rid}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": rid, "status": "open", "openings": in.Openings})
}

func (h *Handler) updateRole(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	pid, rid := chi.URLParam(r, "id"), chi.URLParam(r, "roleID")
	in, err := request.Decode[struct {
		Openings int    `json:"openings"`
		Status   string `json:"status"`
		Version  int    `json:"version"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Openings < 1 || in.Openings > 20 || (in.Status != "open" && in.Status != "closed") || in.Version < 1 {
		h.fail(w, r, 400, "role_invalid", "role update is invalid", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var lockedProject string
	if err = tx.QueryRow(r.Context(), `SELECT id::text FROM projects WHERE id=$1 AND college_id=$2 FOR UPDATE`, pid, s.CollegeID).Scan(&lockedProject); err != nil {
		h.fail(w, r, 404, "project_not_found_or_forbidden", "project not found or you cannot manage it", nil)
		return
	}
	var lockedRole string
	if err = tx.QueryRow(r.Context(), `SELECT rr.id::text FROM project_roles rr WHERE rr.id=$1 AND rr.project_id=$2 AND rr.college_id=$3 AND EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=rr.project_id AND m.user_id=$4 AND m.role IN('owner','maintainer')) FOR UPDATE`, rid, pid, s.CollegeID, s.UserID).Scan(&lockedRole); err != nil {
		h.fail(w, r, 404, "role_not_found_or_forbidden", "role not found or you cannot manage it", nil)
		return
	}
	var accepted int
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM applications WHERE project_role_id=$1 AND status='accepted'`, rid).Scan(&accepted); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Openings < accepted {
		h.fail(w, r, 409, "openings_below_accepted", "openings cannot be fewer than accepted members", nil)
		return
	}
	if in.Status == "open" && in.Openings <= accepted {
		h.fail(w, r, 409, "role_full", "a full role cannot be reopened", nil)
		return
	}
	var version int
	err = tx.QueryRow(r.Context(), `UPDATE project_roles rr SET openings=$4,status=$5,version=version+1 WHERE rr.id=$1 AND rr.project_id=$2 AND rr.college_id=$3 AND rr.version=$6 AND ($5<>'open' OR EXISTS(SELECT 1 FROM projects p WHERE p.id=rr.project_id AND p.lifecycle='active')) AND EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=rr.project_id AND m.user_id=$7 AND m.role IN('owner','maintainer')) RETURNING version`, rid, pid, s.CollegeID, in.Openings, in.Status, in.Version, s.UserID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 409, "role_changed_or_forbidden", "role changed or you cannot manage it", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "project.role_updated", ResourceType: "project_role", ResourceID: rid}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Version(w, int64(version))
	_ = response.OK(w, map[string]any{"id": rid, "openings": in.Openings, "status": in.Status, "version": version})
}

func (h *Handler) apply(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	projectID := chi.URLParam(r, "id")
	in, err := request.Decode[struct {
		RoleID  string `json:"role_id"`
		Message string `json:"message"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Message = strings.TrimSpace(in.Message)
	if !uuidOK(in.RoleID) || len(in.Message) < 1 || len(in.Message) > 2000 {
		h.fail(w, r, 400, "application_invalid", "application is invalid", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var appID string
	err = tx.QueryRow(r.Context(), `INSERT INTO applications(college_id,project_role_id,applicant_user_id,message)
		SELECT $2,rr.id,$3,$4 FROM project_roles rr JOIN projects p ON p.id=rr.project_id AND p.college_id=rr.college_id
		WHERE rr.id=$1 AND rr.project_id=$5 AND rr.college_id=$2 AND rr.status='open' AND p.lifecycle='active' AND p.visibility IN ('campus','public')
		AND NOT EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=$3) RETURNING id::text`, in.RoleID, s.CollegeID, s.UserID, in.Message, projectID).Scan(&appID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "applications_one_pending") {
			h.fail(w, r, 409, "application_exists", "you already have a pending application for this role", err)
		} else if errors.Is(err, pgx.ErrNoRows) {
			h.fail(w, r, 409, "role_unavailable", "this role is not accepting applications", nil)
		} else {
			h.err.Handle(w, r, err)
		}
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "application.submitted", ResourceType: "application", ResourceID: appID}); err == nil {
		payload, _ := json.Marshal(map[string]string{"application_id": appID, "project_id": projectID, "role_id": in.RoleID})
		err = outbox.Append(r.Context(), tx, outbox.Event{TenantID: s.CollegeID, AggregateType: "application", AggregateID: appID, EventType: "application.created", Version: 1, Payload: payload}, []string{"project.activity"})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": appID, "status": "pending"})
}

func (h *Handler) applications(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	var manager bool
	if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$3 AND role IN('owner','maintainer'))`, id, s.CollegeID, s.UserID).Scan(&manager); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !manager {
		h.fail(w, r, 403, "project_manager_required", "project owner or maintainer access is required", nil)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT a.id::text,a.project_role_id::text,a.applicant_user_id::text,u.display_name,a.message,a.status,a.version,a.created_at FROM applications a JOIN users u ON u.id=a.applicant_user_id WHERE a.college_id=$1 AND EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=$2 AND pm.user_id=$3 AND pm.role IN ('owner','maintainer')) AND a.project_role_id IN(SELECT id FROM project_roles WHERE project_id=$2) ORDER BY a.created_at DESC,a.id DESC LIMIT 100`, s.CollegeID, id, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var aid, rid, uid, name, msg, status string
		var version int
		var created time.Time
		if err = rows.Scan(&aid, &rid, &uid, &name, &msg, &status, &version, &created); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": aid, "role_id": rid, "applicant_user_id": uid, "applicant_name": name, "message": msg, "status": status, "version": version, "created_at": created})
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) withdraw(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "applicationID")
	in, err := request.Decode[struct {
		Version int `json:"version"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Version < 1 {
		h.fail(w, r, 400, "application_invalid", "application version is required", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var version int
	err = tx.QueryRow(r.Context(), `UPDATE applications SET status='withdrawn',version=version+1 WHERE id=$1 AND applicant_user_id=$2 AND college_id=$3 AND status='pending' AND version=$4 RETURNING version`, id, s.UserID, s.CollegeID, in.Version).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 409, "application_changed", "application is no longer pending or has changed", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "application.withdrawn", ResourceType: "application", ResourceID: id}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Version(w, int64(version))
	_ = response.OK(w, map[string]any{"id": id, "status": "withdrawn", "version": version})
}

func (h *Handler) decide(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	pid, aid := chi.URLParam(r, "id"), chi.URLParam(r, "applicationID")
	in, err := request.Decode[struct {
		Decision string `json:"decision"`
		Version  int    `json:"version"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if (in.Decision != "accepted" && in.Decision != "rejected") || in.Version < 1 {
		h.fail(w, r, 400, "decision_invalid", "decision is invalid", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var lockedProject string
	if err = tx.QueryRow(r.Context(), `SELECT id::text FROM projects WHERE id=$1 AND college_id=$2 FOR UPDATE`, pid, s.CollegeID).Scan(&lockedProject); err != nil {
		h.fail(w, r, 404, "project_not_found", "project not found", nil)
		return
	}
	var roleID, userID string
	err = tx.QueryRow(r.Context(), `SELECT a.project_role_id::text,a.applicant_user_id::text FROM applications a JOIN project_roles rr ON rr.id=a.project_role_id WHERE a.id=$1 AND rr.project_id=$2 AND a.college_id=$3 FOR UPDATE OF a,rr`, aid, pid, s.CollegeID).Scan(&roleID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "application_not_found", "application not found", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var allowed bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2 AND role IN('owner','maintainer'))`, pid, s.UserID).Scan(&allowed); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !allowed {
		h.fail(w, r, 403, "project_manager_required", "project owner or maintainer access is required", nil)
		return
	}
	var openings, acceptedBefore int
	if in.Decision == "accepted" {
		err = tx.QueryRow(r.Context(), `SELECT openings FROM project_roles WHERE id=$1 AND status='open' FOR UPDATE`, roleID).Scan(&openings)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				h.fail(w, r, 409, "role_unavailable", "role is closed", nil)
			} else {
				h.err.Handle(w, r, err)
			}
			return
		}
		if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM applications WHERE project_role_id=$1 AND status='accepted'`, roleID).Scan(&acceptedBefore); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if acceptedBefore >= openings {
			h.fail(w, r, 409, "role_full", "role has no remaining openings", nil)
			return
		}
	}
	var version int
	err = tx.QueryRow(r.Context(), `UPDATE applications SET status=$2,decided_by=$3,decided_at=now(),version=version+1 WHERE id=$1 AND college_id=$4 AND status='pending' AND version=$5 RETURNING version`, aid, in.Decision, s.UserID, s.CollegeID, in.Version).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 409, "application_changed", "application is no longer pending or has changed", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Decision == "accepted" {
		if _, err = tx.Exec(r.Context(), `INSERT INTO project_members(college_id,project_id,user_id,role) VALUES($1,$2,$3,'member')`, s.CollegeID, pid, userID); err != nil {
			h.fail(w, r, 409, "membership_conflict", "applicant is already a project member", err)
			return
		}
		if acceptedBefore+1 >= openings {
			if _, err = tx.Exec(r.Context(), `UPDATE project_roles SET status='closed',version=version+1 WHERE id=$1 AND status='open'`, roleID); err != nil {
				h.err.Handle(w, r, err)
				return
			}
			if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "project.role_filled", ResourceType: "project_role", ResourceID: roleID}); err != nil {
				h.err.Handle(w, r, err)
				return
			}
		}
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "application." + in.Decision, ResourceType: "application", ResourceID: aid}); err == nil {
		err = outbox.Append(r.Context(), tx, outbox.Event{TenantID: s.CollegeID, AggregateType: "application", AggregateID: aid, EventType: "application." + in.Decision, Version: int64(version), Payload: json.RawMessage(fmt.Sprintf(`{"application_id":%q,"project_id":%q,"user_id":%q}`, aid, pid, userID))}, []string{"project.activity"})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Version(w, int64(version))
	_ = response.OK(w, map[string]any{"id": aid, "status": in.Decision, "version": version})
}
func (h *Handler) roles(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	rows, err := h.pool.Query(r.Context(), `SELECT rr.id::text,rr.title,rr.description,rr.openings,rr.status,rr.version,rr.difficulty,rr.good_first_task,rr.prerequisite_skills
		FROM project_roles rr JOIN projects p ON p.id=rr.project_id AND p.college_id=rr.college_id
		WHERE rr.project_id=$1 AND rr.college_id=$2 AND ((p.lifecycle='active' AND p.visibility IN ('campus','public') AND rr.status='open') OR EXISTS(SELECT 1 FROM project_members WHERE project_id=rr.project_id AND user_id=$3))
		ORDER BY rr.created_at,rr.id`, id, s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var rid, title, description, status, difficulty string
		var openings, version int
		var firstTask bool
		var prerequisites []string
		if err := rows.Scan(&rid, &title, &description, &openings, &status, &version, &difficulty, &firstTask, &prerequisites); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": rid, "title": title, "description": description, "openings": openings, "status": status, "version": version, "difficulty": difficulty, "good_first_task": firstTask, "prerequisite_skills": prerequisites})
	}
	if err := rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) (identity.Session, bool) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, 401, "authentication_required", "sign in is required", nil)
		return s, false
	}
	if s.CollegeID == "" || !s.TermsAccepted || !s.PrivacyAccepted {
		h.fail(w, r, 403, "campus_access_required", "verified campus access and current policy consent are required", nil)
		return s, false
	}
	return s, true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, status int, code, message string, cause error) {
	h.err.Handle(w, r, errormanager.New(status, code, message, cause))
}

func uuidOK(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func difficultyOK(s string) bool {
	return s == "novice" || s == "beginner" || s == "intermediate" || s == "advanced"
}
