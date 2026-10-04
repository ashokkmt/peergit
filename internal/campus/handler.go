package campus

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/mail"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"peergit/internal/identity"
	"peergit/internal/platform/audit"
	"peergit/internal/platform/errormanager"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
)

type Handler struct {
	pool                   *pgxpool.Pool
	auth                   *identity.Handler
	origin                 string
	err                    *errormanager.Manager
	hashKey, encryptionKey string
}

func NewHandler(pool *pgxpool.Pool, auth *identity.Handler, origin string, errors *errormanager.Manager, hashKey, encryptionKey string) *Handler {
	return &Handler{pool: pool, auth: auth, origin: origin, err: errors, hashKey: hashKey, encryptionKey: encryptionKey}
}

func (h *Handler) Register(r chi.Router) {
	r.Group(func(private chi.Router) {
		private.Use(h.auth.Middleware)
		private.Get("/organizations", h.organizations)
		private.Get("/me/onboarding", h.onboarding)
		private.With(h.auth.RequireCSRF).Post("/me/campus-verification/challenges", h.startCampusChallenge)
		private.With(h.auth.RequireCSRF).Post("/me/campus-verification/confirm", h.confirmCampusChallenge)
		private.With(h.auth.RequireCSRF).Post("/me/campus-review-requests", h.createCampusReview)
		private.Get("/admin/campus-review-requests", h.listCampusReviews)
		private.With(h.auth.RequireCSRF).Post("/admin/campus-review-requests/{id}/decision", h.decideCampusReview)
		private.With(h.auth.RequireCSRF).Post("/admin/invitations", h.createInvitation)
		private.With(h.auth.RequireCSRF).Post("/admin/invitations/{id}/revoke", h.revokeInvitation)
		private.With(h.auth.RequireCSRF).Post("/admin/organizations", h.createOrganization)
		private.With(h.auth.RequireCSRF).Post("/admin/roles", h.grantRole)
		private.Get("/admin/users", h.users)
		private.With(h.auth.RequireCSRF).Post("/admin/users/{id}/status", h.setUserStatus)
	})
}

func (h *Handler) organizations(w http.ResponseWriter, r *http.Request) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
		return
	}
	if s.CollegeID == "" {
		h.fail(w, r, http.StatusForbidden, "campus_scope_required", "this account has no campus organization access", nil)
		return
	}
	if !s.TermsAccepted || !s.PrivacyAccepted {
		h.fail(w, r, http.StatusForbidden, "policy_consent_required", "accept the Terms and Privacy Notice to use campus features", nil)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT id::text,kind,slug,name,description FROM organizations WHERE college_id=$1 ORDER BY name,id`, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID          string `json:"id"`
		Kind        string `json:"kind"`
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.ID, &v.Kind, &v.Slug, &v.Name, &v.Description); err != nil {
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

func (h *Handler) createInvitation(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	input, err := request.Decode[struct {
		Email          string `json:"email"`
		AccountType    string `json:"account_type"`
		Role           string `json:"role"`
		ExternalAccess string `json:"external_access_kind"`
		OrganizationID string `json:"organization_id"`
		ExpiresHours   int    `json:"expires_hours"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email || len(input.Email) > 254 {
		h.fail(w, r, http.StatusBadRequest, "invitation_invalid", "email address is invalid", nil)
		return
	}
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if input.ExpiresHours == 0 {
		input.ExpiresHours = 72
	}
	if input.ExpiresHours < 1 || input.ExpiresHours > 168 {
		h.fail(w, r, http.StatusBadRequest, "invitation_invalid", "invitation expiry must be between 1 and 168 hours", nil)
		return
	}
	if (input.AccountType == "campus" && !validRole(input.Role)) || (input.AccountType == "external" && !validExternal(input.ExternalAccess)) || (input.AccountType != "campus" && input.AccountType != "external") {
		h.fail(w, r, http.StatusBadRequest, "invitation_invalid", "invitation type or role is invalid", nil)
		return
	}
	if input.AccountType == "external" && input.OrganizationID != "" {
		h.fail(w, r, http.StatusBadRequest, "invitation_invalid", "external invitations cannot grant campus organization membership", nil)
		return
	}
	if input.Role == "campus_admin" {
		h.fail(w, r, http.StatusForbidden, "role_bootstrap_forbidden", "campus administrator roles must be granted through the audited role workflow", nil)
		return
	}
	if input.OrganizationID != "" {
		var exists bool
		if err = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM organizations WHERE id=$1 AND college_id=$2)`, input.OrganizationID, s.CollegeID).Scan(&exists); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if !exists {
			h.fail(w, r, http.StatusNotFound, "organization_not_found", "organization not found", nil)
			return
		}
	}
	tokenBytes := make([]byte, 32)
	if _, err = rand.Read(tokenBytes); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(token))
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO invitations(college_id,email_normalized,account_type,role,external_access_kind,organization_id,token_hash,invited_by,expires_at)
		VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,'')::uuid,$7,$8,now()+($9::int * interval '1 hour')) RETURNING id::text`, s.CollegeID, email, input.AccountType, input.Role, input.ExternalAccess, input.OrganizationID, hash[:], s.UserID, input.ExpiresHours).Scan(&id)
	if err == nil {
		err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "invitation.created", ResourceType: "invitation", ResourceID: id})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	link := strings.TrimRight(h.origin, "/") + "/api/v1/auth/github?invite=" + url.QueryEscape(token)
	_ = response.Created(w, map[string]any{"id": id, "email": email, "expires_hours": input.ExpiresHours, "invitation_url": link})
}

func (h *Handler) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	ct, err := tx.Exec(r.Context(), `UPDATE invitations SET revoked_at=now() WHERE id=$1 AND college_id=$2 AND revoked_at IS NULL AND accepted_at IS NULL`, id, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if ct.RowsAffected() != 1 {
		h.fail(w, r, http.StatusNotFound, "invitation_not_found", "invitation not found or already used", nil)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "invitation.revoked", ResourceType: "invitation", ResourceID: id}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) createOrganization(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	input, err := request.Decode[struct {
		Kind        string `json:"kind"`
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if !validOrganization(input.Kind) || !validSlug(input.Slug) || len(input.Name) < 2 || len(input.Name) > 160 || len(input.Description) > 2000 {
		h.fail(w, r, http.StatusBadRequest, "organization_invalid", "organization details are invalid", nil)
		return
	}
	var id string
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	err = tx.QueryRow(r.Context(), `INSERT INTO organizations(college_id,kind,slug,name,description) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, s.CollegeID, input.Kind, input.Slug, input.Name, input.Description).Scan(&id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			h.fail(w, r, http.StatusConflict, "organization_slug_taken", "an organization with this slug already exists", err)
		} else {
			h.err.Handle(w, r, err)
		}
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "organization.created", ResourceType: "organization", ResourceID: id}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "kind": input.Kind, "slug": input.Slug, "name": input.Name})
}

func (h *Handler) grantRole(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	input, err := request.Decode[struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !validRole(input.Role) || input.Role == "campus_admin" {
		h.fail(w, r, http.StatusBadRequest, "role_invalid", "role is not grantable through this endpoint", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var exists bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND college_id=$2 AND status='active')`, input.UserID, s.CollegeID).Scan(&exists); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !exists {
		h.fail(w, r, http.StatusNotFound, "user_not_found", "campus user not found", nil)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO campus_roles(college_id,user_id,role,granted_by) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, s.CollegeID, input.UserID, input.Role, s.UserID)
	if err == nil {
		err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "campus_role.granted", ResourceType: "user", ResourceID: input.UserID, Details: jsonDetails("role", input.Role)})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"user_id": input.UserID, "role": input.Role})
}

func (h *Handler) users(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	cursor := r.URL.Query().Get("cursor")
	if len(query) > 80 || cursor != "" && !validUUID(cursor) {
		h.fail(w, r, http.StatusBadRequest, "query_invalid", "user query or cursor is invalid", nil)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT u.id::text,u.email_normalized,u.display_name,u.status,
		COALESCE(array_agg(cr.role ORDER BY cr.role) FILTER(WHERE cr.role IS NOT NULL),'{}'::text[])
		FROM users u LEFT JOIN campus_roles cr ON cr.user_id=u.id AND cr.college_id=u.college_id
		WHERE u.college_id=$1 AND ($2='' OR u.email_normalized ILIKE '%'||$2||'%' OR u.display_name ILIKE '%'||$2||'%')
		AND ($3::uuid IS NULL OR u.id<$3::uuid) GROUP BY u.id ORDER BY u.id DESC LIMIT 51`, s.CollegeID, query, nullIfEmpty(cursor))
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID     string   `json:"id"`
		Email  string   `json:"email"`
		Name   string   `json:"display_name"`
		Status string   `json:"status"`
		Roles  []string `json:"roles"`
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.ID, &v.Email, &v.Name, &v.Status, &v.Roles); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	next := ""
	if len(items) > 50 {
		next = items[49].ID
		items = items[:50]
	}
	_ = response.OK(w, map[string]any{"items": items, "next_cursor": next})
}

func (h *Handler) setUserStatus(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		h.fail(w, r, http.StatusBadRequest, "user_id_invalid", "user ID is invalid", nil)
		return
	}
	if id == s.UserID {
		h.fail(w, r, http.StatusConflict, "self_suspension_forbidden", "administrators cannot suspend their own account", nil)
		return
	}
	input, err := request.Decode[struct {
		Suspended bool `json:"suspended"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var exists, isAdmin bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND college_id=$2),EXISTS(SELECT 1 FROM campus_roles WHERE user_id=$1 AND college_id=$2 AND role='campus_admin')`, id, s.CollegeID).Scan(&exists, &isAdmin); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !exists {
		h.fail(w, r, http.StatusNotFound, "user_not_found", "campus user not found", nil)
		return
	}
	if isAdmin && input.Suspended {
		h.fail(w, r, http.StatusConflict, "admin_suspension_requires_review", "administrator suspension requires the documented break-glass procedure", nil)
		return
	}
	status, action := "active", "user.reactivated"
	if input.Suspended {
		status, action = "suspended", "user.suspended"
	}
	if _, err = tx.Exec(r.Context(), `UPDATE users SET status=$2,updated_at=now() WHERE id=$1 AND college_id=$3`, id, status, s.CollegeID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if input.Suspended {
		if _, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id); err != nil {
			h.err.Handle(w, r, err)
			return
		}
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: action, ResourceType: "user", ResourceID: id}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]string{"user_id": id, "status": status})
}

func (h *Handler) admin(w http.ResponseWriter, r *http.Request) (identity.Session, bool) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
		return s, false
	}
	if s.CollegeID == "" || !s.CampusAdmin {
		h.fail(w, r, http.StatusForbidden, "campus_admin_required", "campus administrator access is required", nil)
		return s, false
	}
	if !s.TermsAccepted || !s.PrivacyAccepted {
		h.fail(w, r, http.StatusForbidden, "policy_consent_required", "accept the Terms and Privacy Notice to continue", nil)
		return s, false
	}
	if !s.MFAEnabled || !s.MFAFresh {
		h.fail(w, r, http.StatusForbidden, "mfa_required", "verify multi-factor authentication to continue", nil)
		return s, false
	}
	return s, true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, status int, code, message string, cause error) {
	h.err.Handle(w, r, errormanager.New(status, code, message, cause))
}
func validRole(role string) bool {
	switch role {
	case "student", "faculty", "alumni_mentor", "moderator", "campus_admin":
		return true
	}
	return false
}
func validExternal(kind string) bool {
	switch kind {
	case "mentor", "recruiter", "organization_guest":
		return true
	}
	return false
}
func validOrganization(kind string) bool {
	switch kind {
	case "department", "club", "innovation_cell", "placement_cell", "event_body":
		return true
	}
	return false
}
func validSlug(s string) bool {
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
func validUUID(s string) bool {
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
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func jsonDetails(key, value string) []byte {
	b, _ := json.Marshal(map[string]string{key: value})
	return b
}
