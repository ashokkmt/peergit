package academic

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
)

func (h *Handler) createTerm(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Label    string `json:"label"`
		StartsAt string `json:"starts_at"`
		EndsAt   string `json:"ends_at"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO academic_terms(college_id,label,starts_at,ends_at,created_by) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, s.CollegeID, strings.TrimSpace(in.Label), in.StartsAt, in.EndsAt, s.UserID).Scan(&id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]string{"id": id})
}

func (h *Handler) listTerms(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT t.id::text,t.label,t.starts_at,t.ends_at FROM academic_terms t WHERE t.college_id=$1 AND (EXISTS(SELECT 1 FROM academic_courses c JOIN academic_course_staff cs ON cs.course_id=c.id JOIN faculty_profiles fp ON fp.college_id=cs.college_id AND fp.user_id=cs.user_id AND fp.status='active' WHERE c.college_id=t.college_id AND cs.user_id=$2) OR EXISTS(SELECT 1 FROM academic_enrollments e JOIN academic_cohorts co ON co.id=e.cohort_id WHERE co.term_id=t.id AND e.user_id=$2 AND e.status='active') OR $3) ORDER BY t.starts_at DESC,t.id`, s.CollegeID, s.UserID, s.CampusAdmin)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, label string
		var starts, ends time.Time
		if err = rows.Scan(&id, &label, &starts, &ends); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "label": label, "starts_at": starts, "ends_at": ends})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) createCourse(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Code         string `json:"code"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		DepartmentID string `json:"department_id"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var department any
	if in.DepartmentID != "" {
		department = in.DepartmentID
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO academic_courses(college_id,department_id,course_code,title,description,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, s.CollegeID, department, strings.TrimSpace(in.Code), strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), s.UserID).Scan(&id)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO academic_course_staff(college_id,course_id,user_id,role,assigned_by) VALUES($1,$2,$3,'lead',$3)`, s.CollegeID, id, s.UserID)
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

func (h *Handler) listCourses(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT c.id::text,c.course_code,c.title,c.description,c.status,EXISTS(SELECT 1 FROM academic_course_staff cs JOIN faculty_profiles fp ON fp.college_id=cs.college_id AND fp.user_id=cs.user_id AND fp.status='active' WHERE cs.course_id=c.id AND cs.user_id=$2) FROM academic_courses c WHERE c.college_id=$1 AND (EXISTS(SELECT 1 FROM academic_course_staff cs JOIN faculty_profiles fp ON fp.college_id=cs.college_id AND fp.user_id=cs.user_id AND fp.status='active' WHERE cs.course_id=c.id AND cs.user_id=$2) OR EXISTS(SELECT 1 FROM academic_enrollments e JOIN academic_cohorts co ON co.id=e.cohort_id WHERE co.course_id=c.id AND e.user_id=$2 AND e.status='active') OR $3) ORDER BY c.course_code,c.id`, s.CollegeID, s.UserID, s.CampusAdmin)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, code, title, description, status string
		var staff bool
		if err = rows.Scan(&id, &code, &title, &description, &status, &staff); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "code": code, "title": title, "description": description, "status": status, "is_staff": staff})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) listFaculty(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT fp.user_id::text,u.display_name,fp.title FROM faculty_profiles fp JOIN users u ON u.id=fp.user_id AND u.college_id=fp.college_id WHERE fp.college_id=$1 AND fp.status='active' ORDER BY u.display_name,fp.user_id LIMIT 100`, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]string, 0)
	for rows.Next() {
		var id, name, title string
		if err = rows.Scan(&id, &name, &title); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]string{"id": id, "name": name, "title": title})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT DISTINCT p.id::text,p.title,p.slug FROM projects p WHERE p.college_id=$1 AND p.project_context='ordinary' AND (EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=$2) OR EXISTS(SELECT 1 FROM project_members pm JOIN academic_enrollments e ON e.user_id=pm.user_id AND e.college_id=pm.college_id AND e.status='active' JOIN academic_cohorts co ON co.id=e.cohort_id AND co.college_id=e.college_id JOIN academic_course_staff cs ON cs.course_id=co.course_id AND cs.user_id=$2 WHERE pm.project_id=p.id AND pm.college_id=p.college_id) OR $3) ORDER BY p.title,p.id LIMIT 500`, s.CollegeID, s.UserID, s.CampusAdmin)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]string, 0)
	for rows.Next() {
		var id, title, slug string
		if err = rows.Scan(&id, &title, &slug); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]string{"id": id, "title": title, "slug": slug})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) createCohort(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		TermID string `json:"term_id"`
		Label  string `json:"label"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO academic_cohorts(college_id,course_id,term_id,label,created_by) SELECT c.college_id,c.id,$3,$4,$5 FROM academic_courses c WHERE c.id=$1 AND c.college_id=$2 AND EXISTS(SELECT 1 FROM academic_course_staff cs WHERE cs.course_id=c.id AND cs.user_id=$5) RETURNING id::text`, chi.URLParam(r, "id"), s.CollegeID, in.TermID, strings.TrimSpace(in.Label), s.UserID).Scan(&id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]string{"id": id})
}

func (h *Handler) listCohorts(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT co.id::text,co.label,c.id::text,c.course_code,t.id::text,t.label FROM academic_cohorts co JOIN academic_courses c ON c.id=co.course_id AND c.college_id=co.college_id JOIN academic_terms t ON t.id=co.term_id AND t.college_id=co.college_id WHERE co.college_id=$1 AND (EXISTS(SELECT 1 FROM academic_course_staff cs JOIN faculty_profiles fp ON fp.college_id=cs.college_id AND fp.user_id=cs.user_id AND fp.status='active' WHERE cs.course_id=c.id AND cs.user_id=$2) OR EXISTS(SELECT 1 FROM academic_enrollments e WHERE e.cohort_id=co.id AND e.user_id=$2 AND e.status='active') OR $3) ORDER BY t.starts_at DESC,c.course_code,co.label,co.id LIMIT 200`, s.CollegeID, s.UserID, s.CampusAdmin)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]string, 0)
	for rows.Next() {
		var id, label, courseID, courseCode, termID, termLabel string
		if err = rows.Scan(&id, &label, &courseID, &courseCode, &termID, &termLabel); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]string{"id": id, "label": label, "course_id": courseID, "course_code": courseCode, "term_id": termID, "term": termLabel})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) enrollStudent(w http.ResponseWriter, r *http.Request) {
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
	tag, err := h.pool.Exec(r.Context(), `INSERT INTO academic_enrollments(college_id,cohort_id,user_id) SELECT co.college_id,co.id,$3 FROM academic_cohorts co JOIN academic_courses c ON c.id=co.course_id AND c.college_id=co.college_id WHERE co.id=$1 AND co.college_id=$2 AND EXISTS(SELECT 1 FROM academic_course_staff cs WHERE cs.course_id=c.id AND cs.user_id=$4) ON CONFLICT(cohort_id,user_id) DO UPDATE SET status='active',enrolled_at=now()`, chi.URLParam(r, "id"), s.CollegeID, in.UserID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		h.fail(w, r, http.StatusForbidden, "cohort_scope_denied", "cohort staff access is required to enroll students", nil)
		return
	}
	response.NoContent(w)
}

func (h *Handler) createAssignment(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Title string `json:"title"`
		Brief string `json:"brief"`
		DueAt string `json:"due_at"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO academic_assignments(college_id,cohort_id,title,brief,due_at,created_by) SELECT co.college_id,co.id,$3,$4,$5,$6 FROM academic_cohorts co JOIN academic_courses c ON c.id=co.course_id AND c.college_id=co.college_id JOIN academic_course_staff cs ON cs.course_id=c.id AND cs.user_id=$6 WHERE co.id=$1 AND co.college_id=$2 RETURNING id::text`, chi.URLParam(r, "id"), s.CollegeID, strings.TrimSpace(in.Title), strings.TrimSpace(in.Brief), in.DueAt, s.UserID).Scan(&id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]string{"id": id})
}

func (h *Handler) listAssignments(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT a.id::text,a.title,a.brief,a.due_at,a.status,c.course_code,co.label FROM academic_assignments a JOIN academic_cohorts co ON co.id=a.cohort_id AND co.college_id=a.college_id JOIN academic_courses c ON c.id=co.course_id AND c.college_id=co.college_id WHERE a.college_id=$1 AND (EXISTS(SELECT 1 FROM academic_enrollments e WHERE e.cohort_id=co.id AND e.user_id=$2 AND e.status='active') OR EXISTS(SELECT 1 FROM academic_course_staff cs JOIN faculty_profiles fp ON fp.college_id=cs.college_id AND fp.user_id=cs.user_id AND fp.status='active' WHERE cs.course_id=c.id AND cs.user_id=$2) OR $3) ORDER BY a.due_at,a.id`, s.CollegeID, s.UserID, s.CampusAdmin)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, title, brief, code, cohort, status string
		var due time.Time
		if err = rows.Scan(&id, &title, &brief, &due, &status, &code, &cohort); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "title": title, "brief": brief, "due_at": due, "status": status, "course_code": code, "cohort": cohort})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) endorseProject(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Statement string `json:"statement"`
		Audience  string `json:"audience"`
		CourseID  string `json:"course_id"`
		CohortID  string `json:"cohort_id"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Audience == "" {
		in.Audience = "campus"
	}
	if len(strings.TrimSpace(in.Statement)) < 10 || len(in.Statement) > 2000 || in.Audience != "campus" && in.Audience != "private" || in.CourseID != "" && in.CohortID != "" {
		h.fail(w, r, http.StatusBadRequest, "endorsement_invalid", "endorsement fields are invalid", nil)
		return
	}
	var course, cohort any
	if in.CourseID != "" {
		course = in.CourseID
	}
	if in.CohortID != "" {
		cohort = in.CohortID
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO project_endorsements(college_id,project_id,faculty_id,course_id,cohort_id,statement,audience) SELECT $1,p.id,$3,$4,$5,$6,$7 FROM projects p WHERE p.id=$2 AND p.college_id=$1 AND p.project_context='ordinary' AND ((($4::uuid IS NULL AND $5::uuid IS NULL) AND EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=$3)) OR ($4::uuid IS NOT NULL AND EXISTS(SELECT 1 FROM academic_course_staff cs JOIN academic_cohorts co ON co.course_id=cs.course_id AND co.college_id=cs.college_id JOIN academic_enrollments e ON e.cohort_id=co.id AND e.college_id=co.college_id AND e.status='active' JOIN project_members pm ON pm.user_id=e.user_id AND pm.college_id=e.college_id AND pm.project_id=p.id WHERE cs.course_id=$4 AND cs.user_id=$3)) OR ($5::uuid IS NOT NULL AND EXISTS(SELECT 1 FROM academic_course_staff cs JOIN academic_cohorts co ON co.course_id=cs.course_id AND co.college_id=cs.college_id JOIN academic_enrollments e ON e.cohort_id=co.id AND e.college_id=co.college_id AND e.status='active' JOIN project_members pm ON pm.user_id=e.user_id AND pm.college_id=e.college_id AND pm.project_id=p.id WHERE co.id=$5 AND cs.user_id=$3))) RETURNING id::text`, s.CollegeID, chi.URLParam(r, "id"), s.UserID, course, cohort, strings.TrimSpace(in.Statement), in.Audience).Scan(&id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]string{"id": id})
}

func (h *Handler) listEndorsements(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT e.id::text,e.faculty_id::text,u.display_name,e.statement,e.audience,e.created_at FROM project_endorsements e JOIN users u ON u.id=e.faculty_id WHERE e.project_id=$1 AND e.college_id=$2 AND e.revoked_at IS NULL AND (e.audience='campus' OR e.faculty_id=$3 OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=e.project_id AND pm.user_id=$3)) ORDER BY e.created_at DESC,e.id`, chi.URLParam(r, "id"), s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, authorID, author, statement, audience string
		var created time.Time
		if err = rows.Scan(&id, &authorID, &author, &statement, &audience, &created); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "faculty_id": authorID, "faculty": author, "statement": statement, "audience": audience, "created_at": created})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) revokeEndorsement(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	tag, err := h.pool.Exec(r.Context(), `UPDATE project_endorsements SET revoked_at=now(),revoked_by=$3 WHERE id=$1 AND college_id=$2 AND faculty_id=$3 AND revoked_at IS NULL`, chi.URLParam(r, "id"), s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		h.fail(w, r, http.StatusNotFound, "endorsement_not_found", "active endorsement was not found", nil)
		return
	}
	response.NoContent(w)
}
