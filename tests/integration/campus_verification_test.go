package integration_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"log/slog"
	"peergit/internal/campus"
	"peergit/internal/identity"
	"peergit/internal/media"
	"peergit/internal/platform/errormanager"
	"peergit/migrations"
)

func TestCampusVerificationCreatesEncryptedDurableDelivery(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run campus verification integration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	key := []byte("campus verification test session key entropy")
	hashKey := "campus verification purpose hash key"
	encryptKey := "campus verification email encryption key"
	slug := fmt.Sprintf("verify-%d", time.Now().UnixNano()%1_000_000_000)
	var college, userID string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES($1,'Verification Test Campus') RETURNING id::text`, slug).Scan(&college); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO college_domains(college_id,domain,verified_at) VALUES($1,'verify.test',now())`, college); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,email_normalized,display_name,handle,account_type) VALUES($1,$1,'Pending Student',$2,'unverified') RETURNING id::text`, fmt.Sprintf("%s@contact.test", slug), strings.ReplaceAll(slug, "-", "_")).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	const token = "campus-verification-session-token"
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,now()+interval '1 hour')`, userID, sessionHash(key, token), sessionCSRFHash(key, token)); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	manager := errormanager.NewManager(logger)
	auth := identity.NewHandler(pool, identity.Config{AppOrigin: "http://peergit.test", SessionHashKey: string(key), CookieSecure: false}, logger, manager)
	campusHandler := campus.NewHandler(pool, auth, "http://peergit.test", manager, hashKey, encryptKey)
	mediaHandler := media.NewHandler(pool, nil, auth, manager)
	router := chi.NewRouter()
	router.Route("/api/v1", func(r chi.Router) { auth.Register(r); campusHandler.Register(r); mediaHandler.Register(r) })
	get := httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/session", nil)
	get.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	gr := httptest.NewRecorder()
	router.ServeHTTP(gr, get)
	if gr.Code != http.StatusOK {
		t.Fatalf("session status=%d %s", gr.Code, gr.Body.String())
	}
	var envelope struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(gr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	privateIntentReq := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/media/intents", strings.NewReader(`{"visibility":"private"}`))
	privateIntentReq.Header.Set("Content-Type", "application/json")
	privateIntentReq.Header.Set("Origin", "http://peergit.test")
	privateIntentReq.Header.Set("X-CSRF-Token", envelope.Data.CSRF)
	privateIntentReq.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	privateIntentRes := httptest.NewRecorder()
	router.ServeHTTP(privateIntentRes, privateIntentReq)
	if privateIntentRes.Code != http.StatusCreated {
		t.Fatalf("unverified private profile image intent status=%d body=%s", privateIntentRes.Code, privateIntentRes.Body.String())
	}
	campusIntentReq := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/media/intents", strings.NewReader(`{"visibility":"campus"}`))
	campusIntentReq.Header.Set("Content-Type", "application/json")
	campusIntentReq.Header.Set("Origin", "http://peergit.test")
	campusIntentReq.Header.Set("X-CSRF-Token", envelope.Data.CSRF)
	campusIntentReq.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	campusIntentRes := httptest.NewRecorder()
	router.ServeHTTP(campusIntentRes, campusIntentReq)
	if campusIntentRes.Code != http.StatusForbidden {
		t.Fatalf("unverified campus-visible image intent status=%d body=%s", campusIntentRes.Code, campusIntentRes.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/me/campus-verification/challenges", strings.NewReader(`{"campus_email":"student@verify.test"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://peergit.test")
	request.Header.Set("X-CSRF-Token", envelope.Data.CSRF)
	request.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	res := httptest.NewRecorder()
	router.ServeHTTP(res, request)
	if res.Code != http.StatusCreated {
		t.Fatalf("challenge status=%d body=%s", res.Code, res.Body.String())
	}
	var challengeResponse struct {
		Data struct {
			ChallengeID string `json:"challenge_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &challengeResponse); err != nil || challengeResponse.Data.ChallengeID == "" {
		t.Fatalf("challenge response missing ID: %s (%v)", res.Body.String(), err)
	}
	if strings.Contains(res.Body.String(), "verify.test") && !strings.Contains(res.Body.String(), "st***@verify.test") {
		t.Fatalf("challenge exposed full address: %s", res.Body.String())
	}
	if strings.Contains(res.Body.String(), "token") || strings.Contains(res.Body.String(), "otp") {
		t.Fatalf("challenge exposed verification secret: %s", res.Body.String())
	}
	var challengeCount, deliveryCount, jobCount, encryptedSize int
	var payload string
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM campus_email_challenges WHERE user_id=$1 AND expires_at>now()`, userID).Scan(&challengeCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*),max(octet_length(encrypted_payload)) FROM campus_email_deliveries d JOIN campus_email_challenges c ON c.id=d.challenge_id WHERE c.user_id=$1`, userID).Scan(&deliveryCount, &encryptedSize); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*),min(payload::text) FROM jobs WHERE actor_id=$1 AND job_type='campus_verification_email'`, userID).Scan(&jobCount, &payload); err != nil {
		t.Fatal(err)
	}
	if challengeCount != 1 || deliveryCount != 1 || jobCount != 1 || encryptedSize == 0 || strings.Contains(payload, "student@verify.test") {
		t.Fatalf("challenge=%d delivery=%d job=%d payload=%q ciphertext-bytes=%d", challengeCount, deliveryCount, jobCount, payload, encryptedSize)
	}
	unsupported := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/me/campus-verification/challenges", strings.NewReader(`{"campus_email":"student@unknown.test"}`))
	unsupported.Header.Set("Content-Type", "application/json")
	unsupported.Header.Set("Origin", "http://peergit.test")
	unsupported.Header.Set("X-CSRF-Token", envelope.Data.CSRF)
	unsupported.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, unsupported)
	if denied.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsupported domain status=%d body=%s", denied.Code, denied.Body.String())
	}
	wrongOTP := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/me/campus-verification/confirm", strings.NewReader(fmt.Sprintf(`{"method":"otp","challenge_id":%q,"otp":"000000"}`, challengeResponse.Data.ChallengeID)))
	wrongOTP.Header.Set("Content-Type", "application/json")
	wrongOTP.Header.Set("Origin", "http://peergit.test")
	wrongOTP.Header.Set("X-CSRF-Token", envelope.Data.CSRF)
	wrongOTP.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	wrong := httptest.NewRecorder()
	router.ServeHTTP(wrong, wrongOTP)
	if wrong.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong OTP status=%d body=%s", wrong.Code, wrong.Body.String())
	}
	for attempt := 1; attempt < 5; attempt++ {
		badOTP := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/me/campus-verification/confirm", strings.NewReader(fmt.Sprintf(`{"method":"otp","challenge_id":%q,"otp":"000000"}`, challengeResponse.Data.ChallengeID)))
		badOTP.Header.Set("Content-Type", "application/json")
		badOTP.Header.Set("Origin", "http://peergit.test")
		badOTP.Header.Set("X-CSRF-Token", envelope.Data.CSRF)
		badOTP.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
		badResponse := httptest.NewRecorder()
		router.ServeHTTP(badResponse, badOTP)
		if badResponse.Code != http.StatusUnprocessableEntity {
			t.Fatalf("wrong OTP attempt %d status=%d body=%s", attempt+1, badResponse.Code, badResponse.Body.String())
		}
	}
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT failed_attempts FROM campus_email_challenges WHERE id=$1`, challengeResponse.Data.ChallengeID).Scan(&attempts); err != nil || attempts != 5 {
		t.Fatalf("failed OTP attempts=%d err=%v, want lock at five", attempts, err)
	}
	resend := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/me/campus-verification/challenges", strings.NewReader(`{"campus_email":"student@verify.test"}`))
	resend.Header.Set("Content-Type", "application/json")
	resend.Header.Set("Origin", "http://peergit.test")
	resend.Header.Set("X-CSRF-Token", envelope.Data.CSRF)
	resend.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	resendResponse := httptest.NewRecorder()
	router.ServeHTTP(resendResponse, resend)
	if resendResponse.Code != http.StatusTooManyRequests {
		t.Fatalf("resend cooldown status=%d body=%s, want 429", resendResponse.Code, resendResponse.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE campus_email_challenges SET created_at=now()-interval '61 seconds' WHERE id=$1`, challengeResponse.Data.ChallengeID); err != nil {
		t.Fatal(err)
	}
	resend = httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/me/campus-verification/challenges", strings.NewReader(`{"campus_email":"student@verify.test"}`))
	resend.Header.Set("Content-Type", "application/json")
	resend.Header.Set("Origin", "http://peergit.test")
	resend.Header.Set("X-CSRF-Token", envelope.Data.CSRF)
	resend.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	resendResponse = httptest.NewRecorder()
	router.ServeHTTP(resendResponse, resend)
	if resendResponse.Code != http.StatusCreated {
		t.Fatalf("resend after cooldown status=%d body=%s", resendResponse.Code, resendResponse.Body.String())
	}
	var replacement struct {
		Data struct {
			ChallengeID string `json:"challenge_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resendResponse.Body.Bytes(), &replacement); err != nil || replacement.Data.ChallengeID == "" || replacement.Data.ChallengeID == challengeResponse.Data.ChallengeID {
		t.Fatalf("replacement challenge missing or reused: %s (%v)", resendResponse.Body.String(), err)
	}
	oldOTP := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/me/campus-verification/confirm", strings.NewReader(fmt.Sprintf(`{"method":"otp","challenge_id":%q,"otp":"123456"}`, challengeResponse.Data.ChallengeID)))
	oldOTP.Header.Set("Content-Type", "application/json")
	oldOTP.Header.Set("Origin", "http://peergit.test")
	oldOTP.Header.Set("X-CSRF-Token", envelope.Data.CSRF)
	oldOTP.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	oldOTPResponse := httptest.NewRecorder()
	router.ServeHTTP(oldOTPResponse, oldOTP)
	if oldOTPResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("superseded challenge status=%d body=%s", oldOTPResponse.Code, oldOTPResponse.Body.String())
	}
	knownOTP := "123456"
	otpHash := verificationHash(hashKey, knownOTP)
	if _, err := pool.Exec(ctx, `UPDATE campus_email_challenges SET otp_hash=$2 WHERE id=$1`, replacement.Data.ChallengeID, otpHash); err != nil {
		t.Fatal(err)
	}
	correctOTP := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/me/campus-verification/confirm", strings.NewReader(fmt.Sprintf(`{"method":"otp","challenge_id":%q,"otp":%q}`, replacement.Data.ChallengeID, knownOTP)))
	correctOTP.Header.Set("Content-Type", "application/json")
	correctOTP.Header.Set("Origin", "http://peergit.test")
	correctOTP.Header.Set("X-CSRF-Token", envelope.Data.CSRF)
	correctOTP.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	confirmed := httptest.NewRecorder()
	router.ServeHTTP(confirmed, correctOTP)
	if confirmed.Code != http.StatusOK {
		t.Fatalf("OTP confirmation status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}
	var accountType, verifiedSource string
	var assignedCollege string
	if err := pool.QueryRow(ctx, `SELECT u.account_type,u.college_id::text,v.source FROM users u JOIN campus_verifications v ON v.user_id=u.id WHERE u.id=$1`, userID).Scan(&accountType, &assignedCollege, &verifiedSource); err != nil {
		t.Fatal(err)
	}
	if accountType != "campus" || assignedCollege != college || verifiedSource != "campus_email_otp" {
		t.Fatalf("account transition type=%s college=%s source=%s", accountType, assignedCollege, verifiedSource)
	}
}

func verificationHash(key, value string) []byte {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func TestCampusReviewApprovalAuditsAndRefreshesTargetSessions(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run campus review integration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	slug := fmt.Sprintf("review-%d", time.Now().UnixNano()%1_000_000_000)
	var campusID, adminID, studentID string
	handlePrefix := strings.ReplaceAll(slug, "-", "_")
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES($1,'Review Test Campus') RETURNING id::text`, slug).Scan(&campusID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(college_id,email,email_normalized,display_name,handle,account_type) VALUES($1,$2,$2,'Review Admin',$3,'campus') RETURNING id::text`, campusID, slug+"-admin@contact.test", handlePrefix+"_admin").Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	for _, setup := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO campus_verifications(college_id,user_id,source) VALUES($1,$2,'administrator_review')`, []any{campusID, adminID}},
		{`INSERT INTO campus_roles(college_id,user_id,role) VALUES($1,$2,'campus_admin')`, []any{campusID, adminID}},
		{`INSERT INTO consent_records(user_id,purpose,policy_version) VALUES($1,'terms','draft-1'),($1,'privacy','draft-1')`, []any{adminID}},
		{`INSERT INTO mfa_credentials(user_id,encrypted_secret,enabled_at) VALUES($1,'test-secret',now())`, []any{adminID}},
	} {
		if _, err := pool.Exec(ctx, setup.query, setup.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,email_normalized,display_name,handle,account_type) VALUES($1,$1,'Pending Student',$2,'unverified') RETURNING id::text`, slug+"-student@contact.test", handlePrefix+"_student").Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	key := []byte("campus review integration session key entropy")
	const adminToken, studentToken = "review-admin-session", "review-student-session"
	for _, session := range []struct{ user, token string }{{adminID, adminToken}, {studentID, studentToken}} {
		if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,college_id,token_hash,csrf_hash,mfa_verified_at,expires_at) VALUES($1,(SELECT college_id FROM users WHERE id=$1),$2,$3,CASE WHEN $1=$4 THEN now() ELSE NULL END,now()+interval '1 hour')`, session.user, sessionHash(key, session.token), sessionCSRFHash(key, session.token), adminID); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	manager := errormanager.NewManager(logger)
	auth := identity.NewHandler(pool, identity.Config{AppOrigin: "http://peergit.test", SessionHashKey: string(key), MFAEncryptionKey: "campus review MFA encryption key entropy", CookieSecure: false}, logger, manager)
	campusHandler := campus.NewHandler(pool, auth, "http://peergit.test", manager, "campus review hashing key entropy", "campus review email encryption key")
	router := chi.NewRouter()
	router.Route("/api/v1", func(r chi.Router) { auth.Register(r); campusHandler.Register(r) })
	csrfFor := func(token string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/session", nil)
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("session status=%d body=%s", res.Code, res.Body.String())
		}
		var value struct {
			Data struct {
				CSRF string `json:"csrf_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &value); err != nil || value.Data.CSRF == "" {
			t.Fatalf("session CSRF missing: %s (%v)", res.Body.String(), err)
		}
		return value.Data.CSRF
	}
	request := func(token, csrf, method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://peergit.test"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://peergit.test")
		req.Header.Set("X-CSRF-Token", csrf)
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, path, res.Code, want, res.Body.String())
		}
		return res
	}
	studentCSRF, adminCSRF := csrfFor(studentToken), csrfFor(adminToken)
	created := request(studentToken, studentCSRF, http.MethodPost, "/api/v1/me/campus-review-requests", `{"campus_id":"`+slug+`","reason":"My campus email is not supported yet."}`, http.StatusCreated)
	var createdEnvelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdEnvelope); err != nil || createdEnvelope.Data.ID == "" {
		t.Fatalf("review request ID missing: %s (%v)", created.Body.String(), err)
	}
	request(adminToken, adminCSRF, http.MethodPost, "/api/v1/admin/campus-review-requests/"+createdEnvelope.Data.ID+"/decision", `{"decision":"approved","reason":"Reviewed institutional records."}`, http.StatusOK)
	var role, source, tenant string
	if err := pool.QueryRow(ctx, `SELECT cr.role,v.source,u.college_id::text FROM campus_roles cr JOIN campus_verifications v ON v.user_id=cr.user_id AND v.college_id=cr.college_id JOIN users u ON u.id=cr.user_id WHERE cr.user_id=$1`, studentID).Scan(&role, &source, &tenant); err != nil {
		t.Fatal(err)
	}
	if role != "student" || source != "administrator_review" || tenant != campusID {
		t.Fatalf("review approval role=%s source=%s tenant=%s", role, source, tenant)
	}
	var sessionCampus string
	if err := pool.QueryRow(ctx, `SELECT college_id::text FROM sessions WHERE user_id=$1`, studentID).Scan(&sessionCampus); err != nil || sessionCampus != campusID {
		t.Fatalf("student session campus=%s err=%v, expected approval to refresh access", sessionCampus, err)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND actor_id=$2 AND action='campus.review.requested'`, campusID, studentID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("campus review request audit count=%d err=%v", auditCount, err)
	}
}
