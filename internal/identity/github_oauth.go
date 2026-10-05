package identity

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
	"peergit/internal/platform/errormanager"
)

type githubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func (h *Handler) githubOAuthConfig() oauth2.Config {
	authorize, token := h.cfg.GitHubAuthorizeURL, h.cfg.GitHubTokenURL
	if authorize == "" {
		authorize = "https://github.com/login/oauth/authorize"
	}
	if token == "" {
		token = "https://github.com/login/oauth/access_token"
	}
	return oauth2.Config{ClientID: h.cfg.GitHubClientID, ClientSecret: h.cfg.GitHubClientSecret, RedirectURL: h.cfg.GitHubRedirectURL,
		Endpoint: oauth2.Endpoint{AuthURL: authorize, TokenURL: token}, Scopes: []string{"read:user", "user:email"}}
}

func (h *Handler) githubAPIURL(path string) string {
	base := strings.TrimRight(h.cfg.GitHubAPIURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	return base + path
}

func (h *Handler) startGitHubLogin(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil || h.cfg.GitHubClientID == "" || h.cfg.GitHubClientSecret == "" || h.cfg.GitHubRedirectURL == "" {
		h.fail(w, r, http.StatusServiceUnavailable, "identity_unavailable", "GitHub sign-in is not configured", nil)
		return
	}
	if !h.rateLimit(w, r, "github-start", 20) {
		return
	}
	state, err := randomToken(32)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	verifier, err := randomToken(32)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var invitationHash any
	if invite := r.URL.Query().Get("invite"); invite != "" {
		if len(invite) > 128 {
			h.fail(w, r, http.StatusBadRequest, "invalid_invitation", "invitation is invalid", nil)
			return
		}
		hash := sha256.Sum256([]byte(invite))
		invitationHash = hash[:]
	}
	stateHash := keyedHash([]byte(h.cfg.SessionHashKey), []byte(state))
	_, err = h.pool.Exec(r.Context(), `DELETE FROM github_login_states WHERE expires_at<=now()`)
	if err == nil {
		_, err = h.pool.Exec(r.Context(), `INSERT INTO github_login_states(state_hash,pkce_verifier,invitation_token_hash,expires_at) VALUES($1,$2,$3,now()+interval '10 minutes')`, stateHash, verifier, invitationHash)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	cookieName := githubStateCookie
	if !h.cfg.CookieSecure {
		cookieName = "peergit_github_state"
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: state, Path: "/", MaxAge: 600, HttpOnly: true, Secure: h.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
	config := h.githubOAuthConfig()
	challenge := oauth2.S256ChallengeFromVerifier(verifier)
	http.Redirect(w, r, config.AuthCodeURL(state, oauth2.SetAuthURLParam("code_challenge", challenge), oauth2.SetAuthURLParam("code_challenge_method", "S256")), http.StatusFound)
}

func (h *Handler) githubCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	cookieName := githubStateCookie
	if !h.cfg.CookieSecure {
		cookieName = "peergit_github_state"
	}
	cookie, cookieErr := r.Cookie(cookieName)
	if state == "" || cookieErr != nil || !hmacEqual([]byte(state), []byte(cookie.Value)) {
		h.fail(w, r, http.StatusUnauthorized, "sign_in_state_invalid", "sign-in could not be verified; start again", nil)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1, HttpOnly: true, Secure: h.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
	var verifier string
	var invitationHash []byte
	err := h.pool.QueryRow(r.Context(), `DELETE FROM github_login_states WHERE state_hash=$1 AND expires_at>now() RETURNING pkce_verifier,COALESCE(invitation_token_hash,decode('','hex'))`, keyedHash([]byte(h.cfg.SessionHashKey), []byte(state))).Scan(&verifier, &invitationHash)
	if err != nil {
		h.fail(w, r, http.StatusUnauthorized, "sign_in_state_expired", "sign-in expired; start again", err)
		return
	}
	if r.URL.Query().Get("error") != "" {
		h.fail(w, r, http.StatusUnauthorized, "sign_in_cancelled", "GitHub authorization was not completed; you can try again", nil)
		return
	}
	oauthConfig := h.githubOAuthConfig()
	token, err := oauthConfig.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
	if err != nil {
		h.fail(w, r, http.StatusUnauthorized, "sign_in_failed", "GitHub sign-in could not be completed", err)
		return
	}
	client := oauthConfig.Client(r.Context(), token)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var profile githubUser
	if err = githubGet(ctx, client, h.githubAPIURL("/user"), &profile); err != nil || profile.ID <= 0 {
		h.fail(w, r, http.StatusBadGateway, "github_identity_unavailable", "GitHub account could not be verified", err)
		return
	}
	var emails []githubEmail
	if err = githubGet(ctx, client, h.githubAPIURL("/user/emails"), &emails); err != nil {
		h.fail(w, r, http.StatusBadGateway, "github_email_unavailable", "GitHub email could not be verified", err)
		return
	}
	contact := ""
	for _, email := range emails {
		if email.Primary && email.Verified {
			contact = strings.ToLower(strings.TrimSpace(email.Email))
			break
		}
	}
	if !validIdentityEmail(contact) {
		h.fail(w, r, http.StatusForbidden, "verified_email_required", "a verified primary GitHub email is required", nil)
		return
	}
	if !h.rateLimit(w, r, "github-callback:"+contact, 10) {
		return
	}
	userID, err := h.upsertGitHubUser(r.Context(), profile, contact)
	if err != nil {
		var apiErr *errormanager.Error
		if errors.As(err, &apiErr) {
			h.err.Handle(w, r, err)
		} else {
			h.err.Handle(w, r, errormanager.New(http.StatusInternalServerError, "identity_error", "sign-in could not be completed", err))
		}
		return
	}
	if len(invitationHash) == sha256.Size {
		if err = h.acceptExternalInvitation(r.Context(), invitationHash, contact, userID); err != nil {
			h.err.Handle(w, r, err)
			return
		}
	}
	session, e := randomToken(32)
	if e != nil {
		h.err.Handle(w, r, e)
		return
	}
	csrf := sessionCSRFToken([]byte(h.cfg.SessionHashKey), session)
	expires := time.Now().UTC().Add(12 * time.Hour)
	var collegeID string
	err = h.pool.QueryRow(r.Context(), `SELECT COALESCE((
		SELECT c.id::text FROM users u JOIN colleges c ON c.id=u.college_id AND c.status='active'
		WHERE u.id=$1 AND u.account_type='campus' AND EXISTS(
			SELECT 1 FROM campus_verifications v WHERE v.college_id=c.id AND v.user_id=u.id AND v.revoked_at IS NULL AND
			(v.source='administrator_review' OR EXISTS(
				SELECT 1 FROM campus_emails ce JOIN college_domains d ON d.college_id=ce.college_id AND d.domain=split_part(ce.email_normalized,'@',2) AND d.verified_at IS NOT NULL
				WHERE ce.id=v.campus_email_id AND ce.verified_at IS NOT NULL
			))
		)),'' )`, userID).Scan(&collegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_, err = h.pool.Exec(r.Context(), `INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,expires_at) VALUES($1,NULLIF($2,'')::uuid,$3,$4,$5)`, userID, collegeID, keyedHash([]byte(h.cfg.SessionHashKey), []byte(session)), keyedHash([]byte(h.cfg.SessionHashKey), []byte(csrf)), expires)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	h.setCookie(w, session, expires)
	if collegeID != "" {
		var complete bool
		if err = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM consent_records WHERE user_id=$1 AND purpose='terms' AND policy_version=$2 AND revoked_at IS NULL) AND EXISTS(SELECT 1 FROM consent_records WHERE user_id=$1 AND purpose='privacy' AND policy_version=$3 AND revoked_at IS NULL)`, userID, termsPolicyVersion, privacyPolicyVersion).Scan(&complete); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if complete {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}

func (h *Handler) acceptExternalInvitation(ctx context.Context, tokenHash []byte, email, userID string) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var collegeID, kind, accountType string
	err = tx.QueryRow(ctx, `SELECT college_id::text,COALESCE(external_access_kind,''),account_type FROM invitations WHERE token_hash=$1 AND email_normalized=$2 AND expires_at>now() AND accepted_at IS NULL AND revoked_at IS NULL FOR UPDATE`, tokenHash, email).Scan(&collegeID, &kind, &accountType)
	if errors.Is(err, pgx.ErrNoRows) {
		return errormanager.New(http.StatusForbidden, "invitation_invalid", "this external invitation is invalid, expired, or belongs to another verified email", nil)
	}
	if err != nil {
		return err
	}
	if accountType == "campus" {
		return nil
	}
	if accountType != "external" {
		return errormanager.New(http.StatusForbidden, "invitation_invalid", "this invitation cannot be accepted", nil)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO external_college_access(college_id,user_id,access_kind) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, collegeID, userID, kind); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE invitations SET accepted_by=$2,accepted_at=now() WHERE token_hash=$1`, tokenHash, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func githubGet(ctx context.Context, client *http.Client, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, (1<<20)+1)).Decode(out)
}

func validIdentityEmail(email string) bool {
	p := strings.Split(email, "@")
	return len(p) == 2 && p[0] != "" && p[1] != "" && !strings.ContainsAny(email, " \r\n")
}

func (h *Handler) upsertGitHubUser(ctx context.Context, profile githubUser, email string) (string, error) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	subject := strconv.FormatInt(profile.ID, 10)
	var id, status string
	err = tx.QueryRow(ctx, `SELECT u.id::text,u.status FROM user_identities i JOIN users u ON u.id=i.user_id WHERE i.provider='github' AND i.subject=$1 FOR UPDATE`, subject).Scan(&id, &status)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		var existing string
		e := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE email_normalized=$1`, email).Scan(&existing)
		if e == nil {
			return "", errormanager.New(http.StatusConflict, "email_already_registered", "this email belongs to a different sign-in identity; contact support", nil)
		}
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return "", e
		}
		id, err = insertUser(ctx, tx, email, profile.Name, "unverified", "")
		if err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO user_identities(user_id,provider,subject) VALUES($1,'github',$2)`, id, subject); err != nil {
			return "", err
		}
	} else if status != "active" {
		return "", errormanager.New(http.StatusForbidden, "account_unavailable", "this account cannot sign in", nil)
	} else {
		var conflict bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email_normalized=$1 AND id<>$2)`, email, id).Scan(&conflict); err != nil {
			return "", err
		}
		if conflict {
			return "", errormanager.New(http.StatusConflict, "email_already_registered", "this email belongs to a different account", nil)
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET email=$2,email_normalized=$2,display_name=CASE WHEN display_name='' THEN $3 ELSE display_name END,updated_at=now() WHERE id=$1`, id, email, cleanName(profile.Name)); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}
