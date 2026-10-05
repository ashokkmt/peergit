// Package project owns campus projects, their teams, and recruitment lifecycle.
package project

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
	"peergit/internal/platform/http/cursor"
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
		api.Get("/projects", h.list)
		api.Get("/projects/{id}", h.get)
		api.Get("/projects/{id}/people", h.people)
		api.Get("/my/project-invitations", h.myInvitations)
		api.With(h.auth.RequireCSRF).Post("/projects", h.repositoryRequired)
		api.With(h.auth.RequireCSRF).Patch("/projects/{id}", h.update)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/invitations", h.invite)
		api.With(h.auth.RequireCSRF).Post("/project-invitations/{id}/accept", h.acceptInvite)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/members/{userID}/role", h.changeMemberRole)
		api.With(h.auth.RequireCSRF).Delete("/projects/{id}/members/{userID}", h.removeMember)
	})
}

type createProjectRequest struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Type        string   `json:"project_type"`
	Visibility  string   `json:"visibility"`
	Lifecycle   string   `json:"lifecycle"`
	Skills      []string `json:"skills"`
	MediaIDs    []string `json:"media_ids"`
}

func (in *createProjectRequest) Validate() error {
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Title, in.Summary = strings.TrimSpace(in.Title), strings.TrimSpace(in.Summary)
	in.Description = strings.TrimSpace(in.Description)
	if in.Visibility == "" {
		in.Visibility = "campus"
	}
	if in.Lifecycle == "" {
		in.Lifecycle = "draft"
	}
	if in.Type == "" {
		in.Type = "side_project"
	}
	if !slugOK(in.Slug) || len(in.Title) < 3 || len(in.Title) > 120 || len(in.Summary) < 1 || len(in.Summary) > 500 || len(in.Description) > 12000 || !projectTypeOK(in.Type) || !visibilityOK(in.Visibility) || !lifecycleOK(in.Lifecycle) || len(in.Skills) > 20 || len(in.MediaIDs) > 10 {
		return errors.New("project fields are invalid")
	}
	normalizedSkills := make([]string, 0, len(in.Skills))
	seenSkills := map[string]bool{}
	for _, skill := range in.Skills {
		skill = strings.TrimSpace(skill)
		slug := skillSlug(skill)
		if len(skill) == 0 || len(skill) > 80 || !slugOK(slug) {
			return errors.New("project skills are invalid")
		}
		if !seenSkills[slug] {
			normalizedSkills = append(normalizedSkills, skill)
			seenSkills[slug] = true
		}
	}
	in.Skills = normalizedSkills
	normalizedMedia := make([]string, 0, len(in.MediaIDs))
	seenMedia := map[string]bool{}
	for _, id := range in.MediaIDs {
		if !uuidOK(id) {
			return errors.New("project media IDs are invalid")
		}
		if !seenMedia[id] {
			normalizedMedia = append(normalizedMedia, id)
			seenMedia[id] = true
		}
	}
	in.MediaIDs = normalizedMedia
	return nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	s, authenticated := identity.CurrentSession(r.Context())
	campusAccess := authenticated && s.CollegeID != "" && s.TermsAccepted && s.PrivacyAccepted
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 100 {
		h.fail(w, r, 400, "query_invalid", "search query is too long", nil)
		return
	}
	cursorToken := r.URL.Query().Get("cursor")
	cursorID := ""
	if cursorToken != "" {
		if authenticated {
			payload, decodeErr := cursor.Decode(s.CSRFHash, cursorToken)
			if decodeErr != nil || !uuidOK(string(payload)) {
				h.fail(w, r, 400, "cursor_invalid", "project cursor is invalid", nil)
				return
			}
			cursorID = string(payload)
		} else if !uuidOK(cursorToken) {
			h.fail(w, r, 400, "cursor_invalid", "project cursor is invalid", nil)
			return
		} else {
			cursorID = cursorToken
		}
	}
	var rows pgx.Rows
	var err error
	if campusAccess {
		rows, err = h.pool.Query(r.Context(), `SELECT p.id::text,p.slug,p.title,p.summary,p.project_type,p.visibility,p.lifecycle,p.version,p.updated_at,
		EXISTS(SELECT 1 FROM project_roles rr WHERE rr.project_id=p.id AND rr.status='open') AS recruiting,
		COALESCE(array_agg(DISTINCT sk.name) FILTER(WHERE sk.name IS NOT NULL),'{}'::text[]),
		EXISTS(SELECT 1 FROM repositories rp WHERE rp.project_id=p.id AND rp.college_id=p.college_id) AS has_repository
		FROM projects p LEFT JOIN project_skills ps ON ps.project_id=p.id LEFT JOIN skills sk ON sk.id=ps.skill_id
		WHERE ((p.visibility='public' AND p.lifecycle='active' AND EXISTS(SELECT 1 FROM repositories rp WHERE rp.project_id=p.id AND rp.college_id=p.college_id)) OR (p.college_id=$1 AND ((p.lifecycle='active' AND p.visibility='campus' AND EXISTS(SELECT 1 FROM repositories rp WHERE rp.project_id=p.id AND rp.college_id=p.college_id)) OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=$4))))
		AND ($2::uuid IS NULL OR p.id<$2) AND ($3='' OR p.title ILIKE '%'||$3||'%' OR p.summary ILIKE '%'||$3||'%')
		GROUP BY p.id ORDER BY p.id DESC LIMIT 51`, s.CollegeID, nullUUID(cursorID), q, s.UserID)
	} else {
		rows, err = h.pool.Query(r.Context(), `SELECT p.id::text,p.slug,p.title,p.summary,p.project_type,p.visibility,p.lifecycle,p.version,p.updated_at,
		EXISTS(SELECT 1 FROM project_roles rr WHERE rr.project_id=p.id AND rr.status='open') AS recruiting,
		COALESCE(array_agg(DISTINCT sk.name) FILTER(WHERE sk.name IS NOT NULL),'{}'::text[]),
		EXISTS(SELECT 1 FROM repositories rp WHERE rp.project_id=p.id AND rp.college_id=p.college_id) AS has_repository
		FROM projects p LEFT JOIN project_skills ps ON ps.project_id=p.id LEFT JOIN skills sk ON sk.id=ps.skill_id
		WHERE p.visibility='public' AND p.lifecycle='active' AND EXISTS(SELECT 1 FROM repositories rp WHERE rp.project_id=p.id AND rp.college_id=p.college_id) AND ($1='' OR p.title ILIKE '%'||$1||'%' OR p.summary ILIKE '%'||$1||'%')
		AND ($2::uuid IS NULL OR p.id<$2) GROUP BY p.id ORDER BY p.id DESC LIMIT 51`, q, nullUUID(cursorID))
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := []projectView{}
	for rows.Next() {
		var v projectView
		if err = rows.Scan(&v.ID, &v.Slug, &v.Title, &v.Summary, &v.Type, &v.Visibility, &v.Lifecycle, &v.Version, &v.UpdatedAt, &v.Recruiting, &v.Skills, &v.HasRepository); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if !v.HasRepository {
			v.Visibility = "private"
			if v.Lifecycle == "active" {
				v.Lifecycle = "draft"
			}
			v.Recruiting = false
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	next := ""
	if len(items) > 50 {
		if authenticated {
			next, err = cursor.Encode(s.CSRFHash, []byte(items[49].ID))
			if err != nil {
				h.err.Handle(w, r, err)
				return
			}
		} else {
			next = items[49].ID
		}
		items = items[:50]
	}
	_ = response.OK(w, map[string]any{"items": items, "next_cursor": next})
}

type projectView struct {
	ID            string    `json:"id"`
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary"`
	Type          string    `json:"project_type"`
	Visibility    string    `json:"visibility"`
	Lifecycle     string    `json:"lifecycle"`
	Version       int       `json:"version"`
	UpdatedAt     time.Time `json:"updated_at"`
	Recruiting    bool      `json:"recruiting"`
	Skills        []string  `json:"skills"`
	HasRepository bool      `json:"-"`
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	s, authenticated := identity.CurrentSession(r.Context())
	campusAccess := authenticated && s.CollegeID != "" && s.TermsAccepted && s.PrivacyAccepted
	id := chi.URLParam(r, "id")
	var v projectView
	var description, creator, projectCollegeID string
	var err error
	if campusAccess {
		err = h.pool.QueryRow(r.Context(), `SELECT p.id::text,p.slug,p.title,p.summary,p.description,p.project_type,p.visibility,p.lifecycle,p.version,p.updated_at,p.created_by::text,p.college_id::text,
		EXISTS(SELECT 1 FROM project_roles rr WHERE rr.project_id=p.id AND rr.status='open'),COALESCE(array_agg(DISTINCT sk.name) FILTER(WHERE sk.name IS NOT NULL),'{}'::text[]),EXISTS(SELECT 1 FROM repositories rp WHERE rp.project_id=p.id AND rp.college_id=p.college_id)
		FROM projects p LEFT JOIN project_skills ps ON ps.project_id=p.id LEFT JOIN skills sk ON sk.id=ps.skill_id
		WHERE p.id=$1 AND ((p.visibility='public' AND p.lifecycle='active' AND EXISTS(SELECT 1 FROM repositories rp WHERE rp.project_id=p.id AND rp.college_id=p.college_id)) OR (p.college_id=$2 AND ((p.lifecycle='active' AND p.visibility='campus' AND EXISTS(SELECT 1 FROM repositories rp WHERE rp.project_id=p.id AND rp.college_id=p.college_id)) OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=$3)))) GROUP BY p.id`, id, s.CollegeID, s.UserID).Scan(&v.ID, &v.Slug, &v.Title, &v.Summary, &description, &v.Type, &v.Visibility, &v.Lifecycle, &v.Version, &v.UpdatedAt, &creator, &projectCollegeID, &v.Recruiting, &v.Skills, &v.HasRepository)
	} else {
		err = h.pool.QueryRow(r.Context(), `SELECT p.id::text,p.slug,p.title,p.summary,p.project_type,p.visibility,p.lifecycle,p.version,p.updated_at,
		EXISTS(SELECT 1 FROM project_roles rr WHERE rr.project_id=p.id AND rr.status='open'),COALESCE(array_agg(DISTINCT sk.name) FILTER(WHERE sk.name IS NOT NULL),'{}'::text[]),EXISTS(SELECT 1 FROM repositories rp WHERE rp.project_id=p.id AND rp.college_id=p.college_id)
		FROM projects p LEFT JOIN project_skills ps ON ps.project_id=p.id LEFT JOIN skills sk ON sk.id=ps.skill_id
		WHERE p.id=$1 AND p.visibility='public' AND p.lifecycle='active' AND EXISTS(SELECT 1 FROM repositories rp WHERE rp.project_id=p.id AND rp.college_id=p.college_id) GROUP BY p.id`, id).Scan(&v.ID, &v.Slug, &v.Title, &v.Summary, &v.Type, &v.Visibility, &v.Lifecycle, &v.Version, &v.UpdatedAt, &v.Recruiting, &v.Skills, &v.HasRepository)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "project_not_found", "project not found", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !v.HasRepository {
		v.Visibility = "private"
		if v.Lifecycle == "active" {
			v.Lifecycle = "draft"
		}
		v.Recruiting = false
	}
	if !campusAccess || projectCollegeID != s.CollegeID {
		_ = response.OK(w, map[string]any{"project": v, "description": "", "created_by": "", "viewer_role": "", "members": []any{}})
		return
	}
	var members []map[string]any
	rows, e := h.pool.Query(r.Context(), `SELECT u.id::text,u.handle,u.display_name,pm.role FROM project_members pm JOIN users u ON u.id=pm.user_id WHERE pm.project_id=$1 AND pm.college_id=$2 ORDER BY CASE pm.role WHEN 'owner' THEN 0 ELSE 1 END,u.display_name`, id, s.CollegeID)
	if e != nil {
		h.err.Handle(w, r, e)
		return
	}
	defer rows.Close()
	members = []map[string]any{}
	for rows.Next() {
		var uid, handle, name, role string
		if e = rows.Scan(&uid, &handle, &name, &role); e != nil {
			h.err.Handle(w, r, e)
			return
		}
		members = append(members, map[string]any{"user_id": uid, "handle": handle, "display_name": name, "role": role})
	}
	if e = rows.Err(); e != nil {
		h.err.Handle(w, r, e)
		return
	}
	var viewerRole string
	if err = h.pool.QueryRow(r.Context(), `SELECT role FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$3`, id, s.CollegeID, s.UserID).Scan(&viewerRole); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"project": v, "description": description, "created_by": creator, "viewer_role": viewerRole, "members": members})
}

func (h *Handler) people(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 || len(q) > 80 {
		h.fail(w, r, 400, "query_invalid", "enter 2 to 80 characters to search campus members", nil)
		return
	}
	var manager bool
	if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$3 AND role IN('owner','maintainer'))`, chi.URLParam(r, "id"), s.CollegeID, s.UserID).Scan(&manager); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !manager {
		h.fail(w, r, 403, "project_manager_required", "project owner or maintainer access is required", nil)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT u.id::text,u.display_name,u.handle FROM users u WHERE u.college_id=$1 AND u.status='active' AND (u.display_name ILIKE '%'||$2||'%' OR u.handle ILIKE '%'||$2||'%') AND NOT EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=$3 AND pm.user_id=u.id) ORDER BY u.display_name,u.id LIMIT 20`, s.CollegeID, q, chi.URLParam(r, "id"))
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, name, handle string
		if err = rows.Scan(&id, &name, &handle); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]string{"id": id, "display_name": name, "handle": handle})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) myInvitations(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT i.id::text,i.project_id::text,p.title,i.role,i.expires_at FROM project_invitations i JOIN projects p ON p.id=i.project_id AND p.college_id=i.college_id WHERE i.invited_user_id=$1 AND i.college_id=$2 AND i.accepted_at IS NULL AND i.revoked_at IS NULL AND i.expires_at>now() ORDER BY i.created_at DESC,i.id DESC LIMIT 50`, s.UserID, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, pid, title, role string
		var expires time.Time
		if err = rows.Scan(&id, &pid, &title, &role, &expires); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "project_id": pid, "project_title": title, "role": role, "expires_at": expires})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) repositoryRequired(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.session(w, r); !ok {
		return
	}
	h.fail(w, r, http.StatusConflict, "github_repository_required", "import an authorized GitHub repository to start a project", nil)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	in, err := request.Decode[struct {
		Summary     string `json:"summary"`
		Description string `json:"description"`
		Visibility  string `json:"visibility"`
		Lifecycle   string `json:"lifecycle"`
		Version     int    `json:"version"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Summary = strings.TrimSpace(in.Summary)
	in.Description = strings.TrimSpace(in.Description)
	if in.Version < 1 || len(in.Summary) < 1 || len(in.Summary) > 500 || len(in.Description) > 12000 || !visibilityOK(in.Visibility) || !lifecycleOK(in.Lifecycle) {
		h.fail(w, r, 400, "project_invalid", "project update is invalid", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var next int
	err = tx.QueryRow(r.Context(), `UPDATE projects p SET summary=$4,description=$5,visibility=$6,lifecycle=$7,version=version+1,updated_at=now() WHERE p.id=$1 AND p.college_id=$2 AND p.version=$3 AND EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=p.id AND m.user_id=$8 AND m.role IN ('owner','maintainer')) AND (EXISTS(SELECT 1 FROM repositories rp WHERE rp.project_id=p.id AND rp.college_id=p.college_id) OR ($6='private' AND $7 IN ('draft','archived'))) RETURNING version`, id, s.CollegeID, in.Version, in.Summary, in.Description, in.Visibility, in.Lifecycle, s.UserID).Scan(&next)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 409, "project_changed_or_forbidden", "project changed or you cannot edit it", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Lifecycle != "active" {
		if _, err = tx.Exec(r.Context(), `UPDATE project_roles SET status='closed',version=version+1 WHERE project_id=$1 AND college_id=$2 AND status='open'`, id, s.CollegeID); err != nil {
			h.err.Handle(w, r, err)
			return
		}
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "project.updated", ResourceType: "project", ResourceID: id}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	updatePayload, _ := json.Marshal(map[string]any{"lifecycle": in.Lifecycle, "visibility": in.Visibility, "version": next})
	if err = outbox.Append(r.Context(), tx, outbox.Event{TenantID: s.CollegeID, AggregateType: "project", AggregateID: id, EventType: "project.updated", Version: int64(next), Payload: updatePayload}, []string{"project.activity"}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Version(w, int64(next))
	_ = response.OK(w, map[string]any{"id": id, "version": next})
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	pid := chi.URLParam(r, "id")
	in, err := request.Decode[struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !uuidOK(in.UserID) || (in.Role != "maintainer" && in.Role != "member" && in.Role != "mentor") {
		h.fail(w, r, 400, "invitation_invalid", "invitation is invalid", nil)
		return
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var lockedProject string
	if err = tx.QueryRow(r.Context(), `SELECT id::text FROM projects WHERE id=$1 AND college_id=$2 FOR UPDATE`, pid, s.CollegeID).Scan(&lockedProject); err != nil {
		h.fail(w, r, 404, "project_or_user_not_found", "project or campus user not found", nil)
		return
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO project_invitations(college_id,project_id,invited_user_id,role,token_hash,invited_by,expires_at) SELECT $2,p.id,$3,$4,$5,$6,now()+interval '7 days' FROM projects p WHERE p.id=$1 AND p.college_id=$2 AND EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=p.id AND m.user_id=$6 AND m.role IN('owner','maintainer')) AND EXISTS(SELECT 1 FROM users u WHERE u.id=$3 AND u.college_id=$2 AND u.status='active') RETURNING id::text`, pid, s.CollegeID, in.UserID, in.Role, hash[:], s.UserID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "project_or_user_not_found", "project or campus user not found", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "project.invitation.created", ResourceType: "project_invitation", ResourceID: id, Details: json.RawMessage(fmt.Sprintf(`{"role":%q}`, in.Role))}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "token": token, "expires_in_days": 7})
}

func (h *Handler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	in, err := request.Decode[struct {
		Token string `json:"token"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if len(in.Token) < 32 || len(in.Token) > 128 {
		h.fail(w, r, 400, "invitation_token_invalid", "invitation token is invalid", nil)
		return
	}
	tokenHash := sha256.Sum256([]byte(in.Token))
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var pid, role string
	err = tx.QueryRow(r.Context(), `UPDATE project_invitations SET accepted_at=now() WHERE id=$1 AND invited_user_id=$2 AND college_id=$3 AND token_hash=$4 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at>now() RETURNING project_id::text,role`, id, s.UserID, s.CollegeID, tokenHash[:]).Scan(&pid, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "invitation_unavailable", "invitation not found or no longer available", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO project_members(college_id,project_id,user_id,role) VALUES($1,$2,$3,$4)`, s.CollegeID, pid, s.UserID, role)
	if err != nil {
		h.fail(w, r, 409, "membership_conflict", "you are already a member of this project", err)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "project.invitation.accepted", ResourceType: "project_invitation", ResourceID: id}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"project_id": pid, "role": role, "accepted": true})
}

func (h *Handler) changeMemberRole(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	pid, uid := chi.URLParam(r, "id"), chi.URLParam(r, "userID")
	in, err := request.Decode[struct {
		Role string `json:"role"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Role != "owner" && in.Role != "maintainer" && in.Role != "member" && in.Role != "mentor" {
		h.fail(w, r, 400, "member_role_invalid", "member role is invalid", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var locked string
	if err = tx.QueryRow(r.Context(), `SELECT id::text FROM projects WHERE id=$1 AND college_id=$2 FOR UPDATE`, pid, s.CollegeID).Scan(&locked); err != nil {
		h.fail(w, r, 404, "project_not_found", "project not found", nil)
		return
	}
	var manager bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2 AND role='owner')`, pid, s.UserID).Scan(&manager); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !manager {
		h.fail(w, r, 403, "project_owner_required", "project owner access is required", nil)
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE project_members SET role=$4 WHERE project_id=$1 AND college_id=$2 AND user_id=$3 AND ($3<>$5 OR $4='owner' OR (SELECT count(*) FROM project_members WHERE project_id=$1 AND role='owner')>1)`, pid, s.CollegeID, uid, in.Role, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if tag.RowsAffected() != 1 {
		h.fail(w, r, 409, "last_owner_required", "a project must retain at least one owner", nil)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "project.member_role_changed", ResourceType: "project", ResourceID: pid}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"user_id": uid, "role": in.Role})
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	pid, uid := chi.URLParam(r, "id"), chi.URLParam(r, "userID")
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var locked string
	if err = tx.QueryRow(r.Context(), `SELECT id::text FROM projects WHERE id=$1 AND college_id=$2 FOR UPDATE`, pid, s.CollegeID).Scan(&locked); err != nil {
		h.fail(w, r, 404, "project_not_found", "project not found", nil)
		return
	}
	var owners int
	var manager bool
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FILTER(WHERE role='owner'),COALESCE(bool_or(user_id=$3 AND role IN('owner','maintainer')),false) FROM project_members WHERE project_id=$1 AND college_id=$2`, pid, s.CollegeID, s.UserID).Scan(&owners, &manager); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !manager {
		h.fail(w, r, 403, "project_manager_required", "project manager access is required", nil)
		return
	}
	var targetRole string
	err = tx.QueryRow(r.Context(), `SELECT role FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$3 FOR UPDATE`, pid, s.CollegeID, uid).Scan(&targetRole)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "member_not_found", "project member not found", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if targetRole == "owner" {
		var owner bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$3 AND role='owner')`, pid, s.CollegeID, s.UserID).Scan(&owner); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if !owner {
			h.fail(w, r, 403, "project_owner_required", "only an owner can remove another owner", nil)
			return
		}
	}
	if targetRole == "owner" && owners <= 1 {
		h.fail(w, r, 409, "last_owner_required", "transfer ownership before removing the last owner", nil)
		return
	}
	tag, err := tx.Exec(r.Context(), `DELETE FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$3`, pid, s.CollegeID, uid)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if tag.RowsAffected() != 1 {
		h.fail(w, r, 404, "member_not_found", "project member not found", nil)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "project.member_removed", ResourceType: "project", ResourceID: pid}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	response.NoContent(w)
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

func slugOK(s string) bool {
	if len(s) < 2 || len(s) > 80 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return !strings.Contains(s, "--")
}
func skillSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.NewReplacer("++", "-plus-plus", "#", "-sharp", "+", "-plus", ".", "-", "_", "-", " ", "-").Replace(s)
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
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
func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func projectTypeOK(s string) bool {
	switch s {
	case "side_project", "coursework", "research", "startup", "open_source", "hackathon":
		return true
	}
	return false
}
func visibilityOK(s string) bool { return s == "private" || s == "campus" || s == "public" }
func lifecycleOK(s string) bool {
	return s == "draft" || s == "active" || s == "on_hold" || s == "completed" || s == "archived"
}
func difficultyOK(s string) bool {
	return s == "novice" || s == "beginner" || s == "intermediate" || s == "advanced"
}
