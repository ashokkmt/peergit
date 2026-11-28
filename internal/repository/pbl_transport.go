package repository

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"peergit/internal/platform/jobs"
)

const zeroOID = "0000000000000000000000000000000000000000"

type refChange struct{ old, next, ref string }

func receiveCommands(body []byte) ([]refChange, error) {
	var changes []refChange
	for len(body) >= 4 && len(changes) < 64 {
		size, err := strconv.ParseUint(string(body[:4]), 16, 16)
		if err != nil {
			break
		}
		if size == 0 && len(changes) != 0 {
			return changes, nil
		}
		if size < 4 || size > uint64(len(body)) || size > 1024 {
			break
		}
		line, _, _ := strings.Cut(string(body[4:size]), "\x00")
		fields := strings.Fields(line)
		if len(fields) != 3 || !regexpCommitSHA(fields[0]) || !regexpCommitSHA(fields[1]) ||
			len(fields[2]) > 255 || strings.HasPrefix(fields[2], "refs/tags/"+pblReceiptTag) ||
			!(strings.HasPrefix(fields[2], "refs/heads/") || strings.HasPrefix(fields[2], "refs/tags/")) {
			break
		}
		changes = append(changes, refChange{fields[0], fields[1], fields[2]})
		body = body[size:]
	}
	return nil, errors.New("invalid or oversized Git reference update")
}

func (h *Handler) pblGit(w http.ResponseWriter, r *http.Request) {
	if h.forge.URL == "" || h.pool == nil {
		http.NotFound(w, r)
		return
	}
	team, valid := strings.CutSuffix(chi.URLParam(r, "repository"), ".git")
	operation := chi.URLParam(r, "*")
	discovery := r.Method == http.MethodGet && operation == "info/refs"
	push := r.Method == http.MethodPost && operation == "git-receive-pack"
	fetch := r.Method == http.MethodPost && operation == "git-upload-pack"
	service := r.URL.Query().Get("service")
	if !valid || !uuidOK(team) || !(discovery && (service == "git-upload-pack" || service == "git-receive-pack") && len(r.URL.Query()) == 1 || (push || fetch) && r.URL.RawQuery == "") {
		http.NotFound(w, r)
		return
	}
	if (push || fetch) && (r.Header.Get("Content-Type") != "application/x-"+operation+"-request" || r.Header.Get("Content-Encoding") != "") {
		http.Error(w, "Unsupported Git request", http.StatusUnsupportedMediaType)
		return
	}
	unauthorized := func() {
		w.Header().Set("WWW-Authenticate", `Basic realm="PeerGit PBL"`)
		http.Error(w, "A current PBL Git credential is required", http.StatusUnauthorized)
	}
	username, password, ok := r.BasicAuth()
	if !ok || username != "peergit" || len(password) != 43 {
		unauthorized()
		return
	}
	select {
	case h.pblTransfers <- struct{}{}:
		defer func() { <-h.pblTransfers }()
	default:
		w.Header().Set("Retry-After", "5")
		http.Error(w, "PBL Git is busy", http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	sum := sha256.Sum256([]byte(password))
	var name, tenant, actor, binding, branch string
	err := h.pool.QueryRow(ctx, `SELECT b.repository_name,n.college_id::text,k.user_id::text,b.id::text,b.default_branch
		FROM pbl_git_credentials k JOIN pbl_active_team_members m ON m.team_id=k.team_id AND m.user_id=k.user_id AND m.college_id=k.college_id
		JOIN pbl_provisions n ON n.team_id=k.team_id AND n.state='ready'
		JOIN repositories p ON p.project_id=n.project_id AND p.college_id=n.college_id
		JOIN repository_bindings b ON b.repository_id=p.id AND b.college_id=p.college_id
		WHERE k.team_id=$1 AND k.secret_hash=$2 AND k.revoked_at IS NULL AND k.expires_at>clock_timestamp()
		AND b.provider='forgejo' AND b.forge_instance=$3 AND b.is_current AND b.access_state='active'
		AND EXISTS(SELECT 1 FROM consent_records c WHERE c.user_id=k.user_id AND c.purpose='terms' AND c.revoked_at IS NULL)
		AND EXISTS(SELECT 1 FROM consent_records c WHERE c.user_id=k.user_id AND c.purpose='privacy' AND c.revoked_at IS NULL)`,
		team, sum[:], h.forge.URL).Scan(&name, &tenant, &actor, &binding, &branch)
	if errors.Is(err, pgx.ErrNoRows) {
		unauthorized()
		return
	}
	if err != nil {
		http.Error(w, "PBL Git authorization is temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	target := h.forge.gitURL(name) + "/" + operation
	if discovery {
		target += "?service=" + service
	}
	if !push {
		var body io.Reader
		if fetch {
			body = http.MaxBytesReader(w, r.Body, 1<<20)
		}
		res, err := h.pblGitRequest(ctx, r, target, body)
		if err != nil {
			http.Error(w, "PBL Git is temporarily unavailable", http.StatusBadGateway)
			return
		}
		defer res.Body.Close()
		writeGitResponse(w, res, res.Body)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, pblMaxPushBytes))
	if err != nil {
		http.Error(w, "Git push exceeds the pilot transfer limit", http.StatusRequestEntityTooLarge)
		return
	}
	changes, err := receiveCommands(body)
	if err != nil {
		http.Error(w, "Git pushes must update branch or tag refs", http.StatusBadRequest)
		return
	}
	h.forwardPBLPush(w, r, ctx, team, tenant, actor, binding, name, branch, target, sum[:], changes, body)
}

func (h *Handler) pblGitRequest(ctx context.Context, upstream *http.Request, target string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, upstream.Method, target, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(h.forge.Owner, h.forge.Token)
	for _, key := range []string{"Content-Type", "Git-Protocol", "Accept"} {
		if value := upstream.Header.Get(key); value != "" {
			req.Header.Set(key, value)
		}
	}
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return client.Do(req)
}

func writeGitResponse(w http.ResponseWriter, res *http.Response, body io.Reader) {
	if res.StatusCode != http.StatusOK {
		http.Error(w, "Forgejo could not complete the Git request", res.StatusCode)
		return
	}
	w.Header().Set("Content-Type", res.Header.Get("Content-Type"))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

func (h *Handler) forwardPBLPush(w http.ResponseWriter, r *http.Request, ctx context.Context,
	team, tenant, actor, binding, name, branch, target string, digest []byte, changes []refChange, body []byte) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		http.Error(w, "PBL Git is temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	defer tx.Rollback(ctx)
	var allowed bool
	err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,61)) AND EXISTS(
		SELECT 1 FROM pbl_git_credentials k JOIN pbl_active_team_members m ON m.team_id=k.team_id AND m.user_id=k.user_id AND m.college_id=k.college_id
		WHERE k.team_id=$2 AND k.secret_hash=$3 AND k.revoked_at IS NULL AND k.expires_at>clock_timestamp())`,
		team, team, digest).Scan(&allowed)
	if err != nil || !allowed {
		http.Error(w, "The repository is busy or your access changed", http.StatusConflict)
		return
	}
	var details forgeRepository
	if err = h.forge.api(ctx, http.MethodGet, h.forge.repositoryPath(name), nil, &details); err != nil || !details.Private {
		http.Error(w, "The private repository could not be verified", http.StatusServiceUnavailable)
		return
	}
	if details.Size >= pblRepositoryKiB || details.Size+int64(len(body)+1023)/1024 > pblRepositoryKiB {
		http.Error(w, "The repository would exceed the pilot quota", http.StatusRequestEntityTooLarge)
		return
	}
	before, err := gitRefs(ctx, h.forge.gitURL(name), h.forge.Owner, h.forge.Token)
	if err != nil {
		http.Error(w, "The repository refs could not be verified", http.StatusBadGateway)
		return
	}
	defaultRef := "refs/heads/" + branch
	firstPush := len(before) == 0
	for _, change := range changes {
		if (change.ref == defaultRef && !firstPush) || firstPush && (len(changes) != 1 || change.ref != defaultRef || change.next == zeroOID) {
			http.Error(w, "Initialize the default branch once, then use a pull request to update it", http.StatusForbidden)
			return
		}
	}
	res, err := h.pblGitRequest(ctx, r, target, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "PBL Git is temporarily unavailable", http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		writeGitResponse(w, res, res.Body)
		return
	}
	report, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(report) > 1<<20 {
		http.Error(w, "The Git response could not be verified", http.StatusBadGateway)
		return
	}
	after, err := gitRefs(ctx, h.forge.gitURL(name), h.forge.Owner, h.forge.Token)
	if err != nil {
		http.Error(w, "The push result could not be verified", http.StatusBadGateway)
		return
	}
	if firstPush && after[defaultRef] != "" {
		if err = h.forge.protectBranch(ctx, name, branch); err != nil {
			http.Error(w, "Default branch protection could not be verified", http.StatusBadGateway)
			return
		}
	}
	for _, change := range changes {
		old := before[change.ref]
		if old == "" {
			old = zeroOID
		}
		if change.next == zeroOID || old != change.old || old == change.next || after[change.ref] != change.next {
			continue
		}
		if _, err = tx.Exec(ctx, `INSERT INTO pbl_git_pushes(college_id,team_id,user_id,ref,oid) VALUES($1,$2,$3,$4,$5)`,
			tenant, team, actor, change.ref, change.next); err != nil {
			break
		}
	}
	if err == nil {
		job, _ := json.Marshal(map[string]string{"binding_id": binding})
		id, _, idErr := randomState()
		if idErr != nil {
			err = idErr
		} else {
			_, err = jobs.EnqueueTx(ctx, tx, tenant, actor, "pbl_reconcile", "push:"+id, job, nil)
		}
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		http.Error(w, "The Git update could not be recorded", http.StatusBadGateway)
		return
	}
	writeGitResponse(w, res, bytes.NewReader(report))
}

func (h *Handler) pblWebhook(w http.ResponseWriter, r *http.Request) {
	if h.forge.URL == "" || h.pool == nil {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	signature, sigErr := hex.DecodeString(r.Header.Get("X-Forgejo-Signature"))
	mac := hmac.New(sha256.New, []byte(h.forge.WebhookSecret))
	_, _ = mac.Write(body)
	if err != nil || sigErr != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		http.Error(w, "Invalid Forgejo signature", http.StatusUnauthorized)
		return
	}
	delivery := r.Header.Get("X-Forgejo-Delivery")
	var event struct {
		Repository struct {
			ID int64 `json:"id"`
		} `json:"repository"`
	}
	if !uuidOK(delivery) || json.Unmarshal(body, &event) != nil || event.Repository.ID <= 0 {
		http.Error(w, "Invalid Forgejo delivery", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var binding, tenant, actor string
	err = tx.QueryRow(ctx, `INSERT INTO pbl_forge_deliveries(delivery_id,binding_id)
		SELECT $1,b.id FROM repository_bindings b WHERE b.provider='forgejo' AND b.forge_instance=$2 AND b.external_repository_id=$3
		AND b.is_current AND b.access_state='active' ON CONFLICT DO NOTHING RETURNING binding_id::text`, delivery, h.forge.URL, event.Repository.ID).Scan(&binding)
	if errors.Is(err, pgx.ErrNoRows) {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT college_id::text,linked_by::text FROM repository_bindings WHERE id=$1`, binding).Scan(&tenant, &actor)
	}
	if err == nil {
		job, _ := json.Marshal(map[string]string{"binding_id": binding})
		_, err = jobs.EnqueueTx(ctx, tx, tenant, actor, "pbl_reconcile", "webhook:"+delivery, job, nil)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
