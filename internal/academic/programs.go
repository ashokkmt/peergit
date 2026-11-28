package academic

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
)

func (h *Handler) listCalls(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT c.id::text,c.kind,c.title,c.description,c.eligibility,c.capacity,c.closes_at,c.status,($3 OR EXISTS(SELECT 1 FROM academic_call_reviewers cr JOIN faculty_profiles fp ON fp.user_id=cr.reviewer_id AND fp.college_id=c.college_id AND fp.status='active' WHERE cr.call_id=c.id AND cr.reviewer_id=$2)) FROM academic_calls c WHERE c.college_id=$1 AND c.status IN ('open','closed') ORDER BY c.closes_at NULLS LAST,c.id LIMIT 100`, s.CollegeID, s.UserID, s.CampusAdmin)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, kind, title, description, eligibility, status string
		var capacity *int
		var closes *time.Time
		var canReview bool
		if err = rows.Scan(&id, &kind, &title, &description, &eligibility, &capacity, &closes, &status, &canReview); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "kind": kind, "title": title, "description": description, "eligibility": eligibility, "capacity": capacity, "closes_at": closes, "status": status, "can_review": canReview})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) listProgramCohorts(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT pc.id::text,pc.call_id::text,c.title,pc.label,pc.starts_at,pc.ends_at,(SELECT count(*) FROM academic_program_cohort_projects cp WHERE cp.cohort_id=pc.id) FROM academic_program_cohorts pc JOIN academic_calls c ON c.id=pc.call_id AND c.college_id=pc.college_id WHERE pc.college_id=$1 AND (c.created_by=$2 OR $3 OR EXISTS(SELECT 1 FROM academic_call_reviewers cr JOIN faculty_profiles fp ON fp.user_id=cr.reviewer_id AND fp.college_id=c.college_id AND fp.status='active' WHERE cr.call_id=c.id AND cr.reviewer_id=$2)) ORDER BY pc.created_at DESC,pc.id LIMIT 200`, s.CollegeID, s.UserID, s.CampusAdmin)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, callID, call, label string
		var starts, ends *time.Time
		var projects int
		if err = rows.Scan(&id, &callID, &call, &label, &starts, &ends, &projects); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "call_id": callID, "call": call, "label": label, "starts_at": starts, "ends_at": ends, "project_count": projects})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) closeCall(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	tag, err := h.pool.Exec(r.Context(), `UPDATE academic_calls SET status='closed',version=version+1 WHERE id=$1 AND college_id=$2 AND status IN ('open','draft') AND (created_by=$3 OR $4)`, chi.URLParam(r, "id"), s.CollegeID, s.UserID, s.CampusAdmin)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		h.fail(w, r, http.StatusNotFound, "call_not_found", "open call was not found or cannot be closed", nil)
		return
	}
	response.NoContent(w)
}

func (h *Handler) listApplications(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT a.id::text,a.project_id::text,p.title,a.applicant_id::text,u.display_name,a.statement,a.state,a.submitted_at,COALESCE(r.recommendation,'') FROM academic_call_applications a JOIN projects p ON p.id=a.project_id AND p.college_id=a.college_id JOIN users u ON u.id=a.applicant_id LEFT JOIN academic_call_reviews r ON r.application_id=a.id AND r.reviewer_id=$3 WHERE a.call_id=$1 AND a.college_id=$2 AND (a.applicant_id=$3 OR EXISTS(SELECT 1 FROM academic_call_reviewers cr JOIN faculty_profiles fp ON fp.user_id=cr.reviewer_id AND fp.college_id=a.college_id AND fp.status='active' WHERE cr.call_id=a.call_id AND cr.reviewer_id=$3) OR $4) ORDER BY a.submitted_at,a.id LIMIT 200`, chi.URLParam(r, "id"), s.CollegeID, s.UserID, s.CampusAdmin)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, projectID, project, applicantID, applicant, statement, state, recommendation string
		var submitted time.Time
		if err = rows.Scan(&id, &projectID, &project, &applicantID, &applicant, &statement, &state, &submitted, &recommendation); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "project_id": projectID, "project": project, "applicant_id": applicantID, "applicant": applicant, "statement": statement, "state": state, "submitted_at": submitted, "my_recommendation": recommendation})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) createCall(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Kind           string     `json:"kind"`
		OrganizationID string     `json:"organization_id"`
		Title          string     `json:"title"`
		Description    string     `json:"description"`
		Eligibility    string     `json:"eligibility"`
		Capacity       *int       `json:"capacity"`
		ClosesAt       *time.Time `json:"closes_at"`
		Status         string     `json:"status"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Kind != "research" && in.Kind != "program" || len(strings.TrimSpace(in.Title)) < 3 || len(in.Title) > 160 || len(strings.TrimSpace(in.Description)) == 0 || len(in.Description) > 8000 || in.Status != "open" && in.Status != "draft" {
		h.fail(w, r, http.StatusBadRequest, "call_invalid", "call details are invalid", nil)
		return
	}
	var organization any
	if in.OrganizationID != "" {
		organization = in.OrganizationID
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO academic_calls(college_id,kind,organization_id,created_by,title,description,eligibility,capacity,closes_at,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id::text`, s.CollegeID, in.Kind, organization, s.UserID, strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), strings.TrimSpace(in.Eligibility), in.Capacity, in.ClosesAt, in.Status).Scan(&id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]string{"id": id})
}

func (h *Handler) applyToCall(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		ProjectID string `json:"project_id"`
		Statement string `json:"statement"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if len(strings.TrimSpace(in.Statement)) < 10 || len(in.Statement) > 4000 {
		h.fail(w, r, http.StatusBadRequest, "application_invalid", "application statement is invalid", nil)
		return
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO academic_call_applications(college_id,call_id,project_id,applicant_id,statement) SELECT c.college_id,c.id,p.id,$4,$5 FROM academic_calls c JOIN projects p ON p.id=$3 AND p.college_id=c.college_id AND p.project_context='ordinary' JOIN project_members pm ON pm.project_id=p.id AND pm.user_id=$4 WHERE c.id=$1 AND c.college_id=$2 AND c.status='open' AND c.kind IN ('research','program') AND (c.closes_at IS NULL OR c.closes_at>now()) RETURNING id::text`, chi.URLParam(r, "id"), s.CollegeID, in.ProjectID, s.UserID, strings.TrimSpace(in.Statement)).Scan(&id)
	if err == pgx.ErrNoRows {
		h.fail(w, r, http.StatusForbidden, "application_scope_denied", "an open call and project membership in this campus scope are required", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]string{"id": id, "state": "submitted"})
}

func (h *Handler) assignReviewer(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		UserID string `json:"user_id"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	tag, err := h.pool.Exec(r.Context(), `INSERT INTO academic_call_reviewers(college_id,call_id,reviewer_id,assigned_by) SELECT c.college_id,c.id,$3,$4 FROM academic_calls c WHERE c.id=$1 AND c.college_id=$2 AND EXISTS(SELECT 1 FROM faculty_profiles fp WHERE fp.college_id=c.college_id AND fp.user_id=$4 AND fp.status='active') AND EXISTS(SELECT 1 FROM faculty_profiles fp WHERE fp.college_id=c.college_id AND fp.user_id=$3 AND fp.status='active') AND $3<>$4 ON CONFLICT DO NOTHING`, chi.URLParam(r, "id"), s.CollegeID, in.UserID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		h.fail(w, r, http.StatusForbidden, "reviewer_scope_invalid", "reviewers must be verified faculty and cannot assign themselves", nil)
		return
	}
	response.NoContent(w)
}

func (h *Handler) reviewApplication(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Recommendation string `json:"recommendation"`
		Notes          string `json:"notes"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Recommendation != "support" && in.Recommendation != "decline" && in.Recommendation != "abstain" || len(in.Notes) > 4000 {
		h.fail(w, r, http.StatusBadRequest, "review_invalid", "review fields are invalid", nil)
		return
	}
	tag, err := h.pool.Exec(r.Context(), `INSERT INTO academic_call_reviews(college_id,call_id,application_id,reviewer_id,recommendation,notes) SELECT a.college_id,a.call_id,a.id,$3,$4,$5 FROM academic_call_applications a JOIN academic_call_reviewers cr ON cr.call_id=a.call_id AND cr.reviewer_id=$3 WHERE a.id=$1 AND a.college_id=$2 AND a.applicant_id<>$3 AND NOT EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=a.project_id AND pm.user_id=$3) ON CONFLICT(application_id,reviewer_id) DO UPDATE SET recommendation=EXCLUDED.recommendation,notes=EXCLUDED.notes,reviewed_at=now()`, chi.URLParam(r, "id"), s.CollegeID, s.UserID, in.Recommendation, strings.TrimSpace(in.Notes))
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		h.fail(w, r, http.StatusForbidden, "review_conflict", "reviewer is unassigned or has a conflict with the application", nil)
		return
	}
	response.NoContent(w)
}

func (h *Handler) decideApplication(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Decision string `json:"decision"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Decision != "accepted" && in.Decision != "rejected" {
		h.fail(w, r, http.StatusBadRequest, "decision_invalid", "decision must be accepted or rejected", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var callID string
	var capacity *int
	var projectID, applicantID string
	err = tx.QueryRow(r.Context(), `SELECT a.call_id::text,c.capacity,a.project_id::text,a.applicant_id::text FROM academic_call_applications a JOIN academic_calls c ON c.id=a.call_id AND c.college_id=a.college_id WHERE a.id=$1 AND a.college_id=$2 AND a.state IN ('submitted','under_review') AND (EXISTS(SELECT 1 FROM academic_call_reviewers cr WHERE cr.call_id=a.call_id AND cr.reviewer_id=$3) OR $4) AND a.applicant_id<>$3 AND NOT EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=a.project_id AND pm.user_id=$3) FOR UPDATE OF c,a`, chi.URLParam(r, "id"), s.CollegeID, s.UserID, s.CampusAdmin).Scan(&callID, &capacity, &projectID, &applicantID)
	if err != nil {
		h.fail(w, r, http.StatusForbidden, "decision_scope_denied", "application is unavailable or you are not an assigned conflict-free reviewer", err)
		return
	}
	if in.Decision == "accepted" && capacity != nil {
		var accepted int
		if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM academic_call_applications WHERE call_id=$1 AND state='accepted'`, callID).Scan(&accepted); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if accepted >= *capacity {
			h.fail(w, r, http.StatusConflict, "call_capacity_full", "program capacity has been reached", nil)
			return
		}
	}
	_, err = tx.Exec(r.Context(), `UPDATE academic_call_applications SET state=$2,decided_by=$3,decided_at=now(),version=version+1 WHERE id=$1`, chi.URLParam(r, "id"), in.Decision, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]string{"state": in.Decision})
}

func (h *Handler) createProgramCohort(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Label    string     `json:"label"`
		StartsAt *time.Time `json:"starts_at"`
		EndsAt   *time.Time `json:"ends_at"`
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
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO academic_program_cohorts(college_id,call_id,label,starts_at,ends_at,created_by) SELECT c.college_id,c.id,$3,$4,$5,$6 FROM academic_calls c WHERE c.id=$1 AND c.college_id=$2 AND c.kind='program' AND EXISTS(SELECT 1 FROM academic_call_reviewers cr WHERE cr.call_id=c.id AND cr.reviewer_id=$6) RETURNING id::text`, chi.URLParam(r, "id"), s.CollegeID, strings.TrimSpace(in.Label), in.StartsAt, in.EndsAt, s.UserID).Scan(&id)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO academic_program_cohort_projects(college_id,cohort_id,call_id,project_id,application_id) SELECT a.college_id,$2,a.call_id,a.project_id,a.id FROM academic_call_applications a WHERE a.call_id=$1 AND a.college_id=$3 AND a.state='accepted' ON CONFLICT DO NOTHING`, chi.URLParam(r, "id"), id, s.CollegeID)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]string{"id": id})
}

func (h *Handler) createMilestone(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		DueAt       *time.Time `json:"due_at"`
		Status      string     `json:"status"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Status == "" {
		in.Status = "planned"
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO academic_program_milestones(college_id,cohort_id,title,description,due_at,status,updated_by) SELECT pc.college_id,pc.id,$3,$4,$5,$6,$7 FROM academic_program_cohorts pc WHERE pc.id=$1 AND pc.college_id=$2 AND EXISTS(SELECT 1 FROM academic_call_reviewers cr WHERE cr.call_id=pc.call_id AND cr.reviewer_id=$7) RETURNING id::text`, chi.URLParam(r, "id"), s.CollegeID, strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), in.DueAt, in.Status, s.UserID).Scan(&id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]string{"id": id})
}

func (h *Handler) saveOutcome(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		ProjectID string `json:"project_id"`
		Summary   string `json:"summary"`
		Result    string `json:"result"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO academic_program_outcomes(college_id,cohort_id,project_id,summary,result,updated_by) SELECT pc.college_id,pc.id,$3,$4,$5,$6 FROM academic_program_cohorts pc JOIN academic_program_cohort_projects cp ON cp.cohort_id=pc.id AND cp.project_id=$3 WHERE pc.id=$1 AND pc.college_id=$2 AND EXISTS(SELECT 1 FROM academic_call_reviewers cr WHERE cr.call_id=pc.call_id AND cr.reviewer_id=$6) ON CONFLICT(cohort_id,project_id) DO UPDATE SET summary=EXCLUDED.summary,result=EXCLUDED.result,updated_by=EXCLUDED.updated_by,updated_at=now() RETURNING id::text`, chi.URLParam(r, "id"), s.CollegeID, in.ProjectID, strings.TrimSpace(in.Summary), strings.TrimSpace(in.Result), s.UserID).Scan(&id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]string{"id": id})
}
