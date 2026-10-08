package proleadfile

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// VerifyWebhookSignature verifies that an incoming HTTP webhook signature matches the expected payload HMAC-SHA256.
func VerifyWebhookSignature(payload []byte, headerSignature, secret string) (bool, error) {
	if secret == "" {
		return false, fmt.Errorf("webhook secret cannot be empty")
	}
	if headerSignature == "" {
		return false, fmt.Errorf("header signature cannot be empty")
	}

	cleanSig := strings.TrimPrefix(headerSignature, "sha256=")
	cleanSig = strings.TrimSpace(cleanSig)

	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write(payload); err != nil {
		return false, err
	}
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(cleanSig), []byte(expectedMAC)), nil
}
