package repository

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestWebhookSignatureUsesExactRawBody(t *testing.T) {
	secret := "a sufficiently long webhook test secret"
	body := []byte("{\"action\":\"created\", \"installation\":{\"id\":42}}\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	header := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !validWebhookSignature(secret, body, header) {
		t.Fatal("valid raw-body HMAC was rejected")
	}
	if validWebhookSignature(secret, []byte("{\"action\":\"created\",\"installation\":{\"id\":42}}\n"), header) {
		t.Fatal("signature for different raw bytes was accepted")
	}
	for _, invalid := range []string{"", "sha1=" + hex.EncodeToString(mac.Sum(nil)), "sha256=not-hex"} {
		if validWebhookSignature(secret, body, invalid) {
			t.Fatalf("invalid signature header %q was accepted", invalid)
		}
	}
	if validWebhookSignature("short", body, header) {
		t.Fatal("short secret was accepted")
	}
}
