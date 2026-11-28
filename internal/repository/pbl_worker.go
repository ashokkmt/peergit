package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"peergit/internal/github"
	"peergit/internal/platform/jobs"
	"peergit/internal/platform/storage"
)

var PBLJobTypes = []string{"pbl_provision", "pbl_snapshot", "pbl_reconcile"}

func PBLJobHandlers(pool *pgxpool.Pool, api *github.Client, appID string, privateKey []byte, store *storage.Store, logger *slog.Logger, forge ForgeConfig) map[string]jobs.Handler {
	w := worker{pool: pool, api: api, appID: appID, privateKey: privateKey, store: store, logger: logger, forge: forge}
	return map[string]jobs.Handler{"pbl_provision": w.provisionPBL, "pbl_snapshot": w.capturePBL, "pbl_reconcile": w.reconcilePBL}
}

type pblSource struct {
	ID                        int64
	InstallationID, Name      string
	ExternalInstall, GitToken string
}

func (w *worker) pblSourceAccess(ctx context.Context, tx pgx.Tx, team string, source pblSource) (pblSource, error) {
	if source.ID == 0 {
		return source, nil
	}
	if !validRepositoryName(source.Name) {
		return source, errors.New("invalid GitHub source identity")
	}
	err := tx.QueryRow(ctx, `SELECT i.external_installation_id::text FROM pbl_provisions n
		JOIN github_installations i ON i.id=n.source_installation_id AND i.college_id=n.college_id AND i.status='active'
		JOIN github_installation_user_grants g ON g.installation_id=i.id AND g.college_id=n.college_id
			AND g.user_id=n.leader_id AND g.revoked_at IS NULL
		WHERE n.team_id=$1 AND n.source_installation_id=$2 AND n.source_repository_id=$3 AND n.source_full_name=$4`,
		team, source.InstallationID, source.ID, source.Name).Scan(&source.ExternalInstall)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return source, &jobs.PermanentError{Code: "pbl_source_revoked", Err: errors.New("GitHub source access was withdrawn")}
		}
		return source, err
	}
	appToken, err := github.JWT(w.appID, w.privateKey, time.Now())
	if err != nil {
		return source, err
	}
	source.GitToken, err = w.api.Token(ctx, source.ExternalInstall, appToken)
	if err != nil {
		return source, err
	}
	upstream, err := w.api.Repository(ctx, source.Name, source.GitToken)
	if err != nil {
		return source, err
	}
	if upstream.ID != source.ID || upstream.Size > pblRepositoryKiB {
		return source, &jobs.PermanentError{Code: "pbl_source_ineligible", Err: errors.New("GitHub source changed or exceeds the pilot quota")}
	}
	return source, nil
}

func (w *worker) provisionPBL(ctx context.Context, claim jobs.Claim) (string, error) {
	var payload struct {
		Team string `json:"team_id"`
	}
	if json.Unmarshal(claim.Payload, &payload) != nil || !uuidOK(payload.Team) || w.forge.URL == "" {
		return "", errors.New("invalid PBL provisioning job")
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var locked bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,61))`, payload.Team).Scan(&locked); err != nil {
		return "", err
	}
	if !locked {
		return "", errors.New("PBL provisioning is already in progress")
	}
	var project, repository, state string
	var source pblSource
	err = tx.QueryRow(ctx, `SELECT n.project_id::text,r.id::text,n.state,COALESCE(n.source_installation_id::text,''),
		COALESCE(n.source_repository_id,0),COALESCE(n.source_full_name,'')
		FROM pbl_provisions n JOIN repositories r ON r.project_id=n.project_id AND r.college_id=n.college_id
		WHERE n.team_id=$1 AND n.job_id=$2 AND n.college_id=$3 AND n.leader_id=$4
		AND EXISTS(SELECT 1 FROM pbl_active_team_members m WHERE m.team_id=n.team_id AND m.user_id=n.leader_id)`,
		payload.Team, claim.ID, claim.TenantID, claim.ActorID).
		Scan(&project, &repository, &state, &source.InstallationID, &source.ID, &source.Name)
	if err != nil {
		return "", err
	}
	if state == "ready" {
		return project, nil
	}
	name := "pbl-" + payload.Team
	var existing forgeRepository
	path := w.forge.repositoryPath(name)
	err = w.forge.api(ctx, http.MethodGet, path, nil, &existing)
	if err == nil {
		// Only this job can create the deterministic name, and queued repositories are never exposed.
		if !existing.Private {
			return "", errors.New("an unexpected repository occupies the PBL destination")
		}
		if err = w.forge.api(ctx, http.MethodDelete, path, nil, nil); err != nil {
			return "", err
		}
	} else if !errors.Is(err, forgeError(http.StatusNotFound)) {
		return "", err
	}
	source, err = w.pblSourceAccess(ctx, tx, payload.Team, source)
	if err != nil {
		return "", err
	}
	finished := false
	defer func() {
		if finished || ctx.Err() != nil {
			return
		}
		var partial forgeRepository
		if w.forge.api(ctx, http.MethodGet, path, nil, &partial) == nil && partial.Private {
			if cleanupErr := w.forge.api(ctx, http.MethodDelete, path, nil, nil); cleanupErr != nil {
				w.logger.WarnContext(ctx, "private PBL destination awaits retry cleanup", "team_id", payload.Team)
			}
		}
	}()
	var made forgeRepository
	var before map[string]string
	branch := ""
	if source.ID == 0 {
		err = w.forge.api(ctx, http.MethodPost, "/user/repos",
			map[string]any{"name": name, "private": true, "auto_init": false, "default_branch": "main"}, &made)
	} else {
		remote := "https://github.com/" + source.Name + ".git"
		before, err = gitRefs(ctx, remote, "x-access-token", source.GitToken)
		if err != nil || len(before) == 0 {
			return "", errors.New("could not verify the GitHub source refs")
		}
		err = w.forge.api(ctx, http.MethodPost, "/repos/migrate", map[string]any{
			"clone_addr": remote, "repo_name": name, "repo_owner": w.forge.Owner, "service": "git",
			"auth_username": "x-access-token", "auth_password": source.GitToken, "private": true,
			"mirror": false, "lfs": false, "issues": false, "pull_requests": false, "wiki": false,
		}, &made)
		branch = strings.TrimPrefix(before["HEAD"], "ref: refs/heads/")
	}
	if err != nil {
		return "", err
	}
	if made.ID <= 0 || !made.Private || made.Size > pblRepositoryKiB {
		return "", errors.New("destination returned by Forgejo is ineligible for PBL")
	}
	if source.ID != 0 {
		after, e := gitRefs(ctx, w.forge.gitURL(name), w.forge.Owner, w.forge.Token)
		if e != nil || !maps.Equal(before, after) || branch == "" || made.DefaultBranch != branch {
			return "", errors.New("PBL copy did not reproduce the authorized refs and default branch")
		}
		if _, err = w.pblSourceAccess(ctx, tx, payload.Team, source); err != nil {
			return "", err
		}
	}
	if err = w.forge.configureRepository(ctx, name, branch); err != nil {
		return "", err
	}
	if made.DefaultBranch == "" {
		made.DefaultBranch = "main"
	}
	var binding string
	err = tx.QueryRow(ctx, `INSERT INTO repository_bindings(college_id,repository_id,provider,forge_instance,
		external_repository_id,owner_login,repository_name,default_branch,linked_by,sync_health,last_synced_at)
		SELECT $1,$2,'forgejo',$3,$4,$5,$6,$7,$8,'healthy',clock_timestamp()
		WHERE EXISTS(SELECT 1 FROM jobs WHERE id=$9 AND generation=$10 AND worker=$11 AND state='running' AND lease_until>clock_timestamp())
		AND EXISTS(SELECT 1 FROM pbl_active_team_members m WHERE m.team_id=$12 AND m.user_id=$8)
		AND EXISTS(SELECT 1 FROM pbl_provisions n WHERE n.team_id=$12 AND (n.source_installation_id IS NULL OR EXISTS(
			SELECT 1 FROM github_installation_user_grants g JOIN github_installations i ON i.id=g.installation_id AND i.status='active'
			WHERE g.installation_id=n.source_installation_id AND g.college_id=n.college_id AND g.user_id=n.leader_id AND g.revoked_at IS NULL)))
		RETURNING id::text`, claim.TenantID, repository, w.forge.URL, made.ID, w.forge.Owner, name, made.DefaultBranch, claim.ActorID,
		claim.ID, claim.Generation, claim.Worker, payload.Team).Scan(&binding)
	if err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE pbl_provisions SET state='ready' WHERE team_id=$1`, payload.Team); err != nil {
		return "", err
	}
	if source.ID != 0 {
		job, _ := json.Marshal(map[string]string{"binding_id": binding})
		if _, err = jobs.EnqueueTx(ctx, tx, claim.TenantID, claim.ActorID, "pbl_reconcile", "initial:"+binding, job, nil); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		// An ambiguous commit must be resolved by reading the durable job state before deleting anything.
		finished = true
		return "", err
	}
	finished = true
	return project, nil
}

func (w *worker) capturePBL(ctx context.Context, c jobs.Claim) (string, error) {
	var in snapshotJob
	if json.Unmarshal(c.Payload, &in) != nil || !uuidOK(in.SnapshotID) || w.store == nil || w.forge.URL == "" {
		return "", errors.New("invalid PBL snapshot job or missing object store")
	}
	var project, name, sha, state, key string
	err := w.pool.QueryRow(ctx, `SELECT s.project_id::text,b.repository_name,s.commit_sha,s.state,COALESCE(s.object_key,'')
		FROM repository_snapshots s JOIN pbl_checkpoints p ON p.snapshot_id=s.id AND p.college_id=s.college_id
		JOIN repository_bindings b ON b.id=s.binding_id AND b.college_id=s.college_id AND b.repository_id=s.repository_id
		WHERE s.id=$1 AND s.college_id=$2 AND b.provider='forgejo' AND b.forge_instance=$3 AND b.is_current AND b.access_state='active'`,
		in.SnapshotID, c.TenantID, w.forge.URL).Scan(&project, &name, &sha, &state, &key)
	if err != nil || state == "verified" {
		return key, err
	}
	if err = w.beginCapture(ctx, c, in.SnapshotID); err != nil {
		return "", err
	}
	res, err := w.forge.call(ctx, http.MethodGet, w.forge.repositoryPath(name)+"/archive/"+url.PathEscape(sha)+".tar.gz", nil)
	if err != nil {
		w.failCapture(ctx, c, in.SnapshotID, "native_archive_failed")
		return "", err
	}
	defer res.Body.Close()
	receipt, err := w.store.Capture(ctx, res.Body, "snapshots/"+project+"/"+in.SnapshotID, maxArchiveBytes)
	if err != nil {
		w.failCapture(ctx, c, in.SnapshotID, "archive_capture_failed")
		return "", err
	}
	if err = w.finishCapture(ctx, c, in.SnapshotID, receipt); err != nil {
		_ = w.store.Delete(context.WithoutCancel(ctx), receipt.Key)
		return "", err
	}
	return receipt.Key, nil
}

func (w *worker) reconcilePBL(ctx context.Context, c jobs.Claim) (string, error) {
	var in struct {
		Binding string `json:"binding_id"`
	}
	if json.Unmarshal(c.Payload, &in) != nil || !uuidOK(in.Binding) || w.forge.URL == "" {
		return "", errors.New("invalid native evidence job")
	}
	var name, repository, team string
	err := w.pool.QueryRow(ctx, `SELECT b.repository_name,b.repository_id::text,n.team_id::text FROM repository_bindings b
		JOIN repositories r ON r.id=b.repository_id AND r.college_id=b.college_id
		JOIN pbl_provisions n ON n.project_id=r.project_id AND n.college_id=r.college_id
		WHERE b.id=$1 AND b.college_id=$2 AND b.provider='forgejo' AND b.forge_instance=$3 AND b.is_current AND b.access_state='active'`,
		in.Binding, c.TenantID, w.forge.URL).Scan(&name, &repository, &team)
	if err != nil {
		return "", err
	}
	repoPath := w.forge.repositoryPath(name)
	var collaborators []pblWorkUser
	if err = w.forge.api(ctx, http.MethodGet, repoPath+"/collaborators?limit=50", nil, &collaborators); err != nil {
		return "", err
	}
	for _, collaborator := range collaborators {
		id, managed := strings.CutPrefix(collaborator.Login, "pbl-")
		if !managed || !uuidOK(id) {
			continue
		}
		var active bool
		if err = w.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pbl_active_team_members m WHERE
			m.team_id=$1 AND m.college_id=$2 AND m.user_id=$3
			AND EXISTS(SELECT 1 FROM consent_records c WHERE c.user_id=$3 AND c.purpose='terms' AND c.revoked_at IS NULL)
			AND EXISTS(SELECT 1 FROM consent_records c WHERE c.user_id=$3 AND c.purpose='privacy' AND c.revoked_at IS NULL))`,
			team, c.TenantID, id).Scan(&active); err != nil {
			return "", err
		}
		if !active {
			err = w.forge.api(ctx, http.MethodDelete, repoPath+"/collaborators/"+collaborator.Login, nil, nil)
			if err != nil && !errors.Is(err, forgeError(404)) {
				return "", err
			}
		}
	}
	refs, err := gitRefs(ctx, w.forge.gitURL(name), w.forge.Owner, w.forge.Token)
	if err != nil {
		return "", err
	}
	seen := make(map[string]bool)
	refCount := 0
	// shortcut: observe the newest 50 commits on up to 50 refs per pass, page deeper when the approved pilot requires it.
	for ref, oid := range refs {
		if !strings.HasPrefix(ref, "refs/heads/") || seen[oid] || refCount >= 50 {
			continue
		}
		refCount++
		var commits []struct {
			SHA    string       `json:"sha"`
			Author *github.User `json:"author"`
			Commit struct {
				github.CommitDetail
				Verification struct {
					Verified bool `json:"verified"`
					Signer   struct {
						Username string `json:"username"`
					} `json:"signer"`
				} `json:"verification"`
			} `json:"commit"`
		}
		branch := url.QueryEscape(strings.TrimPrefix(ref, "refs/heads/"))
		if err = w.forge.api(ctx, http.MethodGet, repoPath+"/commits?limit=50&stat=false&files=false&sha="+branch, nil, &commits); err != nil {
			return "", err
		}
		for _, commit := range commits {
			if !regexpCommitSHA(commit.SHA) || commit.Commit.Author.Date.IsZero() || seen[commit.SHA] {
				continue
			}
			seen[commit.SHA] = true
			title := strings.TrimSpace(strings.SplitN(commit.Commit.Message, "\n", 2)[0])
			verifiedID, peerIdentity := strings.CutPrefix(commit.Commit.Verification.Signer.Username, "pbl-")
			if !peerIdentity || !uuidOK(verifiedID) || !commit.Commit.Verification.Verified ||
				commit.Author == nil || commit.Author.Login != commit.Commit.Verification.Signer.Username {
				verifiedID = ""
			}
			if _, err = w.pool.Exec(ctx, `INSERT INTO contributions(college_id,repository_id,kind,canonical_id,author_name,title,occurred_at,canonical_data)
				VALUES($1,$2,'commit',$3,$4,$5,$6,COALESCE((
					SELECT jsonb_build_object('verified_author_id',m.user_id) FROM pbl_active_team_members m
					JOIN pbl_provisions n ON n.team_id=m.team_id AND n.college_id=m.college_id
					JOIN repositories r ON r.project_id=n.project_id AND r.college_id=n.college_id
					WHERE m.user_id=NULLIF($7,'')::uuid AND m.college_id=$1 AND r.id=$2),'{}'::jsonb))
				ON CONFLICT(repository_id,kind,canonical_id) DO UPDATE SET canonical_data=excluded.canonical_data
				WHERE excluded.canonical_data<>'{}'::jsonb AND contributions.canonical_data='{}'::jsonb`,
				c.TenantID, repository, "git:sha1:"+commit.SHA, truncate(commit.Commit.Author.Name, 160), truncate(title, 500), commit.Commit.Author.Date, verifiedID); err != nil {
				return "", fmt.Errorf("record native commit evidence: %w", err)
			}
		}
	}
	_, err = w.pool.Exec(ctx, `UPDATE repository_bindings SET sync_health='healthy',last_synced_at=clock_timestamp() WHERE id=$1 AND college_id=$2`,
		in.Binding, c.TenantID)
	return "", err
}

func SchedulePBLReconciliation(ctx context.Context, pool *pgxpool.Pool, forge string) error {
	rows, err := pool.Query(ctx, `SELECT id::text,college_id::text,linked_by::text FROM repository_bindings
		WHERE provider='forgejo' AND forge_instance=$1 AND is_current AND access_state='active'
		AND (last_synced_at IS NULL OR last_synced_at < clock_timestamp()-interval '30 minutes')
		ORDER BY last_synced_at NULLS FIRST,id LIMIT 100`, forge)
	if err != nil {
		return err
	}
	targets, err := pgx.CollectRows(rows, pgx.RowTo[struct{ ID, College, Actor string }])
	if err != nil {
		return err
	}
	queue := jobs.Queue{Pool: pool}
	for _, t := range targets {
		payload, _ := json.Marshal(bindingJob{BindingID: t.ID})
		key := "pbl-hour:" + t.ID + ":" + time.Now().UTC().Format("2006010215")
		if _, err = queue.EnqueueWork(ctx, t.College, t.Actor, "pbl_reconcile", key, payload, nil); err != nil {
			return err
		}
	}
	return nil
}
