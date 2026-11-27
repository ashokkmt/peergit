package academic

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
)

func (h *Handler) requestFacultyVerification(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	if s.AccountType != "campus" {
		h.fail(w, r, http.StatusForbidden, "campus_account_required", "faculty verification is limited to campus accounts", nil)
		return
	}
	in, err := request.Decode[struct {
		DepartmentID    string `json:"department_id"`
		Title           string `json:"title"`
		AffiliationNote string `json:"affiliation_note"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.AffiliationNote = strings.TrimSpace(in.AffiliationNote)
	if len(in.Title) < 2 || len(in.Title) > 120 || len(in.AffiliationNote) < 8 || len(in.AffiliationNote) > 2000 {
		h.fail(w, r, http.StatusBadRequest, "faculty_request_invalid", "title or affiliation details are invalid", nil)
		return
	}
	var department any
	if in.DepartmentID != "" {
		department = in.DepartmentID
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO faculty_verification_requests(college_id,user_id,department_id,title,affiliation_note) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, s.CollegeID, s.UserID, department, in.Title, in.AffiliationNote).Scan(&id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "state": "pending"})
}

func (h *Handler) myFacultyVerification(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT id::text,title,affiliation_note,state,review_reason,created_at,reviewed_at FROM faculty_verification_requests WHERE college_id=$1 AND user_id=$2 ORDER BY created_at DESC,id DESC LIMIT 20`, s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, title, note, state string
		var reason *string
		var created time.Time
		var reviewed *time.Time
		if err = rows.Scan(&id, &title, &note, &state, &reason, &created, &reviewed); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "title": title, "affiliation_note": note, "state": state, "review_reason": reason, "created_at": created, "reviewed_at": reviewed})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) facultyQueue(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT fr.id::text,u.display_name,u.email_normalized,fr.title,fr.affiliation_note,COALESCE(o.name,''),fr.created_at FROM faculty_verification_requests fr JOIN users u ON u.id=fr.user_id LEFT JOIN organizations o ON o.id=fr.department_id AND o.college_id=fr.college_id WHERE fr.college_id=$1 AND fr.state='pending' ORDER BY fr.created_at,fr.id LIMIT 100`, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, email, title, note, department string
		var createdAt time.Time
		if err = rows.Scan(&id, &name, &email, &title, &note, &department, &createdAt); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "email": email, "title": title, "affiliation_note": note, "department": department, "created_at": createdAt})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) facultyProfiles(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT fp.user_id::text,u.display_name,fp.title,fp.status,fp.verified_at FROM faculty_profiles fp JOIN users u ON u.id=fp.user_id AND u.college_id=fp.college_id WHERE fp.college_id=$1 ORDER BY fp.status, u.display_name,fp.user_id LIMIT 200`, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, title, status string
		var verified time.Time
		if err = rows.Scan(&id, &name, &title, &status, &verified); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"user_id": id, "name": name, "title": title, "status": status, "verified_at": verified})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) decideFacultyVerification(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if (in.Decision != "approved" && in.Decision != "rejected") || len(in.Reason) > 1000 {
		h.fail(w, r, http.StatusBadRequest, "faculty_decision_invalid", "decision is invalid", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var userID, title string
	var department *string
	err = tx.QueryRow(r.Context(), `UPDATE faculty_verification_requests SET state=$3,reviewed_by=$4,review_reason=$5,reviewed_at=now() WHERE id=$1 AND college_id=$2 AND state='pending' RETURNING user_id::text,title,department_id::text`, chi.URLParam(r, "id"), s.CollegeID, in.Decision, s.UserID, strings.TrimSpace(in.Reason)).Scan(&userID, &title, &department)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Decision == "approved" {
		_, err = tx.Exec(r.Context(), `INSERT INTO faculty_profiles(college_id,user_id,department_id,title,verified_by) VALUES($1,$2,$3,$4,$5) ON CONFLICT(college_id,user_id) DO UPDATE SET department_id=EXCLUDED.department_id,title=EXCLUDED.title,status='active',verified_by=EXCLUDED.verified_by,verified_at=now(),revoked_by=NULL,revoked_at=NULL`, s.CollegeID, userID, department, title, s.UserID)
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO campus_roles(college_id,user_id,role,granted_by) VALUES($1,$2,'faculty',$3) ON CONFLICT DO NOTHING`, s.CollegeID, userID, s.UserID)
		}
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"id": chi.URLParam(r, "id"), "state": in.Decision})
}

func (h *Handler) setFacultyStatus(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Status string `json:"status"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Status != "active" && in.Status != "suspended" && in.Status != "revoked" {
		h.fail(w, r, http.StatusBadRequest, "faculty_status_invalid", "faculty status is invalid", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var revokedAt any
	if in.Status == "revoked" {
		revokedAt = time.Now()
	}
	tag, err := tx.Exec(r.Context(), `UPDATE faculty_profiles SET status=$3,revoked_by=CASE WHEN $3='revoked' THEN $4::uuid ELSE NULL END,revoked_at=$5 WHERE college_id=$1 AND user_id=$2`, s.CollegeID, chi.URLParam(r, "userID"), in.Status, s.UserID, revokedAt)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		h.fail(w, r, http.StatusNotFound, "faculty_not_found", "faculty profile was not found", nil)
		return
	}
	if in.Status == "revoked" {
		if _, err = tx.Exec(r.Context(), `DELETE FROM campus_roles WHERE college_id=$1 AND user_id=$2 AND role='faculty'`, s.CollegeID, chi.URLParam(r, "userID")); err != nil {
			h.err.Handle(w, r, err)
			return
		}
	} else if in.Status == "active" {
		if _, err = tx.Exec(r.Context(), `INSERT INTO campus_roles(college_id,user_id,role,granted_by) VALUES($1,$2,'faculty',$3) ON CONFLICT DO NOTHING`, s.CollegeID, chi.URLParam(r, "userID"), s.UserID); err != nil {
			h.err.Handle(w, r, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]string{"user_id": chi.URLParam(r, "userID"), "status": in.Status})
}
