package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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

	token := "usr_" + generateRandomToken()
	s.mu.Lock()
	s.sessions[token] = &userSession{
		User:      user,
		ExpiresAt: time.Now().Add(24 * time.Hour),
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

	s.mu.RLock()
	sess, exists := s.sessions[token]
	s.mu.RUnlock()

	if !exists || sess == nil {
		return nil
	}

	if time.Now().After(sess.ExpiresAt) {
		s.mu.Lock()
		delete(s.sessions, token)
		s.mu.Unlock()
		return nil
	}

	return sess.User
}

func generateRandomToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
