package identity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

func randomToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func keyedHash(key, value []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(value)
	return h.Sum(nil)
}

func sessionCSRFToken(key []byte, sessionToken string) string {
	return base64.RawURLEncoding.EncodeToString(keyedHash(key, []byte("peergit-csrf:"+sessionToken)))
}

func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func newTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

func totp(secret string, at time.Time) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", fmt.Errorf("decode TOTP secret: %w", err)
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(at.Unix()/30))
	h := hmac.New(sha1.New, key)
	_, _ = h.Write(counter[:])
	sum := h.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	n := (binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff) % 1_000_000
	return fmt.Sprintf("%06d", n), nil
}

func verifyTOTP(secret, code string, now time.Time) bool {
	if len(code) != 6 {
		return false
	}
	for _, delta := range []time.Duration{-30 * time.Second, 0, 30 * time.Second} {
		want, err := totp(secret, now.Add(delta))
		if err == nil && hmac.Equal([]byte(want), []byte(code)) {
			return true
		}
	}
	return false
}

func encryptSecret(masterKey, plaintext []byte) ([]byte, error) {
	key := sha256.Sum256(masterKey)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(nonce, aead.Seal(nil, nonce, plaintext, nil)...), nil
}

func decryptSecret(masterKey, ciphertext []byte) ([]byte, error) {
	key := sha256.Sum256(masterKey)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < aead.NonceSize() {
		return nil, fmt.Errorf("invalid encrypted TOTP secret")
	}
	return aead.Open(nil, ciphertext[:aead.NonceSize()], ciphertext[aead.NonceSize():], nil)
}
