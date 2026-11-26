package collaboration

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
)

type taskItem struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Status         string   `json:"status"`
	Priority       string   `json:"priority"`
	MilestoneID    *string  `json:"milestone_id"`
	MilestoneTitle *string  `json:"milestone_title"`
	AssigneeIDs    []string `json:"assignee_ids"`
	AssigneeNames  []string `json:"assignee_names"`
	Version        int      `json:"version"`
	Position       int      `json:"position"`
	DueAt          any      `json:"due_at"`
	UpdatedAt      any      `json:"updated_at"`
	CanEdit        bool     `json:"can_edit"`
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, false)
	if !ok {
		return
	}
	limit := parseLimit(r)
	rows, err := h.pool.Query(r.Context(), `SELECT t.id::text,t.title,t.description,t.status,t.priority,t.milestone_id::text,m.title,
		COALESCE(array_agg(DISTINCT a.user_id::text) FILTER(WHERE a.user_id IS NOT NULL),'{}'::text[]),
		COALESCE(array_agg(DISTINCT u.display_name) FILTER(WHERE u.display_name IS NOT NULL),'{}'::text[]),
		t.version,t.position,t.due_at,t.updated_at,
		($3 OR t.created_by=$4 OR EXISTS(SELECT 1 FROM project_task_assignees mine WHERE mine.task_id=t.id AND mine.user_id=$4))
		FROM project_tasks t LEFT JOIN project_milestones m ON m.id=t.milestone_id
		LEFT JOIN project_task_assignees a ON a.task_id=t.id LEFT JOIN users u ON u.id=a.user_id
		WHERE t.project_id=$1 AND t.college_id=$2 AND ($5::uuid IS NULL OR t.id<$5)
		GROUP BY t.id,m.id ORDER BY t.id DESC LIMIT $6`, projectID(r), p.collegeID, p.role == "owner" || p.role == "maintainer", s.UserID, nullID(r.URL.Query().Get("cursor")), limit+1)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]taskItem, 0, limit+1)
	for rows.Next() {
		var item taskItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.Status, &item.Priority, &item.MilestoneID, &item.MilestoneTitle, &item.AssigneeIDs, &item.AssigneeNames, &item.Version, &item.Position, &item.DueAt, &item.UpdatedAt, &item.CanEdit); err != nil {
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

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, true)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		Status      string     `json:"status"`
		Priority    string     `json:"priority"`
		MilestoneID string     `json:"milestone_id"`
		AssigneeIDs []string   `json:"assignee_ids"`
		DueAt       *time.Time `json:"due_at"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !validTask(in.Title, in.Description, in.Status, in.Priority, in.MilestoneID, in.AssigneeIDs) {
		fail(w, http.StatusBadRequest, "task_invalid", "task fields or assignees are invalid")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var count int
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM project_tasks WHERE project_id=$1 AND college_id=$2`, projectID(r), p.collegeID).Scan(&count); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if count >= 500 {
		fail(w, http.StatusConflict, "task_limit_reached", "this project has reached its 500-task limit")
		return
	}
	if in.MilestoneID != "" {
		var exists bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM project_milestones WHERE id=$1 AND project_id=$2 AND college_id=$3)`, in.MilestoneID, projectID(r), p.collegeID).Scan(&exists); err != nil || !exists {
			if err != nil {
				h.err.Handle(w, r, err)
			} else {
				fail(w, http.StatusBadRequest, "milestone_invalid", "milestone does not belong to this project")
			}
			return
		}
	}
	if !h.validateAssignees(w, r, tx, projectID(r), p.collegeID, in.AssigneeIDs) {
		return
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO project_tasks(college_id,project_id,milestone_id,created_by,title,description,status,priority,position,due_at)
		SELECT $2,$1,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,COALESCE((SELECT max(position)+1 FROM project_tasks WHERE project_id=$1 AND status=$7),0),$9
		WHERE EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND college_id=$2 AND user_id=$4) RETURNING id::text`, projectID(r), p.collegeID, in.MilestoneID, s.UserID, strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), in.Status, in.Priority, in.DueAt).Scan(&id)
	if err != nil {
		internalError(h, w, r, err)
		return
	}
	if !h.replaceAssignees(r, tx, id, projectID(r), p.collegeID, s.UserID, in.AssigneeIDs) {
		h.err.Handle(w, r, errors.New("assign task members"))
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO project_task_events(college_id,project_id,task_id,actor_id,version,event_type,details) VALUES($1,$2,$3,$4,1,'created',$5)`, p.collegeID, projectID(r), id, s.UserID, marshalDetails(map[string]any{"status": in.Status, "assignee_ids": in.AssigneeIDs})); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.appendAudit(r.Context(), tx, s, "project.task.created", "project_task", id); err == nil {
		err = h.appendEvent(r.Context(), tx, p.collegeID, "project_task", id, "project.task.created", 1, map[string]string{"project_id": projectID(r)})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "status": in.Status, "priority": in.Priority, "version": 1})
}

func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, true)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		Status      string     `json:"status"`
		Priority    string     `json:"priority"`
		MilestoneID string     `json:"milestone_id"`
		AssigneeIDs *[]string  `json:"assignee_ids"`
		DueAt       *time.Time `json:"due_at"`
		Version     int        `json:"version"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if invalidText(in.Title, 3, 180) || len(in.Description) > 5000 || !taskStatusOK(in.Status) || !taskPriorityOK(in.Priority) || in.Version < 1 || (in.AssigneeIDs != nil && (len(*in.AssigneeIDs) > 10 || !uniqueStrings(*in.AssigneeIDs))) {
		fail(w, http.StatusBadRequest, "task_invalid", "task fields, version, or assignees are invalid")
		return
	}
	if in.MilestoneID != "" {
		var exists bool
		if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM project_milestones WHERE id=$1 AND project_id=$2 AND college_id=$3)`, in.MilestoneID, projectID(r), p.collegeID).Scan(&exists); err != nil || !exists {
			if err != nil {
				h.err.Handle(w, r, err)
			} else {
				fail(w, http.StatusBadRequest, "milestone_invalid", "milestone does not belong to this project")
			}
			return
		}
	}
	if in.AssigneeIDs != nil && !h.validateAssignees(w, r, nil, projectID(r), p.collegeID, *in.AssigneeIDs) {
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var oldStatus, oldMilestone string
	var creator string
	err = tx.QueryRow(r.Context(), `SELECT status,COALESCE(milestone_id::text,''),created_by::text FROM project_tasks WHERE id=$1 AND project_id=$2 AND college_id=$3 FOR UPDATE`, chi.URLParam(r, "taskID"), projectID(r), p.collegeID).Scan(&oldStatus, &oldMilestone, &creator)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "task_not_found", "task was not found")
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	manager := p.role == "owner" || p.role == "maintainer"
	var assigned bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM project_task_assignees WHERE task_id=$1 AND user_id=$2)`, chi.URLParam(r, "taskID"), s.UserID).Scan(&assigned); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !manager && creator != s.UserID && !assigned {
		fail(w, http.StatusForbidden, "task_edit_forbidden", "only an assignee, task creator, owner, or maintainer can update this task")
		return
	}
	if in.AssigneeIDs != nil && !manager && creator != s.UserID {
		fail(w, http.StatusForbidden, "task_assign_forbidden", "only the task creator or a project manager can change assignees")
		return
	}
	if in.AssigneeIDs != nil && !h.validateAssignees(w, r, tx, projectID(r), p.collegeID, *in.AssigneeIDs) {
		return
	}
	var version int
	err = tx.QueryRow(r.Context(), `UPDATE project_tasks SET title=$5,description=$6,status=$7,priority=$8,milestone_id=NULLIF($9,'')::uuid,due_at=$10,version=version+1,updated_at=now()
		WHERE id=$1 AND project_id=$2 AND college_id=$3 AND version=$4 RETURNING version`, chi.URLParam(r, "taskID"), projectID(r), p.collegeID, in.Version, strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), in.Status, in.Priority, in.MilestoneID, in.DueAt).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusConflict, "task_changed", "task changed; reload it before saving")
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.AssigneeIDs != nil && !h.replaceAssignees(r, tx, chi.URLParam(r, "taskID"), projectID(r), p.collegeID, s.UserID, *in.AssigneeIDs) {
		h.err.Handle(w, r, errors.New("assign task members"))
		return
	}
	eventType := "updated"
	if oldStatus != in.Status {
		eventType = "status_changed"
	}
	if oldMilestone != in.MilestoneID {
		eventType = "milestone_changed"
	}
	if in.AssigneeIDs != nil {
		eventType = "assignees_changed"
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO project_task_events(college_id,project_id,task_id,actor_id,version,event_type,details) VALUES($1,$2,$3,$4,$5,$6,$7)`, p.collegeID, projectID(r), chi.URLParam(r, "taskID"), s.UserID, version, eventType, marshalDetails(map[string]any{"status": in.Status, "milestone_id": in.MilestoneID})); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.appendAudit(r.Context(), tx, s, "project.task."+eventType, "project_task", chi.URLParam(r, "taskID")); err == nil {
		err = h.appendEvent(r.Context(), tx, p.collegeID, "project_task", chi.URLParam(r, "taskID"), "project.task.updated", int64(version), map[string]string{"project_id": projectID(r)})
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
	_ = response.OK(w, map[string]any{"id": chi.URLParam(r, "taskID"), "version": version, "status": in.Status})
}

func (h *Handler) listMilestones(w http.ResponseWriter, r *http.Request) {
	_, p, ok := h.requireProject(w, r, false)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT m.id::text,m.title,m.description,m.status,m.version,m.due_at,m.created_at,
		count(t.id)::int,count(t.id) FILTER(WHERE t.status='done')::int
		FROM project_milestones m LEFT JOIN project_tasks t ON t.milestone_id=m.id
		WHERE m.project_id=$1 AND m.college_id=$2 GROUP BY m.id ORDER BY m.due_at NULLS LAST,m.id DESC`, projectID(r), p.collegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	type milestone struct {
		ID             string `json:"id"`
		Title          string `json:"title"`
		Description    string `json:"description"`
		Status         string `json:"status"`
		Version        int    `json:"version"`
		TaskCount      int    `json:"task_count"`
		CompletedCount int    `json:"completed_count"`
		DueAt          any    `json:"due_at"`
		CreatedAt      any    `json:"created_at"`
	}
	items := []milestone{}
	for rows.Next() {
		var item milestone
		if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.Status, &item.Version, &item.DueAt, &item.CreatedAt, &item.TaskCount, &item.CompletedCount); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) createMilestone(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, true)
	if !ok {
		return
	}
	if !h.canManage(w, p) {
		return
	}
	in, err := request.Decode[struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		DueAt       *time.Time `json:"due_at"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Title, in.Description = strings.TrimSpace(in.Title), strings.TrimSpace(in.Description)
	if invalidText(in.Title, 3, 160) || len(in.Description) > 3000 {
		fail(w, 400, "milestone_invalid", "milestone fields are invalid")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var count int
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM project_milestones WHERE project_id=$1`, projectID(r)).Scan(&count); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if count >= 100 {
		fail(w, 409, "milestone_limit_reached", "this project has reached its 100-milestone limit")
		return
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO project_milestones(college_id,project_id,created_by,title,description,due_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, p.collegeID, projectID(r), s.UserID, in.Title, in.Description, in.DueAt).Scan(&id)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.appendAudit(r.Context(), tx, s, "project.milestone.created", "project_milestone", id); err == nil {
		err = h.appendEvent(r.Context(), tx, p.collegeID, "project_milestone", id, "project.milestone.created", 1, map[string]string{"project_id": projectID(r)})
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "status": "open", "version": 1})
}

func (h *Handler) updateMilestone(w http.ResponseWriter, r *http.Request) {
	s, p, ok := h.requireProject(w, r, true)
	if !ok {
		return
	}
	if !h.canManage(w, p) {
		return
	}
	in, err := request.Decode[struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		Status      string     `json:"status"`
		DueAt       *time.Time `json:"due_at"`
		Version     int        `json:"version"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Title, in.Description = strings.TrimSpace(in.Title), strings.TrimSpace(in.Description)
	if invalidText(in.Title, 3, 160) || len(in.Description) > 3000 || in.Version < 1 || (in.Status != "open" && in.Status != "completed" && in.Status != "cancelled") {
		fail(w, 400, "milestone_invalid", "milestone fields, status, or version are invalid")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var version int
	err = tx.QueryRow(r.Context(), `UPDATE project_milestones SET title=$5,description=$6,status=$7,due_at=$8,version=version+1,updated_at=now() WHERE id=$1 AND project_id=$2 AND college_id=$3 AND version=$4 RETURNING version`, chi.URLParam(r, "milestoneID"), projectID(r), p.collegeID, in.Version, in.Title, in.Description, in.Status, in.DueAt).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 409, "milestone_changed", "milestone changed; reload it before saving")
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = h.appendAudit(r.Context(), tx, s, "project.milestone.updated", "project_milestone", chi.URLParam(r, "milestoneID")); err == nil {
		err = h.appendEvent(r.Context(), tx, p.collegeID, "project_milestone", chi.URLParam(r, "milestoneID"), "project.milestone.updated", int64(version), map[string]string{"project_id": projectID(r)})
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
	_ = response.OK(w, map[string]any{"id": chi.URLParam(r, "milestoneID"), "version": version, "status": in.Status})
}

func validTask(title, description, status, priority, milestone string, assignees []string) bool {
	return !invalidText(title, 3, 180) && len(description) <= 5000 && taskStatusOK(status) && taskPriorityOK(priority) && (milestone == "" || uuidOK(milestone)) && len(assignees) <= 10 && uniqueStrings(assignees)
}
func taskStatusOK(s string) bool {
	return s == "todo" || s == "in_progress" || s == "blocked" || s == "done"
}
func taskPriorityOK(s string) bool { return s == "low" || s == "normal" || s == "high" }
func uniqueStrings(items []string) bool {
	seen := map[string]struct{}{}
	for _, item := range items {
		if !uuidOK(item) {
			return false
		}
		if _, ok := seen[item]; ok {
			return false
		}
		seen[item] = struct{}{}
	}
	return true
}
func uuidOK(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func (h *Handler) validateAssignees(w http.ResponseWriter, r *http.Request, tx pgx.Tx, pid, cid string, ids []string) bool {
	if len(ids) == 0 {
		return true
	}
	if !uniqueStrings(ids) {
		fail(w, 400, "assignees_invalid", "assignees must be unique active project members")
		return false
	}
	var count int
	var err error
	if tx != nil {
		err = tx.QueryRow(r.Context(), `SELECT count(*) FROM project_members pm JOIN users u ON u.id=pm.user_id AND u.college_id=pm.college_id AND u.status='active' WHERE pm.project_id=$1 AND pm.college_id=$2 AND pm.user_id=ANY($3::uuid[])`, pid, cid, ids).Scan(&count)
	} else {
		err = h.pool.QueryRow(r.Context(), `SELECT count(*) FROM project_members pm JOIN users u ON u.id=pm.user_id AND u.college_id=pm.college_id AND u.status='active' WHERE pm.project_id=$1 AND pm.college_id=$2 AND pm.user_id=ANY($3::uuid[])`, pid, cid, ids).Scan(&count)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return false
	}
	if count != len(ids) {
		fail(w, 400, "assignees_invalid", "assignees must be active members of this project")
		return false
	}
	return true
}

func (h *Handler) replaceAssignees(r *http.Request, tx pgx.Tx, taskID, pid, cid, actor string, ids []string) bool {
	if _, err := tx.Exec(r.Context(), `DELETE FROM project_task_assignees WHERE task_id=$1`, taskID); err != nil {
		return false
	}
	for _, id := range ids {
		if _, err := tx.Exec(r.Context(), `INSERT INTO project_task_assignees(college_id,project_id,task_id,user_id,assigned_by) VALUES($1,$2,$3,$4,$5)`, cid, pid, taskID, id, actor); err != nil {
			return false
		}
	}
	return true
}
