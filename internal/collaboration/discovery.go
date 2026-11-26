package collaboration

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"peergit/internal/platform/http/cursor"
	"peergit/internal/platform/http/response"
)

const discoveryPageSize = 20

type discoveryCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func (h *Handler) decodeCursor(w http.ResponseWriter, r *http.Request) (*discoveryCursor, bool) {
	token := r.URL.Query().Get("cursor")
	if token == "" {
		return nil, true
	}
	payload, err := cursor.Decode(h.cursorKey, token)
	var point discoveryCursor
	if err != nil || json.Unmarshal(payload, &point) != nil || point.ID == "" || point.CreatedAt.IsZero() {
		fail(w, http.StatusBadRequest, "cursor_invalid", "the page cursor is invalid; restart from the first page")
		return nil, false
	}
	return &point, true
}

func (h *Handler) encodeCursor(point discoveryCursor) (string, error) {
	payload, err := json.Marshal(point)
	if err != nil {
		return "", err
	}
	return cursor.Encode(h.cursorKey, payload)
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 120 {
		fail(w, http.StatusBadRequest, "search_query_invalid", "search text must be 120 characters or fewer")
		return
	}
	if query != "" && len([]rune(query)) < 2 {
		_ = response.OK(w, map[string]any{"projects": []any{}, "roles": []any{}, "people": []any{}, "next_cursor": nil})
		return
	}
	type projectResult struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Summary     string   `json:"summary"`
		ProjectType string   `json:"project_type"`
		Visibility  string   `json:"visibility"`
		Recruiting  bool     `json:"recruiting"`
		Skills      []string `json:"skills"`
	}
	projects := []projectResult{}
	rows, err := h.pool.Query(r.Context(), `SELECT p.id::text,p.title,p.summary,p.project_type,p.visibility,
		EXISTS(SELECT 1 FROM project_roles rr WHERE rr.project_id=p.id AND rr.college_id=p.college_id AND rr.status='open'),
		COALESCE(array_agg(DISTINCT sk.name) FILTER (WHERE sk.name IS NOT NULL),'{}')
		FROM projects p
		LEFT JOIN project_skills ps ON ps.project_id=p.id AND ps.college_id=p.college_id
		LEFT JOIN skills sk ON sk.id=ps.skill_id
		WHERE p.college_id=$1 AND p.lifecycle='active' AND p.visibility IN ('campus','public')
		AND p.github_revoke_deadline IS NULL
		AND ($2='' OR p.search_document @@ plainto_tsquery('english',$2) OR p.title OPERATOR(public.%) $2 OR p.summary OPERATOR(public.%) $2)
		GROUP BY p.id ORDER BY CASE WHEN $2='' THEN 0 ELSE ts_rank(p.search_document,plainto_tsquery('english',$2)) END DESC,p.updated_at DESC,p.id DESC LIMIT $3`, s.CollegeID, query, discoveryPageSize)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	for rows.Next() {
		var item projectResult
		if err = rows.Scan(&item.ID, &item.Title, &item.Summary, &item.ProjectType, &item.Visibility, &item.Recruiting, &item.Skills); err != nil {
			rows.Close()
			h.err.Handle(w, r, err)
			return
		}
		projects = append(projects, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		h.err.Handle(w, r, err)
		return
	}
	rows.Close()

	type roleResult struct {
		ID                 string   `json:"id"`
		RoleID             string   `json:"role_id"`
		Title              string   `json:"title"`
		Summary            string   `json:"summary"`
		ProjectTitle       string   `json:"project_title"`
		Difficulty         string   `json:"difficulty"`
		GoodFirstTask      bool     `json:"good_first_task"`
		PrerequisiteSkills []string `json:"prerequisite_skills"`
	}
	roles := []roleResult{}
	rows, err = h.pool.Query(r.Context(), `SELECT p.id::text,rr.id::text,rr.title,p.summary,p.title,rr.difficulty,rr.good_first_task,rr.prerequisite_skills
		FROM project_roles rr JOIN projects p ON p.id=rr.project_id AND p.college_id=rr.college_id
		WHERE p.college_id=$1 AND p.lifecycle='active' AND p.visibility IN ('campus','public') AND p.github_revoke_deadline IS NULL AND rr.status='open'
		AND ($2='' OR rr.title OPERATOR(public.%) $2 OR rr.title ILIKE '%' || $2 || '%' OR p.search_document @@ plainto_tsquery('english',$2))
		ORDER BY CASE WHEN $2='' THEN 0 ELSE public.similarity(rr.title,$2) END DESC,rr.created_at DESC,rr.id DESC LIMIT $3`, s.CollegeID, query, discoveryPageSize)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	for rows.Next() {
		var item roleResult
		if err = rows.Scan(&item.ID, &item.RoleID, &item.Title, &item.Summary, &item.ProjectTitle, &item.Difficulty, &item.GoodFirstTask, &item.PrerequisiteSkills); err != nil {
			rows.Close()
			h.err.Handle(w, r, err)
			return
		}
		roles = append(roles, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		h.err.Handle(w, r, err)
		return
	}
	rows.Close()

	type personResult struct {
		ID          string   `json:"id"`
		Handle      string   `json:"handle"`
		DisplayName string   `json:"display_name"`
		Headline    string   `json:"headline"`
		Skills      []string `json:"skills"`
	}
	people := []personResult{}
	rows, err = h.pool.Query(r.Context(), `SELECT u.id::text,u.handle,u.display_name,COALESCE(u.profile->>'headline',''),
		COALESCE(array_agg(DISTINCT sk.name) FILTER (WHERE sk.name IS NOT NULL),'{}')
		FROM users u JOIN campus_verifications cv ON cv.user_id=u.id AND cv.college_id=u.college_id AND cv.revoked_at IS NULL
		JOIN consent_records pc ON pc.user_id=u.id AND pc.purpose='profile_discovery' AND pc.revoked_at IS NULL
		LEFT JOIN user_skills us ON us.user_id=u.id LEFT JOIN skills sk ON sk.id=us.skill_id
		WHERE u.college_id=$1 AND u.status='active' AND u.account_type='campus' AND u.id<>$2
		AND ($3='' OR u.profile_search_document @@ plainto_tsquery('english',$3) OR u.display_name OPERATOR(public.%) $3 OR u.handle OPERATOR(public.%) $3)
		GROUP BY u.id ORDER BY CASE WHEN $3='' THEN 0 ELSE ts_rank(u.profile_search_document,plainto_tsquery('english',$3)) END DESC,u.display_name,u.id LIMIT $4`, s.CollegeID, s.UserID, query, discoveryPageSize)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	for rows.Next() {
		var item personResult
		if err = rows.Scan(&item.ID, &item.Handle, &item.DisplayName, &item.Headline, &item.Skills); err != nil {
			rows.Close()
			h.err.Handle(w, r, err)
			return
		}
		people = append(people, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"projects": projects, "roles": roles, "people": people, "next_cursor": nil})
}

func (h *Handler) feed(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "campus"
	}
	if scope != "campus" && scope != "following" {
		fail(w, http.StatusBadRequest, "feed_scope_invalid", "feed scope must be campus or following")
		return
	}
	point, ok := h.decodeCursor(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `WITH feed_items AS (
		SELECT u.id,u.project_id,p.title AS project_title,u.author_id AS actor_id,'update'::text AS kind,u.body AS summary,u.created_at
		FROM project_updates u JOIN projects p ON p.id=u.project_id AND p.college_id=u.college_id
		WHERE u.college_id=$1 AND u.deleted_at IS NULL AND (u.audience='campus' OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=$2))
		UNION ALL
		SELECT d.id,d.project_id,p.title,d.author_id,'discussion'::text,d.title || ': ' || d.body,d.created_at
		FROM project_discussions d JOIN projects p ON p.id=d.project_id AND p.college_id=d.college_id
		WHERE d.college_id=$1 AND d.status='open'
		), visible AS (
		SELECT f.* FROM feed_items f JOIN projects p ON p.id=f.project_id AND p.college_id=$1
		WHERE ((p.lifecycle='active' AND p.visibility IN ('campus','public')) OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=$2))
		AND ($3<>'following' OR EXISTS(SELECT 1 FROM project_follows pf WHERE pf.college_id=$1 AND pf.user_id=$2 AND pf.project_id=p.id))
		)
		SELECT v.id::text,v.project_id::text,v.project_title,v.actor_id::text,u.display_name,v.kind,v.summary,v.created_at
		FROM visible v JOIN users u ON u.id=v.actor_id AND u.college_id=$1 AND u.status='active'
		WHERE ($4::timestamptz IS NULL OR (v.created_at,v.id)<($4,$5::uuid))
		ORDER BY v.created_at DESC,v.id DESC LIMIT $6`, s.CollegeID, s.UserID, scope, nullableTimePoint(point), nullableIDPoint(point), discoveryPageSize+1)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	type feedItem struct {
		ID           string    `json:"id"`
		ProjectID    string    `json:"project_id"`
		ProjectTitle string    `json:"project_title"`
		ActorName    string    `json:"actor_name"`
		Kind         string    `json:"kind"`
		Summary      string    `json:"summary"`
		CreatedAt    time.Time `json:"created_at"`
	}
	items := []feedItem{}
	for rows.Next() {
		var item feedItem
		var actorID string
		if err = rows.Scan(&item.ID, &item.ProjectID, &item.ProjectTitle, &actorID, &item.ActorName, &item.Kind, &item.Summary, &item.CreatedAt); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, item)
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
	_ = response.OK(w, map[string]any{"items": items, "next_cursor": next, "scope": scope, "generated_at": time.Now().UTC()})
}

func nullableTimePoint(point *discoveryCursor) any {
	if point == nil {
		return nil
	}
	return point.CreatedAt
}

func nullableIDPoint(point *discoveryCursor) any {
	if point == nil {
		return nil
	}
	return point.ID
}

func (h *Handler) wacp(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	if !s.CampusAdmin {
		fail(w, http.StatusForbidden, "campus_admin_required", "campus administrator access is required")
		return
	}
	var count int64
	err := h.pool.QueryRow(r.Context(), `SELECT COALESCE((SELECT project_count FROM weekly_active_collaborative_projects WHERE college_id=$1),0)`, s.CollegeID).Scan(&count)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var latest time.Time
	var hasFreshness bool
	err = h.pool.QueryRow(r.Context(), `SELECT COALESCE(max(e.recorded_at),statement_timestamp()),count(*)>0
		FROM project_collaboration_events e
		JOIN project_members pm ON pm.college_id=e.college_id AND pm.project_id=e.project_id AND pm.user_id=e.actor_id
		JOIN projects p ON p.college_id=e.college_id AND p.id=e.project_id
		WHERE e.college_id=$1 AND e.occurred_at >= now()-interval '7 days' AND e.occurred_at <= now()
		AND p.lifecycle='active' AND p.github_revoke_deadline IS NULL`, s.CollegeID).Scan(&latest, &hasFreshness)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var sources []string
	rows, err := h.pool.Query(r.Context(), `SELECT DISTINCT e.source_type FROM project_collaboration_events e
		JOIN project_members pm ON pm.college_id=e.college_id AND pm.project_id=e.project_id AND pm.user_id=e.actor_id
		JOIN projects p ON p.college_id=e.college_id AND p.id=e.project_id
		WHERE e.college_id=$1 AND e.occurred_at >= now()-interval '7 days' AND e.occurred_at <= now()
		AND p.lifecycle='active' AND p.github_revoke_deadline IS NULL ORDER BY e.source_type`, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var source string
		if err = rows.Scan(&source); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		sources = append(sources, source)
	}
	if rows.Err() != nil {
		h.err.Handle(w, r, rows.Err())
		return
	}
	if sources == nil {
		sources = []string{}
	}
	var freshness any
	if hasFreshness {
		freshness = latest
	}
	_ = response.OK(w, map[string]any{
		"metric":           "Weekly Active Collaborative Projects",
		"value":            count,
		"window_start":     time.Now().UTC().Add(-7 * 24 * time.Hour),
		"window_end":       time.Now().UTC(),
		"freshness_at":     freshness,
		"sources":          sources,
		"descriptive_only": true,
	})
}
