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

func (h *Handler) mentorQueue(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT mp.id::text,u.display_name,u.email_normalized,mp.bio,mp.expertise,mp.capacity,mp.status,EXISTS(SELECT 1 FROM external_college_access ea WHERE ea.college_id=mp.college_id AND ea.user_id=mp.user_id AND ea.access_kind='mentor' AND ea.revoked_at IS NULL),EXISTS(SELECT 1 FROM users campus_user WHERE campus_user.id=mp.user_id AND campus_user.college_id=mp.college_id) FROM mentor_profiles mp JOIN users u ON u.id=mp.user_id WHERE mp.college_id=$1 AND mp.status IN ('pending','active','suspended') ORDER BY mp.created_at,mp.id LIMIT 100`, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, email, bio, status string
		var expertise []string
		var capacity int
		var external, campusMember bool
		if err = rows.Scan(&id, &name, &email, &bio, &expertise, &capacity, &status, &external, &campusMember); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "email": email, "bio": bio, "expertise": expertise, "capacity": capacity, "status": status, "external_grant_active": external, "campus_member": campusMember})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) decideMentorProfile(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
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
	if in.Decision != "approved" && in.Decision != "rejected" && in.Decision != "suspended" && in.Decision != "revoked" {
		h.fail(w, r, http.StatusBadRequest, "mentor_decision_invalid", "mentor decision is invalid", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var userID string
	var external bool
	err = tx.QueryRow(r.Context(), `SELECT mp.user_id::text,EXISTS(SELECT 1 FROM external_college_access ea WHERE ea.college_id=mp.college_id AND ea.user_id=mp.user_id AND ea.access_kind='mentor' AND ea.revoked_at IS NULL) FROM mentor_profiles mp WHERE mp.id=$1 AND mp.college_id=$2 FOR UPDATE`, chi.URLParam(r, "id"), s.CollegeID).Scan(&userID, &external)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Decision == "approved" && !external {
		var sameCollege bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND college_id=$2)`, userID, s.CollegeID).Scan(&sameCollege); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if !sameCollege {
			h.fail(w, r, http.StatusConflict, "mentor_grant_required", "external mentors need an active college-scoped mentor invitation", nil)
			return
		}
	}
	newStatus := map[string]string{"approved": "active", "rejected": "rejected", "suspended": "suspended", "revoked": "revoked"}[in.Decision]
	var revokedAt any
	if in.Decision == "revoked" {
		revokedAt = time.Now()
	}
	_, err = tx.Exec(r.Context(), `UPDATE mentor_profiles SET status=$2,verified_by=CASE WHEN $2='active' THEN $3 ELSE verified_by END,verified_at=CASE WHEN $2='active' THEN now() ELSE verified_at END,revoked_by=CASE WHEN $2='revoked' THEN $3 ELSE NULL END,revoked_at=$4 WHERE id=$1 AND college_id=$5`, chi.URLParam(r, "id"), newStatus, s.UserID, revokedAt, s.CollegeID)
	if err == nil && in.Decision == "approved" && !external {
		_, err = tx.Exec(r.Context(), `INSERT INTO campus_roles(college_id,user_id,role,granted_by) VALUES($1,$2,'alumni_mentor',$3) ON CONFLICT DO NOTHING`, s.CollegeID, userID, s.UserID)
	}
	if err == nil && (in.Decision == "revoked" || in.Decision == "suspended") {
		_, err = tx.Exec(r.Context(), `UPDATE mentorship_requests SET state='revoked',closed_at=now(),closed_by=$3,close_reason='mentor_access_revoked' WHERE college_id=$1 AND mentor_profile_id=$2 AND state='accepted'`, s.CollegeID, chi.URLParam(r, "id"), s.UserID)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]string{"id": chi.URLParam(r, "id"), "status": newStatus})
}

func (h *Handler) getMentorProfile(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	var item map[string]any
	var id, bio, status string
	var expertise []string
	var capacity int
	err := h.pool.QueryRow(r.Context(), `SELECT id::text,bio,expertise,capacity,status FROM mentor_profiles WHERE college_id=$1 AND user_id=$2`, s.CollegeID, s.UserID).Scan(&id, &bio, &expertise, &capacity, &status)
	if err == pgx.ErrNoRows {
		_ = response.OK(w, map[string]any{"profile": nil})
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	item = map[string]any{"id": id, "bio": bio, "expertise": expertise, "capacity": capacity, "status": status}
	_ = response.OK(w, map[string]any{"profile": item})
}

func (h *Handler) saveMentorProfile(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Bio       string   `json:"bio"`
		Expertise []string `json:"expertise"`
		Capacity  int      `json:"capacity"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Bio = strings.TrimSpace(in.Bio)
	if len(in.Bio) > 2000 || len(in.Expertise) > 20 || in.Capacity < 1 || in.Capacity > 10 {
		h.fail(w, r, http.StatusBadRequest, "mentor_profile_invalid", "mentor profile is invalid", nil)
		return
	}
	for i := range in.Expertise {
		in.Expertise[i] = strings.TrimSpace(in.Expertise[i])
		if in.Expertise[i] == "" || len(in.Expertise[i]) > 80 {
			h.fail(w, r, http.StatusBadRequest, "mentor_profile_invalid", "expertise entries must be 1 to 80 characters", nil)
			return
		}
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO mentor_profiles(college_id,user_id,bio,expertise,capacity) VALUES($1,$2,$3,$4,$5) ON CONFLICT(college_id,user_id) DO UPDATE SET bio=EXCLUDED.bio,expertise=EXCLUDED.expertise,capacity=EXCLUDED.capacity,status=CASE WHEN mentor_profiles.status='rejected' THEN 'pending' ELSE mentor_profiles.status END WHERE EXCLUDED.capacity >= (SELECT count(*) FROM mentorship_requests WHERE mentor_profile_id=mentor_profiles.id AND state='accepted' AND ends_at>now()) RETURNING id::text`, s.CollegeID, s.UserID, in.Bio, in.Expertise, in.Capacity).Scan(&id)
	if err == pgx.ErrNoRows {
		h.fail(w, r, http.StatusConflict, "mentor_capacity_below_engagements", "capacity cannot be lower than current active engagements", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"id": id, "state": "pending_verification"})
}

func (h *Handler) listMentors(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT mp.id::text,u.display_name,mp.bio,mp.expertise,mp.capacity,(SELECT count(*) FROM mentorship_requests mr WHERE mr.mentor_profile_id=mp.id AND mr.state='accepted' AND mr.ends_at>now()) AS active_engagements FROM mentor_profiles mp JOIN users u ON u.id=mp.user_id WHERE mp.college_id=$1 AND mp.status='active' AND (mp.college_id=(SELECT college_id FROM users WHERE id=mp.user_id) OR EXISTS(SELECT 1 FROM external_college_access ea WHERE ea.user_id=mp.user_id AND ea.college_id=mp.college_id AND ea.access_kind='mentor' AND ea.revoked_at IS NULL)) ORDER BY u.display_name,mp.id LIMIT 100`, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, bio string
		var expertise []string
		var capacity, active int
		if err = rows.Scan(&id, &name, &bio, &expertise, &capacity, &active); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "bio": bio, "expertise": expertise, "capacity": capacity, "active_engagements": active})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) listMentorshipRequests(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT mr.id::text,p.title,u.display_name,mr.requester_id=$2 AS is_requester,mr.learning_goal,mr.state,mr.requested_at,mr.ends_at,COALESCE(mc.id::text,'') FROM mentorship_requests mr JOIN projects p ON p.id=mr.project_id AND p.college_id=mr.college_id JOIN mentor_profiles mp ON mp.id=mr.mentor_profile_id AND mp.college_id=mr.college_id JOIN users u ON u.id=mr.requester_id LEFT JOIN mentorship_conversations mc ON mc.mentorship_request_id=mr.id WHERE mr.college_id=$1 AND (mr.requester_id=$2 OR mp.user_id=$2) ORDER BY mr.requested_at DESC LIMIT 100`, s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, title, name, goal, state, conversation string
		var self bool
		var requested time.Time
		var ends *time.Time
		if err = rows.Scan(&id, &title, &name, &self, &goal, &state, &requested, &ends, &conversation); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "project": title, "requester": name, "is_requester": self, "learning_goal": goal, "state": state, "requested_at": requested, "ends_at": ends, "conversation_id": conversation})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) requestMentorship(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		MentorID     string `json:"mentor_id"`
		LearningGoal string `json:"learning_goal"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.LearningGoal = strings.TrimSpace(in.LearningGoal)
	if len(in.LearningGoal) < 10 || len(in.LearningGoal) > 2000 {
		h.fail(w, r, http.StatusBadRequest, "mentorship_request_invalid", "learning goal must be 10 to 2000 characters", nil)
		return
	}
	var member, active bool
	err = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM project_members pm JOIN projects p ON p.id=pm.project_id AND p.college_id=pm.college_id WHERE pm.project_id=$1 AND pm.user_id=$2 AND p.lifecycle='active' AND p.github_revoke_deadline IS NULL), EXISTS(SELECT 1 FROM mentor_profiles mp WHERE mp.id=$3 AND mp.college_id=$4 AND mp.user_id<>$2 AND mp.status='active' AND (mp.college_id=(SELECT college_id FROM users WHERE id=mp.user_id) OR EXISTS(SELECT 1 FROM external_college_access ea WHERE ea.user_id=mp.user_id AND ea.college_id=mp.college_id AND ea.access_kind='mentor' AND ea.revoked_at IS NULL)))`, chi.URLParam(r, "id"), s.UserID, in.MentorID, s.CollegeID).Scan(&member, &active)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !member || !active {
		h.fail(w, r, http.StatusForbidden, "mentorship_scope_denied", "active project membership and an available verified mentor are required", nil)
		return
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO mentorship_requests(college_id,project_id,mentor_profile_id,requester_id,learning_goal) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, s.CollegeID, chi.URLParam(r, "id"), in.MentorID, s.UserID, in.LearningGoal).Scan(&id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "state": "requested"})
}

func (h *Handler) decideMentorship(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Decision string    `json:"decision"`
		EndsAt   time.Time `json:"ends_at"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Decision != "accepted" && in.Decision != "declined" {
		h.fail(w, r, http.StatusBadRequest, "mentorship_decision_invalid", "decision must be accepted or declined", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var profileID, projectID string
	err = tx.QueryRow(r.Context(), `SELECT mp.id::text,mr.project_id::text FROM mentorship_requests mr JOIN mentor_profiles mp ON mp.id=mr.mentor_profile_id AND mp.college_id=mr.college_id WHERE mr.id=$1 AND mr.college_id=$2 AND mp.user_id=$3 AND mr.state='requested' AND mp.status='active' AND (mp.college_id=(SELECT college_id FROM users WHERE id=mp.user_id) OR EXISTS(SELECT 1 FROM external_college_access ea WHERE ea.user_id=mp.user_id AND ea.college_id=mp.college_id AND ea.access_kind='mentor' AND ea.revoked_at IS NULL)) FOR UPDATE OF mp,mr`, chi.URLParam(r, "id"), s.CollegeID, s.UserID).Scan(&profileID, &projectID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	state := in.Decision
	var conversationID string
	if in.Decision == "accepted" {
		if !in.EndsAt.After(time.Now()) || in.EndsAt.After(time.Now().AddDate(1, 0, 0)) {
			h.fail(w, r, http.StatusBadRequest, "mentorship_end_date_invalid", "end date must be within the next year", nil)
			return
		}
		var capacity, engaged int
		err = tx.QueryRow(r.Context(), `SELECT capacity,(SELECT count(*) FROM mentorship_requests WHERE mentor_profile_id=$1 AND state='accepted' AND ends_at>now()) FROM mentor_profiles WHERE id=$1 AND college_id=$2`, profileID, s.CollegeID).Scan(&capacity, &engaged)
		if err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if engaged >= capacity {
			h.fail(w, r, http.StatusConflict, "mentor_capacity_full", "mentor has no available engagement capacity", nil)
			return
		}
		_, err = tx.Exec(r.Context(), `UPDATE mentorship_requests SET state='accepted',accepted_at=now(),ends_at=$2 WHERE id=$1 AND state='requested'`, chi.URLParam(r, "id"), in.EndsAt)
		if err == nil {
			err = tx.QueryRow(r.Context(), `INSERT INTO mentorship_conversations(college_id,mentorship_request_id,project_id) VALUES($1,$2,$3) RETURNING id::text`, s.CollegeID, chi.URLParam(r, "id"), projectID).Scan(&conversationID)
		}
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO mentorship_conversation_members(college_id,conversation_id,user_id) SELECT $1::uuid,$2::uuid,mp.user_id FROM mentor_profiles mp WHERE mp.id=$3 UNION ALL SELECT $1::uuid,$2::uuid,mr.requester_id FROM mentorship_requests mr WHERE mr.id=$4`, s.CollegeID, conversationID, profileID, chi.URLParam(r, "id"))
		}
	} else {
		_, err = tx.Exec(r.Context(), `UPDATE mentorship_requests SET state='declined',closed_at=now(),closed_by=$2,close_reason='mentor_declined' WHERE id=$1 AND state='requested'`, chi.URLParam(r, "id"), s.UserID)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"state": state, "conversation_id": conversationID})
}

func (h *Handler) endMentorship(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	result, err := h.pool.Exec(r.Context(), `UPDATE mentorship_requests mr SET state=CASE WHEN mp.user_id=$3 AND mp.status='revoked' THEN 'revoked' ELSE 'ended' END,closed_at=now(),closed_by=$3,close_reason='ended_by_participant' FROM mentor_profiles mp WHERE mr.mentor_profile_id=mp.id AND mr.college_id=$2 AND mr.id=$1 AND mr.state='accepted' AND mr.ends_at>now() AND (mr.requester_id=$3 OR mp.user_id=$3) AND (mp.user_id<>$3 OR mp.college_id=(SELECT college_id FROM users WHERE id=mp.user_id) OR EXISTS(SELECT 1 FROM external_college_access ea WHERE ea.user_id=mp.user_id AND ea.college_id=mp.college_id AND ea.access_kind='mentor' AND ea.revoked_at IS NULL))`, chi.URLParam(r, "id"), s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if result.RowsAffected() == 0 {
		h.fail(w, r, http.StatusNotFound, "mentorship_not_found", "active mentorship was not found", nil)
		return
	}
	response.NoContent(w)
}

func (h *Handler) mentorshipMessages(w http.ResponseWriter, r *http.Request) {
	s, ok := h.mentorshipSession(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT mm.id::text,mm.sender_id::text,mm.body,mm.created_at FROM mentorship_messages mm JOIN mentorship_conversations mc ON mc.id=mm.conversation_id AND mc.college_id=mm.college_id JOIN mentorship_requests mr ON mr.id=mc.mentorship_request_id AND mr.college_id=mc.college_id JOIN mentor_profiles mp ON mp.id=mr.mentor_profile_id AND mp.college_id=mr.college_id WHERE mc.id=$1 AND mc.college_id=$2 AND mc.state='open' AND mr.state='accepted' AND mr.ends_at>now() AND mp.status='active' AND (mr.requester_id=$3 OR mp.user_id=$3) AND (mp.user_id<>$3 OR mp.college_id=(SELECT college_id FROM users WHERE id=mp.user_id) OR EXISTS(SELECT 1 FROM external_college_access ea WHERE ea.user_id=mp.user_id AND ea.college_id=mp.college_id AND ea.access_kind='mentor' AND ea.revoked_at IS NULL)) AND NOT EXISTS(SELECT 1 FROM mentorship_conversation_members cm JOIN mentorship_blocks b ON b.college_id=mc.college_id AND b.blocker_id=$3 AND b.blocked_id=cm.user_id WHERE cm.conversation_id=mc.id AND cm.user_id<>$3) AND NOT EXISTS(SELECT 1 FROM mentorship_conversation_members cm JOIN mentorship_blocks b ON b.college_id=mc.college_id AND b.blocker_id=cm.user_id AND b.blocked_id=$3 WHERE cm.conversation_id=mc.id AND cm.user_id<>$3) ORDER BY mm.created_at DESC,mm.id DESC LIMIT 100`, chi.URLParam(r, "id"), s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, sender, body string
		var created time.Time
		if err = rows.Scan(&id, &sender, &body, &created); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": id, "sender_id": sender, "body": body, "created_at": created})
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var mentorID, mentorName, requesterID, requesterName string
	err = h.pool.QueryRow(r.Context(), `SELECT mp.user_id::text,mentor.display_name,mr.requester_id::text,requester.display_name FROM mentorship_conversations mc JOIN mentorship_requests mr ON mr.id=mc.mentorship_request_id AND mr.college_id=mc.college_id JOIN mentor_profiles mp ON mp.id=mr.mentor_profile_id AND mp.college_id=mr.college_id JOIN users mentor ON mentor.id=mp.user_id JOIN users requester ON requester.id=mr.requester_id WHERE mc.id=$1 AND mc.college_id=$2 AND mc.state='open' AND mr.state='accepted' AND mr.ends_at>now() AND mp.status='active' AND (mr.requester_id=$3 OR mp.user_id=$3) AND (mp.user_id<>$3 OR mp.college_id=(SELECT college_id FROM users WHERE id=mp.user_id) OR EXISTS(SELECT 1 FROM external_college_access ea WHERE ea.user_id=mp.user_id AND ea.college_id=mp.college_id AND ea.access_kind='mentor' AND ea.revoked_at IS NULL))`, chi.URLParam(r, "id"), s.CollegeID, s.UserID).Scan(&mentorID, &mentorName, &requesterID, &requesterName)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items, "participants": []map[string]string{{"user_id": mentorID, "display_name": mentorName}, {"user_id": requesterID, "display_name": requesterName}}})
}

func (h *Handler) sendMentorshipMessage(w http.ResponseWriter, r *http.Request) {
	s, ok := h.mentorshipSession(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		ClientID string `json:"client_message_id"`
		Body     string `json:"body"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if len(in.Body) == 0 || len(in.Body) > 5000 || in.ClientID == "" {
		h.fail(w, r, http.StatusBadRequest, "message_invalid", "message text or client id is invalid", nil)
		return
	}
	var id string
	err = h.pool.QueryRow(r.Context(), `INSERT INTO mentorship_messages(college_id,conversation_id,sender_id,client_message_id,body) SELECT $1,mc.id,$3,$4,$5 FROM mentorship_conversations mc JOIN mentorship_requests mr ON mr.id=mc.mentorship_request_id AND mr.college_id=mc.college_id JOIN mentor_profiles mp ON mp.id=mr.mentor_profile_id AND mp.college_id=mr.college_id WHERE mc.id=$2 AND mc.college_id=$1 AND mc.state='open' AND mr.state='accepted' AND mr.ends_at>now() AND mp.status='active' AND (mr.requester_id=$3 OR mp.user_id=$3) AND (mp.user_id<>$3 OR mp.college_id=(SELECT college_id FROM users WHERE id=mp.user_id) OR EXISTS(SELECT 1 FROM external_college_access ea WHERE ea.user_id=mp.user_id AND ea.college_id=mp.college_id AND ea.access_kind='mentor' AND ea.revoked_at IS NULL)) AND NOT EXISTS(SELECT 1 FROM mentorship_conversation_members cm JOIN mentorship_blocks b ON b.college_id=mc.college_id AND b.blocker_id=$3 AND b.blocked_id=cm.user_id WHERE cm.conversation_id=mc.id AND cm.user_id<>$3) AND NOT EXISTS(SELECT 1 FROM mentorship_conversation_members cm JOIN mentorship_blocks b ON b.college_id=mc.college_id AND b.blocker_id=cm.user_id AND b.blocked_id=$3 WHERE cm.conversation_id=mc.id AND cm.user_id<>$3) ON CONFLICT(conversation_id,sender_id,client_message_id) DO UPDATE SET body=mentorship_messages.body RETURNING id::text`, s.CollegeID, chi.URLParam(r, "id"), s.UserID, in.ClientID, in.Body).Scan(&id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id})
}

func (h *Handler) blockMentorshipParticipant(w http.ResponseWriter, r *http.Request) {
	s, ok := h.mentorshipSession(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var mentorID, requesterID, requestID string
	err = tx.QueryRow(r.Context(), `SELECT mp.user_id::text,mr.requester_id::text,mr.id::text FROM mentorship_conversations mc JOIN mentorship_requests mr ON mr.id=mc.mentorship_request_id AND mr.college_id=mc.college_id JOIN mentor_profiles mp ON mp.id=mr.mentor_profile_id AND mp.college_id=mr.college_id JOIN mentorship_conversation_members cm ON cm.conversation_id=mc.id AND cm.college_id=mc.college_id AND cm.user_id=$3 WHERE mc.id=$1 AND mc.college_id=$2`, chi.URLParam(r, "id"), s.CollegeID, s.UserID).Scan(&mentorID, &requesterID, &requestID)
	if err != nil {
		h.fail(w, r, http.StatusNotFound, "mentorship_conversation_not_found", "conversation was not found", err)
		return
	}
	other := mentorID
	if other == s.UserID {
		other = requesterID
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO mentorship_blocks(college_id,blocker_id,blocked_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, s.CollegeID, s.UserID, other); err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE mentorship_requests SET state='revoked',closed_at=now(),closed_by=$2,close_reason='participant_blocked' WHERE id=$1 AND state='accepted'`, requestID, s.UserID)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]bool{"blocked": true, "conversation_closed": true})
}

func (h *Handler) reportMentorshipMessage(w http.ResponseWriter, r *http.Request) {
	s, ok := h.mentorshipSession(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		MessageID string `json:"message_id"`
		Reason    string `json:"reason"`
		Details   string `json:"details"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Reason != "spam" && in.Reason != "harassment" && in.Reason != "unsafe" && in.Reason != "other" || len(in.Details) > 1000 {
		h.fail(w, r, http.StatusBadRequest, "report_invalid", "report reason or details are invalid", nil)
		return
	}
	tag, err := h.pool.Exec(r.Context(), `INSERT INTO mentorship_message_reports(college_id,conversation_id,message_id,reporter_id,reason,details) SELECT mc.college_id,mc.id,mm.id,$4,$5,$6 FROM mentorship_conversations mc JOIN mentorship_conversation_members cm ON cm.conversation_id=mc.id AND cm.college_id=mc.college_id AND cm.user_id=$4 JOIN mentorship_messages mm ON mm.id=$3 AND mm.conversation_id=mc.id AND mm.college_id=mc.college_id WHERE mc.id=$1 AND mc.college_id=$2 ON CONFLICT(message_id,reporter_id) DO NOTHING`, chi.URLParam(r, "id"), s.CollegeID, in.MessageID, s.UserID, in.Reason, strings.TrimSpace(in.Details))
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		h.fail(w, r, http.StatusNotFound, "message_not_found", "message was not found in this conversation", nil)
		return
	}
	_ = response.Created(w, map[string]bool{"reported": true})
}
