package campus

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"testing"
	"time"
)

func TestVerifyResendSignature(t *testing.T) {
	secretBytes := []byte("webhook-test-secret")
	secret := "whsec_" + base64.StdEncoding.EncodeToString(secretBytes)
	id := "msg_test"
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	body := []byte(`{"type":"email.delivered"}`)
	mac := hmac.New(sha256.New, secretBytes)
	_, _ = mac.Write([]byte(id + "." + timestamp + "."))
	_, _ = mac.Write(body)
	signature := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !verifyResendSignature(secret, id, timestamp, signature, body, time.Now()) {
		t.Fatal("valid Resend signature was rejected")
	}
	if verifyResendSignature(secret, id, timestamp, signature, append(body, ' '), time.Now()) {
		t.Fatal("modified payload was accepted")
	}
	if verifyResendSignature(secret, id, strconv.FormatInt(time.Now().Add(-6*time.Minute).Unix(), 10), signature, body, time.Now()) {
		t.Fatal("stale signature was accepted")
	}
}
