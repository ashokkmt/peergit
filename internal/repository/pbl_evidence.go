package repository

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"peergit/internal/github"
	"peergit/internal/identity"
	"peergit/internal/platform/audit"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
	"peergit/internal/platform/jobs"
)

func (h *Handler) submitPBLCheckpoint(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	stage, ok := pblStage(r)
	if !ok {
		h.fail(w, r, 404, "pbl_checkpoint_unavailable", "choose a valid PBL checkpoint", nil)
		return
	}
	in, err := request.Decode[struct {
		SHA string `json:"sha"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !regexpCommitSHA(in.SHA) {
		h.fail(w, r, 400, "commit_sha_invalid", "select the exact commit SHA pushed to PeerGit", nil)
		return
	}
	if h.store == nil || h.forge.URL == "" {
		h.fail(w, r, 503, "snapshot_storage_unavailable", "PBL snapshot storage is unavailable", nil)
		return
	}
	team := chi.URLParam(r, "team")
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var project, repository, binding, name string
	var deadline time.Time
	err = tx.QueryRow(ctx, `SELECT p.id::text,r.id::text,b.id::text,b.repository_name,
		CASE $4::smallint WHEN 1 THEN o.phase_one_due WHEN 2 THEN o.phase_two_due ELSE o.phase_three_due END
		FROM pbl_teams t JOIN pbl_offerings o ON o.id=t.offering_id AND o.college_id=t.college_id
		JOIN projects p ON p.id=t.project_id AND p.college_id=t.college_id AND p.lifecycle<>'archived'
		JOIN repositories r ON r.project_id=p.id AND r.college_id=p.college_id
		JOIN repository_bindings b ON b.repository_id=r.id AND b.college_id=r.college_id AND b.is_current AND b.provider='forgejo'
		WHERE t.id=$1 AND t.college_id=$2 AND t.idea_state='approved' AND b.forge_instance=$5 AND b.access_state='active'
		AND EXISTS(SELECT 1 FROM pbl_active_team_members m WHERE m.team_id=t.id AND m.user_id=$3) FOR UPDATE OF t`,
		team, s.CollegeID, s.UserID, stage, h.forge.URL).Scan(&project, &repository, &binding, &name, &deadline)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "pbl_checkpoint_unavailable", "an approved idea and accessible PeerGit repository are required", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var id, sha, state string
	var receipt time.Time
	err = tx.QueryRow(ctx, `SELECT s.id::text,s.commit_sha,s.receipt_at,s.state FROM pbl_checkpoints k
		JOIN repository_snapshots s ON s.id=k.snapshot_id AND s.college_id=k.college_id WHERE k.team_id=$1 AND k.stage=$2`,
		team, stage).Scan(&id, &sha, &receipt, &state)
	if err == nil {
		if sha != in.SHA {
			h.fail(w, r, 409, "pbl_checkpoint_already_received", "this stage already has an immutable receipt", nil)
			return
		}
		_ = response.OK(w, pblReceipt(id, sha, state, receipt, deadline))
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		h.err.Handle(w, r, err)
		return
	}
	var pushed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pbl_git_pushes WHERE team_id=$1 AND college_id=$2 AND oid=$3)`,
		team, s.CollegeID, in.SHA).Scan(&pushed); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !pushed {
		h.fail(w, r, 409, "pbl_commit_not_pushed", "choose a commit recorded in the team's PeerGit push activity", nil)
		return
	}
	var commit github.Commit
	if err = h.forge.api(ctx, http.MethodGet, h.forge.repositoryPath(name)+"/git/commits/"+in.SHA, nil, &commit); err != nil || commit.SHA != in.SHA {
		h.fail(w, r, 409, "pbl_revision_unavailable", "the pushed commit is no longer available", nil)
		return
	}
	key := "pbl:" + team + ":" + strconv.Itoa(stage)
	hash := sha256.Sum256([]byte(key + ":" + in.SHA))
	err = tx.QueryRow(ctx, `INSERT INTO repository_snapshots(college_id,project_id,repository_id,binding_id,requested_by,idempotency_key,request_hash,
		requested_ref,commit_sha,receipt_at,membership_snapshot)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8,clock_timestamp(),
		(SELECT jsonb_agg(jsonb_build_object('user_id',m.user_id) ORDER BY m.user_id) FROM pbl_active_team_members m WHERE m.team_id=$9))
		RETURNING id::text,receipt_at`, s.CollegeID, project, repository, binding, s.UserID, key, hash[:], in.SHA, team).Scan(&id, &receipt)
	if err == nil {
		// Keep the accepted object reachable even if its original branch is rewritten before capture.
		tag := map[string]string{"tag_name": pblReceiptTag + id, "target": in.SHA}
		if pinErr := h.forge.api(ctx, http.MethodPost, h.forge.repositoryPath(name)+"/tags", tag, nil); pinErr != nil {
			h.fail(w, r, 502, "pbl_revision_unavailable", "the commit could not be retained for capture", nil)
			return
		}
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO pbl_checkpoints(college_id,team_id,project_id,stage,snapshot_id,deadline) VALUES($1,$2,$3,$4,$5,$6)`,
			s.CollegeID, team, project, stage, id, deadline)
	}
	if err == nil {
		payload, _ := json.Marshal(snapshotJob{SnapshotID: id})
		var jobID string
		jobID, err = jobs.EnqueueTx(ctx, tx, s.CollegeID, s.UserID, "pbl_snapshot", key, payload, nil)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE repository_snapshots SET job_id=$2 WHERE id=$1`, id, jobID)
		}
	}
	if err == nil {
		err = audit.Append(ctx, tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "pbl.checkpoint_received", ResourceType: "pbl_checkpoint", ResourceID: id})
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, pblReceipt(id, in.SHA, "pending", receipt, deadline))
}

func pblStage(r *http.Request) (int, bool) {
	stage, err := strconv.Atoi(chi.URLParam(r, "stage"))
	return stage, uuidOK(chi.URLParam(r, "team")) && err == nil && stage >= 1 && stage <= 3
}

func pblReceipt(id, sha, state string, receipt, deadline time.Time) map[string]any {
	return map[string]any{"id": id, "sha": sha, "state": state, "receipt_at": receipt, "deadline": deadline, "on_time": !receipt.After(deadline)}
}

func (h *Handler) downloadPBLCheckpoint(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	stage, ok := pblStage(r)
	if !ok || h.store == nil {
		h.fail(w, r, 404, "pbl_checkpoint_unavailable", "the verified archive is unavailable", nil)
		return
	}
	var key, sha string
	err := h.pool.QueryRow(r.Context(), `SELECT n.object_key,n.commit_sha FROM pbl_checkpoints k
		JOIN repository_snapshots n ON n.id=k.snapshot_id AND n.college_id=k.college_id AND n.state='verified'
		WHERE k.team_id=$1 AND k.college_id=$2 AND k.stage=$4 AND ($5
		 OR EXISTS(SELECT 1 FROM pbl_active_team_members m WHERE m.team_id=k.team_id AND m.user_id=$3)
		 OR EXISTS(SELECT 1 FROM pbl_active_faculty f WHERE f.team_id=k.team_id AND f.user_id=$3))`,
		chi.URLParam(r, "team"), s.CollegeID, s.UserID, stage, s.CampusAdmin && s.MFAEnabled && s.MFAFresh).Scan(&key, &sha)
	if err != nil {
		h.fail(w, r, 404, "pbl_checkpoint_unavailable", "the verified archive is unavailable", nil)
		return
	}
	h.serveSnapshotFile(w, r, key, sha)
}

func (h *Handler) retryPBLCheckpoint(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	stage, ok := pblStage(r)
	if !ok {
		h.fail(w, r, 404, "pbl_checkpoint_unavailable", "this checkpoint is unavailable", nil)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var snapshot string
	err = tx.QueryRow(ctx, `UPDATE jobs j SET state='ready',available_at=clock_timestamp(),attempts=0,last_error_code=NULL
		FROM repository_snapshots s JOIN pbl_checkpoints p ON p.snapshot_id=s.id
		WHERE j.id=s.job_id AND j.state='dead' AND p.team_id=$1 AND p.college_id=$2 AND p.stage=$4
		AND EXISTS(SELECT 1 FROM pbl_active_team_members m WHERE m.team_id=p.team_id AND m.user_id=$3)
		RETURNING s.id::text`, chi.URLParam(r, "team"), s.CollegeID, s.UserID, stage).Scan(&snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 409, "pbl_checkpoint_retry_unavailable", "only a current team member can retry a failed checkpoint", nil)
		return
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE repository_snapshots SET state='pending',failure_code=NULL WHERE id=$1 AND state<>'verified'`, snapshot)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) reviewPBLCheckpoint(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	stage, ok := pblStage(r)
	if !ok {
		h.fail(w, r, 404, "pbl_checkpoint_unavailable", "this checkpoint is unavailable", nil)
		return
	}
	in, err := request.Decode[struct {
		Feedback string `json:"feedback"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Feedback = strings.TrimSpace(in.Feedback)
	if len(in.Feedback) > 2000 {
		h.fail(w, r, 400, "pbl_feedback_invalid", "keep feedback within 2000 characters", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var team string
	err = tx.QueryRow(r.Context(), `SELECT t.id::text FROM pbl_teams t WHERE t.id=$1 AND t.college_id=$2 AND t.faculty_id=$3
		AND EXISTS(SELECT 1 FROM pbl_active_faculty f WHERE f.team_id=t.id AND f.user_id=$3) FOR UPDATE`,
		chi.URLParam(r, "team"), s.CollegeID, s.UserID).Scan(&team)
	var snapshot string
	if err == nil {
		err = tx.QueryRow(r.Context(), `UPDATE pbl_checkpoints SET feedback=$4 WHERE team_id=$1 AND college_id=$2 AND stage=$3 RETURNING snapshot_id::text`,
			team, s.CollegeID, stage, in.Feedback).Scan(&snapshot)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, 404, "pbl_checkpoint_unavailable", "only the assigned faculty can review this checkpoint", nil)
		return
	}
	if err == nil {
		err = audit.Append(r.Context(), tx, audit.Entry{TenantID: s.CollegeID, ActorID: s.UserID, Action: "pbl.checkpoint_reviewed", ResourceType: "pbl_checkpoint", ResourceID: snapshot})
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) pblEvidence(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	team := chi.URLParam(r, "team")
	_, repository, ok := h.pblReadySource(w, r, s, team)
	if !ok {
		return
	}
	var commits, pushes json.RawMessage
	err := h.pool.QueryRow(r.Context(), `SELECT COALESCE((SELECT jsonb_agg(to_jsonb(c)) FROM
		(SELECT x.canonical_id,x.title,x.author_name,x.occurred_at,COALESCE(u.display_name,'') AS verified_author
		 FROM contributions x LEFT JOIN users u ON u.college_id=x.college_id AND u.id=NULLIF(x.canonical_data->>'verified_author_id','')::uuid
		 WHERE x.college_id=$1 AND x.repository_id=$2 AND x.kind='commit' ORDER BY x.occurred_at DESC,x.id DESC LIMIT 50) c),'[]'),
		COALESCE((SELECT jsonb_agg(to_jsonb(p)) FROM
		(SELECT u.display_name AS pusher,g.ref,g.oid,g.observed_at FROM pbl_git_pushes g JOIN users u ON u.id=g.user_id AND u.college_id=g.college_id
		 WHERE g.college_id=$1 AND g.team_id=$3 ORDER BY g.observed_at DESC,g.id DESC LIMIT 50) p),'[]')`,
		s.CollegeID, repository, team).Scan(&commits, &pushes)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"commits": commits, "pushes": pushes})
}

func (h *Handler) pblReadySource(w http.ResponseWriter, r *http.Request, s identity.Session, team string) (string, string, bool) {
	if !uuidOK(team) || h.forge.URL == "" {
		h.fail(w, r, 404, "pbl_git_unavailable", "this PBL repository is unavailable", nil)
		return "", "", false
	}
	var name, repo string
	err := h.pool.QueryRow(r.Context(), `SELECT b.repository_name,p.id::text FROM pbl_provisions n
		JOIN repositories p ON p.project_id=n.project_id AND p.college_id=n.college_id
		JOIN repository_bindings b ON b.repository_id=p.id AND b.college_id=p.college_id
		WHERE n.team_id=$1 AND n.college_id=$2 AND n.state='ready' AND b.provider='forgejo' AND b.forge_instance=$4
		AND b.is_current AND b.access_state='active' AND ($5
			OR EXISTS(SELECT 1 FROM pbl_active_team_members m WHERE m.team_id=n.team_id AND m.user_id=$3)
			OR EXISTS(SELECT 1 FROM pbl_active_faculty f WHERE f.team_id=n.team_id AND f.user_id=$3))`,
		team, s.CollegeID, s.UserID, h.forge.URL, s.CampusAdmin && s.MFAEnabled && s.MFAFresh).Scan(&name, &repo)
	if err != nil {
		h.fail(w, r, 404, "pbl_git_unavailable", "this PBL repository is unavailable", nil)
		return "", "", false
	}
	w.Header().Set("Cache-Control", "private, no-store")
	return name, repo, true
}

func (h *Handler) pblBrowse(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	name, _, ok := h.pblReadySource(w, r, s, chi.URLParam(r, "team"))
	if !ok {
		return
	}
	query := r.URL.Query()
	revision, file := strings.TrimSpace(query.Get("ref")), query.Get("path")
	endpoint := h.forge.repositoryPath(name)
	if query.Get("view") == "diff" {
		if !regexpCommitSHA(revision) {
			h.fail(w, r, 400, "pbl_revision_invalid", "choose an exact revision", nil)
			return
		}
		res, err := h.forge.call(r.Context(), http.MethodGet, endpoint+"/git/commits/"+revision+".diff", nil)
		if err != nil {
			h.fail(w, r, 404, "pbl_revision_unavailable", "the revision diff is unavailable", nil)
			return
		}
		defer res.Body.Close()
		data, err := io.ReadAll(io.LimitReader(res.Body, (256<<10)+1))
		if err != nil || len(data) > 256<<10 {
			h.fail(w, r, 413, "pbl_diff_too_large", "the revision diff exceeds the browser limit", nil)
			return
		}
		_ = response.OK(w, map[string]string{"sha": revision, "diff": string(data)})
		return
	}
	if branch := query.Get("branch_history"); branch != "" {
		if !pblBranch.MatchString(branch) {
			h.fail(w, r, 400, "pbl_revision_invalid", "choose a branch for its history", nil)
			return
		}
		var commits []struct {
			SHA    string `json:"sha"`
			Commit struct {
				Message string `json:"message"`
			} `json:"commit"`
		}
		if h.forge.api(r.Context(), http.MethodGet, endpoint+"/commits?limit=30&stat=false&verification=false&files=false&sha="+url.QueryEscape(branch), nil, &commits) != nil {
			h.fail(w, r, 404, "pbl_revision_unavailable", "the selected branch history is unavailable", nil)
			return
		}
		_ = response.OK(w, map[string]any{"history": commits})
		return
	}
	if revision == "" && file == "" {
		var details forgeRepository
		type revisionRef struct {
			Name   string `json:"name"`
			Commit struct {
				ID  string `json:"id,omitempty"`
				SHA string `json:"sha,omitempty"`
			} `json:"commit"`
		}
		branches, tags := []revisionRef{}, []revisionRef{}
		if h.forge.api(r.Context(), http.MethodGet, endpoint, nil, &details) != nil {
			h.fail(w, r, 502, "pbl_source_unavailable", "source browsing is temporarily unavailable", nil)
			return
		}
		branchesErr := h.forge.api(r.Context(), http.MethodGet, endpoint+"/branches?limit=50", nil, &branches)
		if branchesErr != nil && !errors.Is(branchesErr, forgeError(409)) {
			h.fail(w, r, 502, "pbl_source_unavailable", "source browsing is temporarily unavailable", nil)
			return
		}
		tagsErr := h.forge.api(r.Context(), http.MethodGet, endpoint+"/tags?limit=50", nil, &tags)
		if tagsErr != nil && !errors.Is(tagsErr, forgeError(409)) {
			h.fail(w, r, 502, "pbl_source_unavailable", "tags are temporarily unavailable", nil)
			return
		}
		visibleTags := []revisionRef{}
		for _, tag := range tags {
			if !strings.HasPrefix(tag.Name, pblReceiptTag) {
				visibleTags = append(visibleTags, tag)
			}
		}
		_ = response.OK(w, map[string]any{"branches": branches, "tags": visibleTags, "default_branch": details.DefaultBranch})
		return
	}
	if !regexpCommitSHA(revision) || len(file) > 500 || strings.ContainsAny(file, "\\\x00") ||
		file != "" && (path.Clean("/"+file) != "/"+file || strings.HasPrefix(file, "/")) {
		h.fail(w, r, 400, "pbl_revision_invalid", "choose an exact revision and repository-relative file path", nil)
		return
	}
	segments := strings.Split(file, "/")
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	endpoint += "/contents"
	if file != "" {
		endpoint += "/" + strings.Join(segments, "/")
	}
	res, err := h.forge.call(r.Context(), http.MethodGet, endpoint+"?ref="+revision, nil)
	if err != nil {
		h.fail(w, r, 404, "pbl_file_unavailable", "the selected path or revision is unavailable", nil)
		return
	}
	defer res.Body.Close()
	type entry struct {
		Name     string `json:"name"`
		Path     string `json:"path"`
		Type     string `json:"type"`
		Size     int64  `json:"size"`
		Content  string `json:"content,omitempty"`
		Encoding string `json:"encoding,omitempty"`
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		h.fail(w, r, 413, "pbl_file_too_large", "the selected content exceeds the browser limit", nil)
		return
	}
	if len(data) > 0 && data[0] == '[' {
		var files []entry
		if json.Unmarshal(data, &files) == nil {
			_ = response.OK(w, map[string]any{"entries": files, "sha": revision})
			return
		}
	} else {
		var item entry
		if json.Unmarshal(data, &item) == nil {
			if item.Type != "file" || item.Size > 256<<10 {
				item.Content = ""
			}
			_ = response.OK(w, map[string]any{"file": item, "sha": revision})
			return
		}
	}
	h.fail(w, r, 502, "pbl_source_invalid", "could not read the provider response", nil)
}
