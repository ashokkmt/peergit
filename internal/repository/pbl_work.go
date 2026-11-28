package repository

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
)

type pblWorkInput struct {
	Action   string  `json:"action"`
	Number   int64   `json:"number,omitempty"`
	Title    string  `json:"title,omitempty"`
	Body     string  `json:"body,omitempty"`
	Head     string  `json:"head,omitempty"`
	SHA      string  `json:"sha,omitempty"`
	Member   string  `json:"member,omitempty"`
	Decision string  `json:"decision,omitempty"`
	Labels   []int64 `json:"labels,omitempty"`
}

type pblWorkUser struct {
	Login string `json:"login"`
}

type pblWorkRef struct {
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
	Repo *struct {
		ID int64 `json:"id"`
	} `json:"repo,omitempty"`
}

type pblWorkItem struct {
	ID        int64         `json:"id"`
	Number    int64         `json:"number,omitempty"`
	Title     string        `json:"title,omitempty"`
	Body      string        `json:"body,omitempty"`
	State     string        `json:"state,omitempty"`
	User      pblWorkUser   `json:"user"`
	Head      *pblWorkRef   `json:"head,omitempty"`
	Base      *pblWorkRef   `json:"base,omitempty"`
	Merged    bool          `json:"merged,omitempty"`
	CommitID  string        `json:"commit_id,omitempty"`
	Dismissed bool          `json:"dismissed,omitempty"`
	Assignees []pblWorkUser `json:"assignees,omitempty"`
	Labels    []pblLabel    `json:"labels,omitempty"`
}

type pblLabel struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

var pblBranch = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)

func (f ForgeConfig) ensurePBLCollaborator(ctx context.Context, repo, id string) (string, error) {
	username := "pbl-" + id
	err := f.api(ctx, http.MethodGet, "/users/"+username, nil, nil)
	if errors.Is(err, forgeError(404)) {
		password, _, e := randomState()
		if e != nil {
			return "", e
		}
		err = f.api(ctx, http.MethodPost, "/admin/users", map[string]any{"username": username,
			"email": username + "@pbl.invalid", "password": password, "restricted": true,
			"must_change_password": false, "visibility": "private"}, nil)
		if errors.Is(err, forgeError(422)) || errors.Is(err, forgeError(409)) {
			err = f.api(ctx, http.MethodGet, "/users/"+username, nil, nil)
		}
	}
	if err == nil {
		err = f.api(ctx, http.MethodPut, repo+"/collaborators/"+username, map[string]string{"permission": "write"}, nil)
	}
	return username, err
}

func (h *Handler) pblWork(w http.ResponseWriter, r *http.Request) {
	s, ok := h.campusSession(w, r)
	if !ok {
		return
	}
	team := chi.URLParam(r, "team")
	name, _, ok := h.pblReadySource(w, r, s, team)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	endpoint := h.forge.repositoryPath(name)
	rows, err := h.pool.Query(ctx, `SELECT u.id::text,u.display_name,t.leader_id=u.id FROM pbl_active_team_members m
		JOIN users u ON u.id=m.user_id AND u.college_id=m.college_id JOIN pbl_teams t ON t.id=m.team_id
		WHERE m.team_id=$1 AND m.college_id=$2 ORDER BY u.display_name,u.id`, team, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	members := map[string]string{}
	leader := false
	for rows.Next() {
		var id, display string
		var lead bool
		if err = rows.Scan(&id, &display, &lead); err != nil {
			break
		}
		members[id] = display
		leader = leader || lead && id == s.UserID
	}
	rows.Close()
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	fail := func(err error) {
		if errors.Is(err, forgeError(404)) {
			h.fail(w, r, 404, "pbl_work_unavailable", "this work item is unavailable", nil)
		} else if errors.Is(err, forgeError(409)) || errors.Is(err, forgeError(422)) || errors.Is(err, forgeError(405)) {
			h.fail(w, r, 409, "pbl_work_conflict", "the branches, work item or review no longer allow this action", nil)
		} else {
			h.fail(w, r, 502, "pbl_work_unavailable", "repository work is temporarily unavailable", nil)
		}
	}
	number := r.URL.Query().Get("number")
	kind := r.URL.Query().Get("kind")
	if r.Method == http.MethodGet {
		if number == "" {
			var issues, pulls []pblWorkItem
			var labels []pblLabel
			if err = h.forge.api(ctx, http.MethodGet, endpoint+"/issues?type=issues&state=all&limit=50", nil, &issues); err == nil {
				err = h.forge.api(ctx, http.MethodGet, endpoint+"/pulls?state=all&limit=50", nil, &pulls)
			}
			if err == nil {
				err = h.forge.api(ctx, http.MethodGet, endpoint+"/labels?limit=50", nil, &labels)
			}
			if err != nil {
				fail(err)
				return
			}
			_ = response.OK(w, map[string]any{"issues": issues, "pulls": pulls, "members": members, "labels": labels})
			return
		}
		index, invalid := strconv.ParseInt(number, 10, 64)
		if invalid != nil || index < 1 || kind != "issues" && kind != "pulls" {
			h.fail(w, r, 400, "pbl_work_invalid", "choose an issue or pull request number", nil)
			return
		}
		itemPath := "/" + strconv.FormatInt(index, 10)
		var item pblWorkItem
		var comments, reviews []pblWorkItem
		if err = h.forge.api(ctx, http.MethodGet, endpoint+"/"+kind+itemPath, nil, &item); err == nil {
			err = h.forge.api(ctx, http.MethodGet, endpoint+"/issues"+itemPath+"/comments?limit=50", nil, &comments)
		}
		if err == nil && kind == "pulls" {
			err = h.forge.api(ctx, http.MethodGet, endpoint+"/pulls"+itemPath+"/reviews?limit=50", nil, &reviews)
		}
		if err != nil {
			fail(err)
			return
		}
		_ = response.OK(w, map[string]any{"item": item, "comments": comments, "reviews": reviews})
		return
	}
	if _, active := members[s.UserID]; !active {
		h.fail(w, r, 403, "pbl_work_denied", "only confirmed team members can change repository work", nil)
		return
	}
	in, err := request.Decode[pblWorkInput](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	invalid := func() {
		h.fail(w, r, 400, "pbl_work_invalid", "check the work item fields and current team access", nil)
	}
	in.Title, in.Body = strings.TrimSpace(in.Title), strings.TrimSpace(in.Body)
	if len(in.Title) > 160 || len(in.Body) > 8_000 || len(in.Labels) > 10 || in.Number < 0 {
		invalid()
		return
	}
	for _, id := range in.Labels {
		if id <= 0 {
			invalid()
			return
		}
	}
	target := "pbl-" + in.Member
	if in.Member != "" {
		if _, active := members[in.Member]; !active || in.Action == "pull" && in.Member == s.UserID {
			invalid()
			return
		}
	}
	method, route := http.MethodPost, ""
	var payload map[string]any
	itemPath := "/" + strconv.FormatInt(in.Number, 10)
	getPull := func() (pblWorkItem, error) {
		var item pblWorkItem
		err := h.forge.api(ctx, http.MethodGet, endpoint+"/pulls"+itemPath, nil, &item)
		if err == nil && (item.State != "open" || item.Head == nil || item.Base == nil || item.Head.Repo == nil ||
			item.Base.Repo == nil || item.Head.Repo.ID <= 0 || item.Head.Repo.ID != item.Base.Repo.ID || item.Head.SHA != in.SHA) {
			err = forgeError(409)
		}
		return item, err
	}
	switch in.Action {
	case "issue":
		if in.Title == "" {
			invalid()
			return
		}
		route, payload = "/issues", map[string]any{"title": in.Title, "body": in.Body, "labels": in.Labels}
		if in.Member != "" {
			payload["assignees"] = []string{target}
		}
	case "comment":
		if in.Number == 0 || in.Body == "" {
			invalid()
			return
		}
		route, payload = "/issues"+itemPath+"/comments", map[string]any{"body": in.Body}
	case "issue_state":
		if in.Number == 0 || in.Decision != "open" && in.Decision != "closed" {
			invalid()
			return
		}
		method, route, payload = http.MethodPatch, "/issues"+itemPath, map[string]any{"state": in.Decision}
	case "label":
		if in.Title == "" || len(in.Title) > 40 {
			invalid()
			return
		}
		route, payload = "/labels", map[string]any{"name": in.Title, "color": "526cad"}
	case "pull":
		var repo forgeRepository
		if in.Title == "" || !pblBranch.MatchString(in.Head) || in.Member == "" {
			invalid()
			return
		}
		if err = h.forge.api(ctx, http.MethodGet, endpoint, nil, &repo); err != nil {
			fail(err)
			return
		}
		if in.Head == repo.DefaultBranch {
			invalid()
			return
		}
		route, payload = "/pulls", map[string]any{"title": in.Title, "body": in.Body, "head": in.Head, "base": repo.DefaultBranch}
	case "review", "merge":
		if in.Number == 0 || !regexpCommitSHA(in.SHA) || in.Action == "merge" && !leader ||
			in.Action == "review" && in.Decision != "APPROVED" && in.Decision != "REQUEST_CHANGES" && in.Decision != "COMMENT" {
			invalid()
			return
		}
		pull, e := getPull()
		if e != nil {
			fail(e)
			return
		}
		if in.Action == "review" {
			if pull.User.Login == "pbl-"+s.UserID && in.Decision != "COMMENT" {
				invalid()
				return
			}
			route, payload = "/pulls"+itemPath+"/reviews", map[string]any{"body": in.Body, "event": in.Decision, "commit_id": in.SHA}
		} else {
			var reviews []pblWorkItem
			if err = h.forge.api(ctx, http.MethodGet, endpoint+"/pulls"+itemPath+"/reviews?limit=50", nil, &reviews); err != nil {
				fail(err)
				return
			}
			approved := false
			for _, review := range reviews {
				_, active := members[strings.TrimPrefix(review.User.Login, "pbl-")]
				approved = approved || active && !review.Dismissed && review.User.Login != pull.User.Login && review.CommitID == in.SHA && review.State == "APPROVED"
			}
			if !approved {
				h.fail(w, r, 409, "pbl_review_required", "a current teammate must approve the current commit", nil)
				return
			}
			route, payload = "/pulls"+itemPath+"/merge", map[string]any{"Do": "merge", "head_commit_id": in.SHA}
		}
	default:
		invalid()
		return
	}
	actor, err := h.forge.ensurePBLCollaborator(ctx, endpoint, s.UserID)
	if err == nil && in.Member != "" {
		_, err = h.forge.ensurePBLCollaborator(ctx, endpoint, in.Member)
	}
	var item json.RawMessage
	if err == nil {
		if in.Action == "merge" {
			err = h.forge.apiAs(ctx, method, endpoint+route, actor, payload, nil)
		} else {
			err = h.forge.apiAs(ctx, method, endpoint+route, actor, payload, &item)
		}
	}
	var result struct {
		Number int64 `json:"number"`
	}
	if err == nil && (in.Action == "pull" || in.Action == "issue") {
		err = json.Unmarshal(item, &result)
		if err == nil && in.Action == "pull" && result.Number > 0 {
			err = h.forge.apiAs(ctx, http.MethodPost, endpoint+"/pulls/"+strconv.FormatInt(result.Number, 10)+"/requested_reviewers",
				actor, map[string][]string{"reviewers": {target}}, nil)
		}
	}
	if err != nil {
		fail(err)
		return
	}
	_ = response.OK(w, result)
}
