package repository

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"peergit/internal/github"
	"peergit/internal/platform/http/response"
)

func (h *Handler) browserBinding(w http.ResponseWriter, r *http.Request) (string, string, string, string, bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	s, ok := h.campusSession(w, r)
	if !ok {
		return "", "", "", "", false
	}
	pid := chi.URLParam(r, "id")
	if !h.isProjectMember(r.Context(), pid, s) {
		h.fail(w, r, 404, "project_not_found", "project was not found", nil)
		return "", "", "", "", false
	}
	var owner, name, install, branch string
	err := h.pool.QueryRow(r.Context(), `SELECT b.owner_login,b.repository_name,i.external_installation_id::text,b.default_branch FROM repository_bindings b JOIN repositories rp ON rp.id=b.repository_id AND rp.college_id=b.college_id JOIN github_installations i ON i.id=b.installation_id AND i.college_id=b.college_id JOIN projects p ON p.id=rp.project_id AND p.college_id=rp.college_id WHERE p.id=$1 AND p.college_id=$2 AND p.github_revoke_deadline IS NULL AND b.is_current AND b.access_state='active' AND i.status='active'`, pid, s.CollegeID).Scan(&owner, &name, &install, &branch)
	if err != nil {
		h.fail(w, r, 404, "repository_not_available", "repository source is not available", err)
		return "", "", "", "", false
	}
	return owner, name, install, branch, true
}

func (h *Handler) browserToken(r *http.Request, install string) (string, error) {
	jwt, err := github.JWT(h.app.ID, h.app.PrivateKey, time.Now())
	if err != nil {
		h.err.Report(r, err)
		return "", err
	}
	token, err := h.api.Token(r.Context(), install, jwt)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (h *Handler) repositoryTree(w http.ResponseWriter, r *http.Request) {
	owner, name, install, branch, ok := h.browserBinding(w, r)
	if !ok {
		return
	}
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	pinned := r.URL.Query().Get("sha")
	if pinned != "" {
		if !regexpCommitSHA(pinned) {
			h.fail(w, r, 400, "commit_sha_invalid", "select a valid commit", nil)
			return
		}
		ref = pinned
	}
	if ref == "" {
		ref = branch
	}
	if ref == "" || len(ref) > 255 {
		h.fail(w, r, 400, "repository_ref_invalid", "select a valid branch", nil)
		return
	}
	token, err := h.browserToken(r, install)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	repo := owner + "/" + name
	sha := pinned
	if sha == "" {
		sha, err = h.api.Resolve(ctx, repo, ref, token)
		if err != nil {
			h.providerFailure(w, r, err)
			return
		}
	}
	tree, truncated, err := h.api.BrowseTree(ctx, repo, sha, token)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"ref": ref, "sha": sha, "tree": tree, "truncated": truncated})
}

func (h *Handler) repositoryHistory(w http.ResponseWriter, r *http.Request) {
	owner, name, install, ref, ok := h.browserBinding(w, r)
	if !ok {
		return
	}
	if selected := strings.TrimSpace(r.URL.Query().Get("ref")); selected != "" {
		ref = selected
	}
	if ref == "" || len(ref) > 255 {
		h.fail(w, r, 400, "repository_ref_invalid", "select a valid branch", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	token, err := h.browserToken(r.WithContext(ctx), install)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	items, err := h.api.BranchCommits(ctx, owner+"/"+name, token, ref)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items, "ref": ref})
}

func (h *Handler) repositoryBranches(w http.ResponseWriter, r *http.Request) {
	owner, name, install, defaultBranch, ok := h.browserBinding(w, r)
	if !ok {
		return
	}
	token, err := h.browserToken(r, install)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	items, err := h.api.Branches(ctx, owner+"/"+name, token)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items, "default_branch": defaultBranch})
}

func (h *Handler) repositoryCommit(w http.ResponseWriter, r *http.Request) {
	owner, name, install, _, ok := h.browserBinding(w, r)
	if !ok {
		return
	}
	sha := strings.TrimPrefix(r.URL.Query().Get("sha"), "git:sha1:")
	if !regexpCommitSHA(sha) {
		h.fail(w, r, 400, "commit_sha_invalid", "select a valid commit", nil)
		return
	}
	token, err := h.browserToken(r, install)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	files, err := h.api.CommitDiff(ctx, owner+"/"+name, sha, token)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"sha": sha, "files": files})
}

func regexpCommitSHA(s string) bool { return len(s) == 40 && strings.Trim(s, "0123456789abcdef") == "" }

func (h *Handler) repositoryFile(w http.ResponseWriter, r *http.Request) {
	owner, name, install, branch, ok := h.browserBinding(w, r)
	if !ok {
		return
	}
	path := r.URL.Query().Get("path")
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if ref == "" {
		ref = branch
	}
	token, err := h.browserToken(r, install)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	content, err := h.api.File(ctx, owner+"/"+name, path, ref, token)
	if err != nil {
		h.providerFailure(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"path": path, "ref": ref, "content": content})
}
