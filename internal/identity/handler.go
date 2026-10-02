package identity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
	"peergit/internal/platform/errormanager"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
)

const sessionCookie = "__Host-peergit_session"
const oidcStateCookie = "__Host-peergit_oidc_state"
const termsPolicyVersion = "draft-1"
const privacyPolicyVersion = "draft-1"

type Config struct {
	Issuer, ClientID, ClientSecret, RedirectURL, AppOrigin string
	SessionHashKey, MFAEncryptionKey                       string
	CookieSecure                                           bool
}

type Handler struct {
	pool       *pgxpool.Pool
	cfg        Config
	log        *slog.Logger
	err        *errormanager.Manager
	providerMu sync.Mutex
	provider   *oidc.Provider
}

type Session struct {
	ID, UserID, CollegeID, Email, DisplayName, AccountType string
	CSRFHash                                               []byte
	MFAEnabled, CampusAdmin                                bool
	MFAFresh                                               bool
	TermsAccepted, PrivacyAccepted                         bool
}

type sessionKey struct{}

func NewHandler(pool *pgxpool.Pool, cfg Config, logger *slog.Logger, errors *errormanager.Manager) *Handler {
	return &Handler{pool: pool, cfg: cfg, log: logger, err: errors}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/auth/google", h.startLogin)
	r.Get("/auth/callback", h.callback)
	r.Group(func(private chi.Router) {
		private.Use(h.loadSession)
		private.Get("/session", h.session)
		private.Get("/me", h.me)
		private.Post("/auth/logout", h.csrf(h.logout))
		private.Patch("/me/profile", h.csrf(h.updateProfile))
		private.Put("/me/consent", h.csrf(h.updateConsent))
		private.Post("/auth/mfa/enroll", h.csrf(h.enrollMFA))
		private.Post("/auth/mfa/confirm", h.csrf(h.confirmMFA))
		private.Post("/auth/mfa/verify", h.csrf(h.verifyMFA))
	})
}

func (h *Handler) Middleware(next http.Handler) http.Handler { return h.loadSession(next) }

func (h *Handler) RequireCSRF(next http.Handler) http.Handler {
	return h.csrf(func(w http.ResponseWriter, r *http.Request) { next.ServeHTTP(w, r) })
}

func (h *Handler) getProvider(ctx context.Context) (*oidc.Provider, error) {
	h.providerMu.Lock()
	defer h.providerMu.Unlock()
	if h.provider != nil {
		return h.provider, nil
	}
	provider, err := oidc.NewProvider(ctx, h.cfg.Issuer)
	if err != nil {
		return nil, err
	}
	h.provider = provider
	return provider, nil
}

func CurrentSession(ctx context.Context) (Session, bool) {
	s, ok := ctx.Value(sessionKey{}).(Session)
	return s, ok
}

func (h *Handler) loadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(h.cookieName())
		if err == nil && cookie.Value != "" && h.pool != nil && len(h.cfg.SessionHashKey) >= 32 {
			tokenHash := keyedHash([]byte(h.cfg.SessionHashKey), []byte(cookie.Value))
			var s Session
			var adminCount int
			var mfaCount int
			var fresh bool
			var termsAccepted, privacyAccepted bool
			err = h.pool.QueryRow(r.Context(), `SELECT s.id::text,u.id::text,COALESCE(s.college_id::text,''),u.email_normalized,u.display_name,u.account_type,
				s.csrf_hash,
				(SELECT count(*) FROM campus_roles cr WHERE cr.college_id=s.college_id AND cr.user_id=u.id AND cr.role='campus_admin'),
				(SELECT count(*) FROM mfa_credentials mc WHERE mc.user_id=u.id AND mc.enabled_at IS NOT NULL),
			COALESCE(s.mfa_verified_at > now()-interval '10 minutes',false),
				EXISTS(SELECT 1 FROM consent_records co WHERE co.user_id=u.id AND co.purpose='terms' AND co.policy_version=$2 AND co.revoked_at IS NULL),
				EXISTS(SELECT 1 FROM consent_records co WHERE co.user_id=u.id AND co.purpose='privacy' AND co.policy_version=$3 AND co.revoked_at IS NULL)
				FROM sessions s JOIN users u ON u.id=s.user_id
				LEFT JOIN colleges c ON c.id=s.college_id
				WHERE s.token_hash=$1 AND s.expires_at>now() AND u.status='active'
				AND (s.college_id IS NULL OR c.status='active')
				AND (u.account_type<>'campus' OR EXISTS(
					SELECT 1 FROM campus_verifications v WHERE v.college_id=u.college_id AND v.user_id=u.id AND v.revoked_at IS NULL
					AND (v.source<>'verified_domain' OR EXISTS(SELECT 1 FROM college_domains d WHERE d.college_id=u.college_id
						AND d.verified_at IS NOT NULL AND d.domain=split_part(u.email_normalized,'@',2)))))`, tokenHash, termsPolicyVersion, privacyPolicyVersion).Scan(&s.ID, &s.UserID, &s.CollegeID, &s.Email, &s.DisplayName, &s.AccountType, &s.CSRFHash, &adminCount, &mfaCount, &fresh, &termsAccepted, &privacyAccepted)
			if err == nil {
				s.CampusAdmin, s.MFAEnabled, s.MFAFresh = adminCount > 0, mfaCount > 0, fresh
				s.TermsAccepted, s.PrivacyAccepted = termsAccepted, privacyAccepted
				r = r.WithContext(context.WithValue(r.Context(), sessionKey{}, s))
				_, _ = h.pool.Exec(r.Context(), `UPDATE sessions SET last_seen_at=now() WHERE id=$1 AND last_seen_at < now()-interval '5 minutes'`, s.ID)
			} else if !errors.Is(err, pgx.ErrNoRows) {
				h.err.Handle(w, r, err)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) csrf(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, ok := CurrentSession(r.Context())
		if !ok {
			h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
			return
		}
		origin, err := url.Parse(r.Header.Get("Origin"))
		appOrigin, appErr := url.Parse(h.cfg.AppOrigin)
		if err != nil || appErr != nil || origin.Scheme == "" || origin.Host == "" || origin.Scheme+"://"+origin.Host != appOrigin.Scheme+"://"+appOrigin.Host {
			h.fail(w, r, http.StatusForbidden, "origin_rejected", "request origin is not allowed", nil)
			return
		}
		csrf := r.Header.Get("X-CSRF-Token")
		if len(csrf) < 32 || !hmacEqual(s.CSRFHash, keyedHash([]byte(h.cfg.SessionHashKey), []byte(csrf))) {
			h.fail(w, r, http.StatusForbidden, "csrf_rejected", "request could not be verified", nil)
			return
		}
		next(w, r)
	}
}

func (h *Handler) startLogin(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil || h.cfg.Issuer == "" || h.cfg.ClientID == "" || h.cfg.ClientSecret == "" || h.cfg.RedirectURL == "" {
		h.fail(w, r, http.StatusServiceUnavailable, "identity_unavailable", "Google sign-in is not configured", nil)
		return
	}
	if !h.rateLimit(w, r, "oidc-start", 20) {
		return
	}
	provider, err := h.getProvider(r.Context())
	if err != nil {
		h.fail(w, r, http.StatusBadGateway, "identity_provider_unavailable", "sign-in provider is temporarily unavailable", err)
		return
	}
	state, err := randomToken(32)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	nonce, err := randomToken(32)
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
	_, err = h.pool.Exec(r.Context(), `DELETE FROM oidc_login_states WHERE expires_at<=now()`)
	if err == nil {
		_, err = h.pool.Exec(r.Context(), `INSERT INTO oidc_login_states(state_hash,nonce,pkce_verifier,invitation_token_hash,expires_at)
			VALUES($1,$2,$3,$4,now()+interval '10 minutes')`, stateHash, nonce, verifier, invitationHash)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	config := oauth2.Config{ClientID: h.cfg.ClientID, ClientSecret: h.cfg.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: h.cfg.RedirectURL, Scopes: []string{oidc.ScopeOpenID, "email", "profile"}}
	stateCookieName := oidcStateCookie
	if !h.cfg.CookieSecure {
		stateCookieName = "peergit_oidc_state"
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookieName, Value: state, Path: "/", MaxAge: 600, HttpOnly: true, Secure: h.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, config.AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	stateCookieName := oidcStateCookie
	if !h.cfg.CookieSecure {
		stateCookieName = "peergit_oidc_state"
	}
	stateCookie, cookieErr := r.Cookie(stateCookieName)
	if state == "" || cookieErr != nil || !hmacEqual([]byte(state), []byte(stateCookie.Value)) {
		h.fail(w, r, http.StatusUnauthorized, "sign_in_state_invalid", "sign-in could not be verified; start again", nil)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: h.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
	provider, err := h.getProvider(r.Context())
	if err != nil {
		h.fail(w, r, http.StatusBadGateway, "identity_provider_unavailable", "sign-in provider is temporarily unavailable", err)
		return
	}
	if state == "" || r.URL.Query().Get("error") != "" {
		h.fail(w, r, http.StatusUnauthorized, "sign_in_failed", "sign-in could not be completed", nil)
		return
	}
	stateHash := keyedHash([]byte(h.cfg.SessionHashKey), []byte(state))
	var nonce, verifier string
	var inviteHash []byte
	err = h.pool.QueryRow(r.Context(), `DELETE FROM oidc_login_states WHERE state_hash=$1 AND expires_at>now()
		RETURNING nonce,pkce_verifier,COALESCE(invitation_token_hash,decode('','hex'))`, stateHash).Scan(&nonce, &verifier, &inviteHash)
	if err != nil {
		h.fail(w, r, http.StatusUnauthorized, "sign_in_state_expired", "sign-in expired; start again", err)
		return
	}
	config := oauth2.Config{ClientID: h.cfg.ClientID, ClientSecret: h.cfg.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: h.cfg.RedirectURL}
	token, err := config.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
	if err != nil {
		h.fail(w, r, http.StatusUnauthorized, "sign_in_failed", "sign-in could not be completed", err)
		return
	}
	idToken, ok := token.Extra("id_token").(string)
	if !ok || idToken == "" {
		h.fail(w, r, http.StatusUnauthorized, "sign_in_failed", "sign-in could not be completed", nil)
		return
	}
	verified, err := provider.Verifier(&oidc.Config{ClientID: h.cfg.ClientID}).Verify(r.Context(), idToken)
	if err != nil {
		h.fail(w, r, http.StatusUnauthorized, "sign_in_failed", "sign-in could not be completed", err)
		return
	}
	var claims struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		Name          string `json:"name"`
		EmailVerified bool   `json:"email_verified"`
		Nonce         string `json:"nonce"`
	}
	if err := verified.Claims(&claims); err != nil || !claims.EmailVerified || claims.Subject == "" || !hmacEqual([]byte(nonce), []byte(claims.Nonce)) {
		h.fail(w, r, http.StatusUnauthorized, "identity_unverified", "a verified Google email is required", err)
		return
	}
	if !h.rateLimit(w, r, "oidc-callback:"+strings.ToLower(claims.Email), 10) {
		return
	}
	userID, collegeID, err := h.resolveIdentity(r.Context(), claims, inviteHash)
	if err != nil {
		var apiErr *errormanager.Error
		if errors.As(err, &apiErr) {
			h.err.Handle(w, r, err)
		} else {
			h.err.Handle(w, r, errormanager.New(http.StatusInternalServerError, "identity_error", "sign-in could not be completed", err))
		}
		return
	}
	csrf, err := randomToken(32)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	sessionToken, err := randomToken(32)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	expires := time.Now().UTC().Add(12 * time.Hour)
	var admin bool
	_ = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM campus_roles WHERE user_id=$1 AND role='campus_admin')`, userID).Scan(&admin)
	if admin {
		expires = time.Now().UTC().Add(4 * time.Hour)
	}
	_, err = h.pool.Exec(r.Context(), `INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,expires_at)
		VALUES($1,NULLIF($2,'')::uuid,$3,$4,$5)`, userID, collegeID, keyedHash([]byte(h.cfg.SessionHashKey), []byte(sessionToken)), keyedHash([]byte(h.cfg.SessionHashKey), []byte(csrf)), expires)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	h.setCookie(w, sessionToken, expires)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) resolveIdentity(ctx context.Context, claims struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	EmailVerified bool   `json:"email_verified"`
	Nonce         string `json:"nonce"`
}, inviteHash []byte) (string, string, error) {
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errormanager.New(http.StatusUnauthorized, "identity_unverified", "a verified campus email is required", nil)
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)
	var collegeID, accountType, role, externalKind, orgID string
	var inviteFound bool
	if len(inviteHash) == sha256.Size {
		err = tx.QueryRow(ctx, `SELECT college_id::text,account_type,COALESCE(role,''),COALESCE(external_access_kind,''),COALESCE(organization_id::text,'')
			FROM invitations WHERE token_hash=$1 AND email_normalized=$2 AND expires_at>now() AND accepted_at IS NULL AND revoked_at IS NULL FOR UPDATE`, inviteHash, email).Scan(&collegeID, &accountType, &role, &externalKind, &orgID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", "", err
		}
		inviteFound = err == nil
		if !inviteFound {
			return "", "", errormanager.New(http.StatusForbidden, "invitation_invalid", "this invitation is invalid or expired", nil)
		}
	}
	var domainCollege string
	domainErr := tx.QueryRow(ctx, `SELECT c.id::text FROM college_domains d JOIN colleges c ON c.id=d.college_id
		WHERE d.domain=$1 AND d.verified_at IS NOT NULL AND c.status='active'`, parts[1]).Scan(&domainCollege)
	if domainErr != nil && !errors.Is(domainErr, pgx.ErrNoRows) {
		return "", "", domainErr
	}
	hasDomain := domainErr == nil
	knownExternal := false
	if !hasDomain && !inviteFound {
		var status, existingType string
		lookupErr := tx.QueryRow(ctx, `SELECT u.status,u.account_type FROM user_identities i JOIN users u ON u.id=i.user_id WHERE i.provider='google' AND i.subject=$1`, claims.Subject).Scan(&status, &existingType)
		if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
			return "", "", lookupErr
		}
		knownExternal = lookupErr == nil && status == "active" && existingType == "external"
	}
	if !hasDomain && !inviteFound && !knownExternal {
		return "", "", errormanager.New(http.StatusForbidden, "campus_not_verified", "your email domain is not enabled for PeerGit; an invitation is required", nil)
	}
	if hasDomain && collegeID != "" && domainCollege != collegeID && accountType == "campus" {
		return "", "", errormanager.New(http.StatusForbidden, "invitation_campus_mismatch", "invitation belongs to another campus", nil)
	}
	if knownExternal {
		accountType = "external"
	}
	if hasDomain && collegeID == "" {
		collegeID, accountType, role = domainCollege, "campus", "student"
	}
	invitationCollege := collegeID
	if !hasDomain && accountType == "campus" { /* one-time administrator invitation verifies the exception domain */
	}
	if accountType == "external" {
		collegeID = ""
	}
	var userID, existingCollege, status string
	identityErr := tx.QueryRow(ctx, `SELECT u.id::text,COALESCE(u.college_id::text,''),u.status FROM user_identities i JOIN users u ON u.id=i.user_id WHERE i.provider='google' AND i.subject=$1`, claims.Subject).Scan(&userID, &existingCollege, &status)
	if identityErr != nil && !errors.Is(identityErr, pgx.ErrNoRows) {
		return "", "", identityErr
	}
	if errors.Is(identityErr, pgx.ErrNoRows) {
		if err := tx.QueryRow(ctx, `SELECT id::text,COALESCE(college_id::text,''),status FROM users WHERE email_normalized=$1`, email).Scan(&userID, &existingCollege, &status); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", "", err
		}
		if status == "suspended" || status == "deleted" || status == "deletion_pending" {
			return "", "", errormanager.New(http.StatusForbidden, "account_unavailable", "this account cannot sign in", nil)
		}
		if userID == "" {
			userID, err = insertUser(ctx, tx, email, claims.Name, accountType, collegeID)
			if err != nil {
				return "", "", err
			}
		} else if existingCollege != collegeID {
			return "", "", errormanager.New(http.StatusForbidden, "account_campus_mismatch", "this account is already associated with another campus", nil)
		} else if _, err = tx.Exec(ctx, `UPDATE users SET email_verified_at=now(),display_name=CASE WHEN display_name='' THEN $2 ELSE display_name END WHERE id=$1`, userID, cleanName(claims.Name)); err != nil {
			return "", "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO user_identities(user_id,provider,subject) VALUES($1,'google',$2)`, userID, claims.Subject); err != nil {
			return "", "", err
		}
	} else if status != "active" || existingCollege != collegeID {
		return "", "", errormanager.New(http.StatusForbidden, "account_unavailable", "this account cannot sign in", nil)
	}
	if collegeID != "" && accountType == "campus" {
		source := "administrator_invitation"
		if hasDomain {
			source = "verified_domain"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO campus_verifications(college_id,user_id,source) VALUES($1,$2,$3)
			ON CONFLICT(college_id,user_id) DO UPDATE SET source=EXCLUDED.source,revoked_at=NULL`, collegeID, userID, source); err != nil {
			return "", "", err
		}
		if role == "" {
			role = "student"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO campus_roles(college_id,user_id,role) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, collegeID, userID, role); err != nil {
			return "", "", err
		}
	}
	if inviteFound {
		if accountType == "external" {
			if _, err := tx.Exec(ctx, `INSERT INTO external_college_access(college_id,user_id,access_kind) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, invitationCollege, userID, externalKind); err != nil {
				return "", "", err
			}
		} else if orgID != "" {
			if _, err := tx.Exec(ctx, `INSERT INTO organization_members(college_id,organization_id,user_id,role) VALUES($1,$2,$3,'member') ON CONFLICT DO NOTHING`, collegeID, orgID, userID); err != nil {
				return "", "", err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE invitations SET accepted_by=$2,accepted_at=now() WHERE token_hash=$1`, inviteHash, userID); err != nil {
			return "", "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return userID, collegeID, nil
}

func insertUser(ctx context.Context, tx pgx.Tx, email, name, accountType, collegeID string) (string, error) {
	base := strings.Split(email, "@")[0]
	var handle strings.Builder
	for _, c := range strings.ToLower(base) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' {
			handle.WriteRune(c)
		} else if c == '-' || c == '.' {
			handle.WriteRune('_')
		}
	}
	h := strings.Trim(handle.String(), "_")
	if len(h) < 3 {
		h += "_pg"
	}
	if len(h) > 24 {
		h = h[:24]
	}
	for attempt := 0; attempt < 5; attempt++ {
		candidate := h
		if attempt > 0 {
			suffix, _ := randomToken(3)
			candidate = h + "_" + strings.ToLower(strings.TrimRight(suffix, "_-"))
			if len(candidate) > 30 {
				candidate = candidate[:30]
			}
		}
		var id string
		err := tx.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type,email_verified_at)
			VALUES(NULLIF($1,'')::uuid,$2,$2,$3,$4,$5,now()) RETURNING id::text`, collegeID, email, cleanName(name), candidate, accountType).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			return "", err
		}
	}
	return "", fmt.Errorf("could not allocate a unique user handle")
}

func cleanName(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > 120 {
		name = name[:120]
	}
	if name == "" {
		return "PeerGit member"
	}
	return name
}

func skillSlug(name string) string {
	var base strings.Builder
	separator := false
	for _, c := range name {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			base.WriteRune(c)
			separator = false
		} else if base.Len() > 0 && !separator {
			base.WriteByte('-')
			separator = true
		}
	}
	value := strings.Trim(base.String(), "-")
	if value == "" {
		value = "skill"
	}
	digest := sha256.Sum256([]byte(name))
	return value + "-" + fmt.Sprintf("%x", digest[:3])
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	s, ok := CurrentSession(r.Context())
	if !ok {
		_ = response.OK(w, map[string]any{"authenticated": false})
		return
	}
	csrf, err := randomToken(32)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	// The raw token is returned once; only its keyed hash is persisted for validation.
	hash := keyedHash([]byte(h.cfg.SessionHashKey), []byte(csrf))
	if _, err := h.pool.Exec(r.Context(), `UPDATE sessions SET csrf_hash=$2 WHERE id=$1`, s.ID, hash); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"authenticated": true, "csrf_token": csrf, "user": map[string]any{"id": s.UserID, "email": s.Email, "display_name": s.DisplayName, "account_type": s.AccountType, "college_id": s.CollegeID, "campus_admin": s.CampusAdmin, "mfa_enabled": s.MFAEnabled, "mfa_verified": s.MFAFresh, "terms_accepted": s.TermsAccepted, "privacy_accepted": s.PrivacyAccepted}})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	s, ok := CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
		return
	}
	var profile json.RawMessage
	var skills []string
	consents := []string{}
	if err := h.pool.QueryRow(r.Context(), `SELECT profile FROM users WHERE id=$1`, s.UserID).Scan(&profile); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT sk.name FROM user_skills us JOIN skills sk ON sk.id=us.skill_id WHERE us.user_id=$1 ORDER BY sk.name`, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		skills = append(skills, name)
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	rows, err = h.pool.Query(r.Context(), `SELECT DISTINCT ON(purpose) purpose,granted_at,revoked_at FROM consent_records WHERE user_id=$1 ORDER BY purpose,granted_at DESC`, s.UserID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	for rows.Next() {
		var purpose string
		var granted time.Time
		var revoked *time.Time
		if err = rows.Scan(&purpose, &granted, &revoked); err != nil {
			rows.Close()
			h.err.Handle(w, r, err)
			return
		}
		if revoked == nil {
			consents = append(consents, purpose)
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		h.err.Handle(w, r, err)
		return
	}
	rows.Close()
	_ = response.OK(w, map[string]any{"id": s.UserID, "college_id": s.CollegeID, "email": s.Email, "display_name": s.DisplayName, "account_type": s.AccountType, "profile": profile, "skills": skills, "consents": consents, "campus_admin": s.CampusAdmin, "mfa_enabled": s.MFAEnabled})
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	s, _ := CurrentSession(r.Context())
	input, err := request.Decode[struct {
		DisplayName   string   `json:"display_name"`
		Bio           string   `json:"bio"`
		Headline      string   `json:"headline"`
		AvatarMediaID string   `json:"avatar_media_id"`
		Skills        []string `json:"skills"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Bio = strings.TrimSpace(input.Bio)
	input.Headline = strings.TrimSpace(input.Headline)
	if len(input.DisplayName) < 1 || len(input.DisplayName) > 120 || len(input.Bio) > 2000 || len(input.Headline) > 160 || len(input.Skills) > 30 {
		h.fail(w, r, http.StatusBadRequest, "profile_invalid", "profile fields exceed the allowed limits", nil)
		return
	}
	if input.AvatarMediaID != "" {
		var owned bool
		if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM media WHERE id=$1 AND owner_user_id=$2 AND scan_status='clean')`, input.AvatarMediaID, s.UserID).Scan(&owned); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if !owned {
			h.fail(w, r, http.StatusBadRequest, "avatar_not_owned", "profile image must be clean media owned by this account", nil)
			return
		}
	}
	profile, _ := json.Marshal(map[string]string{"bio": input.Bio, "headline": input.Headline, "avatar_media_id": input.AvatarMediaID})
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `UPDATE users SET display_name=$2,profile=$3,updated_at=now() WHERE id=$1`, s.UserID, input.DisplayName, profile); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM user_skills WHERE user_id=$1`, s.UserID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	seen := map[string]bool{}
	for _, raw := range input.Skills {
		name := strings.TrimSpace(raw)
		if len(name) < 2 || len(name) > 60 {
			h.fail(w, r, http.StatusBadRequest, "skills_invalid", "skill names must be between 2 and 60 characters", nil)
			return
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		slug := skillSlug(key)
		_, err = tx.Exec(r.Context(), `INSERT INTO skills(slug,name) VALUES($1,$2) ON CONFLICT(name) DO NOTHING`, slug, name)
		if err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO user_skills(user_id,skill_id) SELECT $1,id FROM skills WHERE lower(name)=lower($2) ON CONFLICT DO NOTHING`, s.UserID, name); err != nil {
			h.err.Handle(w, r, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"updated": true})
}

func (h *Handler) updateConsent(w http.ResponseWriter, r *http.Request) {
	s, _ := CurrentSession(r.Context())
	input, err := request.Decode[struct {
		Purpose       string `json:"purpose"`
		PolicyVersion string `json:"policy_version"`
		Granted       bool   `json:"granted"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	allowed := map[string]bool{"terms": true, "privacy": true, "profile_discovery": true, "portfolio": true, "recruiter_discovery": true, "analytics": true}
	if !allowed[input.Purpose] || input.PolicyVersion == "" || len(input.PolicyVersion) > 40 || input.Purpose == "terms" && input.PolicyVersion != termsPolicyVersion || input.Purpose == "privacy" && input.PolicyVersion != privacyPolicyVersion {
		h.fail(w, r, http.StatusBadRequest, "consent_invalid", "consent request is invalid", nil)
		return
	}
	if input.Granted {
		_, err = h.pool.Exec(r.Context(), `INSERT INTO consent_records(user_id,purpose,policy_version)
			SELECT $1,$2,$3 WHERE NOT EXISTS(SELECT 1 FROM consent_records WHERE user_id=$1 AND purpose=$2 AND policy_version=$3 AND revoked_at IS NULL)`, s.UserID, input.Purpose, input.PolicyVersion)
	} else {
		_, err = h.pool.Exec(r.Context(), `UPDATE consent_records SET revoked_at=now() WHERE user_id=$1 AND purpose=$2 AND revoked_at IS NULL`, s.UserID, input.Purpose)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"updated": true})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	s, _ := CurrentSession(r.Context())
	_, err := h.pool.Exec(r.Context(), `DELETE FROM sessions WHERE id=$1`, s.ID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: h.cookieName(), Value: "", Path: "/", Expires: time.Unix(1, 0), MaxAge: -1, HttpOnly: true, Secure: h.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
	response.NoContent(w)
}

func (h *Handler) enrollMFA(w http.ResponseWriter, r *http.Request) {
	s, _ := CurrentSession(r.Context())
	if !h.rateLimit(w, r, "mfa-enroll:"+s.UserID, 5) {
		return
	}
	var alreadyEnabled bool
	if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM mfa_credentials WHERE user_id=$1 AND enabled_at IS NOT NULL)`, s.UserID).Scan(&alreadyEnabled); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if alreadyEnabled && !s.MFAFresh {
		h.fail(w, r, http.StatusForbidden, "mfa_reauthentication_required", "verify your current authenticator before replacing it", nil)
		return
	}
	secret, err := newTOTPSecret()
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	encrypted, err := encryptSecret([]byte(h.cfg.MFAEncryptionKey), []byte(secret))
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `INSERT INTO mfa_credentials(user_id,encrypted_secret) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET encrypted_secret=EXCLUDED.encrypted_secret,enabled_at=NULL,created_at=now()`, s.UserID, encrypted); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM mfa_recovery_codes WHERE user_id=$1`, s.UserID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE sessions SET mfa_verified_at=NULL WHERE user_id=$1`, s.UserID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]string{"secret": secret, "issuer": "PeerGit", "account": s.Email, "type": "TOTP"})
}

func (h *Handler) confirmMFA(w http.ResponseWriter, r *http.Request) {
	s, _ := CurrentSession(r.Context())
	if !h.rateLimit(w, r, "mfa-confirm:"+s.UserID, 10) {
		return
	}
	input, err := request.Decode[struct {
		Code string `json:"code"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err := h.verifyAuthenticator(r.Context(), s.UserID, input.Code, true); err != nil {
		h.fail(w, r, http.StatusUnauthorized, "mfa_code_invalid", "the authenticator code is invalid", err)
		return
	}
	codes := make([]string, 10)
	for i := range codes {
		raw, e := randomToken(8)
		if e != nil {
			h.err.Handle(w, r, e)
			return
		}
		normalized := strings.ToUpper(strings.TrimRight(strings.ReplaceAll(raw, "-", ""), "_"))
		if len(normalized) > 12 {
			normalized = normalized[:12]
		}
		codes[i] = normalized
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `UPDATE mfa_credentials SET enabled_at=COALESCE(enabled_at,now()) WHERE user_id=$1`, s.UserID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM mfa_recovery_codes WHERE user_id=$1`, s.UserID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	for _, code := range codes {
		hash := keyedHash([]byte(h.cfg.MFAEncryptionKey), []byte(code))
		if _, err = tx.Exec(r.Context(), `INSERT INTO mfa_recovery_codes(user_id,code_hash) VALUES($1,$2)`, s.UserID, hash); err != nil {
			h.err.Handle(w, r, err)
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE sessions SET mfa_verified_at=now(),expires_at=LEAST(expires_at,now()+interval '4 hours') WHERE id=$1`, s.ID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"enabled": true, "recovery_codes": codes})
}

func (h *Handler) verifyMFA(w http.ResponseWriter, r *http.Request) {
	s, _ := CurrentSession(r.Context())
	if !h.rateLimit(w, r, "mfa-verify:"+s.UserID, 10) {
		return
	}
	input, err := request.Decode[struct {
		Code string `json:"code"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err := h.verifyAuthenticator(r.Context(), s.UserID, input.Code, false); err != nil {
		hash := keyedHash([]byte(h.cfg.MFAEncryptionKey), []byte(strings.ToUpper(input.Code)))
		tx, e := h.pool.Begin(r.Context())
		if e != nil {
			h.err.Handle(w, r, e)
			return
		}
		defer tx.Rollback(r.Context())
		ct, e := tx.Exec(r.Context(), `UPDATE mfa_recovery_codes SET used_at=now() WHERE user_id=$1 AND code_hash=$2 AND used_at IS NULL AND EXISTS(SELECT 1 FROM mfa_credentials WHERE user_id=$1 AND enabled_at IS NOT NULL)`, s.UserID, hash)
		if e != nil {
			h.err.Handle(w, r, e)
			return
		}
		if ct.RowsAffected() != 1 {
			h.fail(w, r, http.StatusUnauthorized, "mfa_code_invalid", "the authenticator code is invalid", err)
			return
		}
		if _, e = tx.Exec(r.Context(), `UPDATE sessions SET mfa_verified_at=now(),expires_at=LEAST(expires_at,now()+interval '4 hours') WHERE id=$1`, s.ID); e != nil {
			h.err.Handle(w, r, e)
			return
		}
		if e = tx.Commit(r.Context()); e != nil {
			h.err.Handle(w, r, e)
			return
		}
		_ = response.OK(w, map[string]any{"verified": true})
		return
	}
	if _, err := h.pool.Exec(r.Context(), `UPDATE sessions SET mfa_verified_at=now(),expires_at=LEAST(expires_at,now()+interval '4 hours') WHERE id=$1`, s.ID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"verified": true})
}

func (h *Handler) verifyAuthenticator(ctx context.Context, userID, code string, allowPending bool) error {
	var encrypted []byte
	var enabled sql.NullTime
	if err := h.pool.QueryRow(ctx, `SELECT encrypted_secret,enabled_at FROM mfa_credentials WHERE user_id=$1`, userID).Scan(&encrypted, &enabled); err != nil {
		return err
	}
	secret, err := decryptSecret([]byte(h.cfg.MFAEncryptionKey), encrypted)
	if err != nil {
		return err
	}
	if !verifyTOTP(string(secret), code, time.Now().UTC()) {
		return errors.New("invalid time-based code")
	}
	if !enabled.Valid {
		if !allowPending {
			return errors.New("authenticator is not enabled")
		}
	}
	return nil
}

func (h *Handler) cookieName() string {
	if !h.cfg.CookieSecure {
		return "peergit_session"
	}
	return sessionCookie
}
func (h *Handler) setCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: h.cookieName(), Value: token, Path: "/", Expires: expires, HttpOnly: true, Secure: h.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
}

func (h *Handler) rateLimit(w http.ResponseWriter, r *http.Request, kind string, limit int) bool {
	remote := remoteHost(r.RemoteAddr)
	key := keyedHash([]byte(h.cfg.SessionHashKey), []byte(kind+"\x00"+remote))
	var count int
	_, err := h.pool.Exec(r.Context(), `DELETE FROM login_rate_limits WHERE window_started_at < now()-interval '1 day'`)
	if err == nil {
		err = h.pool.QueryRow(r.Context(), `INSERT INTO login_rate_limits(bucket_hash,window_started_at,request_count)
		VALUES($1,now(),1) ON CONFLICT(bucket_hash) DO UPDATE SET
		window_started_at=CASE WHEN login_rate_limits.window_started_at<now()-interval '1 minute' THEN now() ELSE login_rate_limits.window_started_at END,
		request_count=CASE WHEN login_rate_limits.window_started_at<now()-interval '1 minute' THEN 1 ELSE login_rate_limits.request_count+1 END
		RETURNING request_count`, key).Scan(&count)
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return false
	}
	if count > limit {
		w.Header().Set("Retry-After", "60")
		h.fail(w, r, http.StatusTooManyRequests, "rate_limited", "too many sign-in attempts; try again shortly", nil)
		return false
	}
	return true
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, status int, code, message string, cause error) {
	h.err.Handle(w, r, errormanager.New(status, code, message, cause))
}
func hmacEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
