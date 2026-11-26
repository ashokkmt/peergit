package collaboration

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
)

type updateItem struct {
	ID             string `json:"id"`
	AuthorID       string `json:"author_id"`
	AuthorName     string `json:"author_name"`
	Body           string `json:"body"`
	Audience       string `json:"audience"`
	Version        int    `json:"version"`
	ReactionCount  int    `json:"reaction_count"`
	CommentCount   int    `json:"comment_count"`
	ViewerReaction string `json:"viewer_reaction,omitempty"`
	CreatedAt      any    `json:"created_at"`
	EditedAt       any    `json:"edited_at,omitempty"`
}

func (h *Handler) listUpdates(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, false)
	if !ok {
		return
	}
	limit := parseLimit(r)
	rows, err := h.pool.Query(r.Context(), `SELECT u.id::text,u.author_id::text,COALESCE(a.display_name,''),u.body,u.audience,u.version,
		(SELECT count(*)::int FROM project_reactions rr WHERE rr.update_id=u.id),
		(SELECT count(*)::int FROM project_comments c WHERE c.update_id=u.id AND c.deleted_at IS NULL),
		COALESCE((SELECT rr.reaction FROM project_reactions rr WHERE rr.update_id=u.id AND rr.user_id=$4),''),u.created_at,u.edited_at
		FROM project_updates u JOIN users a ON a.id=u.author_id
		WHERE u.project_id=$1 AND u.college_id=$2 AND u.deleted_at IS NULL
		AND (u.audience='campus' OR $3<>'') AND ($5::uuid IS NULL OR u.id<$5)
		ORDER BY u.id DESC LIMIT $6`, projectID(r), p.collegeID, p.role, s.UserID, nullID(r.URL.Query().Get("cursor")), limit+1)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]updateItem, 0, limit+1)
	for rows.Next() {
		var item updateItem
		if err := rows.Scan(&item.ID, &item.AuthorID, &item.AuthorName, &item.Body, &item.Audience, &item.Version, &item.ReactionCount, &item.CommentCount, &item.ViewerReaction, &item.CreatedAt, &item.EditedAt); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ID
		items = items[:limit]
	}
	_ = response.OK(w, map[string]any{"items": items, "next_cursor": next})
}

func (h *Handler) createUpdate(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, true)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Body     string `json:"body"`
		Audience string `json:"audience"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Audience == "" {
		in.Audience = "members"
	}
	if invalidText(in.Body, 1, 5000) || (in.Audience != "members" && in.Audience != "campus") {
		fail(w, http.StatusBadRequest, "update_invalid", "update text or audience is invalid")
		return
	}
	if in.Audience == "campus" && p.visibility == "private" {
		fail(w, http.StatusConflict, "audience_unavailable", "campus updates require a campus-visible project")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO project_updates(college_id,project_id,author_id,body,audience)
		SELECT $2,$1,$3,$4,$5 WHERE EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$3)
		RETURNING id::text`, projectID(r), p.collegeID, s.UserID, in.Body, in.Audience).Scan(&id)
	if err != nil {
		internalError(h, w, r, err)
		return
	}
	if err = h.appendAudit(r.Context(), tx, s, "project.update.created", "project_update", id); err == nil {
		err = h.appendEvent(r.Context(), tx, s.CollegeID, "project_update", id, "project.update.created", 1, map[string]string{"project_id": projectID(r)})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "body": in.Body, "audience": in.Audience, "version": 1})
}

func (h *Handler) updateUpdate(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, true)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Body     string `json:"body"`
		Audience string `json:"audience"`
		Version  int    `json:"version"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if invalidText(in.Body, 1, 5000) || (in.Audience != "members" && in.Audience != "campus") || in.Version < 1 {
		fail(w, http.StatusBadRequest, "update_invalid", "update text, audience, or version is invalid")
		return
	}
	if in.Audience == "campus" && p.visibility == "private" {
		fail(w, http.StatusConflict, "audience_unavailable", "campus updates require a campus-visible project")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var version int
	err = tx.QueryRow(r.Context(), `UPDATE project_updates SET body=$5,audience=$6,version=version+1,edited_at=now()
		WHERE id=$1 AND project_id=$2 AND college_id=$3 AND version=$4 AND deleted_at IS NULL
		AND (author_id=$7 OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=$2 AND m.user_id=$7 AND m.role IN ('owner','maintainer')))
		RETURNING version`, chi.URLParam(r, "updateID"), projectID(r), p.collegeID, in.Version, in.Body, in.Audience, s.UserID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusConflict, "update_changed_or_forbidden", "update changed or you cannot edit it")
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.appendAudit(r.Context(), tx, s, "project.update.edited", "project_update", chi.URLParam(r, "updateID")); err == nil {
		err = h.appendEvent(r.Context(), tx, s.CollegeID, "project_update", chi.URLParam(r, "updateID"), "project.update.edited", int64(version), map[string]string{"project_id": projectID(r)})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Version(w, int64(version))
	_ = response.OK(w, map[string]any{"id": chi.URLParam(r, "updateID"), "version": version})
}

func (h *Handler) listDiscussions(w http.ResponseWriter, r *http.Request) {
	_, p, ok := h.requireProject(w, r, false)
	if !ok {
		return
	}
	limit := parseLimit(r)
	rows, err := h.pool.Query(r.Context(), `SELECT d.id::text,d.author_id::text,u.display_name,d.title,d.body,d.status,d.version,d.created_at,d.updated_at,
		(SELECT count(*)::int FROM project_comments c WHERE c.discussion_id=d.id AND c.deleted_at IS NULL)
		FROM project_discussions d JOIN users u ON u.id=d.author_id
		WHERE d.project_id=$1 AND d.college_id=$2 AND ($3 OR d.status<>'hidden') AND ($4::uuid IS NULL OR d.id<$4)
		ORDER BY d.id DESC LIMIT $5`, projectID(r), p.collegeID, p.role == "owner" || p.role == "maintainer", nullID(r.URL.Query().Get("cursor")), limit+1)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	type discussion struct {
		ID         string `json:"id"`
		AuthorID   string `json:"author_id"`
		AuthorName string `json:"author_name"`
		Title      string `json:"title"`
		Body       string `json:"body"`
		Status     string `json:"status"`
		Version    int    `json:"version"`
		ReplyCount int    `json:"reply_count"`
		CreatedAt  any    `json:"created_at"`
		UpdatedAt  any    `json:"updated_at"`
	}
	items := make([]discussion, 0, limit+1)
	for rows.Next() {
		var item discussion
		if err := rows.Scan(&item.ID, &item.AuthorID, &item.AuthorName, &item.Title, &item.Body, &item.Status, &item.Version, &item.CreatedAt, &item.UpdatedAt, &item.ReplyCount); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ID
		items = items[:limit]
	}
	_ = response.OK(w, map[string]any{"items": items, "next_cursor": next})
}

func (h *Handler) createDiscussion(w http.ResponseWriter, r *http.Request) {
	s, _, ok := h.requireProject(w, r, true)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Title, in.Body = strings.TrimSpace(in.Title), strings.TrimSpace(in.Body)
	if invalidText(in.Title, 3, 160) || invalidText(in.Body, 1, 5000) {
		fail(w, http.StatusBadRequest, "discussion_invalid", "discussion title or body is invalid")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO project_discussions(college_id,project_id,author_id,title,body)
		SELECT $2,$1,$3,$4,$5 WHERE EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$3) RETURNING id::text`, projectID(r), s.CollegeID, s.UserID, in.Title, in.Body).Scan(&id)
	if err != nil {
		internalError(h, w, r, err)
		return
	}
	if err = h.appendAudit(r.Context(), tx, s, "project.discussion.created", "project_discussion", id); err == nil {
		err = h.appendEvent(r.Context(), tx, s.CollegeID, "project_discussion", id, "project.discussion.created", 1, map[string]string{"project_id": projectID(r)})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "title": in.Title, "body": in.Body, "status": "open", "version": 1})
}

func (h *Handler) updateDiscussion(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, true)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Title   string `json:"title"`
		Body    string `json:"body"`
		Status  string `json:"status"`
		Version int    `json:"version"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Title, in.Body = strings.TrimSpace(in.Title), strings.TrimSpace(in.Body)
	if invalidText(in.Title, 3, 160) || invalidText(in.Body, 1, 5000) || in.Version < 1 || (in.Status != "open" && in.Status != "locked" && in.Status != "hidden") {
		fail(w, http.StatusBadRequest, "discussion_invalid", "discussion title, body, status, or version is invalid")
		return
	}
	if in.Status != "open" && !h.canManage(w, p) {
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var version int
	err = tx.QueryRow(r.Context(), `UPDATE project_discussions SET title=$5,body=$6,status=$7,version=version+1,edited_at=now(),updated_at=now()
		WHERE id=$1 AND project_id=$2 AND college_id=$3 AND version=$4 AND status<>'hidden'
		AND ($8 OR author_id=$9) RETURNING version`, chi.URLParam(r, "discussionID"), projectID(r), p.collegeID, in.Version, in.Title, in.Body, in.Status, p.role == "owner" || p.role == "maintainer", s.UserID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusConflict, "discussion_changed_or_forbidden", "discussion changed or you cannot edit it")
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.appendAudit(r.Context(), tx, s, "project.discussion.updated", "project_discussion", chi.URLParam(r, "discussionID")); err == nil {
		err = h.appendEvent(r.Context(), tx, s.CollegeID, "project_discussion", chi.URLParam(r, "discussionID"), "project.discussion.updated", int64(version), map[string]string{"project_id": projectID(r)})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Version(w, int64(version))
	_ = response.OK(w, map[string]any{"id": chi.URLParam(r, "discussionID"), "version": version, "status": in.Status})
}

func nullID(id string) any {
	if id == "" {
		return nil
	}
	return id
}
