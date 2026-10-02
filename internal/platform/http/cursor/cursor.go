package cursor

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

func Encode(key, payload []byte) (string, error) {
	if len(key) < 32 || len(payload) == 0 {
		return "", errors.New("cursor signing key and payload are required")
	}
	body := "v1." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func Decode(key []byte, token string) ([]byte, error) {
	parts := strings.Split(token, ".")
	if len(key) < 32 || len(parts) != 3 || parts[0] != "v1" {
		return nil, errors.New("invalid cursor")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("invalid cursor")
	}
	body := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(body))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, errors.New("invalid cursor")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(payload) == 0 {
		return nil, errors.New("invalid cursor")
	}
	return payload, nil
}
