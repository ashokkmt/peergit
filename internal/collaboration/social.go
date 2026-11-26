package collaboration

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"peergit/internal/identity"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
)

type contentTarget struct {
	Kind, ID string
}

func readTarget(r *http.Request) contentTarget {
	return contentTarget{Kind: r.URL.Query().Get("target_type"), ID: r.URL.Query().Get("target_id")}
}

func targetColumn(kind string) string {
	switch kind {
	case "update":
		return "update_id"
	case "discussion":
		return "discussion_id"
	case "task":
		return "task_id"
	default:
		return ""
	}
}

func (h *Handler) targetExists(w http.ResponseWriter, r *http.Request, tx pgx.Tx, pid, cid string, target contentTarget) bool {
	column := targetColumn(target.Kind)
	if column == "" || !uuidOK(target.ID) {
		fail(w, http.StatusBadRequest, "comment_target_invalid", "comment target is invalid")
		return false
	}
	var exists bool
	query := ""
	switch target.Kind {
	case "update":
		query = `SELECT EXISTS(SELECT 1 FROM project_updates WHERE id=$1 AND project_id=$2 AND college_id=$3 AND deleted_at IS NULL AND (audience='campus' OR EXISTS(SELECT 1 FROM project_members WHERE project_id=$2 AND user_id=$4)))`
	case "discussion":
		query = `SELECT EXISTS(SELECT 1 FROM project_discussions WHERE id=$1 AND project_id=$2 AND college_id=$3 AND status<>'hidden' AND $4::uuid IS NOT NULL)`
	case "task":
		query = `SELECT EXISTS(SELECT 1 FROM project_tasks WHERE id=$1 AND project_id=$2 AND college_id=$3 AND $4::uuid IS NOT NULL)`
	case "comment":
		query = `SELECT EXISTS(SELECT 1 FROM project_comments c WHERE c.id=$1 AND c.project_id=$2 AND c.college_id=$3 AND c.deleted_at IS NULL
			AND (c.update_id IS NULL OR EXISTS(SELECT 1 FROM project_updates u WHERE u.id=c.update_id AND (u.audience='campus' OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=$2 AND m.user_id=$4))))
			AND (c.discussion_id IS NULL OR EXISTS(SELECT 1 FROM project_discussions d WHERE d.id=c.discussion_id AND d.status<>'hidden'))
			AND $4::uuid IS NOT NULL)`
	}
	var err error
	if tx != nil {
		err = tx.QueryRow(r.Context(), query, target.ID, pid, cid, sessionUser(r)).Scan(&exists)
	} else {
		err = h.pool.QueryRow(r.Context(), query, target.ID, pid, cid, sessionUser(r)).Scan(&exists)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return false
	}
	if !exists {
		fail(w, http.StatusNotFound, "content_not_found", "content was not found")
		return false
	}
	return true
}

func sessionUser(r *http.Request) string {
	s, _ := identity.CurrentSession(r.Context())
	return s.UserID
}

func (h *Handler) listComments(w http.ResponseWriter, r *http.Request) {
	_, p, ok := h.requireProject(w, r, false)
	if !ok {
		return
	}
	target := readTarget(r)
	if !h.targetExists(w, r, nil, projectID(r), p.collegeID, target) {
		return
	}
	column := targetColumn(target.Kind)
	query := `SELECT c.id::text,c.parent_comment_id::text,c.author_id::text,u.display_name,c.body,c.version,c.created_at,c.edited_at
		FROM project_comments c JOIN users u ON u.id=c.author_id WHERE c.project_id=$1 AND c.college_id=$2 AND c.` + column + `=$3 AND c.deleted_at IS NULL ORDER BY c.created_at,c.id LIMIT $4`
	rows, err := h.pool.Query(r.Context(), query, projectID(r), p.collegeID, target.ID, parseLimit(r)+1)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	type comment struct {
		ID         string  `json:"id"`
		ParentID   *string `json:"parent_id"`
		AuthorID   string  `json:"author_id"`
		AuthorName string  `json:"author_name"`
		Body       string  `json:"body"`
		Version    int     `json:"version"`
		CreatedAt  any     `json:"created_at"`
		EditedAt   any     `json:"edited_at,omitempty"`
	}
	items := []comment{}
	for rows.Next() {
		var item comment
		if err := rows.Scan(&item.ID, &item.ParentID, &item.AuthorID, &item.AuthorName, &item.Body, &item.Version, &item.CreatedAt, &item.EditedAt); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if len(items) > parseLimit(r) {
		fail(w, http.StatusBadRequest, "comments_truncated", "this thread has more comments than can be shown; narrow the view")
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) createComment(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, true)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
		ParentID   string `json:"parent_id"`
		Body       string `json:"body"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	target := contentTarget{Kind: in.TargetType, ID: in.TargetID}
	if invalidText(in.Body, 1, 2000) || (in.ParentID != "" && !uuidOK(in.ParentID)) {
		fail(w, 400, "comment_invalid", "comment text or parent is invalid")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if !h.targetExists(w, r, tx, projectID(r), p.collegeID, target) {
		return
	}
	if target.Kind == "discussion" {
		var status string
		if err = tx.QueryRow(r.Context(), `SELECT status FROM project_discussions WHERE id=$1 AND project_id=$2 AND college_id=$3 FOR UPDATE`, target.ID, projectID(r), p.collegeID).Scan(&status); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if status == "locked" {
			fail(w, http.StatusConflict, "discussion_locked", "this discussion is locked for new replies")
			return
		}
	}
	if in.ParentID != "" {
		var sameTarget bool
		column := targetColumn(in.TargetType)
		if column == "" {
			fail(w, 400, "comment_target_invalid", "comment target is invalid")
			return
		}
		query := `SELECT EXISTS(SELECT 1 FROM project_comments WHERE id=$1 AND project_id=$2 AND college_id=$3 AND parent_comment_id IS NULL AND ` + column + `=$4 AND deleted_at IS NULL)`
		if err = tx.QueryRow(r.Context(), query, in.ParentID, projectID(r), p.collegeID, in.TargetID).Scan(&sameTarget); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if !sameTarget {
			fail(w, 400, "reply_invalid", "replies must target a visible top-level comment in the same thread")
			return
		}
	}
	column := targetColumn(in.TargetType)
	if column == "" {
		fail(w, 400, "comment_target_invalid", "comment target is invalid")
		return
	}
	query := `INSERT INTO project_comments(college_id,project_id,` + column + `,parent_comment_id,author_id,body) VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5,$6) RETURNING id::text`
	var id string
	if err = tx.QueryRow(r.Context(), query, p.collegeID, projectID(r), in.TargetID, in.ParentID, s.UserID, in.Body).Scan(&id); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.appendAudit(r.Context(), tx, s, "project.comment.created", "project_comment", id); err == nil {
		err = h.appendEvent(r.Context(), tx, p.collegeID, "project_comment", id, "project.comment.created", 1, map[string]string{"project_id": projectID(r), "target_type": in.TargetType, "target_id": in.TargetID})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "body": in.Body, "version": 1})
}

func (h *Handler) updateComment(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, true)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Body    string `json:"body"`
		Version int    `json:"version"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if invalidText(in.Body, 1, 2000) || in.Version < 1 {
		fail(w, 400, "comment_invalid", "comment text or version is invalid")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var version int
	err = tx.QueryRow(r.Context(), `UPDATE project_comments SET body=$5,version=version+1,edited_at=now()
		WHERE id=$1 AND project_id=$2 AND college_id=$3 AND version=$4 AND deleted_at IS NULL
		AND (author_id=$6 AND created_at>now()-interval '15 minutes' OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=$2 AND m.user_id=$6 AND m.role IN ('owner','maintainer')))
		RETURNING version`, chi.URLParam(r, "commentID"), projectID(r), p.collegeID, in.Version, in.Body, s.UserID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 409, "comment_changed_or_forbidden", "comment changed, its edit window ended, or you cannot edit it")
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.appendAudit(r.Context(), tx, s, "project.comment.edited", "project_comment", chi.URLParam(r, "commentID")); err == nil {
		err = h.appendEvent(r.Context(), tx, p.collegeID, "project_comment", chi.URLParam(r, "commentID"), "project.comment.edited", int64(version), map[string]string{"project_id": projectID(r)})
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
	_ = response.OK(w, map[string]any{"id": chi.URLParam(r, "commentID"), "version": version})
}

func (h *Handler) setReaction(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, true)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
		Reaction   string `json:"reaction"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if targetColumn(in.TargetType) == "task" || targetColumn(in.TargetType) == "" || !uuidOK(in.TargetID) || (in.Reaction != "like" && in.Reaction != "celebrate" && in.Reaction != "insightful" && in.Reaction != "support") {
		fail(w, 400, "reaction_invalid", "reaction target or type is invalid")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	target := contentTarget{Kind: in.TargetType, ID: in.TargetID}
	if !h.targetExists(w, r, tx, projectID(r), p.collegeID, target) {
		return
	}
	column := targetColumn(in.TargetType)
	var id string
	var version int
	query := `INSERT INTO project_reactions(college_id,project_id,` + column + `,user_id,reaction) VALUES($1,$2,$3,$4,$5)
		ON CONFLICT (` + column + `,user_id) WHERE ` + column + ` IS NOT NULL DO UPDATE SET reaction=EXCLUDED.reaction,version=project_reactions.version+1,updated_at=now()
		RETURNING id::text,version`
	if err = tx.QueryRow(r.Context(), query, p.collegeID, projectID(r), in.TargetID, s.UserID, in.Reaction).Scan(&id, &version); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.appendEvent(r.Context(), tx, p.collegeID, "project_reaction", id, "project.reaction.set", int64(version), map[string]string{"project_id": projectID(r), "target_type": in.TargetType, "target_id": in.TargetID}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"id": id, "reaction": in.Reaction, "version": version})
}

func (h *Handler) deleteReaction(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, true)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	column := targetColumn(in.TargetType)
	if column == "" || in.TargetType == "task" || !uuidOK(in.TargetID) {
		fail(w, 400, "reaction_invalid", "reaction target is invalid")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if !h.targetExists(w, r, tx, projectID(r), p.collegeID, contentTarget{in.TargetType, in.TargetID}) {
		return
	}
	query := `DELETE FROM project_reactions WHERE project_id=$1 AND college_id=$2 AND user_id=$3 AND ` + column + `=$4 RETURNING id::text,version`
	var id string
	var version int
	err = tx.QueryRow(r.Context(), query, projectID(r), p.collegeID, s.UserID, in.TargetID).Scan(&id, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Commit(r.Context())
		response.NoContent(w)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.appendEvent(r.Context(), tx, p.collegeID, "project_reaction", id, "project.reaction.removed", int64(version+1), map[string]string{"project_id": projectID(r)}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) setFollow(w http.ResponseWriter, r *http.Request) {
	h.setProjectSignal(w, r, "follow")
}
func (h *Handler) setBookmark(w http.ResponseWriter, r *http.Request) {
	h.setProjectSignal(w, r, "bookmark")
}
func (h *Handler) deleteFollow(w http.ResponseWriter, r *http.Request) {
	h.deleteProjectSignal(w, r, "follow")
}
func (h *Handler) deleteBookmark(w http.ResponseWriter, r *http.Request) {
	h.deleteProjectSignal(w, r, "bookmark")
}

func (h *Handler) setProjectSignal(w http.ResponseWriter, r *http.Request, kind string) {
	s, p, ok := h.requireProject(w, r, false)
	if !ok {
		return
	}
	table := "project_follows"
	event := "project.followed"
	if kind == "bookmark" {
		table = "project_bookmarks"
		event = "project.bookmarked"
	}
	stateKey := kind + "ing"
	if kind == "bookmark" {
		stateKey = "bookmarked"
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	query := `INSERT INTO ` + table + `(college_id,project_id,user_id) VALUES($1,$2,$3) ON CONFLICT(project_id,user_id) DO NOTHING RETURNING id::text`
	var id string
	err = tx.QueryRow(r.Context(), query, p.collegeID, projectID(r), s.UserID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Commit(r.Context())
		_ = response.OK(w, map[string]bool{stateKey: true})
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.appendEvent(r.Context(), tx, p.collegeID, kind, id, event, 1, map[string]string{"project_id": projectID(r)}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"id": id, stateKey: true})
}

func (h *Handler) deleteProjectSignal(w http.ResponseWriter, r *http.Request, kind string) {
	s, p, ok := h.requireProject(w, r, false)
	if !ok {
		return
	}
	table := "project_follows"
	event := "project.unfollowed"
	if kind == "bookmark" {
		table = "project_bookmarks"
		event = "project.unbookmarked"
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	query := `DELETE FROM ` + table + ` WHERE college_id=$1 AND project_id=$2 AND user_id=$3 RETURNING id::text`
	var id string
	err = tx.QueryRow(r.Context(), query, p.collegeID, projectID(r), s.UserID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Commit(r.Context())
		response.NoContent(w)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.appendEvent(r.Context(), tx, p.collegeID, kind, id, event, 2, map[string]string{"project_id": projectID(r)}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) savedProjects(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT p.id::text,p.slug,p.title,p.summary,p.visibility,p.lifecycle,p.updated_at,
		EXISTS(SELECT 1 FROM project_follows f WHERE f.project_id=p.id AND f.user_id=$2) AS following,
		EXISTS(SELECT 1 FROM project_bookmarks b WHERE b.project_id=p.id AND b.user_id=$2) AS bookmarked
		FROM projects p LEFT JOIN project_follows f ON f.project_id=p.id AND f.user_id=$2 LEFT JOIN project_bookmarks b ON b.project_id=p.id AND b.user_id=$2
		WHERE p.college_id=$1 AND p.github_revoke_deadline IS NULL AND (f.user_id IS NOT NULL OR b.user_id IS NOT NULL)
		AND (EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=p.id AND m.user_id=$2) OR (p.visibility<>'private' AND p.lifecycle='active'))
		ORDER BY p.updated_at DESC,p.id DESC LIMIT 100`, s.CollegeID, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID         string `json:"id"`
		Slug       string `json:"slug"`
		Title      string `json:"title"`
		Summary    string `json:"summary"`
		Visibility string `json:"visibility"`
		Lifecycle  string `json:"lifecycle"`
		UpdatedAt  any    `json:"updated_at"`
		Following  bool   `json:"following"`
		Bookmarked bool   `json:"bookmarked"`
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err := rows.Scan(&v.ID, &v.Slug, &v.Title, &v.Summary, &v.Visibility, &v.Lifecycle, &v.UpdatedAt, &v.Following, &v.Bookmarked); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) createReport(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, false)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
		Reason     string `json:"reason"`
		Details    string `json:"details"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Details = strings.TrimSpace(in.Details)
	if (targetColumn(in.TargetType) == "" && in.TargetType != "comment") || !uuidOK(in.TargetID) || (in.Reason != "spam" && in.Reason != "harassment" && in.Reason != "unsafe" && in.Reason != "other") || len(in.Details) > 1000 {
		fail(w, 400, "report_invalid", "report fields are invalid")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if !h.targetExists(w, r, tx, projectID(r), p.collegeID, contentTarget{in.TargetType, in.TargetID}) {
		return
	}
	columns := map[string]string{"update": "update_id", "discussion": "discussion_id", "task": "task_id"}
	if in.TargetType == "comment" {
		columns[in.TargetType] = "comment_id"
	}
	column := columns[in.TargetType]
	if column == "" {
		fail(w, 400, "report_target_invalid", "this content type cannot be reported")
		return
	}
	var id string
	query := `INSERT INTO project_content_reports(college_id,project_id,` + column + `,reporter_id,reason,details) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`
	err = tx.QueryRow(r.Context(), query, p.collegeID, projectID(r), in.TargetID, s.UserID, in.Reason, in.Details).Scan(&id)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			fail(w, 409, "already_reported", "you have already reported this content")
			return
		}
		h.err.Handle(w, r, err)
		return
	}
	if err = h.appendAudit(r.Context(), tx, s, "project.content.reported", "project_content_report", id); err == nil {
		err = h.appendEvent(r.Context(), tx, p.collegeID, "project_content_report", id, "project.content.reported", 1, map[string]string{"project_id": projectID(r), "target_type": in.TargetType, "target_id": in.TargetID})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]string{"id": id, "status": "received"})
}
