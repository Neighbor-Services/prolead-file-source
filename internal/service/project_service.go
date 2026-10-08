package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gostore/internal/core/domain"
	"gostore/internal/repository/sqlite"
)

type ProjectService struct {
	repo       *sqlite.ProjectRepository
	bucketRepo *sqlite.BucketRepository
}

func NewProjectService(repo *sqlite.ProjectRepository, bucketRepo *sqlite.BucketRepository) *ProjectService {
	return &ProjectService{
		repo:       repo,
		bucketRepo: bucketRepo,
	}
}

func (s *ProjectService) CreateProject(ctx context.Context, name, description string) (*domain.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("project name cannot be empty")
	}

	slug := slugify(name)
	if slug == "" {
		slug = "project-" + uuid.New().String()[:6]
	}

	p := &domain.Project{
		ID:          "p-" + uuid.New().String()[:8],
		Name:        name,
		Slug:        slug,
		Description: description,
	}

	if err := s.repo.Create(ctx, p); err != nil {
		// If slug collision, append random suffix
		p.Slug = fmt.Sprintf("%s-%s", slug, uuid.New().String()[:4])
		if err := s.repo.Create(ctx, p); err != nil {
			return nil, err
		}
	}

	// Auto-provision an isolated default bucket for this project
	if s.bucketRepo != nil {
		bucketName := p.Slug
		existing, _ := s.bucketRepo.GetByName(ctx, bucketName)
		if existing != nil {
			bucketName = fmt.Sprintf("%s-%s", p.Slug, uuid.New().String()[:4])
		}
		_ = s.bucketRepo.Create(ctx, &domain.Bucket{
			ID:          "b-" + uuid.New().String()[:8],
			ProjectID:   p.ID,
			Name:        bucketName,
			Description: fmt.Sprintf("Primary storage bucket for %s", p.Name),
			IsPublic:    true,
		})
	}

	return p, nil
}

func (s *ProjectService) ListProjects(ctx context.Context) ([]domain.Project, error) {
	return s.repo.List(ctx)
}

func (s *ProjectService) GetProject(ctx context.Context, id string) (*domain.Project, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ProjectService) DeleteProject(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func slugify(s string) string {
	s = strings.ToLower(s)
	reg := regexp.MustCompile("[^a-z0-9]+")
	s = reg.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
