package academic

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"peergit/internal/platform/audit"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
)

func (h *Handler) exportPBLAudit(w http.ResponseWriter, r *http.Request) {
	s, ok := h.pblSession(w, r)
	if !ok {
		return
	}
	team := chi.URLParam(r, "team")
	if !pblID(team) {
		h.pblFailure(w, r, pgx.ErrNoRows)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		h.pblFailure(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var confirmedTeam string
	err = tx.QueryRow(ctx, `SELECT t.id::text FROM pbl_teams t WHERE t.id=$1 AND t.college_id=$2 AND t.confirmed_at IS NOT NULL AND
		($4 OR EXISTS(SELECT 1 FROM pbl_active_faculty f WHERE f.team_id=t.id AND f.user_id=$3))`,
		team, s.CollegeID, s.UserID, s.CampusAdmin && s.MFAEnabled && s.MFAFresh).Scan(&confirmedTeam)
	if err != nil {
		h.pblFailure(w, r, err)
		return
	}
	rows, err := tx.Query(ctx, `SELECT category,person,stage,result,revision,COALESCE(happened::text,''),checksum FROM (
		SELECT 'idea' AS category,'' AS person,'0' AS stage,t.idea_state AS result,'' AS revision,NULL::timestamptz AS happened,'' AS checksum FROM pbl_teams t WHERE t.id=$1
		UNION ALL SELECT 'attendance',u.display_name,a.stage::text,CASE WHEN a.present THEN 'present' ELSE 'absent' END,'',NULL,''
			FROM pbl_attendance a JOIN users u ON u.id=a.student_id AND u.college_id=a.college_id WHERE a.team_id=$1
		UNION ALL SELECT 'checkpoint',u.display_name,c.stage::text,s.state,s.commit_sha,s.receipt_at,COALESCE(s.archive_sha256,'')
			FROM pbl_checkpoints c JOIN repository_snapshots s ON s.id=c.snapshot_id AND s.college_id=c.college_id
			JOIN users u ON u.id=s.requested_by AND u.college_id=s.college_id WHERE c.team_id=$1
		UNION ALL SELECT 'authenticated_push',p.display_name,'',g.ref,g.oid,g.observed_at,''
			FROM (SELECT * FROM pbl_git_pushes WHERE team_id=$1 ORDER BY observed_at DESC,id DESC LIMIT 500) g
			JOIN users p ON p.id=g.user_id AND p.college_id=g.college_id
		UNION ALL SELECT CASE WHEN u.id IS NOT NULL THEN 'verified_author' ELSE 'unattributed_commit' END,
			COALESCE(u.display_name,a.author_name),'',a.title,a.canonical_id,a.occurred_at,''
			FROM (SELECT x.* FROM pbl_provisions n JOIN repositories p ON p.project_id=n.project_id AND p.college_id=n.college_id
				JOIN contributions x ON x.repository_id=p.id AND x.college_id=p.college_id AND x.kind='commit'
				WHERE n.team_id=$1 ORDER BY x.occurred_at DESC,x.id DESC LIMIT 500) a
			LEFT JOIN users u ON u.college_id=a.college_id AND u.id=NULLIF(a.canonical_data->>'verified_author_id','')::uuid
		) evidence ORDER BY category,stage,happened,person`, confirmedTeam)
	if err != nil {
		h.pblFailure(w, r, err)
		return
	}
	var out strings.Builder
	writer := csv.NewWriter(&out)
	_ = writer.Write([]string{"category", "person", "stage", "result", "revision", "timestamp", "archive_sha256"})
	for rows.Next() {
		record := make([]string, 7)
		if err = rows.Scan(&record[0], &record[1], &record[2], &record[3], &record[4], &record[5], &record[6]); err != nil {
			break
		}
		for i := range record {
			record[i] = safeCSV(record[i])
		}
		_ = writer.Write(record)
	}
	rows.Close()
	if err == nil {
		err = rows.Err()
	}
	writer.Flush()
	if err == nil {
		err = writer.Error()
	}
	if err == nil {
		err = audit.Append(ctx, tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "pbl.audit_exported", ResourceType: "pbl_team", ResourceID: team})
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		h.pblFailure(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pbl-audit.csv"`)
	_, _ = w.Write([]byte(out.String()))
}

func (h *Handler) createExport(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Type           string `json:"type"`
		CallID         string `json:"call_id"`
		CohortID       string `json:"cohort_id"`
		IdempotencyKey string `json:"idempotency_key"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Type != "call" && in.Type != "cohort" || in.IdempotencyKey == "" || len(in.IdempotencyKey) > 200 {
		h.fail(w, r, http.StatusBadRequest, "export_invalid", "export type or idempotency key is invalid", nil)
		return
	}
	if s.CampusAdmin {
		if _, ok = h.admin(w, r); !ok {
			return
		}
	}
	var authorized bool
	if in.Type == "call" {
		err = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM academic_calls c WHERE c.id=$1 AND c.college_id=$2 AND (c.created_by=$3 OR $4 OR EXISTS(SELECT 1 FROM academic_call_reviewers cr JOIN faculty_profiles fp ON fp.user_id=cr.reviewer_id AND fp.college_id=c.college_id AND fp.status='active' WHERE cr.call_id=c.id AND cr.reviewer_id=$3)))`, in.CallID, s.CollegeID, s.UserID, s.CampusAdmin).Scan(&authorized)
	} else {
		err = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM academic_program_cohorts pc JOIN academic_calls c ON c.id=pc.call_id AND c.college_id=pc.college_id WHERE pc.id=$1 AND pc.college_id=$2 AND (c.created_by=$3 OR $4 OR EXISTS(SELECT 1 FROM academic_call_reviewers cr JOIN faculty_profiles fp ON fp.user_id=cr.reviewer_id AND fp.college_id=c.college_id AND fp.status='active' WHERE cr.call_id=c.id AND cr.reviewer_id=$3)))`, in.CohortID, s.CollegeID, s.UserID, s.CampusAdmin).Scan(&authorized)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !authorized {
		h.fail(w, r, http.StatusNotFound, "export_scope_not_found", "call or program cohort is not available for export", nil)
		return
	}
	bodyHash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s", in.Type, in.CallID, in.CohortID)))
	var call, cohort any
	if in.CallID != "" {
		call = in.CallID
	}
	if in.CohortID != "" {
		cohort = in.CohortID
	}
	data, err := h.buildCSV(r, in.Type, in.CallID, in.CohortID, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	digest := sha256.Sum256(data)
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO academic_exports(college_id,requested_by,export_type,call_id,cohort_id,idempotency_key,request_hash,state,csv_content,csv_sha256,completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,'ready',$8,$9,now()) ON CONFLICT(college_id,requested_by,idempotency_key) DO NOTHING RETURNING id::text`, s.CollegeID, s.UserID, in.Type, call, cohort, in.IdempotencyKey, bodyHash[:], data, hex.EncodeToString(digest[:])).Scan(&id)
	if err == pgx.ErrNoRows {
		var existingHash []byte
		var state string
		err = h.pool.QueryRow(r.Context(), `SELECT id::text,request_hash,state FROM academic_exports WHERE college_id=$1 AND requested_by=$2 AND idempotency_key=$3`, s.CollegeID, s.UserID, in.IdempotencyKey).Scan(&id, &existingHash, &state)
		if err == nil && !bytes.Equal(existingHash, bodyHash[:]) {
			h.fail(w, r, http.StatusConflict, "export_idempotency_conflict", "idempotency key was already used for a different export", nil)
			return
		}
		if err == nil && state != "ready" {
			_, err = h.pool.Exec(r.Context(), `UPDATE academic_exports SET state='ready',csv_content=$2,csv_sha256=$3,completed_at=now() WHERE id=$1 AND college_id=$4 AND requested_by=$5`, id, data, hex.EncodeToString(digest[:]), s.CollegeID, s.UserID)
		}
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]string{"id": id, "state": "ready"})
}

func (h *Handler) buildCSV(r *http.Request, kind, callID, cohortID, collegeID string) ([]byte, error) {
	var rows pgx.Rows
	var err error
	if kind == "call" {
		rows, err = h.pool.Query(r.Context(), `SELECT p.slug,a.state,COALESCE(u.display_name,''),to_char(a.submitted_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM academic_call_applications a JOIN projects p ON p.id=a.project_id AND p.college_id=a.college_id JOIN users u ON u.id=a.applicant_id WHERE a.call_id=$1 AND a.college_id=$2 ORDER BY a.submitted_at,a.id LIMIT 500`, callID, collegeID)
	} else {
		rows, err = h.pool.Query(r.Context(), `SELECT p.slug,o.summary,o.result,COALESCE(s.snapshot_id::text,'') FROM academic_program_outcomes o JOIN projects p ON p.id=o.project_id AND p.college_id=o.college_id LEFT JOIN academic_submissions s ON s.id=o.evidence_submission_id AND s.college_id=o.college_id WHERE o.cohort_id=$1 AND o.college_id=$2 ORDER BY p.slug LIMIT 500`, cohortID, collegeID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out strings.Builder
	writer := csv.NewWriter(&out)
	if kind == "call" {
		_ = writer.Write([]string{"project_slug", "application_state", "applicant_name", "submitted_at"})
	} else {
		_ = writer.Write([]string{"project_slug", "outcome_summary", "result", "submission_id"})
	}
	for rows.Next() {
		var a, b, c string
		var fourth string
		if kind == "call" {
			if err = rows.Scan(&a, &b, &c, &fourth); err != nil {
				return nil, err
			}
			c = safeCSV(c)
			_ = writer.Write([]string{safeCSV(a), safeCSV(b), c, fourth})
		} else {
			if err = rows.Scan(&a, &b, &c, &fourth); err != nil {
				return nil, err
			}
			_ = writer.Write([]string{safeCSV(a), safeCSV(b), safeCSV(c), safeCSV(fourth)})
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	writer.Flush()
	if err = writer.Error(); err != nil {
		return nil, err
	}
	if out.Len() > 5<<20 {
		return nil, fmt.Errorf("export exceeds maximum size")
	}
	return []byte(out.String()), nil
}

func safeCSV(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "=") || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "@") || strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") {
		return "'" + value
	}
	return value
}

func (h *Handler) downloadExport(w http.ResponseWriter, r *http.Request) {
	s, ok := h.faculty(w, r)
	if !ok {
		return
	}
	if s.CampusAdmin {
		if _, ok = h.admin(w, r); !ok {
			return
		}
	}
	var body []byte
	var digest string
	err := h.pool.QueryRow(r.Context(), `SELECT e.csv_content,e.csv_sha256 FROM academic_exports e WHERE e.id=$1 AND e.college_id=$2 AND e.requested_by=$3 AND e.state='ready' AND e.expires_at>now() AND ((e.export_type='call' AND EXISTS(SELECT 1 FROM academic_calls c WHERE c.id=e.call_id AND c.college_id=e.college_id AND (c.created_by=$3 OR $4 OR EXISTS(SELECT 1 FROM academic_call_reviewers cr JOIN faculty_profiles fp ON fp.user_id=cr.reviewer_id AND fp.college_id=c.college_id AND fp.status='active' WHERE cr.call_id=c.id AND cr.reviewer_id=$3)))) OR (e.export_type='cohort' AND EXISTS(SELECT 1 FROM academic_program_cohorts pc JOIN academic_calls c ON c.id=pc.call_id AND c.college_id=pc.college_id WHERE pc.id=e.cohort_id AND pc.college_id=e.college_id AND (c.created_by=$3 OR $4 OR EXISTS(SELECT 1 FROM academic_call_reviewers cr JOIN faculty_profiles fp ON fp.user_id=cr.reviewer_id AND fp.college_id=c.college_id AND fp.status='active' WHERE cr.call_id=c.id AND cr.reviewer_id=$3)))))`, chi.URLParam(r, "id"), s.CollegeID, s.UserID, s.CampusAdmin).Scan(&body, &digest)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != digest {
		h.fail(w, r, http.StatusInternalServerError, "export_integrity_failed", "export integrity check failed", nil)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=peergit-academic-export.csv")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h *Handler) saveDigestPreferences(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Cadence string `json:"cadence"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Cadence != "off" && in.Cadence != "weekly" {
		h.fail(w, r, http.StatusBadRequest, "digest_preference_invalid", "digest cadence must be weekly or off", nil)
		return
	}
	_, err = h.pool.Exec(r.Context(), `INSERT INTO academic_digest_preferences(college_id,user_id,cadence) VALUES($1,$2,$3) ON CONFLICT(college_id,user_id) DO UPDATE SET cadence=EXCLUDED.cadence,updated_at=now()`, s.CollegeID, s.UserID, in.Cadence)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]string{"cadence": in.Cadence})
}

func (h *Handler) academicDigest(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	var cadence string
	err := h.pool.QueryRow(r.Context(), `SELECT COALESCE((SELECT cadence FROM academic_digest_preferences WHERE college_id=$1 AND user_id=$2),'weekly')`, s.CollegeID, s.UserID).Scan(&cadence)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var assignmentsDue, reviewsPending, mentorshipRequests int
	err = h.pool.QueryRow(r.Context(), `SELECT
		(SELECT count(*) FROM academic_assignments a JOIN academic_cohorts co ON co.id=a.cohort_id AND co.college_id=a.college_id JOIN academic_enrollments e ON e.cohort_id=co.id AND e.user_id=$2 AND e.status='active' WHERE a.college_id=$1 AND a.status='open' AND a.due_at BETWEEN now() AND now()+interval '7 days'),
		(SELECT count(*) FROM academic_call_applications a JOIN academic_call_reviewers cr ON cr.call_id=a.call_id AND cr.reviewer_id=$2 WHERE a.college_id=$1 AND a.state IN ('submitted','under_review') AND a.submitted_at>=now()-interval '7 days'),
		(SELECT count(*) FROM mentorship_requests mr JOIN mentor_profiles mp ON mp.id=mr.mentor_profile_id AND mp.college_id=mr.college_id WHERE mr.college_id=$1 AND mp.user_id=$2 AND mr.state='requested' AND (mp.college_id=(SELECT college_id FROM users WHERE id=$2) OR EXISTS(SELECT 1 FROM external_college_access ea WHERE ea.user_id=$2 AND ea.college_id=$1 AND ea.access_kind='mentor' AND ea.revoked_at IS NULL)))`, s.CollegeID, s.UserID).Scan(&assignmentsDue, &reviewsPending, &mentorshipRequests)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"cadence": cadence, "period_days": 7, "summary": map[string]int{"assignments_due": assignmentsDue, "reviews_pending": reviewsPending, "mentorship_requests": mentorshipRequests}, "includes_message_text": false})
}
