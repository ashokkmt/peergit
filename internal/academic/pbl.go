package academic

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"peergit/internal/identity"
	"peergit/internal/platform/audit"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
)

func (h *Handler) registerPBL(api chi.Router) {
	api.Get("/academic/pbl", h.pblWorkspace)
	api.Get("/academic/pbl/teams/{team}/audit.csv", h.exportPBLAudit)
	write := api.With(h.auth.RequireCSRF)
	write.Post("/academic/pbl/offerings", h.publishPBLOffering)
	write.Post("/academic/pbl/offerings/{offering}/teams", h.registerPBLTeam)
	write.Put("/academic/pbl/teams/{team}/roster", h.confirmPBLRoster)
	write.Put("/academic/pbl/teams/{team}/faculty", h.assignPBLFaculty)
	write.Post("/academic/pbl/teams/{team}/idea", h.submitPBLIdea)
	write.Put("/academic/pbl/teams/{team}/idea", h.reviewPBLIdea)
	write.Put("/academic/pbl/teams/{team}/attendance", h.recordPBLAttendance)
}

func (h *Handler) pblSession(w http.ResponseWriter, r *http.Request) (identity.Session, bool) {
	s, ok := h.session(w, r)
	if ok && s.AccountType != "campus" {
		h.fail(w, r, http.StatusForbidden, "campus_scope_required", "verified campus access is required", nil)
		return s, false
	}
	return s, ok
}

func pblID(value string) bool {
	var id pgtype.UUID
	return value != "" && id.Scan(value) == nil && id.Valid
}

func (h *Handler) pblFailure(w http.ResponseWriter, r *http.Request, err error) {
	var databaseErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		h.fail(w, r, http.StatusNotFound, "pbl_resource_unavailable", "the requested PBL resource is unavailable", nil)
	case errors.As(err, &databaseErr) && databaseErr.Code == "23505":
		h.fail(w, r, http.StatusConflict, "pbl_duplicate", "this roster or offering conflicts with an existing PBL record", nil)
	case errors.As(err, &databaseErr) && (strings.HasPrefix(databaseErr.Code, "22") || strings.HasPrefix(databaseErr.Code, "23")):
		h.fail(w, r, http.StatusBadRequest, "pbl_invalid", "check the PBL fields and their campus scope", nil)
	default:
		h.err.Handle(w, r, err)
	}
}

func (h *Handler) pblWorkspace(w http.ResponseWriter, r *http.Request) {
	s, ok := h.pblSession(w, r)
	if !ok {
		return
	}
	coordinator := s.CampusAdmin && s.MFAEnabled && s.MFAFresh
	var offerings, teams json.RawMessage
	err := h.pool.QueryRow(r.Context(), `
		WITH visible_offerings AS (
			SELECT o.id,o.course_id,o.term_id,c.course_code,c.title as course,t.label AS term,o.closes_at,o.phase_one_due,o.phase_two_due,o.phase_three_due,
				EXISTS(SELECT 1 FROM academic_enrollments e JOIN academic_cohorts co ON co.id=e.cohort_id AND co.college_id=e.college_id
					WHERE e.user_id=$2 AND e.college_id=$1 AND e.status='active' AND co.course_id=o.course_id AND co.term_id=o.term_id) AS eligible
			FROM pbl_offerings o JOIN academic_courses c ON c.id=o.course_id JOIN academic_terms t ON t.id=o.term_id
			WHERE o.college_id=$1
		), accessible_teams AS (
			SELECT team.*,f.user_id IS NOT NULL AS can_review,COALESCE(u.display_name,'') AS faculty_name
			FROM pbl_teams team
			LEFT JOIN pbl_active_faculty f ON f.team_id=team.id AND f.user_id=$2
			LEFT JOIN users u ON u.id=team.faculty_id
			WHERE team.college_id=$1 AND ($3 OR f.user_id IS NOT NULL
				OR (team.confirmed_at IS NULL AND team.leader_id=$2)
				OR EXISTS(SELECT 1 FROM pbl_active_team_members m WHERE m.team_id=team.id AND m.user_id=$2))
			ORDER BY team.created_at DESC,team.id LIMIT 100
		)
		SELECT COALESCE((SELECT jsonb_agg(to_jsonb(o)) FROM (
			SELECT * FROM visible_offerings v WHERE $3 OR v.eligible OR EXISTS(SELECT 1 FROM accessible_teams t WHERE t.offering_id=v.id)
			ORDER BY v.closes_at DESC,v.id LIMIT 100) o),'[]'),
		COALESCE((SELECT jsonb_agg(jsonb_build_object(
			'id',t.id,'offering_id',t.offering_id,'title',t.title,'confirmed',t.confirmed_at IS NOT NULL,
			'is_leader',t.leader_id=$2,'can_review',t.can_review,'faculty_name',t.faculty_name,'project_id',t.project_id,
			'idea',t.idea,'idea_state',t.idea_state,'idea_feedback',t.idea_feedback,
			'members',COALESCE((SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object(
				'id',m.id,'name',m.student_name,'section',m.section,'user_id',m.user_id,
				'roll_number',CASE WHEN $3 OR (t.confirmed_at IS NULL AND t.leader_id=$2) THEN m.roll_number END
			)) ORDER BY m.id) FROM pbl_team_members m WHERE m.team_id=t.id),'[]'),
			'attendance',COALESCE((SELECT jsonb_agg(jsonb_build_object('student_id',a.student_id,'stage',a.stage,'present',a.present) ORDER BY a.stage,a.student_id)
				FROM pbl_attendance a WHERE a.team_id=t.id AND ($3 OR t.can_review OR a.student_id=$2)),'[]'),
			'checkpoints',COALESCE((SELECT jsonb_agg(jsonb_build_object(
				'stage',p.stage,'snapshot_id',p.snapshot_id,'deadline',p.deadline,'feedback',p.feedback,
				'sha',s.commit_sha,'receipt_at',s.receipt_at,'capture_state',s.state,'archive_sha256',s.archive_sha256) ORDER BY p.stage)
				FROM pbl_checkpoints p JOIN repository_snapshots s ON s.id=p.snapshot_id WHERE p.team_id=t.id),'[]')
		) ORDER BY t.created_at DESC,t.id) FROM accessible_teams t),'[]')`, s.CollegeID, s.UserID, coordinator).Scan(&offerings, &teams)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	_ = response.OK(w, map[string]any{"coordinator": coordinator, "offerings": offerings, "teams": teams})
}

func (h *Handler) publishPBLOffering(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		CourseID  string    `json:"course_id"`
		TermID    string    `json:"term_id"`
		ClosesAt  time.Time `json:"closes_at"`
		FirstDue  time.Time `json:"phase_one_due"`
		SecondDue time.Time `json:"phase_two_due"`
		ThirdDue  time.Time `json:"phase_three_due"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !pblID(in.CourseID) || !pblID(in.TermID) || !time.Now().Before(in.ClosesAt) ||
		!in.ClosesAt.Before(in.FirstDue) || !in.FirstDue.Before(in.SecondDue) || !in.SecondDue.Before(in.ThirdDue) {
		h.fail(w, r, 400, "pbl_deadlines_invalid", "choose a subject, term and increasing future deadlines", nil)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO pbl_offerings(college_id,course_id,term_id,published_by,closes_at,phase_one_due,phase_two_due,phase_three_due)
		SELECT $1,c.id,t.id,$4,$5,$6,$7,$8 FROM academic_courses c JOIN academic_terms t ON t.college_id=c.college_id
		WHERE c.college_id=$1 AND c.id=$2 AND c.status='active' AND t.id=$3 RETURNING id::text`,
		s.CollegeID, in.CourseID, in.TermID, s.UserID, in.ClosesAt, in.FirstDue, in.SecondDue, in.ThirdDue).Scan(&id)
	h.finishPBL(w, r, tx, s, id, "pbl.offering_published", err)
}

type pblRosterEntry struct {
	Name    string `json:"name"`
	Roll    string `json:"roll_number"`
	Section string `json:"section"`
}

func (h *Handler) registerPBLTeam(w http.ResponseWriter, r *http.Request) {
	s, ok := h.pblSession(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Title   string           `json:"title"`
		Members []pblRosterEntry `json:"members"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	offering := chi.URLParam(r, "offering")
	if !pblID(offering) || len(in.Title) < 3 || len(in.Title) > 120 || len(in.Members) < 1 || len(in.Members) > 12 {
		h.fail(w, r, 400, "pbl_roster_invalid", "enter a team name and up to 12 roster entries, including the leader first", nil)
		return
	}
	rolls := make(map[string]bool, len(in.Members))
	for i := range in.Members {
		member := &in.Members[i]
		member.Name = strings.TrimSpace(member.Name)
		member.Roll = strings.ToUpper(strings.TrimSpace(member.Roll))
		member.Section = strings.TrimSpace(member.Section)
		if len(member.Name) < 1 || len(member.Name) > 120 || len(member.Roll) < 1 || len(member.Roll) > 40 ||
			len(member.Section) < 1 || len(member.Section) > 40 || rolls[member.Roll] {
			h.fail(w, r, 400, "pbl_roster_invalid", "enter names, unique roll numbers and sections for each member", nil)
			return
		}
		rolls[member.Roll] = true
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO pbl_teams(college_id,offering_id,leader_id,title)
		SELECT o.college_id,o.id,$3,$4 FROM pbl_offerings o WHERE o.id=$1 AND o.college_id=$2
		AND o.closes_at>clock_timestamp() AND EXISTS(SELECT 1 FROM academic_enrollments e
			JOIN academic_cohorts co ON co.id=e.cohort_id AND co.college_id=e.college_id
			WHERE e.user_id=$3 AND e.college_id=$2 AND e.status='active' AND co.course_id=o.course_id AND co.term_id=o.term_id)
		RETURNING id::text`, offering, s.CollegeID, s.UserID, in.Title).Scan(&id)
	if err == nil {
		payload, _ := json.Marshal(in.Members)
		_, err = tx.Exec(ctx, `INSERT INTO pbl_team_members(college_id,team_id,offering_id,student_name,roll_number,section)
			SELECT $1,$2,$3,item->>'name',item->>'roll_number',item->>'section' FROM jsonb_array_elements($4::jsonb) item`,
			s.CollegeID, id, offering, payload)
	}
	h.finishPBL(w, r, tx, s, id, "pbl.team_registered", err)
}

func (h *Handler) confirmPBLRoster(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	type match struct {
		EntryID string `json:"entry_id"`
		Handle  string `json:"handle"`
	}
	in, err := request.Decode[struct {
		Matches []match `json:"matches"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	id := chi.URLParam(r, "team")
	if !pblID(id) || len(in.Matches) < 1 || len(in.Matches) > 12 {
		h.fail(w, r, 400, "pbl_matches_invalid", "match each entry to a verified student", nil)
		return
	}
	for i := range in.Matches {
		in.Matches[i].Handle = strings.ToLower(strings.TrimSpace(in.Matches[i].Handle))
		if !pblID(in.Matches[i].EntryID) || len(in.Matches[i].Handle) < 3 || len(in.Matches[i].Handle) > 30 {
			h.fail(w, r, 400, "pbl_matches_invalid", "match each entry to a verified student", nil)
			return
		}
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var leader string
	err = tx.QueryRow(ctx, `SELECT leader_id::text FROM pbl_teams WHERE id=$1 AND college_id=$2 AND confirmed_at IS NULL FOR UPDATE`, id, s.CollegeID).Scan(&leader)
	if err != nil {
		h.pblFailure(w, r, err)
		return
	}
	payload, _ := json.Marshal(in.Matches)
	updated, err := tx.Exec(ctx, `UPDATE pbl_team_members m SET user_id=u.id
		FROM jsonb_to_recordset($3::jsonb) AS x(entry_id uuid,handle text)
		JOIN users u ON u.handle=x.handle AND u.college_id=$2 AND u.account_type='campus' AND u.status='active'
		JOIN campus_verifications v ON v.college_id=u.college_id AND v.user_id=u.id AND v.revoked_at IS NULL
		WHERE m.id=x.entry_id AND m.team_id=$1 AND m.college_id=$2
		AND EXISTS(SELECT 1 FROM pbl_offerings o JOIN academic_cohorts c
			ON c.college_id=o.college_id AND c.course_id=o.course_id AND c.term_id=o.term_id
			JOIN academic_enrollments e ON e.cohort_id=c.id AND e.college_id=c.college_id AND e.user_id=u.id AND e.status='active'
			WHERE o.id=m.offering_id AND o.college_id=m.college_id)`, id, s.CollegeID, payload)
	if err != nil {
		h.pblFailure(w, r, err)
		return
	}
	if updated.RowsAffected() != int64(len(in.Matches)) {
		h.fail(w, r, 409, "pbl_roster_unmatched", "some entries did not match a verified student enrolled in this offering", nil)
		return
	}
	err = tx.QueryRow(ctx, `UPDATE pbl_teams t SET confirmed_at=clock_timestamp() WHERE t.id=$1 AND t.college_id=$2
		AND EXISTS(SELECT 1 FROM pbl_team_members m WHERE m.team_id=t.id AND m.user_id=$3)
		AND NOT EXISTS(SELECT 1 FROM pbl_team_members m WHERE m.team_id=t.id AND m.user_id IS NULL)
		RETURNING t.id::text`, id, s.CollegeID, leader).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 409, "pbl_roster_incomplete", "match every roster entry and include the team leader", nil)
		return
	}
	h.finishPBL(w, r, tx, s, id, "pbl.roster_confirmed", err)
}

func (h *Handler) assignPBLFaculty(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		FacultyID string `json:"faculty_id"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	id := chi.URLParam(r, "team")
	if !pblID(id) || !pblID(in.FacultyID) {
		h.fail(w, r, 400, "pbl_faculty_invalid", "choose an active faculty member", nil)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `UPDATE pbl_teams t SET faculty_id=$3 WHERE t.id=$1 AND t.college_id=$2 AND t.confirmed_at IS NOT NULL
		AND EXISTS(SELECT 1 FROM faculty_profiles f JOIN users u ON u.id=f.user_id AND u.college_id=f.college_id
			JOIN campus_verifications v ON v.college_id=u.college_id AND v.user_id=u.id AND v.revoked_at IS NULL
			WHERE f.college_id=$2 AND f.user_id=$3 AND f.status='active' AND u.status='active') RETURNING id::text`,
		id, s.CollegeID, in.FacultyID).Scan(&id)
	h.finishPBL(w, r, tx, s, id, "pbl.faculty_assigned", err)
}

func (h *Handler) submitPBLIdea(w http.ResponseWriter, r *http.Request) {
	h.changePBLIdea(w, r, false)
}

func (h *Handler) reviewPBLIdea(w http.ResponseWriter, r *http.Request) {
	h.changePBLIdea(w, r, true)
}

func (h *Handler) changePBLIdea(w http.ResponseWriter, r *http.Request, review bool) {
	s, ok := h.pblSession(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Idea     string `json:"idea"`
		Decision string `json:"decision"`
		Feedback string `json:"feedback"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	id := chi.URLParam(r, "team")
	in.Idea = strings.TrimSpace(in.Idea)
	in.Feedback = strings.TrimSpace(in.Feedback)
	if !pblID(id) || (!review && (len(in.Idea) < 1 || len(in.Idea) > 4000)) ||
		(review && ((in.Decision != "approved" && in.Decision != "revise") || len(in.Feedback) > 2000)) {
		h.fail(w, r, 400, "pbl_idea_invalid", "check the idea or review decision", nil)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	action := "pbl.idea_submitted"
	if review {
		action = "pbl.idea_reviewed"
		err = tx.QueryRow(ctx, `UPDATE pbl_teams t SET idea_state=$4,idea_feedback=$5 WHERE t.id=$1 AND t.college_id=$2
			AND t.idea_state='submitted' AND t.faculty_id=$3 AND EXISTS(SELECT 1 FROM pbl_active_faculty f WHERE f.team_id=t.id AND f.user_id=$3)
			RETURNING id::text`, id, s.CollegeID, s.UserID, in.Decision, in.Feedback).Scan(&id)
	} else {
		err = tx.QueryRow(ctx, `UPDATE pbl_teams t SET idea=$4,idea_state='submitted',idea_feedback='' WHERE t.id=$1 AND t.college_id=$2
			AND t.leader_id=$3 AND t.idea_state IN ('draft','revise') AND EXISTS(SELECT 1 FROM pbl_active_team_members m WHERE m.team_id=t.id AND m.user_id=$3)
			RETURNING id::text`, id, s.CollegeID, s.UserID, in.Idea).Scan(&id)
	}
	h.finishPBL(w, r, tx, s, id, action, err)
}

func (h *Handler) recordPBLAttendance(w http.ResponseWriter, r *http.Request) {
	s, ok := h.pblSession(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Student string `json:"student_id"`
		Stage   *int16 `json:"stage"`
		Present *bool  `json:"present"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	id := chi.URLParam(r, "team")
	if !pblID(id) || !pblID(in.Student) || in.Stage == nil || *in.Stage < 0 || *in.Stage > 3 || in.Present == nil {
		h.fail(w, r, 400, "pbl_attendance_invalid", "choose a roster member, stage and attendance", nil)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `SELECT t.id::text FROM pbl_teams t WHERE t.id=$1 AND t.college_id=$2 AND t.faculty_id=$3
		AND EXISTS(SELECT 1 FROM pbl_active_faculty f WHERE f.team_id=t.id AND f.user_id=$3)
		AND EXISTS(SELECT 1 FROM pbl_team_members m WHERE m.team_id=t.id AND m.user_id=$4) FOR UPDATE`, id, s.CollegeID, s.UserID, in.Student).Scan(&id)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO pbl_attendance(college_id,team_id,student_id,stage,present,recorded_by)
			VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(team_id,student_id,stage) DO UPDATE
			SET present=EXCLUDED.present,recorded_by=EXCLUDED.recorded_by,recorded_at=clock_timestamp()`,
			s.CollegeID, id, in.Student, in.Stage, in.Present, s.UserID)
	}
	h.finishPBL(w, r, tx, s, id, "pbl.attendance_recorded", err)
}

func (h *Handler) finishPBL(w http.ResponseWriter, r *http.Request, tx pgx.Tx, s identity.Session, id, action string, err error) {
	if err == nil {
		err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: action, ResourceType: "pbl", ResourceID: id})
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		h.pblFailure(w, r, err)
		return
	}
	_ = response.OK(w, map[string]string{"id": id})
}
