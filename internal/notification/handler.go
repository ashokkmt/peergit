package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"peergit/internal/identity"
	"peergit/internal/platform/http/response"
)

type Handler struct {
	pool *pgxpool.Pool
	auth *identity.Handler
}

func NewHandler(pool *pgxpool.Pool, auth *identity.Handler) *Handler {
	return &Handler{pool: pool, auth: auth}
}

func (h *Handler) Register(r chi.Router) {
	r.Group(func(api chi.Router) {
		api.Use(h.auth.Middleware)
		api.Get("/notifications", h.list)
		api.Get("/notifications/preferences", h.getPreferences)
		api.With(h.auth.RequireCSRF).Put("/notifications/preferences", h.putPreferences)
		api.With(h.auth.RequireCSRF).Post("/notifications/{id}/read", h.markRead)
		api.With(h.auth.RequireCSRF).Post("/notifications/read-all", h.markAllRead)
		api.Get("/notifications/stream", h.stream)
	})
}

func (h *Handler) current(w http.ResponseWriter, r *http.Request) (identity.Session, bool) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return identity.Session{}, false
	}
	if s.CollegeID == "" || !s.TermsAccepted || !s.PrivacyAccepted {
		http.Error(w, "verified campus access and current consent are required", http.StatusForbidden)
		return identity.Session{}, false
	}
	return s, true
}

type inboxItem struct {
	ID            string     `json:"id"`
	Sequence      int64      `json:"sequence"`
	Category      string     `json:"category"`
	EventType     string     `json:"event_type"`
	Title         string     `json:"title"`
	Body          string     `json:"body"`
	ProjectID     string     `json:"project_id,omitempty"`
	ApplicationID string     `json:"application_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ReadAt        *time.Time `json:"read_at,omitempty"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	s, ok := h.current(w, r)
	if !ok {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 100 {
			http.Error(w, "limit must be between 1 and 100", 400)
			return
		}
		limit = n
	}
	var after int64
	if raw := r.URL.Query().Get("after"); raw != "" {
		var parseErr error
		after, parseErr = strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || after < 0 {
			http.Error(w, "invalid notification cursor", http.StatusBadRequest)
			return
		}
	}
	unread := r.URL.Query().Get("filter") == "unread"
	rows, err := h.pool.Query(r.Context(), `SELECT n.id::text,n.sequence,n.category,n.event_type,n.title,n.body,COALESCE(n.project_id::text,''),COALESCE(n.application_id::text,''),n.created_at,n.read_at
		FROM notifications n WHERE n.college_id=$1 AND n.user_id=$2 AND n.sequence<$3 AND (NOT $4 OR n.read_at IS NULL)
		AND ((n.project_id IS NULL OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=n.project_id AND m.college_id=n.college_id AND m.user_id=n.user_id)) OR (n.application_id IS NOT NULL AND EXISTS(SELECT 1 FROM applications a WHERE a.id=n.application_id AND a.college_id=n.college_id AND a.applicant_user_id=n.user_id)))
		AND (n.application_id IS NULL OR EXISTS(SELECT 1 FROM applications a WHERE a.id=n.application_id AND a.college_id=n.college_id AND a.applicant_user_id=n.user_id) OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=n.project_id AND m.user_id=n.user_id))
		ORDER BY n.sequence DESC LIMIT $5`, s.CollegeID, s.UserID, afterOrMax(after), unread, limit+1)
	if err != nil {
		http.Error(w, "notifications unavailable", http.StatusServiceUnavailable)
		return
	}
	defer rows.Close()
	items := make([]inboxItem, 0, limit)
	for rows.Next() {
		var i inboxItem
		if err = rows.Scan(&i.ID, &i.Sequence, &i.Category, &i.EventType, &i.Title, &i.Body, &i.ProjectID, &i.ApplicationID, &i.CreatedAt, &i.ReadAt); err != nil {
			http.Error(w, "notifications unavailable", http.StatusServiceUnavailable)
			return
		}
		items = append(items, i)
	}
	if rows.Err() != nil {
		http.Error(w, "notifications unavailable", http.StatusServiceUnavailable)
		return
	}
	next := int64(0)
	if len(items) > limit {
		next = items[limit-1].Sequence
		items = items[:limit]
	}
	var unreadCount int
	var last int64
	if err = h.pool.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM notifications n WHERE n.college_id=$1 AND n.user_id=$2 AND n.read_at IS NULL AND ((n.project_id IS NULL OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=n.project_id AND m.user_id=n.user_id)) OR (n.application_id IS NOT NULL AND EXISTS(SELECT 1 FROM applications a WHERE a.id=n.application_id AND a.applicant_user_id=n.user_id))) AND (n.application_id IS NULL OR EXISTS(SELECT 1 FROM applications a WHERE a.id=n.application_id AND a.applicant_user_id=n.user_id) OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=n.project_id AND m.user_id=n.user_id)))::int,COALESCE((SELECT last_sequence FROM notification_cursors WHERE college_id=$1 AND user_id=$2),0)`, s.CollegeID, s.UserID).Scan(&unreadCount, &last); err != nil {
		http.Error(w, "notifications unavailable", http.StatusServiceUnavailable)
		return
	}
	_ = response.OK(w, map[string]any{"items": items, "next_cursor": next, "unread_count": unreadCount, "last_sequence": last})
}

func afterOrMax(after int64) int64 {
	if after <= 0 {
		return int64(^uint64(0) >> 1)
	}
	return after
}

func (h *Handler) getPreferences(w http.ResponseWriter, r *http.Request) {
	s, ok := h.current(w, r)
	if !ok {
		return
	}
	_, err := h.pool.Exec(r.Context(), `INSERT INTO notification_preferences(college_id,user_id,category,email_enabled) VALUES($1,$2,'applications',true),($1,$2,'project_activity',false) ON CONFLICT DO NOTHING`, s.CollegeID, s.UserID)
	if err != nil {
		http.Error(w, "preferences unavailable", http.StatusServiceUnavailable)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT category,email_enabled FROM notification_preferences WHERE college_id=$1 AND user_id=$2 ORDER BY category`, s.CollegeID, s.UserID)
	if err != nil {
		http.Error(w, "preferences unavailable", http.StatusServiceUnavailable)
		return
	}
	defer rows.Close()
	items := map[string]bool{}
	for rows.Next() {
		var c string
		var enabled bool
		if rows.Scan(&c, &enabled) != nil {
			http.Error(w, "preferences unavailable", http.StatusServiceUnavailable)
			return
		}
		items[c] = enabled
	}
	_ = response.OK(w, map[string]any{"email": items, "in_app": true})
}

func (h *Handler) putPreferences(w http.ResponseWriter, r *http.Request) {
	s, ok := h.current(w, r)
	if !ok {
		return
	}
	var in struct {
		Email map[string]bool `json:"email"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || len(in.Email) != 2 {
		http.Error(w, "preferences are invalid", 400)
		return
	}
	for category, enabled := range in.Email {
		if category != "applications" && category != "project_activity" {
			http.Error(w, "preferences are invalid", 400)
			return
		}
		if category == "applications" && !enabled { // Applications are important, so email cannot be disabled.
			in.Email[category] = true
		}
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "preferences unavailable", http.StatusServiceUnavailable)
		return
	}
	defer tx.Rollback(r.Context())
	for c, v := range in.Email {
		if _, err = tx.Exec(r.Context(), `INSERT INTO notification_preferences(college_id,user_id,category,email_enabled) VALUES($1,$2,$3,$4) ON CONFLICT(college_id,user_id,category) DO UPDATE SET email_enabled=EXCLUDED.email_enabled,updated_at=clock_timestamp()`, s.CollegeID, s.UserID, c, v); err != nil {
			http.Error(w, "preferences unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		http.Error(w, "preferences unavailable", http.StatusServiceUnavailable)
		return
	}
	h.getPreferences(w, r)
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	s, ok := h.current(w, r)
	if !ok {
		return
	}
	tag, err := h.pool.Exec(r.Context(), `UPDATE notifications SET read_at=COALESCE(read_at,clock_timestamp()) WHERE id=$1 AND college_id=$2 AND user_id=$3 AND ((project_id IS NULL OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=notifications.project_id AND m.user_id=$3)) OR (application_id IS NOT NULL AND EXISTS(SELECT 1 FROM applications a WHERE a.id=notifications.application_id AND a.applicant_user_id=$3))) AND (application_id IS NULL OR EXISTS(SELECT 1 FROM applications a WHERE a.id=notifications.application_id AND a.applicant_user_id=$3) OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=notifications.project_id AND m.user_id=$3))`, chi.URLParam(r, "id"), s.CollegeID, s.UserID)
	if err != nil {
		http.Error(w, "notification unavailable", http.StatusServiceUnavailable)
		return
	}
	if tag.RowsAffected() != 1 {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) markAllRead(w http.ResponseWriter, r *http.Request) {
	s, ok := h.current(w, r)
	if !ok {
		return
	}
	_, err := h.pool.Exec(r.Context(), `UPDATE notifications SET read_at=clock_timestamp() WHERE college_id=$1 AND user_id=$2 AND read_at IS NULL AND ((project_id IS NULL OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=notifications.project_id AND m.user_id=$2)) OR (application_id IS NOT NULL AND EXISTS(SELECT 1 FROM applications a WHERE a.id=notifications.application_id AND a.applicant_user_id=$2))) AND (application_id IS NULL OR EXISTS(SELECT 1 FROM applications a WHERE a.id=notifications.application_id AND a.applicant_user_id=$2) OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=notifications.project_id AND m.user_id=$2))`, s.CollegeID, s.UserID)
	if err != nil {
		http.Error(w, "notifications unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	s, ok := h.current(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unavailable", http.StatusInternalServerError)
		return
	}
	lease, err := h.acquireStream(r.Context(), s)
	if errors.Is(err, errStreamLimit) {
		http.Error(w, "notification stream limit reached", http.StatusTooManyRequests)
		return
	}
	if err != nil {
		http.Error(w, "notification stream unavailable", http.StatusServiceUnavailable)
		return
	}
	defer h.pool.Exec(context.Background(), `DELETE FROM notification_stream_leases WHERE id=$1`, lease)
	after := int64(0)
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("after")
	}
	if raw != "" {
		after, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || after < 0 {
			http.Error(w, "invalid event cursor", 400)
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = fmt.Fprint(w, "retry: 2000\n\n")
	flusher.Flush()
	poll := time.NewTicker(2 * time.Second)
	heartbeat := time.NewTicker(15 * time.Second)
	defer poll.Stop()
	defer heartbeat.Stop()
	sent := 0
	for {
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
			if !h.sessionActive(r.Context(), s) {
				return
			}
			if _, err = h.pool.Exec(r.Context(), `UPDATE notification_stream_leases SET expires_at=clock_timestamp()+interval '40 seconds' WHERE id=$1`, lease); err != nil {
				return
			}
			rows, e := h.pool.Query(r.Context(), `SELECT n.id::text,n.sequence FROM notifications n WHERE n.college_id=$1 AND n.user_id=$2 AND n.sequence>$3 AND ((n.project_id IS NULL OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=n.project_id AND m.user_id=n.user_id)) OR (n.application_id IS NOT NULL AND EXISTS(SELECT 1 FROM applications a WHERE a.id=n.application_id AND a.applicant_user_id=n.user_id))) AND (n.application_id IS NULL OR EXISTS(SELECT 1 FROM applications a WHERE a.id=n.application_id AND a.applicant_user_id=n.user_id) OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=n.project_id AND m.user_id=n.user_id)) ORDER BY n.sequence LIMIT 100`, s.CollegeID, s.UserID, after)
			if e != nil {
				return
			}
			batch := make([]struct {
				id  string
				seq int64
			}, 0, 100)
			for rows.Next() {
				var item struct {
					id  string
					seq int64
				}
				if rows.Scan(&item.id, &item.seq) != nil {
					rows.Close()
					return
				}
				batch = append(batch, item)
			}
			rows.Close()
			for _, item := range batch {
				body, _ := json.Marshal(map[string]string{"id": item.id})
				if _, e = fmt.Fprintf(w, "id: %d\nevent: notification\ndata: %s\n\n", item.seq, body); e != nil {
					return
				}
				after = item.seq
				sent++
				flusher.Flush()
				if sent >= 1000 {
					return
				}
			}
		case <-heartbeat.C:
			if !h.sessionActive(r.Context(), s) {
				return
			}
			if _, err = fmt.Fprint(w, "event: access-check\ndata: {}\n\n: heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

var errStreamLimit = errors.New("notification stream capacity reached")

func (h *Handler) acquireStream(ctx context.Context, s identity.Session) (string, error) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var total, perUser int
	if err = tx.QueryRow(ctx, `SELECT total_limit,per_user_limit FROM notification_stream_capacity WHERE id=1 FOR UPDATE`).Scan(&total, &perUser); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM notification_stream_leases WHERE expires_at<=clock_timestamp()`); err != nil {
		return "", err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM notification_stream_leases`).Scan(&count); err != nil {
		return "", err
	}
	var userCount int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM notification_stream_leases WHERE college_id=$1 AND user_id=$2`, s.CollegeID, s.UserID).Scan(&userCount); err != nil {
		return "", err
	}
	if count >= total || userCount >= perUser {
		return "", errStreamLimit
	}
	var id string
	if err = tx.QueryRow(ctx, `INSERT INTO notification_stream_leases(college_id,user_id,session_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '40 seconds') RETURNING id::text`, s.CollegeID, s.UserID, s.ID).Scan(&id); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}
func (h *Handler) sessionActive(ctx context.Context, s identity.Session) bool {
	var active bool
	err := h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions se JOIN users u ON u.id=se.user_id JOIN colleges c ON c.id=COALESCE(se.college_id,u.college_id) WHERE se.id=$1 AND se.user_id=$2 AND c.id=$3 AND se.expires_at>clock_timestamp() AND u.status='active' AND c.status='active' AND EXISTS(SELECT 1 FROM campus_verifications v WHERE v.college_id=c.id AND v.user_id=u.id AND v.revoked_at IS NULL AND (v.source='administrator_review' OR EXISTS(SELECT 1 FROM campus_emails ce JOIN college_domains d ON d.college_id=ce.college_id AND d.domain=split_part(ce.email_normalized,'@',2) AND d.verified_at IS NOT NULL WHERE ce.id=v.campus_email_id AND ce.verified_at IS NOT NULL))) AND EXISTS(SELECT 1 FROM consent_records co WHERE co.user_id=u.id AND co.purpose='terms' AND co.policy_version='draft-1' AND co.revoked_at IS NULL) AND EXISTS(SELECT 1 FROM consent_records co WHERE co.user_id=u.id AND co.purpose='privacy' AND co.policy_version='draft-1' AND co.revoked_at IS NULL))`, s.ID, s.UserID, s.CollegeID).Scan(&active)
	return err == nil && active
}

// Keep delivery data intentionally small; event content and source code never enter email.
type Mail struct {
	Sender                          MailSender
	From, AppOrigin, SuppressionKey string
}
type MailSender interface {
	SendNotification(context.Context, string, string, string, string, string, string) (string, error)
}
