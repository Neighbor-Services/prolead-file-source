package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gostore/internal/core/domain"
	"gostore/internal/core/ports"
	"gostore/internal/repository/sqlite"
)

type ShareService struct {
	shareRepo *sqlite.ShareRepository
	fileRepo  *sqlite.FileRepository
	storage   ports.StorageDriver
	baseURL   string
}

type CreateShareRequest struct {
	Bucket       string     `json:"bucket"`
	Path         string     `json:"path"`
	Password     string     `json:"password,omitempty"`
	MaxDownloads int        `json:"maxDownloads,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	CreatedBy    string     `json:"createdBy,omitempty"`
}

type ShareResponse struct {
	ID            string     `json:"id"`
	Token         string     `json:"token"`
	Bucket        string     `json:"bucket"`
	Path          string     `json:"path"`
	HasPassword   bool       `json:"hasPassword"`
	MaxDownloads  int        `json:"maxDownloads"`
	DownloadCount int        `json:"downloadCount"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	ShareURL      string     `json:"shareUrl"`
	CreatedAt     time.Time  `json:"createdAt"`
}

func NewShareService(
	shareRepo *sqlite.ShareRepository,
	fileRepo *sqlite.FileRepository,
	storage ports.StorageDriver,
	baseURL string,
) *ShareService {
	return &ShareService{
		shareRepo: shareRepo,
		fileRepo:  fileRepo,
		storage:   storage,
		baseURL:   baseURL,
	}
}

func (s *ShareService) CreateShareLink(ctx context.Context, req CreateShareRequest) (*ShareResponse, error) {
	// Verify file exists
	file, err := s.fileRepo.GetByPath(ctx, req.Bucket, req.Path)
	if err != nil || file == nil {
		return nil, errors.New("target file not found")
	}

	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)

	var passwordHash string
	hasPassword := false
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		passwordHash = string(hash)
		hasPassword = true
	}

	share := &domain.ShareLink{
		ID:            uuid.New().String(),
		Token:         token,
		Bucket:        req.Bucket,
		Path:          req.Path,
		PasswordHash:  passwordHash,
		HasPassword:   hasPassword,
		MaxDownloads:  req.MaxDownloads,
		DownloadCount: 0,
		ExpiresAt:     req.ExpiresAt,
		CreatedBy:     req.CreatedBy,
	}

	if err := s.shareRepo.Create(ctx, share); err != nil {
		return nil, err
	}

	return s.toResponse(share), nil
}

func (s *ShareService) GetShareInfo(ctx context.Context, token string) (*ShareResponse, *domain.FileObject, error) {
	share, err := s.shareRepo.GetByToken(ctx, token)
	if err != nil || share == nil {
		return nil, nil, errors.New("share link not found or expired")
	}

	if share.ExpiresAt != nil && share.ExpiresAt.Before(time.Now().UTC()) {
		return nil, nil, errors.New("share link has expired")
	}

	if share.MaxDownloads > 0 && share.DownloadCount >= share.MaxDownloads {
		return nil, nil, errors.New("maximum download limit reached for this share link")
	}

	file, err := s.fileRepo.GetByPath(ctx, share.Bucket, share.Path)
	if err != nil || file == nil {
		return nil, nil, errors.New("shared file no longer exists")
	}

	return s.toResponse(share), file, nil
}

func (s *ShareService) DownloadSharedFile(ctx context.Context, token, password string) (ports.ReadSeekCloser, *domain.FileObject, error) {
	share, err := s.shareRepo.GetByToken(ctx, token)
	if err != nil || share == nil {
		return nil, nil, errors.New("share link not found")
	}

	if share.ExpiresAt != nil && share.ExpiresAt.Before(time.Now().UTC()) {
		return nil, nil, errors.New("share link has expired")
	}

	if share.MaxDownloads > 0 && share.DownloadCount >= share.MaxDownloads {
		return nil, nil, errors.New("download limit reached")
	}

	if share.HasPassword {
		if password == "" {
			return nil, nil, errors.New("password required")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(share.PasswordHash), []byte(password)); err != nil {
			return nil, nil, errors.New("invalid password")
		}
	}

	file, err := s.fileRepo.GetByPath(ctx, share.Bucket, share.Path)
	if err != nil || file == nil {
		return nil, nil, errors.New("shared file not found")
	}

	reader, _, err := s.storage.Open(ctx, share.Bucket, share.Path)
	if err != nil {
		return nil, nil, err
	}

	_ = s.shareRepo.IncrementDownload(ctx, token)
	return reader, file, nil
}

func (s *ShareService) DeleteShareLink(ctx context.Context, token string) error {
	return s.shareRepo.Delete(ctx, token)
}

func (s *ShareService) ListByBucket(ctx context.Context, bucket string) ([]ShareResponse, error) {
	shares, err := s.shareRepo.ListByBucket(ctx, bucket)
	if err != nil {
		return nil, err
	}
	var res []ShareResponse
	for _, sh := range shares {
		res = append(res, *s.toResponse(&sh))
	}
	return res, nil
}

func (s *ShareService) toResponse(sh *domain.ShareLink) *ShareResponse {
	return &ShareResponse{
		ID:            sh.ID,
		Token:         sh.Token,
		Bucket:        sh.Bucket,
		Path:          sh.Path,
		HasPassword:   sh.HasPassword,
		MaxDownloads:  sh.MaxDownloads,
		DownloadCount: sh.DownloadCount,
		ExpiresAt:     sh.ExpiresAt,
		ShareURL:      s.baseURL + "/s/" + sh.Token,
		CreatedAt:     sh.CreatedAt,
	}
}
