package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

type URLSigner struct {
	secretKey []byte
	baseURL   string
}

func NewURLSigner(secretKey, baseURL string) *URLSigner {
	return &URLSigner{
		secretKey: []byte(secretKey),
		baseURL:   baseURL,
	}
}

// GenerateSignedURL generates a tamper-proof expiring URL
func (s *URLSigner) GenerateSignedURL(bucket, path string, duration time.Duration) (string, int64, string) {
	expires := time.Now().Add(duration).Unix()
	sig := s.computeSignature(bucket, path, expires)

	escapedPath := url.PathEscape(path)
	signedURL := fmt.Sprintf("%s/v0/b/%s/o/%s?alt=media&expires=%d&sig=%s", s.baseURL, bucket, escapedPath, expires, sig)
	return signedURL, expires, sig
}

// Verify verifies signature authenticity and checks that current time < expires
func (s *URLSigner) Verify(bucket, path, expiresStr, sigStr string) bool {
	if expiresStr == "" || sigStr == "" {
		return false
	}

	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return false
	}

	// Check expiration
	if time.Now().Unix() > expires {
		return false // Expired
	}

	expectedSig := s.computeSignature(bucket, path, expires)
	return hmac.Equal([]byte(sigStr), []byte(expectedSig))
}

func (s *URLSigner) computeSignature(bucket, path string, expires int64) string {
	message := fmt.Sprintf("%s:%s:%d", bucket, path, expires)
	h := hmac.New(sha256.New, s.secretKey)
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}
