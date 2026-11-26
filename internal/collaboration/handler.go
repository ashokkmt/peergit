// Package collaboration owns project updates, discussions, work planning, and
// lightweight project save/follow actions.
package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"peergit/internal/identity"
	"peergit/internal/platform/audit"
	"peergit/internal/platform/errormanager"
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
		api.Get("/projects/{id}/updates", h.listUpdates)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/updates", h.createUpdate)
		api.With(h.auth.RequireCSRF).Patch("/projects/{id}/updates/{updateID}", h.updateUpdate)
		api.Get("/projects/{id}/discussions", h.listDiscussions)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/discussions", h.createDiscussion)
		api.With(h.auth.RequireCSRF).Patch("/projects/{id}/discussions/{discussionID}", h.updateDiscussion)
		api.Get("/projects/{id}/tasks", h.listTasks)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/tasks", h.createTask)
		api.With(h.auth.RequireCSRF).Patch("/projects/{id}/tasks/{taskID}", h.updateTask)
		api.Get("/projects/{id}/milestones", h.listMilestones)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/milestones", h.createMilestone)
		api.With(h.auth.RequireCSRF).Patch("/projects/{id}/milestones/{milestoneID}", h.updateMilestone)
		api.Get("/projects/{id}/comments", h.listComments)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/comments", h.createComment)
		api.With(h.auth.RequireCSRF).Patch("/projects/{id}/comments/{commentID}", h.updateComment)
		api.With(h.auth.RequireCSRF).Put("/projects/{id}/reactions", h.setReaction)
		api.With(h.auth.RequireCSRF).Delete("/projects/{id}/reactions", h.deleteReaction)
		api.With(h.auth.RequireCSRF).Put("/projects/{id}/follow", h.setFollow)
		api.With(h.auth.RequireCSRF).Delete("/projects/{id}/follow", h.deleteFollow)
		api.With(h.auth.RequireCSRF).Put("/projects/{id}/bookmark", h.setBookmark)
		api.With(h.auth.RequireCSRF).Delete("/projects/{id}/bookmark", h.deleteBookmark)
		api.Get("/me/saved-projects", h.savedProjects)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/reports", h.createReport)
	})
}

type projectAccess struct {
	role, visibility, lifecycle string
	collegeID                   string
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) (identity.Session, bool) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		fail(w, http.StatusUnauthorized, "authentication_required", "sign in is required")
		return identity.Session{}, false
	}
	if s.CollegeID == "" || !s.TermsAccepted || !s.PrivacyAccepted {
		fail(w, http.StatusForbidden, "campus_access_required", "active campus verification and current consent are required")
		return identity.Session{}, false
	}
	return s, true
}

func (h *Handler) access(w http.ResponseWriter, r *http.Request, s identity.Session, projectID string, write bool) (projectAccess, bool) {
	if !uuidOK(projectID) {
		fail(w, http.StatusNotFound, "project_not_found", "project was not found")
		return projectAccess{}, false
	}
	var p projectAccess
	err := h.pool.QueryRow(r.Context(), `SELECT p.college_id::text,COALESCE(m.role,''),p.visibility,p.lifecycle
		FROM projects p LEFT JOIN project_members m ON m.project_id=p.id AND m.user_id=$2
		WHERE p.id=$1 AND p.github_revoke_deadline IS NULL`, projectID, s.UserID).Scan(&p.collegeID, &p.role, &p.visibility, &p.lifecycle)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "project_not_found", "project was not found")
		return projectAccess{}, false
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return projectAccess{}, false
	}
	if p.collegeID != s.CollegeID {
		fail(w, http.StatusNotFound, "project_not_found", "project was not found")
		return projectAccess{}, false
	}
	member := p.role != ""
	visible := member || (p.lifecycle == "active" && p.visibility != "private")
	if !visible || (write && (!member || p.lifecycle == "archived")) {
		fail(w, http.StatusNotFound, "project_not_found", "project was not found")
		return projectAccess{}, false
	}
	return p, true
}

func (h *Handler) requireProject(w http.ResponseWriter, r *http.Request, write bool) (identity.Session, projectAccess, bool) {
	s, ok := h.session(w, r)
	if !ok {
		return identity.Session{}, projectAccess{}, false
	}
	p, ok := h.access(w, r, s, chi.URLParam(r, "id"), write)
	return s, p, ok
}

func (h *Handler) canManage(w http.ResponseWriter, p projectAccess) bool {
	if p.role == "owner" || p.role == "maintainer" {
		return true
	}
	fail(w, http.StatusForbidden, "project_manager_required", "only project owners and maintainers can manage this item")
	return false
}

func (h *Handler) appendEvent(ctx context.Context, tx pgx.Tx, tenant, aggregateType, aggregateID, eventType string, version int64, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return outbox.Append(ctx, tx, outbox.Event{TenantID: tenant, AggregateType: aggregateType, AggregateID: aggregateID, EventType: eventType, Version: version, Payload: body}, []string{"project.activity"})
}

func (h *Handler) appendAudit(ctx context.Context, tx pgx.Tx, s identity.Session, action, kind, id string) error {
	return audit.Append(ctx, tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: action, ResourceType: kind, ResourceID: id})
}

func fail(w http.ResponseWriter, status int, code, message string) {
	_ = response.WriteError(w, status, code, message)
}

func invalidText(value string, min, max int) bool {
	value = strings.TrimSpace(value)
	return len(value) < min || len(value) > max
}

func projectID(r *http.Request) string { return chi.URLParam(r, "id") }

func internalError(h *Handler, w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusConflict, "item_changed_or_unavailable", "this item changed or is no longer available")
		return
	}
	h.err.Handle(w, r, err)
}

func marshalDetails(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

func parseLimit(r *http.Request) int {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		var n int
		if _, err := fmt.Sscanf(raw, "%d", &n); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}
	return limit
}
