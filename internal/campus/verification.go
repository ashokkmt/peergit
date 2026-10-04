package campus

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"peergit/internal/identity"
	"peergit/internal/platform/audit"
	"peergit/internal/platform/http/request"
	"peergit/internal/platform/http/response"
)

type deliveryPayload struct{ To, Code, Link string }

func (h *Handler) onboarding(w http.ResponseWriter, r *http.Request) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
		return
	}
	var campusID, accountType string
	if err := h.pool.QueryRow(r.Context(), `SELECT COALESCE(college_id::text,''),account_type FROM users WHERE id=$1 AND status='active'`, s.UserID).Scan(&campusID, &accountType); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	var challenge *struct {
		ID      string    `json:"challenge_id"`
		Email   string    `json:"email"`
		Expires time.Time `json:"expires_at"`
	}
	var item struct {
		ID      string    `json:"challenge_id"`
		Email   string    `json:"email"`
		Expires time.Time `json:"expires_at"`
	}
	err := h.pool.QueryRow(r.Context(), `SELECT ch.id::text,ce.email_normalized,ch.expires_at FROM campus_email_challenges ch JOIN campus_emails ce ON ce.id=ch.campus_email_id WHERE ch.user_id=$1 AND ch.consumed_at IS NULL AND ch.superseded_at IS NULL AND ch.expires_at>now() ORDER BY ch.created_at DESC LIMIT 1`, s.UserID).Scan(&item.ID, &item.Email, &item.Expires)
	if err == nil {
		item.Email = maskEmail(item.Email)
		challenge = &item
	} else if !errors.Is(err, pgx.ErrNoRows) {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"account_type": accountType, "campus_id": campusID, "campus_verified": campusID != "" && s.CollegeID != "", "pending_challenge": challenge})
}

func (h *Handler) startCampusChallenge(w http.ResponseWriter, r *http.Request) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
		return
	}
	if s.AccountType != "unverified" {
		h.fail(w, r, http.StatusConflict, "account_not_pending_campus", "only an unverified account can request first-time campus verification", nil)
		return
	}
	input, err := request.Decode[struct {
		Email string `json:"campus_email"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	parsed, err := mail.ParseAddress(strings.TrimSpace(input.Email))
	if err != nil || parsed.Address != strings.TrimSpace(input.Email) {
		h.fail(w, r, http.StatusBadRequest, "campus_email_invalid", "enter a valid campus email address", nil)
		return
	}
	email := strings.ToLower(parsed.Address)
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" {
		h.fail(w, r, http.StatusBadRequest, "campus_email_invalid", "enter a valid campus email address", nil)
		return
	}
	if s.CollegeID != "" {
		h.fail(w, r, http.StatusConflict, "campus_already_assigned", "your account already has a campus; contact support for a transfer", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var accountType, existingCampus string
	if err = tx.QueryRow(r.Context(), `SELECT account_type,COALESCE(college_id::text,'') FROM users WHERE id=$1 AND status='active' FOR UPDATE`, s.UserID).Scan(&accountType, &existingCampus); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if accountType != "unverified" || existingCampus != "" {
		h.fail(w, r, http.StatusConflict, "account_not_pending_campus", "this account cannot start first-time campus verification", nil)
		return
	}
	var collegeID string
	err = tx.QueryRow(r.Context(), `SELECT c.id::text FROM college_domains d JOIN colleges c ON c.id=d.college_id WHERE d.domain=$1 AND d.verified_at IS NOT NULL AND c.status='active'`, parts[1]).Scan(&collegeID)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, http.StatusUnprocessableEntity, "campus_domain_unsupported", "this campus email domain is not enabled; request campus review instead", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if !h.allowSend(r.Context(), tx, s.UserID, email, r.RemoteAddr) {
		h.fail(w, r, http.StatusTooManyRequests, "verification_rate_limited", "too many verification messages; wait before trying again", nil)
		return
	}
	var recent time.Time
	err = tx.QueryRow(r.Context(), `SELECT created_at FROM campus_email_challenges WHERE user_id=$1 ORDER BY created_at DESC LIMIT 1`, s.UserID).Scan(&recent)
	if err == nil && time.Since(recent) < time.Minute {
		h.fail(w, r, http.StatusTooManyRequests, "verification_cooldown", "wait 60 seconds before requesting another code", nil)
		return
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.err.Handle(w, r, err)
		return
	}
	var other string
	err = tx.QueryRow(r.Context(), `SELECT id::text FROM campus_emails WHERE email_normalized=$1 AND verified_at IS NOT NULL AND user_id<>$2`, email, s.UserID).Scan(&other)
	if err == nil {
		h.fail(w, r, http.StatusConflict, "campus_email_unavailable", "this campus email cannot be used for this account", nil)
		return
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.err.Handle(w, r, err)
		return
	}
	var campusEmailID string
	err = tx.QueryRow(r.Context(), `INSERT INTO campus_emails(user_id,college_id,email_normalized) VALUES($1,$2,$3) ON CONFLICT(college_id,email_normalized) DO UPDATE SET user_id=EXCLUDED.user_id WHERE campus_emails.user_id=EXCLUDED.user_id RETURNING id::text`, s.UserID, collegeID, email).Scan(&campusEmailID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.fail(w, r, http.StatusConflict, "campus_email_unavailable", "this campus email cannot be used for this account", nil)
		} else {
			h.err.Handle(w, r, err)
		}
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE campus_email_challenges SET superseded_at=now() WHERE user_id=$1 AND consumed_at IS NULL AND superseded_at IS NULL`, s.UserID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	link, err := randomSecret(32)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	code := fmt.Sprintf("%06d", n.Int64())
	linkHash := h.secretHash(link)
	otpHash := h.secretHash(code)
	var challengeID, deliveryID string
	err = tx.QueryRow(r.Context(), `INSERT INTO campus_email_challenges(user_id,college_id,campus_email_id,link_hash,otp_hash,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '10 minutes') RETURNING id::text`, s.UserID, collegeID, campusEmailID, linkHash, otpHash).Scan(&challengeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	message, err := json.Marshal(deliveryPayload{To: email, Code: code, Link: strings.TrimRight(h.origin, "/") + "/onboarding/verify#token=" + link})
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	encrypted, err := h.encrypt(message)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	err = tx.QueryRow(r.Context(), `INSERT INTO campus_email_deliveries(challenge_id,encrypted_payload,expires_at) VALUES($1,$2,now()+interval '10 minutes') RETURNING id::text`, challengeID, encrypted).Scan(&deliveryID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	payload, _ := json.Marshal(map[string]string{"delivery_id": deliveryID})
	if _, err = tx.Exec(r.Context(), `INSERT INTO jobs(tenant_id,actor_id,job_type,dedupe_key,payload,deadline_at) VALUES($1,$2,'campus_verification_email',$3,$4,now()+interval '10 minutes')`, collegeID, s.UserID, deliveryID, payload); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"challenge_id": challengeID, "email": maskEmail(email), "expires_in_seconds": 600})
}

func (h *Handler) confirmCampusChallenge(w http.ResponseWriter, r *http.Request) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
		return
	}
	in, err := request.Decode[struct {
		Method      string `json:"method"`
		Token       string `json:"token"`
		ChallengeID string `json:"challenge_id"`
		OTP         string `json:"otp"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Method != "link" && in.Method != "otp" {
		h.fail(w, r, http.StatusBadRequest, "verification_method_invalid", "choose the email link or six-digit code", nil)
		return
	}
	if in.Method == "link" && (len(in.Token) < 32 || len(in.Token) > 128) || in.Method == "otp" && (len(in.OTP) != 6 || in.ChallengeID == "") {
		h.fail(w, r, http.StatusBadRequest, "verification_credential_invalid", "verification credential is invalid", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var currentCampus, accountType string
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(college_id::text,''),account_type FROM users WHERE id=$1 AND status='active' FOR UPDATE`, s.UserID).Scan(&currentCampus, &accountType); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if currentCampus != "" || accountType != "unverified" {
		h.fail(w, r, http.StatusConflict, "campus_already_assigned", "this account already has a campus affiliation", nil)
		return
	}
	var id, collegeID, email string
	var linkHash, otpHash []byte
	var attempts int
	query := `SELECT ch.id::text,ch.college_id::text,ce.email_normalized,ch.link_hash,ch.otp_hash,ch.failed_attempts FROM campus_email_challenges ch JOIN campus_emails ce ON ce.id=ch.campus_email_id JOIN colleges c ON c.id=ch.college_id JOIN college_domains d ON d.college_id=c.id AND d.domain=split_part(ce.email_normalized,'@',2) AND d.verified_at IS NOT NULL JOIN users u ON u.id=ch.user_id WHERE ch.user_id=$1 AND ch.consumed_at IS NULL AND ch.superseded_at IS NULL AND ch.expires_at>now() AND c.status='active' AND u.status='active' AND u.account_type='unverified' AND u.college_id IS NULL AND `
	if in.Method == "link" {
		query += `ch.link_hash=$2 FOR UPDATE`
		err = tx.QueryRow(r.Context(), query, s.UserID, h.secretHash(in.Token)).Scan(&id, &collegeID, &email, &linkHash, &otpHash, &attempts)
	} else {
		query += `ch.id=$2 AND ch.failed_attempts<5 FOR UPDATE`
		err = tx.QueryRow(r.Context(), query, s.UserID, in.ChallengeID).Scan(&id, &collegeID, &email, &linkHash, &otpHash, &attempts)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, http.StatusUnprocessableEntity, "verification_expired", "this verification request is expired or no longer valid", nil)
		return
	}
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if in.Method == "otp" && !hmac.Equal(otpHash, h.secretHash(in.OTP)) {
		if attempts < 5 {
			if _, err = tx.Exec(r.Context(), `UPDATE campus_email_challenges SET failed_attempts=failed_attempts+1 WHERE id=$1 AND failed_attempts<5`, id); err != nil {
				h.err.Handle(w, r, err)
				return
			}
		}
		_ = tx.Commit(r.Context())
		h.fail(w, r, http.StatusUnprocessableEntity, "verification_code_invalid", "that code is incorrect or locked; request a new email", nil)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE campus_email_challenges SET consumed_at=now() WHERE id=$1 AND consumed_at IS NULL`, id); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE campus_emails SET verified_at=now() WHERE id=(SELECT campus_email_id FROM campus_email_challenges WHERE id=$1)`, id); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE users SET college_id=$2,account_type='campus',updated_at=now() WHERE id=$1`, s.UserID, collegeID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	source := "campus_email_link"
	if in.Method == "otp" {
		source = "campus_email_otp"
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO campus_verifications(college_id,user_id,source,campus_email_id) VALUES($1,$2,$3,(SELECT campus_email_id FROM campus_email_challenges WHERE id=$4))`, collegeID, s.UserID, source, id); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO campus_roles(college_id,user_id,role) VALUES($1,$2,'student') ON CONFLICT DO NOTHING`, collegeID, s.UserID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE invitations SET accepted_by=$3,accepted_at=now() WHERE college_id=$1 AND email_normalized=$2 AND account_type='campus' AND expires_at>now() AND accepted_at IS NULL AND revoked_at IS NULL`, collegeID, email, s.UserID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE sessions SET college_id=$2 WHERE id=$1 AND user_id=$3`, s.ID, collegeID, s.UserID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1 AND id<>$2`, s.UserID, s.ID); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: collegeID, ActorID: s.UserID, Action: "campus.email_verified", ResourceType: "campus_verification", ResourceID: id}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"verified": true, "campus_id": collegeID, "role": "student"})
}

func (h *Handler) secretHash(value string) []byte {
	m := hmac.New(sha256.New, []byte(h.hashKey))
	_, _ = m.Write([]byte(value))
	return m.Sum(nil)
}
func (h *Handler) encrypt(plain []byte) ([]byte, error) {
	key := sha256.Sum256([]byte(h.encryptionKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return append(nonce, gcm.Seal(nil, nonce, plain, nil)...), nil
}
func (h *Handler) decrypt(value []byte) ([]byte, error) {
	key := sha256.Sum256([]byte(h.encryptionKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(value) < gcm.NonceSize() {
		return nil, errors.New("encrypted message is invalid")
	}
	return gcm.Open(nil, value[:gcm.NonceSize()], value[gcm.NonceSize():], nil)
}
func randomSecret(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
func maskEmail(email string) string {
	p := strings.Split(email, "@")
	if len(p) != 2 {
		return ""
	}
	prefix := p[0]
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	return prefix + "***@" + p[1]
}

func (h *Handler) allowSend(ctx context.Context, tx pgx.Tx, user, email, remote string) bool {
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	if remote == "" {
		remote = "unknown"
	}
	for _, key := range []string{"verify:user:" + user, "verify:email:" + email, "verify:ip:" + remote} {
		hash := h.secretHash(key)
		var count int
		err := tx.QueryRow(ctx, `INSERT INTO login_rate_limits(bucket_hash,window_started_at,request_count) VALUES($1,now(),1) ON CONFLICT(bucket_hash) DO UPDATE SET window_started_at=CASE WHEN login_rate_limits.window_started_at<now()-interval '1 hour' THEN now() ELSE login_rate_limits.window_started_at END,request_count=CASE WHEN login_rate_limits.window_started_at<now()-interval '1 hour' THEN 1 ELSE login_rate_limits.request_count+1 END RETURNING request_count`, hash).Scan(&count)
		limit := 5
		if strings.HasPrefix(key, "verify:ip:") {
			limit = 20
		}
		if err != nil || count > limit {
			return false
		}
	}
	return true
}

func (h *Handler) createCampusReview(w http.ResponseWriter, r *http.Request) {
	s, ok := identity.CurrentSession(r.Context())
	if !ok {
		h.fail(w, r, http.StatusUnauthorized, "authentication_required", "sign in is required", nil)
		return
	}
	if s.AccountType != "unverified" {
		h.fail(w, r, http.StatusConflict, "account_not_pending_campus", "only an unverified account can request campus review", nil)
		return
	}
	if s.CollegeID != "" {
		h.fail(w, r, http.StatusConflict, "campus_already_assigned", "your account already has a campus", nil)
		return
	}
	in, err := request.Decode[struct {
		CampusID string `json:"campus_id"`
		Reason   string `json:"reason"`
		Email    string `json:"campus_email"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if len(in.Reason) < 8 || len(in.Reason) > 2000 {
		h.fail(w, r, http.StatusBadRequest, "review_reason_invalid", "explain why the campus should review this request", nil)
		return
	}
	if in.Email != "" {
		parsed, parseErr := mail.ParseAddress(in.Email)
		if parseErr != nil || parsed.Address != in.Email {
			h.fail(w, r, http.StatusBadRequest, "campus_email_invalid", "enter a valid campus email address or leave it blank", nil)
			return
		}
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var id, reviewCampusID string
	err = tx.QueryRow(r.Context(), `INSERT INTO campus_review_requests(user_id,college_id,requested_email,reason) SELECT $1,c.id,$3,$4 FROM colleges c WHERE (c.id::text=$2 OR c.slug=$2) AND c.status='active' RETURNING id::text,college_id::text`, s.UserID, in.CampusID, in.Email, in.Reason).Scan(&id, &reviewCampusID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no rows") {
			h.fail(w, r, http.StatusNotFound, "campus_not_found", "active campus was not found", nil)
		} else if strings.Contains(strings.ToLower(err.Error()), "unique") {
			h.fail(w, r, http.StatusConflict, "review_already_pending", "you already have a pending campus review", nil)
		} else {
			h.err.Handle(w, r, err)
		}
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: reviewCampusID, ActorID: s.UserID, Action: "campus.review.requested", ResourceType: "campus_review_request", ResourceID: id}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.Created(w, map[string]any{"id": id, "state": "pending"})
}

func (h *Handler) listCampusReviews(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT id::text,user_id::text,requested_email,reason,created_at FROM campus_review_requests WHERE college_id=$1 AND state='pending' ORDER BY created_at,id`, s.CollegeID)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID      string    `json:"id"`
		UserID  string    `json:"user_id"`
		Email   string    `json:"email"`
		Reason  string    `json:"reason"`
		Created time.Time `json:"created_at"`
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.ID, &v.UserID, &v.Email, &v.Reason, &v.Created); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"items": items})
}

func (h *Handler) decideCampusReview(w http.ResponseWriter, r *http.Request) {
	s, ok := h.admin(w, r)
	if !ok {
		return
	}
	in, err := request.Decode[struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}](r)
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if (in.Decision != "approved" && in.Decision != "rejected") || len(in.Reason) < 8 || len(in.Reason) > 2000 {
		h.fail(w, r, http.StatusBadRequest, "review_decision_invalid", "choose approved or rejected and provide a review reason", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.err.Handle(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var userID, campusID, requestedEmail string
	err = tx.QueryRow(r.Context(), `SELECT user_id::text,college_id::text,requested_email FROM campus_review_requests WHERE id=$1 AND college_id=$2 AND state='pending' FOR UPDATE`, chi.URLParam(r, "id"), s.CollegeID).Scan(&userID, &campusID, &requestedEmail)
	if err != nil {
		h.fail(w, r, http.StatusNotFound, "review_not_found", "pending campus review was not found", err)
		return
	}
	if userID == s.UserID {
		h.fail(w, r, http.StatusForbidden, "review_self_approval", "you cannot decide your own campus review", nil)
		return
	}
	if in.Decision == "approved" {
		var current, accountType string
		if err = tx.QueryRow(r.Context(), `SELECT COALESCE(college_id::text,''),account_type FROM users WHERE id=$1 AND status='active' FOR UPDATE`, userID).Scan(&current, &accountType); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if current != "" || accountType != "unverified" {
			h.fail(w, r, http.StatusConflict, "campus_already_assigned", "this user already has a campus affiliation", nil)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE users SET college_id=$2,account_type='campus',updated_at=now() WHERE id=$1`, userID, campusID); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO campus_verifications(college_id,user_id,source,verified_by) VALUES($1,$2,'administrator_review',$3)`, campusID, userID, s.UserID); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO campus_roles(college_id,user_id,role,granted_by) VALUES($1,$2,'student',$3)`, campusID, userID, s.UserID); err != nil {
			h.err.Handle(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE sessions SET college_id=$2 WHERE user_id=$1`, userID, campusID); err != nil {
			h.err.Handle(w, r, err)
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE campus_review_requests SET state=$2,reviewed_by=$3,reviewed_at=now(),review_reason=$4 WHERE id=$1`, chi.URLParam(r, "id"), in.Decision, s.UserID, in.Reason); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = audit.Append(r.Context(), tx, audit.Entry{TenantID: campusID, ActorID: s.UserID, Action: "campus.review." + in.Decision, ResourceType: "campus_review_request", ResourceID: chi.URLParam(r, "id"), Details: jsonDetails("reason", in.Reason)}); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.err.Handle(w, r, err)
		return
	}
	_ = response.OK(w, map[string]any{"state": in.Decision, "mailbox_verified": false, "user_id": userID, "requested_email": maskEmail(requestedEmail)})
}

var _ = context.Canceled
