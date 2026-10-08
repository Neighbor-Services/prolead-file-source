package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gostore/internal/core/domain"
	"gostore/internal/core/ports"
	"gostore/internal/repository/sqlite"

	"github.com/google/uuid"
)

type BucketService struct {
	bucketRepo  *sqlite.BucketRepository
	fileRepo    *sqlite.FileRepository
	projectRepo *sqlite.ProjectRepository
	storage     ports.StorageDriver
}

func NewBucketService(bucketRepo *sqlite.BucketRepository, fileRepo *sqlite.FileRepository, projectRepo *sqlite.ProjectRepository, storage ports.StorageDriver) *BucketService {
	return &BucketService{
		bucketRepo:  bucketRepo,
		fileRepo:    fileRepo,
		projectRepo: projectRepo,
		storage:     storage,
	}
}

func (s *BucketService) CreateBucket(ctx context.Context, b *domain.Bucket) (*domain.Bucket, error) {
	name := strings.ToLower(strings.TrimSpace(b.Name))
	if name == "" {
		return nil, errors.New("bucket name is required")
	}

	if b.ProjectID == "" {
		b.ProjectID = "p-default"
	}

	// Validate bucket name (alphanumeric and dashes)
	for _, ch := range name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return nil, errors.New("bucket name must contain only lowercase alphanumeric characters, dashes, or underscores")
		}
	}

	existing, err := s.bucketRepo.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.New("bucket already exists")
	}

	b.ID = uuid.New().String()
	b.Name = name
	b.CreatedAt = time.Now().UTC()
	b.UpdatedAt = time.Now().UTC()

	if err := s.bucketRepo.Create(ctx, b); err != nil {
		return nil, err
	}

	return b, nil
}

func (s *BucketService) GetBucket(ctx context.Context, name string) (*domain.Bucket, error) {
	b, err := s.bucketRepo.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, ErrBucketNotFound
	}
	return b, nil
}

func (s *BucketService) ListBuckets(ctx context.Context, projectID string) ([]domain.Bucket, error) {
	if projectID != "" {
		resolvedPID := projectID
		if s.projectRepo != nil {
			p, err := s.projectRepo.GetByID(ctx, projectID)
			if err == nil && p != nil {
				resolvedPID = p.ID
			}
		}

		buckets, err := s.bucketRepo.List(ctx, resolvedPID)
		if err != nil {
			return nil, err
		}

		// If this is a valid project that doesn't have a bucket yet, auto-provision one for it
		if len(buckets) == 0 && s.projectRepo != nil {
			p, err := s.projectRepo.GetByID(ctx, resolvedPID)
			if err == nil && p != nil {
				bucketName := p.Slug
				existing, _ := s.bucketRepo.GetByName(ctx, bucketName)
				if existing != nil && existing.ProjectID != p.ID {
					bucketName = fmt.Sprintf("%s-%s", p.Slug, uuid.New().String()[:4])
				}
				newBucket := &domain.Bucket{
					ID:          "b-" + uuid.New().String()[:8],
					ProjectID:   p.ID,
					Name:        bucketName,
					Description: fmt.Sprintf("Primary storage bucket for %s", p.Name),
					IsPublic:    true,
				}
				if err := s.bucketRepo.Create(ctx, newBucket); err == nil {
					buckets = append(buckets, *newBucket)
				}
			}
		}

		return buckets, nil
	}

	return s.bucketRepo.List(ctx, "")
}

func (s *BucketService) DeleteBucket(ctx context.Context, name string) error {
	if name == "default" {
		return errors.New("cannot delete default bucket")
	}

	bucket, err := s.bucketRepo.GetByName(ctx, name)
	if err != nil {
		return err
	}
	if bucket == nil {
		return ErrBucketNotFound
	}

	// Delete from storage
	_ = s.storage.DeleteBucket(ctx, name)

	// Delete from repository
	return s.bucketRepo.Delete(ctx, name)
}

func (s *BucketService) UpdateBucket(ctx context.Context, b *domain.Bucket) (*domain.Bucket, error) {
	existing, err := s.bucketRepo.GetByName(ctx, b.Name)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrBucketNotFound
	}

	existing.Description = b.Description
	existing.IsPublic = b.IsPublic
	existing.MaxFileSize = b.MaxFileSize
	existing.AllowedMimes = b.AllowedMimes

	if err := s.bucketRepo.Update(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *BucketService) GetStats(ctx context.Context) (*domain.StorageStats, error) {
	return s.bucketRepo.GetStats(ctx)
}
