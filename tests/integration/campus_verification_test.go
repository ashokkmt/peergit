package integration_test

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"peergit/internal/platform/jobs"
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
	var privateIntentEnvelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(privateIntentRes.Body.Bytes(), &privateIntentEnvelope); err != nil || privateIntentEnvelope.Data.ID == "" {
		t.Fatalf("private upload intent missing ID: %s (%v)", privateIntentRes.Body.String(), err)
	}
	var otherUser string
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,email_normalized,display_name,handle,account_type) VALUES('other@contact.test','other@contact.test','Other User','verify_other','unverified') RETURNING id::text`).Scan(&otherUser); err != nil {
		t.Fatal(err)
	}
	const otherToken = "other-verification-session"
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,now()+interval '1 hour')`, otherUser, sessionHash(key, otherToken), sessionCSRFHash(key, otherToken)); err != nil {
		t.Fatal(err)
	}
	otherSession := httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/session", nil)
	otherSession.AddCookie(&http.Cookie{Name: "peergit_session", Value: otherToken})
	otherSessionResponse := httptest.NewRecorder()
	router.ServeHTTP(otherSessionResponse, otherSession)
	var otherSessionEnvelope struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	if otherSessionResponse.Code != http.StatusOK || json.Unmarshal(otherSessionResponse.Body.Bytes(), &otherSessionEnvelope) != nil || otherSessionEnvelope.Data.CSRF == "" {
		t.Fatalf("other session status=%d body=%s", otherSessionResponse.Code, otherSessionResponse.Body.String())
	}
	foreignUpload := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/media/intents/"+privateIntentEnvelope.Data.ID+"/content", nil)
	foreignUpload.Header.Set("Origin", "http://peergit.test")
	foreignUpload.Header.Set("X-CSRF-Token", otherSessionEnvelope.Data.CSRF)
	foreignUpload.AddCookie(&http.Cookie{Name: "peergit_session", Value: otherToken})
	foreignUploadResponse := httptest.NewRecorder()
	router.ServeHTTP(foreignUploadResponse, foreignUpload)
	if foreignUploadResponse.Code != http.StatusForbidden {
		t.Fatalf("another user consumed a profile upload intent: status=%d body=%s", foreignUploadResponse.Code, foreignUploadResponse.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE media_upload_intents SET expires_at=now()-interval '1 minute' WHERE id=$1`, privateIntentEnvelope.Data.ID); err != nil {
		t.Fatal(err)
	}
	expiredUpload := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/media/intents/"+privateIntentEnvelope.Data.ID+"/content", nil)
	expiredUpload.Header.Set("Origin", "http://peergit.test")
	expiredUpload.Header.Set("X-CSRF-Token", envelope.Data.CSRF)
	expiredUpload.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
	expiredUploadResponse := httptest.NewRecorder()
	router.ServeHTTP(expiredUploadResponse, expiredUpload)
	if expiredUploadResponse.Code != http.StatusForbidden {
		t.Fatalf("expired profile upload intent status=%d body=%s", expiredUploadResponse.Code, expiredUploadResponse.Body.String())
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

func TestCampusVerificationAccountFenceRaceAndDurableLimits(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run campus verification race and limit checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	slug := fmt.Sprintf("verify-fence-%d", time.Now().UnixNano()%1_000_000_000)
	var campusID string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES($1,'Verification Fence Campus') RETURNING id::text`, slug).Scan(&campusID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO college_domains(college_id,domain,verified_at) VALUES($1,'verify.test',now())`, campusID); err != nil {
		t.Fatal(err)
	}
	key := []byte("campus fence integration session key entropy")
	hashKey := "campus fence challenge hash key entropy"
	users, tokens := map[string]string{}, map[string]string{}
	for _, name := range []string{"first", "second"} {
		var userID string
		if err := pool.QueryRow(ctx, `INSERT INTO users(email,email_normalized,display_name,handle,account_type) VALUES($1,$1,$2,$3,'unverified') RETURNING id::text`, slug+"-"+name+"@contact.test", name, strings.ReplaceAll(slug+"_"+name, "-", "_")).Scan(&userID); err != nil {
			t.Fatal(err)
		}
		users[name] = userID
		tokens[name] = "campus-fence-" + name + "-session"
		if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,now()+interval '1 hour')`, users[name], sessionHash(key, tokens[name]), sessionCSRFHash(key, tokens[name])); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := errormanager.NewManager(logger)
	auth := identity.NewHandler(pool, identity.Config{AppOrigin: "http://peergit.test", SessionHashKey: string(key), CookieSecure: false}, logger, manager)
	campusHandler := campus.NewHandler(pool, auth, "http://peergit.test", manager, hashKey, "campus fence email encryption key")
	router := chi.NewRouter()
	router.Route("/api/v1", func(r chi.Router) { auth.Register(r); campusHandler.Register(r) })
	csrfFor := func(token string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "http://peergit.test/api/v1/session", nil)
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		var envelope struct {
			Data struct {
				CSRF string `json:"csrf_token"`
			} `json:"data"`
		}
		if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &envelope) != nil || envelope.Data.CSRF == "" {
			t.Fatalf("session status=%d body=%s", res.Code, res.Body.String())
		}
		return envelope.Data.CSRF
	}
	confirm := func(token, csrf, body string) int {
		req := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/me/campus-verification/confirm", strings.NewReader(body))
		req.RemoteAddr = "192.0.2.1:1234"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://peergit.test")
		req.Header.Set("X-CSRF-Token", csrf)
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: token})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res.Code
	}
	csrfFirst, csrfSecond := csrfFor(tokens["first"]), csrfFor(tokens["second"])
	link, otp := strings.Repeat("L", 43), "123456"
	var emailID, challengeID string
	if err := pool.QueryRow(ctx, `INSERT INTO campus_emails(user_id,college_id,email_normalized) VALUES($1,$2,'first@verify.test') RETURNING id::text`, users["first"], campusID).Scan(&emailID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO campus_email_challenges(user_id,college_id,campus_email_id,link_hash,otp_hash,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '10 minutes') RETURNING id::text`, users["first"], campusID, emailID, verificationHash(hashKey, link), verificationHash(hashKey, otp)).Scan(&challengeID); err != nil {
		t.Fatal(err)
	}
	if got := confirm(tokens["second"], csrfSecond, fmt.Sprintf(`{"method":"link","token":%q}`, link)); got != http.StatusUnprocessableEntity {
		t.Fatalf("cross-account link status=%d, want 422", got)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO campus_emails(user_id,college_id,email_normalized) VALUES($1,$2,'second@verify.test')`, users["second"], campusID); err != nil {
		t.Fatal(err)
	}
	var secondEmailID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM campus_emails WHERE user_id=$1 AND email_normalized='second@verify.test'`, users["second"]).Scan(&secondEmailID); err != nil {
		t.Fatal(err)
	}
	var expiredChallengeID string
	if err := pool.QueryRow(ctx, `INSERT INTO campus_email_challenges(user_id,college_id,campus_email_id,link_hash,otp_hash,expires_at,created_at) VALUES($1,$2,$3,$4,$5,now()-interval '1 minute',now()-interval '2 minutes') RETURNING id::text`, users["second"], campusID, secondEmailID, verificationHash(hashKey, strings.Repeat("E", 43)), verificationHash(hashKey, "654321")).Scan(&expiredChallengeID); err != nil {
		t.Fatal(err)
	}
	if got := confirm(tokens["second"], csrfSecond, fmt.Sprintf(`{"method":"otp","challenge_id":"%s","otp":"654321"}`, expiredChallengeID)); got != http.StatusUnprocessableEntity {
		t.Fatalf("expired challenge status=%d, want 422", got)
	}
	results := make(chan int, 2)
	for _, body := range []string{fmt.Sprintf(`{"method":"link","token":%q}`, link), fmt.Sprintf(`{"method":"otp","challenge_id":%q,"otp":%q}`, challengeID, otp)} {
		go func(body string) { results <- confirm(tokens["first"], csrfFirst, body) }(body)
	}
	a, b := <-results, <-results
	if !((a == http.StatusOK && (b == http.StatusConflict || b == http.StatusUnprocessableEntity)) || (b == http.StatusOK && (a == http.StatusConflict || a == http.StatusUnprocessableEntity))) {
		t.Fatalf("racing link/OTP statuses=(%d,%d), want exactly one winner", a, b)
	}
	var verifications int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM campus_verifications WHERE user_id=$1 AND revoked_at IS NULL`, users["first"]).Scan(&verifications); err != nil || verifications != 1 {
		t.Fatalf("successful verification rows=%d err=%v, want one", verifications, err)
	}
	requestChallenge := func(email string) int {
		req := httptest.NewRequest(http.MethodPost, "http://peergit.test/api/v1/me/campus-verification/challenges", strings.NewReader(fmt.Sprintf(`{"campus_email":%q}`, email)))
		req.RemoteAddr = "192.0.2.1:1234"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://peergit.test")
		req.Header.Set("X-CSRF-Token", csrfSecond)
		req.AddCookie(&http.Cookie{Name: "peergit_session", Value: tokens["second"]})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res.Code
	}
	if got := requestChallenge("first@verify.test"); got != http.StatusConflict {
		t.Fatalf("another account's verified campus email status=%d, want 409", got)
	}
	seedBucket := func(key string, count int) {
		t.Helper()
		_, err := pool.Exec(ctx, `INSERT INTO login_rate_limits(bucket_hash,window_started_at,request_count) VALUES($1,now(),$2) ON CONFLICT(bucket_hash) DO UPDATE SET window_started_at=now(),request_count=EXCLUDED.request_count`, verificationHash(hashKey, key), count)
		if err != nil {
			t.Fatal(err)
		}
	}
	clearBucket := func(key string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `DELETE FROM login_rate_limits WHERE bucket_hash=$1`, verificationHash(hashKey, key)); err != nil {
			t.Fatal(err)
		}
	}
	const rateEmail = "fresh@verify.test"
	for _, bucket := range []struct {
		key   string
		count int
	}{{"verify:user:" + users["second"], 5}, {"verify:email:" + rateEmail, 5}, {"verify:ip:192.0.2.1", 20}} {
		seedBucket(bucket.key, bucket.count)
		if got := requestChallenge(rateEmail); got != http.StatusTooManyRequests {
			t.Fatalf("rate bucket %q count=%d status=%d, want 429", bucket.key, bucket.count, got)
		}
		clearBucket(bucket.key)
	}
}

func TestCampusDeliveryLeaseExpiryRetryFencesStaleWorker(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run campus delivery lease integration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := testPool(t, ctx, databaseURL)
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	slug := fmt.Sprintf("delivery-fence-%d", time.Now().UnixNano()%1_000_000_000)
	var campusID, userID, emailID, challengeID, deliveryID string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES($1,'Delivery Fence Campus') RETURNING id::text`, slug).Scan(&campusID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO college_domains(college_id,domain,verified_at) VALUES($1,'delivery.test',now())`, campusID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,email_normalized,display_name,handle,account_type) VALUES($1,$1,'Delivery Student',$2,'unverified') RETURNING id::text`, slug+"@contact.test", strings.ReplaceAll(slug, "-", "_")).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO campus_emails(user_id,college_id,email_normalized) VALUES($1,$2,$3) RETURNING id::text`, userID, campusID, "student@delivery.test").Scan(&emailID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO campus_email_challenges(user_id,college_id,campus_email_id,link_hash,otp_hash,expires_at) VALUES($1,$2,$3,decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),now()+interval '10 minutes') RETURNING id::text`, userID, campusID, emailID).Scan(&challengeID); err != nil {
		t.Fatal(err)
	}
	const encryptionKey = "delivery lease integration encryption key entropy"
	message, err := json.Marshal(map[string]string{"to": "student@delivery.test", "code": "123456", "link": "http://peergit.test/onboarding/verify#token=secret"})
	if err != nil {
		t.Fatal(err)
	}
	key := sha256.Sum256([]byte(encryptionKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatal(err)
	}
	encrypted := append(nonce, gcm.Seal(nil, nonce, message, nil)...)
	if err = pool.QueryRow(ctx, `INSERT INTO campus_email_deliveries(challenge_id,encrypted_payload,expires_at) VALUES($1,$2,now()+interval '10 minutes') RETURNING id::text`, challengeID, encrypted).Scan(&deliveryID); err != nil {
		t.Fatal(err)
	}
	jobPayload, _ := json.Marshal(map[string]string{"delivery_id": deliveryID})
	queue := jobs.Queue{Pool: pool}
	jobID, err := queue.EnqueueWork(ctx, campusID, userID, "campus_verification_email", "delivery-"+slug, jobPayload, nil)
	if err != nil {
		t.Fatal(err)
	}
	oldClaim, err := queue.ClaimTypes(ctx, "mailer-before-crash", 10*time.Millisecond, []string{"campus_verification_email"})
	if err != nil || oldClaim.ID != jobID {
		t.Fatalf("first delivery claim=%+v err=%v", oldClaim, err)
	}
	time.Sleep(50 * time.Millisecond) // Simulate a worker crash after claiming but before SMTP.
	currentClaim, err := queue.ClaimTypes(ctx, "mailer-after-recovery", time.Minute, []string{"campus_verification_email"})
	if err != nil || currentClaim.ID != jobID || currentClaim.Generation <= oldClaim.Generation {
		t.Fatalf("recovered delivery claim=%+v err=%v", currentClaim, err)
	}
	mailHandler := campus.DeliveryHandler(pool, encryptionKey, campus.MailConfig{Host: "127.0.0.1:1", From: "PeerGit <noreply@peergit.test>"})
	if _, err = mailHandler(ctx, oldClaim); !errors.Is(err, jobs.ErrStale) {
		t.Fatalf("stale delivery attempt err=%v, want ErrStale", err)
	}
	if _, err = mailHandler(ctx, currentClaim); err == nil {
		t.Fatal("unavailable SMTP endpoint unexpectedly accepted the message")
	}
	if err = queue.RetryWithCode(ctx, currentClaim, 0, "smtp_unavailable"); err != nil {
		t.Fatalf("queue retry after SMTP failure: %v", err)
	}
	var sent bool
	var encryptedBytes int
	if err = pool.QueryRow(ctx, `SELECT sent_at IS NOT NULL,octet_length(encrypted_payload) FROM campus_email_deliveries WHERE id=$1`, deliveryID).Scan(&sent, &encryptedBytes); err != nil {
		t.Fatal(err)
	}
	var jobState, lastError string
	var attempts int
	if err = pool.QueryRow(ctx, `SELECT state,last_error_code,attempts FROM jobs WHERE id=$1`, jobID).Scan(&jobState, &lastError, &attempts); err != nil {
		t.Fatal(err)
	}
	if sent || encryptedBytes == 0 || jobState != "ready" || lastError != "smtp_unavailable" || attempts != 2 {
		t.Fatalf("delivery retry state sent=%v encrypted=%d job=%s error=%s attempts=%d", sent, encryptedBytes, jobState, lastError, attempts)
	}
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
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,email_normalized,display_name,handle,account_type) VALUES($1,$1,'Rejected Student',$2,'unverified') RETURNING id::text`, slug+"-rejected@contact.test", handlePrefix+"_rejected").Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	const rejectedToken = "review-rejected-student-session"
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,now()+interval '1 hour')`, studentID, sessionHash(key, rejectedToken), sessionCSRFHash(key, rejectedToken)); err != nil {
		t.Fatal(err)
	}
	rejectedCSRF := csrfFor(rejectedToken)
	rejectedRequest := request(rejectedToken, rejectedCSRF, http.MethodPost, "/api/v1/me/campus-review-requests", `{"campus_id":"`+slug+`","reason":"Please review my campus affiliation."}`, http.StatusCreated)
	var rejectedRequestEnvelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rejectedRequest.Body.Bytes(), &rejectedRequestEnvelope); err != nil || rejectedRequestEnvelope.Data.ID == "" {
		t.Fatalf("rejected review request ID missing: %s (%v)", rejectedRequest.Body.String(), err)
	}
	rejectPath := "/api/v1/admin/campus-review-requests/" + rejectedRequestEnvelope.Data.ID + "/decision"
	request(adminToken, adminCSRF, http.MethodPost, rejectPath, `{"decision":"rejected","reason":"The supplied information was not sufficient."}`, http.StatusOK)
	request(adminToken, adminCSRF, http.MethodPost, rejectPath, `{"decision":"approved","reason":"Duplicate decision must fail."}`, http.StatusNotFound)
	var rejectedCampus sql.NullString
	var rejectedRoles, rejectedVerifications int
	if err := pool.QueryRow(ctx, `SELECT college_id::text FROM users WHERE id=$1`, studentID).Scan(&rejectedCampus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM campus_roles WHERE user_id=$1`, studentID).Scan(&rejectedRoles); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM campus_verifications WHERE user_id=$1`, studentID).Scan(&rejectedVerifications); err != nil {
		t.Fatal(err)
	}
	if rejectedCampus.Valid || rejectedRoles != 0 || rejectedVerifications != 0 {
		t.Fatalf("rejected review granted affiliation: campus=%v roles=%d verifications=%d", rejectedCampus, rejectedRoles, rejectedVerifications)
	}
	var otherCampus, otherStudent, otherReview string
	if err := pool.QueryRow(ctx, `INSERT INTO colleges(slug,name) VALUES($1,'Other Review Campus') RETURNING id::text`, slug+"-other").Scan(&otherCampus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,email_normalized,display_name,handle,account_type) VALUES($1,$1,'Other Student',$2,'unverified') RETURNING id::text`, slug+"-other-student@contact.test", handlePrefix+"_other").Scan(&otherStudent); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO campus_review_requests(user_id,college_id,requested_email,reason) VALUES($1,$2,'','Request from another campus') RETURNING id::text`, otherStudent, otherCampus).Scan(&otherReview); err != nil {
		t.Fatal(err)
	}
	request(adminToken, adminCSRF, http.MethodPost, "/api/v1/admin/campus-review-requests/"+otherReview+"/decision", `{"decision":"approved","reason":"Cross-campus review must fail."}`, http.StatusNotFound)
	var otherReviewState string
	if err := pool.QueryRow(ctx, `SELECT state FROM campus_review_requests WHERE id=$1`, otherReview).Scan(&otherReviewState); err != nil || otherReviewState != "pending" {
		t.Fatalf("cross-campus review state=%q err=%v", otherReviewState, err)
	}
	var selfReview string
	if err := pool.QueryRow(ctx, `INSERT INTO campus_review_requests(user_id,college_id,requested_email,reason) VALUES($1,$2,'','Self review negative case') RETURNING id::text`, adminID, campusID).Scan(&selfReview); err != nil {
		t.Fatal(err)
	}
	request(adminToken, adminCSRF, http.MethodPost, "/api/v1/admin/campus-review-requests/"+selfReview+"/decision", `{"decision":"approved","reason":"A user cannot approve their own request."}`, http.StatusForbidden)
}
