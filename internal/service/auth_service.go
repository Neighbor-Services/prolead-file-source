package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gostore/internal/core/domain"
	"gostore/internal/repository/sqlite"
)

type userSession struct {
	User      *domain.User
	ExpiresAt time.Time
}

type temp2FASession struct {
	User      *domain.User
	ExpiresAt time.Time
}

type AuthService struct {
	userRepo     *sqlite.UserRepository
	masterAPIKey string
	sessions     map[string]*userSession
	temp2FASess  map[string]*temp2FASession
	mu           sync.RWMutex
}

func NewAuthService(userRepo *sqlite.UserRepository, masterAPIKey string) *AuthService {
	return &AuthService{
		userRepo:     userRepo,
		masterAPIKey: masterAPIKey,
		sessions:     make(map[string]*userSession),
		temp2FASess:  make(map[string]*temp2FASession),
	}
}

type LoginResult struct {
	Token       string       `json:"token,omitempty"`
	User        *domain.User `json:"user,omitempty"`
	IsAdmin     bool         `json:"isAdmin"`
	Require2FA  bool         `json:"require2fa,omitempty"`
	TempToken   string       `json:"tempToken,omitempty"`
}

type TwoFactorSetupResult struct {
	Secret        string   `json:"secret"`
	OTPAuthURI    string   `json:"otpAuthUri"`
	RecoveryCodes []string `json:"recoveryCodes"`
}

func (s *AuthService) Login(ctx context.Context, username, password, totpCode string) (*LoginResult, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)

	if username == "" || password == "" {
		return nil, errors.New("username and password are required")
	}

	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil || user == nil {
		return nil, errors.New("invalid username or password")
	}

	if user.Status == "suspended" {
		return nil, errors.New("account is suspended. Please contact the administrator.")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid username or password")
	}

	// If 2FA is enabled on this account
	if user.TwoFactorEnabled && user.TwoFactorSecret != "" {
		if totpCode == "" {
			// Generate temporary challenge token valid for 5 minutes
			tempToken := "2fa_" + generateRandomToken()
			s.mu.Lock()
			s.temp2FASess[tempToken] = &temp2FASession{
				User:      user,
				ExpiresAt: time.Now().Add(5 * time.Minute),
			}
			s.mu.Unlock()

			return &LoginResult{
				Require2FA: true,
				TempToken:  tempToken,
				User: &domain.User{
					ID:               user.ID,
					Username:         user.Username,
					Email:            user.Email,
					Role:             user.Role,
					TwoFactorEnabled: true,
				},
			}, nil
		}

		// Validate 2FA TOTP code
		if !ValidateTOTPCode(user.TwoFactorSecret, totpCode) {
			return nil, errors.New("invalid two-factor authentication code")
		}
	}

	// Update last login timestamp
	now := time.Now().UTC()
	user.LastLoginAt = &now
	_ = s.userRepo.Update(ctx, user)

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	token := s.generateSignedToken(user, expiresAt)

	s.mu.Lock()
	s.sessions[token] = &userSession{
		User:      user,
		ExpiresAt: expiresAt,
	}
	s.mu.Unlock()

	return &LoginResult{
		Token:   token,
		User:    user,
		IsAdmin: user.IsSuperuser || user.Role == "superadmin" || user.Role == "admin",
	}, nil
}

func (s *AuthService) Verify2FALogin(ctx context.Context, tempToken string, totpCode string) (*LoginResult, error) {
	tempToken = strings.TrimSpace(tempToken)
	totpCode = strings.TrimSpace(totpCode)

	if tempToken == "" || totpCode == "" {
		return nil, errors.New("two-factor authentication code is required")
	}

	s.mu.Lock()
	sess, exists := s.temp2FASess[tempToken]
	if exists {
		delete(s.temp2FASess, tempToken)
	}
	s.mu.Unlock()

	if !exists || sess == nil || time.Now().After(sess.ExpiresAt) {
		return nil, errors.New("2FA verification session expired. Please log in again.")
	}

	user, err := s.userRepo.GetByID(ctx, sess.User.ID)
	if err != nil || user == nil {
		return nil, errors.New("user account not found")
	}

	if !ValidateTOTPCode(user.TwoFactorSecret, totpCode) {
		return nil, errors.New("invalid two-factor authentication code")
	}

	now := time.Now().UTC()
	user.LastLoginAt = &now
	_ = s.userRepo.Update(ctx, user)

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	token := s.generateSignedToken(user, expiresAt)

	s.mu.Lock()
	s.sessions[token] = &userSession{
		User:      user,
		ExpiresAt: expiresAt,
	}
	s.mu.Unlock()

	return &LoginResult{
		Token:   token,
		User:    user,
		IsAdmin: user.IsSuperuser || user.Role == "superadmin" || user.Role == "admin",
	}, nil
}

func (s *AuthService) Setup2FA(ctx context.Context, userID string) (*TwoFactorSetupResult, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}

	secret, err := GenerateTOTPSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate 2FA secret: %w", err)
	}

	otpAuthURI := GenerateOTPAuthURI(user.Username, "Prolead File", secret)
	recoveryCodes := []string{
		generateRandomCode(),
		generateRandomCode(),
		generateRandomCode(),
		generateRandomCode(),
	}

	return &TwoFactorSetupResult{
		Secret:        secret,
		OTPAuthURI:    otpAuthURI,
		RecoveryCodes: recoveryCodes,
	}, nil
}

func (s *AuthService) VerifyAndEnable2FA(ctx context.Context, userID, code, secret string) error {
	code = strings.TrimSpace(code)
	secret = strings.TrimSpace(secret)
	if code == "" || secret == "" {
		return errors.New("code and secret are required")
	}

	if !ValidateTOTPCode(secret, code) {
		return errors.New("verification failed: invalid 6-digit TOTP code")
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return errors.New("user not found")
	}

	user.TwoFactorEnabled = true
	user.TwoFactorSecret = secret
	return s.userRepo.Update(ctx, user)
}

func (s *AuthService) Disable2FA(ctx context.Context, userID, code string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return errors.New("user not found")
	}

	if user.TwoFactorEnabled && user.TwoFactorSecret != "" {
		if !ValidateTOTPCode(user.TwoFactorSecret, code) {
			return errors.New("invalid verification code to disable 2FA")
		}
	}

	user.TwoFactorEnabled = false
	user.TwoFactorSecret = ""
	return s.userRepo.Update(ctx, user)
}

// Admin Staff Management
func (s *AuthService) ListStaff(ctx context.Context) ([]domain.User, error) {
	return s.userRepo.List(ctx)
}

func (s *AuthService) CreateStaff(ctx context.Context, username, email, password, role string) (*domain.User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	password = strings.TrimSpace(password)
	role = strings.ToLower(strings.TrimSpace(role))

	if username == "" || password == "" {
		return nil, errors.New("username and password are required")
	}
	if role == "" {
		role = "admin"
	}

	existing, _ := s.userRepo.GetByUsername(ctx, username)
	if existing != nil {
		return nil, errors.New("user with this username or email already exists")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	isSuperuser := role == "superadmin"

	user := &domain.User{
		ID:               uuid.New().String(),
		Username:         username,
		Email:            email,
		PasswordHash:     string(hash),
		Role:             role,
		IsSuperuser:      isSuperuser,
		TwoFactorEnabled: false,
		Status:           "active",
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *AuthService) UpdateStaff(ctx context.Context, id, role, status string) (*domain.User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil || user == nil {
		return nil, errors.New("staff user not found")
	}

	if role != "" {
		user.Role = strings.ToLower(strings.TrimSpace(role))
		user.IsSuperuser = user.Role == "superadmin"
	}
	if status != "" {
		user.Status = strings.ToLower(strings.TrimSpace(status))
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *AuthService) DeleteStaff(ctx context.Context, id string) error {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil || user == nil {
		return errors.New("staff user not found")
	}
	return s.userRepo.Delete(ctx, id)
}

func (s *AuthService) ResetStaffPassword(ctx context.Context, id, newPassword string) error {
	newPassword = strings.TrimSpace(newPassword)
	if len(newPassword) < 6 {
		return errors.New("password must be at least 6 characters long")
	}

	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil || user == nil {
		return errors.New("staff user not found")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.PasswordHash = string(hash)
	return s.userRepo.Update(ctx, user)
}

func (s *AuthService) Logout(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func (s *AuthService) CreateSuperuser(ctx context.Context, username, email, password string) (*domain.User, error) {
	return s.CreateStaff(ctx, username, email, password, "superadmin")
}

func (s *AuthService) ValidateToken(token string) *domain.User {
	if token == s.masterAPIKey && s.masterAPIKey != "" {
		return &domain.User{
			ID:          "usr-master",
			Username:    "master-admin",
			Role:        "superadmin",
			IsSuperuser: true,
			Status:      "active",
		}
	}

	// 1. Fast path: check in-memory cache
	s.mu.RLock()
	sess, exists := s.sessions[token]
	s.mu.RUnlock()

	if exists && sess != nil {
		if time.Now().Before(sess.ExpiresAt) {
			return sess.User
		}
		s.mu.Lock()
		delete(s.sessions, token)
		s.mu.Unlock()
	}

	// 2. Stateless cryptographic verification (works across cluster nodes and restarts)
	if strings.HasPrefix(token, "usr_") {
		raw := strings.TrimPrefix(token, "usr_")
		parts := strings.Split(raw, ".")
		if len(parts) == 2 {
			b64Payload, sig := parts[0], parts[1]
			secret := s.masterAPIKey
			if secret == "" {
				secret = "gostore-default-signing-secret"
			}
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte(b64Payload))
			expectedSig := hex.EncodeToString(mac.Sum(nil))

			if hmac.Equal([]byte(sig), []byte(expectedSig)) {
				payloadBytes, err := base64.RawURLEncoding.DecodeString(b64Payload)
				if err == nil {
					payloadParts := strings.Split(string(payloadBytes), ":")
					if len(payloadParts) >= 4 {
						id := payloadParts[0]
						username := payloadParts[1]
						isSuperuser := payloadParts[2] == "true"
						expiresUnix, _ := strconv.ParseInt(payloadParts[3], 10, 64)

						if time.Now().Unix() < expiresUnix {
							user := &domain.User{
								ID:          id,
								Username:    username,
								IsSuperuser: isSuperuser,
								Role:        "admin",
								Status:      "active",
							}
							s.mu.Lock()
							s.sessions[token] = &userSession{
								User:      user,
								ExpiresAt: time.Unix(expiresUnix, 0),
							}
							s.mu.Unlock()
							return user
						}
					}
				}
			}
		}
	}

	return nil
}

func (s *AuthService) generateSignedToken(user *domain.User, expiresAt time.Time) string {
	payload := fmt.Sprintf("%s:%s:%t:%d", user.ID, user.Username, user.IsSuperuser, expiresAt.Unix())
	b64Payload := base64.RawURLEncoding.EncodeToString([]byte(payload))
	secret := s.masterAPIKey
	if secret == "" {
		secret = "gostore-default-signing-secret"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(b64Payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return "usr_" + b64Payload + "." + sig
}

func generateRandomToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func generateRandomCode() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%04X-%04X", b[:2], b[2:])
}
