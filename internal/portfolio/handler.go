package portfolio

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
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
)

type Handler struct {
	pool *pgxpool.Pool
	auth *identity.Handler
	err  *errormanager.Manager
}

func NewHandler(pool *pgxpool.Pool, auth *identity.Handler, errors *errormanager.Manager) *Handler {
	return &Handler{pool: pool, auth: auth, err: errors}
}

func (h *Handler) Register(r chi.Router) {
	r.Group(func(api chi.Router) {
		api.Use(h.auth.Middleware)
		api.Get("/portfolio/{handle}", h.publicPortfolio)
		api.Get("/portfolio/share/{token}", h.sharedPortfolio)
		api.Get("/me/portfolio", h.getPortfolio)
		api.With(h.auth.RequireCSRF).Put("/me/portfolio/items", h.saveItems)
		api.With(h.auth.RequireCSRF).Post("/me/portfolio/shares", h.createShare)
		api.With(h.auth.RequireCSRF).Delete("/me/portfolio/shares/{id}", h.revokeShare)
		api.Get("/me/handovers", h.handovers)
		api.Get("/me/handoverable", h.handoverable)
		api.Get("/projects/{id}/handover/eligible", h.eligibleProjectSuccessors)
		api.Get("/organizations/{id}/handover/eligible", h.eligibleOrganizationSuccessors)
		api.With(h.auth.RequireCSRF).Post("/projects/{id}/handover", h.proposeProjectHandover)
		api.With(h.auth.RequireCSRF).Post("/organizations/{id}/handover", h.proposeOrganizationHandover)
		api.With(h.auth.RequireCSRF).Post("/handovers/{id}/accept", h.acceptHandover)
		api.With(h.auth.RequireCSRF).Post("/handovers/{id}/decline", h.declineHandover)
		api.With(h.auth.RequireCSRF).Post("/handovers/{id}/cancel", h.cancelHandover)
	})
}

type portfolioItem struct {
	ProjectID string `json:"project_id"`
	Caption   string `json:"caption"`
}

func (h *Handler) getPortfolio(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	var consent bool
	var handle string
	if err := h.pool.QueryRow(r.Context(), `SELECT u.handle,EXISTS(SELECT 1 FROM consent_records WHERE user_id=u.id AND purpose='portfolio' AND revoked_at IS NULL) FROM users u WHERE u.id=$1`, s.UserID).Scan(&handle, &consent); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT pi.project_id::text,pi.caption,pi.sort_order,p.title,p.summary,p.visibility,p.lifecycle,
		COALESCE(pm.role,'member'),EXISTS(SELECT 1 FROM repositories rp JOIN repository_bindings rb ON rb.repository_id=rp.id AND rb.is_current AND rb.access_state='active' WHERE rp.project_id=p.id)
		FROM portfolio_items pi JOIN projects p ON p.id=pi.project_id AND p.college_id=pi.college_id
		LEFT JOIN project_members pm ON pm.project_id=p.id AND pm.user_id=pi.user_id
		WHERE pi.user_id=$1 AND pi.college_id=$2 ORDER BY pi.sort_order,pi.created_at,pi.id`, s.UserID, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, caption, title, summary, visibility, lifecycle, role string
		var sortOrder int
		var sourceActive bool
		if err = rows.Scan(&id, &caption, &sortOrder, &title, &summary, &visibility, &lifecycle, &role, &sourceActive); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"project_id": id, "caption": caption, "sort_order": sortOrder, "title": title, "summary": summary, "visibility": visibility, "lifecycle": lifecycle, "role": role, "source_active": sourceActive})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	rows.Close()
	shares, err := h.pool.Query(r.Context(), `SELECT id::text,audience,expires_at,revoked_at,created_at FROM portfolio_share_grants WHERE user_id=$1 AND college_id=$2 ORDER BY created_at DESC LIMIT 50`, s.UserID, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer shares.Close()
	links := make([]map[string]any, 0)
	for shares.Next() {
		var id, audience string
		var expiresAt, createdAt time.Time
		var revokedAt *time.Time
		if err = shares.Scan(&id, &audience, &expiresAt, &revokedAt, &createdAt); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		links = append(links, map[string]any{"id": id, "audience": audience, "expires_at": expiresAt, "revoked_at": revokedAt, "created_at": createdAt})
	}
	if err = shares.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"handle": handle, "consented": consent, "items": items, "shares": links})
}

func (h *Handler) saveItems(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	input, err := request.Decode[struct {
		Items []portfolioItem `json:"items"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if len(input.Items) > 50 {
		h.fail(w, r, http.StatusBadRequest, "portfolio_limit", "select up to 50 projects", nil)
		return
	}
	seen := make(map[string]bool, len(input.Items))
	for _, item := range input.Items {
		if !uuidOK(item.ProjectID) || len(item.Caption) > 500 || seen[item.ProjectID] {
			h.fail(w, r, http.StatusBadRequest, "portfolio_items_invalid", "portfolio project selections are invalid", nil)
			return
		}
		seen[item.ProjectID] = true
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `DELETE FROM portfolio_items WHERE user_id=$1 AND college_id=$2`, s.UserID, s.CollegeID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	for index, item := range input.Items {
		var inserted int
		err = tx.QueryRow(r.Context(), `INSERT INTO portfolio_items(college_id,user_id,project_id,caption,sort_order)
			SELECT $1,$2,p.id,$4,$5 FROM projects p JOIN project_members pm ON pm.project_id=p.id AND pm.user_id=$2
			WHERE p.id=$3 AND p.college_id=$1 AND p.project_context='ordinary' AND p.github_revoke_deadline IS NULL AND p.lifecycle<>'archived' RETURNING 1`,
			s.CollegeID, s.UserID, item.ProjectID, strings.TrimSpace(item.Caption), index).Scan(&inserted)
		if errors.Is(err, pgx.ErrNoRows) {
			h.fail(w, r, http.StatusConflict, "portfolio_project_unavailable", "one or more selected projects are no longer available to you", nil)
			return
		}
		if err != nil {
			h.err.Handle(w, r, err)
			return
		}
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "portfolio.updated", ResourceType: "portfolio", ResourceID: s.UserID}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"saved": true, "count": len(input.Items)})
}

func (h *Handler) createShare(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	input, err := request.Decode[struct {
		Audience string `json:"audience"`
		Days     int    `json:"days"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if input.Audience != "public" && input.Audience != "campus" || input.Days < 1 || input.Days > 90 {
		h.fail(w, r, http.StatusBadRequest, "share_invalid", "choose a public or campus link that expires within 90 days", nil)
		return
	}
	var consent bool
	if err = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM consent_records WHERE user_id=$1 AND purpose='portfolio' AND revoked_at IS NULL)`, s.UserID).Scan(&consent); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !consent {
		h.fail(w, r, http.StatusForbidden, "portfolio_consent_required", "enable portfolio consent before creating a share link", nil)
		return
	}
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	hash := sha256.Sum256([]byte(token))
	var id string
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	err = tx.QueryRow(r.Context(), `INSERT INTO portfolio_share_grants(college_id,user_id,token_hash,audience,expires_at)
		VALUES($1,$2,$3,$4,now()+($5::int * interval '1 day')) RETURNING id::text`, s.CollegeID, s.UserID, hash[:], input.Audience, input.Days).Scan(&id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "portfolio.share.created", ResourceType: "portfolio_share", ResourceID: id}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "url": "/portfolio/share/" + token, "audience": input.Audience})
}

func (h *Handler) revokeShare(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), `UPDATE portfolio_share_grants SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND college_id=$3 AND revoked_at IS NULL`, chi.URLParam(r, "id"), s.UserID, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if tag.RowsAffected() != 1 {
		h.fail(w, r, http.StatusNotFound, "share_not_found", "active share link not found", nil)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "portfolio.share.revoked", ResourceType: "portfolio_share", ResourceID: chi.URLParam(r, "id")}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) publicPortfolio(w http.ResponseWriter, r *http.Request) {
	h.portfolioView(w, r, chi.URLParam(r, "handle"), "public", "")
}

func (h *Handler) sharedPortfolio(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	token := chi.URLParam(r, "token")
	if len(token) < 40 || len(token) > 60 {
		h.notFound(w)
		return
	}
	hash := sha256.Sum256([]byte(token))
	var handle, audience, collegeID string
	err := h.pool.QueryRow(r.Context(), `SELECT u.handle,g.audience,g.college_id::text FROM portfolio_share_grants g JOIN users u ON u.id=g.user_id
		WHERE g.token_hash=$1 AND g.revoked_at IS NULL AND g.expires_at>now() AND u.status='active'
		AND EXISTS(SELECT 1 FROM consent_records c WHERE c.user_id=u.id AND c.purpose='portfolio' AND c.revoked_at IS NULL)`, hash[:]).Scan(&handle, &audience, &collegeID)
	if errors.Is(err, pgx.ErrNoRows) {
		h.notFound(w)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	s, authenticated := identity.CurrentSession(r.Context())
	if audience == "campus" && (!authenticated || s.CollegeID == "" || s.CollegeID != collegeID || !s.TermsAccepted || !s.PrivacyAccepted) {
		h.fail(w, r, http.StatusForbidden, "campus_access_required", "this portfolio link is limited to verified members of its campus", nil)
		return
	}
	h.portfolioView(w, r, handle, audience, collegeID)
}

func (h *Handler) portfolioView(w http.ResponseWriter, r *http.Request, handle, audience, grantCollege string) {
	w.Header().Set("Cache-Control", "private, no-store")
	var userID, collegeID, displayName, headline, bio string
	err := h.pool.QueryRow(r.Context(), `SELECT u.id::text,u.college_id::text,u.display_name,COALESCE(u.profile->>'headline',''),COALESCE(u.profile->>'bio','')
		FROM users u WHERE u.handle=$1 AND u.status='active' AND EXISTS(SELECT 1 FROM consent_records c WHERE c.user_id=u.id AND c.purpose='portfolio' AND c.revoked_at IS NULL)`, handle).Scan(&userID, &collegeID, &displayName, &headline, &bio)
	if errors.Is(err, pgx.ErrNoRows) {
		h.notFound(w)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if audience == "campus" && (grantCollege == "" || grantCollege != collegeID) {
		h.notFound(w)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT p.id::text,p.title,p.summary,pi.caption,pm.role
		FROM portfolio_items pi JOIN projects p ON p.id=pi.project_id AND p.college_id=pi.college_id
		JOIN project_members pm ON pm.project_id=p.id AND pm.user_id=pi.user_id
		WHERE pi.user_id=$1 AND pi.college_id=$2 AND p.project_context='ordinary' AND p.github_revoke_deadline IS NULL AND p.lifecycle='active'
		AND ($3='public' AND p.visibility='public' AND EXISTS(SELECT 1 FROM repositories rp JOIN repository_bindings rb ON rb.repository_id=rp.id AND rb.is_current AND rb.access_state='active' WHERE rp.project_id=p.id)
		 OR $3='campus' AND p.visibility IN ('campus','public'))
		ORDER BY pi.sort_order,pi.created_at,pi.id`, userID, collegeID, audience)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	type item struct {
		ProjectID string `json:"project_id"`
		Title     string `json:"title"`
		Summary   string `json:"summary"`
		Caption   string `json:"caption"`
		Role      string `json:"role"`
	}
	items := make([]item, 0)
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.ProjectID, &v.Title, &v.Summary, &v.Caption, &v.Role); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if audience == "public" && len(items) == 0 {
		h.notFound(w)
		return
	}
	_ = response.OK(w, map[string]any{"handle": handle, "display_name": displayName, "headline": headline, "bio": bio, "items": items, "audience": audience})
}

type handoverItem struct {
	ID         string    `json:"id"`
	Resource   string    `json:"resource_type"`
	ResourceID string    `json:"resource_id"`
	Title      string    `json:"title"`
	FromUserID string    `json:"from_user_id"`
	ToUserID   string    `json:"to_user_id"`
	State      string    `json:"state"`
	Provider   string    `json:"provider_state"`
	Message    string    `json:"message"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	Recipient  bool      `json:"recipient"`
}

func (h *Handler) handovers(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT hr.id::text,hr.resource_type,COALESCE(hr.project_id,hr.organization_id)::text,
		COALESCE(p.title,o.name),hr.from_user_id::text,hr.to_user_id::text,hr.state,hr.provider_state,hr.message,hr.expires_at,hr.created_at,hr.to_user_id=$2
		FROM handover_requests hr LEFT JOIN projects p ON p.id=hr.project_id LEFT JOIN organizations o ON o.id=hr.organization_id
		WHERE hr.college_id=$1 AND (hr.from_user_id=$2 OR hr.to_user_id=$2) ORDER BY hr.created_at DESC LIMIT 100`, s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]handoverItem, 0)
	for rows.Next() {
		var v handoverItem
		if err = rows.Scan(&v.ID, &v.Resource, &v.ResourceID, &v.Title, &v.FromUserID, &v.ToUserID, &v.State, &v.Provider, &v.Message, &v.ExpiresAt, &v.CreatedAt, &v.Recipient); err != nil {
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

func (h *Handler) handoverable(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT 'project',p.id::text,p.title FROM projects p
		JOIN project_members pm ON pm.project_id=p.id AND pm.user_id=$2 AND pm.role='owner'
		WHERE p.college_id=$1 AND p.project_context='ordinary' AND p.github_revoke_deadline IS NULL
		UNION ALL
		SELECT 'organization',o.id::text,o.name FROM organizations o WHERE o.college_id=$1 AND o.owner_user_id=$2
		ORDER BY 1,3`, s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]string, 0)
	for rows.Next() {
		var kind, id, title string
		if err = rows.Scan(&kind, &id, &title); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]string{"resource_type": kind, "resource_id": id, "title": title})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) eligibleProjectSuccessors(w http.ResponseWriter, r *http.Request) {
	h.eligible(w, r, "project")
}

func (h *Handler) eligibleOrganizationSuccessors(w http.ResponseWriter, r *http.Request) {
	h.eligible(w, r, "organization")
}

func (h *Handler) eligible(w http.ResponseWriter, r *http.Request, kind string) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	var canManage bool
	if kind == "project" {
		err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM project_members pm JOIN projects p ON p.id=pm.project_id AND p.college_id=pm.college_id AND p.project_context='ordinary' WHERE pm.project_id=$1 AND pm.college_id=$2 AND pm.user_id=$3 AND pm.role='owner')`, id, s.CollegeID, s.UserID).Scan(&canManage)
		if err != nil {
			h.err.Handle(w, r, err)
			return
		}
	} else {
		err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM organizations WHERE id=$1 AND college_id=$2 AND owner_user_id=$3)`, id, s.CollegeID, s.UserID).Scan(&canManage)
		if err != nil {
			h.err.Handle(w, r, err)
			return
		}
	}
	if !canManage {
		h.notFound(w)
		return
	}
	query := `SELECT u.id::text,u.display_name,u.handle FROM users u WHERE u.college_id=$1 AND u.status='active' AND u.id<>$2
		AND EXISTS(SELECT 1 FROM campus_verifications v WHERE v.college_id=$1 AND v.user_id=u.id AND v.revoked_at IS NULL)
		AND EXISTS(SELECT 1 FROM consent_records t WHERE t.user_id=u.id AND t.purpose='terms' AND t.revoked_at IS NULL)
		AND EXISTS(SELECT 1 FROM consent_records p WHERE p.user_id=u.id AND p.purpose='privacy' AND p.revoked_at IS NULL)`
	if kind == "project" {
		query += ` AND EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=$3 AND pm.college_id=$1 AND pm.user_id=u.id)`
	} else {
		query += ` AND EXISTS(SELECT 1 FROM organization_members om WHERE om.organization_id=$3 AND om.college_id=$1 AND om.user_id=u.id)`
	}
	query += ` ORDER BY u.display_name,u.id LIMIT 100`
	rows, err := h.pool.Query(r.Context(), query, s.CollegeID, s.UserID, id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]string, 0)
	for rows.Next() {
		var id, name, handle string
		if err = rows.Scan(&id, &name, &handle); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]string{"user_id": id, "display_name": name, "handle": handle})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) proposeProjectHandover(w http.ResponseWriter, r *http.Request) {
	h.proposeHandover(w, r, "project")
}

func (h *Handler) proposeOrganizationHandover(w http.ResponseWriter, r *http.Request) {
	h.proposeHandover(w, r, "organization")
}

func (h *Handler) proposeHandover(w http.ResponseWriter, r *http.Request, kind string) {
	s, ok := h.campusSession(w, r)
	if !ok || !h.freshAuth(w, r, s) {
		return
	}
	input, err := request.Decode[struct {
		ToUserID string `json:"to_user_id"`
		Message  string `json:"message"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	input.Message = strings.TrimSpace(input.Message)
	if !uuidOK(input.ToUserID) || input.ToUserID == s.UserID || len(input.Message) > 500 {
		h.fail(w, r, http.StatusBadRequest, "handover_invalid", "choose a different eligible member and a message under 500 characters", nil)
		return
	}
	resourceID := chi.URLParam(r, "id")
	var id string
	var providerState string
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `UPDATE handover_requests SET state='expired',responded_at=now()
		WHERE college_id=$1 AND state='pending' AND expires_at<=now() AND
		(($2='project' AND resource_type='project' AND project_id=$3) OR ($2='organization' AND resource_type='organization' AND organization_id=$3))`, s.CollegeID, kind, resourceID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if kind == "project" {
		err = tx.QueryRow(r.Context(), `INSERT INTO handover_requests(college_id,resource_type,project_id,from_user_id,to_user_id,message,provider_state)
			SELECT $1,'project',p.id,$2,$3,$4,CASE WHEN EXISTS(SELECT 1 FROM repository_bindings rb JOIN repositories rp ON rp.id=rb.repository_id AND rp.project_id=p.id WHERE rb.is_current AND rb.access_state='active') THEN 'upstream_unchanged'
			WHEN EXISTS(SELECT 1 FROM repositories rp WHERE rp.project_id=p.id) THEN 'source_unavailable' ELSE 'not_applicable' END
			FROM projects p JOIN project_members owner ON owner.project_id=p.id AND owner.user_id=$2 AND owner.role='owner'
			JOIN project_members successor ON successor.project_id=p.id AND successor.user_id=$3
			WHERE p.id=$5 AND p.college_id=$1 AND p.project_context='ordinary' AND p.github_revoke_deadline IS NULL
			AND EXISTS(SELECT 1 FROM users u WHERE u.id=$3 AND u.college_id=$1 AND u.status='active' AND EXISTS(SELECT 1 FROM campus_verifications v WHERE v.user_id=u.id AND v.college_id=$1 AND v.revoked_at IS NULL))
			AND EXISTS(SELECT 1 FROM consent_records c WHERE c.user_id=$3 AND c.purpose IN ('terms','privacy') AND c.revoked_at IS NULL GROUP BY c.user_id HAVING count(DISTINCT c.purpose)=2)
			RETURNING id::text,provider_state`, s.CollegeID, s.UserID, input.ToUserID, input.Message, resourceID).Scan(&id, &providerState)
	} else {
		err = tx.QueryRow(r.Context(), `INSERT INTO handover_requests(college_id,resource_type,organization_id,from_user_id,to_user_id,message)
			SELECT $1,'organization',o.id,$2,$3,$4 FROM organizations o JOIN organization_members owner ON owner.organization_id=o.id AND owner.user_id=$2 AND owner.role='owner'
			JOIN organization_members successor ON successor.organization_id=o.id AND successor.user_id=$3
			WHERE o.id=$5 AND o.college_id=$1 AND o.owner_user_id=$2
			AND EXISTS(SELECT 1 FROM users u WHERE u.id=$3 AND u.college_id=$1 AND u.status='active' AND EXISTS(SELECT 1 FROM campus_verifications v WHERE v.user_id=u.id AND v.college_id=$1 AND v.revoked_at IS NULL))
			AND EXISTS(SELECT 1 FROM consent_records c WHERE c.user_id=$3 AND c.purpose IN ('terms','privacy') AND c.revoked_at IS NULL GROUP BY c.user_id HAVING count(DISTINCT c.purpose)=2)
			RETURNING id::text,provider_state`, s.CollegeID, s.UserID, input.ToUserID, input.Message, resourceID).Scan(&id, &providerState)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, http.StatusConflict, "handover_ineligible", "the project, organization, owner, or successor is no longer eligible", nil)
		return
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			h.fail(w, r, http.StatusConflict, "handover_pending", "a handover is already pending for this item", err)
		} else {
			h.err.Handle(w, r, err)
		}
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: kind + ".handover.proposed", ResourceType: kind + "_handover", ResourceID: id}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "state": "pending", "provider_state": providerState})
}

func (h *Handler) acceptHandover(w http.ResponseWriter, r *http.Request) {
	h.decideHandover(w, r, true)
}

func (h *Handler) declineHandover(w http.ResponseWriter, r *http.Request) {
	h.decideHandover(w, r, false)
}

func (h *Handler) decideHandover(w http.ResponseWriter, r *http.Request, accept bool) {
	s, ok := h.campusSession(w, r)
	if !ok || !h.freshAuth(w, r, s) {
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var item handoverItem
	err = tx.QueryRow(r.Context(), `SELECT id::text,resource_type,COALESCE(project_id,organization_id)::text,from_user_id::text,to_user_id::text,state,provider_state
		FROM handover_requests WHERE id=$1 AND college_id=$2 AND expires_at>now() FOR UPDATE`, chi.URLParam(r, "id"), s.CollegeID).Scan(&item.ID, &item.Resource, &item.ResourceID, &item.FromUserID, &item.ToUserID, &item.State, &item.Provider)
	if errors.Is(err, pgx.ErrNoRows) || item.ToUserID != s.UserID || item.State != "pending" {
		h.fail(w, r, http.StatusNotFound, "handover_not_found", "pending handover not found", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !accept {
		_, err = tx.Exec(r.Context(), `UPDATE handover_requests SET state='declined',responded_at=now() WHERE id=$1`, item.ID)
		if err == nil {
			err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: item.Resource + ".handover.declined", ResourceType: item.Resource + "_handover", ResourceID: item.ID})
		}
		if err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		response.NoContent(w)
		return
	}
	if item.Resource == "project" {
		var activeSource, boundSource bool
		err = tx.QueryRow(r.Context(), `SELECT
			EXISTS(SELECT 1 FROM repository_bindings rb JOIN repositories rp ON rp.id=rb.repository_id
				WHERE rp.project_id=$1 AND rb.is_current AND rb.access_state='active'),
			EXISTS(SELECT 1 FROM repositories rp WHERE rp.project_id=$1)`, item.ResourceID).Scan(&activeSource, &boundSource)
		if err != nil {
			h.err.Handle(w, r, err)
			return
		}
		switch {
		case activeSource:
			item.Provider = "upstream_unchanged"
		case boundSource:
			item.Provider = "source_unavailable"
		default:
			item.Provider = "not_applicable"
		}
		var oldRole string
		_ = tx.QueryRow(r.Context(), `SELECT role FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$3 FOR UPDATE`, item.ResourceID, s.CollegeID, item.FromUserID).Scan(&oldRole)
		_ = tx.QueryRow(r.Context(), `SELECT role FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$3 FOR UPDATE`, item.ResourceID, s.CollegeID, s.UserID).Scan(new(string))
		var currentOwner, eligibleMember, eligibleUser bool
		err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM project_members pm JOIN projects p ON p.id=pm.project_id AND p.college_id=pm.college_id AND p.project_context='ordinary' WHERE pm.project_id=$1 AND pm.college_id=$2 AND pm.user_id=$3 AND pm.role='owner'),
			EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$4),
			EXISTS(SELECT 1 FROM users u WHERE u.id=$3 AND u.college_id=$2 AND u.status='active' AND EXISTS(SELECT 1 FROM campus_verifications v WHERE v.user_id=u.id AND v.college_id=$2 AND v.revoked_at IS NULL)
			AND EXISTS(SELECT 1 FROM consent_records c WHERE c.user_id=u.id AND c.purpose IN ('terms','privacy') AND c.revoked_at IS NULL GROUP BY c.user_id HAVING count(DISTINCT c.purpose)=2))`, item.ResourceID, s.CollegeID, item.FromUserID, s.UserID).Scan(&currentOwner, &eligibleMember, &eligibleUser)
		if err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if !currentOwner || oldRole != "owner" || !eligibleMember || !eligibleUser {
			h.markConflict(w, r, tx, s, item)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE project_members SET role='maintainer' WHERE project_id=$1 AND college_id=$2 AND user_id=$3 AND role='owner'`, item.ResourceID, s.CollegeID, item.FromUserID); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE project_members SET role='owner' WHERE project_id=$1 AND college_id=$2 AND user_id=$3`, item.ResourceID, s.CollegeID, s.UserID); err != nil {
			h.err.Handle(w, r, err)
			return
		}
	} else {
		var currentOwner string
		_ = tx.QueryRow(r.Context(), `SELECT owner_user_id::text FROM organizations WHERE id=$1 AND college_id=$2 FOR UPDATE`, item.ResourceID, s.CollegeID).Scan(&currentOwner)
		var eligible bool
		err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM organizations o JOIN organization_members om ON om.organization_id=o.id AND om.user_id=$4
			JOIN organization_members owner ON owner.organization_id=o.id AND owner.user_id=$3 AND owner.role='owner'
			WHERE o.id=$1 AND o.college_id=$2 AND o.owner_user_id=$3)
			AND EXISTS(SELECT 1 FROM users u WHERE u.id=$4 AND u.college_id=$2 AND u.status='active' AND EXISTS(SELECT 1 FROM campus_verifications v WHERE v.user_id=u.id AND v.college_id=$2 AND v.revoked_at IS NULL)
			AND EXISTS(SELECT 1 FROM consent_records c WHERE c.user_id=u.id AND c.purpose IN ('terms','privacy') AND c.revoked_at IS NULL GROUP BY c.user_id HAVING count(DISTINCT c.purpose)=2))`, item.ResourceID, s.CollegeID, item.FromUserID, s.UserID).Scan(&eligible)
		if err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if !eligible || currentOwner != item.FromUserID {
			h.markConflict(w, r, tx, s, item)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE organizations SET owner_user_id=$3 WHERE id=$1 AND college_id=$2 AND owner_user_id=$4`, item.ResourceID, s.CollegeID, s.UserID, item.FromUserID); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE organization_members SET role='manager' WHERE organization_id=$1 AND college_id=$2 AND user_id=$3`, item.ResourceID, s.CollegeID, item.FromUserID); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE organization_members SET role='owner' WHERE organization_id=$1 AND college_id=$2 AND user_id=$3`, item.ResourceID, s.CollegeID, s.UserID); err != nil {
			h.err.Handle(w, r, err)
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE handover_requests SET state='accepted',provider_state=$2,responded_at=now() WHERE id=$1 AND state='pending'`, item.ID, item.Provider); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: item.Resource + ".handover.accepted", ResourceType: item.Resource + "_handover", ResourceID: item.ID}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"state": "accepted", "provider_state": item.Provider})
}

func (h *Handler) markConflict(w http.ResponseWriter, r *http.Request, tx pgx.Tx, s identity.Session, item handoverItem) {
	if _, err := tx.Exec(r.Context(), `UPDATE handover_requests SET state='conflicted',responded_at=now() WHERE id=$1`, item.ID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err := audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: item.Resource + ".handover.conflicted", ResourceType: item.Resource + "_handover", ResourceID: item.ID}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	h.fail(w, r, http.StatusConflict, "handover_conflict", "membership or ownership changed before acceptance; review eligibility and send a new request", nil)
}

func (h *Handler) cancelHandover(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), `UPDATE handover_requests SET state='cancelled',responded_at=now() WHERE id=$1 AND college_id=$2 AND from_user_id=$3 AND state='pending'`, chi.URLParam(r, "id"), s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if tag.RowsAffected() != 1 {
		h.fail(w, r, http.StatusNotFound, "handover_not_found", "pending handover not found", nil)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "handover.cancelled", ResourceType: "handover", ResourceID: chi.URLParam(r, "id")}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) campusSession(w http.ResponseWriter, r *http.Request) (identity.Session, bool) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
		return s, false
	}
	if s.CollegeID == "" || !s.TermsAccepted || !s.PrivacyAccepted {
		h.fail(w, r, http.StatusForbidden, "campus_access_required", "current verified campus access and policy consent are required", nil)
		return s, false
	}
	return s, true
}

func (h *Handler) freshAuth(w http.ResponseWriter, r *http.Request, s identity.Session) bool {
	if s.MFAEnabled {
		if s.MFAFresh {
			return true
		}
		h.fail(w, r, http.StatusForbidden, "fresh_auth_required", "verify MFA before changing ownership", nil)
		return false
	}
	var fresh bool
	if err := h.pool.QueryRow(r.Context(), `SELECT created_at>now()-interval '10 minutes' FROM sessions WHERE id=$1`, s.ID).Scan(&fresh); err != nil {
		h.err.Handle(w, r, err)
		return false
	}
	if !fresh {
		h.fail(w, r, http.StatusForbidden, "fresh_auth_required", "sign in again or verify MFA before changing ownership", nil)
	}
	return fresh
}

func (h *Handler) notFound(w http.ResponseWriter) {
	_ = response.WriteError(w, http.StatusNotFound, "portfolio_unavailable", "portfolio is unavailable")
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, status int, code, message string, cause error) {
	h.err.Handle(w, r, errormanager.New(status, code, message, cause))
}

func uuidOK(s string) bool {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return err == nil
}
