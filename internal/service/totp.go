package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GenerateTOTPSecret generates a random 20-byte cryptographically secure Base32 secret string
func GenerateTOTPSecret() (string, error) {
	bytes := make([]byte, 20)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(bytes), "="), nil
}

// GenerateOTPAuthURI creates a standard RFC 6238 key URI for Google Authenticator, Authy, etc.
func GenerateOTPAuthURI(accountName, issuer, secret string) string {
	if issuer == "" {
		issuer = "Prolead File"
	}
	label := fmt.Sprintf("%s:%s", issuer, accountName)
	return fmt.Sprintf("otpauth://totp/%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30",
		url.PathEscape(label),
		secret,
		url.QueryEscape(issuer),
	)
}

// GenerateTOTPCode computes the standard 6-digit TOTP code for a given secret and timestamp
func GenerateTOTPCode(secret string, t time.Time) (string, error) {
	// Pad secret if needed
	secret = strings.ToUpper(strings.TrimSpace(secret))
	missingPadding := len(secret) % 8
	if missingPadding > 0 {
		secret += strings.Repeat("=", 8-missingPadding)
	}

	key, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("invalid base32 secret: %w", err)
	}

	interval := uint64(t.Unix() / 30)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], interval)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	hash := mac.Sum(nil)

	// Dynamic truncation (RFC 4226 Section 5.4)
	offset := hash[len(hash)-1] & 0x0f
	binaryCode := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	code := binaryCode % 1000000

	return fmt.Sprintf("%06d", code), nil
}

// ValidateTOTPCode verifies whether a provided 6-digit code matches the secret, allowing +/- 1 time interval skew
func ValidateTOTPCode(secret string, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}

	if _, err := strconv.Atoi(code); err != nil {
		return false
	}

	now := time.Now().UTC()
	// Test current step, previous step (-30s), and next step (+30s)
	skewTimes := []time.Time{
		now,
		now.Add(-30 * time.Second),
		now.Add(30 * time.Second),
	}

	for _, t := range skewTimes {
		expected, err := GenerateTOTPCode(secret, t)
		if err == nil && hmac.Equal([]byte(code), []byte(expected)) {
			return true
		}
	}

	return false
}
