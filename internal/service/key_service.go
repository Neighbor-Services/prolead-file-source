package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"gostore/internal/core/domain"
	"gostore/internal/repository/sqlite"
)

type KeyService struct {
	keyRepo      *sqlite.APIKeyRepository
	masterAPIKey string
}

func NewKeyService(keyRepo *sqlite.APIKeyRepository, masterKey string) *KeyService {
	return &KeyService{
		keyRepo:      keyRepo,
		masterAPIKey: masterKey,
	}
}

type GenerateKeyInput struct {
	ProjectID          string     `json:"projectId"`
	Name               string     `json:"name"`
	Role               string     `json:"role"`
	Permissions        []string   `json:"permissions"`
	AllowedBuckets     []string   `json:"allowedBuckets"`
	AllowedOrigins     []string   `json:"allowedOrigins"`
	RateLimitReqPerMin int        `json:"rateLimitReqPerMin"`
	ExpiresAt          *time.Time `json:"expiresAt"`
	CreatedBy          string     `json:"createdBy"`
}

func (s *KeyService) GenerateKey(ctx context.Context, input GenerateKeyInput) (*domain.APIKey, error) {
	if input.Role == "" {
		input.Role = "read-write"
	}
	if input.ProjectID == "" {
		input.ProjectID = "p-default"
	}
	if input.Name == "" {
		input.Name = "Default Application Key"
	}

	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	rawKey := "gst_" + hex.EncodeToString(bytes)
	keyPrefix := rawKey[:10] + "..." + rawKey[len(rawKey)-4:]

	apiKey := &domain.APIKey{
		ID:                 uuid.New().String(),
		ProjectID:          input.ProjectID,
		Key:                rawKey,
		KeyPrefix:          keyPrefix,
		Name:               input.Name,
		Role:               input.Role,
		Permissions:        input.Permissions,
		AllowedBuckets:     input.AllowedBuckets,
		AllowedOrigins:     input.AllowedOrigins,
		RateLimitReqPerMin: input.RateLimitReqPerMin,
		RequestCount:       0,
		ExpiresAt:          input.ExpiresAt,
		Revoked:            false,
		CreatedBy:          input.CreatedBy,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}

	if err := s.keyRepo.Create(ctx, apiKey); err != nil {
		return nil, err
	}

	return apiKey, nil
}

func (s *KeyService) RotateKey(ctx context.Context, id string) (*domain.APIKey, error) {
	apiKey, err := s.keyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if apiKey == nil {
		return nil, errors.New("key not found")
	}

	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	rawKey := "gst_" + hex.EncodeToString(bytes)
	apiKey.Key = rawKey
	apiKey.KeyPrefix = rawKey[:10] + "..." + rawKey[len(rawKey)-4:]
	apiKey.UpdatedAt = time.Now().UTC()

	if err := s.keyRepo.Update(ctx, apiKey); err != nil {
		return nil, err
	}

	return apiKey, nil
}

func (s *KeyService) UpdateKey(ctx context.Context, id string, name, role string, allowedBuckets, allowedOrigins, permissions []string, rateLimit int, expiresAt *time.Time) (*domain.APIKey, error) {
	apiKey, err := s.keyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if apiKey == nil {
		return nil, errors.New("key not found")
	}

	if name != "" {
		apiKey.Name = name
	}
	if role != "" {
		apiKey.Role = role
	}
	apiKey.AllowedBuckets = allowedBuckets
	apiKey.AllowedOrigins = allowedOrigins
	apiKey.Permissions = permissions
	apiKey.RateLimitReqPerMin = rateLimit
	apiKey.ExpiresAt = expiresAt
	apiKey.UpdatedAt = time.Now().UTC()

	if err := s.keyRepo.Update(ctx, apiKey); err != nil {
		return nil, err
	}

	return apiKey, nil
}

func (s *KeyService) ValidateKey(ctx context.Context, key string) (*domain.APIKey, error) {
	if key == "" {
		return nil, ErrUnauthorized
	}

	// Check master API key
	if s.masterAPIKey != "" && key == s.masterAPIKey {
		return &domain.APIKey{
			ID:             "master-admin",
			Key:            key,
			Name:           "Master Admin Key",
			Role:           "admin",
			AllowedBuckets: nil, // all buckets
			CreatedAt:      time.Now().UTC(),
			Revoked:        false,
		}, nil
	}

	apiKey, err := s.keyRepo.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	if apiKey == nil || apiKey.Revoked {
		return nil, ErrUnauthorized
	}

	// Check expiration
	if apiKey.ExpiresAt != nil && time.Now().UTC().After(*apiKey.ExpiresAt) {
		return nil, errors.New("API key has expired")
	}

	// Asynchronously record telemetry usage
	go s.keyRepo.RecordUsage(context.Background(), apiKey.ID)

	return apiKey, nil
}

func (s *KeyService) ListKeys(ctx context.Context, projectID string) ([]domain.APIKey, error) {
	return s.keyRepo.List(ctx, projectID)
}

func (s *KeyService) RevokeKey(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("key ID is required")
	}
	return s.keyRepo.Revoke(ctx, id)
}

func (s *KeyService) ToggleRevoke(ctx context.Context, id string, revoked bool) error {
	if id == "" {
		return errors.New("key ID is required")
	}
	return s.keyRepo.ToggleRevoke(ctx, id, revoked)
}

func (s *KeyService) DeleteKey(ctx context.Context, id string) error {
	return s.keyRepo.Delete(ctx, id)
}
