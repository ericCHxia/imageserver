package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"imageserver/config"
)

func hmacVerify(cfg config.HMAC, message string, messageMAC string) bool {
	mac := hmac.New(sha256.New, []byte(cfg.SecretKey))
	mac.Write([]byte(message))
	mac.Write([]byte(cfg.Salt))
	expectedMAC := mac.Sum(nil)
	messageMACBytes, err := base64.RawURLEncoding.DecodeString(messageMAC)
	if err != nil {
		return false
	}
	return hmac.Equal(messageMACBytes, expectedMAC)
}
