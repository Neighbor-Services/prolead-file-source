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

type AuthService struct {
	userRepo     *sqlite.UserRepository
	masterAPIKey string
	sessions     map[string]*userSession
	mu           sync.RWMutex
}

func NewAuthService(userRepo *sqlite.UserRepository, masterAPIKey string) *AuthService {
	return &AuthService{
		userRepo:     userRepo,
		masterAPIKey: masterAPIKey,
		sessions:     make(map[string]*userSession),
	}
}

type LoginResult struct {
	Token   string       `json:"token"`
	User    *domain.User `json:"user"`
	IsAdmin bool         `json:"isAdmin"`
}

func (s *AuthService) Login(ctx context.Context, username, password string) (*LoginResult, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)

	if username == "" || password == "" {
		return nil, errors.New("username and password are required")
	}

	// Strictly authenticate against database users (created via createsuperuser CLI)
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil || user == nil {
		return nil, errors.New("invalid username or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid username or password")
	}

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
		IsAdmin: user.IsSuperuser,
	}, nil
}

func (s *AuthService) Logout(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func (s *AuthService) CreateSuperuser(ctx context.Context, username, email, password string) (*domain.User, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return nil, errors.New("username and password cannot be empty")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		ID:           uuid.New().String(),
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		IsSuperuser:  true,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *AuthService) ValidateToken(token string) *domain.User {
	if token == s.masterAPIKey && s.masterAPIKey != "" {
		return &domain.User{
			ID:          "usr-master",
			Username:    "master-admin",
			IsSuperuser: true,
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

