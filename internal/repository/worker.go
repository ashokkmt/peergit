package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"peergit/internal/github"
	"peergit/internal/platform/jobs"
	"peergit/internal/platform/storage"
)

func JobHandlers(pool *pgxpool.Pool, api *github.Client, appID string, privateKey []byte, store *storage.Store, logger *slog.Logger) map[string]jobs.Handler {
	w := &worker{pool: pool, api: api, appID: appID, privateKey: privateKey, store: store, logger: logger}
	return map[string]jobs.Handler{
		"github_webhook":   w.webhook,
		"github_reconcile": w.reconcile,
		"snapshot":         w.capture,
	}
}

type worker struct {
	pool       *pgxpool.Pool
	api        *github.Client
	appID      string
	privateKey []byte
	store      *storage.Store
	logger     *slog.Logger
}

type bindingJob struct {
	BindingID string `json:"binding_id"`
}
type snapshotJob struct {
	SnapshotID string `json:"snapshot_id"`
}
type webhookJob struct {
	DeliveryID string `json:"delivery_id"`
}

func (w *worker) reconcile(ctx context.Context, claim jobs.Claim) (string, error) {
	var payload bindingJob
	if err := json.Unmarshal(claim.Payload, &payload); err != nil || !uuidOK(payload.BindingID) {
		return "", errors.New("invalid GitHub reconciliation job")
	}
	var id, owner, name, externalInstall string
	var repoID int64
	var localInstall string
	var state string
	var permissions []byte
	err := w.pool.QueryRow(ctx, `SELECT b.id::text,b.owner_login,b.repository_name,b.external_repository_id,i.external_installation_id::text,i.id::text,i.status,i.permissions
		FROM repository_bindings b JOIN github_installations i ON i.id=b.installation_id AND i.college_id=b.college_id
		WHERE b.id=$1 AND b.is_current`, payload.BindingID).Scan(&id, &owner, &name, &repoID, &externalInstall, &localInstall, &state, &permissions)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if state != "active" {
		_, err = w.pool.Exec(ctx, `UPDATE repository_bindings SET access_state='permission_lost',sync_health='failed',last_error_code='installation_inactive' WHERE id=$1`, id)
		return "", err
	}
	jwt, err := github.JWT(w.appID, w.privateKey, time.Now())
	if err != nil {
		return "", err
	}
	token, err := w.api.Token(ctx, externalInstall, jwt)
	if err != nil {
		if github.IsDenied(err) {
			_, dbErr := w.pool.Exec(ctx, `UPDATE repository_bindings SET access_state='permission_lost',sync_health='failed',last_error_code='provider_access_lost' WHERE id=$1 AND is_current`, id)
			return "", dbErr
		}
		return "", err
	}
	repo, err := w.api.Repository(ctx, owner+"/"+name, token)
	if github.IsDenied(err) {
		_, dbErr := w.pool.Exec(ctx, `UPDATE repository_bindings SET access_state='permission_lost',sync_health='failed',last_error_code='provider_access_lost' WHERE id=$1`, id)
		return "", dbErr
	}
	if err != nil {
		return "", err
	}
	if repo.ID != repoID {
		_, err = w.pool.Exec(ctx, `UPDATE repository_bindings SET access_state='stale',sync_health='failed',last_error_code='repository_identity_mismatch' WHERE id=$1`, id)
		return "", err
	}
	parts := strings.SplitN(repo.FullName, "/", 2)
	if len(parts) != 2 || !repoSegment.MatchString(parts[0]) || !repoSegment.MatchString(parts[1]) {
		return "", errors.New("GitHub returned an invalid current repository name")
	}
	_, err = w.pool.Exec(ctx, `UPDATE repository_bindings SET default_branch=$2,visibility=$3,owner_login=$4,repository_name=$5 WHERE id=$1 AND is_current`, id, repo.DefaultBranch, visibility(repo), parts[0], parts[1])
	if err != nil {
		return "", err
	}
	var cursorAt *time.Time
	var cursorSHA *string
	err = w.pool.QueryRow(ctx, `SELECT cursor_at,cursor_sha FROM repository_sync_cursors WHERE binding_id=$1 AND scope='commits'`, id).Scan(&cursorAt, &cursorSHA)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	since := time.Time{}
	if cursorAt != nil {
		since = cursorAt.Add(-2 * time.Second)
	}
	var latestAt time.Time
	var latestSHA string
	for page := 1; page <= 100; page++ {
		commits, fetchErr := w.api.Commits(ctx, owner+"/"+name, token, since, page)
		if fetchErr != nil {
			return "", fetchErr
		}
		if len(commits) == 0 {
			break
		}
		if page == 1 {
			latestSHA = commits[0].SHA
			latestAt = commits[0].Commit.Author.Date
		}
		stop := false
		for _, commit := range commits {
			if cursorSHA != nil && commit.SHA == *cursorSHA {
				stop = true
				break
			}
			if err = w.saveCommit(ctx, id, commit); err != nil {
				return "", err
			}
		}
		if stop || len(commits) < 100 {
			break
		}
	}
	if latestSHA != "" {
		_, err = w.pool.Exec(ctx, `INSERT INTO repository_sync_cursors(college_id,repository_id,binding_id,scope,cursor_at,cursor_sha)
			SELECT college_id,repository_id,id,'commits',$2,$3 FROM repository_bindings WHERE id=$1
			ON CONFLICT(binding_id,scope) DO UPDATE SET cursor_at=EXCLUDED.cursor_at,cursor_sha=EXCLUDED.cursor_sha,updated_at=clock_timestamp()`, id, nullableTime(latestAt), latestSHA)
		if err != nil {
			return "", err
		}
	}
	var grants map[string]string
	_ = json.Unmarshal(permissions, &grants)
	if grants["pull_requests"] == "read" {
		if err = w.syncActivity(ctx, id, owner+"/"+name, token, "pull_requests"); err != nil {
			return "", err
		}
	}
	if grants["issues"] == "read" {
		if err = w.syncActivity(ctx, id, owner+"/"+name, token, "issues"); err != nil {
			return "", err
		}
	}
	_, err = w.pool.Exec(ctx, `UPDATE repository_bindings SET access_state='active',sync_health='healthy',last_synced_at=clock_timestamp(),last_error_code=NULL WHERE id=$1 AND is_current`, id)
	return "", err
}

func (w *worker) syncActivity(ctx context.Context, bindingID, repo, token, scope string) error {
	var cursorAt *time.Time
	err := w.pool.QueryRow(ctx, `SELECT cursor_at FROM repository_sync_cursors WHERE binding_id=$1 AND scope=$2`, bindingID, scope).Scan(&cursorAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var latest time.Time
	for page := 1; page <= 100; page++ {
		var items []github.Activity
		if scope == "pull_requests" {
			items, err = w.api.PullRequests(ctx, repo, token, page)
		} else {
			since := time.Time{}
			if cursorAt != nil {
				since = cursorAt.Add(-time.Second)
			}
			items, err = w.api.Issues(ctx, repo, token, since, page)
		}
		if err != nil {
			return err
		}
		if len(items) == 0 {
			break
		}
		if page == 1 {
			latest = items[0].UpdatedAt
		}
		stop := false
		for _, item := range items {
			if cursorAt != nil && !item.UpdatedAt.After(*cursorAt) {
				stop = true
				break
			}
			if scope == "issues" && item.PullRequest != nil {
				continue
			}
			kind := "pull_request"
			if scope == "issues" {
				kind = "issue"
			}
			if err = w.saveActivity(ctx, bindingID, repo, kind, item); err != nil {
				return err
			}
		}
		if stop || len(items) < 100 {
			break
		}
	}
	if !latest.IsZero() {
		_, err = w.pool.Exec(ctx, `INSERT INTO repository_sync_cursors(college_id,repository_id,binding_id,scope,cursor_at)
		SELECT college_id,repository_id,id,$2,$3 FROM repository_bindings WHERE id=$1 ON CONFLICT(binding_id,scope) DO UPDATE SET cursor_at=EXCLUDED.cursor_at,updated_at=clock_timestamp()`, bindingID, scope, latest)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *worker) saveActivity(ctx context.Context, bindingID, repo, kind string, item github.Activity) error {
	var repoID string
	var collegeID string
	var externalRepo int64
	if err := w.pool.QueryRow(ctx, `SELECT repository_id::text,college_id::text,external_repository_id FROM repository_bindings WHERE id=$1 AND is_current AND access_state='active'`, bindingID).Scan(&repoID, &collegeID, &externalRepo); err != nil {
		return err
	}
	body := item.Body
	if len(body) > 2000 {
		body = body[:2000]
	}
	login := login(item.User)
	author := authorID(item.User)
	canonical := fmt.Sprintf("github:%d:%s:%d", externalRepo, kind, item.Number)
	data, err := json.Marshal(map[string]any{"number": item.Number, "state": item.State, "title": item.Title, "body": body, "comments": item.Comments, "author_id": author, "author_login": login, "created_at": item.CreatedAt, "updated_at": item.UpdatedAt})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var contributionID string
	err = tx.QueryRow(ctx, `INSERT INTO contributions(college_id,repository_id,kind,canonical_id,github_author_id,author_login,title,summary,occurred_at,canonical_data)
		VALUES($1,$2,$3,$4,NULLIF($5,0),$6,$7,$8,$9,$10)
		ON CONFLICT(repository_id,kind,canonical_id) DO UPDATE SET github_author_id=EXCLUDED.github_author_id,author_login=EXCLUDED.author_login,title=EXCLUDED.title,summary=EXCLUDED.summary,occurred_at=EXCLUDED.occurred_at,canonical_data=EXCLUDED.canonical_data,last_observed_at=clock_timestamp() RETURNING id::text`, collegeID, repoID, kind, canonical, author, login, item.Title, body, nullableTime(item.CreatedAt), data).Scan(&contributionID)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO contribution_observations(college_id,repository_id,binding_id,contribution_id,provider_updated_at,observation_hash,data) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(binding_id,contribution_id,observation_hash) DO NOTHING`, collegeID, repoID, bindingID, contributionID, nullableTime(item.UpdatedAt), digest[:], data); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *worker) saveCommit(ctx context.Context, bindingID string, commit github.Commit) error {
	data, err := json.Marshal(map[string]any{"message": commit.Commit.Message, "author_name": commit.Commit.Author.Name, "author_login": login(commit.Author), "author_id": authorID(commit.Author), "additions": commit.Stats.Additions, "deletions": commit.Stats.Deletions, "total": commit.Stats.Total})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var collegeID, repositoryID string
	if err = tx.QueryRow(ctx, `SELECT college_id::text,repository_id::text FROM repository_bindings WHERE id=$1 AND is_current AND access_state='active'`, bindingID).Scan(&collegeID, &repositoryID); err != nil {
		return err
	}
	var contributionID string
	err = tx.QueryRow(ctx, `INSERT INTO contributions(college_id,repository_id,kind,canonical_id,github_author_id,author_login,author_name,title,summary,occurred_at,additions,deletions,canonical_data)
		VALUES($1,$2,'commit',$3,NULLIF($4,0),$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT(repository_id,kind,canonical_id) DO UPDATE SET github_author_id=EXCLUDED.github_author_id,author_login=EXCLUDED.author_login,author_name=EXCLUDED.author_name,title=EXCLUDED.title,summary=EXCLUDED.summary,occurred_at=EXCLUDED.occurred_at,additions=EXCLUDED.additions,deletions=EXCLUDED.deletions,canonical_data=EXCLUDED.canonical_data,last_observed_at=clock_timestamp()
		RETURNING id::text`, collegeID, repositoryID, "git:sha1:"+commit.SHA, authorID(commit.Author), login(commit.Author), commit.Commit.Author.Name, firstLine(commit.Commit.Message), commit.Commit.Message, nullableTime(commit.Commit.Author.Date), commit.Stats.Additions, commit.Stats.Deletions, data).Scan(&contributionID)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO contribution_observations(college_id,repository_id,binding_id,contribution_id,observation_hash,data)
		VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(binding_id,contribution_id,observation_hash) DO NOTHING`, collegeID, repositoryID, bindingID, contributionID, digest[:], data); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *worker) capture(ctx context.Context, claim jobs.Claim) (string, error) {
	var payload snapshotJob
	if err := json.Unmarshal(claim.Payload, &payload); err != nil || !uuidOK(payload.SnapshotID) {
		return "", errors.New("invalid snapshot job")
	}
	if w.store == nil {
		return "", errors.New("snapshot storage is not configured")
	}
	var projectID, repoID, bindingID, owner, name, sha, installID, state, existingKey string
	var externalRepo int64
	err := w.pool.QueryRow(ctx, `SELECT s.project_id::text,s.repository_id::text,s.binding_id::text,b.owner_login,b.repository_name,s.commit_sha,i.external_installation_id::text,s.state,COALESCE(s.object_key,''),b.external_repository_id
		FROM repository_snapshots s JOIN repository_bindings b ON b.id=s.binding_id AND b.repository_id=s.repository_id AND b.college_id=s.college_id JOIN github_installations i ON i.id=b.installation_id AND i.college_id=b.college_id
		WHERE s.id=$1 AND b.is_current AND b.access_state='active' AND i.status='active'`, payload.SnapshotID).Scan(&projectID, &repoID, &bindingID, &owner, &name, &sha, &installID, &state, &existingKey, &externalRepo)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("snapshot repository permission is no longer active")
	}
	if err != nil {
		return "", err
	}
	if state == "verified" && existingKey != "" {
		return existingKey, nil
	}
	if state != "pending" && state != "failed" && state != "capturing" {
		return "", fmt.Errorf("snapshot has invalid state %q", state)
	}
	if err = w.beginCapture(ctx, claim, payload.SnapshotID); err != nil {
		return "", err
	}
	jwt, err := github.JWT(w.appID, w.privateKey, time.Now())
	if err != nil {
		w.failCapture(ctx, claim, payload.SnapshotID, "app_auth_failed")
		return "", err
	}
	token, err := w.api.Token(ctx, installID, jwt)
	if err != nil {
		if github.IsDenied(err) {
			w.markBindingPermissionLost(ctx, bindingID, "installation_access_lost")
		}
		w.failCapture(ctx, claim, payload.SnapshotID, "installation_token_failed")
		return "", err
	}
	repo, err := w.api.Repository(ctx, owner+"/"+name, token)
	if err != nil {
		if github.IsDenied(err) {
			w.markBindingPermissionLost(ctx, bindingID, "repository_access_lost")
		}
		w.failCapture(ctx, claim, payload.SnapshotID, "repository_access_failed")
		return "", err
	}
	if repo.ID != externalRepo {
		w.failCapture(ctx, claim, payload.SnapshotID, "repository_identity_mismatch")
		return "", errors.New("repository identity changed")
	}
	archive, err := w.api.Archive(ctx, owner+"/"+name, sha, token)
	if err != nil {
		if github.IsDenied(err) {
			w.markBindingPermissionLost(ctx, bindingID, "repository_access_lost")
		}
		w.failCapture(ctx, claim, payload.SnapshotID, "archive_download_failed")
		return "", err
	}
	defer archive.Close()
	receipt, err := w.store.Capture(ctx, archive, "snapshots/"+projectID+"/"+payload.SnapshotID, maxArchiveBytes)
	if err != nil {
		w.failCapture(ctx, claim, payload.SnapshotID, "archive_capture_failed")
		return "", err
	}
	if err = w.finishCapture(ctx, claim, payload.SnapshotID, receipt); err != nil {
		return "", err
	}
	return receipt.Key, nil
}

func (w *worker) markBindingPermissionLost(ctx context.Context, bindingID, code string) {
	_, _ = w.pool.Exec(ctx, `UPDATE repository_bindings SET access_state='permission_lost',sync_health='failed',last_error_code=$2 WHERE id=$1 AND is_current`, bindingID, code)
}

func (w *worker) beginCapture(ctx context.Context, c jobs.Claim, snapshotID string) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var yes bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE id=$1 AND worker=$2 AND generation=$3 AND state='running' AND lease_until>clock_timestamp())`, c.ID, c.Worker, c.Generation).Scan(&yes); err != nil {
		return err
	}
	if !yes {
		return jobs.ErrStale
	}
	_, err = tx.Exec(ctx, `UPDATE repository_snapshots SET state='capturing',attempt_generation=$2,capture_started_at=clock_timestamp(),failure_code=NULL WHERE id=$1 AND state<>'verified' AND attempt_generation<$2`, snapshotID, c.Generation)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (w *worker) failCapture(ctx context.Context, c jobs.Claim, snapshotID, code string) {
	_, _ = w.pool.Exec(ctx, `UPDATE repository_snapshots SET state='failed',failure_code=$3 WHERE id=$1 AND attempt_generation=$2 AND state='capturing'`, snapshotID, c.Generation, code)
}
func (w *worker) finishCapture(ctx context.Context, c jobs.Claim, snapshotID string, receipt storage.Receipt) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var yes bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE id=$1 AND worker=$2 AND generation=$3 AND state='running' AND lease_until>clock_timestamp())`, c.ID, c.Worker, c.Generation).Scan(&yes); err != nil {
		return err
	}
	if !yes {
		return jobs.ErrStale
	}
	tag, err := tx.Exec(ctx, `UPDATE repository_snapshots SET state='verified',object_key=$3,archive_bytes=$4,archive_sha256=$5,verified_at=clock_timestamp(),failure_code=NULL WHERE id=$1 AND attempt_generation=$2 AND state='capturing'`, snapshotID, c.Generation, receipt.Key, receipt.Bytes, receipt.SHA256)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return jobs.ErrStale
	}
	return tx.Commit(ctx)
}

func (w *worker) webhook(ctx context.Context, claim jobs.Claim) (string, error) {
	var payload webhookJob
	if err := json.Unmarshal(claim.Payload, &payload); err != nil || !uuidOK(payload.DeliveryID) {
		return "", errors.New("invalid GitHub webhook job")
	}
	var event, action, raw string
	var externalInstall *int64
	var college *string
	err := w.pool.QueryRow(ctx, `SELECT event_type,action,external_installation_id,payload::text,college_id::text FROM github_webhook_deliveries WHERE delivery_id=$1`, payload.DeliveryID).Scan(&event, &action, &externalInstall, &raw, &college)
	if err != nil {
		return "", err
	}
	var body struct {
		Installation struct {
			ID int64 `json:"id"`
		} `json:"installation"`
		Repository struct {
			ID int64 `json:"id"`
		} `json:"repository"`
		RepositoriesRemoved []struct {
			ID int64 `json:"id"`
		} `json:"repositories_removed"`
		RepositoriesAdded []struct {
			ID int64 `json:"id"`
		} `json:"repositories_added"`
	}
	if err = json.Unmarshal([]byte(raw), &body); err != nil {
		return "", err
	}
	if externalInstall != nil {
		switch event + "." + action {
		case "installation.deleted", "installation.suspend":
			_, err = w.pool.Exec(ctx, `UPDATE github_installations SET status=CASE WHEN $2='deleted' THEN 'removed' ELSE 'suspended' END,removed_at=CASE WHEN $2='deleted' THEN clock_timestamp() ELSE removed_at END WHERE external_installation_id=$1`, *externalInstall, action)
			if err != nil {
				return "", err
			}
			_, err = w.pool.Exec(ctx, `UPDATE repository_bindings SET access_state='permission_lost',sync_health='failed',last_error_code='installation_revoked' WHERE installation_id IN (SELECT id FROM github_installations WHERE external_installation_id=$1) AND is_current`, *externalInstall)
			if err != nil {
				return "", err
			}
		case "installation.unsuspend":
			_, err = w.pool.Exec(ctx, `UPDATE github_installations SET status='active',removed_at=NULL,checked_at=clock_timestamp() WHERE external_installation_id=$1`, *externalInstall)
			if err != nil {
				return "", err
			}
			_, err = w.pool.Exec(ctx, `UPDATE repository_bindings SET access_state='stale',sync_health='delayed',last_error_code=NULL WHERE installation_id IN (SELECT id FROM github_installations WHERE external_installation_id=$1) AND is_current AND access_state='permission_lost'`, *externalInstall)
			if err != nil {
				return "", err
			}
		case "installation_repositories.removed":
			for _, repo := range body.RepositoriesRemoved {
				_, err = w.pool.Exec(ctx, `UPDATE repository_bindings SET access_state='permission_lost',sync_health='failed',last_error_code='repository_removed' WHERE external_repository_id=$1 AND installation_id IN (SELECT id FROM github_installations WHERE external_installation_id=$2) AND is_current`, repo.ID, *externalInstall)
				if err != nil {
					return "", err
				}
			}
		case "installation_repositories.added":
			for _, repo := range body.RepositoriesAdded {
				_, err = w.pool.Exec(ctx, `UPDATE repository_bindings SET access_state='stale',sync_health='delayed',last_error_code=NULL WHERE external_repository_id=$1 AND installation_id IN (SELECT id FROM github_installations WHERE external_installation_id=$2) AND is_current AND access_state='permission_lost'`, repo.ID, *externalInstall)
				if err != nil {
					return "", err
				}
			}
		default:
			if event != "push" && event != "pull_request" && event != "issues" && event != "installation_repositories" {
				_, err = w.pool.Exec(ctx, `UPDATE github_webhook_deliveries SET state='unmatched',last_error_code='unsupported_event',processed_at=clock_timestamp() WHERE delivery_id=$1`, payload.DeliveryID)
				return "", err
			}
			_, err = w.pool.Exec(ctx, `UPDATE repository_bindings SET sync_health='delayed' WHERE installation_id IN (SELECT id FROM github_installations WHERE external_installation_id=$1) AND is_current AND access_state IN ('active','stale')`, *externalInstall)
			if err != nil {
				return "", err
			}
			rows, queryErr := w.pool.Query(ctx, `SELECT b.id::text,b.college_id::text,b.linked_by::text,b.external_repository_id
				FROM repository_bindings b JOIN github_installations i ON i.id=b.installation_id AND i.college_id=b.college_id
				WHERE i.external_installation_id=$1 AND b.is_current AND b.access_state IN ('active','stale')`, *externalInstall)
			if queryErr != nil {
				return "", queryErr
			}
			type target struct {
				id, tenant, actor string
				repo              int64
			}
			var targets []target
			for rows.Next() {
				var t target
				if queryErr = rows.Scan(&t.id, &t.tenant, &t.actor, &t.repo); queryErr != nil {
					rows.Close()
					return "", queryErr
				}
				if body.Repository.ID == 0 || body.Repository.ID == t.repo {
					targets = append(targets, t)
				}
			}
			queryErr = rows.Err()
			rows.Close()
			if queryErr != nil {
				return "", queryErr
			}
			for _, t := range targets {
				jobPayload, _ := json.Marshal(bindingJob{BindingID: t.id})
				if _, queryErr = (jobs.Queue{Pool: w.pool}).EnqueueWork(ctx, t.tenant, t.actor, "github_reconcile", "webhook:"+payload.DeliveryID+":"+t.id, jobPayload, nil); queryErr != nil {
					return "", queryErr
				}
			}
		}
	}
	_, err = w.pool.Exec(ctx, `UPDATE github_webhook_deliveries SET state='done',processed_at=clock_timestamp(),attempts=attempts+1 WHERE delivery_id=$1`, payload.DeliveryID)
	return "", err
}

func ScheduleReconciliation(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	rows, err := pool.Query(ctx, `SELECT b.id::text,b.college_id::text,b.linked_by::text FROM repository_bindings b JOIN github_installations i ON i.id=b.installation_id AND i.college_id=b.college_id WHERE b.is_current AND b.access_state IN ('active','stale') AND i.status='active' AND (b.last_synced_at IS NULL OR b.last_synced_at<clock_timestamp()-interval '30 minutes') LIMIT 1000`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type target struct{ id, tenant, actor string }
	var targets []target
	for rows.Next() {
		var t target
		if err = rows.Scan(&t.id, &t.tenant, &t.actor); err != nil {
			return err
		}
		targets = append(targets, t)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, target := range targets {
		key := fmt.Sprintf("scheduled:%s:%s", target.id, time.Now().UTC().Format("2006010215"))
		payload, _ := json.Marshal(bindingJob{BindingID: target.id})
		if _, err = (jobs.Queue{Pool: pool}).EnqueueWork(ctx, target.tenant, target.actor, "github_reconcile", key, payload, nil); err != nil {
			logger.ErrorContext(ctx, "repository reconciliation enqueue failed", "binding_id", target.id, "error", err)
		}
	}
	return nil
}

func login(user *github.User) string {
	if user == nil {
		return ""
	}
	return user.Login
}
func authorID(user *github.User) int64 {
	if user == nil {
		return 0
	}
	return user.ID
}
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 500 {
		s = s[:500]
	}
	return strings.TrimSpace(s)
}
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}

func visibility(repo github.Repository) string {
	if repo.Visibility != "" {
		return repo.Visibility
	}
	if repo.Private {
		return "private"
	}
	return "public"
}
