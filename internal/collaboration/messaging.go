package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"peergit/internal/identity"
	"peergit/internal/platform/audit"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
	"peergit/internal/platform/outbox"
)

type conversationContext struct {
	ID            string
	Type          string
	ProjectID     string
	ApplicationID string
	State         string
}

func (h *Handler) projectConversation(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	conversation, err := h.openConversation(r.Context(), s, "project", chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "conversation_context_unavailable", "project conversation is not available to this account")
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": conversation.ID, "context_type": conversation.Type, "project_id": conversation.ProjectID, "state": conversation.State})
}

func (h *Handler) applicationConversation(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	conversation, err := h.openConversation(r.Context(), s, "application", chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "conversation_context_unavailable", "an accepted application conversation is not available to this account")
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": conversation.ID, "context_type": conversation.Type, "project_id": conversation.ProjectID, "application_id": conversation.ApplicationID, "state": conversation.State})
}

func (h *Handler) openConversation(ctx context.Context, s identity.Session, kind, targetID string) (conversationContext, error) {
	if !uuidOK(targetID) {
		return conversationContext{}, pgx.ErrNoRows
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return conversationContext{}, err
	}
	defer tx.Rollback(ctx)
	var conversation conversationContext
	switch kind {
	case "project":
		err = tx.QueryRow(ctx, `SELECT p.id::text FROM projects p JOIN project_members pm ON pm.project_id=p.id AND pm.college_id=p.college_id
			WHERE p.id=$1 AND p.college_id=$2 AND pm.user_id=$3 AND p.lifecycle<>'archived' AND p.github_revoke_deadline IS NULL`, targetID, s.CollegeID, s.UserID).Scan(&conversation.ProjectID)
		if err != nil {
			return conversationContext{}, err
		}
		err = tx.QueryRow(ctx, `INSERT INTO conversations(college_id,context_type,project_id)
			VALUES($1,'project',$2) ON CONFLICT(college_id,project_id) WHERE context_type='project'
			DO UPDATE SET project_id=EXCLUDED.project_id RETURNING id::text,context_type,project_id::text,COALESCE(application_id::text,''),state`, s.CollegeID, targetID).
			Scan(&conversation.ID, &conversation.Type, &conversation.ProjectID, &conversation.ApplicationID, &conversation.State)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO conversation_participants(college_id,conversation_id,user_id)
				SELECT $1,$2,user_id FROM project_members WHERE project_id=$3 AND college_id=$1 ON CONFLICT DO NOTHING`, s.CollegeID, conversation.ID, targetID)
		}
	case "application":
		err = tx.QueryRow(ctx, `SELECT r.project_id::text FROM applications a
			JOIN project_roles r ON r.id=a.project_role_id AND r.college_id=a.college_id
			WHERE a.id=$1 AND a.college_id=$2 AND a.status='accepted' AND a.decided_at IS NOT NULL
			AND (a.applicant_user_id=$3 OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=r.project_id AND pm.college_id=r.college_id AND pm.user_id=$3 AND pm.role IN ('owner','maintainer')))`, targetID, s.CollegeID, s.UserID).Scan(&conversation.ProjectID)
		if err != nil {
			return conversationContext{}, err
		}
		err = tx.QueryRow(ctx, `INSERT INTO conversations(college_id,context_type,application_id,project_id)
			VALUES($1,'application',$2,$3) ON CONFLICT(college_id,application_id) WHERE context_type='application'
			DO UPDATE SET application_id=EXCLUDED.application_id RETURNING id::text,context_type,project_id::text,application_id::text,state`, s.CollegeID, targetID, conversation.ProjectID).
			Scan(&conversation.ID, &conversation.Type, &conversation.ProjectID, &conversation.ApplicationID, &conversation.State)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO conversation_participants(college_id,conversation_id,user_id)
				SELECT $1::uuid,$2::uuid,a.applicant_user_id FROM applications a WHERE a.id=$3 AND a.college_id=$1
				UNION SELECT $1::uuid,$2::uuid,pm.user_id FROM project_members pm WHERE pm.project_id=$4 AND pm.college_id=$1 AND pm.role IN ('owner','maintainer')
				ON CONFLICT DO NOTHING`, s.CollegeID, conversation.ID, targetID, conversation.ProjectID)
		}
	default:
		return conversationContext{}, pgx.ErrNoRows
	}
	if err != nil {
		return conversationContext{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return conversationContext{}, err
	}
	return conversation, nil
}

func (h *Handler) conversationForUser(ctx context.Context, s identity.Session, id string) (conversationContext, error) {
	if !uuidOK(id) {
		return conversationContext{}, pgx.ErrNoRows
	}
	var conversation conversationContext
	err := h.pool.QueryRow(ctx, `SELECT c.id::text,c.context_type,COALESCE(c.project_id::text,''),COALESCE(c.application_id::text,''),c.state
		FROM conversations c
		WHERE c.id=$1 AND c.college_id=$2 AND c.state='open'
		AND ((c.context_type='project' AND EXISTS (
			SELECT 1 FROM project_members pm JOIN projects p ON p.id=pm.project_id AND p.college_id=pm.college_id
			WHERE pm.project_id=c.project_id AND pm.college_id=c.college_id AND pm.user_id=$3 AND p.lifecycle<>'archived' AND p.github_revoke_deadline IS NULL
		)) OR (c.context_type='application' AND EXISTS (
			SELECT 1 FROM applications a JOIN project_roles rr ON rr.id=a.project_role_id AND rr.college_id=a.college_id
			JOIN projects p ON p.id=rr.project_id AND p.college_id=rr.college_id
			WHERE a.id=c.application_id AND a.college_id=c.college_id AND a.status='accepted' AND p.github_revoke_deadline IS NULL
			AND (a.applicant_user_id=$3 OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=rr.project_id AND pm.college_id=rr.college_id AND pm.user_id=$3 AND pm.role IN ('owner','maintainer')))
		)))`, id, s.CollegeID, s.UserID).Scan(&conversation.ID, &conversation.Type, &conversation.ProjectID, &conversation.ApplicationID, &conversation.State)
	return conversation, err
}

func (h *Handler) syncConversationParticipants(ctx context.Context, conversation conversationContext, collegeID string) error {
	if conversation.Type == "project" {
		_, err := h.pool.Exec(ctx, `DELETE FROM conversation_participants cp WHERE cp.conversation_id=$1 AND NOT EXISTS (
			SELECT 1 FROM project_members pm WHERE pm.project_id=$2 AND pm.college_id=$3 AND pm.user_id=cp.user_id
		)`, conversation.ID, conversation.ProjectID, collegeID)
		if err != nil {
			return err
		}
		_, err = h.pool.Exec(ctx, `INSERT INTO conversation_participants(college_id,conversation_id,user_id)
			SELECT $1,$2,user_id FROM project_members WHERE project_id=$3 AND college_id=$1 ON CONFLICT DO NOTHING`, collegeID, conversation.ID, conversation.ProjectID)
		return err
	}
	_, err := h.pool.Exec(ctx, `DELETE FROM conversation_participants cp WHERE cp.conversation_id=$1 AND NOT (
		cp.user_id=(SELECT a.applicant_user_id FROM applications a WHERE a.id=$2 AND a.college_id=$3 AND a.status='accepted') OR
		EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=$4 AND pm.college_id=$3 AND pm.user_id=cp.user_id AND pm.role IN ('owner','maintainer'))
	)`, conversation.ID, conversation.ApplicationID, collegeID, conversation.ProjectID)
	if err != nil {
		return err
	}
	_, err = h.pool.Exec(ctx, `INSERT INTO conversation_participants(college_id,conversation_id,user_id)
		SELECT $1::uuid,$2::uuid,a.applicant_user_id FROM applications a WHERE a.id=$3 AND a.college_id=$1 AND a.status='accepted'
		UNION SELECT $1::uuid,$2::uuid,pm.user_id FROM project_members pm WHERE pm.project_id=$4 AND pm.college_id=$1 AND pm.role IN ('owner','maintainer')
		ON CONFLICT DO NOTHING`, collegeID, conversation.ID, conversation.ApplicationID, conversation.ProjectID)
	return err
}

func (h *Handler) listConversations(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT c.id::text,c.context_type,COALESCE(c.project_id::text,''),COALESCE(c.application_id::text,''),p.title,
		COALESCE((SELECT max(m.created_at) FROM conversation_messages m WHERE m.conversation_id=c.id),c.created_at)
		FROM conversations c JOIN conversation_participants cp ON cp.conversation_id=c.id AND cp.college_id=c.college_id
		LEFT JOIN projects p ON p.id=c.project_id AND p.college_id=c.college_id
		WHERE c.college_id=$1 AND c.state='open' AND cp.user_id=$2 AND (
			(c.context_type='project' AND EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=c.project_id AND pm.college_id=c.college_id AND pm.user_id=$2)) OR
			(c.context_type='application' AND EXISTS(SELECT 1 FROM applications a JOIN project_roles rr ON rr.id=a.project_role_id AND rr.college_id=a.college_id WHERE a.id=c.application_id AND a.college_id=c.college_id AND a.status='accepted' AND (a.applicant_user_id=$2 OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=rr.project_id AND pm.college_id=rr.college_id AND pm.user_id=$2 AND pm.role IN ('owner','maintainer')))))
		) ORDER BY 6 DESC,c.id DESC LIMIT 100`, s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID            string    `json:"id"`
		ContextType   string    `json:"context_type"`
		ProjectID     string    `json:"project_id"`
		ApplicationID string    `json:"application_id,omitempty"`
		ProjectTitle  string    `json:"project_title"`
		LastMessageAt time.Time `json:"last_message_at"`
	}
	items := []item{}
	for rows.Next() {
		var value item
		if err = rows.Scan(&value.ID, &value.ContextType, &value.ProjectID, &value.ApplicationID, &value.ProjectTitle, &value.LastMessageAt); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, value)
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	conversation, err := h.conversationForUser(r.Context(), s, id)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "conversation_unavailable", "this conversation is closed or no longer available to this account")
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.syncConversationParticipants(r.Context(), conversation, s.CollegeID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	participants, err := h.conversationParticipants(r.Context(), conversation.ID, s)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	point, ok := h.decodeCursor(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT m.id::text,m.sender_id::text,u.display_name,m.body,m.created_at
		FROM conversation_messages m JOIN conversation_participants cp ON cp.conversation_id=m.conversation_id AND cp.user_id=$3 AND cp.college_id=m.college_id
		JOIN conversations c ON c.id=m.conversation_id AND c.college_id=m.college_id AND c.state='open'
		JOIN users u ON u.id=m.sender_id AND u.college_id=m.college_id AND u.status='active'
		WHERE m.college_id=$1 AND m.conversation_id=$2 AND ($4::timestamptz IS NULL OR (m.created_at,m.id)<($4,$5::uuid))
		AND CASE c.context_type
			WHEN 'project' THEN EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=c.project_id AND pm.college_id=c.college_id AND pm.user_id=$3)
			WHEN 'application' THEN EXISTS(SELECT 1 FROM applications a JOIN project_roles rr ON rr.id=a.project_role_id AND rr.college_id=a.college_id
				WHERE a.id=c.application_id AND a.college_id=c.college_id AND a.status='accepted'
				AND (a.applicant_user_id=$3 OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=rr.project_id AND pm.college_id=rr.college_id AND pm.user_id=$3 AND pm.role IN ('owner','maintainer'))))
			ELSE false
		END
		AND NOT EXISTS(SELECT 1 FROM user_blocks b WHERE b.college_id=$1 AND ((b.blocker_id=$3 AND b.blocked_id=m.sender_id) OR (b.blocker_id=m.sender_id AND b.blocked_id=$3)))
		ORDER BY m.created_at DESC,m.id DESC LIMIT $6`, s.CollegeID, id, s.UserID, nullableTimePoint(point), nullableIDPoint(point), discoveryPageSize+1)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	type message struct {
		ID         string    `json:"id"`
		SenderID   string    `json:"sender_id"`
		SenderName string    `json:"sender_name"`
		Body       string    `json:"body"`
		CreatedAt  time.Time `json:"created_at"`
	}
	items := []message{}
	for rows.Next() {
		var value message
		if err = rows.Scan(&value.ID, &value.SenderID, &value.SenderName, &value.Body, &value.CreatedAt); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, value)
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var next any
	if len(items) > discoveryPageSize {
		last := items[discoveryPageSize-1]
		token, encodeErr := h.encodeCursor(discoveryCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if encodeErr != nil {
			h.err.Handle(w, r, encodeErr)
			return
		}
		next = token
		items = items[:discoveryPageSize]
	}
	_ = response.OK(w, map[string]any{"items": items, "next_cursor": next, "context_type": conversation.Type, "project_id": conversation.ProjectID, "application_id": conversation.ApplicationID, "state": conversation.State, "participants": participants})
}

func (h *Handler) conversationParticipants(ctx context.Context, conversationID string, s identity.Session) ([]map[string]string, error) {
	rows, err := h.pool.Query(ctx, `SELECT u.id::text,u.handle,u.display_name FROM conversation_participants cp
		JOIN users u ON u.id=cp.user_id AND u.college_id=cp.college_id AND u.status='active'
		WHERE cp.conversation_id=$1 AND cp.college_id=$2
		AND NOT EXISTS(SELECT 1 FROM user_blocks b WHERE b.college_id=$2 AND ((b.blocker_id=$3 AND b.blocked_id=cp.user_id) OR (b.blocker_id=cp.user_id AND b.blocked_id=$3)))
		ORDER BY (cp.user_id=$3) DESC,u.display_name`, conversationID, s.CollegeID, s.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, handle, name string
		if err = rows.Scan(&id, &handle, &name); err != nil {
			return nil, err
		}
		items = append(items, map[string]string{"id": id, "handle": handle, "display_name": name})
	}
	return items, rows.Err()
}

func (h *Handler) createMessage(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	conversation, err := h.conversationForUser(r.Context(), s, id)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusConflict, "conversation_closed", "this context is closed or no longer available for messaging")
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.syncConversationParticipants(r.Context(), conversation, s.CollegeID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in, err := request.Decode[struct {
		ClientMessageID string `json:"client_message_id"`
		Body            string `json:"body"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if !uuidOK(in.ClientMessageID) || invalidText(in.Body, 1, 5000) {
		fail(w, http.StatusBadRequest, "message_invalid", "message text and a unique client message ID are required")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var messageID string
	err = tx.QueryRow(r.Context(), `INSERT INTO conversation_messages(college_id,conversation_id,sender_id,client_message_id,body)
		SELECT $1,c.id,$3,$4,$5 FROM conversations c WHERE c.id=$2 AND c.college_id=$1 AND c.state='open'
		AND EXISTS(SELECT 1 FROM conversation_participants WHERE college_id=$1 AND conversation_id=$2 AND user_id=$3)
		AND CASE c.context_type
			WHEN 'project' THEN EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=c.project_id AND pm.college_id=c.college_id AND pm.user_id=$3)
			WHEN 'application' THEN EXISTS(SELECT 1 FROM applications a JOIN project_roles rr ON rr.id=a.project_role_id AND rr.college_id=a.college_id
				WHERE a.id=c.application_id AND a.college_id=c.college_id AND a.status='accepted'
				AND (a.applicant_user_id=$3 OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=rr.project_id AND pm.college_id=rr.college_id AND pm.user_id=$3 AND pm.role IN ('owner','maintainer'))))
			ELSE false
		END
		ON CONFLICT(conversation_id,sender_id,client_message_id) DO NOTHING RETURNING id::text`, s.CollegeID, id, s.UserID, in.ClientMessageID, in.Body).Scan(&messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingBody string
		err = tx.QueryRow(r.Context(), `SELECT id::text,body FROM conversation_messages WHERE college_id=$1 AND conversation_id=$2 AND sender_id=$3 AND client_message_id=$4`, s.CollegeID, id, s.UserID, in.ClientMessageID).Scan(&messageID, &existingBody)
		if err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if existingBody != in.Body {
			fail(w, http.StatusConflict, "message_idempotency_conflict", "this message ID was already used with different text")
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		_ = response.OK(w, map[string]any{"id": messageID, "duplicate": true})
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	payload, _ := json.Marshal(map[string]string{"conversation_id": id, "actor_id": s.UserID, "project_id": conversation.ProjectID, "application_id": conversation.ApplicationID})
	err = outbox.Append(r.Context(), tx, outbox.Event{TenantID: s.CollegeID, AggregateType: "conversation_message", AggregateID: messageID, EventType: "conversation.message.created", Version: 1, Payload: payload}, []string{"project.activity"})
	if err == nil {
		err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "conversation.message.created", ResourceType: "conversation", ResourceID: id})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": messageID, "duplicate": false, "created_at": time.Now().UTC()})
}

func (h *Handler) reportMessage(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	conversation, err := h.conversationForUser(r.Context(), s, chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "conversation_unavailable", "this conversation is no longer available to this account")
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
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
	in.Details = strings.TrimSpace(in.Details)
	if !uuidOK(in.MessageID) || !validReportReason(in.Reason) || len(in.Details) > 1000 {
		fail(w, http.StatusBadRequest, "message_report_invalid", "report reason, message, or details are invalid")
		return
	}
	tag, err := h.pool.Exec(r.Context(), `INSERT INTO message_reports(college_id,conversation_id,message_id,reporter_id,reason,details)
		SELECT $1,$2,m.id,$3,$4,$5 FROM conversation_messages m
		WHERE m.id=$6 AND m.conversation_id=$2 AND m.college_id=$1 AND m.sender_id<>$3 ON CONFLICT(message_id,reporter_id) DO NOTHING`, s.CollegeID, conversation.ID, s.UserID, in.Reason, in.Details, in.MessageID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, http.StatusConflict, "message_report_exists_or_missing", "that message cannot be reported again from this account")
		return
	}
	_ = response.Created(w, map[string]any{"reported": true, "review_state": "awaiting_moderation"})
}

func (h *Handler) blockUser(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	target := chi.URLParam(r, "userID")
	if !uuidOK(target) || target == s.UserID {
		fail(w, http.StatusBadRequest, "block_target_invalid", "choose another active campus account")
		return
	}
	result, err := h.pool.Exec(r.Context(), `INSERT INTO user_blocks(college_id,blocker_id,blocked_id)
		SELECT $1,$2,$3 WHERE EXISTS(SELECT 1 FROM users WHERE id=$3 AND college_id=$1 AND status='active') ON CONFLICT DO NOTHING`, s.CollegeID, s.UserID, target)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if result.RowsAffected() == 0 {
		var exists bool
		if err = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND college_id=$2 AND status='active')`, target, s.CollegeID).Scan(&exists); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if !exists {
			fail(w, http.StatusNotFound, "block_target_unavailable", "that account is not available in this campus")
			return
		}
	}
	_ = response.OK(w, map[string]any{"blocked": true})
}

func (h *Handler) unblockUser(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	target := chi.URLParam(r, "userID")
	if !uuidOK(target) {
		fail(w, http.StatusBadRequest, "block_target_invalid", "account ID is invalid")
		return
	}
	_, err := h.pool.Exec(r.Context(), `DELETE FROM user_blocks WHERE college_id=$1 AND blocker_id=$2 AND blocked_id=$3`, s.CollegeID, s.UserID, target)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) listBlocks(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT u.id::text,u.handle,u.display_name,b.created_at FROM user_blocks b
		JOIN users u ON u.id=b.blocked_id AND u.college_id=b.college_id WHERE b.college_id=$1 AND b.blocker_id=$2 ORDER BY b.created_at DESC`, s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID          string    `json:"id"`
		Handle      string    `json:"handle"`
		DisplayName string    `json:"display_name"`
		CreatedAt   time.Time `json:"created_at"`
	}
	items := []item{}
	for rows.Next() {
		var value item
		if err = rows.Scan(&value.ID, &value.Handle, &value.DisplayName, &value.CreatedAt); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, value)
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func validReportReason(reason string) bool {
	return reason == "spam" || reason == "harassment" || reason == "unsafe" || reason == "other"
}
